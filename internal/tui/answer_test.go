package tui

import (
	"strings"
	"testing"

	"cairn/internal/state"
)

// A question with options opens as a list, like an agent's own question:
// pick one, or answer in your own words.
func TestAnswerChoices(t *testing.T) {
	m := fixture(t, 160, 60)
	c := m.chantiers[0]
	c.State = state.Parse(c.State.Path, "---\nphase: plan\nattente: Valider le plan\n---\n# YAK-861 · T\n\n- [ ] Une tâche : en deux temps :\n  - d'abord ceci ;\n  - puis cela.\n\n## Questions\n"+
		"- **Spé orpheline** : que fait-on pour un deal sans métier ?\n  - (a) Garder le ciblage actuel.\n  - (b) Élargir l'envoi.\n\n  Je recommande (b) : il touche tout le monde.\n")
	text := strings.Join(screen(m), "\n")
	for _, want := range []string{"Questions ouvertes 1", "(a) Garder le ciblage actuel.", "(b) Élargir l'envoi.  ★ recommandé", "Je recommande (b)", "puis cela."} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
	m = press(t, m, "e")
	if m.answerFree || m.answerOpt != 1 {
		t.Fatalf("opens on the recommended option: free %v opt %d", m.answerFree, m.answerOpt)
	}
	if text := strings.Join(screen(m), "\n"); !strings.Contains(text, "✎ Autre réponse") || !strings.Contains(text, "↑↓ choisir") {
		t.Errorf("choice list:\n%s", text)
	}
	m = press(t, m, "down", "enter")
	if !m.answerFree {
		t.Fatal("the last line opens the box")
	}
	m = press(t, m, "esc")
	if m.answerFree || m.answerQ == "" {
		t.Fatal("esc goes back to the choices")
	}
	m = press(t, m, "a", "enter")
	if got := m.answered[answerKey("YAK-861", c.State.Questions[0].Text)]; got != "(a) Garder le ciblage actuel." || m.answerQ != "" {
		t.Errorf("sent %q", got)
	}
	if text := strings.Join(screen(m), "\n"); !strings.Contains(text, "↳ envoyé à l'agent : (a) Garder") {
		t.Errorf("answered:\n%s", text)
	}
}

func TestClickOption(t *testing.T) {
	m := fixture(t, 160, 60)
	c := m.chantiers[0]
	c.State = state.Parse(c.State.Path, "---\nphase: plan\n---\n## Questions\n- Lequel ?\n  - (a) x\n  - (b) y\n")
	next, _ := m.detailClick("answeropt:0:1")
	m = next.(Model)
	if m.answerQ != "Lequel ?" || m.answerOpt != 1 || m.answerFree {
		t.Fatalf("a first click picks: %q %d %v", m.answerQ, m.answerOpt, m.answerFree)
	}
	next, _ = m.detailClick("answeropt:0:1")
	m = next.(Model)
	if m.answered[answerKey("YAK-861", "Lequel ?")] != "(b) y" {
		t.Errorf("a second click sends: %v", m.answered)
	}
}
