package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"cairn/internal/state"
)

func TestManualTestSection(t *testing.T) {
	t.Setenv("CAIRN_HOME", t.TempDir())
	m := selectKey(t, fixture(t, 120, 40), "YAK-800")
	c := m.selected()
	path := filepath.Join(t.TempDir(), "YAK-800.md")
	plan := "---\nphase: build\nattente: Prêt à pousser — export\n---\n# YAK-800 · Export\n\n- [x] fait\n\n" +
		"## Tester à la main\nLance `pnpm dev`, ouvre /freelances.\n\n" +
		"- [ ] Clique sur « Exporter » → un fichier `freelances.csv` se télécharge\n" +
		"- [ ] Ouvre le fichier → la colonne Modèle est remplie\n\n## Preuves\n- ok\n"
	if err := os.WriteFile(path, []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	c.State, _ = state.Load(path)

	d := m.detailLayout(m.detailWidth())
	text := ansi.Strip(strings.Join(d.lines, "\n"))
	for _, want := range []string{"Tester à la main 0/2", "· Lance pnpm dev", "1. Clique sur « Exporter » → un fichier freelances.csv", "2. Ouvre le fichier"} {
		if !strings.Contains(text, want) {
			t.Errorf("detail misses %q:\n%s", want, text)
		}
	}
	if sub := m.cardFor(c).sub; !strings.Contains(sub, "test à la main 0/2") {
		t.Errorf("card sub: %q", sub)
	}

	// A click on a step to do opens its two actions; « ça marche » ticks it.
	if !hasTarget(d.targets, "manual:0") {
		t.Fatal("step 1 is not clickable")
	}
	nm, _ := m.detailClick("manual:0")
	m = nm.(Model)
	text = ansi.Strip(strings.Join(m.detailLayout(m.detailWidth()).lines, "\n"))
	if !strings.Contains(text, "› ✓ ça marche") || !strings.Contains(text, "› ✗ ça ne marche pas") {
		t.Fatalf("actions:\n%s", text)
	}
	nm, _ = m.detailClick("manual-ok:0")
	m = nm.(Model)
	s, _ := state.Load(path)
	if !s.ManualSteps[0].Done || s.ManualSteps[1].Done {
		t.Fatalf("ticked: %+v", s.ManualSteps)
	}
	if text := ansi.Strip(strings.Join(m.detailLayout(m.detailWidth()).lines, "\n")); !strings.Contains(text, "Tester à la main 1/2") || !strings.Contains(text, "✓  Clique") || strings.Contains(text, "› ✓ ça marche") {
		t.Errorf("after tick:\n%s", text)
	}
	nm, _ = m.detailClick("manual:0")
	m = nm.(Model)
	if s, _ := state.Load(path); s.ManualSteps[0].Done {
		t.Fatal("a click on a ticked step unticks it")
	}

	// « ça ne marche pas » opens a box; what you type goes to the agent.
	nm, _ = m.detailClick("manual:1")
	nm, _ = nm.(Model).detailClick("manual-ko:1")
	m = nm.(Model)
	if !m.manualAsk {
		t.Fatal("the box opens")
	}
	m = press(t, m, "v", "i", "d", "e")
	if m.manualText != "vide" || !strings.Contains(ansi.Strip(strings.Join(m.detailLayout(m.detailWidth()).lines, "\n")), "ce que tu vois à la place") {
		t.Fatalf("typed %q", m.manualText)
	}
	if back := press(t, m, "esc"); back.manualAsk || back.manualText != "" {
		t.Fatal("esc closes the box")
	}
	p := manualPrompt("YAK-800", path, 2, "Ouvre le fichier → la colonne Modèle est remplie", "Lance pnpm dev.", "vide")
	for _, want := range []string{"l'étape 2 ne marche pas", "« vide »", "Ne coche pas l'étape", "Lance pnpm dev."} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt misses %q:\n%s", want, p)
		}
	}

	// A step reported shows it, until it is ticked.
	m.answered[manualKey("YAK-800", s.ManualSteps[1].Text)] = "vide"
	m.manualSel, m.manualAsk = "", false
	text = ansi.Strip(strings.Join(m.detailLayout(m.detailWidth()).lines, "\n"))
	if !strings.Contains(text, "✗  Ouvre le fichier") || !strings.Contains(text, "↳ signalé à l'agent : vide") {
		t.Errorf("reported:\n%s", text)
	}
	nm, _ = m.detailClick("manual:1")
	nm, _ = nm.(Model).detailClick("manual-ok:1")
	if _, still := nm.(Model).answered[manualKey("YAK-800", s.ManualSteps[1].Text)]; still {
		t.Error("ticking drops the report")
	}
}

func hasTarget(ts []string, want string) bool {
	for _, t := range ts {
		if t == want {
			return true
		}
	}
	return false
}
