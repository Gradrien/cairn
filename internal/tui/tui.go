// Package tui is cairn's screen: every chantier, where it stands, and the
// actions that move it — all on keys shown at the bottom.
package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"cairn/internal/chantier"
	"cairn/internal/config"
	"cairn/internal/herdr"
	"cairn/internal/home"
	"cairn/internal/images"
	"cairn/internal/progress"
	"cairn/internal/prthread"
	"cairn/internal/sprintosaur"
	"cairn/internal/state"
)

const (
	localEvery  = 3 * time.Second
	issuesEvery = 60 * time.Second
	listWidth   = 40
)

var (
	accent  = lipgloss.AdaptiveColor{Light: "#7c3aed", Dark: "#bd93f9"}
	muted   = lipgloss.AdaptiveColor{Light: "#6b7280", Dark: "#6272a4"}
	warn    = lipgloss.AdaptiveColor{Light: "#b45309", Dark: "#ffb86c"}
	danger  = lipgloss.AdaptiveColor{Light: "#b91c1c", Dark: "#ff5555"}
	good    = lipgloss.AdaptiveColor{Light: "#15803d", Dark: "#50fa7b"}
	working = lipgloss.AdaptiveColor{Light: "#a16207", Dark: "#f1fa8c"}

	sTitle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	sMuted = lipgloss.NewStyle().Foreground(muted)
	sWarn  = lipgloss.NewStyle().Foreground(warn).Bold(true)
	sGood  = lipgloss.NewStyle().Foreground(good)
	sBold  = lipgloss.NewStyle().Bold(true)
	// Whole rows are re-rendered from plain text with these, so inner colours
	// never cut the background in pieces.
	sSel    = lipgloss.NewStyle().Bold(true).Background(lipgloss.AdaptiveColor{Light: "#ddd6fe", Dark: "#44475a"}).Foreground(lipgloss.AdaptiveColor{Light: "#1f2937", Dark: "#f8f8f2"})
	sHover  = lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "#f3f0ff", Dark: "#343746"})
	sKey    = lipgloss.NewStyle().Foreground(accent).Bold(true)
	sErr    = lipgloss.NewStyle().Foreground(danger)
	sHeader = lipgloss.NewStyle().Foreground(muted).Bold(true)
)

type Model struct {
	cfg    config.Config
	inside bool // running in cairn's own herdr workspace

	sp        *sprintosaur.Client
	spErr     error
	issues    []sprintosaur.Issue
	extra     map[string]sprintosaur.Issue
	worktrees []herdr.Worktree
	snap      herdr.Snapshot
	localErr  error

	chantiers []*chantier.Chantier
	others    []herdr.Agent
	cursor    int
	hoverItem int // list item under the mouse, -1 for none
	hoverBtn  int // button under the mouse, -1 for none

	// Code and PR progress of chantiers, read on demand for the selection.
	gits     map[string]progress.Git
	prs      map[string]progress.PR
	readAt   map[string]time.Time
	reading  map[string]bool
	detailOf string // chantier the detail pane shows, to reset its scroll
	scroll   int    // detail pane scroll offset

	// The comments view ("" is the main screen).
	view      string
	thread    *prthread.Thread
	threadKey string
	threadErr error
	cscroll   int
	showBots  bool
	tokens    map[string]string

	width, height int
	flash         string
	flashErr      bool
	busy          string
}

func New(cfg config.Config) Model {
	m := Model{cfg: cfg, inside: home.Inside(), hoverItem: -1, hoverBtn: -1,
		gits: map[string]progress.Git{}, prs: map[string]progress.PR{}, readAt: map[string]time.Time{}, reading: map[string]bool{}, tokens: map[string]string{}, extra: map[string]sprintosaur.Issue{}}
	m.sp, m.spErr = sprintosaur.New(cfg.SprintosaurCredentials)
	return m
}

// ── messages ────────────────────────────────────────────────────────────────

type localMsg struct {
	worktrees []herdr.Worktree
	snap      herdr.Snapshot
	err       error
}
type issuesMsg struct {
	issues []sprintosaur.Issue
	extra  map[string]sprintosaur.Issue
	err    error
}
type tickLocal struct{}
type tickIssues struct{}
type doneMsg struct {
	text    string
	err     error
	quit    bool
	refresh bool
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadLocal(), m.loadIssues(), tea.SetWindowTitle("cairn"))
}

func (m Model) loadLocal() tea.Cmd {
	repo := m.cfg.Repo
	return func() tea.Msg {
		snap, err := herdr.GetSnapshot()
		if err != nil {
			return localMsg{err: err}
		}
		wts, err := herdr.ListWorktrees(repo)
		return localMsg{worktrees: wts, snap: snap, err: err}
	}
}

func (m Model) loadIssues() tea.Cmd {
	if m.sp == nil {
		return nil
	}
	sp, cfg := m.sp, m.cfg
	return func() tea.Msg {
		issues, err := sp.CurrentIssues(cfg.Team, cfg.Assignee)
		if err != nil {
			return issuesMsg{err: err}
		}
		// Worktrees whose ticket is not in the current cycle still deserve a title.
		known := map[string]bool{}
		for _, is := range issues {
			known[is.ID] = true
		}
		extra := map[string]sprintosaur.Issue{}
		if wts, err := herdr.ListWorktrees(cfg.Repo); err == nil {
			re := chantier.KeyRe(cfg.Team)
			for _, wt := range wts {
				if mm := re.FindStringSubmatch(wt.Branch); mm != nil {
					key := strings.ToUpper(mm[1])
					if !known[key] && len(extra) < 15 {
						if is, err := sp.Issue(key); err == nil && is.ID != "" {
							extra[key] = is
						}
						known[key] = true
					}
				}
			}
		}
		return issuesMsg{issues: issues, extra: extra}
	}
}

