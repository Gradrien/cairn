package mux

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cairn/internal/i18n"
)

// appBackend drives the Herdr app (herdr.dev): `herdr-app` for what the window
// shows (spaces, tabs, focus) and `hc` (herdr-core) for the terminals. A
// workspace is a Herdr space and a pane is a terminal, addressed by its hc
// address (t7). Agents report their state to herdr-core themselves (OSC 7501).
type appBackend struct {
	mu   sync.Mutex
	snap *appSnap
	at   time.Time
}

const appLinks = "herdr-app-worktrees.json"

// appAgentLabel is the hc label that holds the name cairn gave an agent.
const appAgentLabel = "cairn.agent"

func (*appBackend) Name() string { return "Herdr" }

func appBin() string {
	if b := os.Getenv("CAIRN_HERDR_APP_BIN"); b != "" {
		return b
	}
	return "herdr-app"
}

func hcBin() string {
	if b := os.Getenv("CAIRN_HC_BIN"); b != "" {
		return b
	}
	return "hc"
}

func (*appBackend) Available() bool {
	_, err := exec.LookPath(appBin())
	return err == nil
}

func appCall(bin string, args ...string) ([]byte, error) {
	cmd := exec.Command(bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(stdout.Bytes(), &e) == nil && e.Error != "" {
		return nil, fmt.Errorf("%s %s: %s", filepath.Base(bin), strings.Join(args[:min(2, len(args))], " "), e.Error)
	}
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s %s: %s", filepath.Base(bin), strings.Join(args[:min(2, len(args))], " "), msg)
	}
	return stdout.Bytes(), nil
}

// do runs a Herdr app command with its arguments as one JSON object.
func appDo(out any, command string, args map[string]any) error {
	js, _ := json.Marshal(args)
	data, err := appCall(appBin(), "do", command, string(js))
	if err != nil || out == nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func hcRun(args ...string) ([]byte, error) { return appCall(hcBin(), args...) }

type appTerminal struct {
	Terminal string  `json:"terminal"`
	Pane     string  `json:"pane"`
	Status   *string `json:"status"`
	App      *string `json:"app"`
}

type appSpace struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Folder string `json:"folder"`
	Active bool   `json:"active"`
	Tabs   []struct {
		ID        string        `json:"id"`
		Terminals []appTerminal `json:"terminals"`
	} `json:"tabs"`
}

type appSnap struct {
	Spaces []appSpace `json:"spaces"`
	// Programs maps each live terminal to what runs in front of it ("zsh", "aider").
	Programs map[string]string
}

// hcTerminal is the part of `hc get` cairn reads.
type hcTerminal struct {
	Address string            `json:"address"`
	Cwd     string            `json:"cwd"`
	Title   string            `json:"title"`
	State   string            `json:"state"`
	Labels  map[string]string `json:"labels"`
	Status  *struct {
		App   string `json:"app"`
		State string `json:"state"`
	} `json:"status"`
	Foreground *struct {
		Argv []string `json:"argv"`
		Cwd  string   `json:"cwd"`
		Name string   `json:"name"`
	} `json:"foreground"`
}

func hcGet(address string) (hcTerminal, error) {
	var t hcTerminal
	out, err := hcRun("get", address, "--json")
	if err != nil {
		return t, err
	}
	err = json.Unmarshal(out, &t)
	return t, err
}

func (r *appBackend) rawSnapshot(fresh bool) (*appSnap, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !fresh && r.snap != nil && time.Since(r.at) < 2*time.Second {
		return r.snap, nil
	}
	out, err := appCall(appBin(), "ls")
	if err != nil {
		return nil, err
	}
	var s appSnap
	if err := json.Unmarshal(out, &s); err != nil {
		return nil, err
	}
	s.Programs = map[string]string{}
	if out, err := hcRun("ls", "--json"); err == nil {
		var l struct {
			Terminals []struct {
				Address string `json:"address"`
				Agent   string `json:"agent"`
			} `json:"terminals"`
		}
		if json.Unmarshal(out, &l) == nil {
			for _, t := range l.Terminals {
				s.Programs[t.Address] = t.Agent
			}
		}
	}
	r.snap, r.at = &s, time.Now()
	return r.snap, nil
}

