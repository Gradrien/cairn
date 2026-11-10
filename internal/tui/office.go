package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"cairn/internal/chantier"
	"cairn/internal/i18n"
	"cairn/internal/office"
	"cairn/internal/state"
	"cairn/internal/usage"
)

// The office (b) shows each chantier as a room: the main agent at its desk,
// at the whiteboard while it writes the plan, at the shelf while it reads, at
// the test machine while tests run; the subagents it calls come in by the
// door, work at their desks, bring their result and leave. It only shows:
// the actions are the same buttons as everywhere else.

const officeFrame = 120 * time.Millisecond

type officeTick struct{}

func officeTickCmd() tea.Cmd {
	return tea.Tick(officeFrame, func(time.Time) tea.Msg { return officeTick{} })
}

// homeView: the list, the board or the office, the screens that share the
// header, the selection and the global keys.
func (m Model) homeView() bool { return m.view == "" || m.view == "board" || m.view == "office" }

func (m Model) openOffice() (Model, tea.Cmd) {
	m.view, m.officeZoom = "office", false
	if m.rooms == nil {
		m.rooms = map[string]*office.Room{}
	}
	if m.selected() == nil && len(m.chantiers) > 0 {
		m.cursor = 0
	}
	if m.officeTicking {
		return m, nil
	}
	m.officeTicking = true
	return m, officeTickCmd()
}

// stepOffice moves the people of the rooms on screen one frame.
func (m *Model) stepOffice() {
	now := time.Now()
	for _, r := range m.officeRooms() {
		c := m.chantiers[r.item]
		room := m.room(c.Key)
		room.Step(m.officeInput(c, now), office.NewLayout(r.cols, r.rows))
	}
}

func (m *Model) room(key string) *office.Room {
	if m.rooms == nil {
		m.rooms = map[string]*office.Room{}
	}
	r := m.rooms[key]
	if r == nil {
		r = office.NewRoom()
		m.rooms[key] = r
	}
	return r
}

// ── what a room shows ───────────────────────────────────────────────────────

var testCmd = regexp.MustCompile(`\b(go (test|vet|build)|pytest|jest|vitest|tsc|eslint|cargo (test|build)|make( \w+)?|(pnpm|npm|yarn|bun)( run)? (test|build|lint|typecheck|check)\w*)\b`)

func (m Model) officeInput(c *chantier.Chantier, now time.Time) office.Input {
	in := office.Input{Now: now, Board: boardLines(c, m.planEdits[c.Key], now)}
	rep := m.usage[c.Key]

	// Colleagues: the subagents seen in the last minutes.
	newest := ""
	for _, s := range rep.Subagent {
		if now.Sub(s.Last) > 10*time.Minute {
			continue
		}
		id := s.Type + "@" + s.Start.Format(time.RFC3339Nano)
		col := office.Colleague{ID: id, Role: s.Type, Mood: office.Leaving}
		switch {
		case !s.Finished && now.Sub(s.Last) < 3*time.Minute:
			col.Mood = office.Working
			col.Bubble = s.Desc
			for _, a := range rep.Recent {
				if a.Agent == s.Type && !a.At.Before(s.Start) {
					col.Bubble = strings.TrimSpace(a.Tool + " " + a.Target)
					break
				}
			}
			if newest == "" {
				newest = id
			}
		case s.Finished && now.Sub(s.Last) < 20*time.Second:
			col.Mood, col.Bubble = office.Delivering, "✓ "+s.Desc
		}
		in.Colleagues = append(in.Colleagues, col)
	}

	// The main agent.
	var last *usage.Action
	for i, a := range rep.Recent {
		if a.Agent == usage.Main {
			last = &rep.Recent[i]
			break
		}
	}
	working, present := false, len(c.Agents) > 0
	for _, a := range c.Agents {
		working = working || a.Status == "working"
	}
	if last != nil && now.Sub(last.At) < 2*time.Minute {
		present, working = true, working || len(c.Agents) == 0
	}
	act := office.Activity{Kind: office.Idle}
	switch {
	case !present:
		act.Kind = office.Absent
	case c.Waiting() != "":
		act = office.Activity{Kind: office.Waiting, Label: "? " + lowerFirst(c.Waiting())}
	case !working:
		if last == nil || now.Sub(last.At) > 10*time.Minute {
			act.Label = "z z"
		}
	case m.planEdits[c.Key].at.After(now.Add(-time.Minute)):
		act = office.Activity{Kind: office.Planning, Label: m.planEdits[c.Key].label}
	case last == nil || now.Sub(last.At) > 3*time.Minute:
		act = office.Activity{Kind: office.Thinking, Label: "…"}
	default:
		act = mainActivity(c, *last, now, newest)
	}
	in.Main = act
	return in
}

