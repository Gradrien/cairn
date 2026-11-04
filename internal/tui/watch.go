package tui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"cairn/internal/chantier"
	"cairn/internal/config"
	"cairn/internal/herdr"
	"cairn/internal/journal"
	"cairn/internal/progress"
	"cairn/internal/state"
)

// What cairn watches in the background: phase history, decisions waiting for
// you, CI and reviews of every open PR — and the notifications they trigger.

const prsEvery = 2 * time.Minute

type tickPRs struct{}

type prsMsg map[string]progress.PR

// watchState is shared by every copy of the model (maps are references).
type watchState struct {
	history     map[string][]journal.Event
	prevWaiting map[string]string
	prevCI      map[string]int
	prevReview  map[string]string
	seen        map[string]bool // chantier observed once: no notification on startup
	prSeen      map[string]bool
	polling     bool
	// ready: Sprintosaur issues are loaded. Before that, a chantier with a PR
	// looks like Build (no PR known yet): journaling it would log a false
	// Build → Review on every start.
	ready bool
}

func newWatch() *watchState {
	return &watchState{
		history: map[string][]journal.Event{}, prevWaiting: map[string]string{}, prevCI: map[string]int{},
		prevReview: map[string]string{}, seen: map[string]bool{}, prSeen: map[string]bool{},
	}
}

func notify(title, body string) tea.Cmd {
	return func() tea.Msg { _ = herdr.Notify(title, body); return nil }
}

// observe records phase changes in each chantier's journal, keeps Sprintosaur
// in step, and notifies when a chantier starts waiting for you.
func (m *Model) observe() tea.Cmd {
	if !m.watch.ready {
		return nil
	}
	var cmds []tea.Cmd
	dir := config.ChantiersDir()
	for _, c := range m.chantiers {
		key := c.Key
		if c.Phase != chantier.ToPlan {
			events := journal.Read(dir, key)
			last, _ := journal.LastPhase(events)
			if now := c.Phase.String(); last != now {
				if journal.Append(dir, key, "phase:"+now) == nil {
					events = journal.Read(dir, key)
				}
				if last != "" && m.inside {
					cmds = append(cmds, m.onPhase(c, now))
				}
			}
			m.watch.history[key] = events
		}
		w := c.Waiting()
		if m.watch.seen[key] && m.inside && w != "" && m.watch.prevWaiting[key] != w {
			cmds = append(cmds, notify("cairn · "+key+" attend ta décision", w))
		}
		m.watch.prevWaiting[key] = w
		m.watch.seen[key] = true
	}
	return tea.Batch(cmds...)
}

// onPhase reacts to a phase change: Sprintosaur follows the work.
func (m *Model) onPhase(c *chantier.Chantier, phase string) tea.Cmd {
	switch phase {
	case chantier.Build.String():
		return m.ticketInProgress(c)
	case chantier.Shipped.String():
		return notify("cairn · "+c.Key+" livré", c.Title()+" — tu peux le ranger.")
	}
	return nil
}

// ticketInProgress moves a not-started ticket to In Progress.
func (m *Model) ticketInProgress(c *chantier.Chantier) tea.Cmd {
	if m.sp == nil || c.Issue == nil || (c.Issue.Status != "Backlog" && c.Issue.Status != "Todo") {
		return nil
	}
	sp, key := m.sp, c.Key
	return func() tea.Msg {
		if err := sp.SetStatus(key, "In Progress"); err != nil {
			return doneMsg{err: err}
		}
		return doneMsg{text: key + " passé en In Progress dans Sprintosaur.", refresh: true}
	}
}

// pollPRs reads every open PR of the list, so the list shows CI and review
// state and cairn can notify when they change.
func (m Model) pollPRs() tea.Cmd {
	urls := map[string]string{}
	for _, c := range m.chantiers {
		if pr := c.OpenPR(); pr != nil {
			urls[c.Key] = pr.URL
		}
	}
	return func() tea.Msg {
		out := prsMsg{}
		var mu sync.Mutex
		var wg sync.WaitGroup
		for key, url := range urls {
			wg.Add(1)
			go func() {
				defer wg.Done()
				pr := progress.ReadPR(url)
				mu.Lock()
				out[key] = pr
				mu.Unlock()
			}()
		}
		wg.Wait()
		return out
	}
}

// notePR stores a PR reading and notifies on CI breaking or a review verdict.
func (m *Model) notePR(key string, pr progress.PR) tea.Cmd {
	m.prs[key] = pr
	m.readAt[key] = time.Now()
	if pr.Err != "" {
		return nil
	}
	var cmds []tea.Cmd
	if m.watch.prSeen[key] && m.inside {
		if pr.Checks.Failed > 0 && m.watch.prevCI[key] == 0 {
			cmds = append(cmds, notify("cairn · "+key+" : CI en échec", strings.Join(pr.Checks.FailedNames, ", ")))
		}
		if pr.ReviewDecision != m.watch.prevReview[key] {
			switch pr.ReviewDecision {
			case "CHANGES_REQUESTED":
				cmds = append(cmds, notify("cairn · "+key+" : changements demandés", "Ouvre les commentaires de la PR dans cairn."))
			case "APPROVED":
				cmds = append(cmds, notify("cairn · "+key+" : review approuvée", "La PR est approuvée."))
			}
		}
	}
	m.watch.prevCI[key] = pr.Checks.Failed
	m.watch.prevReview[key] = pr.ReviewDecision
	m.watch.prSeen[key] = true
	return tea.Batch(cmds...)
}

type check struct {
	label string
	ok    bool
}

// readiness is the merge checklist of a chantier with an open PR.
func (m Model) readiness(c *chantier.Chantier) ([]check, bool) {
	if c.OpenPR() == nil {
		return nil, false
	}
	pr, ok := m.prs[c.Key]
	if !ok || pr.Err != "" {
		return nil, false
	}
	var out []check
	if s := c.State; s != nil {
		out = append(out, check{"tâches", len(s.Tasks) > 0 && s.DoneCount() == len(s.Tasks)})
		out = append(out, check{"effets de bord", len(s.SideEffects) > 0 && state.Count(s.SideEffects, true) == len(s.SideEffects)})
	}
	ck := pr.Checks
	out = append(out,
		check{"CI", ck.Failed == 0 && ck.Pending == 0 && ck.Passed > 0},
		check{"review", pr.ReviewDecision == "APPROVED"},
		check{"sans conflit", pr.Mergeable == "MERGEABLE"},
		check{"pas un brouillon", !pr.Draft},
	)
	ready := true
	for _, k := range out {
		ready = ready && k.ok
	}
	return out, ready
}

// since turns a start time into "depuis 2 j".
func since(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	ago := progress.Ago(t)
	if !strings.HasPrefix(ago, "il y a ") {
		return "depuis peu"
	}
	return "depuis " + strings.TrimPrefix(ago, "il y a ")
}

// historyLines renders the latest journal events, newest first.
func historyLines(events []journal.Event, n int) []string {
	var out []string
	for i := len(events) - 1; i >= 0 && len(out) < n; i-- {
		e := events[i]
		text := e.Text
		if p, ok := strings.CutPrefix(text, "phase:"); ok {
			text = "→ " + p
		}
		out = append(out, fmt.Sprintf("  %s  %s", sMuted.Render(e.At.Format("02/01 15:04")), text))
	}
	return out
}
