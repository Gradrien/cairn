package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"cairn/internal/chantier"
	"cairn/internal/i18n"
	"cairn/internal/native"
)

// The screens besides the chantier, as a native window draws them: taking a
// new ticket, the settings, the board by phase, the ctrl+k search. Each is
// the TUI's own state, and each action runs the TUI's own code.

type nativeIssue struct {
	Key      string `json:"key"`
	Title    string `json:"title"`
	Status   string `json:"status,omitempty"`
	Priority string `json:"priority,omitempty"`
	Estimate string `json:"estimate,omitempty"`
	URL      string `json:"url,omitempty"`
}

// nativePick is the new ticket screen (n).
type nativePick struct {
	Project  int           `json:"project"`
	Projects []string      `json:"projects"`
	Tracker  string        `json:"tracker"`
	Creates  bool          `json:"creates"` // a local tracker: type a title to create one
	Loading  bool          `json:"loading"`
	Input    string        `json:"input"`
	Issues   []nativeIssue `json:"issues"`
	Error    string        `json:"error,omitempty"`
}

// nativeSetting is one row of the settings (s): a section header, or a field.
type nativeSetting struct {
	Index   int    `json:"index"`
	Kind    string `json:"kind"` // header, text, cycle, delete, add
	Project string `json:"project,omitempty"`
	Field   string `json:"field,omitempty"`
	Label   string `json:"label,omitempty"`
	Value   string `json:"value,omitempty"`
	Default bool   `json:"default,omitempty"`
	Raw     string `json:"raw,omitempty"` // what the file holds, to edit
	Hint    string `json:"hint,omitempty"`
	Confirm bool   `json:"confirm,omitempty"` // delete waits for a second click
}

type nativeCardRef struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Waiting string `json:"waiting,omitempty"`
	Tasks   string `json:"tasks,omitempty"`
}

type nativeColumn struct {
	Phase string          `json:"phase"`
	Cards []nativeCardRef `json:"cards"`
}

type nativePaletteEntry struct {
	Icon  string `json:"icon"`
	Label string `json:"label"`
	Hint  string `json:"hint,omitempty"`
}

// nativePalette is ctrl+k: every action of every chantier, every chantier,
// the shipped tickets and the global commands; the window filters them as
// matchWords does and runs one by its index.
type nativePalette struct {
	Query   string               `json:"query"`
	Entries []nativePaletteEntry `json:"entries"`
}

func (m Model) nativePick() *nativePick {
	p := m.pick
	if p == nil {
		return nil
	}
	out := &nativePick{Project: p.project, Loading: p.loading, Input: p.input, Creates: m.pickCreates(), Issues: []nativeIssue{}}
	for _, q := range m.projects {
		out.Projects = append(out.Projects, q.cfg.Name)
	}
	if q := m.pickProject(); q != nil {
		out.Tracker = q.cfg.Tracker
		if q.tr != nil {
			out.Tracker = q.tr.Name()
		} else if q.err != nil {
			out.Error = q.err.Error()
		}
	}
	for _, is := range p.list {
		out.Issues = append(out.Issues, nativeIssue{Key: strings.ToUpper(is.ID), Title: is.Title, Status: is.Status, Priority: is.Priority, Estimate: is.Estimate, URL: is.URL})
	}
	return out
}

func (m Model) nativeSettings() []nativeSetting {
	s := m.settings
	if s == nil {
		return nil
	}
	var out []nativeSetting
	for i, r := range s.rows() {
		row := nativeSetting{Index: i, Field: r.field}
		switch {
		case r.proj == -1 && r.field == "":
			row.Kind, row.Label = "header", i18n.T("Pour tous les projets", "For every project")
		case r.proj == -2 && r.field == "":
			row.Kind, row.Label = "header", i18n.T("Nouveau projet", "New project")
		case r.field == "":
			row.Kind, row.Label = "header", s.filled[r.proj].Name
		case r.field == "delete":
			row.Kind, row.Label, row.Confirm = "delete", fieldLabel(r.field), s.confirm == i
		case r.field == "add":
			row.Kind, row.Label = "add", fieldLabel(r.field)
		case r.field == "tracker" || (r.field == "lang" && r.proj < 0):
			row.Kind, row.Label = "cycle", fieldLabel(r.field)
		default:
			row.Kind, row.Label = "text", fieldLabel(r.field)
		}
		if r.proj >= 0 {
			row.Project = s.filled[r.proj].Name
		}
		if row.Kind == "text" || row.Kind == "cycle" {
			row.Value, row.Default = s.shown(r)
			row.Raw = s.get(r)
			if r.field == "api_key" {
				row.Raw = ""
			}
			tr := ""
			if r.proj >= 0 {
				tr = s.filled[r.proj].Tracker
			}
			row.Hint = fieldHint(r, tr)
		}
		out = append(out, row)
	}
	return out
}

