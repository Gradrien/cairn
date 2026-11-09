package tui

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"cairn/internal/chantier"
	"cairn/internal/i18n"
	"cairn/internal/journal"
	"cairn/internal/progress"
	"cairn/internal/state"
)

// The detail pane leads with what is expected of you (the card), then shows
// the sections that matter for the phase first: questions while planning,
// the work while building, the side effects before a push, the merge
// checklist in review. Finished lists fold away.

// card says what the selected chantier is waiting for, with the buttons that do it.
type card struct {
	color   lipgloss.TerminalColor
	title   string
	since   string
	sub     string
	actions []button
	primary bool // the first action is what to do now
}

// lowerFirst lowers the first letter of a sentence put after a colon,
// leaving acronyms ("PR") alone.
func lowerFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 || len(s) <= n {
		return s
	}
	if r2, _ := utf8.DecodeRuneInString(s[n:]); unicode.IsUpper(r2) {
		return s
	}
	return string(unicode.ToLower(r)) + s[n:]
}

func (m Model) cardFor(c *chantier.Chantier) *card {
	s := c.State
	hasWT := c.Worktree != ""
	agent := button{"enter", i18n.T("Aller à l'agent", "Go to agent"), len(c.Agents) > 0 || len(c.Workspaces) > 0}
	comments := button{"c", i18n.T("Commentaires", "Comments"), len(c.PRs) > 0}
	pr, hasPR := m.prs[c.Key]
	prOK := hasPR && pr.Err == "" && c.OpenPR() != nil
	since := ""
	if s != nil {
		since = progress.Elapsed(s.UpdatedAt)
		if since != "" {
			since = fmt.Sprintf(i18n.T("depuis %s", "for %s"), since)
		}
	}
	working := false
	for _, a := range c.Agents {
		working = working || a.Status == "working"
	}

	switch {
	case c.Phase == chantier.Shipped:
		sub := i18n.T("Rangement automatique en cours : worktree supprimé et plan archivé.", "Cleaning up on its own: worktree removed, plan archived.")
		if why := m.watch.keep[c.Key]; why != "" {
			sub = fmt.Sprintf(i18n.T("Gardé (%s) : ferme-le quand tu as vérifié.", "Kept (%s): close it once you checked."), why)
		}
		return &card{color: good, title: "✓ " + i18n.T("Livré", "Shipped"), sub: sub, primary: true, actions: []button{
			{"x", i18n.T("Fermer le chantier", "Close the ticket"), true},
			{"d", i18n.T("Relire le diff", "Review the diff"), hasWT},
			agent,
		}}
	case s != nil && (isPushWait(s.Waiting) || isPRWait(s.Waiting)):
		var sub []string
		if len(s.Tasks) > 0 {
			sub = append(sub, fmt.Sprintf(i18n.T("tâches %d/%d", "tasks %d/%d"), s.DoneCount(), len(s.Tasks)))
		}
		if n := len(s.SideEffects); n > 0 {
			sub = append(sub, fmt.Sprintf(i18n.T("effets de bord %d/%d vérifiés", "side effects %d/%d checked"), state.Count(s.SideEffects, true), n))
		}
		if g, ok := m.gits[c.Key]; ok && g.Ahead > 0 {
			sub = append(sub, plural(g.Ahead, i18n.T("commit prêt", "commit ready"), i18n.T("commits prêts", "commits ready")))
		}
		openPR := button{"P", i18n.T("Ouvrir la PR", "Open the PR"), c.OpenPR() == nil && !s.PRAllowed()}
		if isPRWait(s.Waiting) {
			return &card{color: warn, title: "⚠ " + i18n.T("À toi : ouvrir la PR", "Your call: open the PR"), since: since,
				sub: strings.Join(sub, " · "), primary: true, actions: []button{openPR, {"d", i18n.T("Relire le diff", "Review the diff"), hasWT}, agent}}
		}
		return &card{color: warn, title: "⚠ " + i18n.T("À toi : autoriser le push", "Your call: allow the push"), since: since,
			sub: strings.Join(sub, " · "), primary: true, actions: []button{
				{"a", i18n.T("Autoriser le push", "Allow push"), true},
				openPR,
				{"d", i18n.T("Relire le diff", "Review the diff"), hasWT},
				agent,
			}}
	case s != nil && s.Waiting != "":
		var sub []string
		q := len(s.OpenQuestions())
		if q > 0 {
			sub = append(sub, plural(q, i18n.T("question ouverte à trancher", "open question to settle"), i18n.T("questions ouvertes à trancher", "open questions to settle")))
		}
		if n := len(s.Tasks); n > 0 && s.Phase == "plan" {
			sub = append(sub, plural(n, i18n.T("tâche", "task"), i18n.T("tâches", "tasks")))
		}
		if n := len(s.SideEffects); n > 0 {
			sub = append(sub, plural(n, i18n.T("effet de bord recensé", "side effect listed"), i18n.T("effets de bord recensés", "side effects listed")))
		}
		return &card{color: warn, title: "⚠ " + i18n.T("À toi : ", "Your call: ") + lowerFirst(s.Waiting), since: since,
			sub: strings.Join(sub, " · "), primary: true, actions: []button{
				{"v", i18n.T("Valider", "Approve"), true},
				{"e", i18n.T("Répondre aux questions", "Answer the questions"), q > 0},
				{"d", "Diff", hasWT},
				comments,
			}}
	case c.Waiting() != "":
		return &card{color: warn, title: "⚠ " + c.Waiting(), sub: i18n.T("Réponds-lui dans son pane.", "Answer it in its pane."),
			primary: true, actions: []button{agent}}
	case prOK && pr.Checks.Failed > 0:
		return &card{color: danger, title: fmt.Sprintf(i18n.T("✗ La CI casse : %s en échec", "✗ CI is broken: %s failing"), plural(pr.Checks.Failed, "check", "checks")),
			sub:     strings.Join(pr.Checks.FailedNames, " · ") + i18n.T(" · Corriger la CI envoie les logs en échec à l'agent", " · Fix CI sends the failing logs to the agent"),
			primary: true, actions: []button{{"f", i18n.T("Corriger la CI", "Fix CI"), true}, comments, agent}}
	case prOK && pr.ReviewDecision == "CHANGES_REQUESTED":
		return &card{color: warn, title: fmt.Sprintf(i18n.T("⚠ Changements demandés sur la PR #%d", "⚠ Changes requested on PR #%d"), c.OpenPR().Number),
			sub:     i18n.T("Dans les commentaires, [→ agent] envoie un retour à l'agent.", "In the comments, [→ agent] sends feedback to the agent."),
			primary: true, actions: []button{comments, agent}}
	case prOK:
		checks, ready := m.readiness(c)
		var ok, missing []string
		for _, k := range checks {
			if k.ok {
				ok = append(ok, k.label)
			} else {
				missing = append(missing, k.label)
			}
		}
		if ready {
			return &card{color: good, title: fmt.Sprintf(i18n.T("✓ PR #%d prête à merger", "✓ PR #%d ready to merge"), c.OpenPR().Number),
				sub: strings.Join(ok, " · "), primary: true, actions: []button{comments}}
		}
		sub := ""
		if len(ok) > 0 {
			sub = i18n.T("déjà bon : ", "already fine: ") + strings.Join(ok, " · ")
		}
		return &card{color: line, title: i18n.T("○ En attente : ", "○ Waiting on: ") + strings.Join(missing, ", "), sub: sub,
			actions: []button{comments}}
	case c.Phase == chantier.ToPlan:
		return &card{color: dim, title: i18n.T("› Pas encore commencé", "› Not started yet"),
			sub:     i18n.T("Planifier crée le worktree et son espace herdr, lance Claude et lui demande un plan à la taille du ticket.", "Plan creates the worktree and its herdr workspace, starts Claude and asks it for a plan sized to the ticket."),
			primary: true, actions: []button{{"p", i18n.T("Planifier", "Plan"), c.Issue != nil}}}
	case working:
		sub := i18n.T("cairn remonte ce chantier en haut dès qu'il attend une décision.", "cairn moves this ticket to the top as soon as it waits for a decision.")
		at := ""
		if s != nil && len(s.Tasks) > 0 {
			at = fmt.Sprintf(i18n.T("tâche %d/%d", "task %d/%d"), min(len(s.Tasks), s.DoneCount()+1), len(s.Tasks))
		}
		return &card{color: line, title: i18n.T("◐ L'agent travaille : rien à faire pour toi", "◐ The agent is working: nothing for you to do"),
			since: at, sub: sub, actions: []button{{"enter", i18n.T("Regarder l'agent", "Watch the agent"), true}}}
	case len(c.Agents) > 0 && c.Phase != chantier.Shipped:
		return &card{color: line, title: i18n.T("○ L'agent est inactif et ne demande rien", "○ The agent is idle and asks for nothing"),
			sub: i18n.T("Va voir où il en est, ou relance-le.", "See where it stands, or nudge it."), actions: []button{agent}}
	}
	return nil
}