// mainActivity reads the main agent's last tool call.
func mainActivity(c *chantier.Chantier, a usage.Action, now time.Time, newest string) office.Activity {
	plan := strings.EqualFold(a.Target, c.Key+".md")
	switch a.Tool {
	case "Agent", "Task":
		if newest != "" && now.Sub(a.At) < 20*time.Second {
			return office.Activity{Kind: office.Delegating, Label: "→ " + a.Target, To: newest}
		}
		if newest != "" {
			return office.Activity{Kind: office.Thinking, Label: i18n.T("attend son collègue…", "waits for a colleague…")}
		}
		return office.Activity{Kind: office.Thinking, Label: "…"}
	case "Read", "Grep", "Glob", "LS", "WebFetch", "WebSearch", "NotebookRead":
		return office.Activity{Kind: office.Reading, Label: i18n.T("lit ", "reads ") + a.Target}
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		if plan {
			return office.Activity{Kind: office.Planning, Label: i18n.T("met à jour le plan", "updates the plan")}
		}
		return office.Activity{Kind: office.Writing, Label: i18n.T("écrit ", "writes ") + a.Target}
	case "Bash":
		if testCmd.MatchString(a.Command) {
			return office.Activity{Kind: office.Testing, Label: "▶ " + firstWords(a.Command, 4)}
		}
		label := a.Target
		if label == "" {
			label = "$ " + firstWords(a.Command, 4)
		}
		return office.Activity{Kind: office.Running, Label: label}
	}
	return office.Activity{Kind: office.Running, Label: strings.TrimSpace(a.Tool + " " + a.Target)}
}

func firstWords(s string, n int) string {
	f := strings.Fields(s)
	if len(f) > n {
		return strings.Join(f[:n], " ") + " …"
	}
	return strings.Join(f, " ")
}

// boardLines are the whiteboard: where the plan stands, section by section.
func boardLines(c *chantier.Chantier, edit planEdit, now time.Time) []office.Line {
	s := c.State
	if s == nil {
		return []office.Line{{Text: i18n.T("pas de plan", "no plan")}}
	}
	hot := ""
	if edit.at.After(now.Add(-time.Minute)) {
		hot = edit.section
	}
	var out []office.Line
	add := func(section, text string, done bool) {
		out = append(out, office.Line{Text: text, Done: done, Hot: section == hot})
	}
	if len(s.Summary) > 0 || hot == "goal" {
		add("goal", i18n.T("Objectif", "Goal"), len(s.Summary) > 0 && hot != "goal")
	}
	if n := len(s.OpenQuestions()); n > 0 {
		add("questions", "? "+plural(n, "question", "questions"), false)
	} else if len(s.Questions) > 0 {
		add("questions", i18n.T("Questions tranchées", "Questions settled"), true)
	}
	if len(s.Tasks) > 0 {
		add("tasks", fmt.Sprintf(i18n.T("Tâches %d/%d", "Tasks %d/%d"), s.DoneCount(), len(s.Tasks)), s.DoneCount() == len(s.Tasks))
	}
	if n := len(s.SideEffects); n > 0 {
		d := 0
		for _, t := range s.SideEffects {
			if t.Done {
				d++
			}
		}
		add("side", fmt.Sprintf(i18n.T("Effets de bord %d/%d", "Side effects %d/%d"), d, n), d == n)
	}
	if s.HowItWorks != "" {
		add("how", howTitle(), true)
	}
	if n := len(s.ManualSteps); n > 0 {
		d := 0
		for _, t := range s.ManualSteps {
			if t.Done {
				d++
			}
		}
		add("manual", fmt.Sprintf(i18n.T("Tester à la main %d/%d", "Manual test %d/%d"), d, n), d == n)
	}
	if n := len(s.Proofs); n > 0 {
		add("proofs", plural(n, i18n.T("preuve", "proof"), i18n.T("preuves", "proofs")), true)
	}
	return out
}

