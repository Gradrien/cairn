package tui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"

	"cairn/internal/i18n"
	"cairn/internal/tracker"
)

// The "new ticket" view starts any ticket, not only yours. With a tracker:
// type its number, or pick one nobody has. Without one (local): type a title,
// cairn numbers it. tab switches the project when there are several.

type pickState struct {
	project int // index in m.projects
	input   string
	list    []tracker.Issue
	loading bool
	cursor  int // row of list, -1 for the input
	hover   int
}

type pickListMsg struct {
	project int
	issues  []tracker.Issue
	err     error
}

type pickIssueMsg struct {
	project int
	issue   tracker.Issue
	err     error
}

// Rows of the body: 0 input, 1 hint, 2 blank, 3 list header, then the list.
const pickListTop = 4

func (m Model) openPick() (tea.Model, tea.Cmd) {
	if len(m.projects) == 0 {
		m.say(i18n.T("Aucun projet : lance « cairn add » dans un dépôt.", "No project: run \"cairn add\" in a repository."), nil)
		return m, nil
	}
	start := 0
	if p := m.projectOf(m.selected()); p != nil {
		for i, q := range m.projects {
			if q == p {
				start = i
			}
		}
	}
	m.view = "pick"
	m.pick = &pickState{project: start, cursor: -1, hover: -1}
	return m, m.pickLoad()
}

// pickProject is the project new tickets go to.
func (m Model) pickProject() *project {
	if m.pick == nil || m.pick.project >= len(m.projects) {
		return nil
	}
	return m.projects[m.pick.project]
}

// pickCreates reports a project whose tickets are created here from a title.
func (m Model) pickCreates() bool {
	p := m.pickProject()
	if p == nil || p.tr == nil {
		return false
	}
	_, ok := p.tr.(tracker.Creator)
	return ok
}

func (m Model) pickLoad() tea.Cmd {
	p := m.pickProject()
	m.pick.list = nil
	if p == nil || p.tr == nil || m.pickCreates() {
		m.pick.loading = false // the view shows the tracker's error, if any
		return nil
	}
	m.pick.loading = true
	tr, idx := p.tr, m.pick.project
	return func() tea.Msg {
		issues, err := tr.Free()
		return pickListMsg{project: idx, issues: issues, err: err}
	}
}

// pickList keeps the tickets nobody has that are not chantiers yet, most urgent first.
func (m Model) pickList(all []tracker.Issue) []tracker.Issue {
	known := map[string]bool{}
	for _, c := range m.chantiers {
		known[c.Key] = true
	}
	var out []tracker.Issue
	for _, is := range all {
		if !is.Assigned && !known[strings.ToUpper(is.ID)] {
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
		if p := m.pickProject(); p != nil {
			return p.cfg.Key + "-" + strings.TrimLeft(in, "-")
		}
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
	case tea.KeyTab:
		if len(m.projects) > 1 {
			p.project = (p.project + 1) % len(m.projects)
			p.input, p.cursor = "", -1
			return m, m.pickLoad()
		}
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
			return m.startTicket(m.pickProject(), p.list[p.cursor])
		}
		if m.pickCreates() {
			return m.createTyped()
		}
		return m.fetchTyped()
	case tea.KeySpace:
		if m.pickCreates() {
			p.input += " "
		}
	case tea.KeyRunes:
		for _, r := range k.Runes {
			switch {
			case m.pickCreates():
				p.input += string(r)
			case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-':
				p.input += string(unicode.ToUpper(r))
			}
		}
		p.cursor = -1
	}
	return m, nil
}

// createTyped creates a local ticket from the title typed.
func (m Model) createTyped() (tea.Model, tea.Cmd) {
	title := strings.TrimSpace(m.pick.input)
	if title == "" {
		m.say(i18n.T("Tape le titre du ticket.", "Type the ticket's title."), nil)
		return m, nil
	}
	cr := m.pickProject().tr.(tracker.Creator)
	idx := m.pick.project
	m.busy = i18n.T("Création du ticket…", "Creating the ticket…")
	return m, func() tea.Msg {
		is, err := cr.Create(title)
		return pickIssueMsg{project: idx, issue: is, err: err}
	}
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
	// A key of another project goes to that project.
	idx := m.pick.project
	if p := m.projectForKey(key); p != nil {
		for i, q := range m.projects {
			if q == p {
				idx = i
			}
		}
	}
	p := m.projects[idx]
	if p.tr == nil {
		m.say("", p.err)
		return m, nil
	}
	tr := p.tr
	m.busy = i18n.T("Recherche de ", "Looking up ") + key + "…"
	return m, func() tea.Msg {
		is, err := tr.Get(key)
		if err == nil && is.ID == "" {
			err = fmt.Errorf(i18n.T("%s introuvable dans %s", "%s not found in %s"), key, tr.Name())
		}
		return pickIssueMsg{project: idx, issue: is, err: err}
	}
}