// ── layout ──────────────────────────────────────────────────────────────────

type detail struct {
	lines    []string
	targets  []string // what a click on each line does, "" for nothing
	sections []int    // first line of each section, for [ and ]
	cardRow  int      // line of the card's buttons, -1 if none
	cardHits []hit    // the card's buttons, x relative to the pane
}

type detailBuilder struct {
	detail
	w int
}

func (b *detailBuilder) add(s ...string) {
	for _, l := range s {
		b.lines = append(b.lines, l)
		b.targets = append(b.targets, "")
	}
}

// addT adds lines that all do target when clicked.
func (b *detailBuilder) addT(target string, s ...string) {
	for _, l := range s {
		b.lines = append(b.lines, l)
		b.targets = append(b.targets, target)
	}
}

// section starts a section: "── Title count ───────── right".
func (b *detailBuilder) section(title, count string, right string) {
	b.add("")
	b.sections = append(b.sections, len(b.lines))
	head := sDim.Render("── ") + sSection.Render(title)
	if count != "" {
		head += " " + count
	}
	tail := ""
	if right != "" {
		tail = " " + sMuted.Render(right)
	}
	fill := b.w - lipgloss.Width(head) - lipgloss.Width(tail) - 1
	if fill < 2 {
		b.add(ansi.Truncate(head+tail, b.w, "…"))
		return
	}
	b.add(head + " " + sLine.Render(strings.Repeat("─", fill)) + tail)
}