// planEdit is the plan section an agent changed last, and when.
type planEdit struct {
	section, label string
	at             time.Time
}

// planSig fingerprints each section of the plan, to see which one changes.
func planSig(c *chantier.Chantier) map[string]string {
	s := c.State
	if s == nil {
		return nil
	}
	tasks := func(ts []state.Task) string {
		var b strings.Builder
		for _, t := range ts {
			fmt.Fprintf(&b, "%v%v%s|", t.Done, t.Doing, t.Text)
		}
		return b.String()
	}
	return map[string]string{
		"goal":      strings.Join(s.Summary, "\n"),
		"questions": tasks(s.Questions),
		"tasks":     tasks(s.Tasks),
		"side":      tasks(s.SideEffects),
		"how":       s.HowItWorks,
		"manual":    strings.Join(s.ManualSetup, "\n") + tasks(s.ManualSteps),
		"proofs":    strings.Join(s.Proofs, "\n"),
	}
}

func editLabel(section string) string {
	switch section {
	case "goal":
		return i18n.T("écrit l'objectif", "writes the goal")
	case "questions":
		return i18n.T("rédige les questions", "writes the questions")
	case "tasks":
		return i18n.T("met à jour les tâches", "updates the tasks")
	case "side":
		return i18n.T("liste les effets de bord", "lists the side effects")
	case "how":
		return i18n.T("explique comment ça marche", "explains how it works")
	case "manual":
		return i18n.T("écrit le test à la main", "writes the manual test")
	case "proofs":
		return i18n.T("ajoute une preuve", "adds a proof")
	}
	return i18n.T("met à jour le plan", "updates the plan")
}

// notePlanChanges remembers which section of each plan changed since the
// last read: the office shows the agent writing it.
func (m *Model) notePlanChanges() {
	if m.planSigs == nil {
		m.planSigs, m.planEdits = map[string]map[string]string{}, map[string]planEdit{}
	}
	now := time.Now()
	for _, c := range m.chantiers {
		sig := planSig(c)
		prev, seen := m.planSigs[c.Key]
		m.planSigs[c.Key] = sig
		if !seen || sig == nil {
			continue
		}
		for _, sec := range []string{"goal", "questions", "tasks", "side", "how", "manual", "proofs"} {
			if prev[sec] != sig[sec] {
				m.planEdits[c.Key] = planEdit{sec, editLabel(sec), now}
			}
		}
	}
}

// ── open space and zoom ─────────────────────────────────────────────────────

// officeRoom is a room on screen: the chantier, where it is, its size.
type officeRoom struct {
	item       int
	x, y       int // the box's top-left cell, in the body
	w, h       int // the box, border included
	cols, rows int // the room's canvas
}

const (
	roomMinW = 46
	roomRows = 9
	roomBoxH = roomRows + 4 // border, title, canvas, status
	roomGapX = 2
	roomGapY = 1
)

