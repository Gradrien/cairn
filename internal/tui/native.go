package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"

	"cairn/internal/chantier"
	"cairn/internal/i18n"
	"cairn/internal/native"
	"cairn/internal/progress"
	"cairn/internal/state"
)

// A native window (the Herdr app's cairn panel) draws the selected chantier
// from ~/.cairn/cache/detail.json and acts on it by dropping requests in
// ~/.cairn/cache/requests/. The model stays the one engine: the card, the
// sections and their order, the confirmations are the TUI's own, and a
// request runs the same code as the key or click it stands for.

func nativeText(s string) string { return strings.TrimRight(xansi.Strip(s), " ") }

type nativeButton struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Enabled bool   `json:"enabled"`
}

type nativeCard struct {
	Tone    string         `json:"tone"` // warn, danger, good, accent, line, dim
	Title   string         `json:"title"`
	Since   string         `json:"since,omitempty"`
	Sub     string         `json:"sub,omitempty"`
	Primary bool           `json:"primary"`
	Actions []nativeButton `json:"actions"`
}

type nativeStep struct {
	Label string `json:"label"`
	State string `json:"state"` // done, current, todo
}

type nativeRow struct {
	Label string `json:"label"`
	Tone  string `json:"tone"`
	Text  string `json:"text"`
	Note  string `json:"note,omitempty"`
}

type nativeItem struct {
	Text  string `json:"text"`
	Image string `json:"image,omitempty"` // a proof's screenshot, to open
	Thumb string `json:"thumb,omitempty"` // its copy the app can show, relative to the app's folder
	Gone  bool   `json:"gone,omitempty"`  // the screenshot's file no longer exists
	Depth int    `json:"depth,omitempty"`
	Plain bool   `json:"plain,omitempty"`
	Code  bool   `json:"code,omitempty"`
}

type nativeTask struct {
	Index    int          `json:"index"`
	Text     string       `json:"text"`
	State    string       `json:"state"` // done, doing, todo; reported for a manual step that fails
	Body     []nativeItem `json:"body,omitempty"`
	Reported string       `json:"reported,omitempty"`
}

type nativeOption struct {
	Key         string `json:"key"`
	Text        string `json:"text"`
	Recommended bool   `json:"recommended,omitempty"`
}

type nativeQuestion struct {
	Index          int            `json:"index"`
	Text           string         `json:"text"`
	Body           []nativeItem   `json:"body,omitempty"`
	Options        []nativeOption `json:"options,omitempty"`
	Recommendation string         `json:"recommendation,omitempty"`
	Answered       string         `json:"answered,omitempty"`
}

type nativeCheck struct {
	Label string `json:"label"`
	OK    bool   `json:"ok"`
}

type nativePR struct {
	Number    int           `json:"number"`
	URL       string        `json:"url"`
	State     string        `json:"state"`
	Line      string        `json:"line,omitempty"`
	Error     string        `json:"error,omitempty"`
	Failed    []string      `json:"failed,omitempty"`
	Checks    []nativeCheck `json:"checks,omitempty"`
	Ready     bool          `json:"ready"`
	Checklist bool          `json:"checklist"` // the merge checklist leads the section
}

type nativeCode struct {
	Ahead       int    `json:"ahead"`
	Files       int    `json:"files"`
	Added       int    `json:"added"`
	Removed     int    `json:"removed"`
	Uncommitted int    `json:"uncommitted"`
	Last        string `json:"last,omitempty"`
	Branch      string `json:"branch"`
	Folder      string `json:"folder"`
	Main        bool   `json:"main,omitempty"`
	Error       string `json:"error,omitempty"`
	Reading     bool   `json:"reading,omitempty"`
}

// nativeSection is one section of the detail. Interactive ones carry their
// data; the others (news, how it works, history, cost, agents' last actions)
// their lines as the TUI writes them.
type nativeSection struct {
	ID        string           `json:"id"`
	Title     string           `json:"title,omitempty"`
	Count     string           `json:"count,omitempty"`
	Tone      string           `json:"tone,omitempty"`
	Hint      string           `json:"hint,omitempty"`
	Questions []nativeQuestion `json:"questions,omitempty"`
	Tasks     []nativeTask     `json:"tasks,omitempty"`
	Setup     []string         `json:"setup,omitempty"`
	Items     []nativeItem     `json:"items,omitempty"`
	Text      string           `json:"text,omitempty"`
	PR        *nativePR        `json:"pr,omitempty"`
	Code      *nativeCode      `json:"code,omitempty"`
	Lines     []string         `json:"lines,omitempty"`
}