// fold says whether section id of the selection shows folded.
func (m Model) unfolded(key, id string) bool {
	return m.unfold[key+":*"] || m.unfold[key+":"+id]
}

func (b *detailBuilder) card(cd *card, primary bool) {
	inner := b.w - 4
	title := lipgloss.NewStyle().Foreground(cd.color).Bold(true).Render(cd.title)
	if cd.color == line || cd.color == dim {
		title = sSoftBold.Render(cd.title)
	}
	content := []string{spread(title, sMuted.Render(cd.since), inner)}
	if cd.sub != "" {
		for _, l := range wrap(cd.sub, inner, "") {
			content = append(content, sSoft.Render(l))
		}
	}
	var row strings.Builder
	var hits []hit
	x := 2 // border and padding
	for i, bt := range cd.actions {
		if !bt.enabled {
			continue
		}
		r := renderButton(bt, false, primary && i == 0)
		w := lipgloss.Width(r)
		if x-2+w > inner {
			break
		}
		if row.Len() > 0 {
			row.WriteString(" ")
			x++
		}
		row.WriteString(r)
		hits = append(hits, hit{x, x + w, bt.key})
		x += w
	}
	content = append(content, row.String())
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cd.color).Padding(0, 1).Width(b.w - 2).
		Render(strings.Join(content, "\n"))
	b.add("")
	start := len(b.lines)
	b.addT("card", strings.Split(box, "\n")...)
	b.cardRow = start + len(content) // border row, then the content: the buttons are last
	b.cardHits = hits
}

