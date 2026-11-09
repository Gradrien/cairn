package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"cairn/internal/chantier"
	"cairn/internal/i18n"
	"cairn/internal/progress"
	"cairn/internal/state"
)

// ── layout ──────────────────────────────────────────────────────────────────

// Below narrowWidth columns only one pane shows: the list, or the detail.
const narrowWidth = 90

func (m Model) narrow() bool { return m.width < narrowWidth }

// listW is the list's width: about a third of the screen, all of it when narrow.
func (m Model) listW() int {
	if m.narrow() {
		return m.width
	}
	return min(60, max(32, m.width*3/10))
}

// detailX is the screen column where the detail pane starts.
func (m Model) detailX() int {
	if m.narrow() {
		return 0
	}
	return m.listW() + 3
}

func (m Model) detailWidth() int {
	if m.narrow() {
		return max(20, m.width-2)
	}
	return max(20, m.width-m.listW()-3)
}

func (m Model) showList() bool   { return !m.narrow() || !m.detailFocus }
func (m Model) showDetail() bool { return !m.narrow() || m.detailFocus }

// Screen rows: 0 header, 1 spacer (or a message that found no room below),
// 2…2+bodyH-1 body, then the button row.
func (m Model) bodyHeight() int { return max(3, m.height-4) }

// ── main view ───────────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	switch m.view {
	case "comments":
		return m.commentsView()
	case "how":
		return m.howView()
	case "pick":
		return m.pickView()
	case "settings":
		return m.settingsView()
	case "board":
		return m.overlaid(m.boardView())
	}
	return m.overlaid(m.mainView())
}

func (m Model) mainView() string {
	bodyH := m.bodyHeight()
	var body string
	switch {
	case !m.narrow():
		left := m.viewList(m.listW(), bodyH)
		right := m.viewDetail(m.detailWidth(), bodyH)
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, m.separator(bodyH, " ", " "), right)
	case m.detailFocus:
		body = lipgloss.JoinHorizontal(lipgloss.Top, m.viewDetail(m.detailWidth(), bodyH), m.separator(bodyH, " ", ""))
	default:
		body = m.viewList(m.width, bodyH)
	}
	foot, _, spill := m.footer()
	return m.header() + "\n" + spill + "\n" + body + "\n" + foot
}

func (m Model) header() string {
	left := sTitle.Render("cairn")
	var names []string
	for _, p := range m.projects {
		names = append(names, p.cfg.Name)
	}
	if len(names) > 0 {
		left += sMuted.Render("  " + strings.Join(names, " · "))
	}
	if len(m.projects) == 1 && m.projects[0].cfg.Assignee != "" {
		left += sMuted.Render(" · " + m.projects[0].cfg.Assignee)
	}
	if is := m.cycle(); is != "" {
		left += sMuted.Render(" · cycle " + is)
	}
	switch {
	case m.view == "board":
		left += sMuted.Render(i18n.T(" · tableau", " · board"))
	case m.narrow() && m.detailFocus:
		left = sTitle.Render("cairn") + "  " + sKey.Render("‹ esc") + sMuted.Render(i18n.T(" liste", " list"))
	}
	right := m.health()
	if n := m.newsCount(); n > 0 {
		right = sFresh.Render("● "+plural(n, i18n.T("chantier a bougé", "ticket moved"), i18n.T("chantiers ont bougé", "tickets moved"))) + "   " + right
	}
	if b := m.testBadges(); b != "" {
		right = b + "   " + right
	}
	if b := m.updateBadge(); b != "" {
		right = b + "   " + right
	}
	if n := m.waitingCount(); n > 0 && (m.view == "board" || (m.narrow() && m.detailFocus)) {
		right = sWarn.Render("⚠ "+plural(n, i18n.T("attend ta décision", "waiting for you"), i18n.T("attendent ta décision", "waiting for you"))) + "   " + right
	}
	return spread(left, right, m.width)
}

// health says whether herdr and each tracker answer, with the error if not.
// Local projects have nothing to answer.
func (m Model) health() string {
	dot := func(name string, err error) string {
		if err != nil {
			return sErr.Render("● " + name + " : " + err.Error())
		}
		return sGood.Render("●") + sMuted.Render(" "+name)
	}
	out := dot("herdr", m.localErr)
	shown := map[string]bool{}
	for _, p := range m.projects {
		if p.cfg.Tracker == "local" || p.cfg.Tracker == "" {
			continue
		}
		name := p.cfg.Name
		if p.tr != nil {
			name = p.tr.Name()
			if shown[name] && p.err == nil {
				continue
			}
		}
		shown[name] = true
		out += "  " + dot(name, p.err)
	}
	return out
}

