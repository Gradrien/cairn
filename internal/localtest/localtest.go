// Package localtest puts a chantier's branch in the project's main checkout,
// where the env, secrets, installed packages and running stack already are,
// so the human can try it there; then puts the checkout back as it was.
//
// git will not check out a branch twice, and the chantier's worktree has it:
// the main checkout goes to the branch's last commit, detached. Its own
// uncommitted changes are stashed for the test and restored after. Only
// committed work is tried: the worktree's uncommitted changes stay there.
package localtest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"cairn/internal/i18n"
)

// Session is a test under way in a main checkout.
type Session struct {
	Key    string `json:"key"`             // the chantier tried
	Branch string `json:"branch"`          // its branch
	Back   string `json:"back"`            // where the checkout was: a branch, or a commit if detached
	Stash  string `json:"stash,omitempty"` // the stash of its uncommitted changes, by commit
}

// Status is a main checkout under test, as it stands now.
type Status struct {
	Session
	Head string // the commit checked out
	Tip  string // the branch's last commit: Head lags behind when the agent committed since
}

// Behind tells whether the agent committed since the test began.
func (s Status) Behind() bool { return s.Tip != "" && s.Head != s.Tip }

// Store keeps the sessions by repository, so they survive a restart.
type Store struct{ Path string }