// detailLayout renders the detail pane of the selection.
func (m Model) detailLayout(w int) detail {
	b := &detailBuilder{w: w, detail: detail{cardRow: -1}}
	if a := m.selectedOther(); a != nil {
		b.add(sBold.Render(a.Label())+" "+agentIcon(a.Status)+" "+a.Status, "")
		b.add(hang(lbl(i18n.T("Titre", "Title")), a.Title, w)...)
		b.add(hang(lbl(i18n.T("Dossier", "Folder")), tilde(a.Dir()), w)...)
		b.add(lbl("Pane")+a.PaneID, "",
			sMuted.Render(i18n.T("Cet agent ne travaille sur aucun ticket connu.", "This agent works on no known ticket.")))
		if len(m.chantiers) > 0 {
			b.section(i18n.T("Rattacher à un chantier", "Attach to a ticket"), "", i18n.T("clic : l'agent prend le nom du ticket", "click: the agent takes the ticket's name"))
			for _, c := range m.chantiers {
				b.addT("attach:"+c.Key, hang("  "+sImg.Render("› "+c.Key)+" ", c.Title(), w)...)
			}
		}
		return b.detail
	}
	c := m.selected()
	if c == nil {
		b.add(m.statusLines(w)...)
		return b.detail
	}
	s := c.State

	// Title and ticket.
	title := wrap(c.Title(), w-len(c.Key)-2, "")
	b.add(sBold.Render(c.Key) + "  " + sBold.Render(title[0]))
	for _, l := range title[1:] {
		b.add(strings.Repeat(" ", len(c.Key)+2) + sBold.Render(l))
	}
	if c.Issue != nil {
		meta := []string{c.Issue.Status}
		if c.Issue.Priority != "" {
			meta = append(meta, c.Issue.Priority)
		}
		if c.Issue.Estimate != "" {
			meta = append(meta, c.Issue.Estimate)
		}
		src := c.Project
		if p := m.projectOf(c); p != nil && p.tr != nil {
			src = p.tr.Name()
			if len(m.projects) > 1 {
				src = p.cfg.Name + " · " + src
			}
		}
		var parts []string
		for _, s := range append([]string{src}, meta...) {
			if s != "" {
				parts = append(parts, s)
			}
		}
		b.add(sMuted.Render(strings.Join(parts, " · ")))
	}

	// What it waits for.
	if cd := m.cardFor(c); cd != nil {
		b.card(cd, cd.primary)
	}
	m.secNews(b, c)

	// Where it stands.
	b.add("")
	if c.Phase == chantier.ToPlan {
		b.add(lbl("Phase") + sSoftBold.Render(chantier.ToPlan.Label()))
	} else {
		var flow []string
		for _, p := range chantier.Flow {
			switch {
			case p < c.Phase:
				flow = append(flow, sGood.Render(p.Label()+" ✓"))
			case p == c.Phase:
				flow = append(flow, phaseStyle(p).Bold(true).Render(p.Label()+" ●"))
			default:
				flow = append(flow, sDim.Render(p.Label()))
			}
		}
		start := journal.PhaseSince(m.watch.history[c.Key])
		b.add(lbl("Phase") + strings.Join(flow, sDim.Render(" → ")) + sMuted.Render("  "+since(start)))
	}
	var activity []string
	if s != nil && !s.UpdatedAt.IsZero() {
		activity = append(activity, "plan "+progress.Ago(s.UpdatedAt))
	}
	if g, ok := m.gits[c.Key]; ok && g.Ahead > 0 && !g.LastAt.IsZero() {
		activity = append(activity, "commit "+progress.Ago(g.LastAt))
	}
	if len(activity) > 0 {
		b.add(lbl(i18n.T("Activité", "Activity")) + strings.Join(activity, sMuted.Render(" · ")))
	}
	if s != nil {
		switch {
		case m.prs[c.Key].State == "OPEN":
			b.add(lbl("Push") + sGood.Render(i18n.T("libre", "free")) + sMuted.Render(i18n.T(" : PR ouverte, la relecture se fait dessus", ": the PR is open, review happens there")))
		case m.pushValid(c) && s.PushCommit() == "":
			b.add(lbl("Push") + sGood.Render(i18n.T("autorisé à ta demande", "allowed on your request")) + sMuted.Render(i18n.T(" : le prochain push fixe le commit", ": the next push pins the commit")))
		case m.pushValid(c):
			b.add(lbl("Push") + sGood.Render(fmt.Sprintf(i18n.T("autorisé pour %s", "allowed for %s"), shortSHA(s.PushCommit()))) + sMuted.Render(i18n.T(" : ce commit et ses nouvelles tentatives passent", ": this commit and its retries pass")))
		case s.PushAllowed():
			b.add(lbl("Push") + sWarn.Render(fmt.Sprintf(i18n.T("nouveaux commits depuis l'autorisation (%s) : à réautoriser", "new commits since the allowance (%s): allow again"), shortSHA(s.PushCommit()))))
		case isPushWait(s.Waiting):
			b.add(lbl("Push") + sWarn.Render(i18n.T("bloqué : l'agent attend ton feu vert", "blocked: the agent waits for your go-ahead")))
		default:
			b.add(lbl("Push") + sMuted.Render(i18n.T("bloqué jusqu'à ton feu vert, ici ou demandé dans le chat", "blocked until you allow it, here or by asking in the chat")))
		}
		switch {
		case m.prs[c.Key].State == "OPEN" || c.OpenPR() != nil || c.Phase == chantier.Shipped:
		case s.PRAllowed():
			b.add(lbl("PR") + sGood.Render(i18n.T("ouverture autorisée", "opening allowed")) + sMuted.Render(i18n.T(" : l'agent l'ouvre", ": the agent opens it")))
		case isPRWait(s.Waiting):
			b.add(lbl("PR") + sWarn.Render(i18n.T("l'agent attend ton feu vert pour l'ouvrir", "the agent waits for your go-ahead to open it")))
		case c.Phase >= chantier.Build:
			b.add(lbl("PR") + sMuted.Render(i18n.T("pas encore : Ouvrir la PR (P) quand tu veux", "not yet: Open the PR (P) when you want")))
		}
	}
	if s == nil && c.Phase != chantier.ToPlan {
		b.add("", sMuted.Render(i18n.T("Pas de plan cairn pour ce chantier : seuls le code et la PR sont suivis.", "No cairn plan for this ticket: only the code and the PR are tracked.")))
	}

	// The sections, in the order the phase calls for.
	_, prOpen := m.prs[c.Key]
	prOpen = prOpen && c.OpenPR() != nil
	switch {
	case s != nil && (isPushWait(s.Waiting) || isPRWait(s.Waiting)):
		m.secSide(b, c)
		m.secHow(b, c)
		m.secCode(b, c)
		m.secProofs(b, c)
		m.secQuestions(b, c)
		m.secTasks(b, c)
		m.secGoal(b, c)
		m.secPR(b, c, false)
	case c.Phase <= chantier.Plan && s != nil && s.Waiting != "":
		// A plan to approve leads with its questions, even with a PR already open.
		m.secQuestions(b, c)
		m.secTasks(b, c)
		m.secSide(b, c)
		m.secGoal(b, c)
		m.secPR(b, c, false)
		m.secProofs(b, c)
		m.secCode(b, c)
	case prOpen || c.Phase == chantier.Review:
		m.secPR(b, c, true)
		m.secQuestions(b, c)
		m.secHow(b, c)
		m.secSide(b, c)
		m.secCode(b, c)
		m.secProofs(b, c)
		m.secTasks(b, c)
		m.secGoal(b, c)
	case c.Phase == chantier.Build:
		m.secQuestions(b, c)
		m.secTasks(b, c)
		m.secHow(b, c)
		m.secProofs(b, c)
		m.secSide(b, c)
		m.secGoal(b, c)
		m.secCode(b, c)
		m.secPR(b, c, false)
	default:
		m.secQuestions(b, c)
		m.secTasks(b, c)
		m.secSide(b, c)
		m.secGoal(b, c)
		m.secHow(b, c)
		m.secProofs(b, c)
		m.secCode(b, c)
		m.secPR(b, c, false)
	}
	m.secHistory(b, c)
	m.secAgents(b, c)
	if s != nil {
		b.add("", sDim.Render(i18n.T("plan : ", "plan: ")+tilde(s.Path)))
	}
	return b.detail
}

