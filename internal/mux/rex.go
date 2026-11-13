package mux

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"cairn/internal/config"
	"cairn/internal/i18n"
)

// rexBackend drives Rex (superlogical.com/rex) through its CLI. A workspace
// is a Rex session, a pane is a terminal block, and an agent is a block
// running a coding agent: Claude Code and others report their state to Rex
// themselves (OSC 7501), which is where Status comes from.
type rexBackend struct {
	mu   sync.Mutex
	snap *rexSnap
	at   time.Time
}

//go:embed rex.lua
var rexScript string

func (*rexBackend) Name() string { return "Rex" }

func rexBin() string {
	if b := os.Getenv("CAIRN_REX_BIN"); b != "" {
		return b
	}
	if _, err := exec.LookPath("rex"); err == nil {
		return "rex"
	}
	if d := os.Getenv("REX_BIN_DIR"); d != "" {
		return filepath.Join(d, "rex")
	}
	return "rex"
}

func (*rexBackend) Available() bool {
	_, err := exec.LookPath(rexBin())
	return err == nil
}

func rexCLI(args ...string) ([]byte, error) {
	var global []string
	if c := os.Getenv("CAIRN_REX_CLIENT"); c != "" {
		global = append(global, "-C", c)
	}
	cmd := exec.Command(rexBin(), append(global, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if len(args) > 1 && args[0] == "do" && args[1] == "-" {
		cmd.Stdin = strings.NewReader(rexScript)
	}
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(stderr.String()), "Error:"))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf(i18n.T("rex %s : %s", "rex %s: %s"), args[0], msg)
	}
	return stdout.Bytes(), nil
}

