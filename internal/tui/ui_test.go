package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"cairn/internal/chantier"
	"cairn/internal/config"
	"cairn/internal/herdr"
	"cairn/internal/journal"
	"cairn/internal/progress"
	"cairn/internal/state"
	"cairn/internal/tracker"
)

// fixture is a screen with one chantier in each situation the UI tells apart.
func fixture(t *testing.T, w, h int) Model {
	t.Helper()
	dir := t.TempDir()
	plan := func(key, body string) *state.State {
		s := state.Parse(filepath.Join(dir, key+".md"), body)
		s.UpdatedAt = time.Now().Add(-32 * time.Minute)
		return s
	}
	issue := func(key, title string) *tracker.Issue {
		return &tracker.Issue{ID: key, Title: title, Status: "In Progress", Priority: "High", Estimate: "S", Started: true, URL: "https://t/" + key}
	}
	agent := func(name, status string) herdr.Agent {
		return herdr.Agent{Name: name, Status: status, PaneID: "p-" + name, Title: "travaille"}
	}
	m := Model{
		width: w, height: h, hoverItem: -1, hoverBtn: -1, watch: newWatch(),
		gits: map[string]progress.Git{}, prs: map[string]progress.PR{}, readAt: map[string]time.Time{}, reading: map[string]bool{},
		tokens: map[string]string{}, unfold: map[string]bool{}, answered: map[string]string{},
	}
	m.projects = []*project{{cfg: config.Project{Name: "yak", Key: "YAK", Tracker: "sprintosaur", Assignee: "adrien", Base: "origin/main"}}}
	m.chantiers = []*chantier.Chantier{
		{Key: "YAK-861", Phase: chantier.Plan, Worktree: "/w/861", Branch: "adrien/yak-861",
			Issue:  issue("YAK-861", "Diffusion d'une opportunité interrompue (504) au-delà de 2000 freelances"),
			Agents: []herdr.Agent{agent("yak-861", "idle")},
			State: plan("YAK-861", "---\nphase: plan\nattente: Valider le plan\n---\nLe correctif passe à triggerBulk.\n\n"+
				"- [ ] Remplacer forChunks par triggerBulk\n- [ ] Tests unitaires\n\n## Questions\n- Reprendre la PR #3321 ?\n- Garder notification_count ?\n\n"+
				"## Effets de bord\n- [ ] Autres appelants\n- [ ] Quota Novu\n")},
		{Key: "YAK-800", Phase: chantier.Build, Worktree: "/w/800", Branch: "adrien/yak-800",
			Issue:  issue("YAK-800", "Fiche freelance : rendre le Résumé lisible"),
			Agents: []herdr.Agent{agent("yak-800", "idle")},
			State: plan("YAK-800", "---\nphase: build\nattente: Prêt à pousser\n---\nRésumé.\n\n- [x] Lire la synthèse\n- [x] Rendu markdown\n\n"+
				"## Effets de bord\n- [x] Recherche sémantique\n- [ ] Fiche mobile\n")},
		{Key: "YAK-712", Phase: chantier.Build, Worktree: "/w/712", Branch: "adrien/yak-712",
			Issue:  issue("YAK-712", "Sparrow : permissions des fiches"),
			Agents: []herdr.Agent{agent("yak-712", "working")},
			State:  plan("YAK-712", "---\nphase: build\n---\n- [x] Une\n- [~] Deux\n- [ ] Trois\n")},
		{Key: "YAK-788", Phase: chantier.Review, Worktree: "/w/788", Branch: "adrien/yak-788",
			Issue: issue("YAK-788", "Supprimer « Définir comme modèle »"), PRs: []tracker.PullRequest{{Number: 3323, State: "open", URL: "u788"}}},
		{Key: "YAK-874", Phase: chantier.ToPlan, Issue: issue("YAK-874", "Export CSV des CRA mensuels")},
	}
	m.prs["YAK-788"] = progress.PR{URL: "u788", State: "OPEN", Mergeable: "MERGEABLE", Checks: progress.Checks{Passed: 3, Failed: 2, FailedNames: []string{"e2e", "typecheck"}}}
	m.gits["YAK-800"] = progress.Git{Ahead: 3, Files: 14, Added: 408, Removed: 145, LastSubject: "feat: résumé"}
	return m
}

func screen(m Model) []string { return strings.Split(ansi.Strip(m.View()), "\n") }

