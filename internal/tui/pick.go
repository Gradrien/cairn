package tui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"

	"cairn/internal/config"
	"cairn/internal/i18n"
	"cairn/internal/journal"
	"cairn/internal/sprintosaur"
)

// The "new ticket" view starts any ticket, not only the ones of your cycle:
// type its number, or pick one of the cycle's unassigned tickets.

type pickState struct {
	input   string
	list    []sprintosaur.Issue
	loading bool
	cursor  int // row of list, -1 for the input
	hover   int
}

type pickListMsg struct {
	issues []sprintosaur.Issue
	err    error
}

type pickIssueMsg struct {
	issue sprintosaur.Issue
	err   error
}

// Rows of the body: 0 input, 1 hint, 2 blank, 3 list header, then the list.
const pickListTop = 4

func (m Model) openPick() (tea.Model, tea.Cmd) {
	if m.sp == nil {
		m.say(i18n.T("Sprintosaur est indisponible.", "Sprintosaur is unavailable."), nil)
		return m, nil
	}
	m.view = "pick"
	m.pick = &pickState{loading: true, cursor: -1, hover: -1}
	sp, team := m.sp, m.cfg.Team
	return m, func() tea.Msg {
		issues, err := sp.CurrentIssues(team, "")
		return pickListMsg{issues: issues, err: err}
	}
}

// pickList keeps the cycle's unassigned tickets that are not chantiers yet, most urgent first.
func (m Model) pickList(all []sprintosaur.Issue) []sprintosaur.Issue {
	known := map[string]bool{}
	for _, c := range m.chantiers {
		known[c.Key] = true
	}
	var out []sprintosaur.Issue
	for _, is := range all {
		if is.Assignee == nil && !known[strings.ToUpper(is.ID)] {
			out = append(out, is)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi, pj := out[i].PriorityValue, out[j].PriorityValue
		if pi == 0 {
			pi = 99
		}
		if pj == 0 {
			pj = 99
		}
		return pi < pj
	})
	return out
}

// pickKey returns the full key typed so far: "123" → "YAK-123".
func (m Model) pickKey() string {
	in := strings.ToUpper(strings.TrimSpace(m.pick.input))
	if in == "" {
		return ""
	}
	if strings.IndexFunc(in, unicode.IsLetter) < 0 {
		return strings.ToUpper(m.cfg.Team) + "-" + strings.TrimLeft(in, "-")
	}
	return in
}

func (m Model) pickKeyPress(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.pick
	m.flash = ""
	switch k.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.view = ""
		return m, nil
	case tea.KeyBackspace:
		if r := []rune(p.input); len(r) > 0 {
			p.input = string(r[:len(r)-1])
		}
		p.cursor = -1
		return m, nil
	case tea.KeyUp:
		p.cursor = max(-1, p.cursor-1)
		return m, nil
	case tea.KeyDown:
		p.cursor = min(len(p.list)-1, p.cursor+1)
		return m, nil
	case tea.KeyEnter:
		if p.cursor >= 0 && p.cursor < len(p.list) {
			return m.startTicket(p.list[p.cursor])
		}
		return m.fetchTyped()
	case tea.KeyRunes:
		for _, r := range k.Runes {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
				p.input += string(unicode.ToUpper(r))
			}
		}
		p.cursor = -1
	}
	return m, nil
}

func (m Model) fetchTyped() (tea.Model, tea.Cmd) {
	key := m.pickKey()
	if key == "" {
		m.say(i18n.T("Tape le numéro du ticket, ou choisis-en un dans la liste.", "Type the ticket number, or pick one from the list."), nil)
		return m, nil
	}
	for i, c := range m.chantiers {
		if c.Key == key {
			m.view, m.cursor = "", i
			m.say(key+i18n.T(" est déjà dans la liste.", " is already in the list."), nil)
			return m, nil
		}
	}
	sp := m.sp
	m.busy = i18n.T("Recherche de ", "Looking up ") + key + "…"
	return m, func() tea.Msg {
		is, err := sp.Issue(key)
		if err == nil && is.ID == "" {
			err = fmt.Errorf(i18n.T("%s introuvable dans Sprintosaur", "%s not found in Sprintosaur"), key)
		}
		return pickIssueMsg{issue: is, err: err}
	}
}

