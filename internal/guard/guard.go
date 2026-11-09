// Package guard keeps the code of a cairn chantier from leaving without a
// human. As a Claude Code PreToolUse hook (Check), it blocks:
//   - git push, until a human allowed it: in cairn, or by asking for it in the
//     chat (CheckPrompt). The allowance covers one commit: the push and its
//     retries pass while the branch stays on it. Once the PR is open, pushes
//     pass freely: the PR is where the human reviews from then on;
//   - gh pr create / ready, until a human allowed opening the PR in cairn:
//     pushing no longer opens it;
//   - gh pr merge, always: merging is the human's.
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

// chantierAt finds the chantier whose branch is checked out in dir: its
// key, its plan and the commit at HEAD. A nil plan means not a chantier.
func chantierAt(dir string, keys []string, chantiersDir string) (string, *state.State, string) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil || len(keys) == 0 {
		return "", nil, ""
	}
	m := chantier.KeyRe(keys...).FindStringSubmatch(strings.TrimSpace(string(out)))
	if m == nil {
		return "", nil, ""
	}
	key := strings.ToUpper(m[1])
	s, err := state.Load(state.PathFor(chantiersDir, key))
	if err != nil || s == nil {
		return "", nil, ""
	}
	head := ""
	if out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output(); err == nil {
		head = strings.TrimSpace(string(out))
	}
	return key, s, head
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
	push, pr := gitPush.MatchString(cmd), ghPR.MatchString(cmd)
	if !push && !pr {
		return ""
	}
	dir := in.Cwd
	if m := cdPath.FindStringSubmatch(cmd); m != nil {
		dir = resolve(dir, m[1])
	}
	if m := gitDashC.FindStringSubmatch(cmd); m != nil {
		dir = resolve(dir, m[1])
	}
	key, s, head := chantierAt(dir, keys, chantiersDir)
	if s == nil {
		return "" // not a cairn chantier
	}
	if ghMerge.MatchString(cmd) {
		return fmt.Sprintf(i18n.T("cairn : le merge de %s revient à l'humain. Ne merge pas et ne contourne pas ce blocage : dis que la PR est prête et arrête-toi.",
			"cairn: merging %s is the human's call. Do not merge and do not work around this block: say the PR is ready and stop."), key)
	}
	if pr && !s.PRAllowed() && !OpenPR(dir) {
		return fmt.Sprintf(i18n.T("cairn : l'ouverture de la PR de %s attend l'humain, à part du push. "+
			"Ne contourne pas ce blocage. Mets `attente: Prêt à ouvrir la PR` dans %s avec en une ligne ce qu'elle contient, "+
			"lance `cairn sync`, puis arrête-toi : l'humain l'autorise dans cairn (bouton « Ouvrir la PR »).",
			"cairn: opening %s's PR waits for the human, apart from pushing. "+
				"Do not work around this block. Set `attente: Prêt à ouvrir la PR` in %s with one line on what it holds, "+
				"run `cairn sync`, then stop: the human allows it in cairn (\"Open the PR\" button)."), key, s.Path)
	}
	if !push {
		return ""
	}
	// An open PR is reviewed on GitHub: fixes after review or CI push freely.
	if s.PushCovers(head) && s.PushCommit() != "" || OpenPR(dir) {
		return ""
	}
	if s.PushAllowed() {
		if s.PushCommit() == "" && head != "" {
			// Allowed without a commit (asked in the chat): pin it to this one.
			s.Set("push", "autorisé "+head)
			_ = s.Save()
		}
		if s.PushCovers(head) {
			return ""
		}
		return fmt.Sprintf(i18n.T("cairn : %s a de nouveaux commits depuis l'autorisation (accordée pour %s). "+
			"Ne contourne pas ce blocage. Mets `attente: Prêt à pousser` dans %s avec en une ligne ce qui partira, "+
			"lance `cairn sync`, puis arrête-toi : l'humain autorise ce nouveau commit dans cairn, ou te demande de pousser dans le chat.",
			"cairn: %s has new commits since the approval (given for %s). "+
				"Do not work around this block. Set `attente: Prêt à pousser` in %s with one line on what will ship, "+
				"run `cairn sync`, then stop: the human allows this new commit in cairn, or asks you to push in the chat."), key, short(s.PushCommit()), s.Path)
	}
	return fmt.Sprintf(i18n.T("cairn : %s attend une validation humaine avant tout push. "+
		"Ne contourne pas ce blocage. Termine ce que tu fais, mets `attente: Prêt à pousser` dans %s"+
		" avec en une ligne ce qui partira, lance `cairn sync`, puis arrête-toi : l'humain autorise le push dans cairn, ou te demande de pousser dans le chat.",
		"cairn: %s needs a human approval before any push. "+
			"Do not work around this block. Finish what you are doing, set `attente: Prêt à pousser` in %s"+
			" with one line on what will ship, run `cairn sync`, then stop: the human allows the push in cairn, or asks you to push in the chat."), key, s.Path)
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