func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case "space":
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
		case "backspace":
			msg = tea.KeyMsg{Type: tea.KeyBackspace}
		default:
			msg = keyFor(k)
		}
		next, _ := m.key(msg)
		m = next.(Model)
	}
	return m
}

func TestScreenFitsTerminal(t *testing.T) {
	for _, size := range [][2]int{{160, 45}, {100, 30}, {80, 30}} {
		m := fixture(t, size[0], size[1])
		for _, view := range []Model{m, press(t, m, "tab"), press(t, m, "t"), press(t, m, "?"), press(t, m, "ctrl+k")} {
			lines := screen(view)
			if len(lines) > size[1] {
				t.Errorf("%dx%d: %d lines", size[0], size[1], len(lines))
			}
			for i, l := range lines {
				if w := ansi.StringWidth(l); w > size[0] {
					t.Errorf("%dx%d line %d is %d wide: %q", size[0], size[1], i, w, l)
				}
			}
		}
	}
}

func TestWaitingFirstWithWhatToDo(t *testing.T) {
	m := fixture(t, 160, 45)
	m.rebuildOrder()
	text := strings.Join(screen(m), "\n")
	wait := strings.Index(text, "Attend ta décision")
	work := strings.Index(text, "En cours")
	if wait < 0 || work < 0 || wait > work {
		t.Fatalf("waiting group must come first:\n%s", text)
	}
	for _, want := range []string{"⚠ Valider le plan · 2 questions", "✗ CI en échec · PR #3323", "À planifier"} {
		if !strings.Contains(text, want) {
			t.Errorf("list misses %q", want)
		}
	}
	// CI broken counts as waiting: YAK-788 sits above YAK-712.
	if strings.Index(text, "YAK-788") > strings.Index(text, "YAK-712") {
		t.Error("a broken CI must sort with what waits for you")
	}
}

func TestCardAndPrimaryButton(t *testing.T) {
	cases := map[string]struct{ title, first string }{
		"YAK-861": {"À toi : valider le plan", "v"},
		"YAK-800": {"À toi : autoriser le push", "a"},
		"YAK-788": {"La CI casse : 2 checks en échec", "f"},
		"YAK-712": {"L'agent travaille", "enter"},
		"YAK-874": {"Pas encore commencé", "p"},
	}
	m := fixture(t, 160, 45)
	for i, c := range m.chantiers {
		want := cases[c.Key]
		m.cursor = i
		cd := m.cardFor(c)
		if cd == nil || !strings.Contains(cd.title, want.title) {
			t.Errorf("%s: card %+v, want %q", c.Key, cd, want.title)
		}
		if bs := m.mainButtons(); len(bs) == 0 || bs[0].key != want.first {
			t.Errorf("%s: first button %v, want %q", c.Key, bs, want.first)
		}
		for _, b := range m.mainButtons() {
			if !b.enabled {
				t.Errorf("%s: disabled button %q shown", c.Key, b.label)
			}
		}
	}
}

func TestDetailOrderFollowsPhase(t *testing.T) {
	m := fixture(t, 160, 80)
	pos := func(text, s string) int {
		i := strings.Index(text, s)
		if i < 0 {
			t.Fatalf("missing %q in\n%s", s, text)
		}
		return i
	}
	m.cursor = 0 // plan waiting: questions before tasks before side effects
	text := strings.Join(screen(m), "\n")
	if !(pos(text, "Questions ouvertes") < pos(text, "── Tâches") && pos(text, "── Tâches") < pos(text, "── Effets de bord")) {
		t.Error("plan: questions, tasks, side effects")
	}
	// A plan to approve keeps its questions first even with a PR open.
	m.chantiers[0].PRs = []tracker.PullRequest{{Number: 3321, State: "open", URL: "u861"}}
	m.prs["YAK-861"] = progress.PR{URL: "u861", State: "OPEN", Checks: progress.Checks{Passed: 1}}
	text = strings.Join(screen(m), "\n")
	if pos(text, "Questions ouvertes") > pos(text, "── PR #3321") || strings.Contains(text, "Prêt à merger") {
		t.Error("plan with a PR: questions first, no merge checklist")
	}
	m.cursor = 1 // push waiting: side effects first, done tasks folded
	text = strings.Join(screen(m), "\n")
	if pos(text, "── Effets de bord") > pos(text, "── Tâches") {
		t.Error("push: side effects before tasks")
	}
	if !strings.Contains(text, "2 tâches faites  › afficher") || strings.Contains(text, "Rendu markdown") {
		t.Error("push: done tasks must fold")
	}
	m = press(t, m, "z")
	if text = strings.Join(screen(m), "\n"); !strings.Contains(text, "Rendu markdown") {
		t.Error("z unfolds")
	}
	m.cursor = 3 // CI broken: merge checklist first
	text = strings.Join(screen(m), "\n")
	if !strings.Contains(text, "✗ CI · e2e, typecheck") {
		t.Errorf("review: checklist shows the failing CI\n%s", text)
	}
}

