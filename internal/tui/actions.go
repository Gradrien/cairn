package tui

import (
	"fmt"
	"os"
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

type cleanedMsg struct {
	key  string
	keep string // why the chantier was left as is, "" once cleaned
	text string
	err  error
}

// cleanup is what happens to a shipped chantier: its linked worktree and
// herdr workspace (agents included) are removed and its plan archived. On its
// own it only runs when nothing can be lost: no uncommitted change, no commit
// missing from GitHub, no agent at work. force is you closing it by hand
// after seeing what goes. The main checkout is never touched.
func (m Model) cleanup(c *chantier.Chantier, force bool) tea.Cmd {
	key, wt := c.Key, c.Worktree
	linked := wt != "" && !c.Main
	for _, a := range c.Agents {
		if a.Status == "working" && !force {
			return func() tea.Msg {
				return cleanedMsg{key: key, keep: i18n.T("un agent y travaille", "an agent is working")}
			}
		}
	}
	var ws string
	for _, w := range m.allWorktrees() {
		if linked && filepath.Clean(w.Path) == filepath.Clean(wt) {
			ws = w.OpenWorkspaceID
		}
	}
	head := m.prs[key].HeadOid
	return func() tea.Msg {
		if linked {
			if !force && isDirty(wt) {
				return cleanedMsg{key: key, keep: i18n.T("changements non commités", "uncommitted changes")}
			}
			if !force && unpushed(wt, head) {
				return cleanedMsg{key: key, keep: i18n.T("commits absents de GitHub", "commits not on GitHub")}
			}
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
				return cleanedMsg{key: key, keep: i18n.T("nettoyage impossible", "cleanup failed"), err: err}
			}
		}
		dir := config.ChantiersDir()
		if _, err := os.Stat(state.PathFor(dir, key)); err == nil || linked {
			_ = journal.Append(dir, key, "rangé")
			if err := journal.Archive(dir, key); err != nil {
				return cleanedMsg{key: key, err: err}
			}
		}
		text := ""
		if linked {
			text = key + i18n.T(" livré : worktree et espace herdr supprimés, plan archivé.", " shipped: worktree and herdr workspace removed, plan archived.")
		}
		return cleanedMsg{key: key, text: text}
	}
}

// unpushed reports commits of the worktree that GitHub does not have: none
// when HEAD is the merged PR's last commit (its branch may be deleted since),
// else none when every commit is on a remote branch.
func unpushed(dir, prHead string) bool {
	if out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output(); err == nil && prHead != "" && strings.TrimSpace(string(out)) == prHead {
		return false
	}
	out, err := exec.Command("git", "-C", dir, "rev-list", "--count", "HEAD", "--not", "--remotes").Output()
	return err != nil || strings.TrimSpace(string(out)) != "0"
}

// closeRisk is what closing a chantier by hand would throw away.
type closeRisk struct {
	dirty    int // uncommitted files
	unpushed int // commits GitHub does not have
}

type closeRiskMsg struct {
	key  string
	risk closeRisk
}

// openClose asks before closing a shipped chantier: worktree, herdr
// workspace and agents go, the plan is archived.
func (m Model) openClose() (tea.Model, tea.Cmd) {
	c := m.selected()
	if c == nil || c.Phase != chantier.Shipped {
		m.say(i18n.T("Seul un chantier livré se ferme : sa PR doit être mergée.", "Only a shipped ticket closes: its PR must be merged."), nil)
		return m, nil
	}
	m.overlay, m.closeKey, m.closeRisk = "close", c.Key, nil
	key, wt, head := c.Key, c.Worktree, m.prs[c.Key].HeadOid
	if wt == "" || c.Main {
		m.closeRisk = &closeRisk{}
		return m, nil
	}
	return m, func() tea.Msg {
		var r closeRisk
		if out, err := exec.Command("git", "-C", wt, "status", "--porcelain").Output(); err == nil && strings.TrimSpace(string(out)) != "" {
			r.dirty = len(strings.Split(strings.TrimSpace(string(out)), "\n"))
		}
		if unpushed(wt, head) {
			out, _ := exec.Command("git", "-C", wt, "rev-list", "--count", "HEAD", "--not", "--remotes").Output()
			fmt.Sscan(strings.TrimSpace(string(out)), &r.unpushed)
		}
		return closeRiskMsg{key: key, risk: r}
	}
}

// closeChantier closes the chantier the confirmation was about.
func (m Model) closeChantier() (tea.Model, tea.Cmd) {
	for _, c := range m.all {
		if c.Key == m.closeKey {
			m.busy = fmt.Sprintf(i18n.T("Fermeture de %s…", "Closing %s…"), c.Key)
			return m, m.cleanup(c, true)
		}
	}
	return m, nil
}

func isDirty(dir string) bool {
	out, _ := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	return len(strings.TrimSpace(string(out))) > 0
}