// startTicket adds the ticket to the list, takes it if nobody has it, and
// launches its plan, exactly like the Plan button.
func (m Model) startTicket(p *project, is tracker.Issue) (tea.Model, tea.Cmd) {
	key := strings.ToUpper(is.ID)
	m.view = ""
	if p == nil {
		return m, nil
	}
	var assign tea.Cmd
	if !is.Assigned && p.tr != nil {
		tr := p.tr
		assign = func() tea.Msg {
			if err := tr.Take(key); err != nil {
				return doneMsg{err: err}
			}
			return nil
		}
	}
	if p.extra == nil {
		p.extra = map[string]tracker.Issue{}
	}
	p.extra[key] = is
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
	item := -1
	if _, owners := m.pickRows(); !p.loading {
		if row := ev.Y - 2 - pickListTop; row >= 0 && row < len(owners) {
			item = owners[row]
		}
	}
	if ev.Action == tea.MouseActionMotion {
		_, m.hoverBtn = m.hitTest(ev.X, ev.Y)
		p.hover = item
		return m, nil
	}
	if ev.Button != tea.MouseButtonLeft || ev.Action != tea.MouseActionPress || m.busy != "" {
		return m, nil
	}
	switch {
	case ev.Y == 0 && ev.X < lipgloss.Width(backLabel())+1:
		m.view = ""
	case ev.Y == 2+m.bodyHeight():
		return m.footerClick(ev.X)
	case item >= 0:
		return m.startTicket(m.pickProject(), p.list[item])
	}
	return m, nil
}

func (m Model) pickButtons() []button {
	p := m.pick
	out := []button{
		{"esc", i18n.T("Retour", "Back"), true},
		{"enter", i18n.T("Lancer le plan", "Start the plan"), p.input != "" || p.cursor >= 0},
	}
	if len(m.projects) > 1 {
		out = append(out, button{"tab", i18n.T("Projet", "Project"), true})
	}
	return out
}

// pickRows renders the ticket list, long titles wrapped, and the ticket index of each row.
func (m Model) pickRows() ([]string, []int) {
	p := m.pick
	var lines []string
	var owners []int
	for i, is := range p.list {
		meta := is.Status
		if is.Priority != "" {
			meta += " · " + is.Priority
		}
		if is.Estimate != "" {
			meta += " · " + is.Estimate
		}
		key := strings.ToUpper(is.ID)
		rows := hang("  "+sBold.Render(key)+"  ", is.Title+"  "+sMuted.Render(meta), m.width)
		for _, l := range rows {
			switch {
			case i == p.cursor:
				l = sSel.Width(m.width).Render(xansi.Strip(l))
			case i == p.hover:
				l = sHover.Width(m.width).Render(xansi.Strip(l))
			}
			lines = append(lines, l)
			owners = append(owners, i)
		}
	}
	return lines, owners
}

func (m Model) pickView() string {
	p := m.pick
	head := sKey.Render(backLabel()) + "   " + sTitle.Render(i18n.T("Nouveau ticket", "New ticket"))
	proj := m.pickProject()
	if proj != nil && len(m.projects) > 1 {
		head += sMuted.Render(" · ") + sBold.Render(proj.cfg.Name) + sDim.Render(i18n.T("  (tab : autre projet)", "  (tab: other project)"))
	}
	creates := m.pickCreates()
	in := p.input
	if in == "" {
		in = sMuted.Render(i18n.T("numéro", "number"))
		if creates {
			in = sMuted.Render(i18n.T("de quoi s'agit-il ?", "what is it about?"))
		}
	}
	label, prefix := i18n.T("Clé  ", "Key  "), ""
	if creates {
		label = i18n.T("Titre  ", "Title  ")
	} else if strings.IndexFunc(p.input, unicode.IsLetter) < 0 && proj != nil {
		prefix = proj.cfg.Key + "-"
	}
	cursor := "▏"
	if p.cursor >= 0 {
		cursor = ""
	}
	lines := []string{sHeader.Render(label) + prefix + in + cursor}
	switch {
	case creates:
		lines = append(lines,
			sMuted.Render(fmt.Sprintf(i18n.T("  Entrée crée %s-… et lance le plan : worktree, espace herdr, agent. Le titre suffit : l'agent pose ses questions dans le plan.",
				"  Enter creates %s-… and starts the plan: worktree, herdr workspace, agent. The title is enough: the agent asks its questions in the plan."), proj.cfg.Key)))
	default:
		lines = append(lines,
			sMuted.Render(i18n.T("  Entrée lance le plan : worktree, espace herdr, agent. Le ticket t'est assigné s'il n'a personne.",
				"  Enter starts the plan: worktree, herdr workspace, agent. The ticket is assigned to you if nobody has it.")),
			"",
			sHeader.Render(i18n.T("Sans personne", "Unassigned")))
		switch {
		case proj != nil && proj.tr == nil:
			lines = append(lines, sErr.Render("  "+proj.err.Error()))
		case p.loading:
			lines = append(lines, sMuted.Render(i18n.T("  Chargement…", "  Loading…")))
		case len(p.list) == 0:
			lines = append(lines, sMuted.Render(i18n.T("  Aucun ticket libre.", "  No free ticket.")))
		}
		rows, _ := m.pickRows()
		lines = append(lines, rows...)
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
