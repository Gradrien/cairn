// Package office draws a chantier as a small office in the terminal: the main
// agent at its desk, the subagents it calls at theirs, a whiteboard with the
// plan. Pixels are half blocks (▀): two per cell, the top one in the
// foreground colour, the bottom one in the background colour.
package office

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Color is "#rrggbb", or "" for nothing (the terminal shows through).
type Color string

type cell struct {
	top, bot Color // the two pixels
	r        rune  // text written over the pixels, 0 if none
	fg, bg   Color
	bold     bool
}

// Canvas is cols×rows cells, cols×2·rows pixels.
type Canvas struct {
	Cols, Rows int
	cells      [][]cell
}

func NewCanvas(cols, rows int) *Canvas {
	c := &Canvas{Cols: cols, Rows: rows, cells: make([][]cell, rows)}
	for i := range c.cells {
		c.cells[i] = make([]cell, cols)
	}
	return c
}

// Px sets pixel (x, y); out of bounds is ignored, "" leaves it.
func (c *Canvas) Px(x, y int, col Color) {
	if col == "" || x < 0 || y < 0 || x >= c.Cols || y >= 2*c.Rows {
		return
	}
	cl := &c.cells[y/2][x]
	if y%2 == 0 {
		cl.top = col
	} else {
		cl.bot = col
	}
}

func (c *Canvas) Rect(x, y, w, h int, col Color) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			c.Px(i, j, col)
		}
	}
}

// Sprite is rows of palette keys; '.' and ' ' are transparent.
type Sprite []string

func (c *Canvas) Sprite(x, y int, s Sprite, pal map[byte]Color) {
	for j, row := range s {
		for i := 0; i < len(row); i++ {
			if col, ok := pal[row[i]]; ok {
				c.Px(x+i, y+j, col)
			}
		}
	}
}

// Text writes s over the cells from (col, row). With no bg, each letter
// takes the colour of the pixels under it.
func (c *Canvas) Text(col, row int, s string, fg, bg Color, bold bool) {
	if row < 0 || row >= c.Rows {
		return
	}
	for _, r := range s {
		w := runewidth.RuneWidth(r)
		if w != 1 {
			r, w = '?', 1
		}
		if col >= 0 && col < c.Cols {
			cl := &c.cells[row][col]
			b := bg
			if b == "" {
				b = cl.bot
				if b == "" {
					b = cl.top
				}
			}
			cl.r, cl.fg, cl.bg, cl.bold = r, fg, b, bold
		}
		col += w
	}
}

// Render turns the canvas into lines, one style per run of identical cells.
func (c *Canvas) Render() string {
	lines := make([]string, c.Rows)
	for y, row := range c.cells {
		var b strings.Builder
		for x := 0; x < len(row); {
			k := key(row[x])
			n := 1
			for x+n < len(row) && key(row[x+n]) == k {
				n++
			}
			b.WriteString(paint(row[x], row[x:x+n]))
			x += n
		}
		lines[y] = b.String()
	}
	return strings.Join(lines, "\n")
}

type cellKey struct {
	top, bot, fg, bg Color
	text, bold       bool
}

func key(cl cell) cellKey {
	if cl.r != 0 {
		return cellKey{fg: cl.fg, bg: cl.bg, text: true, bold: cl.bold}
	}
	return cellKey{top: cl.top, bot: cl.bot}
}

func paint(first cell, run []cell) string {
	st := lipgloss.NewStyle()
	var s string
	switch {
	case first.r != 0:
		var b strings.Builder
		for _, cl := range run {
			b.WriteRune(cl.r)
		}
		s = b.String()
		if first.fg != "" {
			st = st.Foreground(lipgloss.Color(first.fg))
		}
		if first.bg != "" {
			st = st.Background(lipgloss.Color(first.bg))
		}
		st = st.Bold(first.bold)
	case first.top == "" && first.bot == "":
		return strings.Repeat(" ", len(run))
	case first.top == first.bot:
		s = strings.Repeat(" ", len(run))
		st = st.Background(lipgloss.Color(first.top))
	case first.top == "":
		s = strings.Repeat("▄", len(run))
		st = st.Foreground(lipgloss.Color(first.bot))
	case first.bot == "":
		s = strings.Repeat("▀", len(run))
		st = st.Foreground(lipgloss.Color(first.top))
	default:
		s = strings.Repeat("▀", len(run))
		st = st.Foreground(lipgloss.Color(first.top)).Background(lipgloss.Color(first.bot))
	}
	return st.Render(s)
}
