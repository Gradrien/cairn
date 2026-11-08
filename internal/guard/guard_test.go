package guard

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"cairn/internal/state"
)

func TestPatterns(t *testing.T) {
	for cmd, want := range map[string]bool{
		"git push":                             true,
		"git push -u origin HEAD":              true,
		"cd /x && git push --force-with-lease": true,
		"git -C /x push":                       true,
		"gh pr create --fill":                  true,
		"gh pr ready 12":                       true,
		"git status && git log":                false,
		"echo 'do not git pushover'":           false,
		"gh pr view 12":                        false,
		"git commit -m 'push button'":          false,
	} {
		got := gitPush.MatchString(cmd) || ghPR.MatchString(cmd)
		if got != want {
			t.Errorf("%q: got %v want %v", cmd, got, want)
		}
	}
}

func TestAllowanceCoversACommit(t *testing.T) {
	repo, plans := t.TempDir(), t.TempDir()
	git := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q", "-b", "adrien/yak-1-x")
	git("commit", "-q", "--allow-empty", "-m", "one")
	head := git("rev-parse", "HEAD")
	plan := state.PathFor(plans, "YAK-1")
	write := func(push string) {
		body := "---\nticket: YAK-1\nphase: build\n"
		if push != "" {
			body += "push: " + push + "\n"
		}
		if err := os.WriteFile(plan, []byte(body+"---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pr := false
	OpenPR = func(string) bool { return pr }
	t.Cleanup(func() { OpenPR = func(string) bool { return false } })
	check := func(cmd string) string {
		payload, _ := json.Marshal(map[string]any{"tool_name": "Bash", "cwd": repo, "tool_input": map[string]string{"command": cmd}})
		return Check(bytes.NewReader(payload), []string{"YAK", "EIDO"}, plans)
	}

	write("")
	if check("git push") == "" {
		t.Fatal("no allowance: blocked")
	}
	write("autorisé " + head)
	for _, cmd := range []string{"git push -u origin HEAD", "gh pr create --fill", "gh pr create --fill", "git push"} {
		if reason := check(cmd); reason != "" {
			t.Fatalf("%q on the allowed commit must pass, got %q", cmd, reason)
		}
	}
	git("commit", "-q", "--allow-empty", "-m", "two")
	if reason := check("git push"); !strings.Contains(reason, "nouveaux commits") {
		t.Fatalf("a new commit needs a new allowance, got %q", reason)
	}
	// Once the PR is open, fixes push freely; merging still needs the human.
	pr = true
	if reason := check("git push"); reason != "" {
		t.Fatalf("an open PR lets pushes through, got %q", reason)
	}
	if check("gh pr merge --squash") == "" {
		t.Fatal("merging an unreviewed commit stays blocked")
	}
	pr = false
	// An allowance from an older cairn, with no commit, is pinned on first use.
	write("autorisé")
	if check("git push") != "" {
		t.Fatal("old allowance passes once pinned")
	}
	if s, _ := state.Load(plan); s.PushCommit() != git("rev-parse", "HEAD") {
		t.Fatalf("pinned to %q", s.PushCommit())
	}
}