func every(d time.Duration, msg tea.Msg) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return msg })
}

// rebuild recomputes chantiers and pushes changed sidebar tokens to herdr.
func (m *Model) rebuild() tea.Cmd {
	var selKey string
	if c := m.selected(); c != nil {
		selKey = c.Key
	}
	m.chantiers, m.others = chantier.Gather(chantier.Input{
		Team: m.cfg.Team, Repo: m.cfg.Repo, ChantiersDir: config.ChantiersDir(),
		Issues: m.issues, ExtraIssues: m.extra, Worktrees: m.worktrees, Snapshot: m.snap,
	})
	if selKey != "" {
		for i, c := range m.chantiers {
			if c.Key == selKey {
				m.cursor = i
			}
		}
	}
	if n := m.items(); m.cursor >= n {
		m.cursor = max(0, n-1)
	}
	return m.pushTokens()
}

func (m *Model) pushTokens() tea.Cmd {
	want := TokensFor(m.chantiers, m.snap)
	var cmds []tea.Cmd
	for ws, v := range want {
		if m.tokens[ws] != v {
			m.tokens[ws] = v
			ws, v := ws, v
			cmds = append(cmds, func() tea.Msg { _ = herdr.ReportToken(ws, v); return nil })
		}
	}
	return tea.Batch(cmds...)
}

// TokensFor maps each herdr workspace to its chantier's sidebar token, and
// cairn's own workspace to a summary: the sidebar button says if you are needed.
func TokensFor(list []*chantier.Chantier, snap herdr.Snapshot) map[string]string {
	out := map[string]string{}
	if ws := snap.WorkspaceByLabel(home.Label); ws != nil {
		waiting := 0
		for _, c := range list {
			if c.Waiting() != "" {
				waiting++
			}
		}
		switch {
		case waiting == 1:
			out[ws.ID] = "⚠ 1 attend ta décision"
		case waiting > 1:
			out[ws.ID] = fmt.Sprintf("⚠ %d attendent ta décision", waiting)
		default:
			out[ws.ID] = fmt.Sprintf("%d chantiers", len(list))
		}
	}
	for _, c := range list {
		for _, ws := range c.Workspaces {
			if _, taken := out[ws]; !taken {
				out[ws] = c.Token()
			}
		}
	}
	return out
}

// ── update ──────────────────────────────────────────────────────────────────

func (m Model) items() int { return len(m.chantiers) + len(m.others) }

func (m Model) selected() *chantier.Chantier {
	if m.cursor < len(m.chantiers) {
		return m.chantiers[m.cursor]
	}
	return nil
}

func (m Model) selectedOther() *herdr.Agent {
	i := m.cursor - len(m.chantiers)
	if i >= 0 && i < len(m.others) {
		return &m.others[i]
	}
	return nil
}

func (m *Model) say(text string, err error) {
	m.flash, m.flashErr = text, err != nil
	if err != nil {
		m.flash = err.Error()
	}
}

type progressMsg struct {
	key string
	git *progress.Git
	pr  *progress.PR
}

const progressEvery = 30 * time.Second

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if pm, ok := msg.(progressMsg); ok {
		delete(m.reading, pm.key)
		m.readAt[pm.key] = time.Now()
		if pm.git != nil {
			m.gits[pm.key] = *pm.git
		}
		if pm.pr != nil {
			m.prs[pm.key] = *pm.pr
		}
		return m, nil
	}
	next, cmd := m.update(msg)
	nm := next.(Model)
	return nm, tea.Batch(cmd, nm.readProgress())
}

// readProgress reads git and PR state of the selected chantier when stale.
// It mutates maps shared by every copy of the model, which is what we want.
func (m *Model) readProgress() tea.Cmd {
	c := m.selected()
	if c == nil {
		return nil
	}
	if c.Key != m.detailOf {
		m.detailOf, m.scroll = c.Key, 0
	}
	key, wt, base := c.Key, c.Worktree, m.cfg.Base
	var prURL string
	if c.Issue != nil && len(c.Issue.PullRequests) > 0 {
		prURL = c.Issue.PullRequests[len(c.Issue.PullRequests)-1].URL
		if pr := c.OpenPR(); pr != nil {
			prURL = pr.URL
		}
	}
	// Read again when stale, or when a PR or worktree showed up since (the
	// first read can happen before Sprintosaur answered).
	_, hasPR := m.prs[key]
	_, hasGit := m.gits[key]
	missing := (prURL != "" && !hasPR) || (wt != "" && !hasGit)
	if m.reading[key] || (!missing && time.Since(m.readAt[key]) < progressEvery) {
		return nil
	}
	m.reading[key] = true
	return func() tea.Msg {
		msg := progressMsg{key: key}
		if wt != "" {
			g := progress.ReadGit(wt, base)
			msg.git = &g
		}
		if prURL != "" {
			pr := progress.ReadPR(prURL)
			msg.pr = &pr
		}
		return msg
	}
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case threadMsg:
		if msg.key == m.threadKey {
			m.thread, m.threadErr = msg.thread, msg.err
		}
		return m, nil
	case imagesMsg:
		m.busy = ""
		if msg.err != nil {
			m.say("", msg.err)
		}
		if len(msg.files) == 0 {
			return m, nil
		}
		return m, m.showImages(msg)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case localMsg:
		m.localErr = msg.err
		if msg.err == nil {
			m.worktrees, m.snap = msg.worktrees, msg.snap
		}
		return m, tea.Batch(m.rebuild(), every(localEvery, tickLocal{}))

	case issuesMsg:
		m.spErr = msg.err
		if msg.err == nil {
			m.issues, m.extra = msg.issues, msg.extra
		}
		return m, tea.Batch(m.rebuild(), every(issuesEvery, tickIssues{}))

	case tickLocal:
		return m, tea.Batch(m.loadLocal(), tea.EnableMouseAllMotion)
	case tickIssues:
		return m, m.loadIssues()

	case doneMsg:
		m.busy = ""
		m.say(msg.text, msg.err)
		if msg.quit && msg.err == nil {
			return m, tea.Quit
		}
		if msg.refresh {
			return m, tea.Batch(m.loadLocal(), m.loadIssues())
		}
		return m, nil

	case tea.KeyMsg:
		return m.key(msg)
	case tea.MouseMsg:
		return m.mouse(msg)
	case tea.FocusMsg:
		// herdr may drop the pane's mouse mode while another workspace is in
		// front: ask for it again whenever cairn comes back.
		return m, tea.Batch(tea.EnableMouseAllMotion, m.loadLocal())
	case tea.BlurMsg:
		m.hoverItem, m.hoverBtn = -1, -1
		return m, nil
	}
	return m, nil
}

