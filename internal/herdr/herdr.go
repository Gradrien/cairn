// Package herdr drives the herdr CLI (socket API) and reads its live state.
package herdr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"cairn/internal/config"
	"cairn/internal/i18n"
	"cairn/internal/prompted"
)

type Agent struct {
	Kind          string `json:"agent"`
	Name          string `json:"name"`
	Status        string `json:"agent_status"`
	Cwd           string `json:"cwd"`
	ForegroundCwd string `json:"foreground_cwd"`
	PaneID        string `json:"pane_id"`
	WorkspaceID   string `json:"workspace_id"`
	Title         string `json:"terminal_title_stripped"`
}

// Dir is where the agent is really working.
func (a Agent) Dir() string {
	if a.ForegroundCwd != "" {
		return a.ForegroundCwd
	}
	return a.Cwd
}

// Label is the most useful short name for the agent.
func (a Agent) Label() string {
	if a.Name != "" {
		return a.Name
	}
	return a.Kind
}

type Workspace struct {
	ID       string `json:"workspace_id"`
	Label    string `json:"label"`
	Worktree *struct {
		CheckoutPath string `json:"checkout_path"`
	} `json:"worktree"`
}

type Snapshot struct {
	Agents             []Agent     `json:"agents"`
	Workspaces         []Workspace `json:"workspaces"`
	FocusedWorkspaceID string      `json:"focused_workspace_id"`
}

// WorkspaceByLabel finds a live workspace by its label.
func (s Snapshot) WorkspaceByLabel(label string) *Workspace {
	for i, w := range s.Workspaces {
		if w.Label == label {
			return &s.Workspaces[i]
		}
	}
	return nil
}

func (s Snapshot) HasWorkspace(id string) bool {
	for _, w := range s.Workspaces {
		if w.ID == id {
			return true
		}
	}
	return false
}

type Worktree struct {
	Branch          string `json:"branch"`
	Path            string `json:"path"`
	IsLinked        bool   `json:"is_linked_worktree"`
	OpenWorkspaceID string `json:"open_workspace_id"`
}

// bin is the herdr binary: HERDR_BIN_PATH when herdr launched us (plugins), else PATH.
func bin() string {
	if b := os.Getenv("HERDR_BIN_PATH"); b != "" {
		return b
	}
	return "herdr"
}

// Available reports whether the herdr binary can be found.
func Available() bool {
	_, err := exec.LookPath(bin())
	return err == nil
}

func run(args ...string) ([]byte, error) {
	cmd := exec.Command(bin(), args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		var e struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(msg), &e) == nil && e.Error.Message != "" {
			msg = e.Error.Message
		}
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf(i18n.T("herdr %s : %s", "herdr %s: %s"), args[0]+" "+args[1], msg)
	}
	return stdout.Bytes(), nil
}

func GetSnapshot() (Snapshot, error) {
	out, err := run("api", "snapshot")
	if err != nil {
		return Snapshot{}, err
	}
	var res struct {
		Result struct {
			Snapshot Snapshot `json:"snapshot"`
		} `json:"result"`
	}
	err = json.Unmarshal(out, &res)
	return res.Result.Snapshot, err
}

func ListWorktrees(repo string) ([]Worktree, error) {
	out, err := run("worktree", "list", "--cwd", repo)
	if err != nil {
		return nil, err
	}
	var res struct {
		Result struct {
			Worktrees []Worktree `json:"worktrees"`
		} `json:"result"`
	}
	err = json.Unmarshal(out, &res)
	return res.Result.Worktrees, err
}

func FocusAgent(paneID string) error {
	_, err := run("agent", "focus", paneID)
	return err
}

func FocusWorkspace(id string) error {
	_, err := run("workspace", "focus", id)
	return err
}

// ReportToken sets the $cairn token shown in herdr's sidebar for a workspace;
// an empty value clears it.
func ReportToken(workspaceID, value string) error {
	args := []string{"workspace", "report-metadata", workspaceID, "--source", "cairn"}
	if value == "" {
		args = append(args, "--clear-token", "cairn")
	} else {
		args = append(args, "--token", "cairn="+value)
	}
	_, err := run(args...)
	return err
}

