// Package tui is cairn's screen: every chantier, where it stands, and the
// actions that move it — all on keys shown at the bottom.
package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cairn/internal/chantier"
	"cairn/internal/config"
	"cairn/internal/herdr"
	"cairn/internal/home"
	"cairn/internal/i18n"
	"cairn/internal/images"
	"cairn/internal/journal"
	"cairn/internal/localtest"
	"cairn/internal/progress"
	"cairn/internal/prthread"
	"cairn/internal/state"
	"cairn/internal/update"
)

const (
	localEvery  = 3 * time.Second
	issuesEvery = 60 * time.Second
)

var (
	accent  = lipgloss.AdaptiveColor{Light: "#7c3aed", Dark: "#bd93f9"}
	muted   = lipgloss.AdaptiveColor{Light: "#4b5563", Dark: "#8b93c0"} // secondary text, readable
	dim     = lipgloss.AdaptiveColor{Light: "#9ca3af", Dark: "#6272a4"} // marks and hints
	line    = lipgloss.AdaptiveColor{Light: "#d1d5db", Dark: "#44475a"} // rules and borders
	soft    = lipgloss.AdaptiveColor{Light: "#374151", Dark: "#c3c8e6"} // text under a title
	selBg   = lipgloss.AdaptiveColor{Light: "#ddd6fe", Dark: "#44475a"}
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
	sSel    = lipgloss.NewStyle().Bold(true).Background(selBg).Foreground(lipgloss.AdaptiveColor{Light: "#1f2937", Dark: "#f8f8f2"})
	sSelDim = lipgloss.NewStyle().Bold(true).Background(lipgloss.AdaptiveColor{Light: "#ede9fe", Dark: "#3a3c4e"}).Foreground(lipgloss.AdaptiveColor{Light: "#1f2937", Dark: "#f8f8f2"})
	sHover  = lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "#f3f0ff", Dark: "#343746"})
	sKey    = lipgloss.NewStyle().Foreground(accent).Bold(true)
	sErr    = lipgloss.NewStyle().Foreground(danger)
	sHeader = lipgloss.NewStyle().Foreground(muted).Bold(true)
	sDim    = lipgloss.NewStyle().Foreground(dim)
	sLine   = lipgloss.NewStyle().Foreground(line)
	sSoft   = lipgloss.NewStyle().Foreground(soft)
	// Section titles stand out from the field labels.
	sSection  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#111827", Dark: "#d6daf5"}).Bold(true)
	sSoftBold = sSoft.Bold(true)
)

type Model struct {
	cfg    config.Config
	inside bool // running in cairn's own herdr workspace

	projects []*project
	loaded   bool // the trackers answered once
	snap     herdr.Snapshot
	localErr error

	chantiers []*chantier.Chantier
	all       []*chantier.Chantier // chantiers, shipped ones the list hides included
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

	detailFocus bool            // keys scroll the detail; when narrow, the detail shows
	unfold      map[string]bool // "KEY:section" unfolded by hand, "KEY:*" for all
	filter      string          // "/" filter of the list
	filtering   bool            // typing the filter
	overlay     string          // "", "help", "palette", "push", "close", "update"
	palQuery    string
	palCursor   int
	pushKey     string   // chantier the push confirmation is about
	pushLog     []string // its commits, "<hash> <subject>"
	pushPR      bool     // the confirmation opens the PR, not just the push
	closeKey    string   // chantier the close confirmation is about
	closeRisk   *closeRisk
	answerQ     string // open question being answered, "" if none
	answerText  string
	answered    map[string]string // answers sent, by answerKey

	seen        *seenStore // when you last looked at each chantier
	lookedAt    time.Time  // since when the detail shows the selection
	watch       *watchState
	shipped     int        // shipped chantiers left out of the list
	showShipped bool       // list them anyway
	pick        *pickState // the "new ticket" view
	settings    *settingsState
	upd         *updState                   // the update check; nil turns it off
	tests       map[string]localtest.Status // main checkouts under test, by project
	ltKey       string                      // chantier the "test here" confirmation is about
	ltPlan      *localtest.Plan
	ltErr       error

	// The comments view ("" is the main screen).
	view      string
	thread    *prthread.Thread
	threadKey string
	threadErr error
	cscroll   int
	showBots  bool
	hscroll   int  // scroll of the "how it works" view
	howAsk    bool // typing a question for the agent there
	howText   string
	howArch   string           // archived ticket the view shows, "" for the selection
	archive   []state.Archived // shipped tickets' plans, latest first
	archMod   time.Time        // when the archive was last read
	tokens    map[string]string

	width, height int
	flash         string
	flashErr      bool
	busy          string
}