func (m Model) key(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.busy != "" && k.String() != "ctrl+c" {
		return m, nil
	}
	if m.view == "comments" {
		return m.commentsKey(k)
	}
	m.flash = ""
	switch k.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q", "esc":
		if m.inside {
			// cairn stays alive in its workspace; q just hands herdr back.
			return m, func() tea.Msg { return doneMsg{text: "← retour à herdr", err: home.Back()} }
		}
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < m.items()-1 {
			m.cursor++
		}
	case "r":
		if c := m.selected(); c != nil {
			delete(m.readAt, c.Key)
		}
		m.say("Rafraîchi.", nil)
		return m, tea.Batch(m.loadLocal(), m.loadIssues())
	case "enter":
		return m, m.goTo()
	case "d":
		return m, m.browsediff()
	case "o":
		if c := m.selected(); c != nil && c.Issue != nil {
			url := m.cfg.SprintosaurWeb + c.Key
			return m, func() tea.Msg { return doneMsg{text: "Ticket ouvert.", err: exec.Command("open", url).Run()} }
		}
	case "c":
		return m.openComments()
	case "p":
		return m.plan()
	case "v":
		return m.validate()
	case "a":
		return m.allowPush()
	}
	return m, nil
}

// Screen rows: 0 header, 1 flash, 2…2+bodyH-1 body, then the button row.
func (m Model) bodyHeight() int { return max(3, m.height-4) }

func (m Model) mouse(ev tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.view == "comments" {
		return m.commentsMouse(ev)
	}
	bodyH := m.bodyHeight()
	inList := ev.X < listWidth
	if ev.Action == tea.MouseActionMotion {
		m.hoverItem, m.hoverBtn = m.hitTest(ev.X, ev.Y)
		return m, nil
	}
	if m.busy != "" {
		return m, nil
	}
	switch ev.Button {
	case tea.MouseButtonWheelUp:
		if inList && m.cursor > 0 {
			m.cursor--
		} else if !inList && m.scroll > 0 {
			m.scroll -= 2
			m.scroll = max(0, m.scroll)
		}
		return m, nil
	case tea.MouseButtonWheelDown:
		if inList && m.cursor < m.items()-1 {
			m.cursor++
		} else if !inList {
			lines, _ := m.detailLayout(m.detailWidth())
			m.scroll = min(m.scroll+2, max(0, len(lines)-bodyH))
		}
		return m, nil
	}
	if ev.Button != tea.MouseButtonLeft || ev.Action != tea.MouseActionPress {
		return m, nil
	}
	row := ev.Y - 2
	switch {
	case ev.Y == 2+bodyH: // button row
		_, ranges := m.footerLayout()
		for i, b := range m.buttons() {
			if b.enabled && ev.X >= ranges[i][0] && ev.X < ranges[i][1] {
				return m.key(keyFor(b.key))
			}
		}
	case row >= 0 && row < bodyH && inList:
		_, owners := m.listLayout(listWidth, bodyH)
		if row < len(owners) && owners[row] >= 0 {
			if owners[row] == m.cursor {
				m.flash = ""
				return m, m.goTo() // second click on the selection: go there
			}
			m.cursor = owners[row]
			m.flash = ""
		}
	case row >= 0 && row < bodyH && ev.X >= listWidth+3:
		lines, targets := m.detailLayout(m.detailWidth())
		row += min(m.scroll, max(0, len(lines)-bodyH))
		if row < len(targets) && targets[row] != "" {
			return m.detailClick(targets[row])
		}
	}
	return m, nil
}

// hitTest returns the list item and the enabled button under (x, y), -1 if none.
func (m Model) hitTest(x, y int) (item, btn int) {
	item, btn = -1, -1
	bodyH := m.bodyHeight()
	if y == 2+bodyH {
		_, ranges := m.footerLayout()
		for i, b := range m.buttons() {
			if b.enabled && x >= ranges[i][0] && x < ranges[i][1] {
				btn = i
			}
		}
		return
	}
	if row := y - 2; row >= 0 && row < bodyH && x < listWidth {
		_, owners := m.listLayout(listWidth, bodyH)
		if row < len(owners) {
			item = owners[row]
		}
	}
	return
}