// op runs one operation of rex.lua and decodes its result into out.
func (*rexBackend) op(out any, name string, kv ...string) error {
	a := map[string]string{"op": name}
	for i := 0; i+1 < len(kv); i += 2 {
		a[kv[i]] = kv[i+1]
	}
	js, _ := json.Marshal(a)
	data, err := rexCLI("do", "-", "--args", string(js))
	if err != nil {
		return err
	}
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &e) == nil && e.Error != "" {
		return fmt.Errorf("rex %s: %s", name, e.Error)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

type rexRecord struct {
	ID    string `json:"id"`
	State string `json:"state"`
	App   string `json:"app"`
}

// rexRecords reads Lua's empty table, which comes out as {}, as no record.
type rexRecords []rexRecord

func (r *rexRecords) UnmarshalJSON(b []byte) error {
	if t := bytes.TrimSpace(b); len(t) > 0 && t[0] == '[' {
		return json.Unmarshal(t, (*[]rexRecord)(r))
	}
	*r = nil
	return nil
}

type rexBlock struct {
	SessionID string     `json:"session_id"`
	WindowID  string     `json:"window_id"`
	BlockID   string     `json:"block_id"`
	Label     string     `json:"label"`
	Argv0     string     `json:"argv0"`
	Name      string     `json:"name"`
	Cwd       string     `json:"cwd"`
	ShellCwd  string     `json:"shell_cwd"`
	Title     string     `json:"title"`
	Records   rexRecords `json:"records"`
	Missing   bool       `json:"missing"`
}

type rexSession struct {
	ID    string `json:"session_id"`
	Label string `json:"label"`
}

type rexSnap struct {
	Sessions []rexSession `json:"sessions"`
	Blocks   []rexBlock   `json:"blocks"`
}

// agentKinds are the agents cairn knows by their program or the app they
// report to Rex.
var agentKinds = map[string]string{
	"claude": "claude", "claude-code": "claude", "codex": "codex", "opencode": "opencode",
	"gemini": "gemini", "aider": "aider", "amp": "amp", "cursor-agent": "cursor",
	"goose": "goose", "crush": "crush", "droid": "droid", "qwen": "qwen", "pi": "pi",
}

// kind is the agent running in the block, "" when it runs none.
func (b rexBlock) kind() string {
	if k := agentKinds[filepath.Base(b.Argv0)]; k != "" {
		return k
	}
	if k := agentKinds[b.Name]; k != "" {
		return k
	}
	for _, r := range b.Records {
		if k := agentKinds[r.App]; k != "" && (b.Argv0 != "" && !isShell(b.Argv0)) {
			return k
		}
	}
	return ""
}

func isShell(argv0 string) bool {
	switch strings.TrimPrefix(filepath.Base(argv0), "-") {
	case "sh", "bash", "zsh", "fish", "dash", "ksh", "tcsh", "nu", "login":
		return true
	}
	return false
}

// status folds the block's records into herdr's words: what needs you first.
func (b rexBlock) status() string {
	rank := map[string]int{"blocked": 4, "error": 4, "done": 3, "working": 2, "idle": 1}
	best := ""
	for _, r := range b.Records {
		if rank[r.State] > rank[best] {
			best = r.State
		}
	}
	switch best {
	case "":
		return "idle"
	case "error":
		return "blocked"
	}
	return best
}

func (b rexBlock) agent() Agent {
	name := b.Label
	if name == "shell" { // Rex's default label: the agent has no name yet
		name = ""
	}
	return Agent{Kind: b.kind(), Name: name, Status: b.status(), Cwd: b.ShellCwd, ForegroundCwd: b.Cwd,
		PaneID: b.BlockID, WorkspaceID: b.SessionID, Title: b.Title}
}

// sep joins a session's own label and cairn's token, as in "YAK-12 · build".
const sep = " · "

func baseLabel(label string) string {
	base, _, _ := strings.Cut(label, sep)
	return base
}

func (r *rexBackend) rawSnapshot(fresh bool) (*rexSnap, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !fresh && r.snap != nil && time.Since(r.at) < 2*time.Second {
		return r.snap, nil
	}
	var s rexSnap
	if err := r.op(&s, "snapshot"); err != nil {
		return nil, err
	}
	r.snap, r.at = &s, time.Now()
	return r.snap, nil
}

func (r *rexBackend) forget() {
	r.mu.Lock()
	r.snap = nil
	r.mu.Unlock()
}

func (r *rexBackend) Snapshot() (Snapshot, error) {
	raw, err := r.rawSnapshot(true)
	if err != nil {
		return Snapshot{}, err
	}
	links := loadLinks(raw)
	var out Snapshot
	for _, s := range raw.Sessions {
		w := Workspace{ID: s.ID, Label: baseLabel(s.Label)}
		if p := links[s.ID]; p != "" {
			w.Worktree = &struct {
				CheckoutPath string `json:"checkout_path"`
			}{p}
		}
		out.Workspaces = append(out.Workspaces, w)
	}
	for _, b := range raw.Blocks {
		if b.kind() != "" {
			out.Agents = append(out.Agents, b.agent())
		}
	}
	// Rex does not say which session the app shows; the toggle action passes
	// it, and a command typed in a Rex terminal runs in the one in front.
	out.FocusedWorkspaceID = os.Getenv("CAIRN_FROM_SESSION")
	if out.FocusedWorkspaceID == "" {
		out.FocusedWorkspaceID = os.Getenv("REX_SESSION")
	}
	return out, nil
}

// ── worktrees ───────────────────────────────────────────────────────────────

// links remembers which worktree each session cairn made is for: Rex knows
// nothing of git.
func linksFile() string { return filepath.Join(config.Dir(), "cache", "rex-worktrees.json") }

func readLinks() map[string]string {
	out := map[string]string{}
	if data, err := os.ReadFile(linksFile()); err == nil {
		_ = json.Unmarshal(data, &out)
	}
	return out
}

func writeLinks(l map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(linksFile()), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(l, "", "  ")
	return os.WriteFile(linksFile(), data, 0o644)
}

// loadLinks returns the links of the live sessions.
func loadLinks(raw *rexSnap) map[string]string {
	live := map[string]bool{}
	for _, s := range raw.Sessions {
		live[s.ID] = true
	}
	out := map[string]string{}
	for id, p := range readLinks() {
		if live[id] {
			out[id] = p
		}
	}
	return out
}

func within(dir, root string) bool {
	if dir == "" {
		return false
	}
	dir, root = filepath.Clean(dir), filepath.Clean(root)
	return dir == root || strings.HasPrefix(dir, root+string(filepath.Separator))
}

func (r *rexBackend) ListWorktrees(repo string) ([]Worktree, error) {
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
	raw, err := r.rawSnapshot(false)
	if err != nil {
		return wts, nil // the worktrees still tell where each chantier stands
	}
	links := loadLinks(raw)
	for i := range wts {
		if !wts[i].IsLinked {
			continue
		}
		for id, p := range links {
			if filepath.Clean(p) == filepath.Clean(wts[i].Path) {
				wts[i].OpenWorkspaceID = id
			}
		}
		if wts[i].OpenWorkspaceID != "" {
			continue
		}
		// A session made by hand: the one whose terminals sit in the worktree.
		for _, b := range raw.Blocks {
			if within(b.ShellCwd, wts[i].Path) || within(b.Cwd, wts[i].Path) {
				wts[i].OpenWorkspaceID = b.SessionID
				break
			}
		}
	}
	return wts, nil
}

func (r *rexBackend) CreateWorktree(repo, branch, base, path, label string) (workspaceID, paneID string, err error) {
	cmd := exec.Command("git", "-C", repo, "worktree", "add", "-b", branch, path, base)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("git worktree add: %s", strings.TrimSpace(string(out)))
	}
	ws, pane, err := r.newSession(label, path)
	if err != nil {
		return "", "", err
	}
	links := readLinks()
	links[ws] = path
	return ws, pane, writeLinks(links)
}

