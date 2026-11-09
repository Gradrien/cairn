package tui

import (
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"cairn/internal/chantier"
	"cairn/internal/i18n"
	"cairn/internal/state"
)

// Overlays float over the main and board screens, which dim behind them:
// the help (?), the command palette (ctrl+k) and the push confirmation.

type pushLogMsg struct {
	key   string
	lines []string
}

// openPush asks for confirmation before allowing a push, with what will leave.
func (m Model) openPush() (tea.Model, tea.Cmd) {
	c := m.selected()
	if c == nil || c.State == nil {
		m.say(i18n.T("Ce chantier n'a pas de plan cairn : ses push ne sont pas bloqués.", "This ticket has no cairn plan: its pushes are not blocked."), nil)
		return m, nil
	}
	if m.pushValid(c) {
		m.say(i18n.T("Ce commit est déjà autorisé : push et PR passent.", "This commit is already allowed: pushes and PR go through."), nil)
		return m, nil
	}
	m.overlay, m.pushKey, m.pushLog = "push", c.Key, nil
	wt, base, key := c.Worktree, m.base(c), c.Key
	if wt == "" {
		return m, nil
	}
	return m, func() tea.Msg {
		out, _ := exec.Command("git", "-C", wt, "log", "--format=%h %s", "-n", "8", base+"..HEAD").Output()
		return pushLogMsg{key: key, lines: strings.Split(strings.TrimSpace(string(out)), "\n")}
	}
}

// ── palette ─────────────────────────────────────────────────────────────────

type palEntry struct {
	icon  string
	label string
	hint  string
	run   func(Model) (tea.Model, tea.Cmd)
}

// paletteEntries are every action of every chantier, every chantier, and
// the global commands.
func (m Model) paletteEntries() []palEntry {
	var out []palEntry
	for i, c := range m.chantiers {
		i := i
		need := m.needsYou(c)
		for j, b := range m.chantierButtons(c) {
			icon := sDim.Render("›")
			if need && j == 0 {
				icon = sWarn.Render("⚠")
			}
			k := b.key
			out = append(out, palEntry{icon, c.Key + " · " + b.label, keyGlyph(k), func(m Model) (tea.Model, tea.Cmd) {
				m.cursor, m.overlay = i, ""
				return m.key(keyFor(k))
			}})
		}
		out = append(out, palEntry{sDim.Render("#"), c.Key + " · " + c.Title(), i18n.T("ouvrir", "open"), func(m Model) (tea.Model, tea.Cmd) {
			m.cursor, m.overlay, m.view = i, "", ""
			return m, nil
		}})
	}
	for _, g := range []button{
		{"n", i18n.T("Nouveau ticket", "New ticket"), len(m.projects) > 0},
		{"t", i18n.T("Vue tableau / liste", "Board / list view"), true},
		{"s", i18n.T("Réglages : projets, dépôts, clés, trackers", "Settings: projects, repositories, keys, trackers"), true},
		{"l", i18n.T("English", "Français"), true},
		{"r", i18n.T("Rafraîchir", "Refresh"), true},
		{"U", i18n.T("Mettre à jour cairn", "Update cairn"), m.canCheck()},
		{"?", i18n.T("Aide : tous les raccourcis", "Help: every key"), true},
		{"q", i18n.T("Retour à herdr / quitter", "Back to herdr / quit"), true},
	} {
		if !g.enabled {
			continue
		}
		k := g.key
		out = append(out, palEntry{sDim.Render("›"), g.label, k, func(m Model) (tea.Model, tea.Cmd) {
			m.overlay = ""
			return m.key(keyFor(k))
		}})
	}
	return out
}

const paletteRows = 10

func (m Model) paletteFound() []palEntry {
	var out []palEntry
	for _, e := range m.paletteEntries() {
		if matchWords(e.label, m.palQuery) {
			out = append(out, e)
		}
	}
	return out
}