// detailClick acts on a detail-pane target: "pane:<id>", "img:<path>", "comments".
func (m Model) detailClick(target string) (tea.Model, tea.Cmd) {
	kind, arg, _ := strings.Cut(target, ":")
	switch kind {
	case "pane":
		return m, func() tea.Msg { return doneMsg{text: "→ " + arg, err: herdr.FocusAgent(arg)} }
	case "comments":
		return m.openComments()
	case "img":
		files := m.proofImages()
		start := 0
		for i, f := range files {
			if f == arg {
				start = i
			}
		}
		caps := make([]string, len(files))
		for i, f := range files {
			caps[i] = "preuve · " + tilde(f)
		}
		m.busy = "Ouverture des captures…"
		return m, openImages(files, caps, images.Source{}, start)
	}
	return m, nil
}

func keyFor(k string) tea.KeyMsg {
	switch k {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	}
	return keyForRunes(k)
}

func keyForRunes(k string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

func (m Model) goTo() tea.Cmd {
	if a := m.selectedOther(); a != nil {
		pane := a.PaneID
		return func() tea.Msg { return doneMsg{text: "→ " + a.Label(), err: herdr.FocusAgent(pane)} }
	}
	c := m.selected()
	if c == nil {
		return nil
	}
	// Prefer the agent that needs you, then one that works, then any.
	var target *herdr.Agent
	for _, want := range []string{"blocked", "done", "idle", "working", ""} {
		for i, a := range c.Agents {
			if target == nil && (want == "" || a.Status == want) {
				target = &c.Agents[i]
			}
		}
	}
	if target != nil {
		pane, label := target.PaneID, target.Label()
		return func() tea.Msg { return doneMsg{text: "→ " + label, err: herdr.FocusAgent(pane)} }
	}
	if len(c.Workspaces) > 0 {
		ws := c.Workspaces[0]
		return func() tea.Msg { return doneMsg{text: "→ " + c.Key, err: herdr.FocusWorkspace(ws)} }
	}
	return func() tea.Msg { return doneMsg{text: c.Key + " n'a pas encore d'espace herdr : p pour le lancer."} }
}

func (m Model) browsediff() tea.Cmd {
	c := m.selected()
	if c == nil || c.Worktree == "" {
		return func() tea.Msg { return doneMsg{text: "Pas de worktree pour ce chantier."} }
	}
	if _, err := exec.LookPath("browsediff"); err != nil {
		return func() tea.Msg { return doneMsg{err: fmt.Errorf("browsediff n'est pas installé")} }
	}
	cmd := exec.Command("browsediff", m.cfg.Base+"...HEAD")
	cmd.Dir = c.Worktree
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return doneMsg{err: err} })
}

func (m Model) plan() (tea.Model, tea.Cmd) {
	c := m.selected()
	switch {
	case c == nil:
		return m, nil
	case c.Issue == nil:
		m.say("Ticket introuvable dans Sprintosaur.", nil)
		return m, nil
	case c.Worktree != "" && !c.Main:
		m.say(c.Key+" a déjà son worktree : enter pour y aller.", nil)
		return m, nil
	}
	cfg, issue, agents := m.cfg, *c.Issue, m.snap.Agents
	key := strings.ToUpper(issue.ID)
	lower := strings.ToLower(key)
	_, num, _ := strings.Cut(lower, "-")
	branch := cfg.BranchPrefix + lower + "-" + chantier.Slug(issue.Title, 50)
	path := filepath.Join(cfg.WorktreeParent, filepath.Base(cfg.Repo)+"-"+num)
	statePath := state.PathFor(config.ChantiersDir(), key)
	m.busy = "Lancement de " + key + "…"
	return m, func() tea.Msg {
		if _, err := os.Stat(path); err == nil {
			return doneMsg{err: fmt.Errorf("%s existe déjà", path)}
		}
		_ = exec.Command("git", "-C", cfg.Repo, "fetch", "--quiet", "origin").Run()
		_, pane, err := herdr.CreateWorktree(cfg.Repo, branch, cfg.Base, path, key)
		if err != nil {
			return doneMsg{err: err}
		}
		if s, _ := state.Load(statePath); s == nil {
			if err := state.New(statePath, key, issue.Title).Save(); err != nil {
				return doneMsg{err: err}
			}
		}
		name := herdr.FreeName(lower, agents)
		if err := herdr.StartAgent(name, cfg.AgentKind, pane); err != nil {
			return doneMsg{err: fmt.Errorf("worktree prêt, mais l'agent n'a pas démarré : %w", err), refresh: true}
		}
		err = herdr.Prompt(name, chantier.PlanPrompt(key, issue.Title, statePath))
		return doneMsg{text: key + " : l'agent " + name + " écrit le plan.", err: err, refresh: true}
	}
}

// allowPush lets the agent push once: the guard hook consumes the allowance.
func (m Model) allowPush() (tea.Model, tea.Cmd) {
	c := m.selected()
	if c == nil || c.State == nil {
		m.say("Ce chantier n'a pas de plan cairn : ses push ne sont pas bloqués.", nil)
		return m, nil
	}
	s := c.State
	s.Set("push", "autorisé")
	if isPushWait(s.Waiting) {
		s.Set("attente", "")
	}
	if err := s.Save(); err != nil {
		m.say("", err)
		return m, nil
	}
	text := "Push autorisé pour " + c.Key + " (un seul push)."
	cmd := m.rebuild()
	if len(c.Agents) > 0 {
		pane := c.Agents[0].PaneID
		prompt := "Push autorisé dans cairn pour " + c.Key + " : pousse maintenant, et ouvre la PR si elle n'existe pas encore " +
			"(puis lie-la au ticket). L'autorisation vaut pour un seul push : pour les changements suivants, redemande avec `attente: Prêt à pousser`."
		cmd = tea.Batch(cmd, func() tea.Msg { return doneMsg{text: text + " Agent relancé.", err: herdr.Prompt(pane, prompt)} })
	}
	m.say(text, nil)
	return m, cmd
}

