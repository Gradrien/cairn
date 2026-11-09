package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"cairn/internal/state"
)

func TestOpenThePRIsItsOwnStep(t *testing.T) {
	t.Setenv("CAIRN_HOME", t.TempDir())
	m := selectKey(t, fixture(t, 140, 45), "YAK-800")

	// Waiting for a push: allowing it and opening the PR are two buttons.
	cd := m.cardFor(m.selected())
	if cd.actions[0].key != "a" || cd.actions[1].key != "P" || !cd.actions[1].enabled {
		t.Fatalf("push card: %+v", cd.actions)
	}
	push := press(t, m, "a")
	if push.overlay != "push" || push.pushPR || !strings.Contains(strings.Join(screen(push), "\n"), "La PR ne s'ouvre pas") {
		t.Fatal("allowing the push says the PR stays shut")
	}

	// Waiting for the PR: its own card, its own confirmation.
	c := m.selected()
	c.State = state.Parse(filepath.Join(t.TempDir(), "YAK-800.md"), "---\nphase: build\nattente: Prêt à ouvrir la PR — le résumé\npush: autorisé abc1234\n---\n- [x] fait\n")
	cd = m.cardFor(c)
	if !strings.Contains(cd.title, "ouvrir la PR") || cd.actions[0].key != "P" {
		t.Fatalf("PR card: %q %+v", cd.title, cd.actions)
	}
	for _, b := range m.chantierButtons(c) {
		if b.key == "v" {
			t.Error("a PR wait is not approved with Valider")
		}
	}
	o := press(t, m, "P")
	if o.overlay != "push" || !o.pushPR || !strings.Contains(strings.Join(screen(o), "\n"), "Ouvrir la PR de YAK-800 ?") {
		t.Fatalf("P asks to confirm the PR: %q %v", o.overlay, o.pushPR)
	}
	done := press(t, o, "enter")
	s, _ := state.Load(c.State.Path)
	if s == nil || !s.PRAllowed() || s.Waiting != "" || done.overlay != "" {
		t.Fatalf("confirmed: %+v", s)
	}
	if !strings.Contains(strings.Join(m.detailLayout(m.detailWidth()).lines, "\n"), "ouverture autorisée") {
		t.Error("the detail says the PR may open")
	}
}

func TestPushAskedInChatShowsAsAllowed(t *testing.T) {
	m := selectKey(t, fixture(t, 140, 45), "YAK-712")
	c := m.selected()
	c.State = state.Parse(filepath.Join(t.TempDir(), "YAK-712.md"), "---\nphase: build\npush: autorisé\n---\n- [~] une\n")
	if !m.pushValid(c) {
		t.Fatal("an unpinned allowance is valid")
	}
	text := strings.Join(m.detailLayout(m.detailWidth()).lines, "\n")
	if !strings.Contains(text, "autorisé à ta demande") || !strings.Contains(text, "Ouvrir la PR (P)") {
		t.Errorf("detail:\n%s", text)
	}
}