func (r *appBackend) forget() {
	r.mu.Lock()
	r.snap = nil
	r.mu.Unlock()
}

// appKind is the agent in a terminal: what it reports, else the program in
// front of it. Claude Code's process is named after its version (2.1.323), so
// before it reports anything, only its command line tells it apart.
func appKind(t appTerminal, program string, get func() (hcTerminal, error)) (string, *hcTerminal) {
	if t.App != nil {
		if k := agentKinds[*t.App]; k != "" {
			return k, nil
		}
	}
	if k := agentKinds[filepath.Base(program)]; k != "" {
		return k, nil
	}
	if program == "" || isShell(program) {
		return "", nil
	}
	d, err := get()
	if err != nil || d.Foreground == nil || len(d.Foreground.Argv) == 0 {
		return "", nil
	}
	return agentKinds[filepath.Base(d.Foreground.Argv[0])], &d
}

func (r *appBackend) Snapshot() (Snapshot, error) {
	raw, err := r.rawSnapshot(true)
	if err != nil {
		return Snapshot{}, err
	}
	live := map[string]bool{}
	for _, s := range raw.Spaces {
		live[s.ID] = true
	}
	links := liveLinks(appLinks, live)
	var out Snapshot
	for _, s := range raw.Spaces {
		w := Workspace{ID: s.ID, Label: baseLabel(s.Name)}
		if p := links[s.ID]; p != "" {
			w.Worktree = &struct {
				CheckoutPath string `json:"checkout_path"`
			}{p}
		}
		out.Workspaces = append(out.Workspaces, w)
		if s.Active {
			out.FocusedWorkspaceID = s.ID
		}
		for _, tab := range s.Tabs {
			for _, t := range tab.Terminals {
				kind, d := appKind(t, raw.Programs[t.Terminal], func() (hcTerminal, error) { return hcGet(t.Terminal) })
				if kind == "" {
					continue
				}
				a := Agent{Kind: kind, Status: "idle", PaneID: t.Terminal, WorkspaceID: s.ID}
				if t.Status != nil {
					a.Status = *t.Status
				}
				if d == nil {
					if got, err := hcGet(t.Terminal); err == nil {
						d = &got
					}
				}
				if d != nil {
					a.Name, a.Cwd, a.Title = d.Labels[appAgentLabel], d.Cwd, d.Title
					if d.Foreground != nil {
						a.ForegroundCwd = d.Foreground.Cwd
					}
				}
				out.Agents = append(out.Agents, a)
			}
		}
	}
	if st, err := appCall(appBin(), "status"); err == nil {
		var v struct {
			Space string `json:"space"`
		}
		if json.Unmarshal(st, &v) == nil && v.Space != "" {
			out.FocusedWorkspaceID = v.Space
		}
	}
	return out, nil
}

func (r *appBackend) ListWorktrees(repo string) ([]Worktree, error) {
	wts, err := gitWorktrees(repo)
	if err != nil {
		return nil, err
	}
	raw, err := r.rawSnapshot(false)
	if err != nil {
		return wts, nil
	}
	live := map[string]bool{}
	for _, s := range raw.Spaces {
		live[s.ID] = true
	}
	links := liveLinks(appLinks, live)
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
		// A space opened by hand: the one that starts its terminals in the worktree.
		for _, s := range raw.Spaces {
			if within(s.Folder, wts[i].Path) {
				wts[i].OpenWorkspaceID = s.ID
				break
			}
		}
	}
	return wts, nil
}

func (r *appBackend) CreateWorktree(repo, branch, base, path, label string) (workspaceID, paneID string, err error) {
	cmd := exec.Command("git", "-C", repo, "worktree", "add", "-b", branch, path, base)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("git worktree add: %s", strings.TrimSpace(string(out)))
	}
	ws, pane, err := r.CreateWorkspace(label, path, false)
	if err != nil {
		return "", "", err
	}
	links := readLinks(appLinks)
	links[ws] = path
	return ws, pane, writeLinks(appLinks, links)
}