func isPushWait(w string) bool {
	l := strings.ToLower(w)
	return strings.Contains(l, "pousser") || strings.Contains(l, "push")
}

func (m Model) validate() (tea.Model, tea.Cmd) {
	c := m.selected()
	if c == nil || c.State == nil || c.State.Waiting == "" {
		m.say("Rien à valider ici.", nil)
		return m, nil
	}
	s := c.State
	waiting := s.Waiting
	s.Set("attente", "")
	text := "Validé : " + waiting
	var prompt string
	if s.Phase == "plan" {
		s.Set("phase", "build")
		prompt = chantier.BuildPrompt(s.Path)
		text = "Plan validé : " + c.Key + " passe en Build."
	}
	if err := s.Save(); err != nil {
		m.say("", err)
		return m, nil
	}
	var agent string
	for _, a := range c.Agents {
		agent = a.PaneID
		break
	}
	m.say(text, nil)
	cmd := m.rebuild()
	if prompt != "" && agent != "" {
		cmd = tea.Batch(cmd, func() tea.Msg { return doneMsg{text: text + " Agent relancé.", err: herdr.Prompt(agent, prompt)} })
	}
	return m, cmd
}

// ── view ────────────────────────────────────────────────────────────────────

func trunc(s string, w int) string {
	if w <= 1 {
		return ""
	}
	return runewidth.Truncate(s, w, "…")
}

func agentIcon(status string) string {
	switch status {
	case "working":
		return lipgloss.NewStyle().Foreground(working).Render("◐")
	case "blocked":
		return sWarn.Render("⚠")
	case "done":
		return sGood.Render("✓")
	case "idle":
		return sMuted.Render("○")
	}
	return sMuted.Render("?")
}

func phaseStyle(p chantier.Phase) lipgloss.Style {
	switch p {
	case chantier.Review:
		return lipgloss.NewStyle().Foreground(accent)
	case chantier.Build:
		return lipgloss.NewStyle().Foreground(working)
	case chantier.Shipped:
		return sGood
	}
	return sMuted
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	if m.view == "comments" {
		return m.commentsView()
	}
	header := sTitle.Render("cairn") + sMuted.Render(fmt.Sprintf("  %s · %s", m.cfg.Team, m.cfg.Assignee))
	if is := m.cycle(); is != "" {
		header += sMuted.Render(" · cycle " + is)
	}
	switch {
	case m.localErr != nil:
		header += "  " + sErr.Render(trunc("herdr : "+m.localErr.Error(), m.width/2))
	case m.spErr != nil:
		header += "  " + sErr.Render(trunc("Sprintosaur : "+m.spErr.Error(), m.width/2))
	}
	bodyH := m.bodyHeight()
	left := m.viewList(listWidth, bodyH)
	right := m.viewDetail(m.detailWidth(), bodyH)
	sep := sMuted.Render(strings.TrimSuffix(strings.Repeat(" │ \n", bodyH), "\n"))
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, sep, right)
	return header + "\n" + m.viewFlash() + "\n" + body + "\n" + m.viewFooter()
}

func (m Model) cycle() string {
	for _, is := range m.issues {
		if is.Cycle != nil {
			return fmt.Sprint(is.Cycle.Number)
		}
	}
	return ""
}

// listLayout renders the list and says which item each visible line belongs
// to (-1 for headers), so clicks map back to items.
func (m Model) listLayout(w, h int) ([]string, []int) {
	var lines []string
	var owners []int
	curLine := 0
	add := func(item int, s ...string) {
		for _, l := range s {
			switch {
			case item >= 0 && item == m.cursor:
				l = sSel.Width(w).Render(ansi.Strip(l))
			case item >= 0 && item == m.hoverItem:
				l = sHover.Width(w).Render(ansi.Strip(l))
			}
			lines = append(lines, l)
			owners = append(owners, item)
		}
	}
	if len(m.chantiers) == 0 {
		add(-1, sMuted.Render(trunc("Chargement…", w)))
	}
	waitingHdr, restHdr := false, false
	for i, c := range m.chantiers {
		if c.Waiting() != "" && !waitingHdr {
			add(-1, sWarn.Render("Attend ta décision"))
			waitingHdr = true
		}
		if c.Waiting() == "" && !restHdr {
			if waitingHdr {
				add(-1, "")
			}
			add(-1, sHeader.Render("Chantiers"))
			restHdr = true
		}
		mark := " "
		if c.Waiting() != "" {
			mark = sWarn.Render("⚠")
		}
		l1 := mark + " " + sBold.Render(c.Key) + " " + trunc(c.Title(), w-len(c.Key)-3)
		info := phaseStyle(c.Phase).Render(c.Phase.String())
		if c.State != nil && len(c.State.Tasks) > 0 {
			info += sMuted.Render(fmt.Sprintf(" %d/%d", c.State.DoneCount(), len(c.State.Tasks)))
		}
		if len(c.Agents) > 0 {
			icons := ""
			for _, a := range c.Agents {
				icons += agentIcon(a.Status)
			}
			info += sMuted.Render(" · ") + icons
		}
		if pr := c.OpenPR(); pr != nil {
			info += sMuted.Render(fmt.Sprintf(" · PR #%d", pr.Number))
		}
		if i == m.cursor {
			curLine = len(lines)
		}
		add(i, l1, "    "+info)
	}
	if len(m.others) > 0 {
		add(-1, "", sHeader.Render("Hors chantier"))
		for j, a := range m.others {
			l := agentIcon(a.Status) + " " + trunc(a.Title, w-3)
			if a.Title == "" {
				l = agentIcon(a.Status) + " " + a.Label()
			}
			if len(m.chantiers)+j == m.cursor {
				curLine = len(lines)
			}
			add(len(m.chantiers)+j, l)
		}
	}
	// Keep the cursor in view.
	start := 0
	if curLine+2 > h {
		start = curLine + 2 - h
	}
	lines, owners = lines[start:], owners[start:]
	if len(lines) > h {
		lines, owners = lines[:h], owners[:h]
	}
	return lines, owners
}

