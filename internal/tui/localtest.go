package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"cairn/internal/chantier"
	"cairn/internal/config"
	"cairn/internal/i18n"
	"cairn/internal/localtest"
)

// "Tester ici": a chantier's worktree lacks what the main checkout has (env,
// secrets, packages, the running stack). h puts the chantier's branch in the
// main checkout to try it there, after a confirmation that says what moves;
// h again catches up with new commits, then puts the checkout back.

func ltStore() localtest.Store {
	return localtest.Store{Path: filepath.Join(config.Dir(), "cache", "localtest.json")}
}

type ltPlanMsg struct {
	key  string
	plan localtest.Plan
	err  error
}

// testOf is the test under way in c's project, if any.
func (m Model) testOf(c *chantier.Chantier) (localtest.Status, bool) {
	p := m.projectOf(c)
	if p == nil {
		return localtest.Status{}, false
	}
	st, ok := m.tests[p.cfg.Name]
	return st, ok
}

// testButton is h for c: try it, catch up, or end the test.
func (m Model) testButton(c *chantier.Chantier) button {
	b := button{"h", i18n.T("Tester ici", "Test here"), c.Worktree != "" && !c.Main && c.Branch != "" && m.projectOf(c) != nil}
	if st, ok := m.testOf(c); ok && st.Key == c.Key {
		b.enabled = true
		b.label = i18n.T("Fin du test", "End the test")
		if st.Behind() {
			b.label = i18n.T("Actualiser le test", "Update the test")
		}
	}
	return b
}

func (m Model) testKey() (tea.Model, tea.Cmd) {
	c := m.selected()
	if c == nil || !m.testButton(c).enabled {
		return m, nil
	}
	repo, name := m.projectOf(c).cfg.Repo, m.projectOf(c).cfg.Name
	st, testing := m.testOf(c)
	switch {
	case testing && st.Key == c.Key && st.Behind():
		head, tip, key := st.Head, st.Tip, c.Key
		m.busy = i18n.T("Actualisation du test…", "Updating the test…")
		return m, func() tea.Msg {
			if err := ltStore().Refresh(repo); err != nil {
				return doneMsg{err: err, refresh: true}
			}
			text := fmt.Sprintf(i18n.T("%s : test de %s actualisé au dernier commit.", "%s: test of %s moved to the last commit."), name, key)
			if localtest.DepsChanged(repo, head, tip) {
				text += i18n.T(" Les dépendances ont changé : relance l'install.", " Dependencies changed: run the install again.")
			}
			return doneMsg{text: text, refresh: true}
		}
	case testing && st.Key == c.Key:
		m.busy = i18n.T("Fin du test…", "Ending the test…")
		back, stash := st.Back, st.Stash != ""
		return m, func() tea.Msg {
			if err := ltStore().Stop(repo); err != nil {
				return doneMsg{err: err, refresh: true}
			}
			text := fmt.Sprintf(i18n.T("%s est revenu sur %s.", "%s is back on %s."), name, back)
			if stash {
				text += i18n.T(" Tes changements sont restaurés.", " Your changes are restored.")
			}
			return doneMsg{text: text, refresh: true}
		}
	}
	m.overlay, m.ltKey, m.ltPlan, m.ltErr = "localtest", c.Key, nil, nil
	branch, key := c.Branch, c.Key
	return m, func() tea.Msg {
		p, err := localtest.Prepare(repo, branch)
		return ltPlanMsg{key: key, plan: p, err: err}
	}
}

// workingIn is an agent at work in repo itself (not in a worktree beside it).
func (m Model) workingIn(repo string) string {
	repo = filepath.Clean(repo)
	for _, a := range m.snap.Agents {
		d := filepath.Clean(a.Dir())
		if a.Status == "working" && (d == repo || strings.HasPrefix(d, repo+string(filepath.Separator))) {
			return a.Label()
		}
	}
	return ""
}

func (m Model) startTest() (tea.Model, tea.Cmd) {
	m.overlay = ""
	c := m.selected()
	if c == nil || c.Key != m.ltKey || m.ltPlan == nil || m.ltErr != nil || m.projectOf(c) == nil {
		return m, nil
	}
	repo, name := m.projectOf(c).cfg.Repo, m.projectOf(c).cfg.Name
	if who := m.workingIn(repo); who != "" {
		m.say("", fmt.Errorf(i18n.T("%s travaille dans %s : attends qu'il ait fini", "%s is working in %s: wait until it is done"), who, name))
		return m, nil
	}
	key, branch, deps := c.Key, c.Branch, m.ltPlan.DepsDiff
	m.busy = fmt.Sprintf(i18n.T("%s passe sur %s…", "%s switches to %s…"), name, key)
	return m, func() tea.Msg {
		if err := ltStore().Start(repo, key, branch); err != nil {
			return doneMsg{err: err, refresh: true}
		}
		text := fmt.Sprintf(i18n.T("%s teste %s : lance tes commandes là-bas, h pour revenir.", "%s tests %s: run your commands there, h to go back."), name, key)
		if deps {
			text += i18n.T(" Les dépendances changent : lance l'install.", " Dependencies change: run the install.")
		}
		return doneMsg{text: text, refresh: true}
	}
}

