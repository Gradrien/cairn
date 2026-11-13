package chantier

import (
	"os"
	"path/filepath"
	"testing"

	"cairn/internal/mux"
	"cairn/internal/tracker"
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
		ChantiersDir: dir,
		Sources: []Source{{Project: "yak", Key: "YAK", Repo: "/r/Yak-App",
			Issues: []tracker.Issue{
				{ID: "YAK-788", Title: "modèles"},
				{ID: "YAK-861", Title: "504", PullRequests: []tracker.PullRequest{{Number: 1, State: "open"}}},
				{ID: "YAK-900", Title: "nouveau"},
				{ID: "YAK-770", Title: "CRA"},
			},
			Worktrees: []mux.Worktree{
				{Branch: "adrien/yak-770-cra", Path: "/r/Yak-App"},
				{Branch: "adrien/yak-788-modeles", Path: "/r/Yak-App-788", OpenWorkspaceID: "w10"},
			},
		}},
		Snapshot: mux.Snapshot{Agents: []mux.Agent{
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

// A tracker can still say "open" for a PR GitHub has merged: GitHub wins.
func TestGitHubStateWins(t *testing.T) {
	is := tracker.Issue{ID: "YAK-1", PullRequests: []tracker.PullRequest{{URL: "u", State: "open"}}}
	list, _ := Gather(Input{Sources: []Source{{Project: "yak", Key: "YAK", Issues: []tracker.Issue{is}}}, GitHubPRs: map[string]string{"u": "MERGED"}})
	if len(list) != 1 || list[0].Phase != Shipped || list[0].OpenPR() != nil {
		t.Fatalf("got %+v", list)
	}
	if is.PullRequests[0].State != "open" {
		t.Fatal("the input issue was modified")
	}
}

// Two projects side by side, one without a tracker: its chantier comes from
// its worktree, its PR from GitHub by branch, its title from its file.
func TestTwoProjects(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "EIDO-3.md"), []byte("---\nticket: EIDO-3\nphase: build\n---\n# EIDO-3 · Export PDF\n"), 0o644)
	list, _ := Gather(Input{ChantiersDir: dir, Sources: []Source{
		{Project: "yak", Key: "YAK", Repo: "/r/Yak-App", Issues: []tracker.Issue{{ID: "YAK-1", Title: "a"}},
			Worktrees: []mux.Worktree{{Branch: "adrien/eido-3-export", Path: "/r/Yak-App-3"}}},
		{Project: "eido", Key: "EIDO", Repo: "/r/eido",
			Worktrees: []mux.Worktree{{Branch: "adrien/eido-3-export", Path: "/r/eido-3"}},
			BranchPRs: map[string][]tracker.PullRequest{"adrien/eido-3-export": {{URL: "new", State: "open"}, {URL: "old", State: "closed"}}}},
	}})
	by := map[string]*Chantier{}
	for _, c := range list {
		by[c.Key] = c
	}
	c := by["EIDO-3"]
	if len(list) != 2 || c == nil || c.Project != "eido" || c.Worktree != "/r/eido-3" || c.Title() != "Export PDF" {
		t.Fatalf("got %+v", c)
	}
	if c.Phase != Build || c.OpenPR() == nil || c.OpenPR().URL != "new" || c.PRs[0].URL != "old" {
		t.Fatalf("PRs: %+v phase %v", c.PRs, c.Phase)
	}
	if !KeyRe("YAK", "EIDO").MatchString("x/eido-3") || KeyRe("YAK").MatchString("x/eido-3") {
		t.Fatal("KeyRe prefixes")
	}
}