// officeRooms lays the rooms on screen: one big room when zoomed, a grid of
// small ones otherwise, scrolled to keep the selection in view.
func (m Model) officeRooms() []officeRoom {
	bodyH := m.bodyHeight()
	if len(m.chantiers) == 0 {
		return nil
	}
	if m.officeZoom {
		i := min(max(m.cursor, 0), len(m.chantiers)-1)
		return []officeRoom{{item: i, w: m.width, h: bodyH, cols: m.width, rows: max(4, bodyH-3)}}
	}
	per := max(1, (m.width+roomGapX)/(roomMinW+roomGapX))
	w := (m.width - (per-1)*roomGapX) / per
	fit := max(1, (bodyH+roomGapY)/(roomBoxH+roomGapY))
	row0 := 0
	if cur := m.cursor / per; cur >= fit {
		row0 = cur - fit + 1
	}
	var out []officeRoom
	for i := range m.chantiers {
		r, col := i/per-row0, i%per
		if r < 0 || r >= fit {
			continue
		}
		out = append(out, officeRoom{item: i, x: col * (w + roomGapX), y: r * (roomBoxH + roomGapY), w: w, h: roomBoxH, cols: w - 2, rows: roomRows})
	}
	return out
}

func (m Model) officeView() string {
	bodyH := m.bodyHeight()
	rows := make([]string, bodyH)
	for i := range rows {
		rows[i] = strings.Repeat(" ", m.width)
	}
	now := time.Now()
	if len(m.chantiers) == 0 {
		rows[0] = sMuted.Render(i18n.T("Aucun chantier : personne au bureau.", "No ticket: nobody at the office."))
	}
	for _, r := range m.officeRooms() {
		var block []string
		if m.officeZoom {
			block = m.bigRoom(r, now)
		} else {
			block = m.smallRoom(r, now)
		}
		for j, l := range block {
			if y := r.y + j; y < bodyH {
				rows[y] = overlay(rows[y], l, r.x, m.width)
			}
		}
	}
	foot, _, spill := m.footer()
	return m.header() + "\n" + spill + "\n" + strings.Join(rows, "\n") + "\n\n" + foot
}

// overlay writes s over line from column x.
func overlay(line, s string, x, width int) string {
	left := ansi.Truncate(line, x, "")
	right := ansi.Cut(line, x+ansi.StringWidth(s), width)
	return left + s + right
}

func (m Model) canvasFor(r officeRoom, now time.Time, big bool) string {
	c := m.chantiers[r.item]
	in := m.officeInput(c, now)
	cv := office.NewCanvas(r.cols, r.rows)
	room := m.rooms[c.Key]
	if room == nil {
		room = office.NewRoom() // drawn still until the next frame keeps it
	}
	room.Draw(cv, in, office.NewLayout(r.cols, r.rows), office.Options{Bubbles: big, Text: big})
	return cv.Render()
}

// roomStatus is the line under a room: what the main agent does, or what
// the chantier waits for, and what it cost.
func (m Model) roomStatus(c *chantier.Chantier, now time.Time, w int) string {
	in := m.officeInput(c, now)
	var left string
	switch in.Main.Kind {
	case office.Absent:
		left = sDim.Render(i18n.T("personne au bureau", "nobody at the office"))
	case office.Waiting:
		left = sWarn.Render("⚠ " + c.Waiting())
	default:
		left = phaseStyle(c.Phase).Render(c.Phase.Label())
		if in.Main.Label != "" && in.Main.Label != "z z" {
			left += sMuted.Render(" · " + in.Main.Label)
		}
	}
	if n := len(in.Colleagues); n > 0 {
		left += sMuted.Render(" · ") + sSoft.Render(plural(n, i18n.T("collègue", "colleague"), i18n.T("collègues", "colleagues")))
	}
	right := ""
	if sum, ok := m.costOf(c); ok && sum.Cost >= 0.05 {
		right = sMuted.Render(shortMoney(sum.Cost))
	}
	return spread(left, right, w)
}