func (m Model) waitingCount() int {
	n := 0
	for _, c := range m.chantiers {
		if m.needsYou(c) {
			n++
		}
	}
	return n
}

// spread puts left and right on one line of width w, right flush; right is
// cut first when both do not fit.
func spread(left, right string, w int) string {
	lw := lipgloss.Width(left)
	room := w - lw - 2
	if room < 8 || right == "" {
		return ansi.Truncate(left, w, "…")
	}
	right = ansi.Truncate(right, room, "…")
	return left + strings.Repeat(" ", w-lw-lipgloss.Width(right)) + right
}

func (m Model) cycle() string {
	if len(m.projects) != 1 {
		return ""
	}
	for _, is := range m.projects[0].issues {
		if is.Cycle != 0 {
			return fmt.Sprint(is.Cycle)
		}
	}
	return ""
}

// ── separator and scrollbar ─────────────────────────────────────────────────

// thumb is the scrollbar's thumb (first row, length) for total lines shown h
// at a time from top. Nothing to show when everything fits.
func thumb(total, h, top int) (int, int) {
	if total <= h || h <= 0 {
		return 0, 0
	}
	n := max(1, h*h/total)
	maxTop := total - h
	top = min(max(0, top), maxTop)
	return top * (h - n) / maxTop, n
}

// separator is the column between the panes; it doubles as the detail's
// scrollbar, brighter when the detail has the focus.
func (m Model) separator(h int, before, after string) string {
	var total, top int
	if m.showDetail() {
		d := m.detailLayout(m.detailWidth())
		total, top = len(d.lines), m.detailTop(len(d.lines), h)
	}
	start, n := thumb(total, h, top)
	track := sLine
	if m.detailFocus {
		track = sDim
	}
	rows := make([]string, h)
	for i := range rows {
		if n > 0 && i >= start && i < start+n {
			rows[i] = before + sKey.Render("┃") + after
		} else {
			rows[i] = before + track.Render("│") + after
		}
	}
	return strings.Join(rows, "\n")
}

// ── list ────────────────────────────────────────────────────────────────────

// needsYou: the chantier waits for a decision, or its CI broke.
func (m Model) needsYou(c *chantier.Chantier) bool { return c.Waiting() != "" || m.ciFailed(c) }

func (m Model) ciFailed(c *chantier.Chantier) bool {
	pr, ok := m.prs[c.Key]
	return ok && pr.Err == "" && pr.Checks.Failed > 0 && c.OpenPR() != nil
}

// groupOf is the list section a chantier sits in.
func (m Model) groupOf(c *chantier.Chantier) string {
	switch {
	case m.needsYou(c):
		return "wait"
	case c.Phase == chantier.ToPlan:
		return "toplan"
	case c.Phase == chantier.Shipped:
		return "kept"
	}
	return "work"
}

func groupHeader(g string) string {
	switch g {
	case "wait":
		return sWarn.Render(i18n.T("Attend ta décision", "Waiting for you"))
	case "toplan":
		return sHeader.Render(i18n.T("À planifier", "To plan"))
	case "kept":
		return sHeader.Render(i18n.T("Livrés, pas encore rangés", "Shipped, not cleaned up"))
	}
	return sHeader.Render(i18n.T("En cours", "In progress"))
}

// matches reports whether list item i passes the "/" filter.
func (m Model) matches(i int) bool {
	if m.filter == "" {
		return true
	}
	var text string
	if i < len(m.chantiers) {
		c := m.chantiers[i]
		text = c.Key + " " + c.Title()
	} else if j := i - len(m.chantiers); j < len(m.others) {
		text = m.others[j].Label() + " " + m.others[j].Title
	}
	return matchWords(text, m.filter)
}

// matchWords: every word of query appears in text, case ignored.
func matchWords(text, query string) bool {
	text = strings.ToLower(text)
	for _, w := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(text, w) {
			return false
		}
	}
	return true
}

