// Package mux drives the terminal multiplexer cairn sits in — herdr, the
// Herdr app or Rex — and reads its live state: which agents run where, and in
// which workspace.
//
// All speak the same words here. A workspace is a herdr workspace, a Herdr
// space or a Rex session; a pane is a herdr pane, an hc terminal or a Rex block.
package mux

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"cairn/internal/config"
	"cairn/internal/i18n"
	"cairn/internal/prompted"
)

type Agent struct {
	Kind          string `json:"agent"`
	Name          string `json:"name"`
	Status        string `json:"agent_status"` // working, blocked, done, idle
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

// Backend is one multiplexer.
type Backend interface {
	Name() string
	Available() bool
	Snapshot() (Snapshot, error)
	ListWorktrees(repo string) ([]Worktree, error)
	FocusAgent(paneID string) error
	FocusWorkspace(id string) error
	ReportToken(workspaceID, value string) error
	CreateWorktree(repo, branch, base, path, label string) (workspaceID, paneID string, err error)
	StartAgent(name, kind, paneID string, args ...string) error
	StopAgent(paneID string) error
	Prompt(target, text string) error
	CreateWorkspace(label, cwd string, focus bool) (workspaceID, paneID string, err error)
	FirstPane(workspaceID string) (string, error)
	Runs(paneID, name string) bool
	RunInPane(paneID, command string) error
	Notify(title, body string) error
	RenameAgent(paneID, name string) error
	RemoveWorktree(workspaceID string, force bool) error
}

var (
	mu     sync.Mutex
	chosen Backend
	wanted string
)

// Use picks the multiplexer: "herdr", "herdr-app", "rex", or "" to find the
// one cairn runs in. CAIRN_MUX wins over it.
func Use(name string) {
	mu.Lock()
	defer mu.Unlock()
	wanted, chosen = name, nil
}

func current() Backend {
	mu.Lock()
	defer mu.Unlock()
	if chosen == nil {
		chosen = pick(wanted)
	}
	return chosen
}

func pick(name string) Backend {
	if env := os.Getenv("CAIRN_MUX"); env != "" {
		name = env
	}
	switch strings.ToLower(name) {
	case "herdr":
		return herdrBackend{}
	case "herdr-app":
		return &appBackend{}
	case "rex":
		return &rexBackend{}
	}
	// Inside a pane, the multiplexer that made it. A herdr pane in a Rex
	// terminal is herdr's: it is the closer one.
	if os.Getenv("HERDR_PANE_ID") != "" || os.Getenv("HERDR_ENV") != "" || os.Getenv("HERDR_BIN_PATH") != "" {
		return herdrBackend{}
	}
	if os.Getenv("HC_TERMINAL_ID") != "" {
		return &appBackend{}
	}
	if os.Getenv("REX_BLOCK") != "" || os.Getenv("REX_SESSION") != "" {
		return &rexBackend{}
	}
	if (herdrBackend{}).Available() {
		return herdrBackend{}
	}
	if (&appBackend{}).Available() {
		return &appBackend{}
	}
	if (&rexBackend{}).Available() {
		return &rexBackend{}
	}
	return herdrBackend{}
}

// Name is the multiplexer's name as the user knows it: herdr, Herdr or Rex.
func Name() string { return current().Name() }

// Words fits a sentence written for herdr to the multiplexer in use: under Rex,
// a herdr workspace is a Rex session; in the Herdr app, a Herdr space.
func Words(s string) string {
	switch current().Name() {
	case "herdr":
		return s
	case "Herdr":
		return strings.NewReplacer(
			"espaces herdr", "espaces Herdr", "espace herdr", "espace Herdr",
			"herdr workspaces", "Herdr spaces", "herdr workspace", "Herdr space",
			"herdr", "Herdr",
		).Replace(s)
	}
	return strings.NewReplacer(
		"espaces herdr", "sessions Rex", "espace herdr", "session Rex",
		"herdr workspaces", "Rex sessions", "herdr workspace", "Rex session",
		"herdr's sidebar", "Rex's sidebar", "barre latérale herdr", "barre latérale Rex",
		"herdr", "Rex",
	).Replace(s)
}

// Available reports whether the multiplexer's CLI can be found.
func Available() bool { return current().Available() }

// NotFound says the multiplexer is missing.
func NotFound() string {
	return i18n.T(Name()+" est introuvable dans le PATH", Name()+" is not in the PATH")
}

func GetSnapshot() (Snapshot, error)                { return current().Snapshot() }
func ListWorktrees(repo string) ([]Worktree, error) { return current().ListWorktrees(repo) }
func FocusAgent(paneID string) error                { return current().FocusAgent(paneID) }
func FocusWorkspace(id string) error                { return current().FocusWorkspace(id) }

// ReportToken sets the $cairn token shown in the sidebar for a workspace; an
// empty value clears it.
func ReportToken(workspaceID, value string) error {
	return current().ReportToken(workspaceID, value)
}

// CreateWorktree makes a git worktree with its own workspace and returns the
// workspace and its root pane.
func CreateWorktree(repo, branch, base, path, label string) (workspaceID, paneID string, err error) {
	return current().CreateWorktree(repo, branch, base, path, label)
}

// StartAgent starts an agent of kind in the pane; args go to the agent itself
// (claude --model sonnet…).
func StartAgent(name, kind, paneID string, args ...string) error {
	return current().StartAgent(name, kind, paneID, args...)
}

// StopAgent ends the agent in the pane (/exit) and waits until the pane is
// back at its shell, ready for another agent. Its transcript stays: it can
// be resumed with claude --resume.
func StopAgent(paneID string) error { return current().StopAgent(paneID) }

// Prompt submits text to an agent, by pane or name, without waiting for its turn.
func Prompt(target, text string) error {
	// Remembered so the chat hook never takes cairn's words for the human's.
	prompted.Record(config.Dir(), text)
	return current().Prompt(target, text)
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
	return current().CreateWorkspace(label, cwd, true)
}

func CreateWorkspaceNoFocus(label, cwd string) (workspaceID, paneID string, err error) {
	return current().CreateWorkspace(label, cwd, false)
}

// FirstPane returns the first pane of a workspace.
func FirstPane(workspaceID string) (string, error) { return current().FirstPane(workspaceID) }

// Runs reports whether a process named name is in the pane's foreground.
func Runs(paneID, name string) bool { return current().Runs(paneID, name) }

// RunInPane types command into the pane's shell and runs it.
func RunInPane(paneID, command string) error { return current().RunInPane(paneID, command) }

// Notify shows a notification (in-app or system, per the multiplexer).
func Notify(title, body string) error { return current().Notify(title, body) }

// RenameAgent names the agent in a pane; cairn uses the ticket key, which is
// how it ties an agent of the main checkout to its chantier.
func RenameAgent(paneID, name string) error { return current().RenameAgent(paneID, name) }

// EnsureHomePanel gives cairn's workspace a native board next to the TUI,
// where the multiplexer can draw one (the Herdr app); elsewhere it does nothing.
func EnsureHomePanel(workspaceID string) error {
	if h, ok := current().(interface{ EnsureHomePanel(string) error }); ok {
		return h.EnsureHomePanel(workspaceID)
	}
	return nil
}

// RemoveWorktree removes a linked worktree together with its workspace.
// Without force, it refuses a worktree with uncommitted changes.
func RemoveWorktree(workspaceID string, force bool) error {
	return current().RemoveWorktree(workspaceID, force)
}