func New(cfg config.Config) Model {
	m := Model{cfg: cfg, inside: home.Inside(), hoverItem: -1, hoverBtn: -1, watch: newWatch(),
		gits: map[string]progress.Git{}, prs: map[string]progress.PR{}, readAt: map[string]time.Time{}, reading: map[string]bool{}, tokens: map[string]string{},
		unfold: map[string]bool{}, answered: map[string]string{}, seen: loadSeen(seenPath())}
	m.projects = newProjects(cfg)
	return m
}

// ── messages ────────────────────────────────────────────────────────────────

type tickLocal struct{}
type tickIssues struct{}
type doneMsg struct {
	text    string
	err     error
	quit    bool
	refresh bool
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadLocal(), m.loadIssues(), tea.SetWindowTitle("cairn"), m.checkUpdate(update.Fresh), every(update.Fresh, tickUpdate{}))
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
		ChantiersDir: config.ChantiersDir(), Sources: m.sources(), Snapshot: m.snap,
		GitHubPRs: m.githubPRs(),
	})
	// A shipped chantier leaves the list (cairn cleans it up), unless
	// something keeps it: it then stays in view with the reason.
	m.refreshArchive()
	m.rebuildOrder()
	m.all = append([]*chantier.Chantier(nil), m.chantiers...)
	m.shipped = 0
	kept := m.chantiers[:0]
	var done []*chantier.Chantier
	for _, c := range m.chantiers {
		switch {
		case !m.isShipped(c):
			kept = append(kept, c)
		case m.showShipped:
			done = append(done, c)
		default:
			m.shipped++
		}
	}
	m.chantiers = append(kept, done...)
	if m.showShipped {
		m.shipped = len(done)
	}
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
	if m.seen != nil && m.watch.ready {
		m.seen.adopt(m.chantiers)
	}
	return tea.Batch(m.pushTokens(), m.observe())
}

