package chantier

import (
	"os"
	"path/filepath"
	"testing"

	"cairn/internal/herdr"
	"cairn/internal/sprintosaur"
)

func TestSlug(t *testing.T) {
	got := Slug("Supprimer « Définir comme modèle » et passer automatiquement", 50)
	want := "supprimer-definir-comme-modele-et-passer-automatiq"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if Slug("  --Été!! ", 50) != "ete" {
		t.Fatal(Slug("  --Été!! ", 50))
	}
}

func TestKeyRe(t *testing.T) {
	re := KeyRe("YAK")
	for branch, want := range map[string]string{
		"adrien/yak-788-supprimer":       "yak-788",
		"agent/yak-861-push":             "yak-861",
		"YAK-12":                         "YAK-12",
		"adrien/notyak-1-x":              "",
		"paul/scratch-auth-refresh-loop": "",
	} {
		m := re.FindStringSubmatch(branch)
		got := ""
		if m != nil {
			got = m[1]
		}
		if got != want {
			t.Errorf("%s: got %q want %q", branch, got, want)
		}
	}
}

func TestBuild(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "YAK-788.md"), []byte("---\nphase: plan\nattente: Valider le plan\n---\n- [ ] a\n"), 0o644)
	in := Input{
		Team: "YAK", Repo: "/r/Yak-App", ChantiersDir: dir,
		Issues: []sprintosaur.Issue{
			{ID: "YAK-788", Title: "modèles"},
			{ID: "YAK-861", Title: "504", PullRequests: []sprintosaur.PullRequest{{Number: 1, State: "open"}}},
			{ID: "YAK-900", Title: "nouveau"},
			{ID: "YAK-770", Title: "CRA"},
		},
		Worktrees: []herdr.Worktree{
			{Branch: "adrien/yak-770-cra", Path: "/r/Yak-App"},
			{Branch: "adrien/yak-788-modeles", Path: "/r/Yak-App-788", OpenWorkspaceID: "w10"},
		},
		Snapshot: herdr.Snapshot{Agents: []herdr.Agent{
			{Kind: "claude", Status: "idle", Cwd: "/r/Yak-App-788/apps", PaneID: "w10:p1", WorkspaceID: "w10"},
			{Kind: "claude", Status: "working", Cwd: "/r/Yak-App", PaneID: "w1:p1", WorkspaceID: "w1", Title: "Tables activity"},
			{Kind: "claude", Status: "blocked", Cwd: "/r/Yak-App", PaneID: "w2:p1", WorkspaceID: "w2", Title: "PR YAK-861 conformité"},
		}},
	}
	list, others := Gather(in)
	by := map[string]*Chantier{}
	for _, c := range list {
		by[c.Key] = c
	}
	if c := by["YAK-788"]; c.Phase != Plan || len(c.Agents) != 1 || c.Waiting() == "" || c.Workspaces[0] != "w10" {
		t.Fatalf("788: %+v", c)
	}
	if c := by["YAK-861"]; c.Phase != Review || len(c.Agents) != 1 || c.Agents[0].Status != "blocked" {
		t.Fatalf("861: %+v", c)
	}
	if by["YAK-900"].Phase != ToPlan {
		t.Fatalf("900: %v", by["YAK-900"].Phase)
	}
	// The main checkout's branch is a chantier, but agents there are not claimed blindly.
	if c := by["YAK-770"]; !c.Main || len(c.Agents) != 0 {
		t.Fatalf("770: %+v", c)
	}
	if len(others) != 1 || others[0].Title != "Tables activity" {
		t.Fatalf("others: %+v", others)
	}
	// Waiting chantiers first.
	if list[0].Waiting() == "" || list[1].Waiting() == "" {
		t.Fatalf("order: %s %s", list[0].Key, list[1].Key)
	}
}
