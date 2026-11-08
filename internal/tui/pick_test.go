package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"cairn/internal/chantier"
	"cairn/internal/config"
	"cairn/internal/tracker"
)

func TestPickKeyAndList(t *testing.T) {
	m := Model{projects: []*project{{cfg: config.Project{Key: "YAK"}}}, pick: &pickState{}}
	for in, want := range map[string]string{"123": "YAK-123", "yak-9": "YAK-9", "SPI-4": "SPI-4", "": ""} {
		m.pick.input = in
		if got := m.pickKey(); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
	m.chantiers = []*chantier.Chantier{{Key: "YAK-2"}}
	got := m.pickList([]tracker.Issue{
		{ID: "YAK-1", PriorityValue: 0},
		{ID: "YAK-2"},                 // already a chantier
		{ID: "YAK-3", Assigned: true}, // taken
		{ID: "YAK-4", PriorityValue: 1},
	})
	if len(got) != 2 || got[0].ID != "YAK-4" || got[1].ID != "YAK-1" {
		t.Fatalf("got %+v", got)
	}
}

// Without a tracker, "new ticket" takes a title, spaces and accents included;
// tab goes to the other project, which takes a number.
func TestPickLocalTitle(t *testing.T) {
	dir := t.TempDir()
	m := Model{width: 120, height: 30, watch: newWatch(), projects: []*project{
		{cfg: config.Project{Name: "eido", Key: "EIDO", Tracker: "local"}, tr: tracker.NewLocal("EIDO", dir)},
		{cfg: config.Project{Name: "yak", Key: "YAK", Tracker: "sprintosaur"}, err: errTest},
	}}
	next, _ := m.openPick()
	m = next.(Model)
	for _, k := range []tea.KeyMsg{keyForRunes("Écran"), {Type: tea.KeySpace}, keyForRunes("vide")} {
		next, _ = m.pickKeyPress(k)
		m = next.(Model)
	}
	if !m.pickCreates() || m.pick.input != "Écran vide" {
		t.Fatalf("input %q", m.pick.input)
	}
	if v := m.pickView(); !strings.Contains(v, "Titre") || !strings.Contains(v, "EIDO-") {
		t.Fatalf("view:\n%s", v)
	}
	next, _ = m.pickKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if m.pickCreates() || m.pick.input != "" || m.pickProject().cfg.Key != "YAK" {
		t.Fatal("tab: the other project")
	}
	m.pick.input = "12"
	if m.pickKey() != "YAK-12" {
		t.Fatal(m.pickKey())
	}
}

var errTest = errors.New("no credentials")