func (m Model) smallRoom(r officeRoom, now time.Time) []string {
	c := m.chantiers[r.item]
	inner := r.w - 2
	mark := " "
	switch {
	case c.Waiting() != "":
		mark = sWarn.Render("⚠")
	case m.ciFailed(c):
		mark = sErr.Render("✗")
	}
	title := ansi.Truncate(mark+" "+sBold.Render(c.Key)+" "+sSoft.Render(c.Title()), inner, "…")
	content := []string{title, m.canvasFor(r, now, false), m.roomStatus(c, now, inner)}
	border := line
	if r.item == m.cursor {
		border = accent
	}
	if c.Waiting() != "" && r.item != m.cursor {
		border = warn
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Width(inner).
		Render(strings.Join(content, "\n"))
	return strings.Split(box, "\n")
}

func (m Model) bigRoom(r officeRoom, now time.Time) []string {
	c := m.chantiers[r.item]
	title := sBold.Render(c.Key) + "  " + sSoft.Render(c.Title())
	var flow []string
	for _, p := range chantier.Flow {
		switch {
		case p < c.Phase:
			flow = append(flow, sGood.Render(p.Label()))
		case p == c.Phase:
			flow = append(flow, phaseStyle(p).Bold(true).Render(p.Label()+" ●"))
		default:
			flow = append(flow, sDim.Render(p.Label()))
		}
	}
	out := []string{spread(title, strings.Join(flow, sDim.Render(" → ")), r.w)}
	out = append(out, strings.Split(m.canvasFor(r, now, true), "\n")...)
	// Who is in the room, with their model.
	var who []string
	rep := m.usage[c.Key]
	for _, s := range rep.Subagent {
		if now.Sub(s.Last) < 10*time.Minute {
			who = append(who, lipgloss.NewStyle().Foreground(lipgloss.Color(string(roleColor(s.Type)))).Render("■ ")+s.Type+sMuted.Render(" ("+usage.ShortModel(s.Model)+")"))
		}
	}
	status := m.roomStatus(c, now, r.w)
	if len(who) > 0 {
		status = spread(m.roomStatus(c, now, r.w/2), strings.Join(who, "  "), r.w)
	}
	return append(out, "", status)
}

func roleColor(role string) office.Color { return office.Shirt(role) }

// ── keys and mouse ──────────────────────────────────────────────────────────

func (m Model) officeKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	per := 1
	if rs := m.officeRooms(); !m.officeZoom && len(rs) > 0 {
		per = max(1, (m.width+roomGapX)/(rs[0].w+roomGapX))
	}
	n := len(m.chantiers)
	moveTo := func(i int) {
		if i >= 0 && i < n {
			m.cursor, m.flash = i, ""
		}
	}
	switch k.String() {
	case "esc":
		if m.officeZoom {
			m.officeZoom = false
		} else {
			m.view = ""
		}
		return m, nil
	case "b":
		m.view = ""
		return m, nil
	case "left", "h":
		moveTo(m.cursor - 1)
		return m, nil
	case "right", "l":
		moveTo(m.cursor + 1)
		return m, nil
	case "up", "k":
		if m.officeZoom {
			moveTo(m.cursor - 1)
		} else {
			moveTo(m.cursor - per)
		}
		return m, nil
	case "down", "j":
		if m.officeZoom {
			moveTo(m.cursor + 1)
		} else {
			moveTo(m.cursor + per)
		}
		return m, nil
	case "enter":
		if !m.officeZoom && m.selected() != nil {
			m.officeZoom = true
			return m, nil
		}
	}
	return m.mainKey(k)
}

func (m Model) officeMouse(ev tea.MouseMsg) (tea.Model, tea.Cmd) {
	if ev.Action == tea.MouseActionMotion {
		_, m.hoverBtn = m.hitTest(ev.X, ev.Y)
		return m, nil
	}
	if ev.Button != tea.MouseButtonLeft || ev.Action != tea.MouseActionPress || m.busy != "" {
		return m, nil
	}
	if ev.Y == m.footerRow() {
		return m.footerClick(ev.X)
	}
	y := ev.Y - 2
	for _, r := range m.officeRooms() {
		if ev.X >= r.x && ev.X < r.x+r.w && y >= r.y && y < r.y+r.h {
			if r.item == m.cursor && !m.officeZoom {
				m.officeZoom = true // second click: step in
			}
			m.cursor, m.flash = r.item, ""
		}
	}
	return m, nil
}
