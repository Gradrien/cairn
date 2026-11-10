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

	// A click on a step ticks it in the plan file.
	target := ""
	for _, tg := range d.targets {
		if tg == "manual:0" {
			target = tg
		}
	}
	if target == "" {
		t.Fatal("step 1 is not clickable")
	}
	nm, _ := m.detailClick(target)
	s, _ := state.Load(path)
	if !s.ManualSteps[0].Done || s.ManualSteps[1].Done {
		t.Fatalf("ticked: %+v", s.ManualSteps)
	}
	m = nm.(Model)
	if text := ansi.Strip(strings.Join(m.detailLayout(m.detailWidth()).lines, "\n")); !strings.Contains(text, "Tester à la main 1/2") || !strings.Contains(text, "✓  Clique") {
		t.Errorf("after tick:\n%s", text)
	}
	m.detailClick("manual:0")
	if s, _ := state.Load(path); s.ManualSteps[0].Done {
		t.Fatal("a second click unticks")
	}
}
