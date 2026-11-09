package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"cairn/internal/update"
)

func withUpdate(t *testing.T, m Model) Model {
	t.Helper()
	compare := func(context.Context, string) ([]update.Change, bool, error) {
		return []update.Change{
			{Subject: "Push libre une fois la PR ouverte", Body: "Les correctifs suivants poussent\nsans demander.", Date: time.Now()},
			{Subject: "Interface : l'action attendue d'abord"},
		}, true, nil
	}
	return m.WithUpdates(Updates{File: filepath.Join(t.TempDir(), "update.json"), Rev: "abc", SourceDir: "/src/cairn", Compare: compare})
}

// run feeds a command's message back into the model, as bubbletea would.
func run(m Model, cmd tea.Cmd) Model {
	next, _ := m.Update(cmd())
	return next.(Model)
}

func TestUpdateOfferShowsChangelog(t *testing.T) {
	m := withUpdate(t, fixture(t, 140, 40))
	m = run(m, m.checkUpdate(update.Fresh))
	if m.overlay != "update" {
		t.Fatalf("an update found should be offered, overlay %q", m.overlay)
	}
	view := strings.Join(screen(m), "\n")
	for _, want := range []string{"Nouvelle version de cairn", "2 commits sur main", "Push libre une fois la PR ouverte",
		"Les correctifs suivants poussent sans demander.", "Interface : l'action attendue d'abord", "Mettre à jour", "Plus tard"} {
		if !strings.Contains(view, want) {
			t.Errorf("the offer should show %q:\n%s", want, view)
		}
	}

	// Later: closed, not offered again today, the header keeps a reminder.
	m = press(t, m, "enter", "esc")
	if m.overlay != "" || m.UpdateRequested() {
		t.Fatalf("esc should put it off, overlay %q", m.overlay)
	}
	if !strings.Contains(screen(m)[0], "mise à jour") {
		t.Errorf("the header should remind of the update: %q", screen(m)[0])
	}
	m = run(m, m.checkUpdate(0))
	if m.overlay != "" {
		t.Fatal("an update put off should not pop up again the same day")
	}

	// U shows it again, U there installs.
	m = press(t, m, "U")
	if m.overlay != "update" {
		t.Fatalf("U should show the changelog, overlay %q", m.overlay)
	}
	next, cmd := m.key(keyFor("U"))
	m = next.(Model)
	if !m.UpdateRequested() || cmd == nil {
		t.Fatal("U in the offer should quit to install")
	}
}

func TestUpdateWaitsForOpenOverlay(t *testing.T) {
	m := withUpdate(t, fixture(t, 140, 40))
	m = press(t, m, "?")
	m = run(m, m.checkUpdate(update.Fresh))
	if m.overlay != "help" {
		t.Fatalf("the offer should not cover the help, overlay %q", m.overlay)
	}
	m = press(t, m, "esc")
	next, _ := m.Update(tickLocal{}) // any message once the help is closed
	if m = next.(Model); m.overlay != "update" {
		t.Fatalf("the offer should come once the help is closed, overlay %q", m.overlay)
	}
}
