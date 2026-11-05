package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"cairn/internal/chantier"
	"cairn/internal/config"
	"cairn/internal/progress"
	"cairn/internal/state"
)

func TestUnpushed(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q")
	run("-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "x")
	head := run("rev-parse", "HEAD")
	if !unpushed(dir, "") {
		t.Fatal("a commit on no remote counts as unpushed")
	}
	if unpushed(dir, head) {
		t.Fatal("HEAD is the merged PR's last commit: nothing is lost")
	}
}

// A shipped chantier the list hides is still cleaned up; one that would lose
// work is kept, until you close it by hand.
func TestCloseShipped(t *testing.T) {
	t.Setenv("CAIRN_HOME", t.TempDir())
	_ = os.MkdirAll(config.ChantiersDir(), 0o755)
	repo := t.TempDir()
	wt := filepath.Join(t.TempDir(), "wt")
	git := func(dir string, args ...string) {
		if out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	git(repo, "init", "-q")
	git(repo, "commit", "-q", "--allow-empty", "-m", "x")
	git(repo, "worktree", "add", "-q", "-b", "yak-9", wt)
	_ = os.WriteFile(filepath.Join(wt, "wip.txt"), []byte("wip"), 0o644)
	plan := state.PathFor(config.ChantiersDir(), "YAK-9")
	_ = os.WriteFile(plan, []byte("---\nphase: review\n---\n"), 0o644)

	c := &chantier.Chantier{Key: "YAK-9", Phase: chantier.Shipped, Worktree: wt, Branch: "yak-9"}
	m := Model{watch: newWatch(), inside: true, prs: map[string]progress.PR{}, all: []*chantier.Chantier{c}}
	m.watch.ready = true
	if m.observe() == nil || m.watch.cleanedAt["YAK-9"].IsZero() {
		t.Fatal("a shipped chantier hidden from the list must be cleaned up")
	}
	if msg := m.cleanup(c, false)().(cleanedMsg); msg.keep == "" {
		t.Fatalf("uncommitted work must keep the chantier, got %+v", msg)
	}
	if _, err := os.Stat(wt); err != nil {
		t.Fatal("kept worktree was removed")
	}
	if msg := m.cleanup(c, true)().(cleanedMsg); msg.keep != "" || msg.err != nil {
		t.Fatalf("closing by hand: %+v", msg)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatal("the worktree must be gone")
	}
	if _, err := os.Stat(plan); !os.IsNotExist(err) {
		t.Fatal("the plan must be archived")
	}
}
