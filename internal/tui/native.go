package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"

	"cairn/internal/chantier"
	"cairn/internal/config"
	"cairn/internal/i18n"
	"cairn/internal/state"
)

// A native window (the Herdr app's cairn panel) draws the selected chantier
// from ~/.cairn/cache/detail.json and acts on it by dropping requests in
// ~/.cairn/cache/requests/. The model stays the one engine: the card, the
// sections and their order, the confirmations are the TUI's own, and a
// request runs the same code as the key or click it stands for.

func DetailFile() string         { return filepath.Join(config.Dir(), "cache", "detail.json") }
func RequestsDir() string        { return filepath.Join(config.Dir(), "cache", "requests") }
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

type nativeDetail struct {
	Lang     string          `json:"lang"`
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
			sec.Items = append(sec.Items, nativeItem{Text: p})
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
	d := nativeDetail{Lang: i18n.Lang(), Busy: m.busy}
	if m.flash != "" {
		d.Flash = &nativeFlash{Text: nativeText(m.flash), Error: m.flashErr}
	}
	if m.overlay == "push" || m.overlay == "close" || m.overlay == "localtest" {
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
	path := DetailFile()
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil && os.Rename(tmp, path) == nil {
		m.detailOut = data
	}
}

// nativeRequest is what the native window asks for. Key selects the
// chantier first; Action is then one of:
//   - "key": Arg is a TUI key ("a", "v", "p", "enter"…), run as if pressed;
//   - "confirm" / "cancel": the open confirmation, as enter / esc;
//   - "overlay": Arg is a key of the open confirmation's buttons;
//   - "answer": open question Index, with option Option, or Text in your words;
//   - "side": tick or untick side effect Index;
//   - "manual-ok" / "manual-ko": manual step Index works, or fails as Text says;
//   - "fold": unfold or fold section Arg ("tasks", "goal", "approach").
type nativeRequest struct {
	Key    string `json:"key"`
	Action string `json:"action"`
	Arg    string `json:"arg,omitempty"`
	Index  int    `json:"index,omitempty"`
	Option *int   `json:"option,omitempty"`
	Text   string `json:"text,omitempty"`
}

// Request drops a request for the running cairn, which runs it within 300 ms.
func Request(key, action, arg string, index int, option *int, text string) error {
	data, err := json.Marshal(nativeRequest{Key: key, Action: action, Arg: arg, Index: index, Option: option, Text: text})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(RequestsDir(), 0o755); err != nil {
		return err
	}
	name := fmt.Sprintf("%d-%d.json", time.Now().UnixNano(), os.Getpid())
	tmp := filepath.Join(RequestsDir(), "."+name)
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(RequestsDir(), name))
}

// takeRequests returns the waiting requests, oldest first, and removes them.
func takeRequests() []nativeRequest {
	dir := RequestsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var out []nativeRequest
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		_ = os.Remove(path)
		var r nativeRequest
		if err == nil && json.Unmarshal(data, &r) == nil {
			out = append(out, r)
		}
	}
	return out
}

// runRequest runs one request on the model, the way the TUI's key or click would.
func (m Model) runRequest(r nativeRequest) (Model, tea.Cmd) {
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
	case "fold":
		c := m.selected()
		k := c.Key + ":" + r.Arg
		m.unfold[k] = !m.unfolded(c.Key, r.Arg)
		return m, nil
	}
	return next.(Model), cmd
}