// answerKey identifies an answered question across reloads of the plan.
func answerKey(key, question string) string { return key + "\x00" + question }

func (m Model) secQuestions(b *detailBuilder, c *chantier.Chantier) {
	if c.State == nil {
		return
	}
	qs := c.State.OpenQuestions()
	if len(qs) == 0 {
		return
	}
	b.section(i18n.T("Questions ouvertes", "Open questions"), sWarn.Render(fmt.Sprint(len(qs))), "")
	for i, q := range qs {
		if reply, ok := m.answered[answerKey(c.Key, q)]; ok {
			b.add(hang("  "+sGood.Render("✓")+" ", sMuted.Render(plainMD(q)), b.w)...)
			b.add(hang("    "+sGood.Render(i18n.T("↳ envoyé à l'agent : ", "↳ sent to the agent: ")), reply, b.w)...)
			continue
		}
		text := plainMD(q)
		if m.answerQ != q {
			text += "  " + sKey.Render(i18n.T("› répondre", "› answer"))
		}
		b.addT(fmt.Sprintf("answer:%d", i), hang("  "+sWarn.Render("?")+" ", text, b.w)...)
		if m.answerQ == q {
			b.add(m.answerBox(b.w)...)
		}
	}
}

func (m Model) secTasks(b *detailBuilder, c *chantier.Chantier) {
	s := c.State
	if s == nil || len(s.Tasks) == 0 {
		return
	}
	done, doing := state.Count(s.Tasks, true), state.Count(s.Tasks, false)
	count := sMuted.Render(fmt.Sprintf("%d/%d", done, len(s.Tasks)))
	if done == len(s.Tasks) {
		count = sGood.Render(fmt.Sprintf("%d/%d", done, len(s.Tasks)))
	}
	right := ""
	if doing > 0 {
		right = fmt.Sprintf(i18n.T("%d en cours", "%d in progress"), doing)
	}
	b.section(i18n.T("Tâches", "Tasks"), count+" "+bar(done, doing, len(s.Tasks), 16), right)
	// Once the build started, finished tasks fold away under one line.
	fold := c.Phase >= chantier.Build && done > 0 && !m.unfolded(c.Key, "tasks")
	for _, t := range s.Tasks {
		if !(fold && t.Done) {
			b.add(hang("  "+taskIcon(t)+" ", plainMD(t.Text), b.w)...)
		}
	}
	switch {
	case fold:
		b.addT("fold:tasks", "  "+sGood.Render("✓")+" "+sMuted.Render(plural(done, i18n.T("tâche faite", "task done"), i18n.T("tâches faites", "tasks done")))+"  "+sKey.Render(i18n.T("› afficher", "› show")))
	case c.Phase >= chantier.Build && done > 0:
		b.addT("fold:tasks", "  "+sKey.Render(i18n.T("‹ replier les tâches faites", "‹ fold the done tasks")))
	}
}