// listInfo is a chantier's second line: what to do when it waits for you,
// where it stands otherwise.
func (m Model) listInfo(c *chantier.Chantier) string {
	icons := ""
	for _, a := range c.Agents {
		icons += agentIcon(a.Status)
	}
	if icons != "" {
		icons = sMuted.Render(" · ") + icons
	}
	if w := c.Waiting(); w != "" {
		info := sWarn.Render("⚠ " + w)
		if s := c.State; s != nil {
			if q := len(s.OpenQuestions()); q > 0 {
				info += sMuted.Render(" · " + plural(q, "question", "questions"))
			}
			if e := progress.Elapsed(s.UpdatedAt); e != "" && !s.UpdatedAt.IsZero() {
				info += sMuted.Render(" · " + e)
			}
		}
		return info + icons
	}
	if m.ciFailed(c) {
		info := sErr.Render(i18n.T("✗ CI en échec", "✗ CI failing"))
		if pr := c.OpenPR(); pr != nil {
			info += sMuted.Render(fmt.Sprintf(" · PR #%d", pr.Number))
		}
		return info + icons
	}
	info := phaseStyle(c.Phase).Render(c.Phase.Label())
	if why := m.watch.keep[c.Key]; why != "" && c.Phase == chantier.Shipped {
		info += sWarn.Render(" · " + why)
	}
	if c.State != nil && len(c.State.Tasks) > 0 {
		info += sMuted.Render(fmt.Sprintf(" %d/%d", c.State.DoneCount(), len(c.State.Tasks)))
	}
	if c.Phase == chantier.ToPlan && c.Issue != nil {
		if c.Issue.Priority != "" {
			info += sMuted.Render(" · " + c.Issue.Priority)
		}
		if c.Issue.Estimate != "" {
			info += sMuted.Render(" · " + c.Issue.Estimate)
		}
	}
	info += icons
	if pr := c.OpenPR(); pr != nil {
		info += sMuted.Render(fmt.Sprintf(" · PR #%d", pr.Number))
		if _, ready := m.readiness(c); ready {
			info += sGood.Render(i18n.T(" ✓ prêt", " ✓ ready"))
		}
	}
	return info
}

// agentsSummary is what the status line says when the mouse is over a chantier.
func agentsSummary(c *chantier.Chantier) string {
	if len(c.Agents) == 0 {
		return ""
	}
	var parts []string
	for _, a := range c.Agents {
		parts = append(parts, a.Label()+" "+a.Status)
	}
	return c.Key + " · " + plural(len(c.Agents), "agent", "agents") + " : " + strings.Join(parts, ", ")
}

