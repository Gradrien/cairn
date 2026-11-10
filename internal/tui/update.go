package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"cairn/internal/i18n"
	"cairn/internal/update"
)

// Updates: cairn asks every hour whether main has moved past the commit it
// was built from. If so, it shows what changed and offers to update; nothing
// installs until you press U. Updating quits, pulls main into the clone,
// runs install.sh, and starts the new cairn in the same pane.

// Updates is what main tells the TUI to check for updates. An empty Rev or
// File turns the check off.
type Updates struct {
	File      string // where the last check is kept
	Rev       string // the commit this build was made from
	SourceDir string // the clone install.sh ran from, to update from
	Compare   update.Compare
}

type updState struct {
	Updates
	st       update.Status
	checked  bool  // a check came back
	checking bool  // a check is under way
	asked    bool  // U started that check: act on what it finds
	err      error // why the last check failed, if it did
	pending  bool  // to offer once nothing else is in the way
	wanted   bool  // U was pressed in the offer: install once the TUI is down
	scroll   int   // first changelog line shown
}

type updateMsg struct {
	st  update.Status
	err error
}

type tickUpdate struct{}

// WithUpdates turns the update check on.
func (m Model) WithUpdates(u Updates) Model {
	m.upd = &updState{Updates: u}
	return m
}

// UpdateRequested tells main to install the update once the program is done.
func (m Model) UpdateRequested() bool { return m.upd != nil && m.upd.wanted }

func (m Model) canCheck() bool {
	return m.upd != nil && m.upd.File != "" && m.upd.Rev != "" && m.upd.Compare != nil
}

// checkUpdate asks, without holding anything up, whether this build is
// behind main, trusting a check younger than maxAge.
func (m Model) checkUpdate(maxAge time.Duration) tea.Cmd {
	if !m.canCheck() || m.upd.checking {
		return nil
	}
	u := m.upd
	u.checking, u.err = true, nil
	file, rev, compare := u.File, u.Rev, u.Compare
	return func() tea.Msg {
		st, err := update.Check(context.Background(), file, rev, time.Now(), maxAge, compare)
		return updateMsg{st, err}
	}
}

// updateChecked takes in a check's result: it offers an update found on its
// own, and answers the U that asked for it.
func (m Model) updateChecked(msg updateMsg) (tea.Model, tea.Cmd) {
	u := m.upd
	u.checking = false
	asked := u.asked
	u.asked = false
	if msg.err != nil {
		u.err = msg.err // offline, or no gh: say nothing unless asked
		if asked {
			m.say("", fmt.Errorf(i18n.T("impossible de vérifier les mises à jour (%s)", "could not check for updates (%s)"), firstLine(msg.err.Error())))
		}
		return m, nil
	}
	u.st, u.checked = msg.st, true
	switch {
	case asked && msg.st.Behind == 0:
		m.say(i18n.T("cairn est à jour.", "cairn is up to date."), nil)
	case asked:
		m.openUpdate()
	case msg.st.Offer(time.Now()):
		u.pending = true
	}
	return m, nil
}

// offerPending opens the offer once you are on the main screen with nothing
// else open: it never covers a question you are answering, say.
func (m *Model) offerPending() {
	if m.upd == nil || !m.upd.pending || m.overlay != "" || (m.view != "" && m.view != "board") ||
		m.answerQ != "" || m.manualAsk || m.filtering || m.busy != "" {
		return
	}
	m.openUpdate()
}

func (m *Model) openUpdate() {
	m.upd.pending, m.upd.scroll = false, 0
	m.overlay = "update"
}

// updateKey is U on the main screen: show what changed when there is an
// update, else ask main afresh.
func (m Model) updateKey() (tea.Model, tea.Cmd) {
	u := m.upd
	switch {
	case !m.canCheck():
		m.say("", fmt.Errorf("%s", i18n.T("cette version ne peut pas se mettre à jour : installe-la avec ./install.sh depuis un clone de main sans changement local",
			"this build cannot update itself: install it with ./install.sh from a clone of main without local edits")))
		return m, nil
	case u.checking:
		m.say(i18n.T("Vérification des mises à jour en cours…", "Checking for updates…"), nil)
		return m, nil
	case u.checked && u.st.Behind > 0:
		m.openUpdate()
		return m, nil
	}
	// What was found earlier may be stale: ask main again.
	m.say(i18n.T("Vérification des mises à jour…", "Checking for updates…"), nil)
	cmd := m.checkUpdate(0)
	u.asked = true
	return m, cmd
}

// installUpdate quits for main to pull, install and start the new cairn.
func (m Model) installUpdate() (tea.Model, tea.Cmd) {
	m.overlay, m.upd.pending = "", false
	if m.upd.SourceDir == "" {
		m.say("", fmt.Errorf("%s", i18n.T("cette version ne connaît pas son clone : lance git pull && ./install.sh dedans",
			"this build does not know its clone: run git pull && ./install.sh there")))
		return m, nil
	}
	m.upd.wanted = true
	m.leave(m.detailOf)
	return m, tea.Quit
}

// remindLater closes the offer until tomorrow; the header keeps a reminder.
func (m Model) remindLater() (tea.Model, tea.Cmd) {
	m.overlay, m.upd.pending = "", false
	if m.upd.File != "" {
		_ = update.Dismiss(m.upd.File, time.Now())
		m.upd.st.Dismissed = time.Now()
	}
	return m, nil
}

func (m Model) updateOverlayKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "U":
		return m.installUpdate()
	case "esc", "q":
		return m.remindLater()
	case "down", "j", " ", "pgdown":
		m.scrollUpdate(1)
	case "up", "k", "pgup":
		m.scrollUpdate(-1)
	}
	return m, nil
}

func (m *Model) scrollUpdate(delta int) {
	lines, rows := m.changelog(m.updateWidth()), m.changelogRows()
	m.upd.scroll = max(0, min(len(lines)-rows, m.upd.scroll+delta*max(1, rows/3)))
}

func (m Model) updateWidth() int { return min(m.width-4, 90) - 4 }

// changelogRows is how many changelog lines fit on screen.
func (m Model) changelogRows() int { return max(4, m.height-16) }

// changelog lists what main has that this build lacks, newest first: each
// commit's subject, its date, and its message rewrapped.
func (m Model) changelog(w int) []string {
	var out []string
	for i, c := range m.upd.st.Changes {
		if i > 0 {
			out = append(out, "")
		}
		date := ""
		if !c.Date.IsZero() {
			date = sDim.Render(c.Date.Local().Format("02/01"))
		}
		out = append(out, hang(sKey.Render("• "), spread(sSoftBold.Render(c.Subject), date, w-2), w)...)
		for _, p := range strings.Split(c.Body, "\n\n") {
			p = strings.Join(strings.Fields(p), " ")
			if p != "" {
				out = append(out, wrap(sMuted.Render(p), w, "  ")...)
			}
		}
	}
	return out
}

// updateBadge reminds, in the header, that an update waits.
func (m Model) updateBadge() string {
	if m.upd == nil || m.upd.st.Behind == 0 || m.overlay == "update" {
		return ""
	}
	return sFresh.Render(i18n.T("↑ mise à jour", "↑ update")) + sDim.Render(" U")
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return l
}