func TestThumb(t *testing.T) {
	if s, n := thumb(10, 20, 0); n != 0 || s != 0 {
		t.Error("no thumb when it fits")
	}
	if s, n := thumb(100, 20, 0); s != 0 || n != 4 {
		t.Errorf("top: %d %d", s, n)
	}
	if s, n := thumb(100, 20, 80); s+n != 20 {
		t.Errorf("bottom: %d %d", s, n)
	}
}

func TestPlace(t *testing.T) {
	out := place([]string{"aaaaaaaaaa", "bbbbbbbbbb"}, []string{"XY"}, 3, 1, 10)
	if got := ansi.Strip(out[1]); got != "bbbXYbbbbb" {
		t.Errorf("place = %q", got)
	}
}

func TestFilterAndPalette(t *testing.T) {
	m := fixture(t, 160, 45)
	m = press(t, m, "/", "8", "7", "4")
	if c := m.selected(); c == nil || c.Key != "YAK-874" {
		t.Fatalf("filter moves the selection to a match, got %v", m.selected())
	}
	text := strings.Join(screen(m), "\n")
	if strings.Contains(text, "YAK-861") {
		t.Error("filter hides the others")
	}
	m = press(t, m, "esc")
	if m.filter != "" || m.filtering {
		t.Error("esc clears the filter")
	}

	m = press(t, m, "ctrl+k", "8", "0", "0", "space", "p", "u", "s", "h")
	found := m.paletteFound()
	if len(found) == 0 || !strings.Contains(found[0].label, "YAK-800 · Autoriser le push") {
		t.Fatalf("palette found %v", found)
	}
	next, _ := found[0].run(m)
	m = next.(Model)
	if c := m.selected(); c.Key != "YAK-800" || m.overlay != "push" {
		t.Errorf("palette runs the action on its ticket: %s %q", c.Key, m.overlay)
	}
	if text := strings.Join(screen(m), "\n"); !strings.Contains(text, "Autoriser le push de YAK-800") || !strings.Contains(text, "1 reste ouvert") {
		t.Errorf("push confirmation:\n%s", text)
	}
}

func TestAnswerBox(t *testing.T) {
	m := fixture(t, 160, 45)
	m = press(t, m, "e")
	if m.answerQ != "Reprendre la PR #3321 ?" {
		t.Fatalf("e opens the first open question, got %q", m.answerQ)
	}
	m = press(t, m, "n", "o", "n", "space", "!")
	if m.answerText != "non !" {
		t.Errorf("typed %q", m.answerText)
	}
	if text := strings.Join(screen(m), "\n"); !strings.Contains(text, "non !") || !strings.Contains(text, "envoyer à l'agent") {
		t.Errorf("answer box:\n%s", text)
	}
	m = press(t, m, "esc")
	if m.answerQ != "" {
		t.Error("esc closes the box")
	}
}

func TestNarrowShowsOnePane(t *testing.T) {
	m := fixture(t, 80, 30)
	text := strings.Join(screen(m), "\n")
	if strings.Contains(text, "Phase") {
		t.Error("narrow: the list alone")
	}
	m = press(t, m, "tab")
	text = strings.Join(screen(m), "\n")
	if !strings.Contains(text, "‹ esc liste") || !strings.Contains(text, "Phase") {
		t.Errorf("narrow: tab shows the detail\n%s", text)
	}
	m = press(t, m, "esc")
	if m.detailFocus {
		t.Error("narrow: esc goes back to the list")
	}
}

