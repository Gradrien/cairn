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
	"cairn/internal/i18n"
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
	keep        map[string]string    // shipped chantiers left as is, and why
	cleanedAt   map[string]time.Time // last cleanup attempt
	polling     bool
	lang        string // language of prevWaiting: a toggle is not a new wait
	// ready: the trackers' issues are loaded. Before that, a chantier with a PR
	// looks like Build (no PR known yet): journaling it would log a false
	// Build → Review on every start.
	ready bool
}

func newWatch() *watchState {
	return &watchState{
		history: map[string][]journal.Event{}, prevWaiting: map[string]string{}, prevCI: map[string]int{},
		prevReview: map[string]string{}, seen: map[string]bool{}, prSeen: map[string]bool{},
		keep: map[string]string{}, cleanedAt: map[string]time.Time{},
	}
}

func notify(title, body string) tea.Cmd {
	return func() tea.Msg { _ = herdr.Notify(title, body); return nil }
}

// observe records phase changes in each chantier's journal, keeps the tracker
// in step, and notifies when a chantier starts waiting for you.
func (m *Model) observe() tea.Cmd {
	if !m.watch.ready {
		return nil
	}
	var cmds []tea.Cmd
	dir := config.ChantiersDir()
	sameLang := m.watch.lang == i18n.Lang()
	m.watch.lang = i18n.Lang()
	// Every chantier, the shipped ones the list hides included: those are
	// the ones to clean up.
	all := m.all
	if all == nil {
		all = m.chantiers
	}
	for _, c := range all {
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
		// Shipped: clean it up (one cairn does it, the one in its workspace),
		// retrying now and then if something kept it.
		if c.Phase == chantier.Shipped && m.inside && time.Since(m.watch.cleanedAt[key]) > 2*time.Minute {
			m.watch.cleanedAt[key] = time.Now()
			cmds = append(cmds, m.cleanup(c, false))
		}
		w := c.Waiting()
		if m.watch.seen[key] && m.inside && sameLang && w != "" && m.watch.prevWaiting[key] != w {
			cmds = append(cmds, notify("cairn · "+key+i18n.T(" attend ta décision", " is waiting for you"), w))
		}
		m.watch.prevWaiting[key] = w
		m.watch.seen[key] = true
	}
	return tea.Batch(cmds...)
}

// onPhase reacts to a phase change: the tracker follows the work.
func (m *Model) onPhase(c *chantier.Chantier, phase string) tea.Cmd {
	switch phase {
	case chantier.Build.String():
		return m.ticketInProgress(c)
	case chantier.Shipped.String():
		return notify("cairn · "+c.Key+i18n.T(" livré", " shipped"), c.Title())
	}
	return nil
}

// ticketInProgress moves a not-started ticket to in progress in its tracker.
func (m *Model) ticketInProgress(c *chantier.Chantier) tea.Cmd {
	p := m.projectOf(c)
	if p == nil || p.tr == nil || c.Issue == nil || c.Issue.Started {
		return nil
	}
	tr, is, key := p.tr, *c.Issue, c.Key
	return func() tea.Msg {
		moved, err := tr.Start(is)
		if err != nil {
			return doneMsg{err: err}
		}
		if !moved {
			return nil
		}
		return doneMsg{text: fmt.Sprintf(i18n.T("%s passé en cours dans %s.", "%s moved to in progress in %s."), key, tr.Name()), refresh: true}
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
			cmds = append(cmds, notify("cairn · "+key+i18n.T(" : CI en échec", ": CI failing"), strings.Join(pr.Checks.FailedNames, ", ")))
		}
		if pr.ReviewDecision != m.watch.prevReview[key] {
			switch pr.ReviewDecision {
			case "CHANGES_REQUESTED":
				cmds = append(cmds, notify("cairn · "+key+i18n.T(" : changements demandés", ": changes requested"), i18n.T("Ouvre les commentaires de la PR dans cairn.", "Open the PR comments in cairn.")))
			case "APPROVED":
				cmds = append(cmds, notify("cairn · "+key+i18n.T(" : review approuvée", ": review approved"), i18n.T("La PR est approuvée.", "The PR is approved.")))
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
		out = append(out, check{i18n.T("tâches", "tasks"), len(s.Tasks) > 0 && s.DoneCount() == len(s.Tasks)})
		out = append(out, check{i18n.T("effets de bord", "side effects"), len(s.SideEffects) > 0 && state.Count(s.SideEffects, true) == len(s.SideEffects)})
	}
	ck := pr.Checks
	out = append(out,
		check{"CI", ck.Failed == 0 && ck.Pending == 0 && ck.Passed > 0},
		check{"review", pr.ReviewDecision == "APPROVED"},
		check{i18n.T("sans conflit", "no conflicts"), pr.Mergeable == "MERGEABLE"},
		check{i18n.T("pas un brouillon", "not a draft"), !pr.Draft},
	)
	ready := true
	for _, k := range out {
		ready = ready && k.ok
	}
	return out, ready
}

// since turns a start time into "depuis 2 j" / "for 2 d".
func since(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := progress.Elapsed(t)
	if d == "" {
		return i18n.T("depuis peu", "just now")
	}
	return fmt.Sprintf(i18n.T("depuis %s", "for %s"), d)
}

// historyText translates a journal event for display. Events are stored in
// French (stable data); unknown text passes through unchanged.
func historyText(text string) string {
	if p, ok := strings.CutPrefix(text, "phase:"); ok {
		for _, ph := range []chantier.Phase{chantier.ToPlan, chantier.Plan, chantier.Build, chantier.Review, chantier.Shipped} {
			if ph.String() == p {
				return "→ " + ph.Label()
			}
		}
		return "→ " + p
	}
	if text == "effet de bord vérifié" {
		return i18n.T(text, "side effect checked")
	}
	if model, ok := strings.CutPrefix(text, "modèle : "); ok {
		return i18n.T(text, "model: "+model)
	}
	switch text {
	case "plan validé":
		return i18n.T(text, "plan approved")
	case "push autorisé":
		return i18n.T(text, "push allowed")
	case "rangé":
		return i18n.T(text, "put away")
	case "CI envoyée à l'agent":
		return i18n.T(text, "CI sent to the agent")
	case "réponse envoyée à l'agent":
		return i18n.T(text, "answer sent to the agent")
	}
	if name, ok := strings.CutPrefix(text, "plan lancé ("); ok && strings.HasSuffix(name, ")") {
		return fmt.Sprintf(i18n.T("plan lancé (%s)", "plan started (%s)"), strings.TrimSuffix(name, ")"))
	}
	if who, ok := strings.CutPrefix(text, "retour de "); ok {
		if who, ok := strings.CutSuffix(who, " envoyé à l'agent"); ok {
			return fmt.Sprintf(i18n.T("retour de %s envoyé à l'agent", "feedback from %s sent to the agent"), who)
		}
	}
	return text
}

// historyLines renders the latest journal events, newest first.
func historyLines(events []journal.Event, n int) []string {
	var out []string
	for i := len(events) - 1; i >= 0 && len(out) < n; i-- {
		e := events[i]
		out = append(out, fmt.Sprintf("  %s  %s", sMuted.Render(e.At.Format("02/01 15:04")), historyText(e.Text)))
	}
	return out
}