// CreateWorktree makes a git worktree with its own herdr workspace and returns
// the workspace and its root pane.
func CreateWorktree(repo, branch, base, path, label string) (workspaceID, paneID string, err error) {
	out, err := run("worktree", "create", "--cwd", repo, "--branch", branch, "--base", base,
		"--path", path, "--label", label, "--no-focus")
	if err != nil {
		return "", "", err
	}
	var res struct {
		Result struct {
			Workspace struct {
				ID string `json:"workspace_id"`
			} `json:"workspace"`
			RootPane struct {
				ID string `json:"pane_id"`
			} `json:"root_pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return "", "", err
	}
	if res.Result.RootPane.ID == "" {
		return res.Result.Workspace.ID, "", fmt.Errorf("%s", i18n.T("herdr worktree create : pas de pane dans la réponse", "herdr worktree create: no pane in the response"))
	}
	return res.Result.Workspace.ID, res.Result.RootPane.ID, nil
}

// StartAgent starts an agent of kind in the pane; args go to the agent itself
// (claude --model sonnet…).
func StartAgent(name, kind, paneID string, args ...string) error {
	cmd := []string{"agent", "start", name, "--kind", kind, "--pane", paneID, "--timeout", "60000"}
	if len(args) > 0 {
		cmd = append(append(cmd, "--"), args...)
	}
	_, err := run(cmd...)
	return err
}

// StopAgent ends the agent in the pane (/exit) and waits until the pane is
// back at its shell, ready for another agent. Its transcript stays: it can
// be resumed with claude --resume.
func StopAgent(paneID string) error {
	if _, err := run("agent", "prompt", paneID, "/exit"); err != nil {
		return err
	}
	for i := 0; i < 60; i++ {
		time.Sleep(250 * time.Millisecond)
		if _, err := run("agent", "get", paneID); err != nil {
			time.Sleep(500 * time.Millisecond) // let the shell draw its prompt
			return nil
		}
	}
	return fmt.Errorf("%s", i18n.T("l'agent ne s'est pas arrêté (/exit) : ferme-le à la main", "the agent did not stop (/exit): close it by hand"))
}

// Prompt submits text to an agent without waiting for its turn.
func Prompt(target, text string) error {
	// Remembered so the chat hook never takes cairn's words for the human's.
	prompted.Record(config.Dir(), text)
	_, err := run("agent", "prompt", target, text)
	return err
}

// FreeName returns base, or base-2, base-3… if a live agent already has it.
func FreeName(base string, agents []Agent) string {
	taken := map[string]bool{}
	for _, a := range agents {
		taken[a.Name] = true
	}
	name := base
	for i := 2; taken[name]; i++ {
		name = base + "-" + strconv.Itoa(i)
	}
	return name
}

// CreateWorkspace opens a workspace and returns it with its root pane.
func CreateWorkspace(label, cwd string) (workspaceID, paneID string, err error) {
	return createWorkspace(label, cwd, "--focus")
}

func CreateWorkspaceNoFocus(label, cwd string) (workspaceID, paneID string, err error) {
	return createWorkspace(label, cwd, "--no-focus")
}

func createWorkspace(label, cwd, focus string) (workspaceID, paneID string, err error) {
	out, err := run("workspace", "create", "--label", label, "--cwd", cwd, focus)
	if err != nil {
		return "", "", err
	}
	var res struct {
		Result struct {
			Workspace struct {
				ID string `json:"workspace_id"`
			} `json:"workspace"`
			RootPane struct {
				ID string `json:"pane_id"`
			} `json:"root_pane"`
		} `json:"result"`
	}
	err = json.Unmarshal(out, &res)
	return res.Result.Workspace.ID, res.Result.RootPane.ID, err
}

// FirstPane returns the first pane of a workspace.
func FirstPane(workspaceID string) (string, error) {
	out, err := run("pane", "list", "--workspace", workspaceID)
	if err != nil {
		return "", err
	}
	var res struct {
		Result struct {
			Panes []struct {
				ID string `json:"pane_id"`
			} `json:"panes"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return "", err
	}
	if len(res.Result.Panes) == 0 {
		return "", fmt.Errorf(i18n.T("herdr : l'espace %s n'a pas de pane", "herdr: workspace %s has no pane"), workspaceID)
	}
	return res.Result.Panes[0].ID, nil
}

// Runs reports whether a process named name is in the pane's foreground.
func Runs(paneID, name string) bool {
	out, err := run("pane", "process-info", "--pane", paneID)
	if err != nil {
		return false
	}
	var res struct {
		Result struct {
			Info struct {
				Procs []struct {
					Argv0 string `json:"argv0"`
				} `json:"foreground_processes"`
			} `json:"process_info"`
		} `json:"result"`
	}
	if json.Unmarshal(out, &res) != nil {
		return false
	}
	for _, p := range res.Result.Info.Procs {
		if p.Argv0 == name || strings.HasSuffix(p.Argv0, "/"+name) {
			return true
		}
	}
	return false
}

func RunInPane(paneID, command string) error {
	_, err := run("pane", "run", paneID, command)
	return err
}

// Notify shows a herdr notification (in-app toast or system, per herdr's config).
func Notify(title, body string) error {
	_, err := run("notification", "show", title, "--body", body, "--sound", "request")
	return err
}

// RenameAgent names the agent in a pane; cairn uses the ticket key, which is
// how it ties an agent of the main checkout to its chantier.
func RenameAgent(paneID, name string) error {
	_, err := run("agent", "rename", paneID, name)
	return err
}

// RemoveWorktree removes a linked worktree together with its herdr workspace.
// Without force, herdr refuses a worktree with uncommitted changes.
func RemoveWorktree(workspaceID string, force bool) error {
	args := []string{"worktree", "remove", "--workspace", workspaceID}
	if force {
		args = append(args, "--force")
	}
	_, err := run(args...)
	return err
}
