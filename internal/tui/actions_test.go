package tui

import (
	"os/exec"
	"strings"
	"testing"
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
