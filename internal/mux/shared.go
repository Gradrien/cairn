package mux

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"cairn/internal/config"
	"cairn/internal/i18n"
)

// Links remember which worktree each workspace cairn made is for, for the
// multiplexers that know nothing of git (Rex, the Herdr app).
func linksFile(name string) string { return filepath.Join(config.Dir(), "cache", name) }

func readLinks(name string) map[string]string {
	out := map[string]string{}
	if data, err := os.ReadFile(linksFile(name)); err == nil {
		_ = json.Unmarshal(data, &out)
	}
	return out
}

func writeLinks(name string, l map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(linksFile(name)), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(l, "", "  ")
	return os.WriteFile(linksFile(name), data, 0o644)
}

// liveLinks returns the links of the workspaces still open.
func liveLinks(name string, live map[string]bool) map[string]string {
	out := map[string]string{}
	for id, p := range readLinks(name) {
		if live[id] {
			out[id] = p
		}
	}
	return out
}

// gitWorktrees lists the repository's checkouts, the main one first.
func gitWorktrees(repo string) ([]Worktree, error) {
	cmd := exec.Command("git", "-C", repo, "worktree", "list", "--porcelain")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list (%s): %s", repo, strings.TrimSpace(stderr.String()+" "+err.Error()))
	}
	var wts []Worktree
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "worktree "):
			wts = append(wts, Worktree{Path: strings.TrimPrefix(line, "worktree "), IsLinked: len(wts) > 0})
		case strings.HasPrefix(line, "branch ") && len(wts) > 0:
			wts[len(wts)-1].Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		}
	}
	return wts, nil
}

// removeGitWorktree removes a linked worktree; it refuses the main checkout.
func removeGitWorktree(path string, force bool) error {
	dirs, err := exec.Command("git", "-C", path, "rev-parse", "--absolute-git-dir", "--git-common-dir").Output()
	if err != nil {
		return fmt.Errorf("git rev-parse (%s): %w", path, err)
	}
	if d := strings.Fields(string(dirs)); len(d) == 2 && filepath.Clean(d[0]) == filepath.Clean(absTo(path, d[1])) {
		return fmt.Errorf(i18n.T("%s est le dépôt principal, pas un worktree : il reste", "%s is the main checkout, not a worktree: it stays"), path)
	}
	args := []string{"-C", path, "worktree", "remove", path}
	if force {
		args = append(args, "--force")
	}
	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func within(dir, root string) bool {
	if dir == "" {
		return false
	}
	dir, root = filepath.Clean(dir), filepath.Clean(root)
	return dir == root || strings.HasPrefix(dir, root+string(filepath.Separator))
}

func absTo(base, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

// systemNotify shows a macOS notification; elsewhere it does nothing.
func systemNotify(title, body string) error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	q := func(s string) string { return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"` }
	return exec.Command("osascript", "-e", "display notification "+q(body)+" with title "+q(title)).Run()
}
