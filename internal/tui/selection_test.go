package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSelectCopies(t *testing.T) {
	var got string
	copyToClipboard = func(s string) error { got = s; return nil }
	t.Cleanup(func() { copyToClipboard = systemClipboard })

	m := fixture(t, 160, 45)
	lines := screen(m)
	// The first detail row with words on it.
	x0 := m.padX + m.detailX()
	row := -1
	for i, l := range lines {
		if len([]rune(l)) > x0+8 && strings.TrimSpace(string([]rune(l)[x0:])) != "" && i > m.padTop+2 {
			row = i
			break
		}
	}
	if row < 0 {
		t.Fatal("no detail text")
	}
	want := strings.TrimSpace(string([]rune(lines[row])[x0:]))

	mouse := func(m Model, x, y int, a tea.MouseAction) Model {
		next, _ := m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: a})
		return next.(Model)
	}
	// From the list side of the row's start, dragged to the far right: the
	// selection keeps to the detail.
	m = mouse(m, x0, row, tea.MouseActionPress)
	m = mouse(m, 200, row, tea.MouseActionMotion)
	if !m.sel.on || !strings.Contains(m.View(), "\x1b[0;7m") {
		t.Fatal("the drag draws a selection")
	}
	m = mouse(m, 200, row, tea.MouseActionRelease)
	if strings.TrimSpace(got) != want {
		t.Errorf("copied %q, want %q", got, want)
	}
	if !strings.Contains(m.flash, "Copié") {
		t.Errorf("flash %q", m.flash)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.sel.on {
		t.Error("esc drops the selection")
	}

	// A plain click copies nothing.
	got = ""
	m = mouse(m, x0, row, tea.MouseActionPress)
	m = mouse(m, x0, row, tea.MouseActionRelease)
	if got != "" || m.sel.on {
		t.Error("a click is not a selection")
	}
}