func TestFoldList(t *testing.T) {
	m := fixture(t, 160, 45)
	if !strings.Contains(strings.Join(screen(m), "\n"), "Attend ta décision") {
		t.Fatal("the list shows by default")
	}
	m = press(t, m, "\\")
	text := strings.Join(screen(m), "\n")
	if strings.Contains(text, "Attend ta décision") || !strings.Contains(text, "afficher la liste") || !strings.Contains(text, "Phase") {
		t.Errorf("\\ folds the list, the detail takes the width\n%s", text)
	}
	if m.detailWidth() < m.width-4 {
		t.Errorf("folded: detail %d wide on %d", m.detailWidth(), m.width)
	}
	for _, k := range []string{"\\", "esc", "left"} {
		f := press(t, press(t, m, "\\"), "\\") // folded again
		f = press(t, f, k)
		if f.listHidden || !strings.Contains(strings.Join(screen(f), "\n"), "Attend ta décision") {
			t.Errorf("%s brings the list back", k)
		}
	}
	if n := press(t, fixture(t, 80, 30), "\\"); n.listHidden {
		t.Error("narrow: nothing to fold")
	}
}

func TestMargins(t *testing.T) {
	m := fixture(t, 10, 10)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 45})
	m = next.(Model)
	lines := screen(m)
	if len(lines) != 44 || lines[0] != "" || !strings.HasPrefix(lines[1], "  cairn") {
		t.Fatalf("a blank row on top, two columns on the left\n%s", strings.Join(lines, "\n"))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w > 160 {
			t.Errorf("line %d is %d wide", i, w)
		}
	}
	// A click on the footer, margins included, still hits its first button.
	// The footer is the last row: hovering its first button, margins included,
	// lights it.
	next, _ = m.Update(tea.MouseMsg{X: 3, Y: len(lines) - 1, Action: tea.MouseActionMotion})
	if next.(Model).hoverBtn != 0 {
		t.Errorf("hover on the first button: %d", next.(Model).hoverBtn)
	}
	next, _ = m.Update(tea.MouseMsg{X: 0, Y: len(lines) - 1, Action: tea.MouseActionMotion})
	if next.(Model).hoverBtn != -1 {
		t.Error("the margin is not a button")
	}
}

// TestDump writes the screens to $CAIRN_DUMP to look at them.
func TestDump(t *testing.T) {
	dir := os.Getenv("CAIRN_DUMP")
	if dir == "" {
		t.Skip("CAIRN_DUMP not set")
	}
	m := fixture(t, 160, 45)
	m.rebuildOrder()
	shots := map[string]Model{"main": m, "board": press(t, m, "t"), "help": press(t, m, "?"), "palette": press(t, m, "ctrl+k", "8", "0", "0"),
		"answer": press(t, m, "e", "o", "u", "i"), "narrow": press(t, fixture(t, 80, 40), "tab"), "folded": press(t, m, "\\"), "cost": withCost(m)}
	for i := range m.chantiers {
		mm := m
		mm.cursor = i
		shots["detail-"+m.chantiers[i].Key] = mm
	}
	push := m
	push.cursor = 1
	shots["push"] = press(t, push, "a")
	for name, v := range shots {
		_ = os.WriteFile(filepath.Join(dir, name+".txt"), []byte(ansi.Strip(v.View())), 0o644)
		_ = os.WriteFile(filepath.Join(dir, name+".ans"), []byte(v.View()), 0o644)
	}
}

func TestNewsSinceLastVisit(t *testing.T) {
	m := fixture(t, 160, 45)
	visit := time.Now().Add(-time.Hour)
	m.seen = &seenStore{at: map[string]time.Time{"YAK-788": visit, "YAK-712": visit}}
	pr := m.prs["YAK-788"]
	pr.Author = "Gradrien"
	pr.Activity = []progress.Activity{
		{Who: "BATTAULT-p", At: visit.Add(-time.Hour), Body: "old"}, // before the visit
		{Who: "Gradrien", At: visit.Add(time.Minute), Body: "mine"}, // your own
		{Who: "github-actions", At: visit.Add(time.Minute), Body: "## PR Reviewer Guide"},
		{Who: "BATTAULT-p", At: visit.Add(10 * time.Minute), Review: true, State: "COMMENTED",
			Body: "> 🤖 **Automated review** by the Yalink console agent. Verdict below is advisory.\n\n**Verdict:** changes requested — two blocking findings."},
		{Who: "BATTAULT-p", At: visit.Add(11 * time.Minute), Review: true, State: "COMMENTED"},
	}
	m.prs["YAK-788"] = pr
	m.watch.history["YAK-712"] = []journal.Event{{At: visit.Add(5 * time.Minute), Text: "phase:" + chantier.Build.String()}}

	for i, c := range m.chantiers {
		if c.Key == "YAK-788" {
			m.cursor = i
		}
	}
	text := strings.Join(screen(m), "\n")
	for _, want := range []string{"2 chantiers ont bougé", "nouveau sur la PR", "→ Build", "Nouveau depuis ta visite",
		"BATTAULT-p a laissé une review", "changes requested — two blocking findings", "c lire la conversation"} {
		if !strings.Contains(text, want) {
			t.Errorf("screen misses %q:\n%s", want, text)
		}
	}
	for _, not := range []string{"Gradrien", "github-actions"} {
		if strings.Contains(text, not) {
			t.Errorf("%s is not news", not)
		}
	}
	if n := m.news(m.chantiers[m.cursor]); len(n) != 1 {
		t.Fatalf("one line per person, got %d", len(n))
	}

	// Passing by does not count; leaving after a look does.
	m.detailOf, m.lookedAt = "YAK-788", time.Now()
	m.leave("YAK-788")
	if len(m.news(m.chantiers[m.cursor])) == 0 {
		t.Fatal("a glance is not a visit")
	}
	m.lookedAt = time.Now().Add(-3 * time.Second)
	m.leave("YAK-788")
	if len(m.news(m.chantiers[m.cursor])) != 0 {
		t.Fatal("seen after a look")
	}
}