// ── keys ────────────────────────────────────────────────────────────────────

func (m Model) overlayKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}
	switch m.overlay {
	case "help":
		m.overlay = ""
	case "push":
		switch k.String() {
		case "enter", "a":
			m.overlay = ""
			if c := m.selected(); c == nil || c.Key != m.pushKey {
				return m, nil
			}
			return m.allowPush()
		case "d":
			m.overlay = ""
			return m, m.diff()
		case "esc", "q":
			m.overlay = ""
		}
	case "close":
		switch k.String() {
		case "enter", "x":
			m.overlay = ""
			return m.closeChantier()
		case "esc", "q":
			m.overlay = ""
		}
	case "update":
		return m.updateOverlayKey(k)
	case "palette":
		found := m.paletteFound()
		switch k.Type {
		case tea.KeyEsc:
			m.overlay = ""
		case tea.KeyEnter:
			if m.palCursor < len(found) {
				return found[m.palCursor].run(m)
			}
		case tea.KeyUp, tea.KeyCtrlP:
			m.palCursor = max(0, m.palCursor-1)
		case tea.KeyDown, tea.KeyCtrlN:
			m.palCursor = min(min(len(found), paletteRows)-1, m.palCursor+1)
		case tea.KeyBackspace:
			if r := []rune(m.palQuery); len(r) > 0 {
				m.palQuery = string(r[:len(r)-1])
			}
			m.palCursor = 0
		case tea.KeySpace:
			m.palQuery += " "
			m.palCursor = 0
		case tea.KeyRunes:
			m.palQuery += string(k.Runes)
			m.palCursor = 0
		}
	}
	return m, nil
}

// ── layout ──────────────────────────────────────────────────────────────────