func (r *rexBackend) RemoveWorktree(workspaceID string, force bool) error {
	links := readLinks()
	path := links[workspaceID]
	if path == "" {
		raw, err := r.rawSnapshot(true)
		if err != nil {
			return err
		}
		for _, b := range raw.Blocks {
			if b.SessionID == workspaceID && b.ShellCwd != "" {
				if top, err := exec.Command("git", "-C", b.ShellCwd, "rev-parse", "--show-toplevel").Output(); err == nil {
					path = strings.TrimSpace(string(top))
					break
				}
			}
		}
	}
	if path == "" {
		return fmt.Errorf(i18n.T("rex : pas de worktree connu pour la session %s", "rex: no known worktree for session %s"), workspaceID)
	}
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
	if _, err := rexCLI("kill", workspaceID); err != nil {
		return err
	}
	delete(links, workspaceID)
	r.forget()
	return writeLinks(links)
}

func absTo(base, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

// ── sessions and blocks ─────────────────────────────────────────────────────

func (r *rexBackend) newSession(label, cwd string) (workspaceID, paneID string, err error) {
	out, err := rexCLI("new", label, "-c", cwd, "--json")
	if err != nil {
		return "", "", err
	}
	var res struct {
		SessionID string `json:"session_id"`
		Windows   []struct {
			Blocks []string `json:"block_ids"`
		} `json:"initial_windows"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return "", "", err
	}
	if len(res.Windows) == 0 || len(res.Windows[0].Blocks) == 0 {
		return res.SessionID, "", fmt.Errorf("%s", i18n.T("rex new : pas de terminal dans la réponse", "rex new: no terminal in the response"))
	}
	r.forget()
	return res.SessionID, res.Windows[0].Blocks[0], nil
}

func (r *rexBackend) CreateWorkspace(label, cwd string, focus bool) (workspaceID, paneID string, err error) {
	ws, pane, err := r.newSession(label, cwd)
	if err == nil && focus {
		_ = r.FocusWorkspace(ws)
	}
	return ws, pane, err
}

func (r *rexBackend) FirstPane(workspaceID string) (string, error) {
	var res struct {
		BlockID string `json:"block_id"`
	}
	err := r.op(&res, "first_block", "session", workspaceID)
	return res.BlockID, err
}

// focus brings a block or session to the front of the Rex app. Called from
// cairn's toggle action, it hands the session to the action, which may switch
// the app's session without Remote Control.
func (r *rexBackend) focus(session, block string) error {
	var res struct {
		SessionID string `json:"session_id"`
		WindowID  string `json:"window_id"`
	}
	if block != "" {
		if err := r.op(&res, "focus", "block", block); err != nil {
			return err
		}
		session = res.SessionID
	}
	if os.Getenv("CAIRN_REX_ACTION") != "" {
		fmt.Println("select " + session)
		return nil
	}
	args := []string{"do", "session.select", "session_id=" + session}
	if res.WindowID != "" {
		args = append(args, "window_id="+res.WindowID)
	}
	if _, err := rexCLI(args...); err != nil {
		if strings.Contains(err.Error(), "Remote Control") {
			return errors.New(i18n.T(
				"Rex n'accepte pas qu'on change sa session : active Remote Control dans ses réglages (Rex Server), ou utilise la touche cairn",
				"Rex does not let cairn switch its session: turn on Remote Control in its settings (Rex Server), or use the cairn key"))
		}
		return err
	}
	return nil
}

func (r *rexBackend) FocusWorkspace(id string) error { return r.focus(id, "") }

func (r *rexBackend) FocusAgent(paneID string) error { return r.focus("", paneID) }

func (r *rexBackend) block(paneID string) (rexBlock, error) {
	var b rexBlock
	err := r.op(&b, "block", "block", paneID)
	return b, err
}

func (r *rexBackend) Runs(paneID, name string) bool {
	b, err := r.block(paneID)
	if err != nil || b.Missing {
		return false
	}
	return filepath.Base(strings.TrimPrefix(b.Argv0, "-")) == name || b.Name == name
}

func (r *rexBackend) RunInPane(paneID, command string) error {
	return r.op(nil, "write", "block", paneID, "data", command+"\r")
}

func (r *rexBackend) RenameAgent(paneID, name string) error {
	r.forget()
	return r.op(nil, "label", "block", paneID, "label", name)
}

// resolve turns an agent name into its block; a block ID stays as is.
func (r *rexBackend) resolve(target string) (string, error) {
	if strings.HasPrefix(target, "block:") {
		return target, nil
	}
	raw, err := r.rawSnapshot(true)
	if err != nil {
		return "", err
	}
	for _, b := range raw.Blocks {
		if b.Label == target && b.kind() != "" {
			return b.BlockID, nil
		}
	}
	for _, b := range raw.Blocks {
		if b.Label == target || strings.HasSuffix(b.BlockID, target) {
			return b.BlockID, nil
		}
	}
	return "", fmt.Errorf(i18n.T("rex : aucun agent %s", "rex: no agent %s"), target)
}

func (r *rexBackend) Prompt(target, text string) error {
	pane, err := r.resolve(target)
	if err != nil {
		return err
	}
	return r.op(nil, "prompt", "block", pane, "text", text)
}

// StartAgent types the agent's command into the pane's shell, then waits
// until it is ready for a prompt: it says so itself (OSC 7501 idle), or it
// has run a while without saying anything.
func (r *rexBackend) StartAgent(name, kind, paneID string, args ...string) error {
	if err := r.RenameAgent(paneID, name); err != nil {
		return err
	}
	words := []string{shellQuote(kind)}
	for _, a := range args {
		words = append(words, shellQuote(a))
	}
	if err := r.RunInPane(paneID, strings.Join(words, " ")); err != nil {
		return err
	}
	var seen time.Time
	for deadline := time.Now().Add(60 * time.Second); time.Now().Before(deadline); time.Sleep(400 * time.Millisecond) {
		b, err := r.block(paneID)
		if err != nil {
			return err
		}
		if b.Missing {
			return fmt.Errorf(i18n.T("rex : le terminal de %s a disparu", "rex: %s's terminal is gone"), name)
		}
		if b.kind() == "" {
			continue
		}
		if seen.IsZero() {
			seen = time.Now()
		}
		switch st := b.status(); {
		case len(b.Records) > 0 && st == "idle":
			return nil
		case len(b.Records) == 0 && time.Since(seen) > 8*time.Second:
			return nil // an agent that reports nothing: give it time to draw
		}
	}
	return fmt.Errorf(i18n.T("rex : %s n'est pas prêt après 60 s (il attend peut-être une confirmation)", "rex: %s is not ready after 60 s (it may be waiting for a confirmation)"), kind)
}

func (r *rexBackend) StopAgent(paneID string) error {
	if err := r.op(nil, "prompt", "block", paneID, "text", "/exit"); err != nil {
		return err
	}
	for i := 0; i < 60; i++ {
		time.Sleep(250 * time.Millisecond)
		b, err := r.block(paneID)
		if err != nil {
			return err
		}
		if b.Missing || b.kind() == "" {
			time.Sleep(500 * time.Millisecond) // let the shell draw its prompt
			r.forget()
			return nil
		}
	}
	return fmt.Errorf("%s", i18n.T("l'agent ne s'est pas arrêté (/exit) : ferme-le à la main", "the agent did not stop (/exit): close it by hand"))
}

// ticketLabel is a session cairn may relabel: one named after a ticket.
var ticketLabel = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*-[0-9]+$`)

// ReportToken shows the token in the session's label, the closest Rex has to
// herdr's sidebar rows: "YAK-12 · build 2/5". Only cairn's own session and
// sessions named after a ticket are touched; yours keep the name you gave.
func (r *rexBackend) ReportToken(workspaceID, value string) error {
	raw, err := r.rawSnapshot(false)
	if err != nil {
		return err
	}
	for _, s := range raw.Sessions {
		if s.ID != workspaceID {
			continue
		}
		base := baseLabel(s.Label)
		if base != "cairn" && !ticketLabel.MatchString(base) {
			return nil
		}
		want := base
		if value != "" {
			want = base + sep + value
		}
		if want == s.Label {
			return nil
		}
		r.forget()
		return r.op(nil, "session_label", "session", workspaceID, "label", want)
	}
	return nil
}

// Notify asks the Rex app for a notification the way any program can: an
// OSC 777 sequence on cairn's own terminal. Outside Rex, macOS shows it.
func (r *rexBackend) Notify(title, body string) error {
	clean := strings.NewReplacer(";", ",", "\x1b", "", "\a", "", "\n", " ", "\r", " ")
	if os.Getenv("REX_BLOCK") != "" {
		if tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0); err == nil {
			defer tty.Close()
			_, err = fmt.Fprintf(tty, "\x1b]777;notify;%s;%s\x1b\\", clean.Replace(title), clean.Replace(body))
			return err
		}
	}
	if runtime.GOOS == "darwin" {
		q := func(s string) string { return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"` }
		return exec.Command("osascript", "-e", "display notification "+q(body)+" with title "+q(title)).Run()
	}
	return nil
}

func shellQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " '\"$\\`!*?[]{}()<>|&;#~\t\n") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
