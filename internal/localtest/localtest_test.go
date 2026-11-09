package localtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, _ := os.ReadFile(path)
	return string(b)
}

// A main checkout on its own ticket branch, with work in progress and an
// ignored .env, and a worktree where an agent builds another ticket.
func TestTestHereAndBack(t *testing.T) {
	root := t.TempDir()
	repo, wt := filepath.Join(root, "app"), filepath.Join(root, "app-800")
	run(t, root, "init", "-q", "-b", "main", repo)
	write(t, filepath.Join(repo, ".gitignore"), ".env\n")
	write(t, filepath.Join(repo, "app.txt"), "v1\n")
	run(t, repo, "add", ".")
	run(t, repo, "commit", "-qm", "init")
	run(t, repo, "worktree", "add", "-q", "-b", "yak-800", wt)
	write(t, filepath.Join(wt, "app.txt"), "v800\n")
	run(t, wt, "commit", "-qam", "800")
	run(t, repo, "checkout", "-q", "-b", "yak-861")
	write(t, filepath.Join(repo, ".env"), "SECRET=1\n")
	write(t, filepath.Join(repo, "app.txt"), "wip 861\n")

	p, err := Prepare(repo, "yak-800")
	if err != nil || p.From != "yak-861" || p.Dirty != 1 || p.Commits != 1 {
		t.Fatalf("prepare: %+v %v", p, err)
	}
	s := Store{Path: filepath.Join(root, "cache", "localtest.json")}
	if err := s.Start(repo, "YAK-800", "yak-800"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(repo, "app.txt")); got != "v800\n" {
		t.Fatalf("the main checkout should show the branch, got %q", got)
	}
	if read(t, filepath.Join(repo, ".env")) != "SECRET=1\n" {
		t.Fatal("the env should stay in place")
	}
	st, ok := s.Get(repo)
	if !ok || st.Key != "YAK-800" || st.Behind() {
		t.Fatalf("status: %+v %v", st, ok)
	}

	// The agent commits again: the test lags, Refresh catches up.
	write(t, filepath.Join(wt, "app.txt"), "v800 bis\n")
	run(t, wt, "commit", "-qam", "800 bis")
	if st, _ = s.Get(repo); !st.Behind() {
		t.Fatal("the test should lag behind the branch")
	}
	if err := s.Refresh(repo); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(repo, "app.txt")); got != "v800 bis\n" {
		t.Fatalf("refresh should bring the last commit, got %q", got)
	}

	// Changes made during the test block the way back.
	write(t, filepath.Join(repo, "app.txt"), "poked\n")
	if err := s.Stop(repo); err == nil {
		t.Fatal("stop should refuse changes made during the test")
	}
	run(t, repo, "checkout", "--", "app.txt")

	if err := s.Stop(repo); err != nil {
		t.Fatal(err)
	}
	if b := run(t, repo, "branch", "--show-current"); b != "yak-861" {
		t.Fatalf("back on %q, want yak-861", b)
	}
	if got := read(t, filepath.Join(repo, "app.txt")); got != "wip 861\n" {
		t.Fatalf("the work in progress should be restored, got %q", got)
	}
	if _, ok := s.Get(repo); ok {
		t.Fatal("no test should be left")
	}
}

func TestMovedByHandIsNoTest(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "app")
	run(t, root, "init", "-q", "-b", "main", repo)
	write(t, filepath.Join(repo, "a"), "1")
	run(t, repo, "add", ".")
	run(t, repo, "commit", "-qm", "init")
	run(t, repo, "branch", "yak-1")
	s := Store{Path: filepath.Join(root, "t.json")}
	if err := s.Start(repo, "YAK-1", "yak-1"); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "checkout", "-q", "main")
	if _, ok := s.Get(repo); ok {
		t.Fatal("a checkout moved back to a branch by hand is no longer under test")
	}
}
