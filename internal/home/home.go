// Package home gives cairn its own full-screen workspace (a herdr workspace or
// a Rex session), toggled by one key: you keep the multiplexer's sidebar and
// panes, and cairn is one keystroke away.
package home

import (
	"os"
	"path/filepath"
	"strings"

	"cairn/internal/config"
	"cairn/internal/mux"
)

// Label names cairn's workspace in the sidebar.
const Label = "cairn"

// Env is set in the pane cairn runs in, so the TUI knows "q" means "go back".
const Env = "CAIRN_IN_HERDR=1"

func lastFile() string { return filepath.Join(config.Dir(), "last-workspace") }

// Toggle shows cairn, or — when cairn is already in front — returns to the
// workspace you came from. It starts cairn's workspace and process if needed.
func Toggle() error {
	snap, err := mux.GetSnapshot()
	if err != nil {
		return err
	}
	ws := snap.WorkspaceByLabel(Label)
	if ws != nil && snap.FocusedWorkspaceID == ws.ID {
		return Back()
	}
	if snap.FocusedWorkspaceID != "" {
		_ = os.WriteFile(lastFile(), []byte(snap.FocusedWorkspaceID), 0o644)
	}
	self, err := os.Executable()
	if err != nil {
		self = "cairn"
	}
	command := Env + " " + shellQuote(self)
	if ws == nil {
		home, _ := os.UserHomeDir()
		id, pane, err := mux.CreateWorkspace(Label, home)
		if err != nil {
			return err
		}
		if err := mux.RunInPane(pane, command); err != nil {
			return err
		}
		_ = mux.EnsureHomePanel(id)
		// --focus at creation does not move the client; focusing does.
		return mux.FocusWorkspace(id)
	}
	_ = mux.EnsureHomePanel(ws.ID)
	if err := mux.FocusWorkspace(ws.ID); err != nil {
		return err
	}
	pane, err := mux.FirstPane(ws.ID)
	if err != nil {
		return err
	}
	if !mux.Runs(pane, "cairn") {
		return mux.RunInPane(pane, command)
	}
	return nil
}

// Ensure makes sure cairn's workspace exists and runs cairn, without moving
// focus: it is the sidebar button herdr's startup hook (or install.sh --rex)
// keeps in place.
func Ensure() error {
	snap, err := mux.GetSnapshot()
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		self = "cairn"
	}
	command := Env + " " + shellQuote(self)
	if ws := snap.WorkspaceByLabel(Label); ws != nil {
		_ = mux.EnsureHomePanel(ws.ID)
		pane, err := mux.FirstPane(ws.ID)
		if err != nil {
			return err
		}
		if !mux.Runs(pane, "cairn") {
			return mux.RunInPane(pane, command)
		}
		return nil
	}
	home, _ := os.UserHomeDir()
	id, pane, err := mux.CreateWorkspaceNoFocus(Label, home)
	if err != nil {
		return err
	}
	if err := mux.RunInPane(pane, command); err != nil {
		return err
	}
	return mux.EnsureHomePanel(id)
}

// Back focuses the workspace that was in front before cairn.
func Back() error {
	data, err := os.ReadFile(lastFile())
	if err != nil {
		return nil
	}
	id := strings.TrimSpace(string(data))
	snap, err := mux.GetSnapshot()
	if err != nil || !snap.HasWorkspace(id) {
		return err
	}
	return mux.FocusWorkspace(id)
}

// Inside reports whether this process is the one running in cairn's workspace.
func Inside() bool { return os.Getenv("CAIRN_IN_HERDR") == "1" }

func shellQuote(s string) string {
	if !strings.ContainsAny(s, " '\"$\\") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