// rebuildOrder puts what needs you first: a decision, or a broken CI.
func (m *Model) rebuildOrder() {
	sort.SliceStable(m.chantiers, func(i, j int) bool {
		return m.needsYou(m.chantiers[i]) && !m.needsYou(m.chantiers[j])
	})
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
			out[ws.ID] = i18n.T("⚠ 1 attend ta décision", "⚠ 1 waiting for you")
		case waiting > 1:
			out[ws.ID] = fmt.Sprintf(i18n.T("⚠ %d attendent ta décision", "⚠ %d waiting for you"), waiting)
		default:
			out[ws.ID] = fmt.Sprintf(i18n.T("%d chantiers", "%d tickets"), len(list))
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
		var cmd tea.Cmd
		if pm.pr != nil {
			changed := pm.pr.State != m.prs[pm.key].State
			cmd = m.notePR(pm.key, *pm.pr)
			if changed {
				cmd = tea.Batch(cmd, m.rebuild())
			}
		}
		return m, cmd
	}
	next, cmd := m.update(msg)
	nm := next.(Model)
	nm.offerPending()
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
		m.leave(m.detailOf)
		m.lookedAt = time.Now()
		m.detailOf, m.scroll, m.answerQ, m.answerText = c.Key, 0, "", ""
	}
	key, wt, base := c.Key, c.Worktree, m.base(c)
	var prURL string
	if pr := c.LastPR(); pr != nil {
		prURL = pr.URL
	}
	// Read again when stale, or when a PR or worktree showed up since (the
	// first read can happen before the tracker answered).
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
	case prsMsg:
		var cmds []tea.Cmd
		for key, pr := range msg {
			cmds = append(cmds, m.notePR(key, pr))
		}
		// Phases are journaled once issues AND GitHub's PR states are known:
		// before, a merged PR still looks open (trackers lag).
		m.watch.ready = true
		cmds = append(cmds, m.rebuild())
		return m, tea.Batch(append(cmds, every(prsEvery, tickPRs{}))...)
	case tickPRs:
		return m, m.pollPRs()
	case updateMsg:
		return m.updateChecked(msg)
	case tickUpdate:
		return m, tea.Batch(m.checkUpdate(update.Fresh), every(update.Fresh, tickUpdate{}))
	case pushLogMsg:
		if msg.key == m.pushKey {
			m.pushLog = msg.lines
		}
		return m, nil
	case threadMsg:
		if msg.key == m.threadKey {
			m.thread, m.threadErr = msg.thread, msg.err
			if m.view == "comments" {
				m.scrollComments(m.firstNew())
			}
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
			m.snap = msg.snap
			for _, p := range m.projects {
				p.worktrees = msg.worktrees[p.cfg.Name]
			}
			m.tests = msg.tests
		}
		return m, tea.Batch(m.rebuild(), every(localEvery, tickLocal{}))

	case ltPlanMsg:
		if msg.key == m.ltKey {
			m.ltPlan, m.ltErr = &msg.plan, msg.err
		}
		return m, nil
	case closeRiskMsg:
		if msg.key == m.closeKey {
			m.closeRisk = &msg.risk
		}
		return m, nil
	case cleanedMsg:
		m.watch.keep[msg.key] = msg.keep
		if m.busy != "" && msg.key == m.closeKey {
			m.busy = ""
			if msg.err == nil && msg.text == "" {
				msg.text = msg.key + i18n.T(" fermé : plan archivé.", " closed: plan archived.")
			}
		}
		if msg.err != nil || msg.text != "" {
			m.say(msg.text, msg.err)
		}
		cmd := m.rebuild()
		return m, cmd
	case pickListMsg:
		if m.pick != nil {
			if msg.project != m.pick.project {
				return m, nil
			}
			m.pick.loading = false
			m.pick.list = m.pickList(msg.issues)
			if msg.err != nil {
				m.say("", msg.err)
			}
		}
		return m, nil
	case connCheckMsg:
		if m.settings != nil {
			m.settings.checks[msg.name] = &connCheck{conn: msg.conn, err: msg.err}
		}
		return m, nil
	case pickIssueMsg:
		m.busy = ""
		if msg.err != nil {
			m.say("", msg.err)
			return m, nil
		}
		if msg.project >= len(m.projects) {
			return m, nil
		}
		return m.startTicket(m.projects[msg.project], msg.issue)

	case issuesMsg:
		ok := m.apply(msg)
		m.loaded = true
		cmd := m.rebuild()
		if !m.watch.polling && ok {
			m.watch.polling = true
			cmd = tea.Batch(cmd, m.pollPRs())
		}
		return m, tea.Batch(cmd, every(issuesEvery, tickIssues{}))

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
	switch {
	case m.overlay != "":
		return m.overlayKey(k)
	case m.view == "comments":
		return m.commentsKey(k)
	case m.view == "how":
		return m.howKey(k)
	case m.view == "pick":
		return m.pickKeyPress(k)
	case m.view == "settings":
		return m.settingsKey(k)
	case m.answerQ != "":
		return m.answerKeyPress(k)
	case m.filtering:
		return m.filterKey(k)
	case m.view == "board":
		return m.boardKey(k)
	}
	return m.mainKey(k)
}

