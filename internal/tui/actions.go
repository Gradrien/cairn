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
	"cairn/internal/i18n"
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
	m.busy = i18n.T("Récupération des logs de CI…", "Fetching CI logs…")
	return m, func() tea.Msg {
		path, names, err := ci.FailedLogs(url, filepath.Join(config.Dir(), "cache", "ci"), key)
		if err != nil {
			return doneMsg{err: err}
		}
		if pane == "" {
			return doneMsg{text: fmt.Sprintf(i18n.T("Logs de CI dans %s (aucun agent sur ce chantier pour les traiter).", "CI logs in %s (no agent on this ticket to handle them)."), tilde(path))}
		}
		prompt := fmt.Sprintf(i18n.T("La CI de la PR #%d échoue (%s). Les logs des étapes en échec sont dans %s. "+
			"Lis-les, trouve la cause, corrige en local, ajoute la tâche et la preuve dans %s, "+
			"puis demande le push avec `attente: Prêt à pousser`.",
			"CI of PR #%d is failing (%s). The logs of the failing steps are in %s. "+
				"Read them, find the cause, fix it locally, add the task and the proof in %s, "+
				"then ask for the push with `attente: Prêt à pousser`."), num, strings.Join(names, ", "), path, plan)
		_ = journal.Append(config.ChantiersDir(), key, "CI envoyée à l'agent")
		return doneMsg{text: fmt.Sprintf(i18n.T("Logs de CI envoyés à l'agent de %s.", "CI logs sent to %s's agent."), key), err: herdr.Prompt(pane, prompt)}
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
		m.say(fmt.Sprintf(i18n.T("Aucun agent sur %s : lance-en un (Planifier ou enter) avant de lui envoyer un retour.", "No agent on %s: start one (Plan or enter) before sending it feedback."), c.Key), nil)
		return m, nil
	}
	e := m.thread.Entries[i]
	where := e.Kind
	if e.Kind == "inline" {
		where = fmt.Sprintf(i18n.T("commentaire inline sur %s:%d", "inline comment on %s:%d"), e.Path, e.Line)
	}
	body := e.Body
	if len(body) > 6000 {
		body = body[:6000] + i18n.T("\n… (tronqué, voir la PR)", "\n… (truncated, see the PR)")
	}
	prompt := fmt.Sprintf(i18n.T("Retour sur la PR #%d, de %s (%s) :\n\n%s\n\n"+
		"Traite ce retour en local : ajoute-le comme tâche dans %s, corrige, ajoute la preuve, "+
		"puis demande le push avec `attente: Prêt à pousser`. Si tu n'es pas d'accord avec le retour, "+
		"explique pourquoi et mets la question sous « ## Questions » au lieu de corriger.",
		"Feedback on PR #%d, from %s (%s):\n\n%s\n\n"+
			"Handle this feedback locally: add it as a task in %s, fix it, add the proof, "+
			"then ask for the push with `attente: Prêt à pousser`. If you disagree with the feedback, "+
			"explain why and put the question under \"## Questions\" instead of fixing it."),
		m.thread.Number, e.Author, where, body, planPath(c))
	key := c.Key
	m.say(i18n.T("Envoi à l'agent…", "Sending to the agent…"), nil)
	return m, func() tea.Msg {
		_ = journal.Append(config.ChantiersDir(), key, "retour de "+e.Author+" envoyé à l'agent")
		return doneMsg{text: fmt.Sprintf(i18n.T("Retour de %s envoyé à l'agent de %s.", "Feedback from %s sent to %s's agent."), e.Author, key), err: herdr.Prompt(pane, prompt)}
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
		return doneMsg{text: fmt.Sprintf(i18n.T("Agent rattaché à %s (renommé %s).", "Agent attached to %s (renamed %s)."), key, name), err: err, refresh: true}
	}
}

