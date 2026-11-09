package tui

import (
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
	if !ask.howAsk || ask.howText != "où ?" || !strings.Contains(strings.Join(screen(ask), "\n"), "l'agent répond dans cette section") {
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
	p := howPrompt("YAK-854", "/c/YAK-854.md", "où est sauvée la taille ?")
	for _, want := range []string{"## Comment ça marche", "/c/YAK-854.md", "où est sauvée la taille ?", "### Questions posées", "Ne modifie pas le code", "**Quand**"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt misses %q", want)
		}
	}
}