func (m Model) mainKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.flash = ""
	switch k.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		switch {
		case m.narrow() && m.detailFocus:
			m.detailFocus = false
			return m, nil
		case m.filter != "":
			m.filter = ""
			return m, nil
		}
		return m.quit()
	case "q":
		return m.quit()
	case "tab":
		m.detailFocus = !m.detailFocus
	case "right":
		m.detailFocus = true
	case "left":
		m.detailFocus = false
	case "up", "k":
		if m.detailFocus {
			m.scrollDetail(-1)
		} else {
			m.moveCursor(-1)
		}
	case "down", "j":
		if m.detailFocus {
			m.scrollDetail(1)
		} else {
			m.moveCursor(1)
		}
	case " ", "pgdown":
		m.scrollDetail(max(1, m.bodyHeight()-3))
	case "pgup":
		m.scrollDetail(-max(1, m.bodyHeight()-3))
	case "]":
		m.jumpSection(1)
	case "[":
		m.jumpSection(-1)
	case "z":
		if c := m.selected(); c != nil {
			all := !m.unfold[c.Key+":*"]
			for k := range m.unfold {
				if strings.HasPrefix(k, c.Key+":") {
					delete(m.unfold, k)
				}
			}
			m.unfold[c.Key+":*"] = all
		}
	case "/":
		m.filtering = true
	case "ctrl+k":
		m.overlay, m.palQuery, m.palCursor = "palette", "", 0
	case "?":
		m.overlay = "help"
	case "t":
		if m.view == "board" {
			m.view = ""
		} else {
			m = m.openBoard()
		}
	case "e":
		return m.startAnswer(-1)
	case "l":
		lang := "en"
		if i18n.English() {
			lang = "fr"
		}
		i18n.Set(lang)
		m.say(i18n.T("Interface en français.", "Interface in English."), config.SetLang(lang))
		cmd := m.rebuild()
		return m, cmd
	case "n":
		return m.openPick()
	case "s":
		return m.openSettings()
	case "r":
		if c := m.selected(); c != nil {
			delete(m.readAt, c.Key)
		}
		m.say(i18n.T("Rafraîchi.", "Refreshed."), nil)
		return m, tea.Batch(m.loadLocal(), m.loadIssues())
	case "enter":
		return m, m.goTo()
	case "d":
		return m, m.diff()
	case "o":
		if c := m.selected(); c != nil && c.Issue != nil && c.Issue.URL != "" {
			url := c.Issue.URL
			return m, func() tea.Msg {
				return doneMsg{text: i18n.T("Ticket ouvert.", "Ticket opened."), err: openURL(url)}
			}
		}
	case "c":
		return m.openComments()
	case "i":
		return m.openHow()
	case "f":
		return m.fixCI()
	case "p":
		return m.plan()
	case "v":
		return m.validate()
	case "a":
		return m.openPush()
	case "P":
		return m.openPR()
	case "x":
		return m.openClose()
	case "U":
		return m.updateKey()
	case "h":
		return m.testKey()
	}
	return m, nil
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	if m.inside {
		// cairn stays alive in its workspace; q just hands herdr back.
		return m, func() tea.Msg {
			return doneMsg{text: i18n.T("← retour à herdr", "← back to herdr"), err: home.Back()}
		}
	}
	m.leave(m.detailOf)
	return m, tea.Quit
}

// moveCursor moves the selection by delta among the items the filter keeps.
func (m *Model) moveCursor(delta int) {
	for i := m.cursor + delta; i >= 0 && i < m.items(); i += delta {
		if m.matches(i) {
			m.cursor = i
			return
		}
	}
}

func (m Model) filterKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.filter, m.filtering = "", false
		return m, nil
	case tea.KeyEnter:
		m.filtering = false
		return m, nil
	case tea.KeyUp:
		m.moveCursor(-1)
		return m, nil
	case tea.KeyDown:
		m.moveCursor(1)
		return m, nil
	case tea.KeyBackspace:
		if r := []rune(m.filter); len(r) > 0 {
			m.filter = string(r[:len(r)-1])
		}
	case tea.KeySpace:
		m.filter += " "
	case tea.KeyRunes:
		m.filter += string(k.Runes)
	}
	// Keep the selection on an item the filter shows.
	if !m.matches(m.cursor) {
		for i := 0; i < m.items(); i++ {
			if m.matches(i) {
				m.cursor = i
				break
			}
		}
	}
	return m, nil
}