// listLayout renders the list and says which item each visible line belongs
// to (-1 for headers), so clicks map back to items.
func (m Model) listLayout(w, h int) ([]string, []int) {
	var lines []string
	var owners []int
	curLine := 0
	sel := sSel
	if m.detailFocus && !m.narrow() {
		sel = sSelDim
	}
	add := func(item int, s ...string) {
		for _, l := range s {
			switch {
			case item >= 0 && item == m.cursor:
				l = sel.Width(w).Render(ansi.Truncate(ansi.Strip(l), w, "…"))
			case item >= 0 && item == m.hoverItem:
				l = sHover.Width(w).Render(ansi.Truncate(ansi.Strip(l), w, "…"))
			default:
				l = ansi.Truncate(l, w, "…")
			}
			lines = append(lines, l)
			owners = append(owners, item)
		}
	}

	switch {
	case m.filtering:
		add(-1, sKey.Render("/ ")+m.filter+sKey.Render("▏")+sMuted.Render(i18n.T("  ↵ garder · esc effacer", "  ↵ keep · esc clear")))
	case m.filter != "":
		add(-1, sKey.Render("/ ")+m.filter+sMuted.Render(i18n.T("  (esc efface)", "  (esc clears)")))
	default:
		add(-1, sDim.Render(i18n.T("/ filtrer · ctrl+k tout chercher", "/ filter · ctrl+k search everything")))
	}
	switch {
	case len(m.chantiers) > 0:
	case m.loaded:
		add(-1, sMuted.Render(i18n.T("Aucun chantier : n pour en lancer un.", "No ticket yet: n to start one.")))
	default:
		add(-1, sMuted.Render(i18n.T("Chargement…", "Loading…")))
	}
	// The shipped chantiers sit at the end of m.chantiers, under their toggle.
	toggled := false
	toggle := func() {
		toggled = true
		if m.shipped == 0 {
			return
		}
		label := fmt.Sprintf(i18n.T("› %d livré(s) : afficher", "› %d shipped: show"), m.shipped)
		if m.showShipped {
			label = i18n.T("‹ masquer les livrés", "‹ hide shipped")
		}
		l := sMuted.Render(label)
		if m.hoverItem == shippedItem {
			l = sHover.Width(w).Render(label)
		}
		lines = append(lines, "", l)
		owners = append(owners, -1, shippedItem)
	}
	group := ""
	for i, c := range m.chantiers {
		if m.isShipped(c) && !toggled {
			toggle()
		}
		if !m.matches(i) {
			continue
		}
		if g := m.groupOf(c); !toggled && g != group {
			if group != "" {
				add(-1, "")
			}
			add(-1, groupHeader(g))
			group = g
		}
		mark := " "
		switch {
		case c.Waiting() != "":
			mark = sWarn.Render("⚠")
		case m.ciFailed(c):
			mark = sErr.Render("✗")
		case m.isShipped(c):
			mark = sGood.Render("✓")
		}
		info := m.listInfo(c)
		key := sBold.Render(c.Key)
		if n := m.newsShort(c); n != "" {
			if mark == " " {
				mark = sFresh.Render("●")
			} else {
				key += sFresh.Render(" ●")
			}
			info = sFresh.Render(n) + sMuted.Render(" · ") + info
		}
		if i == m.cursor {
			curLine = len(lines)
		}
		add(i, mark+" "+key+" "+c.Title(), "    "+info)
	}
	if !toggled {
		toggle()
	}
	hdr := false
	for j, a := range m.others {
		item := len(m.chantiers) + j
		if !m.matches(item) {
			continue
		}
		if !hdr {
			add(-1, "", sHeader.Render(i18n.T("Hors chantier", "Not on a ticket")))
			hdr = true
		}
		l := agentIcon(a.Status) + " " + a.Title
		if a.Title == "" {
			l = agentIcon(a.Status) + " " + a.Label()
		}
		if item == m.cursor {
			curLine = len(lines)
		}
		add(item, l)
	}
	// Keep the cursor in view.
	start := 0
	if curLine+2 > h {
		start = curLine + 2 - h
	}
	lines, owners = lines[start:], owners[start:]
	if len(lines) > h {
		lines, owners = lines[:h], owners[:h]
	}
	return lines, owners
}

// githubPRs is what GitHub says of every PR read so far, by URL.
func (m Model) githubPRs() map[string]string {
	out := map[string]string{}
	for _, pr := range m.prs {
		if pr.URL != "" && pr.Err == "" {
			out[pr.URL] = pr.State
		}
	}
	return out
}

// isShipped reports a shipped chantier with nothing left to look at: it
// leaves the list. One cairn could not clean up stays, with the reason.
func (m Model) isShipped(c *chantier.Chantier) bool {
	return c.Phase == chantier.Shipped && m.watch.keep[c.Key] == ""
}

// shippedItem is the list row that shows or hides the shipped chantiers.
const shippedItem = -2

func (m Model) viewList(w, h int) string {
	lines, _ := m.listLayout(w, h)
	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(strings.Join(lines, "\n"))
}

// ── footer ──────────────────────────────────────────────────────────────────

type button struct {
	key, label string
	enabled    bool
}

// hit is a clickable span of a row: [x0, x1) runs key.
type hit struct {
	x0, x1 int
	key    string
}

// buttons are the actions for the current screen; each is also a key.
func (m Model) buttons() []button {
	switch m.view {
	case "comments":
		return m.commentsButtons()
	case "how":
		return m.howButtons()
	case "pick":
		return m.pickButtons()
	case "settings":
		return m.settingsButtons()
	case "board":
		return m.boardButtons()
	}
	return m.mainButtons()
}

// primaryFirst: the first button is what the selection is waiting for.
func (m Model) primaryFirst() bool {
	if m.view != "" && m.view != "board" {
		return false
	}
	c := m.selected()
	if c == nil {
		return m.selectedOther() != nil
	}
	cd := m.cardFor(c)
	return cd != nil && cd.primary
}

