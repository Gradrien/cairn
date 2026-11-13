package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cairn/internal/config"
	"cairn/internal/i18n"
	"cairn/internal/journal"
	"cairn/internal/mux"
	"cairn/internal/state"
)

// The manual test, in the detail: a click on a step to do opens two
// actions under it, « ça marche » (ticks it in the plan) and « ça ne marche
// pas » (a box to say what you see instead, sent to the agent). A click on a
// ticked step unticks it.

// manualKey identifies a step across reloads of the plan; reports sent live
// in m.answered under it.
func manualKey(key, step string) string { return answerKey(key, "manual\x00"+step) }

func (m Model) manualClick(kind string, i int) (tea.Model, tea.Cmd) {
	c := m.selected()
	if c == nil || c.State == nil || i < 0 || i >= len(c.State.ManualSteps) {
		return m, nil
	}
	step := c.State.ManualSteps[i]
	id := manualKey(c.Key, step.Text)
	switch {
	case kind == "manual-ok" || (kind == "manual" && step.Done):
		m.manualSel, m.manualAsk, m.manualText = "", false, ""
		return m.tickManual(i, !step.Done)
	case kind == "manual-ko":
		m.manualSel, m.manualAsk, m.manualText, m.detailFocus = id, true, "", true
	case m.manualSel == id:
		m.manualSel, m.manualAsk, m.manualText = "", false, ""
	default:
		m.manualSel, m.manualAsk, m.manualText = id, false, ""
	}
	return m, nil
}

// tickManual checks or unchecks a step in the plan file itself: the agent
// sees what the human already tried. A ticked step drops its report.
func (m Model) tickManual(i int, done bool) (tea.Model, tea.Cmd) {
	c := m.selected()
	s, err := state.Load(c.State.Path)
	if err != nil {
		m.say("", err)
		return m, nil
	}
	if s == nil || i >= len(s.ManualSteps) || !s.TickManual(i, done) {
		return m, nil
	}
	if err := s.Save(); err != nil {
		m.say("", err)
		return m, nil
	}
	if done {
		delete(m.answered, manualKey(c.Key, s.ManualSteps[i].Text))
	}
	c.State = s // the watcher reloads it too
	return m, nil
}

func (m Model) manualKeyPress(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.manualAsk, m.manualText = false, ""
	case tea.KeyEnter:
		return m.sendManualReport()
	case tea.KeyBackspace:
		if r := []rune(m.manualText); len(r) > 0 {
			m.manualText = string(r[:len(r)-1])
		}
	case tea.KeySpace:
		m.manualText += " "
	case tea.KeyRunes:
		m.manualText += string(k.Runes)
	}
	return m, nil
}

// manualBox is the input drawn under a step that does not work.
func (m Model) manualBox(w int) []string {
	inner := max(10, w-10)
	text := m.manualText + sKey.Render("▏")
	if m.manualText == "" {
		text = sKey.Render("▏") + sDim.Render(i18n.T("ex. : rien ne se télécharge, la colonne Modèle est vide", "e.g. nothing downloads, the Model column is empty"))
	}
	content := []string{sMuted.Render(i18n.T("↳ ce que tu vois à la place, envoyé à l'agent", "↳ what you see instead, sent to the agent"))}
	content = append(content, wrap(text, inner, "")...)
	content = append(content, sKey.Render("↵")+sMuted.Render(i18n.T(" envoyer à l'agent   ", " send to the agent   "))+sKey.Render("esc")+sMuted.Render(i18n.T(" annuler", " cancel")))
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(danger).Padding(0, 1).Width(inner + 2).
		Render(strings.Join(content, "\n"))
	out := strings.Split(box, "\n")
	for i := range out {
		out[i] = "    " + out[i]
	}
	return out
}

