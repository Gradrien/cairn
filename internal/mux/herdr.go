package mux

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"cairn/internal/i18n"
)

// herdrBackend drives the herdr CLI (socket API).
type herdrBackend struct{}

func (herdrBackend) Name() string { return "herdr" }

// bin is the herdr binary: HERDR_BIN_PATH when herdr launched us (plugins), else PATH.
func herdrBin() string {
	if b := os.Getenv("HERDR_BIN_PATH"); b != "" {
		return b
	}
	return "herdr"
}

func (herdrBackend) Available() bool {
	_, err := exec.LookPath(herdrBin())
	return err == nil
}

func herdrRun(args ...string) ([]byte, error) {
	cmd := exec.Command(herdrBin(), args...)
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

func (herdrBackend) Snapshot() (Snapshot, error) {
	out, err := herdrRun("api", "snapshot")
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

func (herdrBackend) ListWorktrees(repo string) ([]Worktree, error) {
	out, err := herdrRun("worktree", "list", "--cwd", repo)
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

func (herdrBackend) FocusAgent(paneID string) error {
	_, err := herdrRun("agent", "focus", paneID)
	return err
}

func (herdrBackend) FocusWorkspace(id string) error {
	_, err := herdrRun("workspace", "focus", id)
	return err
}

func (herdrBackend) ReportToken(workspaceID, value string) error {
	args := []string{"workspace", "report-metadata", workspaceID, "--source", "cairn"}
	if value == "" {
		args = append(args, "--clear-token", "cairn")
	} else {
		args = append(args, "--token", "cairn="+value)
	}
	_, err := herdrRun(args...)
	return err
}

func (herdrBackend) CreateWorktree(repo, branch, base, path, label string) (workspaceID, paneID string, err error) {
	out, err := herdrRun("worktree", "create", "--cwd", repo, "--branch", branch, "--base", base,
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

func (herdrBackend) StartAgent(name, kind, paneID string, args ...string) error {
	cmd := []string{"agent", "start", name, "--kind", kind, "--pane", paneID, "--timeout", "60000"}
	if len(args) > 0 {
		cmd = append(append(cmd, "--"), args...)
	}
	_, err := herdrRun(cmd...)
	return err
}

func (herdrBackend) StopAgent(paneID string) error {
	if _, err := herdrRun("agent", "prompt", paneID, "/exit"); err != nil {
		return err
	}
	for i := 0; i < 60; i++ {
		time.Sleep(250 * time.Millisecond)
		if _, err := herdrRun("agent", "get", paneID); err != nil {
			time.Sleep(500 * time.Millisecond) // let the shell draw its prompt
			return nil
		}
	}
	return fmt.Errorf("%s", i18n.T("l'agent ne s'est pas arrêté (/exit) : ferme-le à la main", "the agent did not stop (/exit): close it by hand"))
}

func (herdrBackend) Prompt(target, text string) error {
	_, err := herdrRun("agent", "prompt", target, text)
	return err
}

func (herdrBackend) CreateWorkspace(label, cwd string, focus bool) (workspaceID, paneID string, err error) {
	flag := "--no-focus"
	if focus {
		flag = "--focus"
	}
	out, err := herdrRun("workspace", "create", "--label", label, "--cwd", cwd, flag)
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

func (herdrBackend) FirstPane(workspaceID string) (string, error) {
	out, err := herdrRun("pane", "list", "--workspace", workspaceID)
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

func (herdrBackend) Runs(paneID, name string) bool {
	out, err := herdrRun("pane", "process-info", "--pane", paneID)
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

func (herdrBackend) RunInPane(paneID, command string) error {
	_, err := herdrRun("pane", "run", paneID, command)
	return err
}

func (herdrBackend) Notify(title, body string) error {
	_, err := herdrRun("notification", "show", title, "--body", body, "--sound", "request")
	return err
}

func (herdrBackend) RenameAgent(paneID, name string) error {
	_, err := herdrRun("agent", "rename", paneID, name)
	return err
}

func (herdrBackend) RemoveWorktree(workspaceID string, force bool) error {
	args := []string{"worktree", "remove", "--workspace", workspaceID}
	if force {
		args = append(args, "--force")
	}
	_, err := herdrRun(args...)
	return err
}