// mainButtons only lists what can be done for the selection, the expected
// action first. Everything else is in ctrl+k and ?.
func (m Model) mainButtons() []button {
	if m.selectedOther() != nil {
		return []button{{"enter", i18n.T("Aller à l'agent", "Go to agent"), true}}
	}
	c := m.selected()
	if c == nil {
		return nil
	}
	return m.chantierButtons(c)
}

// chantierButtons are the actions possible on c, the card's first.
func (m Model) chantierButtons(c *chantier.Chantier) []button {
	var bs []button
	seen := map[string]bool{}
	push := func(b button) {
		if b.enabled && !seen[b.key] {
			seen[b.key] = true
			bs = append(bs, b)
		}
	}
	if cd := m.cardFor(c); cd != nil {
		for _, b := range cd.actions {
			push(b)
		}
	}
	s := c.State
	hasWT := c.Worktree != ""
	push(button{"e", i18n.T("Répondre", "Answer"), s != nil && len(s.OpenQuestions()) > 0})
	push(button{"enter", i18n.T("Aller à l'agent", "Go to agent"), len(c.Agents) > 0 || len(c.Workspaces) > 0})
	push(button{"d", "Diff", hasWT})
	push(m.testButton(c))
	push(button{"p", i18n.T("Planifier", "Plan"), c.Issue != nil && (!hasWT || c.Main)})
	push(button{"v", i18n.T("Valider", "Approve"), s != nil && s.Waiting != "" && !isPushWait(s.Waiting) && !isPRWait(s.Waiting)})
	push(button{"a", i18n.T("Autoriser le push", "Allow push"), m.canAllowPush(c)})
	push(button{"P", i18n.T("Ouvrir la PR", "Open the PR"), s != nil && c.OpenPR() == nil && !s.PRAllowed() && c.Phase >= chantier.Build && c.Phase != chantier.Shipped})
	push(button{"f", i18n.T("Corriger la CI", "Fix CI"), m.ciFailed(c)})
	push(button{"c", i18n.T("Commentaires", "Comments"), len(c.PRs) > 0})
	if plan, _ := m.planOf(c); plan != nil {
		push(button{"i", howTitle(), plan.HowItWorks != "" || c.Phase >= chantier.Build})
	}
	push(button{"o", "Ticket", c.Issue != nil && c.Issue.URL != ""})
	push(button{"x", i18n.T("Fermer", "Close"), c.Phase == chantier.Shipped})
	return bs
}

// canAllowPush: the plan blocks pushes and the work is far enough to push.
func (m Model) canAllowPush(c *chantier.Chantier) bool {
	s := c.State
	return s != nil && c.Phase != chantier.Shipped && !m.pushValid(c) && (isPushWait(s.Waiting) || c.Phase >= chantier.Build)
}

// hints are the global keys shown on the right of the main and board footers.
func (m Model) hints() []button {
	t := button{"t", i18n.T("tableau", "board"), true}
	if m.view == "board" {
		t.label = i18n.T("liste", "list")
	}
	return []button{
		{"n", i18n.T("nouveau", "new"), len(m.projects) > 0},
		t,
		{"ctrl+k", "palette", true},
		{"?", i18n.T("aide", "help"), true},
	}
}

var (
	btnBg      = lipgloss.AdaptiveColor{Light: "#ede9fe", Dark: "#44475a"}
	btnHoverBg = lipgloss.AdaptiveColor{Light: "#c4b5fd", Dark: "#6272a4"}
	btnFg      = lipgloss.AdaptiveColor{Light: "#1f2937", Dark: "#f8f8f2"}
	onAccent   = lipgloss.AdaptiveColor{Light: "#ffffff", Dark: "#282a36"}
	sBtnOff    = lipgloss.NewStyle().Padding(0, 1).Foreground(muted)
)

func keyGlyph(k string) string {
	switch k {
	case "enter":
		return "↵"
	case "esc":
		return "⎋"
	}
	return k
}