type nativeOverlay struct {
	Kind    string         `json:"kind"` // push, close, localtest
	Lines   []string       `json:"lines"`
	Buttons []nativeButton `json:"buttons"`
}

type nativeFlash struct {
	Text  string `json:"text"`
	Error bool   `json:"error,omitempty"`
}

// nativeHow is the "how it works" view: the agent's explanation, or for a
// ticket shipped before it wrote one, its plan.
type nativeHow struct {
	Key      string `json:"key"`
	Title    string `json:"title"`
	Markdown string `json:"markdown,omitempty"`
	Note     string `json:"note,omitempty"`
	Archived bool   `json:"archived,omitempty"`
	CanAsk   bool   `json:"can_ask"`
}

type nativeEntry struct {
	Index  int    `json:"index"`
	Author string `json:"author"`
	Kind   string `json:"kind"`
	State  string `json:"state,omitempty"`
	Where  string `json:"where,omitempty"`
	Ago    string `json:"ago"`
	Body   string `json:"body"`
	Bot    bool   `json:"bot,omitempty"`
	New    bool   `json:"new,omitempty"`
}

type nativeImage struct {
	N      int    `json:"n"`
	URL    string `json:"url"`
	Broken bool   `json:"broken,omitempty"`
}

// nativeComments is the PR conversation view.
type nativeComments struct {
	Number   int           `json:"number"`
	URL      string        `json:"url"`
	Error    string        `json:"error,omitempty"`
	Loading  bool          `json:"loading,omitempty"`
	ShowBots bool          `json:"show_bots"`
	Entries  []nativeEntry `json:"entries"`
	Images   []nativeImage `json:"images"`
}

// nativeOther is an agent on no known ticket, which can be attached to one.
type nativeOther struct {
	Name    string   `json:"name"`
	Status  string   `json:"status"`
	Title   string   `json:"title"`
	Dir     string   `json:"dir"`
	Pane    string   `json:"pane"`
	Targets []string `json:"targets"`
}

type nativeDetail struct {
	Lang     string          `json:"lang"`
	View     string          `json:"view"` // "", how, comments, pick, settings, board
	Buttons  []nativeButton  `json:"buttons"`
	Primary  bool            `json:"primary"` // the first button is what to do now
	How      *nativeHow      `json:"how,omitempty"`
	Pick     *nativePick     `json:"pick,omitempty"`
	Settings []nativeSetting `json:"settings,omitempty"`
	Board    []nativeColumn  `json:"board,omitempty"`
	Palette  *nativePalette  `json:"palette,omitempty"`
	Comments *nativeComments `json:"comments,omitempty"`
	Other    *nativeOther    `json:"other,omitempty"`
	Key      string          `json:"key,omitempty"`
	Title    string          `json:"title,omitempty"`
	Meta     string          `json:"meta,omitempty"`
	URL      string          `json:"url,omitempty"`
	Card     *nativeCard     `json:"card,omitempty"`
	Flow     []nativeStep    `json:"flow,omitempty"`
	Since    string          `json:"since,omitempty"`
	Status   []nativeRow     `json:"status,omitempty"`
	NoPlan   bool            `json:"no_plan,omitempty"`
	Sections []nativeSection `json:"sections,omitempty"`
	Agents   []exportAgent   `json:"agents,omitempty"`
	Plan     string          `json:"plan,omitempty"`
	Lint     int             `json:"lint,omitempty"`
	Overlay  *nativeOverlay  `json:"overlay,omitempty"`
	Flash    *nativeFlash    `json:"flash,omitempty"`
	Busy     string          `json:"busy,omitempty"`
}

func toneOf(c lipgloss.TerminalColor) string {
	switch c {
	case warn:
		return "warn"
	case danger:
		return "danger"
	case good:
		return "good"
	case accent:
		return "accent"
	case dim:
		return "dim"
	}
	return "line"
}

func nativeItems(items []state.Item) []nativeItem {
	var out []nativeItem
	for _, it := range items {
		out = append(out, nativeItem{Text: it.Text, Depth: it.Depth, Plain: it.Plain, Code: it.Code})
	}
	return out
}