func (m Model) nativeBoard() []nativeColumn {
	var out []nativeColumn
	for _, p := range boardPhases {
		col := nativeColumn{Phase: p.Label(), Cards: []nativeCardRef{}}
		for _, c := range m.chantiers {
			if c.Phase != p {
				continue
			}
			ref := nativeCardRef{Key: c.Key, Title: c.Title(), Waiting: c.Waiting()}
			if c.State != nil && len(c.State.Tasks) > 0 {
				ref.Tasks = strings.TrimSpace(nativeText(taskCount(c)))
			}
			col.Cards = append(col.Cards, ref)
		}
		out = append(out, col)
	}
	return out
}

func taskCount(c *chantier.Chantier) string {
	return fmt.Sprintf("%d/%d", c.State.DoneCount(), len(c.State.Tasks))
}

func (m Model) nativePalette() *nativePalette {
	out := &nativePalette{Query: m.palQuery, Entries: []nativePaletteEntry{}}
	for _, e := range m.paletteEntries() {
		out.Entries = append(out.Entries, nativePaletteEntry{Icon: nativeText(e.icon), Label: e.label, Hint: nativeText(e.hint)})
	}
	return out
}

// runViewRequest runs the requests of these screens; ok is false for others.
func (m Model) runViewRequest(r native.Request) (Model, tea.Cmd, bool) {
	var next tea.Model = m
	var cmd tea.Cmd
	switch r.Action {
	case "palette":
		m.overlay, m.palQuery, m.palCursor = "palette", r.Text, 0
		return m, nil, true
	case "palette-run":
		entries := m.paletteEntries()
		if r.Index < 0 || r.Index >= len(entries) {
			return m, nil, true
		}
		m.overlay = ""
		next, cmd = entries[r.Index].run(m)
	case "pick-take":
		if m.pick == nil || r.Index < 0 || r.Index >= len(m.pick.list) {
			return m, nil, true
		}
		next, cmd = m.startTicket(m.pickProject(), m.pick.list[r.Index])
	case "pick-type":
		if m.pick == nil {
			return m, nil, true
		}
		m.pick.input, m.pick.cursor = r.Text, -1
		if !m.pickCreates() {
			m.pick.input = strings.ToUpper(strings.TrimSpace(r.Text))
		}
		next, cmd = m.pickKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	case "pick-project":
		if m.pick == nil || r.Index < 0 || r.Index >= len(m.projects) {
			return m, nil, true
		}
		m.pick.project, m.pick.input, m.pick.cursor = r.Index, "", -1
		return m, m.pickLoad(), true
	case "settings-set":
		if m.settings == nil || r.Index < 0 || r.Index >= len(m.settings.rows()) {
			return m, nil, true
		}
		m.settings.cursor, m.settings.editing = r.Index, false
		next, cmd = m.settingsCommit(m.settings.rows()[r.Index], r.Text)
	case "settings-do":
		if m.settings == nil || r.Index < 0 || r.Index >= len(m.settings.rows()) {
			return m, nil, true
		}
		next, cmd = m.settingsActivate(r.Index)
		if mm := next.(Model); mm.settings != nil {
			mm.settings.editing = false // the window edits in its own box
			next = mm
		}
	case "select-other":
		for i, a := range m.others {
			if a.PaneID == r.Arg {
				m.cursor, m.view, m.overlay = len(m.chantiers)+i, "", ""
			}
		}
		return m, nil, true
	default:
		return m, nil, false
	}
	return next.(Model), cmd, true
}