// overlayLayout renders the open overlay box, where it goes on screen, and
// its clickable spans in screen coordinates. Rows of the palette's results
// are spans keyed "pal:<i>".
func (m Model) overlayLayout() (box []string, x0, y0 int, hits []hit, hitRows []int) {
	var content []string
	color := lipgloss.TerminalColor(accent)
	w := min(m.width-4, 96)
	type rowHit struct {
		row int
		h   hit
	}
	var rh []rowHit
	buttonsRow := func(bs []button, primary bool) {
		var parts []string
		x := 0
		for i, b := range bs {
			r := renderButton(b, false, primary && i == 0)
			rh = append(rh, rowHit{len(content), hit{x, x + lipgloss.Width(r), b.key}})
			parts = append(parts, r)
			x += lipgloss.Width(r) + 1
		}
		content = append(content, strings.Join(parts, " "))
	}
	inner := func() int { return w - 4 }

	switch m.overlay {
	case "help":
		w = min(m.width-4, 112)
		cols := [][]string{
			helpCol(i18n.T("Naviguer", "Move"), [][2]string{
				{"j k ↑↓", i18n.T("ticket précédent, suivant", "previous, next ticket")},
				{"tab", i18n.T("liste ⇄ détail", "list ⇄ detail")},
				{"espace", i18n.T("détail : page suivante", "detail: next page")},
				{"pgup", i18n.T("détail : page précédente", "detail: previous page")},
				{"[ ]", i18n.T("section précédente, suivante", "previous, next section")},
				{"z", i18n.T("tout déplier / replier", "unfold / fold all")},
				{"/", i18n.T("filtrer la liste", "filter the list")},
				{"ctrl+k", i18n.T("chercher ticket ou action", "search ticket or action")},
			}),
			helpCol(i18n.T("Agir", "Act"), [][2]string{
				{"↵", i18n.T("aller à l'agent", "go to the agent")},
				{"v", i18n.T("valider ce qui attend", "approve what waits")},
				{"e", i18n.T("répondre aux questions", "answer the questions")},
				{"a", i18n.T("autoriser le push", "allow the push")},
				{"x", i18n.T("fermer un chantier livré", "close a shipped ticket")},
				{"f", i18n.T("corriger la CI", "fix the CI")},
				{"p", i18n.T("planifier", "plan")},
				{"n", i18n.T("nouveau ticket", "new ticket")},
			}),
			helpCol(i18n.T("Voir", "See"), [][2]string{
				{"d", i18n.T("diff de la branche", "branch diff")},
				{"c", i18n.T("commentaires de la PR", "PR comments")},
				{"o", i18n.T("ticket dans son tracker", "ticket in its tracker")},
				{"t", i18n.T("vue tableau", "board view")},
				{"s", i18n.T("réglages et projets", "settings and projects")},
				{"l", i18n.T("English / Français", "Français / English")},
				{"r", i18n.T("rafraîchir", "refresh")},
				{"U", i18n.T("mettre à jour cairn", "update cairn")},
				{"q", i18n.T("retour à herdr", "back to herdr")},
			}),
		}
		content = append(content, spread(sTitle.Render(i18n.T("Raccourcis", "Keys")), sKey.Render("esc")+sMuted.Render(i18n.T(" fermer", " close")), inner()), "")
		var blocks []string
		total := 0
		for i, c := range cols {
			b := strings.Join(c, "\n")
			if i > 0 {
				b = lipgloss.NewStyle().PaddingLeft(3).Render(b)
			}
			blocks = append(blocks, b)
			total += lipgloss.Width(b)
		}
		if total <= inner() {
			content = append(content, strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, blocks...), "\n")...)
		} else {
			for _, c := range cols {
				content = append(content, c...)
				content = append(content, "")
			}
		}
		content = append(content, "")
		content = append(content, wrap(sMuted.Render(i18n.T("La barre du bas ne montre que ce qui est possible pour le ticket choisi, l'action attendue en premier. Le reste est ici, ou dans ctrl+k.",
			"The bottom bar only shows what can be done for the selected ticket, the expected action first. The rest is here, or in ctrl+k.")), inner(), "")...)

	case "palette":
		w = min(m.width-4, 84)
		q := m.palQuery + sKey.Render("▏")
		if m.palQuery == "" {
			q = sKey.Render("▏") + sDim.Render(i18n.T("un ticket, une action… (ex. « val 861 »)", "a ticket, an action… (e.g. \"appr 861\")"))
		}
		content = append(content, spread(sKey.Render("› ")+q, sDim.Render("esc"), inner()), sLine.Render(strings.Repeat("─", inner())))
		found := m.paletteFound()
		if len(found) == 0 {
			content = append(content, sMuted.Render(i18n.T("Rien ne correspond.", "Nothing matches.")))
		}
		for i, e := range found {
			if i == paletteRows {
				content = append(content, sDim.Render(fmt.Sprintf(i18n.T("… %d de plus : précise ta recherche", "… %d more: narrow your search"), len(found)-paletteRows)))
				break
			}
			row := spread(e.icon+" "+e.label, sDim.Render(e.hint), inner())
			if i == m.palCursor {
				row = sSel.Width(inner()).Render(ansi.Strip(row))
			}
			rh = append(rh, rowHit{len(content), hit{0, inner(), fmt.Sprintf("pal:%d", i)}})
			content = append(content, row)
		}
		content = append(content, sLine.Render(strings.Repeat("─", inner())), sDim.Render(i18n.T("↑↓ choisir · ↵ lancer · esc fermer", "↑↓ pick · ↵ run · esc close")))

	case "push":
		color = warn
		w = min(m.width-4, 90)
		c := m.selected()
		if c == nil || c.State == nil {
			return nil, 0, 0, nil, nil
		}
		s := c.State
		row := func(k, v string) { content = append(content, hang(lbl(k), v, inner())...) }
		content = append(content, sWarn.Render(fmt.Sprintf(i18n.T("Autoriser le push de %s ?", "Allow pushing %s?"), c.Key)))
		content = append(content, wrap(sMuted.Render(i18n.T("Tu autorises ce commit à partir : push, PR et nouvelles tentatives passent. Une fois la PR ouverte, les correctifs suivants poussent librement : la relecture se fait sur la PR.",
			"You let this commit leave: pushes, the PR and retries go through. Once the PR is open, later fixes push freely: review happens on the PR.")), inner(), "")...)
		content = append(content, "")
		if c.Branch != "" {
			row(i18n.T("Branche", "Branch"), c.Branch)
		}
		if g, ok := m.gits[c.Key]; ok && g.Err == "" {
			row(i18n.T("Part", "Leaves"), plural(g.Ahead, "commit", "commits")+" · "+plural(g.Files, i18n.T("fichier", "file"), i18n.T("fichiers", "files"))+" "+
				sGood.Render(fmt.Sprintf("+%d", g.Added))+" "+sErr.Render(fmt.Sprintf("−%d", g.Removed)))
			if g.Uncommitted > 0 && !c.Main {
				row("", sWarn.Render("⚠ "+plural(g.Uncommitted, i18n.T("changement non commité ne partira pas", "uncommitted change will not leave"), i18n.T("changements non commités ne partiront pas", "uncommitted changes will not leave"))))
			}
		}
		for _, l := range m.pushLog {
			if l == "" {
				continue
			}
			h, subj, _ := strings.Cut(l, " ")
			content = append(content, ansi.Truncate(strings.Repeat(" ", label)+sDim.Render(h)+" "+sSoft.Render(subj), inner(), "…"))
		}
		if n := len(s.Tasks); n > 0 {
			d := s.DoneCount()
			v := sGood.Render(fmt.Sprintf("✓ %d/%d", d, n))
			if d < n {
				v = sWarn.Render(fmt.Sprintf("⚠ %d/%d", d, n))
			}
			row(i18n.T("Tâches", "Tasks"), v)
		}
		if n := len(s.SideEffects); n > 0 {
			d := state.Count(s.SideEffects, true)
			if d == n {
				row(i18n.T("Effets", "Effects"), sGood.Render(fmt.Sprintf(i18n.T("✓ %d/%d vérifiés", "✓ %d/%d checked"), d, n)))
			} else {
				row(i18n.T("Effets", "Effects"), sWarn.Render(fmt.Sprintf(i18n.T("⚠ %d/%d vérifiés : ", "⚠ %d/%d checked: "), d, n)+
					plural(n-d, i18n.T("reste ouvert", "still open"), i18n.T("restent ouverts", "still open"))))
				shown := 0
				for _, t := range s.SideEffects {
					if !t.Done && shown < 5 {
						content = append(content, hang(strings.Repeat(" ", label)+sWarn.Render("○ "), plainMD(t.Text), inner())...)
						shown++
					}
				}
			}
		} else {
			row(i18n.T("Effets", "Effects"), sWarn.Render(i18n.T("⚠ aucun recensé dans le plan", "⚠ none listed in the plan")))
		}
		if q := len(s.OpenQuestions()); q > 0 {
			row("Questions", sWarn.Render(fmt.Sprintf(i18n.T("⚠ %d encore ouverte(s)", "⚠ %d still open"), q)))
		}
		content = append(content, "")
		buttonsRow([]button{
			{"enter", i18n.T("Autoriser ce commit", "Allow this commit"), true},
			{"d", i18n.T("Relire le diff", "Review the diff"), c.Worktree != ""},
			{"esc", i18n.T("Annuler", "Cancel"), true},
		}, true)
	case "close":
		color = danger
		w = min(m.width-4, 80)
		var c *chantier.Chantier
		for _, x := range m.all {
			if x.Key == m.closeKey {
				c = x
			}
		}
		if c == nil {
			return nil, 0, 0, nil, nil
		}
		row := func(k, v string) { content = append(content, hang(lbl(k), v, inner())...) }
		content = append(content, sErr.Render(fmt.Sprintf(i18n.T("Fermer %s ?", "Close %s?"), c.Key)), sMuted.Render(trunc(c.Title(), inner())), "")
		if c.Worktree != "" && !c.Main {
			row("Worktree", tilde(c.Worktree)+sMuted.Render(i18n.T(" : supprimé", ": removed")))
		}
		if n := len(c.Agents); n > 0 {
			row(i18n.T("Agents", "Agents"), plural(n, i18n.T("agent fermé", "agent closed"), i18n.T("agents fermés", "agents closed"))+sMuted.Render(i18n.T(", avec leur espace herdr", ", with their herdr workspace")))
		}
		row("Plan", i18n.T("archivé dans ~/.cairn/archive", "archived in ~/.cairn/archive"))
		if c.Branch != "" {
			row(i18n.T("Branche", "Branch"), c.Branch+sMuted.Render(i18n.T(" : gardée (mergée sur GitHub)", ": kept (merged on GitHub)")))
		}
		content = append(content, "")
		r := m.closeRisk
		switch {
		case r == nil:
			content = append(content, sMuted.Render(i18n.T("Vérification de ce qui serait perdu…", "Checking what would be lost…")))
		case r.dirty == 0 && r.unpushed == 0:
			content = append(content, sGood.Render(i18n.T("✓ Rien ne sera perdu : tout est sur GitHub.", "✓ Nothing will be lost: everything is on GitHub.")))
		default:
			if r.dirty > 0 {
				content = append(content, sErr.Render("⚠ "+plural(r.dirty, i18n.T("fichier non commité sera perdu", "uncommitted file will be lost"), i18n.T("fichiers non commités seront perdus", "uncommitted files will be lost"))))
			}
			if r.unpushed > 0 {
				content = append(content, sErr.Render("⚠ "+plural(r.unpushed, i18n.T("commit absent de GitHub sera perdu", "commit not on GitHub will be lost"), i18n.T("commits absents de GitHub seront perdus", "commits not on GitHub will be lost"))))
			}
		}
		for _, a := range c.Agents {
			if a.Status == "working" {
				content = append(content, sWarn.Render(fmt.Sprintf(i18n.T("⚠ %s travaille encore", "⚠ %s is still working"), a.Label())))
			}
		}
		content = append(content, "")
		buttonsRow([]button{
			{"enter", i18n.T("Fermer et supprimer le worktree", "Close and remove the worktree"), true},
			{"esc", i18n.T("Annuler", "Cancel"), true},
		}, true)
	case "update":
		color = fresh
		w = min(m.width-4, 90)
		u := m.upd
		content = append(content, spread(sFresh.Bold(true).Render(i18n.T("Nouvelle version de cairn", "A new cairn is out")),
			sMuted.Render(plural(u.st.Behind, i18n.T("commit sur main", "commit on main"), i18n.T("commits sur main", "commits on main"))), inner()), "")
		lines, rows := m.changelog(inner()), m.changelogRows()
		from := min(u.scroll, max(0, len(lines)-rows))
		end := min(len(lines), from+rows)
		if from > 0 {
			content = append(content, sDim.Render(i18n.T("↑ plus récent", "↑ newer")))
		}
		content = append(content, lines[from:end]...)
		if end < len(lines) {
			content = append(content, sDim.Render(i18n.T("↓ j/k ou la molette pour la suite", "↓ j/k or the wheel for more")))
		}
		content = append(content, "", sLine.Render(strings.Repeat("─", inner())))
		if u.SourceDir == "" {
			content = append(content, wrap(sMuted.Render(i18n.T("Cette version ne connaît pas son clone : lance git pull && ./install.sh dedans.",
				"This build does not know its clone: run git pull && ./install.sh there.")), inner(), "")...)
		} else {
			content = append(content, wrap(sMuted.Render(fmt.Sprintf(i18n.T("Mettre à jour tire main dans %s, lance install.sh (tests compris) puis relance cairn ici. Plus tard : redemandé demain.",
				"Updating pulls main into %s, runs install.sh (tests included), then starts cairn again here. Later: asked again tomorrow."), tilde(u.SourceDir))), inner(), "")...)
		}
		content = append(content, "")
		buttonsRow([]button{
			{"U", i18n.T("Mettre à jour", "Update"), u.SourceDir != ""},
			{"esc", i18n.T("Plus tard", "Later"), true},
		}, true)
	default:
		return nil, 0, 0, nil, nil
	}

	box = strings.Split(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(color).Padding(0, 1).Width(w-2).
		Render(strings.Join(content, "\n")), "\n")
	x0 = max(0, (m.width-w)/2)
	y0 = max(1, min(4, m.height-len(box)-2))
	for _, r := range rh {
		hits = append(hits, hit{x0 + 2 + r.h.x0, x0 + 2 + r.h.x1, r.h.key})
		hitRows = append(hitRows, y0+1+r.row)
	}
	return box, x0, y0, hits, hitRows
}