func (m Model) secSide(b *detailBuilder, c *chantier.Chantier) {
	s := c.State
	if s == nil {
		return
	}
	if len(s.SideEffects) == 0 {
		if c.Phase >= chantier.Build {
			b.add("", sWarn.Render(i18n.T("Effets de bord : aucun recensé dans le plan", "Side effects: none listed in the plan")))
		}
		return
	}
	done := state.Count(s.SideEffects, true)
	st := sMuted
	if done < len(s.SideEffects) && (c.Phase >= chantier.Build || isPushWait(s.Waiting)) {
		st = sWarn
	} else if done == len(s.SideEffects) {
		st = sGood
	}
	right := ""
	if done < len(s.SideEffects) {
		right = i18n.T("à cocher avant la PR", "to check before the PR")
	}
	b.section(i18n.T("Effets de bord", "Side effects"), st.Render(fmt.Sprintf(i18n.T("%d/%d vérifiés", "%d/%d checked"), done, len(s.SideEffects))), right)
	// Unchecked first: they are what is left to do.
	for _, pass := range []bool{false, true} {
		for _, t := range s.SideEffects {
			if t.Done == pass {
				b.add(hang("  "+taskIcon(t)+" ", plainMD(t.Text), b.w)...)
			}
		}
	}
}

func (m Model) secGoal(b *detailBuilder, c *chantier.Chantier) {
	s := c.State
	if s == nil || len(s.Summary) == 0 {
		return
	}
	b.section(i18n.T("Objectif", "Goal"), "", "")
	lines := wrap(plainMD(strings.Join(s.Summary, " ")), b.w, "  ")
	if len(lines) > 3 && !m.unfolded(c.Key, "goal") {
		b.add(lines[:2]...)
		b.addT("fold:goal", "  "+sMuted.Render("…")+" "+sKey.Render(fmt.Sprintf(i18n.T("› tout lire (%d lignes)", "› read it all (%d lines)"), len(lines))))
		return
	}
	b.add(lines...)
	if len(lines) > 3 {
		b.addT("fold:goal", "  "+sKey.Render(i18n.T("‹ replier", "‹ fold")))
	}
}