func (m Model) mouse(ev tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch {
	case m.overlay != "":
		return m.overlayMouse(ev)
	case m.view == "comments":
		return m.commentsMouse(ev)
	case m.view == "how":
		return m.howMouse(ev)
	case m.view == "pick":
		return m.pickMouse(ev)
	case m.view == "settings":
		return m.settingsMouse(ev)
	case m.view == "board":
		return m.boardMouse(ev)
	}
	bodyH := m.bodyHeight()
	inList := m.showList() && ev.X < m.listW()
	inDetail := m.showDetail() && ev.X >= m.detailX()
	if ev.Action == tea.MouseActionMotion {
		m.hoverItem, m.hoverBtn = m.hitTest(ev.X, ev.Y)
		return m, nil
	}
	if m.busy != "" {
		return m, nil
	}
	switch ev.Button {
	case tea.MouseButtonWheelUp:
		if inList {
			m.moveCursor(-1)
		} else if inDetail {
			m.scrollDetail(-2)
		}
		return m, nil
	case tea.MouseButtonWheelDown:
		if inList {
			m.moveCursor(1)
		} else if inDetail {
			m.scrollDetail(2)
		}
		return m, nil
	}
	if ev.Button != tea.MouseButtonLeft || ev.Action != tea.MouseActionPress {
		return m, nil
	}
	row := ev.Y - 2
	switch {
	case ev.Y == 0 && m.narrow() && m.detailFocus:
		m.detailFocus = false // "‹ esc liste"
	case ev.Y == 2+bodyH:
		return m.footerClick(ev.X)
	case row >= 0 && row < bodyH && inList:
		m.detailFocus = false
		_, owners := m.listLayout(m.listW(), bodyH)
		if row < len(owners) && owners[row] == shippedItem {
			m.showShipped = !m.showShipped
			cmd := m.rebuild()
			return m, cmd
		}
		if row < len(owners) && owners[row] >= 0 {
			m.flash = ""
			if m.narrow() {
				m.cursor, m.detailFocus = owners[row], true // narrow: a click opens the detail
				return m, nil
			}
			if owners[row] == m.cursor {
				return m, m.goTo() // second click on the selection: go there
			}
			m.cursor = owners[row]
		}
	case row >= 0 && row < bodyH && inDetail:
		m.detailFocus = true
		d := m.detailLayout(m.detailWidth())
		line := row + m.detailTop(len(d.lines), bodyH)
		if line == d.cardRow {
			x := ev.X - m.detailX()
			for _, h := range d.cardHits {
				if x >= h.x0 && x < h.x1 {
					return m.key(keyFor(h.key))
				}
			}
			return m, nil
		}
		if line < len(d.targets) && d.targets[line] != "" {
			return m.detailClick(d.targets[line])
		}
	}
	return m, nil
}

// footerClick runs the footer button under column x.
func (m Model) footerClick(x int) (tea.Model, tea.Cmd) {
	_, hits, _ := m.footer()
	for _, h := range hits {
		if h.key != "" && x >= h.x0 && x < h.x1 {
			switch m.view {
			case "pick":
				return m.pickKeyPress(keyFor(h.key))
			case "settings":
				return m.settingsKey(keyFor(h.key))
			case "comments":
				return m.commentsKey(keyFor(h.key))
			case "how":
				return m.howKey(keyFor(h.key))
			}
			return m.key(keyFor(h.key))
		}
	}
	return m, nil
}

// hitTest returns the list item and the footer span under (x, y), -1 if none.
func (m Model) hitTest(x, y int) (item, btn int) {
	item, btn = -1, -1
	bodyH := m.bodyHeight()
	if y == 2+bodyH {
		_, hits, _ := m.footer()
		for i, h := range hits {
			if h.key != "" && x >= h.x0 && x < h.x1 {
				btn = i
			}
		}
		return
	}
	if row := y - 2; m.view == "" && row >= 0 && row < bodyH && m.showList() && x < m.listW() {
		_, owners := m.listLayout(m.listW(), bodyH)
		if row < len(owners) {
			item = owners[row]
		}
	}
	return
}