func TestCloseAsksFirst(t *testing.T) {
	m := fixture(t, 160, 45)
	c := &chantier.Chantier{Key: "YAK-770", Phase: chantier.Shipped, Worktree: "/w/770", Branch: "adrien/yak-770",
		Issue: &tracker.Issue{ID: "YAK-770", Title: "CRA", Status: "Done", Done: true}}
	m.chantiers = append(m.chantiers, c)
	m.all = m.chantiers
	m.watch.keep["YAK-770"] = "changements non commités"
	m.cursor = len(m.chantiers) - 1
	text := strings.Join(screen(m), "\n")
	for _, want := range []string{"✓ Livré", "Gardé (changements non commités)", "x Fermer le chantier"} {
		if !strings.Contains(text, want) {
			t.Errorf("detail misses %q:\n%s", want, text)
		}
	}
	m = press(t, m, "x")
	next, _ := m.Update(closeRiskMsg{key: "YAK-770", risk: closeRisk{dirty: 2}})
	m = next.(Model)
	text = strings.Join(screen(m), "\n")
	for _, want := range []string{"Fermer YAK-770 ?", "2 fichiers non commités seront perdus", "Fermer et supprimer le worktree"} {
		if !strings.Contains(text, want) {
			t.Errorf("confirmation misses %q:\n%s", want, text)
		}
	}
	if m.busy != "" {
		t.Fatal("nothing happens before you confirm")
	}
	// Not shipped, no plan and no worktree: nothing to close or abandon.
	bare := &chantier.Chantier{Key: "YAK-771", Phase: chantier.ToPlan, Issue: &tracker.Issue{ID: "YAK-771", Title: "Rien"}}
	m.chantiers = append(m.chantiers, bare)
	m.all = m.chantiers
	m.overlay, m.cursor = "", len(m.chantiers)-1
	if m = press(t, m, "x"); m.overlay != "" {
		t.Fatal("a ticket with nothing to put away does not close")
	}
}

func TestAbandonAsksFirst(t *testing.T) {
	m := fixture(t, 160, 45)
	c := &chantier.Chantier{Key: "YAK-772", Phase: chantier.Plan, Worktree: "/w/772", Branch: "adrien/yak-772",
		State: &state.State{}, PRs: []tracker.PullRequest{{Number: 9, State: "draft"}},
		Issue: &tracker.Issue{ID: "YAK-772", Title: "Plus pour moi", Status: "Todo"}}
	m.chantiers = append(m.chantiers, c)
	m.all = m.chantiers
	m.cursor = len(m.chantiers) - 1
	if text := strings.Join(screen(m), "\n"); !strings.Contains(text, "x Abandonner") {
		t.Errorf("detail misses the abandon button:\n%s", text)
	}
	m = press(t, m, "x")
	next, _ := m.Update(closeRiskMsg{key: "YAK-772", risk: closeRisk{unpushed: 1}})
	m = next.(Model)
	text := strings.Join(screen(m), "\n")
	for _, want := range []string{"Abandonner YAK-772 ?", "1 commit absent de GitHub sera perdu", "#9", "reste ouverte", "désassigne-le", "Abandonner et supprimer le worktree"} {
		if !strings.Contains(text, want) {
			t.Errorf("confirmation misses %q:\n%s", want, text)
		}
	}
	if m.busy != "" {
		t.Fatal("nothing happens before you confirm")
	}
}