func (m Model) secProofs(b *detailBuilder, c *chantier.Chantier) {
	s := c.State
	if s == nil || len(s.Proofs) == 0 {
		return
	}
	b.section(i18n.T("Preuves", "Proofs"), sMuted.Render(fmt.Sprint(len(s.Proofs))), "")
	for _, p := range s.Proofs {
		if f := imagePath(p); f != "" {
			b.addT("img:"+f, hang("  "+sImg.Render("[img]")+" ", p, b.w)...)
			continue
		}
		b.add(hang("  • ", p, b.w)...)
	}
}

func (m Model) secCode(b *detailBuilder, c *chantier.Chantier) {
	if c.Worktree == "" {
		return
	}
	b.section("Code", "", "")
	if g, ok := m.gits[c.Key]; ok {
		if g.Err != "" {
			b.add("  " + sErr.Render(g.Err))
		} else {
			line := sMuted.Render(i18n.T("aucun commit sur la branche", "no commit on the branch"))
			if g.Ahead > 0 {
				line = plural(g.Ahead, "commit", "commits") + " · " + plural(g.Files, i18n.T("fichier", "file"), i18n.T("fichiers", "files")) + " " +
					sGood.Render(fmt.Sprintf("+%d", g.Added)) + " " + sErr.Render(fmt.Sprintf("−%d", g.Removed))
			}
			if g.Uncommitted > 0 && !c.Main {
				line += sWarn.Render(" · " + plural(g.Uncommitted, i18n.T("changement non commité", "uncommitted change"), i18n.T("changements non commités", "uncommitted changes")))
			}
			b.add("  " + line)
			if c.Main {
				// The shared checkout's changes belong to whatever else runs there.
				note := i18n.T("dans le dépôt principal, partagé : cairn n'y touche pas", "in the shared main checkout: cairn leaves it alone")
				if g.Uncommitted > 0 {
					note += fmt.Sprintf(i18n.T(" (%d changements non commités, pas forcément de ce ticket)", " (%d uncommitted changes, not necessarily this ticket's)"), g.Uncommitted)
				}
				b.add(hang("  ", sMuted.Render(note), b.w)...)
			}
			if g.Ahead > 0 && g.LastSubject != "" {
				b.add(hang("  "+sMuted.Render(i18n.T("dernier : ", "last: ")), g.LastSubject, b.w)...)
			}
		}
	} else {
		b.add("  " + sMuted.Render(i18n.T("lecture…", "reading…")))
	}
	b.add(hang("  "+sMuted.Render(i18n.T("branche ", "branch  ")), c.Branch, b.w)...)
	b.add(hang("  "+sMuted.Render(i18n.T("dossier ", "folder  ")), tilde(c.Worktree), b.w)...)
}