func (m Model) viewList(w, h int) string {
	lines, _ := m.listLayout(w, h)
	return lipgloss.NewStyle().Width(w).Height(h).Render(strings.Join(lines, "\n"))
}

func (m Model) detailWidth() int { return max(20, m.width-listWidth-3) }

func (m Model) viewDetail(w, h int) string {
	lines, _ := m.detailLayout(w)
	start := min(m.scroll, max(0, len(lines)-h))
	lines = lines[start:]
	if len(lines) > h {
		lines = lines[:h]
		lines[h-1] = sMuted.Render("  ↓ suite (molette)")
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, w, "…") // never let a line wrap
	}
	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(strings.Join(lines, "\n"))
}

// wrap breaks text to width w and indents every line.
func wrap(text string, w int, indent string) []string {
	out := strings.Split(lipgloss.NewStyle().Width(max(10, w-len(indent))).Render(text), "\n")
	for i := range out {
		out[i] = indent + strings.TrimRight(out[i], " ")
	}
	return out
}

func bar(done, doing, total, width int) string {
	if total == 0 {
		return ""
	}
	d := done * width / total
	g := min(width-d, (doing*width+total-1)/total)
	return sGood.Render(strings.Repeat("█", d)) +
		lipgloss.NewStyle().Foreground(working).Render(strings.Repeat("▓", g)) +
		sMuted.Render(strings.Repeat("░", width-d-g))
}

func taskIcon(t state.Task) string {
	switch {
	case t.Done:
		return sGood.Render("✓")
	case t.Doing:
		return lipgloss.NewStyle().Foreground(working).Render("◐")
	}
	return sMuted.Render("○")
}

const label = 10 // width of the left labels in the detail pane

func lbl(s string) string {
	return sMuted.Render(s + strings.Repeat(" ", max(1, label-runewidth.StringWidth(s))))
}

