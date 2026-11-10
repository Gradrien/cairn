package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"cairn/internal/state"
)

const howPlan = "---\nphase: build\n---\n# YAK-712 · Colonnes\n\nObjectif.\n\n- [x] Une\n\n" +
	"## Comment ça marche\n\nLa poignée règle la largeur ; rien n'est sauvé.\n\n" +
	"### Largeur d'une colonne\n- **Où** : `useColumnWidths`\n- **Quand** : au relâchement du geste\n" +
	"- **Données** : état React, perdu au rechargement\n\n### Panneau des Vues\n- **Où** : `TableViewsPanel`\n\n## Preuves\n- ok\n"

func selectKey(t *testing.T, m Model, key string) Model {
	t.Helper()
	for i, c := range m.chantiers {
		if c.Key == key {
			m.cursor = i
			return m
		}
	}
	t.Fatalf("no %s", key)
	return m
}

func TestHowItWorksView(t *testing.T) {
	m := selectKey(t, fixture(t, 120, 40), "YAK-712")
	c := m.selected()
	c.State = state.Parse(filepath.Join(t.TempDir(), "YAK-712.md"), howPlan)

	detail := ansi.Strip(strings.Join(m.detailLayout(m.detailWidth()).lines, "\n"))
	for _, want := range []string{"Comment ça marche", "rien n'est sauvé", "Largeur d'une colonne · Panneau des Vues", "(clic ou i)"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail misses %q:\n%s", want, detail)
		}
	}

	v := press(t, m, "i")
	if v.view != "how" {
		t.Fatalf("i opens the view, got %q", v.view)
	}
	text := strings.Join(screen(v), "\n")
	for _, want := range []string{"Comment ça marche · YAK-712", "Largeur d'une colonne", "useColumnWidths", "au relâchement", "Poser une question", "Mettre à jour"} {
		if !strings.Contains(text, want) {
			t.Errorf("view misses %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Preuves") {
		t.Error("the view shows only its section")
	}

	ask := press(t, v, "e", "o", "ù", "space", "?")
	if !ask.howAsk || ask.howText != "où ?" || !strings.Contains(strings.Join(screen(ask), "\n"), "un agent à part lit le code") {
		t.Fatalf("question box: ask=%v %q", ask.howAsk, ask.howText)
	}
	if back := press(t, ask, "esc"); back.howAsk || back.view != "how" {
		t.Fatal("esc closes the box, not the view")
	}
	if back := press(t, v, "esc"); back.view != "" {
		t.Fatal("esc leaves the view")
	}
	for _, size := range [][2]int{{80, 24}, {160, 45}} {
		v.width, v.height = size[0], size[1]
		ask.width, ask.height = size[0], size[1]
		for _, mm := range []Model{v, ask} {
			lines := screen(mm)
			if len(lines) > size[1] {
				t.Errorf("%dx%d: %d lines", size[0], size[1], len(lines))
			}
			for i, l := range lines {
				if ansi.StringWidth(l) > size[0] {
					t.Errorf("%dx%d line %d too wide: %q", size[0], size[1], i, l)
				}
			}
		}
	}
}

func TestHowItWorksMissing(t *testing.T) {
	m := selectKey(t, fixture(t, 120, 40), "YAK-712")
	detail := ansi.Strip(strings.Join(m.detailLayout(m.detailWidth()).lines, "\n"))
	if !strings.Contains(detail, "Pas encore écrit par l'agent") {
		t.Errorf("a build without the section offers to ask for it:\n%s", detail)
	}
	text := strings.Join(screen(press(t, m, "i")), "\n")
	if !strings.Contains(text, "demander l'explication") || !strings.Contains(text, "Demander l'explication") {
		t.Errorf("empty view says how to get it:\n%s", text)
	}
	// Planning: nothing to explain yet, no section.
	p := selectKey(t, m, "YAK-861")
	if strings.Contains(ansi.Strip(strings.Join(p.detailLayout(p.detailWidth()).lines, "\n")), "Comment ça marche") {
		t.Error("no section while planning")
	}
}

func TestHowPromptAsksForTheSection(t *testing.T) {
	p := howPrompt("YAK-854", "/c/YAK-854.md")
	for _, want := range []string{"## Comment ça marche", "/c/YAK-854.md", "Ne modifie pas le code", "**Quand**"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt misses %q", want)
		}
	}
}

