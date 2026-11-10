package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"cairn/internal/state"
)

func TestTickSideEffect(t *testing.T) {
	t.Setenv("CAIRN_HOME", t.TempDir())
	m := selectKey(t, fixture(t, 120, 40), "YAK-800")
	c := m.selected()
	path := filepath.Join(t.TempDir(), "YAK-800.md")
	plan := "---\nphase: build\n---\n# YAK-800 · Export\n\n- [x] fait\n\n## Effets de bord\n- [ ] Autres appelants\n- [ ] Quota Novu\n"
	if err := os.WriteFile(path, []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	c.State, _ = state.Load(path)
	d := m.detailLayout(m.detailWidth())
	if !hasTarget(d.targets, "side:1") || !strings.Contains(ansi.Strip(strings.Join(d.lines, "\n")), "clic : cocher ce que tu as vérifié") {
		t.Fatal("each side effect is clickable")
	}
	next, _ := m.detailClick("side:1")
	m = next.(Model)
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "- [x] Quota Novu") || strings.Contains(string(data), "- [x] Autres") {
		t.Errorf("the click ticks that one in the plan:\n%s", data)
	}
	if text := ansi.Strip(strings.Join(m.detailLayout(m.detailWidth()).lines, "\n")); !strings.Contains(text, "Effets de bord 1/2 vérifiés") {
		t.Errorf("the count follows:\n%s", text)
	}
	if !strings.Contains(m.flash, "coché") {
		t.Errorf("flash: %q", m.flash)
	}
	next, _ = m.detailClick("side:1")
	data, _ = os.ReadFile(path)
	if !strings.Contains(string(data), "- [ ] Quota Novu") {
		t.Errorf("a second click unticks it:\n%s", data)
	}
}