// tidy puts a chantier away, whatever its phase: its linked worktree and
// herdr workspace are removed, its plan and journal archived, and cairn stops
// listing it. The main checkout is never touched. A first click explains, a
// second confirms; uncommitted changes need a third click to be thrown away.
func (m Model) tidy() (tea.Model, tea.Cmd) {
	c := m.selected()
	if c == nil {
		return m, nil
	}
	key, wt := c.Key, c.Worktree
	if m.closed[key] && c.State == nil {
		_ = journal.SetClosed(config.ChantiersDir(), key, false)
		m.say(key+i18n.T(" est de retour dans la liste.", " is back in the list."), nil)
		cmd := m.rebuild()
		return m, cmd
	}
	linked := wt != "" && !c.Main
	dirty := linked && isDirty(wt)
	switch m.confirm {
	case "tidy:" + key:
		if dirty {
			m.confirm = "force:" + key
			m.say(tilde(wt)+i18n.T(" a des changements non commités. Clique encore pour supprimer quand même : ils seront perdus.", " has uncommitted changes. Click again to delete anyway: they will be lost."), nil)
			return m, nil
		}
	case "force:" + key:
	default:
		m.confirm = "tidy:" + key
		m.say(i18n.T("Clique encore pour confirmer : ", "Click again to confirm: ")+tidyPlan(c, linked, m.prs[key].ReviewDecision), nil)
		return m, nil
	}
	force := m.confirm == "force:"+key
	m.confirm = ""
	var ws string
	for _, w := range m.worktrees {
		if linked && filepath.Clean(w.Path) == filepath.Clean(wt) {
			ws = w.OpenWorkspaceID
		}
	}
	m.busy = fmt.Sprintf(i18n.T("Rangement de %s…", "Putting %s away…"), key)
	return m, func() tea.Msg {
		if linked {
			var err error
			if ws != "" {
				err = herdr.RemoveWorktree(ws, force)
			} else {
				args := []string{"-C", wt, "worktree", "remove", wt}
				if force {
					args = append(args, "--force")
				}
				err = exec.Command("git", args...).Run()
			}
			if err != nil {
				return doneMsg{err: err, refresh: true}
			}
		}
		dir := config.ChantiersDir()
		_ = journal.Append(dir, key, "rangé")
		if err := journal.Archive(dir, key); err != nil {
			return doneMsg{err: err, refresh: true}
		}
		if err := journal.SetClosed(dir, key, true); err != nil {
			return doneMsg{err: err, refresh: true}
		}
		text := key + i18n.T(" rangé : plan archivé dans ~/.cairn/archive", " put away: plan archived in ~/.cairn/archive")
		if linked {
			text += i18n.T(", worktree supprimé", ", worktree deleted")
		}
		return doneMsg{text: text + i18n.T(". Planifier le ticket le fait revenir.", ". Planning the ticket brings it back."), refresh: true}
	}
}

func isDirty(dir string) bool {
	out, _ := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	return len(strings.TrimSpace(string(out))) > 0
}

// tidyPlan says what putting a chantier away will do, and what is unusual.
func tidyPlan(c *chantier.Chantier, linked bool, review string) string {
	var what []string
	if linked {
		what = append(what, fmt.Sprintf(i18n.T("supprime %s et son espace herdr", "deletes %s and its herdr workspace"), tilde(c.Worktree)))
	} else if c.Main {
		what = append(what, i18n.T("laisse le dépôt principal tel quel (la branche reste checkoutée)", "leaves the main checkout as is (the branch stays checked out)"))
	}
	what = append(what, i18n.T("archive le plan", "archives the plan"), i18n.T("le retire de la liste", "removes it from the list"))
	msg := strings.Join(what, ", ") + "."
	if c.Phase != chantier.Shipped {
		msg += i18n.T(" Il n'est pas livré", " It is not shipped")
		if c.OpenPR() != nil {
			msg += i18n.T(" : sa PR reste ouverte sur GitHub", ": its PR stays open on GitHub")
			if review == "CHANGES_REQUESTED" {
				msg += i18n.T(" avec des changements demandés", " with changes requested")
			}
		}
		msg += "."
	}
	return msg
}
