package tui

import (
	"strings"
	"testing"

	"cairn/internal/localtest"
	"cairn/internal/progress"
)

func TestTestHereConfirmsFirst(t *testing.T) {
	m := fixture(t, 140, 40)
	m.all = m.chantiers
	m.cursor = 1 // YAK-800, in its worktree
	if b := m.testButton(m.selected()); !b.enabled || b.label != "Tester ici" {
		t.Fatalf("a chantier in a worktree should offer Tester ici, got %+v", b)
	}
	m.gits["YAK-800"] = progress.Git{Uncommitted: 2}
	next, cmd := m.testKey()
	m = next.(Model)
	if m.overlay != "localtest" || cmd == nil {
		t.Fatalf("h should ask first, overlay %q", m.overlay)
	}
	m.ltPlan = &localtest.Plan{From: "adrien/yak-861", Dirty: 3, Commits: 2, DepsDiff: true}
	view := strings.Join(screen(m), "\n")
	for _, want := range []string{"Tester YAK-800 dans yak ?", "adrien/yak-861", "3 changements non commités", "restaurés à la fin du test",
		"2 changements non commités du worktree ne sont pas inclus", "les dépendances changent", "Tester ici", "Annuler"} {
		if !strings.Contains(view, want) {
			t.Errorf("the confirmation should say %q:\n%s", want, view)
		}
	}
	m = press(t, m, "esc")
	if m.overlay != "" {
		t.Fatal("esc should cancel")
	}

	// Under test: h catches up when the agent committed since, else ends it.
	m.tests = map[string]localtest.Status{"yak": {Session: localtest.Session{Key: "YAK-800"}, Head: "a", Tip: "b"}}
	if b := m.testButton(m.selected()); b.label != "Actualiser le test" {
		t.Fatalf("got %q, want Actualiser le test", b.label)
	}
	m.tests["yak"] = localtest.Status{Session: localtest.Session{Key: "YAK-800"}, Head: "b", Tip: "b"}
	if b := m.testButton(m.selected()); b.label != "Fin du test" {
		t.Fatalf("got %q, want Fin du test", b.label)
	}
	if !strings.Contains(screen(m)[0], "yak : test YAK-800") {
		t.Errorf("the header should say a test is under way: %q", screen(m)[0])
	}
}
