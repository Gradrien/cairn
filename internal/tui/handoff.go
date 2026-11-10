package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"cairn/internal/chantier"
	"cairn/internal/config"
	"cairn/internal/herdr"
	"cairn/internal/i18n"
	"cairn/internal/journal"
)

// An agent's context grows with everything it reads, and all of it is sent
// again at every turn. Rather than compacting it, cairn hands the chantier to
// a new agent: the plan is the chantier's memory, git holds the code, and the
// old agent writes the rest in "## Reprise" before it goes. Two steps, both
// yours: R asks for the handoff, R again starts the new agent.

// contextHeavy is the context size from which cairn suggests a new agent.
const contextHeavy = 120_000

// claudeAgent is the chantier's Claude agent, if it has one.
func claudeAgent(c *chantier.Chantier) (herdr.Agent, bool) {
	for _, a := range c.Agents {
		if a.Kind == "claude" {
			return a, true
		}
	}
	return herdr.Agent{}, false
}

// canHandoff: a Claude agent works on a plan that is being built or reviewed.
func (m Model) canHandoff(c *chantier.Chantier) bool {
	_, ok := claudeAgent(c)
	return ok && c.State != nil && (c.Phase == chantier.Build || c.Phase == chantier.Review)
}

// handoffAsked: the agent was asked for its handoff and has not been replaced.
func (m Model) handoffAsked(c *chantier.Chantier) bool {
	_, ok := m.handoffs[c.Key]
	return ok
}

// handoffReady: the handoff is written and the agent stopped working.
func (m Model) handoffReady(c *chantier.Chantier) bool {
	if !m.handoffAsked(c) || c.State == nil || c.State.Reprise == "" {
		return false
	}
	a, ok := claudeAgent(c)
	return ok && a.Status != "working" && !c.State.UpdatedAt.Before(m.handoffs[c.Key])
}

func (m Model) handoffButton(c *chantier.Chantier) button {
	if m.handoffReady(c) {
		return button{"R", i18n.T("Lancer le nouvel agent", "Start the new agent"), true}
	}
	return button{"R", i18n.T("Repartir à neuf", "Start afresh"), m.canHandoff(c) && !m.handoffAsked(c)}
}

func (m Model) handoff() (tea.Model, tea.Cmd) {
	c := m.selected()
	if c == nil || !m.canHandoff(c) {
		m.say(i18n.T("Repartir à neuf : il faut un agent Claude sur un chantier en build ou en review.", "Start afresh: needs a Claude agent on a ticket in build or review."), nil)
		return m, nil
	}
	agent, _ := claudeAgent(c)
	key, path := c.Key, c.State.Path
	switch {
	case m.handoffReady(c):
		delete(m.handoffs, key)
		model := ""
		if p := m.projectOf(c); p != nil {
			model = p.cfg.ModelBuild
		}
		m.busy = i18n.T("Nouvel agent…", "New agent…")
		text := fmt.Sprintf(i18n.T("%s : reprise lue.", "%s: handoff read."), key)
		return m, m.freshAgent(agent, model, chantier.ResumePrompt(key, path, m.base(c)), key, "nouvel agent (reprise)", text)
	case m.handoffAsked(c):
		m.say(i18n.T("L'agent écrit sa reprise : le bouton s'allume quand elle est dans le plan et qu'il s'est arrêté.",
			"The agent is writing its handoff: the button lights up once it is in the plan and the agent stopped."), nil)
		return m, nil
	}
	if m.handoffs == nil {
		m.handoffs = map[string]time.Time{}
	}
	m.handoffs[key] = time.Now()
	m.say(fmt.Sprintf(i18n.T("%s : l'agent écrit sa reprise dans le plan. R à nouveau pour lancer le nouvel agent.",
		"%s: the agent writes its handoff in the plan. R again to start the new agent."), key), nil)
	return m, func() tea.Msg {
		_ = journal.Append(config.ChantiersDir(), key, "reprise demandée")
		if err := herdr.Prompt(agent.PaneID, chantier.HandoffPrompt(path)); err != nil {
			return doneMsg{err: err}
		}
		return nil
	}
}

// contextLine says how heavy the main agent's context is, and what to do
// about it once it is too heavy.
func (m Model) contextLine(c *chantier.Chantier) string {
	rep, ok := m.usage[c.Key]
	if !ok || rep.Context == 0 || time.Since(rep.ContextAt) > 24*time.Hour {
		return ""
	}
	size := fmt.Sprintf(i18n.T("contexte %s", "context %s"), count(rep.Context))
	if rep.Context < contextHeavy {
		return "    " + sDim.Render("↳ ") + sMuted.Render(size+i18n.T(" relu à chaque tour", " re-read at each turn"))
	}
	hint := ""
	if m.canHandoff(c) && !m.handoffAsked(c) {
		hint = sMuted.Render(" · ") + sKey.Render("R") + sMuted.Render(i18n.T(" repartir à neuf", " start afresh"))
	}
	return "    " + sDim.Render("↳ ") + sWarn.Render(size+i18n.T(", relu à chaque tour", ", re-read at each turn")) + hint
}

// heavy: the chantier's agent carries a context worth a new agent.
func (m Model) heavy(c *chantier.Chantier) bool {
	rep, ok := m.usage[c.Key]
	return ok && rep.Context >= contextHeavy && time.Since(rep.ContextAt) < 24*time.Hour && m.canHandoff(c)
}