// detailClick acts on a detail-pane target: "pane:<id>", "img:<path>", "comments"…
func (m Model) detailClick(target string) (tea.Model, tea.Cmd) {
	kind, arg, _ := strings.Cut(target, ":")
	switch kind {
	case "pane":
		return m, func() tea.Msg { return doneMsg{text: "→ " + arg, err: herdr.FocusAgent(arg)} }
	case "comments":
		return m.openComments()
	case "how":
		return m.openHow()
	case "fixci":
		return m.fixCI()
	case "attach":
		return m.attach(arg)
	case "answer":
		var i int
		fmt.Sscan(arg, &i)
		return m.startAnswer(i)
	case "manual":
		var i int
		fmt.Sscan(arg, &i)
		return m.tickManual(i)
	case "fold":
		if c := m.selected(); c != nil {
			k := c.Key + ":" + arg
			m.unfold[k] = !m.unfolded(c.Key, arg)
		}
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
			caps[i] = i18n.T("preuve · ", "proof · ") + tilde(f)
		}
		m.busy = i18n.T("Ouverture des captures…", "Opening screenshots…")
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
	case "ctrl+k":
		return tea.KeyMsg{Type: tea.KeyCtrlK}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
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
	return func() tea.Msg {
		return doneMsg{text: c.Key + i18n.T(" n'a pas encore d'espace herdr : p pour le lancer.", " has no herdr workspace yet: p to start it.")}
	}
}

// base is the ref the chantier's branch compares against.
func (m Model) base(c *chantier.Chantier) string {
	if p := m.projectOf(c); p != nil {
		return p.cfg.Base
	}
	return "origin/main"
}

// diffCommand is the project's diff command, {base} filled in: browsediff
// when installed, else git diff in its pager.
func diffCommand(custom, base string) string {
	if custom == "" {
		custom = "git diff {base}...HEAD"
		if _, err := exec.LookPath("browsediff"); err == nil {
			custom = "browsediff {base}...HEAD"
		}
	}
	return strings.ReplaceAll(custom, "{base}", base)
}

func (m Model) diff() tea.Cmd {
	c := m.selected()
	if c == nil || c.Worktree == "" {
		return func() tea.Msg {
			return doneMsg{text: i18n.T("Pas de worktree pour ce chantier.", "No worktree for this ticket.")}
		}
	}
	custom := ""
	if p := m.projectOf(c); p != nil {
		custom = p.cfg.Diff
	}
	cmd := exec.Command("sh", "-c", diffCommand(custom, m.base(c)))
	cmd.Dir = c.Worktree
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return doneMsg{err: err} })
}

func (m Model) plan() (tea.Model, tea.Cmd) {
	c := m.selected()
	switch {
	case c == nil:
		return m, nil
	case c.Issue == nil || m.projectOf(c) == nil || m.projectOf(c).tr == nil:
		m.say(i18n.T("Ticket introuvable dans le tracker du projet.", "Ticket not found in the project's tracker."), nil)
		return m, nil
	case c.Worktree != "" && !c.Main:
		m.say(c.Key+i18n.T(" a déjà son worktree : enter pour y aller.", " already has its worktree: enter to go there."), nil)
		return m, nil
	}
	p := m.projectOf(c)
	cfg, issue, agents, tr := p.cfg, *c.Issue, m.snap.Agents, p.tr
	key := strings.ToUpper(issue.ID)
	lower := strings.ToLower(key)
	_, num, _ := strings.Cut(lower, "-")
	branch := cfg.BranchPrefix + lower + "-" + chantier.Slug(issue.Title, 50)
	path := filepath.Join(cfg.WorktreeParent, filepath.Base(cfg.Repo)+"-"+num)
	statePath := state.PathFor(config.ChantiersDir(), key)
	m.busy = fmt.Sprintf(i18n.T("Lancement de %s…", "Starting %s…"), key)
	return m, func() tea.Msg {
		if _, err := os.Stat(path); err == nil {
			return doneMsg{err: fmt.Errorf(i18n.T("%s existe déjà", "%s already exists"), path)}
		}
		_ = exec.Command("git", "-C", cfg.Repo, "fetch", "--quiet", "origin").Run()
		_, pane, err := herdr.CreateWorktree(cfg.Repo, branch, cfg.Base, path, key)
		if err != nil {
			return doneMsg{err: err}
		}
		switch s, _ := state.Load(statePath); {
		case s == nil:
			if err := state.New(statePath, key, issue.Title).Save(); err != nil {
				return doneMsg{err: err}
			}
		case s.Phase == "todo" || s.Phase == "":
			// A local ticket waiting for its plan.
			s.Set("phase", "plan")
			if err := s.Save(); err != nil {
				return doneMsg{err: err}
			}
		}
		name := herdr.FreeName(lower, agents)
		if err := herdr.StartAgent(name, cfg.AgentKind, pane); err != nil {
			return doneMsg{err: fmt.Errorf(i18n.T("worktree prêt, mais l'agent n'a pas démarré : %w", "worktree ready, but the agent did not start: %w"), err), refresh: true}
		}
		err = herdr.Prompt(name, chantier.PlanPrompt(key, issue.Title, statePath, tr.Guide(key)))
		text := fmt.Sprintf(i18n.T("%s : l'agent %s écrit le plan.", "%s: agent %s is writing the plan."), key, name)
		if moved, err := tr.Start(issue); moved && err == nil {
			text += i18n.T(" Ticket passé en cours.", " Ticket moved to in progress.")
		}
		_ = journal.Append(config.ChantiersDir(), key, "plan lancé ("+name+")")
		return doneMsg{text: text, err: err, refresh: true}
	}
}

