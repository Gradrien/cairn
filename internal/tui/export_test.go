package tui

import (
	"encoding/json"
	"os"
	"testing"

	"cairn/internal/chantier"
	"cairn/internal/tracker"
)

func TestExportBoard(t *testing.T) {
	t.Setenv("CAIRN_HOME", t.TempDir())
	m := fixture(t, 140, 40)
	m.chantiers = append(m.chantiers, &chantier.Chantier{Key: "YAK-700", Phase: chantier.Shipped,
		PRs: []tracker.PullRequest{{Number: 3300, State: "merged", URL: "u700"}}})
	m.all = m.chantiers
	m.exportBoard()
	data, err := os.ReadFile(BoardFile())
	if err != nil {
		t.Fatal(err)
	}
	var b exportBoard
	if err := json.Unmarshal(data, &b); err != nil {
		t.Fatal(err)
	}
	if b.Shipped != 1 || len(b.Chantiers) != 5 {
		t.Fatalf("shipped chantiers are counted, not listed: %d listed, %d shipped", len(b.Chantiers), b.Shipped)
	}
	by := map[string]exportChantier{}
	for _, c := range b.Chantiers {
		by[c.Key] = c
	}
	if c := by["YAK-861"]; c.Group != "wait" || c.Waiting != "Valider le plan" || c.Tasks != 2 || len(c.Agents) != 1 || c.Agents[0].Pane != "p-yak-861" {
		t.Errorf("YAK-861: %+v", c)
	}
	if c := by["YAK-788"]; !c.CIFailed || c.Group != "wait" || c.PR == nil || c.PR.Number != 3323 || c.PR.Failed != 2 {
		t.Errorf("YAK-788: %+v", c)
	}
	if c := by["YAK-874"]; c.Group != "toplan" || c.PhaseID != "toplan" {
		t.Errorf("YAK-874: %+v", c)
	}
	if b.Groups["wait"] == "" {
		t.Error("section titles are written")
	}

	before, _ := os.Stat(BoardFile())
	m.exportBoard()
	if after, _ := os.Stat(BoardFile()); !after.ModTime().Equal(before.ModTime()) {
		t.Error("an unchanged board is not written again")
	}
}
