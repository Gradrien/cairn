package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"cairn/internal/chantier"
	"cairn/internal/i18n"
)

// The board (t) lays the chantiers out by phase, to see the flow at a
// glance. The selection is the list's: enter opens its detail.

var boardPhases = []chantier.Phase{chantier.ToPlan, chantier.Plan, chantier.Build, chantier.Review, chantier.Shipped}

const cardH = 6 // a card's rows, border included

// boardCols lists the chantier indexes of each column, in list order.
func (m Model) boardCols() [][]int {
	cols := make([][]int, len(boardPhases))
	for i, c := range m.chantiers {
		for j, p := range boardPhases {
			if c.Phase == p {
				cols[j] = append(cols[j], i)
			}
		}
	}
	return cols
}

// boardPos is the column and row of the selection.
func (m Model) boardPos(cols [][]int) (int, int) {
	for x, col := range cols {
		for y, i := range col {
			if i == m.cursor {
				return x, y
			}
		}
	}
	return -1, -1
}

func (m Model) openBoard() Model {
	m.view = "board"
	if m.selected() == nil && len(m.chantiers) > 0 {
		m.cursor = 0
	}
	return m
}

func (m Model) boardKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	cols := m.boardCols()
	x, y := m.boardPos(cols)
	move := func(dx int) {
		for nx := x + dx; nx >= 0 && nx < len(cols); nx += dx {
			if len(cols[nx]) > 0 {
				m.cursor = cols[nx][min(max(y, 0), len(cols[nx])-1)]
				return
			}
		}
	}
	switch k.String() {
	case "left", "h":
		move(-1)
	case "right", "l":
		move(1)
	case "up", "k":
		if x >= 0 && y > 0 {
			m.cursor = cols[x][y-1]
		}
	case "down", "j":
		if x >= 0 && y < len(cols[x])-1 {
			m.cursor = cols[x][y+1]
		}
	case "enter", "t", "esc":
		m.view = ""
	default:
		return m.mainKey(k)
	}
	m.flash = ""
	return m, nil
}

func (m Model) boardButtons() []button {
	var bs []button
	if c := m.selected(); c != nil {
		if b := m.chantierButtons(c); len(b) > 0 && b[0].key != "enter" {
			bs = append(bs, b[0])
		}
	}
	return append(bs, button{"enter", i18n.T("Ouvrir le détail", "Open the detail"), true})
}

type cardHit struct {
	x0, x1, y0, y1 int
	item           int
}

// boardLayout renders the columns and where each card sits on screen.
func (m Model) boardLayout(h int) (string, []cardHit) {
	all := m.boardCols()
	gap := 2
	// When space is tight, empty columns go.
	shown := make([]int, 0, len(all))
	for x := range all {
		shown = append(shown, x)
	}
	if (m.width-gap*(len(all)-1))/len(all) < 16 {
		shown = shown[:0]
		for x, col := range all {
			if len(col) > 0 {
				shown = append(shown, x)
			}
		}
	}
	if len(shown) == 0 {
		return sMuted.Render(i18n.T("Aucun chantier.", "No ticket.")), nil
	}
	cols := make([][]int, len(shown))
	for i, x := range shown {
		cols[i] = all[x]
	}
	colW := max(12, (m.width-gap*(len(cols)-1))/len(cols))
	fit := max(1, (h-1)/cardH)
	sx, sy := m.boardPos(cols)
	var blocks []string
	var hits []cardHit
	for x, col := range cols {
		p := boardPhases[shown[x]]
		head := phaseStyle(p).Bold(true).Render(p.Label())
		if p == chantier.ToPlan || p == chantier.Plan {
			head = sSoftBold.Render(p.Label())
		}
		head += " " + sMuted.Render(fmt.Sprint(len(col)))
		if fill := colW - lipgloss.Width(head) - 1; fill > 0 {
			head += " " + sLine.Render(strings.Repeat("─", fill))
		}
		lines := []string{head}
		// Keep the selection in view when the column is too tall.
		start := 0
		if x == sx && sy >= fit {
			start = sy - fit + 1
		}
		shown := col[start:]
		more := 0
		if len(shown) > fit {
			more = len(shown) - fit + 1
			shown = shown[:fit-1]
		}
		for _, i := range shown {
			card := m.boardCard(m.chantiers[i], colW, i == m.cursor)
			y0 := 2 + len(lines)
			lines = append(lines, card...)
			hits = append(hits, cardHit{x * (colW + gap), x*(colW+gap) + colW, y0, y0 + len(card), i})
		}
		if more > 0 {
			lines = append(lines, sDim.Render(fmt.Sprintf(i18n.T("… %d de plus", "… %d more"), more)))
		}
		if p == chantier.Shipped && m.shipped > 0 && !m.showShipped {
			lines = append(lines, sMuted.Render(fmt.Sprintf(i18n.T("%d livré(s) rangé(s)", "%d shipped, cleaned up"), m.shipped)))
		}
		blocks = append(blocks, lipgloss.NewStyle().Width(colW).Height(h).MaxHeight(h).Render(strings.Join(lines, "\n")))
		if x < len(cols)-1 {
			blocks = append(blocks, strings.Repeat(" ", gap))
		}
	}
	rows := strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, blocks...), "\n")
	for i, r := range rows {
		rows[i] = ansi.Truncate(r, m.width, "")
	}
	return strings.Join(rows, "\n"), hits
}

func (m Model) boardCard(c *chantier.Chantier, w int, sel bool) []string {
	inner := w - 4
	border := lipgloss.TerminalColor(line)
	mark := ""
	switch {
	case c.Waiting() != "":
		border, mark = warn, sWarn.Render("⚠ ")
	case m.ciFailed(c):
		border, mark = danger, sErr.Render("✗ ")
	case c.Phase == chantier.Shipped:
		mark = sGood.Render("✓ ")
	}
	if sel {
		border = accent
	}
	key := sBold.Render(c.Key)
	if len(m.news(c)) > 0 {
		key += sFresh.Render(" ●")
	}
	title := wrap(c.Title(), inner, "")
	if len(title) > 2 {
		title = title[:2]
		title[1] = ansi.Truncate(title[1], inner-1, "") + "…"
	}
	for len(title) < 2 {
		title = append(title, "")
	}
	content := []string{
		ansi.Truncate(mark+key, inner, "…"),
		sSoft.Render(title[0]), sSoft.Render(title[1]),
		ansi.Truncate(m.listInfo(c), inner, "…"),
	}
	st := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 1).Width(w - 2)
	return strings.Split(st.Render(strings.Join(content, "\n")), "\n")
}

func (m Model) boardView() string {
	bodyH := m.bodyHeight()
	body, _ := m.boardLayout(bodyH)
	foot, _, spill := m.footer()
	return m.header() + "\n" + spill + "\n" + body + "\n\n" + foot
}

func (m Model) boardMouse(ev tea.MouseMsg) (tea.Model, tea.Cmd) {
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
	_, hits := m.boardLayout(m.bodyHeight())
	for _, h := range hits {
		if ev.X >= h.x0 && ev.X < h.x1 && ev.Y >= h.y0 && ev.Y < h.y1 {
			if h.item == m.cursor {
				m.view = "" // second click: open the detail
			}
			m.cursor, m.flash = h.item, ""
		}
	}
	return m, nil
}