// testBadges say, in the header, which main checkout is under test.
func (m Model) testBadges() string {
	var out []string
	for _, p := range m.projects {
		st, ok := m.tests[p.cfg.Name]
		if !ok {
			continue
		}
		b := sWarn.Render("⚑ "+p.cfg.Name+" : test "+st.Key) + sDim.Render(" h")
		if st.Behind() {
			b += sMuted.Render(i18n.T(" · nouveaux commits", " · new commits"))
		}
		out = append(out, b)
	}
	return strings.Join(out, "   ")
}

// testOverlay is the confirmation before trying a chantier in the main checkout.
func (m Model) testOverlay(inner int) (content []string, buttons []button) {
	var c *chantier.Chantier
	for _, x := range m.all {
		if x.Key == m.ltKey {
			c = x
		}
	}
	if c == nil || m.projectOf(c) == nil {
		return nil, nil
	}
	p := m.projectOf(c).cfg
	row := func(k, v string) { content = append(content, hang(lbl(k), v, inner)...) }
	content = append(content, sWarn.Render(fmt.Sprintf(i18n.T("Tester %s dans %s ?", "Test %s in %s?"), c.Key, p.Name)))
	content = append(content, wrap(sMuted.Render(i18n.T("Ton dépôt principal passe sur le dernier commit de la branche, avec son env, ses secrets et sa stack déjà là. h le remet comme avant.",
		"Your main checkout moves to the branch's last commit, with its env, secrets and stack already there. h puts it back.")), inner, "")...)
	content = append(content, "")
	ok := m.ltErr == nil && m.ltPlan != nil
	switch {
	case m.ltErr != nil:
		content = append(content, wrap(sErr.Render("✗ "+m.ltErr.Error()), inner, "")...)
	case m.ltPlan == nil:
		content = append(content, sMuted.Render(i18n.T("Lecture du dépôt…", "Reading the checkout…")))
	default:
		lp := m.ltPlan
		row(i18n.T("Dépôt", "Checkout"), tilde(p.Repo))
		from := lp.From
		if st, testing := m.tests[p.Name]; testing {
			from += sMuted.Render(fmt.Sprintf(i18n.T(" (test de %s, remplacé)", " (test of %s, replaced)"), st.Key))
		}
		row(i18n.T("Quitte", "Leaves"), from)
		row(i18n.T("Arrive", "Goes to"), c.Branch+sMuted.Render(i18n.T(" · détaché, dernier commit", " · detached, last commit")))
		if lp.Dirty > 0 {
			row(i18n.T("De côté", "Set aside"), sWarn.Render(plural(lp.Dirty, i18n.T("changement non commité", "uncommitted change"), i18n.T("changements non commités", "uncommitted changes")))+
				sMuted.Render(i18n.T(" : git stash, restaurés à la fin du test", ": git stash, restored when the test ends")))
		}
		if g, ok := m.gits[c.Key]; ok && g.Uncommitted > 0 {
			content = append(content, hang(strings.Repeat(" ", label)+sWarn.Render("⚠ "), plural(g.Uncommitted,
				i18n.T("changement non commité du worktree n'est pas inclus", "uncommitted change of the worktree is not included"),
				i18n.T("changements non commités du worktree ne sont pas inclus", "uncommitted changes of the worktree are not included")), inner)...)
		}
		if lp.DepsDiff {
			content = append(content, hang(strings.Repeat(" ", label)+sWarn.Render("⚠ "), i18n.T("les dépendances changent : lance l'install après", "dependencies change: run the install after"), inner)...)
		}
		if who := m.workingIn(p.Repo); who != "" {
			ok = false
			content = append(content, hang(strings.Repeat(" ", label)+sErr.Render("✗ "), fmt.Sprintf(i18n.T("%s travaille dans ce dépôt : attends qu'il ait fini", "%s is working in this checkout: wait until it is done"), who), inner)...)
		}
	}
	content = append(content, "")
	return content, []button{
		{"enter", i18n.T("Tester ici", "Test here"), ok},
		{"esc", i18n.T("Annuler", "Cancel"), true},
	}
}