func (s Store) load() map[string]Session {
	out := map[string]Session{}
	if b, err := os.ReadFile(s.Path); err == nil {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

func (s Store) save(all map[string]Session) error {
	b, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.Path, append(b, '\n'), 0o644)
}

// Get returns the test under way in repo, if any. A checkout someone moved
// to a branch by hand is no longer under test.
func (s Store) Get(repo string) (Status, bool) {
	ses, ok := s.load()[repo]
	if !ok {
		return Status{}, false
	}
	if branch, _ := git(repo, "symbolic-ref", "--short", "-q", "HEAD"); branch != "" {
		return Status{}, false
	}
	head, _ := git(repo, "rev-parse", "HEAD")
	tip, _ := git(repo, "rev-parse", "--verify", "-q", "refs/heads/"+ses.Branch)
	return Status{Session: ses, Head: head, Tip: tip}, true
}

// Plan is what starting a test would do, for the human to confirm.
type Plan struct {
	From     string // where the checkout is: its branch, or a short commit
	Dirty    int    // its uncommitted changes, to stash
	Commits  int    // commits of the branch not in From
	DepsDiff bool   // a lockfile differs: packages to install
}

// Prepare says what testing branch in repo would do. An error says why it
// cannot.
func Prepare(repo, branch string) (Plan, error) {
	var p Plan
	if _, err := git(repo, "rev-parse", "--verify", "-q", "refs/heads/"+branch); err != nil {
		return p, fmt.Errorf(i18n.T("la branche %s n'existe pas dans %s", "branch %s does not exist in %s"), branch, filepath.Base(repo))
	}
	for _, busy := range []string{"MERGE_HEAD", "rebase-merge", "rebase-apply", "CHERRY_PICK_HEAD"} {
		path, _ := git(repo, "rev-parse", "--git-path", busy)
		if path != "" && !filepath.IsAbs(path) {
			path = filepath.Join(repo, path)
		}
		if path != "" && fileExists(path) {
			return p, fmt.Errorf(i18n.T("%s est au milieu d'un merge ou d'un rebase", "%s is in the middle of a merge or rebase"), filepath.Base(repo))
		}
	}
	p.From = current(repo)
	dirty, _ := git(repo, "status", "--porcelain", "--untracked-files=no")
	if dirty != "" {
		p.Dirty = len(strings.Split(dirty, "\n"))
	}
	if n, err := git(repo, "rev-list", "--count", "HEAD.."+branch); err == nil {
		fmt.Sscan(n, &p.Commits)
	}
	p.DepsDiff = depsDiffer(repo, "HEAD", branch)
	return p, nil
}

// Start stashes repo's uncommitted changes and checks out branch's last
// commit, detached. A test already under way there keeps where it came from.
func (s Store) Start(repo, key, branch string) error {
	all := s.load()
	prev, testing := all[repo]
	if testing {
		if st, ok := s.Get(repo); !ok {
			testing = false // moved by hand since
		} else {
			prev = st.Session
		}
	}
	ses := Session{Key: key, Branch: branch, Back: current(repo)}
	if branch, _ := git(repo, "symbolic-ref", "--short", "-q", "HEAD"); branch == "" {
		ses.Back, _ = git(repo, "rev-parse", "HEAD")
	}
	if testing {
		ses.Back, ses.Stash = prev.Back, prev.Stash
	}
	if dirty, _ := git(repo, "status", "--porcelain", "--untracked-files=no"); dirty != "" {
		if testing {
			return fmt.Errorf(i18n.T("%s a des changements faits pendant le test : commit, stash ou annule-les d'abord", "%s has changes made during the test: commit, stash or discard them first"), filepath.Base(repo))
		}
		if _, err := git(repo, "stash", "push", "-m", "cairn: test "+key); err != nil {
			return err
		}
		ses.Stash, _ = git(repo, "rev-parse", "stash@{0}")
	}
	// Recorded before the checkout: if it fails half-way, the way back is known.
	all[repo] = ses
	if err := s.save(all); err != nil {
		return err
	}
	if _, err := git(repo, "checkout", "--detach", "refs/heads/"+branch); err != nil {
		if !testing {
			_ = s.Stop(repo)
		}
		return err
	}
	return nil
}

// Refresh moves the test to the branch's last commit.
func (s Store) Refresh(repo string) error {
	st, ok := s.Get(repo)
	if !ok {
		return errors.New(i18n.T("aucun test en cours", "no test under way"))
	}
	return s.Start(repo, st.Key, st.Branch)
}

// Stop puts repo back where it was and restores its stashed changes.
func (s Store) Stop(repo string) error {
	all := s.load()
	ses, ok := all[repo]
	if !ok {
		return nil
	}
	if dirty, _ := git(repo, "status", "--porcelain", "--untracked-files=no"); dirty != "" {
		return fmt.Errorf(i18n.T("%s a des changements faits pendant le test : commit, stash ou annule-les d'abord", "%s has changes made during the test: commit, stash or discard them first"), filepath.Base(repo))
	}
	if _, err := git(repo, "checkout", ses.Back); err != nil {
		return err
	}
	if ses.Stash != "" {
		ref, err := stashRef(repo, ses.Stash)
		if err != nil {
			return err
		}
		if _, err := git(repo, "stash", "pop", "--index", ref); err != nil {
			return fmt.Errorf(i18n.T("retour sur %s, mais tes changements n'ont pas pu être restaurés (%v) : ils sont dans git stash list", "back on %s, but your changes could not be restored (%v): they are in git stash list"), ses.Back, err)
		}
	}
	delete(all, repo)
	return s.save(all)
}

func stashRef(repo, sha string) (string, error) {
	list, _ := git(repo, "stash", "list", "--format=%H")
	for i, l := range strings.Split(list, "\n") {
		if l == sha {
			return fmt.Sprintf("stash@{%d}", i), nil
		}
	}
	return "", fmt.Errorf(i18n.T("le stash de tes changements (%s) a disparu", "the stash of your changes (%s) is gone"), short(sha))
}

// current is the checkout's branch, or its short commit when detached.
func current(repo string) string {
	if b, _ := git(repo, "symbolic-ref", "--short", "-q", "HEAD"); b != "" {
		return b
	}
	h, _ := git(repo, "rev-parse", "--short", "HEAD")
	return h
}

var lockfiles = []string{"pnpm-lock.yaml", "package-lock.json", "yarn.lock", "bun.lockb", "bun.lock", "go.sum", "Gemfile.lock", "poetry.lock", "uv.lock", "Cargo.lock", "composer.lock"}

func depsDiffer(repo, a, b string) bool {
	args := []string{"diff", "--quiet", a, b, "--"}
	for _, f := range lockfiles {
		args = append(args, ":(glob)**/"+f)
	}
	_, err := git(repo, args...)
	return err != nil
}

// DepsChanged tells whether the lockfiles differ between two commits.
func DepsChanged(repo, a, b string) bool {
	return a != "" && b != "" && a != b && depsDiffer(repo, a, b)
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func git(repo string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("git %s : %s", args[0], strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
