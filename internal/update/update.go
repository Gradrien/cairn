// Package update tells whether this cairn is behind main on GitHub, and
// brings it up to date from the clone it was installed from.
//
// A build knows its own commit: install.sh stamps it (go build's own stamp
// counts untracked files as edits). Main is asked through the gh CLI, at most
// once an hour, and only for a clean build of a pushed commit; anything else
// (go run, local edits, a commit main does not know) is never offered an
// update. Nothing installs without the human pressing the button.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"cairn/internal/i18n"
)

// Repo is where main lives.
const Repo = "Gradrien/cairn"

// Every is how long "later" lasts.
const Every = 24 * time.Hour

// Fresh is how long a check is trusted: main may move on any time, and
// asking costs one gh call.
const Fresh = time.Hour

// Change is one commit main has that this build lacks.
type Change struct {
	Subject string    `json:"subject"`
	Body    string    `json:"body,omitempty"` // the message after its subject
	Date    time.Time `json:"date,omitzero"`
}

// Status is what a check found, kept in the state file between launches.
type Status struct {
	Rev       string    `json:"rev"` // the build it was for
	Checked   time.Time `json:"checked"`
	Behind    int       `json:"behind"`            // commits on main this build lacks
	Changes   []Change  `json:"changes,omitempty"` // newest first
	Dismissed time.Time `json:"dismissed,omitzero"`
}

// Offer tells whether to propose the update now: there is one, and it was
// not put off in the last day.
func (s Status) Offer(now time.Time) bool {
	return s.Behind > 0 && now.Sub(s.Dismissed) >= Every
}

// Build is the commit this binary was built from, and whether the checkout
// had uncommitted changes. ok is false without that stamp (go run, go test).
func Build() (rev string, modified, ok bool) {
	info, found := debug.ReadBuildInfo()
	if !found {
		return "", false, false
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	return rev, modified, rev != ""
}

// Compare lists the commits main has that rev lacks, newest first. ok is
// false when rev is not an ancestor of main (unpushed, or diverged).
type Compare func(ctx context.Context, rev string) (changes []Change, ok bool, err error)

// Check returns the status of build rev, from the state file when it was
// checked less than maxAge ago, else from compare, recorded for next time.
func Check(ctx context.Context, file, rev string, now time.Time, maxAge time.Duration, compare Compare) (Status, error) {
	old, _ := Load(file)
	if old.Rev == rev && now.Sub(old.Checked) < maxAge {
		return old, nil
	}
	changes, ok, err := compare(ctx, rev)
	if err != nil {
		return Status{}, err
	}
	st := Status{Rev: rev, Checked: now, Dismissed: old.Dismissed}
	if ok {
		st.Behind, st.Changes = len(changes), changes
	}
	return st, Save(file, st)
}

// Load reads the state file; a missing or unreadable one is a zero Status.
func Load(file string) (Status, error) {
	var st Status
	raw, err := os.ReadFile(file)
	if err != nil {
		return st, err
	}
	return st, json.Unmarshal(raw, &st)
}

func Save(file string, st Status) error {
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(file, append(raw, '\n'), 0o644)
}

// Dismiss records that the update was put off, so it is not proposed again
// for a day.
func Dismiss(file string, now time.Time) error {
	st, _ := Load(file)
	st.Dismissed = now
	return Save(file, st)
}

// GitHub compares rev with main through gh.
func GitHub(ctx context.Context, rev string) ([]Change, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gh", "api", "repos/"+Repo+"/compare/"+rev+"...main").Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && strings.Contains(string(ee.Stderr), "404") {
			return nil, false, nil // a commit main has never seen
		}
		return nil, false, err
	}
	return parseCompare(out)
}

// parseCompare reads GitHub's comparison of rev...main. "ahead" means main
// has commits on top of rev, and nothing else.
func parseCompare(raw []byte) ([]Change, bool, error) {
	var c struct {
		Status  string `json:"status"`
		Commits []struct {
			Commit struct {
				Message string `json:"message"`
				Author  struct {
					Date time.Time `json:"date"`
				} `json:"author"`
			} `json:"commit"`
			Parents []struct{} `json:"parents"`
		} `json:"commits"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, false, err
	}
	switch c.Status {
	case "identical":
		return nil, true, nil
	case "ahead":
	default:
		return nil, false, nil
	}
	var changes []Change
	for _, k := range c.Commits {
		if len(k.Parents) > 1 {
			continue // a merge says again what its commits say
		}
		subject, body, _ := strings.Cut(k.Commit.Message, "\n")
		changes = append(changes, Change{Subject: strings.TrimSpace(subject), Body: cleanBody(body), Date: k.Commit.Author.Date})
	}
	slices.Reverse(changes) // GitHub lists them oldest first
	return changes, true, nil
}

// cleanBody drops the trailers (Co-Authored-By…) and the blank lines around.
func cleanBody(body string) string {
	var keep []string
	for _, l := range strings.Split(body, "\n") {
		t := strings.TrimSpace(l)
		if k, _, ok := strings.Cut(t, ": "); ok && !strings.Contains(k, " ") && strings.Contains(k, "-") {
			continue // Co-Authored-By: …, Signed-off-by: …
		}
		keep = append(keep, strings.TrimRight(l, " \t"))
	}
	return strings.TrimSpace(strings.Join(keep, "\n"))
}

// Install brings the clone in dir to the latest main and installs it with
// its install.sh, writing what git and the script say to out. It refuses a
// clone off main or with uncommitted changes, rather than build those.
// install.sh is told which process is updating, so it restarts every other
// cairn but leaves this one to start the new build itself.
func Install(dir string, out io.Writer) error {
	git := func(args ...string) (string, error) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Stderr = out
		b, err := cmd.Output()
		return strings.TrimSpace(string(b)), err
	}
	branch, err := git("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf(i18n.T("%s n'est pas un clone git : %w", "%s is not a git clone: %w"), dir, err)
	}
	if branch != "main" {
		return fmt.Errorf(i18n.T("%s est sur la branche %s, pas main", "%s is on branch %s, not main"), dir, branch)
	}
	if dirty, err := git("status", "--porcelain", "--untracked-files=no"); err != nil || dirty != "" {
		return fmt.Errorf(i18n.T("%s a des changements non commités", "%s has uncommitted changes"), dir)
	}
	pull := exec.Command("git", "-C", dir, "pull", "--ff-only", "origin", "main")
	pull.Stdout, pull.Stderr = out, out
	if err := pull.Run(); err != nil {
		return fmt.Errorf("git pull : %w", err)
	}
	install := exec.Command("bash", filepath.Join(dir, "install.sh"))
	install.Dir = dir
	install.Stdin = nil // no first-install questions
	install.Env = append(os.Environ(), fmt.Sprintf("CAIRN_UPDATING_PID=%d", os.Getpid()))
	install.Stdout, install.Stderr = out, out
	if err := install.Run(); err != nil {
		return fmt.Errorf("install.sh : %w", err)
	}
	return nil
}