func (r *appBackend) RemoveWorktree(workspaceID string, force bool) error {
	links := readLinks(appLinks)
	path := links[workspaceID]
	if path == "" {
		raw, err := r.rawSnapshot(true)
		if err != nil {
			return err
		}
		for _, s := range raw.Spaces {
			if s.ID != workspaceID || s.Folder == "" {
				continue
			}
			if top, err := exec.Command("git", "-C", s.Folder, "rev-parse", "--show-toplevel").Output(); err == nil {
				path = strings.TrimSpace(string(top))
			}
		}
	}
	if path == "" {
		return fmt.Errorf(i18n.T("Herdr : pas de worktree connu pour l'espace %s", "Herdr: no known worktree for space %s"), workspaceID)
	}
	if err := removeGitWorktree(path, force); err != nil {
		return err
	}
	if err := appDo(nil, "space.close", map[string]any{"id": workspaceID, "confirmed": true}); err != nil {
		return err
	}
	delete(links, workspaceID)
	r.forget()
	return writeLinks(appLinks, links)
}

// CreateWorkspace opens a space. The Herdr app always shows a new space, so
// without focus cairn puts back the one that was in front.
func (r *appBackend) CreateWorkspace(label, cwd string, focus bool) (workspaceID, paneID string, err error) {
	var before string
	if !focus {
		if st, err := appCall(appBin(), "status"); err == nil {
			var v struct {
				Space string `json:"space"`
			}
			_ = json.Unmarshal(st, &v)
			before = v.Space
		}
	}
	var res struct {
		Space    string `json:"space"`
		Terminal string `json:"terminal"`
	}
	if err := appDo(&res, "space.new", map[string]any{"name": label, "folder": cwd}); err != nil {
		return "", "", err
	}
	r.forget()
	if res.Terminal == "" {
		return res.Space, "", fmt.Errorf("%s", i18n.T("Herdr space.new : pas de terminal dans la réponse", "Herdr space.new: no terminal in the response"))
	}
	if before != "" {
		_ = r.FocusWorkspace(before)
	}
	return res.Space, res.Terminal, nil
}

func (r *appBackend) FirstPane(workspaceID string) (string, error) {
	raw, err := r.rawSnapshot(true)
	if err != nil {
		return "", err
	}
	for _, s := range raw.Spaces {
		if s.ID != workspaceID {
			continue
		}
		for _, tab := range s.Tabs {
			for _, t := range tab.Terminals {
				return t.Terminal, nil
			}
		}
	}
	return "", fmt.Errorf(i18n.T("Herdr : l'espace %s n'a pas de terminal", "Herdr: space %s has no terminal"), workspaceID)
}

func (r *appBackend) FocusWorkspace(id string) error {
	return appDo(nil, "space.select", map[string]any{"id": id})
}

func (r *appBackend) FocusAgent(paneID string) error {
	return appDo(nil, "pane.focus", map[string]any{"terminal": paneID})
}

func (r *appBackend) Runs(paneID, name string) bool {
	t, err := hcGet(paneID)
	if err != nil || t.Foreground == nil {
		return false
	}
	if t.Foreground.Name == name {
		return true
	}
	return len(t.Foreground.Argv) > 0 && filepath.Base(strings.TrimPrefix(t.Foreground.Argv[0], "-")) == name
}

func (r *appBackend) RunInPane(paneID, command string) error {
	_, err := hcRun("send", paneID, command)
	return err
}

// RenameAgent keeps the name as an hc label, and names the agent's tab after
// it so it shows in the window.
func (r *appBackend) RenameAgent(paneID, name string) error {
	if _, err := hcRun("label", paneID, appAgentLabel+"="+name); err != nil {
		return err
	}
	r.forget()
	if t, err := hcGet(paneID); err == nil && t.Labels["herdr.tab"] != "" {
		_ = appDo(nil, "tab.rename", map[string]any{"id": t.Labels["herdr.tab"], "name": name})
	}
	return nil
}

