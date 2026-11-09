package guard

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"cairn/internal/prompted"
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

// fixture is a chantier repo on branch adrien/yak-1-x with one commit; write
// sets its plan's push and pr lines; check runs a command through the guard.
type fixture struct {
	t           *testing.T
	repo, plans string
	plan        string
	pr          *bool
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{t: t, repo: t.TempDir(), plans: t.TempDir(), pr: new(bool)}
	f.git("init", "-q", "-b", "adrien/yak-1-x")
	f.git("commit", "-q", "--allow-empty", "-m", "one")
	f.plan = state.PathFor(f.plans, "YAK-1")
	OpenPR = func(string) bool { return *f.pr }
	t.Cleanup(func() { OpenPR = func(string) bool { return false } })
	return f
}

func (f *fixture) git(args ...string) string {
	out, err := exec.Command("git", append([]string{"-C", f.repo, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...).CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v: %s", args, out)
	}
	return strings.TrimSpace(string(out))
}

func (f *fixture) write(front string) {
	if err := os.WriteFile(f.plan, []byte("---\nticket: YAK-1\nphase: build\n"+front+"---\n"), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) check(cmd string) string {
	payload, _ := json.Marshal(map[string]any{"tool_name": "Bash", "cwd": f.repo, "tool_input": map[string]string{"command": cmd}})
	return Check(bytes.NewReader(payload), []string{"YAK", "EIDO"}, f.plans)
}

func TestAllowanceCoversACommit(t *testing.T) {
	f := newFixture(t)
	head := f.git("rev-parse", "HEAD")

	f.write("")
	if f.check("git push") == "" {
		t.Fatal("no allowance: blocked")
	}
	f.write("push: autorisé " + head + "\n")
	for _, cmd := range []string{"git push -u origin HEAD", "git push"} {
		if reason := f.check(cmd); reason != "" {
			t.Fatalf("%q on the allowed commit must pass, got %q", cmd, reason)
		}
	}
	if reason := f.check("gh pr create --fill"); !strings.Contains(reason, "Prêt à ouvrir la PR") {
		t.Fatalf("allowing the push does not open the PR, got %q", reason)
	}
	f.git("commit", "-q", "--allow-empty", "-m", "two")
	if reason := f.check("git push"); !strings.Contains(reason, "nouveaux commits") {
		t.Fatalf("a new commit needs a new allowance, got %q", reason)
	}
	// Once the PR is open, fixes push freely; merging is never the agent's.
	*f.pr = true
	if reason := f.check("git push"); reason != "" {
		t.Fatalf("an open PR lets pushes through, got %q", reason)
	}
	if f.check("gh pr merge --squash") == "" {
		t.Fatal("merging stays blocked")
	}
	*f.pr = false
	// An allowance with no commit (asked in the chat) is pinned on first use.
	f.write("push: autorisé\n")
	if f.check("git push") != "" {
		t.Fatal("an unpinned allowance passes once pinned")
	}
	if s, _ := state.Load(f.plan); s.PushCommit() != f.git("rev-parse", "HEAD") {
		t.Fatalf("pinned to %q", s.PushCommit())
	}
}

func TestPRIsAllowedApart(t *testing.T) {
	f := newFixture(t)
	f.write("pr: autorisée\n")
	if reason := f.check("gh pr create --fill"); reason != "" {
		t.Fatalf("an allowed PR opens, got %q", reason)
	}
	if f.check("git push") == "" {
		t.Fatal("allowing the PR alone does not allow an unreviewed push")
	}
	f.write("push: autorisé\npr: autorisée\n")
	if f.check("gh pr merge 3") == "" {
		t.Fatal("merging stays blocked even with every allowance")
	}
}

func prompt(f *fixture, text string, cairnDir string) string {
	payload, _ := json.Marshal(map[string]any{"hook_event_name": "UserPromptSubmit", "cwd": f.repo, "prompt": text})
	return CheckPrompt(bytes.NewReader(payload), []string{"YAK"}, f.plans, cairnDir)
}

func TestPushAskedInTheChat(t *testing.T) {
	f := newFixture(t)
	cairnDir := t.TempDir()
	f.write("attente: Prêt à pousser — le correctif\n")
	if note := prompt(f, "ok c'est bon, push moi ça", cairnDir); !strings.Contains(note, "autorisé") || !strings.Contains(note, "ne l'ouvre pas") {
		t.Fatalf("a push request allows it, got %q", note)
	}
	s, _ := state.Load(f.plan)
	if !s.PushAllowed() || s.PushCommit() != "" || s.Waiting != "" {
		t.Fatalf("allowed, unpinned, no longer waiting: push=%q attente=%q", s.Push, s.Waiting)
	}
	f.git("commit", "-q", "--allow-empty", "-m", "committed after the request")
	if reason := f.check("git push"); reason != "" {
		t.Fatalf("commit then push passes, got %q", reason)
	}
	if f.check("gh pr create") == "" {
		t.Fatal("a chat push does not open the PR")
	}

	f.write("")
	for _, text := range []string{"ne push pas encore", "avant de push, relance les tests", "tu as déjà push ?", "Do not push yet", "relis le pushover"} {
		if note := prompt(f, text, cairnDir); note != "" {
			t.Errorf("%q is not a push request, got %q", text, note)
		}
	}
	prompted.Record(cairnDir, "Push autorisé dans cairn : pousse maintenant.")
	if note := prompt(f, "Push autorisé dans cairn : pousse maintenant.", cairnDir); note != "" {
		t.Errorf("cairn's own prompt is not the human's, got %q", note)
	}
	if s, _ := state.Load(f.plan); s.PushAllowed() {
		t.Fatal("nothing was allowed")
	}
}

func TestAsksPush(t *testing.T) {
	for text, want := range map[string]bool{
		"push":                        true,
		"pousse la branche":           true,
		"commit et push":              true,
		"tu peux push stp":            true,
		"Push it.":                    true,
		"ne pousse rien":              false,
		"push pas tout de suite":      false,
		"sans push":                   false,
		"don't push":                  false,
		"le bouton push est cassé":    true, // a known false positive: it only lets a branch leave, never a PR
		"j'ai poussé un fix sur main": false,
		"pushes are blocked, why?":    false,
	} {
		if got := asksPush(text); got != want {
			t.Errorf("%q: got %v want %v", text, got, want)
		}
	}
}