// detailLayout renders the detail pane and the pane each line jumps to on click.
func (m Model) detailLayout(w int) ([]string, []string) {
	var b, targets []string
	add := func(s ...string) {
		b = append(b, s...)
		for range s {
			targets = append(targets, "")
		}
	}
	section := func(title string) { add("", sHeader.Render(title)) }

	if a := m.selectedOther(); a != nil {
		add(sBold.Render(a.Label())+" "+agentIcon(a.Status)+" "+a.Status, "",
			lbl("Titre")+trunc(a.Title, w-label),
			lbl("Dossier")+trunc(tilde(a.Dir()), w-label),
			lbl("Pane")+a.PaneID, "",
			sMuted.Render("Cet agent ne travaille sur aucun ticket connu."))
		return b, targets
	}
	c := m.selected()
	if c == nil {
		return m.statusLines(w), nil
	}
	s := c.State

	// Title and ticket.
	title := wrap(c.Title(), w-len(c.Key)-2, "")
	add(sBold.Render(c.Key) + "  " + title[0])
	for _, l := range title[1:] {
		add(strings.Repeat(" ", len(c.Key)+2) + l)
	}
	if c.Issue != nil {
		meta := []string{c.Issue.Status}
		if c.Issue.Priority != "" && c.Issue.Priority != "No priority" {
			meta = append(meta, c.Issue.Priority)
		}
		if c.Issue.EstimateLabel != "" {
			meta = append(meta, c.Issue.EstimateLabel)
		}
		add(sMuted.Render("Sprintosaur · " + strings.Join(meta, " · ")))
	}
	add("")

	// Where it stands.
	var flow []string
	for _, p := range chantier.Flow {
		switch {
		case p < c.Phase:
			flow = append(flow, sGood.Render(p.String()+" ✓"))
		case p == c.Phase:
			flow = append(flow, phaseStyle(p).Bold(true).Render(p.String()+" ●"))
		default:
			flow = append(flow, sMuted.Render(p.String()))
		}
	}
	if c.Phase == chantier.ToPlan {
		add(lbl("Phase") + "À planifier : bouton " + sKey.Render("Planifier") + " pour lancer un agent")
	} else {
		add(lbl("Phase") + strings.Join(flow, sMuted.Render(" → ")))
	}
	var activity []string
	if s != nil && !s.UpdatedAt.IsZero() {
		activity = append(activity, "plan "+progress.Ago(s.UpdatedAt))
	}
	if g, ok := m.gits[c.Key]; ok && g.Ahead > 0 && !g.LastAt.IsZero() {
		activity = append(activity, "commit "+progress.Ago(g.LastAt))
	}
	if len(activity) > 0 {
		add(lbl("Activité") + strings.Join(activity, sMuted.Render(" · ")))
	}
	if s != nil {
		if s.PushAllowed() {
			add(lbl("Push") + sGood.Render("autorisé") + sMuted.Render(" (le prochain push seulement)"))
		} else {
			add(lbl("Push") + sMuted.Render("bloqué jusqu'à ton feu vert"))
		}
	}
	if wt := c.Waiting(); wt != "" {
		hint := ""
		if s != nil && s.Waiting != "" {
			hint = sMuted.Render("  → bouton Valider")
			if isPushWait(s.Waiting) {
				hint = sMuted.Render("  → relis le Diff, puis Autoriser le push")
			}
		}
		add(lbl("Attente") + sWarn.Render("⚠ "+wt) + hint)
	}

	// The plan.
	if s == nil && c.Phase != chantier.ToPlan {
		add("", sMuted.Render("Pas de plan cairn pour ce chantier : seuls le code et la PR sont suivis."))
	}
	if s != nil && len(s.Summary) > 0 {
		section("Objectif")
		lines := wrap(plainMD(strings.Join(s.Summary, " ")), w, "  ")
		if len(lines) > 6 {
			lines = append(lines[:5], "  "+sMuted.Render("…"))
		}
		add(lines...)
	}
	if s != nil {
		if q := s.OpenQuestions(); len(q) > 0 {
			section(fmt.Sprintf("Questions ouvertes (%d)", len(q)))
			for _, t := range q {
				add(sWarn.Render("  ? ") + trunc(plainMD(t), w-4))
			}
		}
	}
	if s != nil && len(s.Tasks) > 0 {
		done, doing := state.Count(s.Tasks, true), state.Count(s.Tasks, false)
		head := fmt.Sprintf("Tâches %d/%d", done, len(s.Tasks))
		if doing > 0 {
			head += fmt.Sprintf(" · %d en cours", doing)
		}
		add("", sHeader.Render(head)+"  "+bar(done, doing, len(s.Tasks), 20))
		for _, t := range s.Tasks {
			add("  " + taskIcon(t) + " " + trunc(plainMD(t.Text), w-4))
		}
	}
	if s != nil && len(s.SideEffects) > 0 {
		done := state.Count(s.SideEffects, true)
		section(fmt.Sprintf("Effets de bord %d/%d vérifiés", done, len(s.SideEffects)))
		for _, t := range s.SideEffects {
			add("  " + taskIcon(t) + " " + trunc(plainMD(t.Text), w-4))
		}
	} else if s != nil && c.Phase >= chantier.Build {
		add("", sWarn.Render("Effets de bord : aucun recensé dans le plan"))
	}
	if s != nil && len(s.Proofs) > 0 {
		section(fmt.Sprintf("Preuves (%d)", len(s.Proofs)))
		for _, p := range s.Proofs {
			if f := imagePath(p); f != "" {
				add("  " + sImg.Render("[img]") + " " + trunc(p, w-8))
				targets[len(targets)-1] = "img:" + f
				continue
			}
			add("  • " + trunc(p, w-4))
		}
	}

	// Code and pull request.
	if c.Worktree != "" {
		section("Code")
		if g, ok := m.gits[c.Key]; ok {
			if g.Err != "" {
				add("  " + sErr.Render(g.Err))
			} else {
				line := sMuted.Render("aucun commit sur la branche")
				if g.Ahead > 0 {
					line = plural(g.Ahead, "commit", "commits") + " · " + plural(g.Files, "fichier", "fichiers") + " " +
						sGood.Render(fmt.Sprintf("+%d", g.Added)) + " " + sErr.Render(fmt.Sprintf("−%d", g.Removed))
				}
				if g.Uncommitted > 0 {
					line += sWarn.Render(" · " + plural(g.Uncommitted, "changement non commité", "changements non commités"))
				}
				add("  " + line)
				if g.Ahead > 0 && g.LastSubject != "" {
					add("  " + sMuted.Render("dernier : ") + trunc(g.LastSubject, w-14))
				}
			}
		} else {
			add("  " + sMuted.Render("lecture…"))
		}
		add("  "+sMuted.Render("branche ")+trunc(c.Branch, w-10), "  "+sMuted.Render("dossier ")+trunc(tilde(c.Worktree), w-10))
	}
	if c.Issue != nil && len(c.Issue.PullRequests) > 0 {
		pr := c.Issue.PullRequests[len(c.Issue.PullRequests)-1]
		if o := c.OpenPR(); o != nil {
			pr = *o
		}
		section(fmt.Sprintf("PR #%d", pr.Number))
		if info, ok := m.prs[c.Key]; ok && info.Err == "" {
			add("  " + prLine(info))
			if len(info.Checks.FailedNames) > 0 {
				add("  " + sErr.Render("✗ ") + trunc(strings.Join(info.Checks.FailedNames, ", "), w-6))
			}
		} else if ok {
			add("  " + pr.State + sMuted.Render(" · "+info.Err))
		} else {
			add("  " + pr.State + sMuted.Render(" · lecture…"))
		}
		add("  " + sImg.Render("› Commentaires et images") + sMuted.Render("  (clic ou c)"))
		targets[len(targets)-1] = "comments"
	}

	if len(c.Agents) > 0 {
		section("Agents" + sMuted.Render("  (clic pour y aller)"))
		for _, a := range c.Agents {
			add("  " + agentIcon(a.Status) + " " + sBold.Render(a.Label()) + sMuted.Render(" "+a.Status+" · ") + trunc(a.Title, w-len(a.Label())-len(a.Status)-9))
			targets[len(targets)-1] = "pane:" + a.PaneID
		}
	}
	if s != nil {
		add("", sMuted.Render("plan : "+tilde(s.Path)))
	}
	return b, targets
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// plainMD drops the markdown marks that only add noise in a terminal.
func plainMD(s string) string {
	return strings.NewReplacer("**", "", "`", "", "__", "").Replace(s)
}

func prLine(p progress.PR) string {
	var parts []string
	switch {
	case p.State == "MERGED":
		parts = append(parts, sGood.Render("mergée"))
	case p.State == "CLOSED":
		parts = append(parts, sMuted.Render("fermée"))
	case p.Draft:
		parts = append(parts, sMuted.Render("brouillon"))
	default:
		parts = append(parts, "ouverte")
	}
	ck := p.Checks
	switch {
	case ck.Failed > 0:
		parts = append(parts, sErr.Render(fmt.Sprintf("CI ✗ %d en échec", ck.Failed)))
	case ck.Pending > 0:
		parts = append(parts, lipgloss.NewStyle().Foreground(working).Render(fmt.Sprintf("CI ◐ %d en cours", ck.Pending)))
	case ck.Passed > 0:
		parts = append(parts, sGood.Render(fmt.Sprintf("CI ✓ %d", ck.Passed)))
	}
	switch p.ReviewDecision {
	case "APPROVED":
		parts = append(parts, sGood.Render("review approuvée"))
	case "CHANGES_REQUESTED":
		parts = append(parts, sErr.Render("changements demandés"))
	default:
		if p.State == "OPEN" {
			parts = append(parts, sMuted.Render("review en attente"))
		}
	}
	if p.State == "OPEN" {
		switch p.Mergeable {
		case "MERGEABLE":
			parts = append(parts, sGood.Render("mergeable"))
		case "CONFLICTING":
			parts = append(parts, sErr.Render("conflits"))
		}
	}
	if p.Comments > 0 {
		parts = append(parts, sMuted.Render(fmt.Sprintf("%d commentaires", p.Comments)))
	}
	return strings.Join(parts, sMuted.Render(" · "))
}

func (m Model) statusLines(w int) []string {
	var b []string
	if m.localErr != nil {
		b = append(b, sErr.Render(trunc(m.localErr.Error(), w)))
	}
	if m.spErr != nil {
		b = append(b, sErr.Render(trunc(m.spErr.Error(), w)))
	}
	return b
}

type button struct {
	key, label string
	enabled    bool
}

// buttons are the actions for the current selection; each is also a key.
func (m Model) buttons() []button {
	if m.view == "comments" {
		return m.commentsButtons()
	}
	return m.mainButtons()
}

func (m Model) mainButtons() []button {
	c := m.selected()
	other := m.selectedOther() != nil
	hasWT := c != nil && c.Worktree != ""
	back := "Quitter"
	if m.inside {
		back = "Retour à herdr"
	}
	return []button{
		{"enter", "Aller à l'agent", other || (c != nil && (len(c.Agents) > 0 || len(c.Workspaces) > 0))},
		{"d", "Diff", hasWT},
		{"p", "Planifier", c != nil && c.Issue != nil && (!hasWT || c.Main)},
		{"v", "Valider", c != nil && c.State != nil && c.State.Waiting != "" && !isPushWait(c.State.Waiting)},
		{"a", "Autoriser le push", c != nil && c.State != nil && !c.State.PushAllowed()},
		{"c", "Commentaires", c != nil && c.Issue != nil && len(c.Issue.PullRequests) > 0},
		{"o", "Ticket", c != nil && c.Issue != nil},
		{"r", "Rafraîchir", true},
		{"q", back, true},
	}
}

var (
	btnBg      = lipgloss.AdaptiveColor{Light: "#ede9fe", Dark: "#44475a"}
	btnHoverBg = lipgloss.AdaptiveColor{Light: "#c4b5fd", Dark: "#6272a4"}
	btnFg      = lipgloss.AdaptiveColor{Light: "#1f2937", Dark: "#f8f8f2"}
	sBtnOff    = lipgloss.NewStyle().Padding(0, 1).Foreground(muted)
)

func renderButton(b button, hover bool) string {
	key := b.key
	switch key {
	case "enter":
		key = "↵"
	case "esc":
		key = "⎋"
	}
	if !b.enabled {
		return sBtnOff.Render(key + " " + b.label)
	}
	bg := btnBg
	if hover {
		bg = btnHoverBg
	}
	k := lipgloss.NewStyle().Background(bg).Foreground(accent).Bold(true).Render(" " + key)
	l := lipgloss.NewStyle().Background(bg).Foreground(btnFg).Render(" " + b.label + " ")
	return k + l
}

// footerLayout renders the button row and the x-range of each button.
func (m Model) footerLayout() (string, [][2]int) {
	var parts []string
	var ranges [][2]int
	x := 0
	for i, b := range m.buttons() {
		r := renderButton(b, i == m.hoverBtn)
		wd := lipgloss.Width(r)
		parts = append(parts, r)
		ranges = append(ranges, [2]int{x, x + wd})
		x += wd + 1
	}
	return lipgloss.NewStyle().MaxWidth(m.width).Render(strings.Join(parts, " ")), ranges
}

func (m Model) viewFooter() string {
	f, _ := m.footerLayout()
	return f
}

func (m Model) viewFlash() string {
	switch {
	case m.busy != "":
		return sWarn.Render(trunc(m.busy, m.width))
	case m.flash != "" && m.flashErr:
		return sErr.Render(trunc(m.flash, m.width))
	case m.flash != "":
		return sGood.Render(trunc(m.flash, m.width))
	}
	return ""
}

func tilde(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