// secPR shows the pull request; with checklist, the merge checklist leads,
// one check per line.
func (m Model) secPR(b *detailBuilder, c *chantier.Chantier, checklist bool) {
	last := c.LastPR()
	if last == nil {
		return
	}
	pr := *last
	info, ok := m.prs[c.Key]
	if checklist && ok && info.Err == "" {
		if checks, ready := m.readiness(c); len(checks) > 0 {
			verdict := sMuted.Render(i18n.T("pas encore", "not yet"))
			if ready {
				verdict = sGood.Bold(true).Render(i18n.T("oui", "yes"))
			}
			b.section(i18n.T("Prêt à merger ?", "Ready to merge?"), verdict, "")
			for _, k := range checks {
				switch {
				case k.ok:
					b.add("  " + sGood.Render("✓") + " " + k.label)
				case k.label == "CI" && info.Checks.Failed > 0:
					b.add("  " + sErr.Render("✗") + " " + k.label + sErr.Render(" · "+strings.Join(info.Checks.FailedNames, ", ")))
				default:
					b.add("  " + sDim.Render("○") + " " + k.label)
				}
			}
		}
	}
	b.section(fmt.Sprintf("PR #%d", pr.Number), "", "")
	switch {
	case ok && info.Err == "":
		b.add("  " + prLine(info))
		if len(info.Checks.FailedNames) > 0 {
			b.add(hang("  "+sErr.Render("✗ "), strings.Join(info.Checks.FailedNames, ", "), b.w)...)
			b.addT("fixci", "  "+sImg.Render(i18n.T("› Corriger la CI", "› Fix CI"))+sMuted.Render(i18n.T("  (clic ou f : les logs en échec partent à l'agent)", "  (click or f: the failing logs go to the agent)")))
		}
		if !checklist {
			if checks, ready := m.readiness(c); len(checks) > 0 {
				var parts []string
				for _, k := range checks {
					if k.ok {
						parts = append(parts, sGood.Render("✓ "+k.label))
					} else {
						parts = append(parts, sDim.Render("○ ")+sMuted.Render(k.label))
					}
				}
				verdict := sMuted.Render(i18n.T("pas encore prêt à merger", "not ready to merge yet"))
				if ready {
					verdict = sGood.Bold(true).Render(i18n.T("prêt à merger", "ready to merge"))
				}
				b.add("  "+verdict, "  "+strings.Join(parts, sMuted.Render(" · ")))
			}
		}
	case ok:
		b.add("  " + pr.State + sMuted.Render(" · "+info.Err))
	default:
		b.add("  " + pr.State + sMuted.Render(i18n.T(" · lecture…", " · reading…")))
	}
	b.addT("comments", "  "+sImg.Render(i18n.T("› Commentaires et images", "› Comments and images"))+sMuted.Render(i18n.T("  (clic ou c)", "  (click or c)")))
}

func (m Model) secHistory(b *detailBuilder, c *chantier.Chantier) {
	if events := m.watch.history[c.Key]; len(events) > 0 {
		b.section(i18n.T("Historique", "History"), "", "")
		b.add(historyLines(events, 6)...)
	}
}

func (m Model) secAgents(b *detailBuilder, c *chantier.Chantier) {
	if len(c.Agents) == 0 {
		return
	}
	b.section("Agents", "", i18n.T("clic pour y aller", "click to go there"))
	for _, a := range c.Agents {
		b.addT("pane:"+a.PaneID, hang("  "+agentIcon(a.Status)+" "+sBold.Render(a.Label())+sMuted.Render(" "+a.Status+" · "), a.Title, b.w)...)
	}
}

func (m Model) statusLines(w int) []string {
	var b []string
	if m.localErr != nil {
		b = append(b, sErr.Render(trunc(m.localErr.Error(), w)))
	}
	for _, p := range m.projects {
		if p.err != nil {
			b = append(b, sErr.Render(trunc(p.cfg.Name+" : "+p.err.Error(), w)))
		}
	}
	return b
}

// detailTop is the first line shown of a detail of n lines in h rows.
func (m Model) detailTop(n, h int) int { return min(m.scroll, max(0, n-h)) }

func (m Model) viewDetail(w, h int) string {
	d := m.detailLayout(w)
	top := m.detailTop(len(d.lines), h)
	lines := d.lines[top:]
	if len(lines) > h {
		more := len(d.lines) - top - h + 1
		lines = append([]string(nil), lines[:h]...)
		lines[h-1] = sMuted.Render(fmt.Sprintf(i18n.T("  ↓ %d lignes de plus · espace", "  ↓ %d more lines · space"), more))
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = ansi.Truncate(l, w, "…") // never let a line wrap
	}
	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(strings.Join(out, "\n"))
}

// scrollDetail moves the detail by delta lines, within bounds.
func (m *Model) scrollDetail(delta int) {
	d := m.detailLayout(m.detailWidth())
	h := m.bodyHeight()
	m.scroll = max(0, min(m.detailTop(len(d.lines), h)+delta, len(d.lines)-h))
}

// jumpSection scrolls to the next (dir 1) or previous (dir -1) section.
func (m *Model) jumpSection(dir int) {
	d := m.detailLayout(m.detailWidth())
	h := m.bodyHeight()
	top := m.detailTop(len(d.lines), h)
	target := -1
	for _, s := range d.sections {
		if dir > 0 && s > top+1 && target < 0 {
			target = s - 1
		}
		if dir < 0 && s-1 < top {
			target = s - 1
		}
	}
	if dir < 0 && target < 0 {
		target = 0
	}
	if target >= 0 {
		m.scroll = max(0, min(target, len(d.lines)-h))
	}
}