// tickManual checks or unchecks a step of the manual test, in the plan file
// itself: the agent sees what the human already tried.
func (m Model) tickManual(i int) (tea.Model, tea.Cmd) {
	c := m.selected()
	if c == nil || c.State == nil {
		return m, nil
	}
	s, err := state.Load(c.State.Path)
	if err != nil {
		m.say("", err)
		return m, nil
	}
	if s == nil || i >= len(s.ManualSteps) {
		return m, nil
	}
	if !s.TickManual(i, !s.ManualSteps[i].Done) {
		return m, nil
	}
	if err := s.Save(); err != nil {
		m.say("", err)
		return m, nil
	}
	c.State = s // the watcher reloads it too
	return m, nil
}

// allowPush lets the agent push the commit the human just reviewed: the push
// and its retries pass while the branch stays on it. The PR is apart (allowPR).
func (m Model) allowPush() (tea.Model, tea.Cmd) {
	c := m.selected()
	if c == nil || c.State == nil {
		m.say(i18n.T("Ce chantier n'a pas de plan cairn : ses push ne sont pas bloqués.", "This ticket has no cairn plan: its pushes are not blocked."), nil)
		return m, nil
	}
	s := c.State
	head := headOf(c.Worktree)
	s.Set("push", strings.TrimSpace("autorisé "+head))
	if isPushWait(s.Waiting) {
		s.Set("attente", "")
	}
	if err := s.Save(); err != nil {
		m.say("", err)
		return m, nil
	}
	_ = journal.Append(config.ChantiersDir(), c.Key, "push autorisé")
	text := fmt.Sprintf(i18n.T("Push autorisé pour %s, commit %s.", "Push allowed for %s, commit %s."), c.Key, shortSHA(head))
	cmd := m.rebuild()
	if len(c.Agents) > 0 {
		pane := c.Agents[0].PaneID
		prompt := fmt.Sprintf(i18n.T("Push autorisé dans cairn pour %s, sur le commit %s : pousse maintenant (les nouvelles tentatives passent). "+
			"N'ouvre pas de PR : son ouverture s'autorise à part dans cairn ; si elle est déjà ouverte, rien à faire de plus.",
			"Push allowed in cairn for %s, on commit %s: push now (retries pass). "+
				"Do not open a PR: opening it is allowed separately in cairn; if it is already open, nothing more to do."), c.Key, shortSHA(head))
		cmd = tea.Batch(cmd, func() tea.Msg {
			return doneMsg{text: text + i18n.T(" Agent relancé.", " Agent resumed."), err: herdr.Prompt(pane, prompt)}
		})
	}
	m.say(text, nil)
	return m, cmd
}