// startTicket adds the ticket to the list, takes it if nobody has it, and
// launches its plan, exactly like the Plan button.
func (m Model) startTicket(is sprintosaur.Issue) (tea.Model, tea.Cmd) {
	key := strings.ToUpper(is.ID)
	m.view = ""
	if m.extra == nil {
		m.extra = map[string]sprintosaur.Issue{}
	}
	var assign tea.Cmd
	if is.Assignee == nil && m.cfg.Assignee != "" {
		sp, handle := m.sp, m.cfg.Assignee
		assign = func() tea.Msg {
			if err := sp.Assign(key, handle); err != nil {
				return doneMsg{err: err}
			}
			return nil
		}
	}
	m.extra[key] = is
	_ = journal.SetClosed(config.ChantiersDir(), key, false)
	rebuild := m.rebuild()
	for i, c := range m.chantiers {
		if c.Key == key {
			m.cursor = i
		}
	}
	next, plan := m.plan()
	return next, tea.Batch(rebuild, assign, plan)
}

func (m Model) pickMouse(ev tea.MouseMsg) (tea.Model, tea.Cmd) {
	p := m.pick
	row := ev.Y - 2 - pickListTop
	if ev.Action == tea.MouseActionMotion {
		_, m.hoverBtn = m.hitTest(ev.X, ev.Y)
		p.hover = -1
		if row >= 0 && row < len(p.list) {
			p.hover = row
		}
		return m, nil
	}
	if ev.Button != tea.MouseButtonLeft || ev.Action != tea.MouseActionPress || m.busy != "" {
		return m, nil
	}
	switch {
	case ev.Y == 0 && ev.X < lipgloss.Width(backLabel())+1:
		m.view = ""
	case ev.Y == 2+m.bodyHeight():
		_, ranges := m.footerLayout()
		for i, b := range m.buttons() {
			if b.enabled && ev.X >= ranges[i][0] && ev.X < ranges[i][1] {
				return m.pickKeyPress(keyFor(b.key))
			}
		}
	case row >= 0 && row < len(p.list):
		return m.startTicket(p.list[row])
	}
	return m, nil
}

func (m Model) pickButtons() []button {
	p := m.pick
	return []button{
		{"esc", i18n.T("Retour", "Back"), true},
		{"enter", i18n.T("Lancer le plan", "Start the plan"), p.input != "" || p.cursor >= 0},
	}
}

func (m Model) pickView() string {
	p := m.pick
	head := sKey.Render(backLabel()) + "   " + sTitle.Render(i18n.T("Nouveau ticket", "New ticket"))
	in := p.input
	if in == "" {
		in = sMuted.Render(i18n.T("numéro", "number"))
	}
	prefix := ""
	if strings.IndexFunc(p.input, unicode.IsLetter) < 0 {
		prefix = strings.ToUpper(m.cfg.Team) + "-"
	}
	cursor := "▏"
	if p.cursor >= 0 {
		cursor = ""
	}
	lines := []string{
		sHeader.Render(i18n.T("Clé  ", "Key  ")) + prefix + in + cursor,
		sMuted.Render(i18n.T("  Entrée lance le plan : worktree, espace herdr, agent. Le ticket t'est assigné s'il n'a personne.",
			"  Enter starts the plan: worktree, herdr workspace, agent. The ticket is assigned to you if nobody has it.")),
		"",
		sHeader.Render(i18n.T("Non assignés du cycle", "Unassigned in this cycle")),
	}
	switch {
	case p.loading:
		lines = append(lines, sMuted.Render(i18n.T("  Chargement…", "  Loading…")))
	case len(p.list) == 0:
		lines = append(lines, sMuted.Render(i18n.T("  Aucun ticket libre dans le cycle.", "  No free ticket in this cycle.")))
	}
	for i, is := range p.list {
		meta := is.Status
		if is.Priority != "" {
			meta += " · " + is.Priority
		}
		if is.EstimateLabel != "" {
			meta += " · " + is.EstimateLabel
		}
		key := strings.ToUpper(is.ID)
		room := max(10, m.width-len(key)-lipgloss.Width(meta)-8)
		l := "  " + sBold.Render(key) + "  " + trunc(is.Title, room) + "  " + sMuted.Render(meta)
		switch {
		case i == p.cursor:
			l = sSel.Width(m.width).Render(xansi.Strip(l))
		case i == p.hover:
			l = sHover.Width(m.width).Render(xansi.Strip(l))
		}
		lines = append(lines, l)
	}
	bodyH := m.bodyHeight()
	if len(lines) > bodyH {
		lines = lines[:bodyH]
	}
	for i, l := range lines {
		lines[i] = xansi.Truncate(l, m.width, "…")
	}
	body := lipgloss.NewStyle().Width(m.width).Height(bodyH).MaxHeight(bodyH).Render(strings.Join(lines, "\n"))
	return head + "\n" + m.viewFlash() + "\n" + body + "\n" + m.viewFooter()
}
