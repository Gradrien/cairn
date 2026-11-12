package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"cairn/internal/i18n"
)

// cairn reads the mouse, so the terminal no longer selects text by itself:
// a drag selects here instead, and the release copies it to the clipboard.
type selection struct {
	armed  bool // left button down, a drag would select
	on     bool // dragged: a selection is drawn
	ax, ay int  // where the button went down
	bx, by int  // where the pointer is
	lo, hi int  // columns the selection keeps to: the pane it started in
}

// selectMouse handles what a drag does, before the screens see the event.
// done means the event was a selection's and goes no further.
func (m Model) selectMouse(ev tea.MouseMsg) (next Model, cmd tea.Cmd, done bool) {
	x := min(max(ev.X, 0), max(m.width-1, 0))
	y := min(max(ev.Y, 0), max(m.height-m.padTop-1, 0))
	switch {
	case ev.Button == tea.MouseButtonLeft && ev.Action == tea.MouseActionPress:
		lo, hi := m.selCols(x)
		m.sel = selection{armed: true, ax: x, ay: y, bx: x, by: y, lo: lo, hi: hi}
		return m, nil, false // the click still clicks
	case ev.Action == tea.MouseActionMotion && ev.Button == tea.MouseButtonLeft && m.sel.armed:
		m.sel.bx, m.sel.by = min(max(x, m.sel.lo), m.sel.hi-1), y
		m.sel.on = m.sel.on || m.sel.bx != m.sel.ax || m.sel.by != m.sel.ay
		return m, nil, true
	case ev.Action == tea.MouseActionRelease && m.sel.armed:
		m.sel.armed = false
		if !m.sel.on {
			return m, nil, false
		}
		text := m.selText()
		if text == "" {
			m.sel.on = false
			return m, nil, true
		}
		if err := copyToClipboard(text); err != nil {
			m.say("", err)
		} else {
			m.say(fmt.Sprintf(i18n.T("Copié (%d caractères).", "Copied (%d characters)."), len([]rune(text))), nil)
		}
		return m, nil, true
	}
	return m, nil, false
}

// selCols is the column range a selection started at x keeps to: side by
// side, the list or the detail, not both.
func (m Model) selCols(x int) (lo, hi int) {
	if m.view == "" && m.overlay == "" && !m.single() {
		if x < m.listW() {
			return 0, m.listW()
		}
		if x >= m.detailX() {
			return m.detailX(), m.width
		}
	}
	return 0, m.width
}

// selRows gives, for each selected screen row, the columns selected on it.
func (m Model) selRows(rows int, each func(row, from, to int)) {
	s := m.sel
	ax, ay, bx, by := s.ax, s.ay, s.bx, s.by
	if by < ay || (by == ay && bx < ax) {
		ax, ay, bx, by = bx, by, ax, ay
	}
	for r := ay; r <= by && r < rows; r++ {
		from, to := s.lo, s.hi
		if r == ay {
			from = ax
		}
		if r == by {
			to = bx + 1
		}
		if from < to {
			each(r, from, to)
		}
	}
}

// selText is the selected text, as shown, without the trailing blanks.
func (m Model) selText() string {
	lines := strings.Split(m.screen(), "\n")
	var out []string
	m.selRows(len(lines), func(r, from, to int) {
		out = append(out, strings.TrimRight(ansi.Cut(ansi.Strip(lines[r]), from, to), " "))
	})
	return strings.Trim(strings.Join(out, "\n"), "\n")
}

// highlighted draws the selection, in reverse video, over a screen.
func (m Model) highlighted(screen string) string {
	if !m.sel.on {
		return screen
	}
	lines := strings.Split(screen, "\n")
	m.selRows(len(lines), func(r, from, to int) {
		l := lines[r]
		to = min(to, ansi.StringWidth(l))
		if from >= to {
			return
		}
		lines[r] = ansi.Truncate(l, from, "") + "\x1b[0;7m" + ansi.Strip(ansi.Cut(l, from, to)) + "\x1b[0m" + ansi.TruncateLeft(l, to, "")
	})
	return strings.Join(lines, "\n")
}

// copyToClipboard puts text on the system clipboard (a variable, so tests
// leave the real one alone).
var copyToClipboard = systemClipboard

// systemClipboard copies with pbcopy, wl-copy or xclip when there, else
// with an OSC 52 sequence the terminal may honour.
func systemClipboard(text string) error {
	for _, c := range [][]string{{"pbcopy"}, {"wl-copy"}, {"xclip", "-selection", "clipboard"}} {
		if _, err := exec.LookPath(c[0]); err != nil {
			continue
		}
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	}
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return errors.New(i18n.T("Pas de presse-papier où copier.", "No clipboard to copy to."))
	}
	defer tty.Close()
	_, err = tty.WriteString(ansi.SetSystemClipboard(text))
	return err
}