// resolve turns an agent name into its terminal; an address stays as is.
func (r *appBackend) resolve(target string) (string, error) {
	snap, err := r.Snapshot()
	if err != nil {
		return "", err
	}
	for _, a := range snap.Agents {
		if a.PaneID == target || a.Name == target {
			return a.PaneID, nil
		}
	}
	if _, err := hcGet(target); err == nil {
		return target, nil
	}
	return "", fmt.Errorf(i18n.T("Herdr : aucun agent %s", "Herdr: no agent %s"), target)
}

func (r *appBackend) Prompt(target, text string) error {
	pane, err := r.resolve(target)
	if err != nil {
		return err
	}
	return r.RunInPane(pane, text)
}

// StartAgent types the agent's command into the terminal's shell, then waits
// until it is ready for a prompt: it says so itself, or it has run a while
// without saying anything.
func (r *appBackend) StartAgent(name, kind, paneID string, args ...string) error {
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
		t, err := hcGet(paneID)
		if err != nil {
			return err
		}
		if t.State != "live" {
			return fmt.Errorf(i18n.T("Herdr : le terminal de %s s'est fermé", "Herdr: %s's terminal closed"), name)
		}
		if t.Status != nil && agentKinds[t.Status.App] != "" {
			if t.Status.State == "idle" {
				return nil
			}
			continue
		}
		if t.Foreground == nil || isShell(t.Foreground.Name) {
			continue
		}
		if seen.IsZero() {
			seen = time.Now()
		}
		if time.Since(seen) > 8*time.Second {
			return nil // an agent that reports nothing: give it time to draw
		}
	}
	return fmt.Errorf(i18n.T("Herdr : %s n'est pas prêt après 60 s (il attend peut-être une confirmation)", "Herdr: %s is not ready after 60 s (it may be waiting for a confirmation)"), kind)
}

func (r *appBackend) StopAgent(paneID string) error {
	if err := r.RunInPane(paneID, "/exit"); err != nil {
		return err
	}
	for i := 0; i < 60; i++ {
		time.Sleep(250 * time.Millisecond)
		t, err := hcGet(paneID)
		if err != nil {
			return err
		}
		if t.State != "live" || t.Foreground == nil || isShell(t.Foreground.Name) {
			time.Sleep(500 * time.Millisecond) // let the shell draw its prompt
			r.forget()
			return nil
		}
	}
	return fmt.Errorf("%s", i18n.T("l'agent ne s'est pas arrêté (/exit) : ferme-le à la main", "the agent did not stop (/exit): close it by hand"))
}

// ReportToken shows the token in the space's name, as under Rex:
// "YAK-12 · build 2/5". Only cairn's own space and spaces named after a
// ticket are touched; yours keep the name you gave.
func (r *appBackend) ReportToken(workspaceID, value string) error {
	raw, err := r.rawSnapshot(false)
	if err != nil {
		return err
	}
	for _, s := range raw.Spaces {
		if s.ID != workspaceID {
			continue
		}
		base := baseLabel(s.Name)
		if base != "cairn" && !ticketLabel.MatchString(base) {
			return nil
		}
		want := base
		if value != "" {
			want = base + sep + value
		}
		if want == s.Name {
			return nil
		}
		r.forget()
		return appDo(nil, "space.rename", map[string]any{"id": workspaceID, "name": want})
	}
	return nil
}

// Notify uses the system's notifications: the Herdr app shows its own toasts
// only to its mods.
func (r *appBackend) Notify(title, body string) error { return systemNotify(title, body) }

// CloseWorkspace closes a space and the terminals in it.
func (r *appBackend) CloseWorkspace(id string) error {
	r.forget()
	return appDo(nil, "space.close", map[string]any{"id": id, "confirmed": true})
}

// ShowPanel shows cairn's native panel, right of the terminals.
func (r *appBackend) ShowPanel() error {
	if err := appDo(nil, "layout.show", map[string]any{"id": "panel"}); err != nil {
		return err
	}
	return appDo(nil, "layout.select", map[string]any{"stack": "panel-body", "child": "cairn-detail"})
}

// StartEngine runs cmd in a terminal of its own that no space shows.
func (r *appBackend) StartEngine(name string, cmd ...string) error {
	_, err := hcRun(append([]string{"run", "--bg", "--no-parent", "--no-notify", "--name", name, "--"}, cmd...)...)
	return err
}