func helpCol(title string, keys [][2]string) []string {
	out := []string{sSection.Render(title)}
	for _, k := range keys {
		out = append(out, sKey.Render(k[0]+strings.Repeat(" ", max(1, 8-lipgloss.Width(k[0]))))+sSoft.Render(k[1]))
	}
	return out
}

// overlaid draws the open overlay over the screen, which dims behind it.
func (m Model) overlaid(screen string) string {
	if m.overlay == "" {
		return screen
	}
	box, x0, y0, _, _ := m.overlayLayout()
	if box == nil {
		return screen
	}
	base := strings.Split(screen, "\n")
	for i, l := range base {
		base[i] = sLine.Render(ansi.Strip(l))
	}
	return strings.Join(place(base, box, x0, y0, m.width), "\n")
}

// place writes box over base from column x0, row y0.
func place(base, box []string, x0, y0, width int) []string {
	out := append([]string(nil), base...)
	for i, bl := range box {
		y := y0 + i
		if y >= len(out) {
			break
		}
		row := out[y]
		left := ansi.Truncate(row, x0, "")
		if pad := x0 - ansi.StringWidth(left); pad > 0 {
			left += strings.Repeat(" ", pad)
		}
		right := ansi.Cut(row, x0+ansi.StringWidth(bl), width)
		out[y] = left + "\x1b[0m" + bl + "\x1b[0m" + right
	}
	return out
}