func renderButton(b button, hover, primary bool) string {
	key := keyGlyph(b.key)
	if !b.enabled {
		return sBtnOff.Render(key + " " + b.label)
	}
	if primary {
		bg := lipgloss.TerminalColor(accent)
		if hover {
			bg = btnHoverBg
		}
		return lipgloss.NewStyle().Background(bg).Foreground(onAccent).Bold(true).Render(" " + key + " " + b.label + " ")
	}
	bg := btnBg
	if hover {
		bg = btnHoverBg
	}
	k := lipgloss.NewStyle().Background(bg).Foreground(accent).Bold(true).Render(" " + key)
	l := lipgloss.NewStyle().Background(bg).Foreground(btnFg).Render(" " + b.label + " ")
	return k + l
}

func renderHint(b button, hover bool) string {
	st := sMuted
	if hover {
		st = st.Underline(true)
	}
	return sKey.Render(b.key) + st.Render(" "+b.label)
}

// status is the message of the status line: work in progress, the last
// outcome, or what the mouse points at.
func (m Model) status() string {
	switch {
	case m.busy != "":
		return sWarn.Render(m.busy)
	case m.flash != "" && m.flashErr:
		return sErr.Render(m.flash)
	case m.flash != "":
		return sGood.Render(m.flash)
	}
	if m.hoverItem >= 0 && m.hoverItem < len(m.chantiers) {
		if s := agentsSummary(m.chantiers[m.hoverItem]); s != "" {
			return lipgloss.NewStyle().Foreground(soft).Render(s)
		}
	}
	return ""
}

// footer renders the button row and its clickable spans. On the main and
// board screens the right side holds the status and the global keys; a
// status with no room there spills to the row under the header.
func (m Model) footer() (string, []hit, string) {
	bs := m.buttons()
	primary := m.primaryFirst()
	var b strings.Builder
	var hits []hit
	x := 0
	for i, bt := range bs {
		r := renderButton(bt, i == m.hoverBtn, primary && i == 0)
		w := lipgloss.Width(r)
		if i > 0 {
			b.WriteString(" ")
			x++
		}
		b.WriteString(r)
		key := ""
		if bt.enabled {
			key = bt.key
		}
		hits = append(hits, hit{x, x + w, key})
		x += w
	}
	left := b.String()
	if m.view != "" && m.view != "board" {
		return ansi.Truncate(left, m.width, ""), hits, ""
	}
	room := m.width - x - 3
	hs := m.hints()
	status := m.status()
	spill := ""
	// Drop global keys from the left until the status and the keys fit.
	for len(hs) > 0 {
		need := lipgloss.Width(status) + 2
		for _, h := range hs {
			need += lipgloss.Width(renderHint(h, false)) + 2
		}
		if need <= room {
			break
		}
		hs = hs[1:]
	}
	keysW := 0
	for _, h := range hs {
		keysW += lipgloss.Width(renderHint(h, false)) + 2
	}
	if status != "" && lipgloss.Width(status) > room-keysW {
		if room-keysW >= 24 {
			status = ansi.Truncate(status, room-keysW, "…")
		} else {
			spill, status = ansi.Truncate(status, m.width, "…"), ""
		}
	}
	var parts []string
	if status != "" {
		parts = append(parts, status)
	}
	for i, h := range hs {
		parts = append(parts, renderHint(h, m.hoverBtn == len(bs)+i))
	}
	right := strings.Join(parts, "  ")
	gap := m.width - x - lipgloss.Width(right)
	if gap < 1 {
		return ansi.Truncate(left, m.width, ""), hits, spill
	}
	// The global keys' spans, from where the right side starts.
	off := x + gap
	for i, p := range parts {
		w := lipgloss.Width(p)
		if j := i - (len(parts) - len(hs)); j >= 0 {
			hits = append(hits, hit{off, off + w, hs[j].key})
		}
		off += w + 2
	}
	return left + strings.Repeat(" ", gap) + right, hits, spill
}

func (m Model) viewFooter() string {
	f, _, _ := m.footer()
	return f
}

// viewFlash is the message row of the comments and new-ticket screens.
func (m Model) viewFlash() string { return ansi.Truncate(m.status(), m.width, "…") }

// ── small renderers ─────────────────────────────────────────────────────────

func trunc(s string, w int) string {
	if w <= 1 {
		return ""
	}
	return runewidth.Truncate(s, w, "…")
}

func agentIcon(status string) string {
	switch status {
	case "working":
		return lipgloss.NewStyle().Foreground(working).Render("◐")
	case "blocked":
		return sWarn.Render("⚠")
	case "done":
		return sGood.Render("✓")
	case "idle":
		return sDim.Render("○")
	}
	return sDim.Render("?")
}