// allowPR lets the agent open the pull request, pushing the reviewed commit
// first if it has not left yet.
func (m Model) allowPR() (tea.Model, tea.Cmd) {
	c := m.selected()
	if c == nil || c.State == nil {
		return m, nil
	}
	s := c.State
	head := headOf(c.Worktree)
	s.Set("pr", "autorisée")
	if !m.pushValid(c) {
		s.Set("push", strings.TrimSpace("autorisé "+head))
	}
	if isPushWait(s.Waiting) || isPRWait(s.Waiting) {
		s.Set("attente", "")
	}
	if err := s.Save(); err != nil {
		m.say("", err)
		return m, nil
	}
	_ = journal.Append(config.ChantiersDir(), c.Key, "PR autorisée")
	text := fmt.Sprintf(i18n.T("Ouverture de la PR autorisée pour %s.", "Opening the PR allowed for %s."), c.Key)
	cmd := m.rebuild()
	if pane := agentPane(c); pane != "" {
		prompt := fmt.Sprintf(i18n.T("Ouverture de la PR autorisée dans cairn pour %s : pousse la branche si ce n'est pas fait (le commit %s est autorisé), "+
			"ouvre la PR, lie-la au ticket et passe le plan en `phase: review`. Ne la merge pas. "+
			"Une fois la PR ouverte, tes pushes suivants (correctifs de CI ou de review) passent sans redemander.",
			"Opening the PR allowed in cairn for %s: push the branch if not done yet (commit %s is allowed), "+
				"open the PR, link it to the ticket and set the plan to `phase: review`. Do not merge it. "+
				"Once the PR is open, your next pushes (CI or review fixes) pass without asking again."), c.Key, shortSHA(head))
		cmd = tea.Batch(cmd, func() tea.Msg {
			return doneMsg{text: text + i18n.T(" Agent relancé.", " Agent resumed."), err: herdr.Prompt(pane, prompt)}
		})
	}
	m.say(text, nil)
	return m, cmd
}

// headOf is the commit checked out in dir, "" if unknown.
func headOf(dir string) string {
	if dir == "" {
		return ""
	}
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// pushValid: an allowance exists and still covers the branch's commit. One
// asked in the chat has no commit until the first push pins it: it is valid.
func (m Model) pushValid(c *chantier.Chantier) bool {
	if pr, ok := m.prs[c.Key]; ok && pr.State == "OPEN" {
		return true // the PR is where review happens: pushes are free
	}
	s := c.State
	return s != nil && s.PushAllowed() && s.PushCovers(m.gits[c.Key].Head)
}

func isPushWait(w string) bool {
	l := strings.ToLower(w)
	return !isPRWait(w) && (strings.Contains(l, "pousser") || strings.Contains(l, "push"))
}

// isPRWait: the agent waits for the go-ahead to open the PR.
func isPRWait(w string) bool {
	l := strings.ToLower(w)
	return (strings.Contains(l, "ouvrir") || strings.Contains(l, "open")) && (strings.Contains(l, "pr") || strings.Contains(l, "pull request"))
}

func (m Model) validate() (tea.Model, tea.Cmd) {
	c := m.selected()
	if c == nil || c.State == nil || c.State.Waiting == "" {
		m.say(i18n.T("Rien à valider ici.", "Nothing to approve here."), nil)
		return m, nil
	}
	s := c.State
	waiting := s.Waiting
	s.Set("attente", "")
	text := i18n.T("Validé : ", "Approved: ") + waiting
	var prompt string
	if s.Phase == "plan" {
		s.Set("phase", "build")
		prompt = chantier.BuildPrompt(s.Path)
		_ = journal.Append(config.ChantiersDir(), c.Key, "plan validé")
		text = fmt.Sprintf(i18n.T("Plan validé : %s passe en Build.", "Plan approved: %s moves to Build."), c.Key)
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
		cmd = tea.Batch(cmd, func() tea.Msg {
			return doneMsg{text: text + i18n.T(" Agent relancé.", " Agent resumed."), err: herdr.Prompt(agent, prompt)}
		})
	}
	return m, cmd
}

// refreshArchive rereads the archived plans when a chantier was put away.
func (m *Model) refreshArchive() {
	dir := config.ArchiveDir()
	fi, err := os.Stat(dir)
	if err != nil || fi.ModTime().Equal(m.archMod) {
		return
	}
	m.archive, m.archMod = state.LoadArchive(dir), fi.ModTime()
}