func (m Model) overlayMouse(ev tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.overlay == "update" && ev.Action == tea.MouseActionPress {
		switch ev.Button {
		case tea.MouseButtonWheelUp:
			m.scrollUpdate(-1)
			return m, nil
		case tea.MouseButtonWheelDown:
			m.scrollUpdate(1)
			return m, nil
		}
	}
	if ev.Button != tea.MouseButtonLeft || ev.Action != tea.MouseActionPress {
		return m, nil
	}
	box, x0, y0, hits, rows := m.overlayLayout()
	for i, h := range hits {
		if ev.Y == rows[i] && ev.X >= h.x0 && ev.X < h.x1 {
			if n, ok := strings.CutPrefix(h.key, "pal:"); ok {
				var j int
				fmt.Sscan(n, &j)
				if found := m.paletteFound(); j < len(found) {
					return found[j].run(m)
				}
				return m, nil
			}
			return m.overlayKey(keyFor(h.key))
		}
	}
	// A click outside the box closes it.
	if box != nil && (ev.Y < y0 || ev.Y >= y0+len(box) || ev.X < x0 || ev.X >= x0+lipgloss.Width(box[0])) {
		if m.overlay == "update" {
			return m.remindLater()
		}
		m.overlay = ""
	}
	return m, nil
}
