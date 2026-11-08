// Package guard is a Claude Code PreToolUse hook: on a cairn chantier, an
// agent may not push or open a pull request until a human allowed it in cairn.
// The allowance covers the commit the human reviewed: the first push, PR
// creation and retries pass as long as the branch stays on it. Once the PR is
// open, pushes to its branch pass freely: the PR is where the human reviews
// from then on, and nothing merges without them.
package guard

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"cairn/internal/chantier"
	"cairn/internal/i18n"
	"cairn/internal/state"
)

var (
	gitPush  = regexp.MustCompile(`(^|[;&|(\s])git(\s+-C\s+\S+|\s+-c\s+\S+)*\s+push\b`)
	ghPR     = regexp.MustCompile(`(^|[;&|(\s])gh\s+pr\s+(create|ready|merge)\b`)
	ghMerge  = regexp.MustCompile(`(^|[;&|(\s])gh\s+pr\s+merge\b`)
	gitDashC = regexp.MustCompile(`git\s+-C\s+("[^"]+"|'[^']+'|\S+)`)
	cdPath   = regexp.MustCompile(`^\s*cd\s+("[^"]+"|'[^']+'|[^\s;&|]+)`)
)

type input struct {
	ToolName  string `json:"tool_name"`
	Cwd       string `json:"cwd"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
}

// Check reads a hook payload and returns a non-empty reason to block.
// Anything unexpected lets the command through: the guard must never get in
// the way of work that is not a cairn chantier.
func Check(r io.Reader, keys []string, chantiersDir string) string {
	var in input
	if err := json.NewDecoder(r).Decode(&in); err != nil || in.ToolName != "Bash" || len(keys) == 0 {
		return ""
	}
	cmd := in.ToolInput.Command
	if !gitPush.MatchString(cmd) && !ghPR.MatchString(cmd) {
		return ""
	}
	dir := in.Cwd
	if m := cdPath.FindStringSubmatch(cmd); m != nil {
		dir = resolve(dir, m[1])
	}
	if m := gitDashC.FindStringSubmatch(cmd); m != nil {
		dir = resolve(dir, m[1])
	}
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	m := chantier.KeyRe(keys...).FindStringSubmatch(strings.TrimSpace(string(out)))
	if m == nil {
		return ""
	}
	key := strings.ToUpper(m[1])
	s, err := state.Load(state.PathFor(chantiersDir, key))
	if err != nil || s == nil {
		return "" // not a cairn chantier
	}
	head := ""
	if out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output(); err == nil {
		head = strings.TrimSpace(string(out))
	}
	if s.PushCovers(head) && s.PushCommit() != "" {
		return ""
	}
	// An open PR is reviewed on GitHub: fixes after review or CI push freely.
	// Merging stays the human's call.
	if !ghMerge.MatchString(cmd) && OpenPR(dir) {
		return ""
	}
	if s.PushAllowed() {
		if s.PushCommit() == "" && head != "" {
			// Allowed without a commit (older cairn): pin it to this one.
			s.Set("push", "autorisé "+head)
			_ = s.Save()
		}
		if s.PushCovers(head) {
			return ""
		}
		return fmt.Sprintf(i18n.T("cairn : %s a de nouveaux commits depuis l'autorisation (accordée pour %s). "+
			"Ne contourne pas ce blocage. Mets `attente: Prêt à pousser` dans %s avec en une ligne ce qui partira, "+
			"lance `cairn sync`, puis arrête-toi : l'humain relit et autorise ce nouveau commit dans cairn.",
			"cairn: %s has new commits since the approval (given for %s). "+
				"Do not work around this block. Set `attente: Prêt à pousser` in %s with one line on what will ship, "+
				"run `cairn sync`, then stop: the human reviews and allows this new commit in cairn."), key, short(s.PushCommit()), s.Path)
	}
	return fmt.Sprintf(i18n.T("cairn : %s attend une validation humaine avant tout push ou PR. "+
		"Ne contourne pas ce blocage. Termine ce que tu fais, mets `attente: Prêt à pousser` dans %s"+
		" avec en une ligne ce qui partira, lance `cairn sync`, puis arrête-toi : l'humain relit dans cairn et autorise le push.",
		"cairn: %s needs a human approval before any push or PR. "+
			"Do not work around this block. Finish what you are doing, set `attente: Prêt à pousser` in %s"+
			" with one line on what will ship, run `cairn sync`, then stop: the human reviews in cairn and allows the push."), key, s.Path)
}

// OpenPR reports whether the branch checked out in dir has an open pull
// request. A variable so tests can stub gh.
var OpenPR = func(dir string) bool {
	cmd := exec.Command("gh", "pr", "view", "--json", "state", "-q", ".state")
	cmd.Dir = dir
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "OPEN"
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func resolve(base, p string) string {
	p = strings.Trim(p, `"'`)
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}
