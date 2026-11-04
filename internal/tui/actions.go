package tui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"cairn/internal/chantier"
	"cairn/internal/ci"
	"cairn/internal/config"
	"cairn/internal/herdr"
	"cairn/internal/journal"
	"cairn/internal/state"
)

// agentPane is the pane of the agent to talk to for a chantier, "" if none.
func agentPane(c *chantier.Chantier) string {
	for _, want := range []string{"idle", "done", "blocked", ""} {
		for _, a := range c.Agents {
			if want == "" || a.Status == want {
				return a.PaneID
			}
		}
	}
	return ""
}

func planPath(c *chantier.Chantier) string {
	if c.State != nil {
		return c.State.Path
	}
	return state.PathFor(config.ChantiersDir(), c.Key)
}

// fixCI hands the failing checks' logs to the chantier's agent.
func (m Model) fixCI() (tea.Model, tea.Cmd) {
	c := m.selected()
	url, num := m.prURLOf()
	if c == nil || url == "" {
		return m, nil
	}
	pane := agentPane(c)
	key, plan := c.Key, planPath(c)
	m.busy = "Récupération des logs de CI…"
	return m, func() tea.Msg {
		path, names, err := ci.FailedLogs(url, filepath.Join(config.Dir(), "cache", "ci"), key)
		if err != nil {
			return doneMsg{err: err}
		}
		if pane == "" {
			return doneMsg{text: "Logs de CI dans " + tilde(path) + " (aucun agent sur ce chantier pour les traiter)."}
		}
		prompt := fmt.Sprintf("La CI de la PR #%d échoue (%s). Les logs des étapes en échec sont dans %s. "+
			"Lis-les, trouve la cause, corrige en local, ajoute la tâche et la preuve dans %s, "+
			"puis demande le push avec `attente: Prêt à pousser`.", num, strings.Join(names, ", "), path, plan)
		_ = journal.Append(config.ChantiersDir(), key, "CI envoyée à l'agent")
		return doneMsg{text: "Logs de CI envoyés à l'agent de " + key + ".", err: herdr.Prompt(pane, prompt)}
	}
}

// sendComment forwards one entry of the PR conversation to the agent.
func (m Model) sendComment(i int) (tea.Model, tea.Cmd) {
	c := m.selected()
	if c == nil || m.thread == nil || i < 0 || i >= len(m.thread.Entries) {
		return m, nil
	}
	pane := agentPane(c)
	if pane == "" {
		m.say("Aucun agent sur "+c.Key+" : lance-en un (Planifier ou enter) avant de lui envoyer un retour.", nil)
		return m, nil
	}
	e := m.thread.Entries[i]
	where := e.Kind
	if e.Kind == "inline" {
		where = fmt.Sprintf("commentaire inline sur %s:%d", e.Path, e.Line)
	}
	body := e.Body
	if len(body) > 6000 {
		body = body[:6000] + "\n… (tronqué, voir la PR)"
	}
	prompt := fmt.Sprintf("Retour sur la PR #%d, de %s (%s) :\n\n%s\n\n"+
		"Traite ce retour en local : ajoute-le comme tâche dans %s, corrige, ajoute la preuve, "+
		"puis demande le push avec `attente: Prêt à pousser`. Si tu n'es pas d'accord avec le retour, "+
		"explique pourquoi et mets la question sous « ## Questions » au lieu de corriger.",
		m.thread.Number, e.Author, where, body, planPath(c))
	key := c.Key
	m.say("Envoi à l'agent…", nil)
	return m, func() tea.Msg {
		_ = journal.Append(config.ChantiersDir(), key, "retour de "+e.Author+" envoyé à l'agent")
		return doneMsg{text: "Retour de " + e.Author + " envoyé à l'agent de " + key + ".", err: herdr.Prompt(pane, prompt)}
	}
}

// attach ties an agent of the shared checkout to a chantier by naming it after
// the ticket: cairn (and herdr's sidebar) then show it under that chantier.
func (m Model) attach(key string) (tea.Model, tea.Cmd) {
	a := m.selectedOther()
	if a == nil {
		return m, nil
	}
	name := herdr.FreeName(strings.ToLower(key), m.snap.Agents)
	pane := a.PaneID
	return m, func() tea.Msg {
		err := herdr.RenameAgent(pane, name)
		return doneMsg{text: "Agent rattaché à " + key + " (renommé " + name + ").", err: err, refresh: true}
	}
}

// tidy removes a shipped chantier's worktree and herdr workspace and archives
// its plan; it asks for a second click first, and never forces a dirty tree.
func (m Model) tidy() (tea.Model, tea.Cmd) {
	c := m.selected()
	if c == nil || c.Worktree == "" || c.Main {
		return m, nil
	}
	if m.confirm != "tidy:"+c.Key {
		m.confirm = "tidy:" + c.Key
		m.say("Clique encore sur Ranger pour confirmer : supprime "+tilde(c.Worktree)+", ferme son espace herdr et archive le plan.", nil)
		return m, nil
	}
	m.confirm = ""
	key, wt := c.Key, c.Worktree
	var ws string
	for _, w := range m.worktrees {
		if filepath.Clean(w.Path) == filepath.Clean(wt) {
			ws = w.OpenWorkspaceID
		}
	}
	m.busy = "Rangement de " + key + "…"
	return m, func() tea.Msg {
		if out, _ := exec.Command("git", "-C", wt, "status", "--porcelain").Output(); len(strings.TrimSpace(string(out))) > 0 {
			return doneMsg{err: fmt.Errorf("%s a des changements non commités : rien n'a été supprimé", tilde(wt))}
		}
		var err error
		if ws != "" {
			err = herdr.RemoveWorktree(ws)
		} else {
			err = exec.Command("git", "-C", wt, "worktree", "remove", wt).Run()
		}
		if err != nil {
			return doneMsg{err: err}
		}
		_ = journal.Append(config.ChantiersDir(), key, "rangé")
		if err := journal.Archive(config.ChantiersDir(), key); err != nil {
			return doneMsg{err: err, refresh: true}
		}
		return doneMsg{text: key + " rangé : worktree supprimé, plan archivé dans ~/.cairn/archive.", refresh: true}
	}
}