// A question goes to an agent of its own, in the worktree; cairn writes its
// answer in the plan.
func TestAskAside(t *testing.T) {
	t.Setenv("CAIRN_HOME", t.TempDir())
	m := selectKey(t, fixture(t, 120, 40), "YAK-800")
	c := m.selected()
	path := filepath.Join(t.TempDir(), "YAK-800.md")
	if err := os.WriteFile(path, []byte(howPlan), 0o644); err != nil {
		t.Fatal(err)
	}
	c.State, _ = state.Load(path)
	c.Worktree = "/w/800"
	var gotDir, gotPrompt string
	old := ask
	ask = func(dir, prompt string) (string, error) {
		gotDir, gotPrompt = dir, prompt
		return "Nulle part : en mémoire, `useColumnWidths.ts:12`.", nil
	}
	defer func() { ask = old }()

	next, _ := m.openHow()
	m = next.(Model)
	next, cmd := m.askAside("où est sauvée la taille ?")
	m = next.(Model)
	if cmd == nil || !strings.Contains(m.flash, "agent à part") {
		t.Fatalf("flash %q", m.flash)
	}
	if msg, ok := cmd().(doneMsg); !ok || msg.err != nil {
		t.Fatalf("done: %+v", msg)
	}
	if gotDir != "/w/800" || !strings.Contains(gotPrompt, "où est sauvée la taille ?") || !strings.Contains(gotPrompt, path) {
		t.Errorf("asked in %q: %q", gotDir, gotPrompt)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "### Questions posées\n- **où est sauvée la taille ?**\n  Nulle part : en mémoire, `useColumnWidths.ts:12`.") {
		t.Errorf("the answer is in the plan:\n%s", data)
	}
}

func TestHowItWorksArchived(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAIRN_HOME", home)
	arch := filepath.Join(home, "archive")
	if err := os.MkdirAll(arch, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(arch, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("YAK-854-20261109-120000.md", strings.Replace(howPlan, "YAK-712 · Colonnes", "YAK-854 · Largeur des colonnes", 1))
	write("YAK-700-20261001-090000.md", "---\nticket: YAK-700\n---\n# YAK-700 · Ancien export\n\nExport en CSV.\n\n- [x] Bouton export\n")
	write("YAK-712-20261001-090000.md", "---\nticket: YAK-712\n---\n# YAK-712 · Encore en cours\n")

	m := fixture(t, 120, 40)
	m.all = m.chantiers
	m.refreshArchive()
	if len(m.archive) != 3 {
		t.Fatalf("archive: %d", len(m.archive))
	}

	m.overlay, m.palQuery = "palette", "854"
	found := m.paletteFound()
	if len(found) != 1 || !strings.Contains(found[0].label, "Largeur des colonnes · livré, comment ça marche") {
		t.Fatalf("palette finds the shipped ticket: %+v", found)
	}
	m.palQuery = "712 livré"
	if f := m.paletteFound(); len(f) != 0 {
		t.Errorf("a live chantier is not listed from its old archive: %d", len(f))
	}
	m.palQuery = "tickets livrés"
	f := m.paletteFound()
	if len(f) != 1 {
		t.Fatalf("narrowing entry: %d", len(f))
	}
	next, _ := f[0].run(m)
	if n := next.(Model); n.overlay != "palette" || len(n.paletteFound()) != 2 {
		t.Fatalf("it narrows to the shipped tickets: %q %d", n.palQuery, len(n.paletteFound()))
	}

	m.palQuery = "854"
	next, _ = m.paletteFound()[0].run(m)
	v := next.(Model)
	text := strings.Join(screen(v), "\n")
	for _, want := range []string{"YAK-854 · Largeur des colonnes · livré, rangé le 09/11/2026", "useColumnWidths", "Retour"} {
		if !strings.Contains(text, want) {
			t.Errorf("archived view misses %q:\n%s", want, text)
		}
	}
	if v.howSubj().agent() != "" {
		t.Error("nobody to ask about an archived ticket")
	}
	if back := press(t, v, "esc"); back.view != "" || back.howArch != "" {
		t.Error("esc leaves the archived view")
	}

	m.palQuery = "700"
	next, _ = m.paletteFound()[0].run(m)
	text = strings.Join(screen(next.(Model)), "\n")
	if !strings.Contains(text, "Livré avant que l'agent l'explique") || !strings.Contains(text, "Bouton export") || strings.Contains(text, "# YAK-700") {
		t.Errorf("unexplained ticket shows its plan:\n%s", text)
	}
}