func manualPrompt(key, path string, n int, step, setup, seen string) string {
	if setup != "" {
		setup = fmt.Sprintf(i18n.T("Préparation suivie : %s\n", "Setup followed: %s\n"), setup)
	}
	return fmt.Sprintf(i18n.T("Test à la main de %s : l'étape %d ne marche pas.\n« %s »\n%sCe que l'humain voit à la place : « %s »\n\n"+
		"Retire d'abord l'attente du plan si elle dit que c'est prêt. Trouve la cause et corrige-la, avec un test qui la couvre si c'est possible. Si c'est l'étape qui est fausse "+
		"(mauvais bouton, mauvaise attente), corrige l'étape dans « ## Tester à la main » de %s et dis-le. Ne coche pas l'étape : "+
		"l'humain la refera. Garde le plan vrai, lance `cairn sync`, puis remets l'attente qui convient "+
		"(`attente: Prêt à pousser — …` si c'est prêt) et arrête-toi.",
		"Manual test of %s: step %d does not work.\n\"%s\"\n%sWhat the human sees instead: \"%s\"\n\n"+
			"First remove the plan's waiting line if it says it is ready. Find the cause and fix it, with a test covering it if you can. If the step itself is wrong "+
			"(wrong button, wrong expectation), fix the step in \"## Tester à la main\" of %s and say so. Do not tick the step: "+
			"the human will try it again. Keep the plan true, run `cairn sync`, then set the right waiting line "+
			"(`attente: Prêt à pousser — …` if it is ready) and stop."), key, n, step, setup, seen, path)
}

func (m Model) sendManualReport() (tea.Model, tea.Cmd) {
	c := m.selected()
	seen := strings.TrimSpace(m.manualText)
	if c == nil || c.State == nil {
		return m, nil
	}
	if seen == "" {
		m.say(i18n.T("Dis ce que tu vois à la place, ou esc pour annuler.", "Say what you see instead, or esc to cancel."), nil)
		return m, nil
	}
	n := -1
	for i, t := range c.State.ManualSteps {
		if manualKey(c.Key, t.Text) == m.manualSel {
			n = i
		}
	}
	if n < 0 {
		m.manualSel, m.manualAsk, m.manualText = "", false, ""
		return m, nil
	}
	pane := agentPane(c)
	if pane == "" {
		m.say(fmt.Sprintf(i18n.T("Aucun agent sur %s pour recevoir le retour.", "No agent on %s to receive the report."), c.Key), nil)
		return m, nil
	}
	step := plainMD(c.State.ManualSteps[n].Text)
	var setup []string
	for _, l := range c.State.ManualSetup {
		setup = append(setup, plainMD(l))
	}
	prompt := manualPrompt(c.Key, planPath(c), n+1, step, strings.Join(setup, " "), seen)
	m.answered[m.manualSel] = seen
	m.manualSel, m.manualAsk, m.manualText = "", false, ""
	key := c.Key
	return m, func() tea.Msg {
		_ = journal.Append(config.ChantiersDir(), key, fmt.Sprintf("test à la main : étape %d ne marche pas", n+1))
		return doneMsg{text: fmt.Sprintf(i18n.T("Étape %d signalée à l'agent de %s.", "Step %d reported to %s's agent."), n+1, key), err: mux.Prompt(pane, prompt)}
	}
}

// tickSide checks or unchecks a side effect in the plan file: the human
// verified the behaviour. The agent sees it there, and the merge checklist
// counts it.
func (m Model) tickSide(i int) (tea.Model, tea.Cmd) {
	c := m.selected()
	if c == nil || c.State == nil {
		return m, nil
	}
	s, err := state.Load(c.State.Path)
	if err != nil {
		m.say("", err)
		return m, nil
	}
	if s == nil || i < 0 || i >= len(s.SideEffects) {
		return m, nil
	}
	done := !s.SideEffects[i].Done
	if !s.TickSide(i, done) {
		return m, nil
	}
	if err := s.Save(); err != nil {
		m.say("", err)
		return m, nil
	}
	c.State = s // the watcher reloads it too
	if done {
		_ = journal.Append(config.ChantiersDir(), c.Key, "effet de bord vérifié")
		m.say(i18n.T("Effet de bord coché dans le plan.", "Side effect ticked in the plan."), nil)
	} else {
		m.say(i18n.T("Effet de bord décoché.", "Side effect unticked."), nil)
	}
	return m, nil
}
