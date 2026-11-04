package tui

import (
	"testing"

	"cairn/internal/chantier"
	"cairn/internal/config"
	"cairn/internal/sprintosaur"
)

func TestPickKeyAndList(t *testing.T) {
	m := Model{cfg: config.Config{Team: "YAK"}, pick: &pickState{}}
	for in, want := range map[string]string{"123": "YAK-123", "yak-9": "YAK-9", "SPI-4": "SPI-4", "": ""} {
		m.pick.input = in
		if got := m.pickKey(); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
	me := &struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}{ID: "adrien"}
	m.chantiers = []*chantier.Chantier{{Key: "YAK-2"}}
	got := m.pickList([]sprintosaur.Issue{
		{ID: "YAK-1", PriorityValue: 0},
		{ID: "YAK-2"},               // already a chantier
		{ID: "YAK-3", Assignee: me}, // taken
		{ID: "YAK-4", PriorityValue: 1},
	})
	if len(got) != 2 || got[0].ID != "YAK-4" || got[1].ID != "YAK-1" {
		t.Fatalf("got %+v", got)
	}
}