func phaseStyle(p chantier.Phase) lipgloss.Style {
	switch p {
	case chantier.Review:
		return lipgloss.NewStyle().Foreground(accent)
	case chantier.Build:
		return lipgloss.NewStyle().Foreground(working)
	case chantier.Shipped:
		return sGood
	}
	return sMuted
}

// wrap breaks text to width w and indents every line.
func wrap(text string, w int, indent string) []string {
	out := strings.Split(lipgloss.NewStyle().Width(max(10, w-len(indent))).Render(text), "\n")
	for i := range out {
		out[i] = indent + strings.TrimRight(out[i], " ")
	}
	return out
}

// hang wraps text after prefix, continuation lines aligned under the text:
// nothing in the detail pane is cut, the pane scrolls instead.
func hang(prefix, text string, w int) []string {
	pw := lipgloss.Width(prefix)
	body := wrap(text, w-pw, "")
	out := make([]string, len(body))
	for i, l := range body {
		if i == 0 {
			out[i] = prefix + l
		} else {
			out[i] = strings.Repeat(" ", pw) + l
		}
	}
	return out
}

func bar(done, doing, total, width int) string {
	if total == 0 {
		return ""
	}
	d := done * width / total
	g := min(width-d, (doing*width+total-1)/total)
	return sGood.Render(strings.Repeat("█", d)) +
		lipgloss.NewStyle().Foreground(working).Render(strings.Repeat("▓", g)) +
		sLine.Render(strings.Repeat("░", width-d-g))
}

func taskIcon(t state.Task) string {
	switch {
	case t.Done:
		return sGood.Render("✓")
	case t.Doing:
		return lipgloss.NewStyle().Foreground(working).Render("◐")
	}
	return sDim.Render("○")
}

const label = 10 // width of the left labels in the detail pane

func lbl(s string) string {
	return sMuted.Render(s + strings.Repeat(" ", max(1, label-runewidth.StringWidth(s))))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// plainMD drops the markdown marks that only add noise in a terminal.
func plainMD(s string) string {
	return strings.NewReplacer("**", "", "`", "", "__", "").Replace(s)
}

func prLine(p progress.PR) string {
	var parts []string
	switch {
	case p.State == "MERGED":
		parts = append(parts, sGood.Render(i18n.T("mergée", "merged")))
	case p.State == "CLOSED":
		parts = append(parts, sMuted.Render(i18n.T("fermée", "closed")))
	case p.Draft:
		parts = append(parts, sMuted.Render(i18n.T("brouillon", "draft")))
	default:
		parts = append(parts, i18n.T("ouverte", "open"))
	}
	ck := p.Checks
	switch {
	case ck.Failed > 0:
		parts = append(parts, sErr.Render(fmt.Sprintf(i18n.T("CI ✗ %d en échec", "CI ✗ %d failing"), ck.Failed)))
	case ck.Pending > 0:
		parts = append(parts, lipgloss.NewStyle().Foreground(working).Render(fmt.Sprintf(i18n.T("CI ◐ %d en cours", "CI ◐ %d running"), ck.Pending)))
	case ck.Passed > 0:
		parts = append(parts, sGood.Render(fmt.Sprintf("CI ✓ %d", ck.Passed)))
	}
	switch p.ReviewDecision {
	case "APPROVED":
		parts = append(parts, sGood.Render(i18n.T("review approuvée", "review approved")))
	case "CHANGES_REQUESTED":
		parts = append(parts, sErr.Render(i18n.T("changements demandés", "changes requested")))
	default:
		if p.State == "OPEN" {
			parts = append(parts, sMuted.Render(i18n.T("review en attente", "review pending")))
		}
	}
	if p.State == "OPEN" {
		switch p.Mergeable {
		case "MERGEABLE":
			parts = append(parts, sGood.Render("mergeable"))
		case "CONFLICTING":
			parts = append(parts, sErr.Render(i18n.T("conflits", "conflicts")))
		}
	}
	if p.Comments > 0 {
		parts = append(parts, sMuted.Render(fmt.Sprintf(i18n.T("%d commentaires", "%d comments"), p.Comments)))
	}
	return strings.Join(parts, sMuted.Render(" · "))
}

func tilde(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
