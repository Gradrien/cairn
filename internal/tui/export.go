package tui

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"cairn/internal/chantier"
	"cairn/internal/home"
	"cairn/internal/i18n"
	"cairn/internal/mux"
	"cairn/internal/native"
)

// The list, as the TUI shows it, is also written to ~/.cairn/cache/board.json
// for windows that draw it natively (the Herdr app's cairn panel). Written
// only when it changes; the reader watches the file.

func BoardFile() string { return native.BoardFile() }

type exportBoard struct {
	Lang      string            `json:"lang"`
	Groups    map[string]string `json:"groups"` // each section's title, in the interface language
	Mux       string            `json:"mux"`
	UI        string            `json:"ui,omitempty"`
	Home      string            `json:"home_workspace,omitempty"`
	Shipped   int               `json:"shipped"`
	ShippedOf []exportRef       `json:"shipped_list"`
	Others    []exportAgent     `json:"others"` // agents on no known chantier
	Chantiers []exportChantier  `json:"chantiers"`
}

type exportRef struct {
	Key   string `json:"key"`
	Title string `json:"title"`
}

type exportChantier struct {
	Key       string        `json:"key"`
	Project   string        `json:"project"`
	Title     string        `json:"title"`
	Phase     string        `json:"phase"`
	PhaseID   string        `json:"phase_id"`
	Group     string        `json:"group"` // wait, work, toplan, kept: the TUI's sections
	Waiting   string        `json:"waiting,omitempty"`
	CIFailed  bool          `json:"ci_failed,omitempty"`
	TasksDone int           `json:"tasks_done"`
	Tasks     int           `json:"tasks"`
	PR        *exportPR     `json:"pr,omitempty"`
	Workspace string        `json:"workspace,omitempty"`
	Agents    []exportAgent `json:"agents"`
}

type exportPR struct {
	Number  int    `json:"number"`
	URL     string `json:"url"`
	State   string `json:"state"`
	Review  string `json:"review,omitempty"`
	Passed  int    `json:"checks_passed"`
	Failed  int    `json:"checks_failed"`
	Pending int    `json:"checks_pending"`
}

type exportAgent struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
	Pane   string `json:"pane"`
}

func phaseID(p chantier.Phase) string {
	return map[chantier.Phase]string{chantier.ToPlan: "toplan", chantier.Plan: "plan", chantier.Build: "build",
		chantier.Review: "review", chantier.Shipped: "shipped"}[p]
}

func (m Model) board() exportBoard {
	b := exportBoard{ShippedOf: []exportRef{}, Others: []exportAgent{}, Lang: i18n.Lang(), Mux: mux.Name(), UI: m.cfg.UI, Chantiers: []exportChantier{}, Groups: map[string]string{
		"wait":   i18n.T("Attend ta décision", "Waiting for you"),
		"work":   i18n.T("En cours", "In progress"),
		"toplan": i18n.T("À planifier", "To plan"),
		"kept":   i18n.T("Livrés, pas encore rangés", "Shipped, not cleaned up"),
	}}
	if ws := m.snap.WorkspaceByLabel(home.Label); ws != nil {
		b.Home = ws.ID
	}
	for _, c := range m.all {
		if m.isShipped(c) {
			b.Shipped++
			b.ShippedOf = append(b.ShippedOf, exportRef{c.Key, c.Title()})
			continue
		}
		e := exportChantier{Key: c.Key, Project: c.Project, Title: c.Title(), Phase: c.Phase.Label(),
			PhaseID: phaseID(c.Phase), Group: m.groupOf(c), Waiting: c.Waiting(), CIFailed: m.ciFailed(c),
			Agents: []exportAgent{}}
		if c.State != nil {
			e.TasksDone, e.Tasks = c.State.DoneCount(), len(c.State.Tasks)
		}
		if pr := c.LastPR(); pr != nil {
			e.PR = &exportPR{Number: pr.Number, URL: pr.URL, State: pr.State}
			if p, ok := m.prs[c.Key]; ok && p.Err == "" {
				e.PR.Review = strings.ToLower(p.ReviewDecision)
				e.PR.Passed, e.PR.Failed, e.PR.Pending = p.Checks.Passed, p.Checks.Failed, p.Checks.Pending
			}
		}
		if len(c.Workspaces) > 0 {
			e.Workspace = c.Workspaces[0]
		}
		for _, a := range c.Agents {
			e.Agents = append(e.Agents, exportAgent{Name: a.Label(), Kind: a.Kind, Status: a.Status, Pane: a.PaneID})
		}
		b.Chantiers = append(b.Chantiers, e)
	}
	for _, a := range m.others {
		b.Others = append(b.Others, exportAgent{Name: a.Label(), Kind: a.Kind, Status: a.Status, Pane: a.PaneID})
	}
	return b
}

// exportBoard writes the board when it changed since the last write.
func (m *Model) exportBoard() {
	data, err := json.MarshalIndent(m.board(), "", "  ")
	if err != nil || bytes.Equal(data, m.exported) {
		return
	}
	path := BoardFile()
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil && os.Rename(tmp, path) == nil {
		m.exported = data
	}
}