func untilde(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func (m Model) nativeHow() *nativeHow {
	h := m.howSubj()
	if h == nil {
		return nil
	}
	out := &nativeHow{Key: h.key, Title: h.title, Markdown: h.plan.HowItWorks, Archived: !h.archivedAt.IsZero(), CanAsk: m.askDir(h) != ""}
	switch {
	case out.Markdown != "":
	case out.Archived:
		out.Note = i18n.T("Livré avant que l'agent l'explique : voici son plan, ce qui a été fait.", "Shipped before the agent explained it: here is its plan, what was done.")
		body := strings.TrimSpace(h.plan.Body())
		if k, rest, ok := strings.Cut(body, "\n"); ok && strings.HasPrefix(k, "# ") {
			body = rest
		}
		out.Markdown = body
	default:
		out.Note = i18n.T("L'agent n'a pas encore expliqué comment ce ticket est construit.", "The agent has not explained yet how this ticket is built.")
		if h.agent() == "" {
			out.Note += " " + i18n.T("Aucun agent sur ce chantier pour l'écrire.", "No agent on this ticket to write it.")
		}
	}
	return out
}

func (m Model) nativeComments() *nativeComments {
	url, num := m.prURLOf()
	out := &nativeComments{Number: num, URL: url, ShowBots: m.showBots, Entries: []nativeEntry{}, Images: []nativeImage{}}
	switch {
	case m.threadErr != nil:
		out.Error = m.threadErr.Error()
		return out
	case m.thread == nil:
		out.Loading = true
		return out
	}
	since, tracked := time.Time{}, false
	if m.seen != nil {
		since, tracked = m.seen.get(m.threadKey)
	}
	for i, e := range m.thread.Entries {
		ne := nativeEntry{Index: i, Author: e.Author, Kind: e.Kind, State: e.State, Ago: progress.Ago(e.At), Body: e.Body, Bot: e.Bot,
			New: tracked && e.Kind != "description" && e.At.After(since)}
		if e.Kind == "inline" {
			ne.Where = fmt.Sprintf("%s:%d", e.Path, e.Line)
		}
		out.Entries = append(out.Entries, ne)
	}
	for i, u := range m.thread.Images {
		out.Images = append(out.Images, nativeImage{N: i + 1, URL: u, Broken: m.thread.Broken[i+1]})
	}
	return out
}

func nativeTasks(ts []state.Task) []nativeTask {
	var out []nativeTask
	for i, t := range ts {
		st := "todo"
		switch {
		case t.Done:
			st = "done"
		case t.Doing:
			st = "doing"
		}
		out = append(out, nativeTask{Index: i, Text: t.Text, State: st, Body: nativeItems(t.Body)})
	}
	return out
}

// lines runs a TUI section and keeps its text.
func (m Model) lines(c *chantier.Chantier, id string) []string {
	b := &detailBuilder{w: 72, detail: detail{cardRow: -1}}
	m.section(b, c, id)
	var out []string
	for _, l := range b.lines {
		out = append(out, nativeText(l))
	}
	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	return out
}

func (m Model) nativeSection(c *chantier.Chantier, id string) *nativeSection {
	s := c.State
	switch id {
	case "questions":
		if s == nil || len(s.OpenQuestions()) == 0 {
			return nil
		}
		sec := &nativeSection{ID: id, Title: i18n.T("Questions ouvertes", "Open questions"), Tone: "warn"}
		for i, q := range s.OpenQuestions() {
			nq := nativeQuestion{Index: i, Text: q.Text, Body: nativeItems(q.Body), Recommendation: q.Recommendation,
				Answered: m.answered[answerKey(c.Key, q.Text)]}
			for j, o := range q.Options {
				nq.Options = append(nq.Options, nativeOption{Key: o.Key, Text: o.Text, Recommended: j == q.Recommended})
			}
			sec.Questions = append(sec.Questions, nq)
		}
		sec.Count = fmt.Sprint(len(sec.Questions))
		return sec
	case "tasks":
		if s == nil || len(s.Tasks) == 0 {
			return nil
		}
		done := state.Count(s.Tasks, true)
		tone := "muted"
		if done == len(s.Tasks) {
			tone = "good"
		}
		return &nativeSection{ID: id, Title: i18n.T("Tâches", "Tasks"), Count: fmt.Sprintf("%d/%d", done, len(s.Tasks)), Tone: tone, Tasks: nativeTasks(s.Tasks)}
	case "side":
		if s == nil {
			return nil
		}
		if len(s.SideEffects) == 0 {
			if c.Phase >= chantier.Build {
				return &nativeSection{ID: id, Title: i18n.T("Effets de bord", "Side effects"), Tone: "warn", Hint: i18n.T("aucun recensé dans le plan", "none listed in the plan")}
			}
			return nil
		}
		done := state.Count(s.SideEffects, true)
		tone := "muted"
		if done < len(s.SideEffects) && (c.Phase >= chantier.Build || isPushWait(s.Waiting)) {
			tone = "warn"
		} else if done == len(s.SideEffects) {
			tone = "good"
		}
		sec := &nativeSection{ID: id, Title: i18n.T("Effets de bord", "Side effects"), Tone: tone,
			Count: fmt.Sprintf(i18n.T("%d/%d vérifiés", "%d/%d checked"), done, len(s.SideEffects)), Tasks: nativeTasks(s.SideEffects)}
		if done < len(s.SideEffects) {
			sec.Hint = i18n.T("clic : cocher ce que tu as vérifié", "click: tick what you checked")
		}
		return sec
	case "manual":
		if s == nil || (len(s.ManualSteps) == 0 && len(s.ManualSetup) == 0) {
			if s != nil && c.Phase >= chantier.Build {
				return &nativeSection{ID: id, Title: i18n.T("Tester à la main", "Manual test"), Hint: i18n.T("pas encore de protocole dans le plan", "no protocol in the plan yet")}
			}
			return nil
		}
		sec := &nativeSection{ID: id, Title: i18n.T("Tester à la main", "Manual test"), Setup: s.ManualSetup, Tasks: nativeTasks(s.ManualSteps)}
		if n := len(s.ManualSteps); n > 0 {
			done := state.Count(s.ManualSteps, true)
			sec.Count, sec.Tone = fmt.Sprintf("%d/%d", done, n), "muted"
			if done == n {
				sec.Tone = "good"
			}
			sec.Hint = i18n.T("clic sur une étape une fois essayée", "click a step once tried")
		}
		for i := range sec.Tasks {
			if r, ok := m.answered[manualKey(c.Key, sec.Tasks[i].Text)]; ok && sec.Tasks[i].State != "done" {
				sec.Tasks[i].State, sec.Tasks[i].Reported = "reported", r
			}
		}
		return sec
	case "goal":
		if s == nil || len(s.Summary) == 0 {
			return nil
		}
		return &nativeSection{ID: id, Title: i18n.T("Objectif", "Goal"), Text: strings.Join(s.Summary, " ")}
	case "approach":
		if s == nil || len(s.Approach) == 0 {
			return nil
		}
		return &nativeSection{ID: id, Title: i18n.T("Approche", "Approach"), Items: nativeItems(s.Approach)}
	case "proofs":
		if s == nil || len(s.Proofs) == 0 {
			return nil
		}
		sec := &nativeSection{ID: id, Title: i18n.T("Preuves", "Proofs"), Count: fmt.Sprint(len(s.Proofs))}
		for _, p := range s.Proofs {
			it := nativeItem{Text: p, Image: untilde(imagePath(p))}
			if it.Image != "" {
				it.Thumb = native.Thumb(it.Image)
				it.Gone = it.Thumb == ""
			}
			sec.Items = append(sec.Items, it)
		}
		return sec
	case "how":
		plan, _ := m.planOf(c)
		if plan == nil || (plan.HowItWorks == "" && c.Phase < chantier.Build) {
			return nil
		}
		sec := &nativeSection{ID: id, Title: howTitle(), Hint: i18n.T("pour répondre aux questions sur le ticket", "to answer questions on the ticket"), Text: plan.HowItWorks}
		if plan.HowItWorks == "" {
			sec.Lines = []string{i18n.T("Pas encore écrit par l'agent.", "Not written by the agent yet.")}
		}
		return sec
	case "code":
		if c.Worktree == "" {
			return nil
		}
		code := &nativeCode{Branch: c.Branch, Folder: tilde(c.Worktree), Main: c.Main}
		if g, ok := m.gits[c.Key]; ok {
			code.Ahead, code.Files, code.Added, code.Removed, code.Uncommitted, code.Error = g.Ahead, g.Files, g.Added, g.Removed, g.Uncommitted, g.Err
			if g.Ahead > 0 {
				code.Last = g.LastSubject
			}
		} else {
			code.Reading = true
		}
		return &nativeSection{ID: id, Title: "Code", Code: code}
	case "pr", "pr+":
		last := c.LastPR()
		if last == nil {
			return nil
		}
		pr := &nativePR{Number: last.Number, URL: last.URL, State: last.State, Checklist: id == "pr+"}
		if info, ok := m.prs[c.Key]; ok {
			if info.Err != "" {
				pr.Error = info.Err
			} else {
				pr.Line, pr.Failed = nativeText(prLine(info)), info.Checks.FailedNames
				checks, ready := m.readiness(c)
				for _, k := range checks {
					pr.Checks = append(pr.Checks, nativeCheck{k.label, k.ok})
				}
				pr.Ready = ready
			}
		}
		return &nativeSection{ID: "pr", Title: fmt.Sprintf("PR #%d", last.Number), PR: pr}
	}
	if l := m.lines(c, id); len(l) > 0 {
		return &nativeSection{ID: id, Lines: l}
	}
	return nil
}

func (m Model) nativeDetail() nativeDetail {
	d := nativeDetail{Lang: i18n.Lang(), Busy: m.busy, View: m.view, Primary: m.primaryFirst(), Buttons: []nativeButton{}}
	for _, b := range m.buttons() {
		d.Buttons = append(d.Buttons, nativeButton{Key: b.key, Label: b.label, Enabled: b.enabled})
	}
	switch m.view {
	case "how":
		d.How = m.nativeHow()
	case "comments":
		d.Comments = m.nativeComments()
	case "pick":
		d.Pick = m.nativePick()
	case "settings":
		d.Settings = m.nativeSettings()
	case "board":
		d.Board = m.nativeBoard()
	}
	if m.overlay == "palette" {
		d.Palette = m.nativePalette()
	}
	if a := m.selectedOther(); a != nil {
		o := &nativeOther{Name: a.Label(), Status: a.Status, Title: a.Title, Dir: tilde(a.Dir()), Pane: a.PaneID, Targets: []string{}}
		for _, c := range m.chantiers {
			o.Targets = append(o.Targets, c.Key)
		}
		d.Other = o
	}
	if m.flash != "" {
		d.Flash = &nativeFlash{Text: nativeText(m.flash), Error: m.flashErr}
	}
	if m.overlay == "push" || m.overlay == "close" || m.overlay == "localtest" || m.overlay == "help" {
		box, x0, y0, hits, rows := m.overlayLayout()
		o := &nativeOverlay{Kind: m.overlay}
		buttonRows := map[int]bool{}
		for i, h := range hits {
			row := rows[i] - y0
			if row >= 0 && row < len(box) {
				buttonRows[row] = true
				label := strings.Trim(nativeText(xansi.Cut(box[row], h.x0-x0, h.x1-x0)), " []")
				o.Buttons = append(o.Buttons, nativeButton{Key: h.key, Label: label, Enabled: true})
			}
		}
		for i, l := range box {
			if i == 0 || i == len(box)-1 || buttonRows[i] {
				continue // the frame and the buttons, drawn natively
			}
			t := strings.TrimSpace(strings.Trim(nativeText(l), "│"))
			o.Lines = append(o.Lines, t)
		}
		d.Overlay = o
	}
	c := m.selected()
	if c == nil {
		return d
	}
	d.Key, d.Title = c.Key, c.Title()
	if c.Issue != nil {
		d.URL = c.Issue.URL
		parts := []string{}
		src := c.Project
		if p := m.projectOf(c); p != nil && p.tr != nil {
			src = p.tr.Name()
		}
		for _, s := range []string{src, c.Issue.Status, c.Issue.Priority, c.Issue.Estimate} {
			if s != "" {
				parts = append(parts, s)
			}
		}
		d.Meta = strings.Join(parts, " · ")
	}
	if cd := m.cardFor(c); cd != nil {
		nc := &nativeCard{Tone: toneOf(cd.color), Title: cd.title, Since: cd.since, Sub: cd.sub, Primary: cd.primary}
		for _, b := range cd.actions {
			nc.Actions = append(nc.Actions, nativeButton{Key: b.key, Label: b.label, Enabled: b.enabled})
		}
		d.Card = nc
	}
	if c.Phase != chantier.ToPlan {
		for _, p := range chantier.Flow {
			st := "todo"
			switch {
			case p < c.Phase:
				st = "done"
			case p == c.Phase:
				st = "current"
			}
			d.Flow = append(d.Flow, nativeStep{p.Label(), st})
		}
	} else {
		d.Flow = []nativeStep{{chantier.ToPlan.Label(), "current"}}
	}
	d.Status = make([]nativeRow, 0)
	for _, r := range m.statusRows(c) {
		d.Status = append(d.Status, nativeRow{r.label, r.tone, r.text, strings.TrimPrefix(r.note, " : ")})
	}
	d.NoPlan = c.State == nil && c.Phase != chantier.ToPlan
	for _, id := range append([]string{"news"}, append(m.sectionOrder(c), "history", "cost")...) {
		if sec := m.nativeSection(c, id); sec != nil {
			d.Sections = append(d.Sections, *sec)
		}
	}
	for _, a := range c.Agents {
		d.Agents = append(d.Agents, exportAgent{Name: a.Label(), Kind: a.Kind, Status: a.Status, Pane: a.PaneID})
	}
	if c.State != nil {
		d.Plan, d.Lint = tilde(c.State.Path), len(c.State.Lint())
	}
	return d
}

// exportDetail writes detail.json when it changed.
func (m *Model) exportDetail() {
	data, err := json.MarshalIndent(m.nativeDetail(), "", "  ")
	if err != nil || bytes.Equal(data, m.detailOut) {
		return
	}
	path := native.DetailFile()
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil && os.Rename(tmp, path) == nil {
		m.detailOut = data
	}
}

// runRequest runs one request on the model, the way the TUI's key or click would.
func (m Model) runRequest(r native.Request) (Model, tea.Cmd) {
	if mm, cmd, ok := m.runViewRequest(r); ok {
		return mm, cmd
	}
	if r.Key != "" {
		m.wantKey = strings.ToUpper(r.Key)
		m.focusWanted()
		if c := m.selected(); c == nil || c.Key != strings.ToUpper(r.Key) {
			m.say(fmt.Sprintf(i18n.T("%s n'est pas dans la liste.", "%s is not in the list."), r.Key), nil)
			return m, nil
		}
	}
	var next tea.Model = m
	var cmd tea.Cmd
	switch r.Action {
	case "", "select":
		return m, nil
	case "key":
		if r.Arg == "ctrl+c" || r.Arg == "U" {
			return m, nil // they stop the program: the engine stays
		}
		if m.overlay != "" || m.answerQ != "" {
			m.overlay, m.answerQ = "", ""
		}
		next, cmd = m.key(keyFor(r.Arg))
	case "confirm":
		next, cmd = m.overlayKey(tea.KeyMsg{Type: tea.KeyEnter})
	case "cancel":
		next, cmd = m.overlayKey(tea.KeyMsg{Type: tea.KeyEsc})
	case "overlay":
		next, cmd = m.overlayKey(keyFor(r.Arg))
	case "answer":
		next, cmd = m.startAnswer(r.Index)
		mm := next.(Model)
		if mm.answerQ == "" {
			return mm, cmd
		}
		if r.Option != nil {
			mm.answerOpt, mm.answerFree = *r.Option, false
		} else {
			mm.answerFree, mm.answerText = true, r.Text
		}
		next, cmd = mm.sendAnswer()
	case "side":
		next, cmd = m.tickSide(r.Index)
	case "manual-ok":
		next, cmd = m.manualClick("manual-ok", r.Index)
	case "manual-ko":
		next, cmd = m.manualClick("manual-ko", r.Index)
		if mm := next.(Model); mm.manualAsk {
			mm.manualText = r.Text
			next, cmd = mm.sendManualReport()
		}
	case "comment":
		next, cmd = m.sendComment(r.Index)
	case "ask":
		next, cmd = m.askAside(r.Text)
	case "attach":
		next, cmd = m.attach(strings.ToUpper(r.Arg))
	case "fold":
		c := m.selected()
		k := c.Key + ":" + r.Arg
		m.unfold[k] = !m.unfolded(c.Key, r.Arg)
		return m, nil
	}
	return next.(Model), cmd
}
