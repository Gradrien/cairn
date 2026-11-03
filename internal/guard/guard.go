// Package guard is a Claude Code PreToolUse hook: on a cairn chantier, an
// agent may not push or open a pull request until a human allowed it in cairn.
// Each allowance covers one push, so unfinished work never reaches the PR.
package guard

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"cairn/internal/chantier"
	"cairn/internal/state"
)

var (
	gitPush  = regexp.MustCompile(`(^|[;&|(\s])git(\s+-C\s+\S+|\s+-c\s+\S+)*\s+push\b`)
	ghPR     = regexp.MustCompile(`(^|[;&|(\s])gh\s+pr\s+(create|ready|merge)\b`)
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
func Check(r io.Reader, team, chantiersDir string) string {
	var in input
	if err := json.NewDecoder(r).Decode(&in); err != nil || in.ToolName != "Bash" {
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
	m := chantier.KeyRe(team).FindStringSubmatch(strings.TrimSpace(string(out)))
	if m == nil {
		return ""
	}
	key := strings.ToUpper(m[1])
	s, err := state.Load(state.PathFor(chantiersDir, key))
	if err != nil || s == nil {
		return "" // not a cairn chantier
	}
	if s.PushAllowed() {
		s.Set("push", "") // one allowance, one push
		_ = s.Save()
		return ""
	}
	return "cairn : " + key + " attend une validation humaine avant tout push ou PR. " +
		"Ne contourne pas ce blocage. Termine ce que tu fais, mets `attente: Prêt à pousser` dans " + s.Path +
		" avec en une ligne ce qui partira, lance `cairn sync`, puis arrête-toi : l'humain relit dans cairn et autorise le push."
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
