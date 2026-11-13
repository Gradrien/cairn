// Package home gives cairn its own full-screen workspace (a herdr workspace or
// a Rex session), toggled by one key: you keep the multiplexer's sidebar and
// panes, and cairn is one keystroke away.
package home

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"cairn/internal/config"
	"cairn/internal/mux"
	"cairn/internal/native"
)

// Label names cairn's workspace in the sidebar.
const Label = "cairn"

// Env is set in the pane cairn runs in, so the TUI knows "q" means "go back".
const Env = "CAIRN_IN_HERDR=1"

func lastFile() string { return filepath.Join(config.Dir(), "last-workspace") }

// Native reports whether cairn shows as the Herdr app's own panels ("ui":
// "native" in the config): its engine (cairn serve) then runs out of sight
// and no TUI shows.
func Native() bool {
	if !mux.Native() {
		return false
	}
	cfg, err := config.Load()
	return err == nil && cfg.UI == "native"
}

// Toggle shows cairn, or — when cairn is already in front — returns to the
// workspace you came from. It starts cairn's workspace and process if needed.
// Native, it shows the panel.
func Toggle() error {
	if Native() {
		if err := ensureEngine(); err != nil {
			return err
		}
		return mux.ShowPanel()
	}
	return show(true)
}

// Open shows cairn on a chantier: the running TUI selects it (see TakeFocus),
// or natively, the engine does and the panel shows it.
func Open(key string) error {
	if Native() {
		if err := ensureEngine(); err != nil {
			return err
		}
		if err := native.Send(native.Request{Key: strings.ToUpper(strings.TrimSpace(key)), Action: "select"}); err != nil {
			return err
		}
		return mux.ShowPanel()
	}
	if err := os.MkdirAll(filepath.Dir(focusFile()), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(focusFile(), []byte(strings.ToUpper(strings.TrimSpace(key))), 0o644); err != nil {
		return err
	}
	return show(false)
}

func focusFile() string { return filepath.Join(config.Dir(), "cache", "focus") }

// TakeFocus returns the chantier cairn was asked to show, once; "" when none.
func TakeFocus() string {
	data, err := os.ReadFile(focusFile())
	if err != nil {
		return ""
	}
	_ = os.Remove(focusFile())
	return strings.TrimSpace(string(data))
}

func show(toggle bool) error {
	snap, err := mux.GetSnapshot()
	if err != nil {
		return err
	}
	ws := snap.WorkspaceByLabel(Label)
	if ws != nil && snap.FocusedWorkspaceID == ws.ID {
		if toggle {
			return Back()
		}
		return nil
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
		// --focus at creation does not move the client; focusing does.
		return mux.FocusWorkspace(id)
	}
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
// keeps in place. Native, it makes sure the engine runs instead, and closes
// the TUI's workspace.
func Ensure() error {
	snap, err := mux.GetSnapshot()
	if err != nil {
		return err
	}
	if Native() {
		if ws := snap.WorkspaceByLabel(Label); ws != nil {
			_ = mux.CloseWorkspace(ws.ID)
		}
		return ensureEngine()
	}
	self, err := os.Executable()
	if err != nil {
		self = "cairn"
	}
	command := Env + " " + shellQuote(self)
	if ws := snap.WorkspaceByLabel(Label); ws != nil {
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
	_, pane, err := mux.CreateWorkspaceNoFocus(Label, home)
	if err != nil {
		return err
	}
	return mux.RunInPane(pane, command)
}

// EngineName is the terminal the native engine runs in.
const EngineName = "cairn-engine"

func ensureEngine() error {
	if native.Running() {
		return nil
	}
	self, err := os.Executable()
	if err != nil {
		self = "cairn"
	}
	if err := mux.StartEngine(EngineName, "/usr/bin/env", "CAIRN_MUX=herdr-app", self, "serve"); err != nil {
		return err
	}
	for i := 0; i < 30 && !native.Running(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	return nil
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
