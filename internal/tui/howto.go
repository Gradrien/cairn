package tui

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"

	"cairn/internal/chantier"
	"cairn/internal/config"
	"cairn/internal/i18n"
	"cairn/internal/journal"
	"cairn/internal/mux"
	"cairn/internal/prompted"
	"cairn/internal/state"
)

// The "how it works" view: the chantier file's "## Comment ça marche"
// section, full screen, so you can say what was built and how without
// reading the code. A question typed here goes to a short-lived agent of its
// own, which reads the code; cairn writes its answer in that section, so it
// stays with the ticket and the agent at work is not interrupted.

// howFormat is how the agent is asked to write the section, in the build
// prompt and in the requests sent from this view.
func howFormat() string {
	return i18n.T("Écris pour quelqu'un qui doit répondre à des questions sur le ticket sans lire le code. "+
		"Commence par 2 ou 3 lignes sur le parcours d'ensemble, puis un `###` par mécanisme, avec ces puces quand elles s'appliquent : "+
		"**Ce qu'on voit** (le comportement), **Où** (fichiers et fonctions, hooks, composants, en `code`), "+
		"**Quand** (ce qui déclenche : montage, clic, relâchement, sauvegarde, job, webhook…), "+
		"**Données** (où vit l'état : état local, hook, store, URL, localStorage, table ; persisté ou non, envoyé au serveur ou non), "+
		"**Pas fait / limites**. Des faits vérifiés dans le code, pas d'intentions.",
		"Write for someone who must answer questions about the ticket without reading the code. "+
			"Start with 2 or 3 lines on the overall flow, then one `###` per mechanism, with these bullets when they apply: "+
			"**What you see** (the behaviour), **Where** (files and functions, hooks, components, in `code`), "+
			"**When** (what triggers it: mount, click, release, save, job, webhook…), "+
			"**Data** (where the state lives: local state, hook, store, URL, localStorage, table; persisted or not, sent to the server or not), "+
			"**Not done / limits**. Facts checked in the code, not intentions.")
}

// howPrompt asks the agent at work to write or refresh the section.
func howPrompt(key, path string) string {
	head := fmt.Sprintf(i18n.T("Explique comment %s est implémenté : écris ou mets à jour la section `## Comment ça marche` de %s (avant `## Preuves`).",
		"Explain how %s is implemented: write or update the `## Comment ça marche` section of %s (before `## Preuves`)."), key, path)
	return head + "\n\n" + howFormat() + "\n\n" + i18n.T(
		"Ne modifie pas le code, ni la phase, ni l'attente du plan. Lance `cairn sync`, puis reprends où tu en étais, ou arrête-toi si tu attendais.",
		"Do not change the code, the phase or the plan's waiting line. Run `cairn sync`, then resume where you were, or stop if you were waiting.")
}

// howSubject is what the view explains: the selected chantier, or a shipped
// ticket's archived plan (opened from ctrl+k).
type howSubject struct {
	key, title string
	plan       *state.State
	c          *chantier.Chantier // nil for an archived plan
	archivedAt time.Time          // zero unless the plan comes from the archive
}

// archivedPlan is the latest archived plan of key, nil if none.
func (m Model) archivedPlan(key string) *state.Archived {
	for i := range m.archive {
		if m.archive[i].Key == key {
			return &m.archive[i]
		}
	}
	return nil
}

// planOf is c's plan, or its archived one once it shipped and was put away.
func (m Model) planOf(c *chantier.Chantier) (*state.State, time.Time) {
	if c.State != nil {
		return c.State, time.Time{}
	}
	if a := m.archivedPlan(c.Key); a != nil && c.Phase == chantier.Shipped {
		return a.State, a.ArchivedAt
	}
	return nil, time.Time{}
}

func (m Model) howSubj() *howSubject {
	if m.howArch != "" {
		if a := m.archivedPlan(m.howArch); a != nil {
			title := a.Title
			if k, rest, ok := strings.Cut(title, " · "); ok && strings.EqualFold(k, a.Key) {
				title = rest
			}
			return &howSubject{key: a.Key, title: title, plan: a.State, archivedAt: a.ArchivedAt}
		}
		return nil
	}
	c := m.selected()
	if c == nil {
		return nil
	}
	plan, at := m.planOf(c)
	if plan == nil {
		return nil
	}
	return &howSubject{key: c.Key, title: c.Title(), plan: plan, c: c, archivedAt: at}
}

// agent is the pane to ask, "" when nobody can answer (shipped, archived).
func (h *howSubject) agent() string {
	if h == nil || h.c == nil || !h.archivedAt.IsZero() {
		return ""
	}
	return agentPane(h.c)
}

func (m Model) openHow() (tea.Model, tea.Cmd) {
	m.howArch = ""
	if m.howSubj() == nil {
		m.say(i18n.T("Pas de plan cairn pour ce chantier.", "No cairn plan for this ticket."), nil)
		return m, nil
	}
	m.view, m.hscroll, m.howAsk, m.howText = "how", 0, false, ""
	return m, nil
}

// openArchivedHow shows how a shipped ticket was built, from its archived plan.
func (m Model) openArchivedHow(key string) (tea.Model, tea.Cmd) {
	m.howArch, m.overlay = key, ""
	m.view, m.hscroll, m.howAsk, m.howText = "how", 0, false, ""
	return m, nil
}

// requestHow asks the agent at work to write or refresh the section.
func (m Model) requestHow() (tea.Model, tea.Cmd) {
	h := m.howSubj()
	if h == nil {
		return m, nil
	}
	pane := h.agent()
	if pane == "" {
		why := mux.Words(i18n.T("Aucun agent sur %s pour expliquer : relance-le depuis son espace herdr.", "No agent on %s to explain: restart it from its herdr workspace."))
		if !h.archivedAt.IsZero() {
			why = i18n.T("%s est livré et rangé : plus d'agent pour répondre.", "%s shipped and was put away: no agent left to answer.")
		}
		m.say(fmt.Sprintf(why, h.key), nil)
		return m, nil
	}
	prompt, key := howPrompt(h.key, planPath(h.c)), h.key
	text, event := i18n.T("Explication demandée à l'agent de %s.", "Explanation asked from %s's agent."), "explication demandée"
	return m, func() tea.Msg {
		_ = journal.Append(config.ChantiersDir(), key, event)
		return doneMsg{text: fmt.Sprintf(text, key), err: mux.Prompt(pane, prompt)}
	}
}

func howTitle() string { return i18n.T("Comment ça marche", "How it works") }

// howLayout renders the section, or says how to get it. A shipped ticket
// explained by nobody shows its whole plan instead: what was done.
func (m Model) howLayout(w int) []string {
	h := m.howSubj()
	if h == nil {
		return nil
	}
	if body := h.plan.HowItWorks; body != "" {
		return renderMarkdown(body, w)
	}
	if !h.archivedAt.IsZero() {
		note := sMuted.Render("  " + i18n.T("Livré avant que l'agent l'explique : voici son plan, ce qui a été fait.", "Shipped before the agent explained it: here is its plan, what was done."))
		body := h.plan.Body()
		if k, _, ok := strings.Cut(strings.TrimSpace(body), "\n"); ok && strings.HasPrefix(k, "# ") {
			body = strings.TrimSpace(body)[len(k):] // the title is already in the header
		}
		return append([]string{note, ""}, renderMarkdown(body, w)...)
	}
	lines := []string{"", sMuted.Render("  " + i18n.T("L'agent n'a pas encore expliqué comment ce ticket est construit.", "The agent has not explained yet how this ticket is built."))}
	if h.c.Phase < chantier.Build {
		lines = append(lines, sMuted.Render("  "+i18n.T("Il l'écrira pendant le build, avec le code.", "It will write it during the build, along with the code.")))
	}
	if h.agent() == "" {
		return append(lines, "", sMuted.Render("  "+i18n.T("Aucun agent sur ce chantier pour l'écrire.", "No agent on this ticket to write it.")))
	}
	return append(lines, "", "  "+sKey.Render("g")+sMuted.Render(i18n.T(" demander l'explication à l'agent maintenant", " ask the agent for it now")))
}

// howAskBox is the question input, drawn at the bottom of the view.
func (m Model) howAskBox(w int) []string {
	inner := max(10, w-6)
	text := m.howText + sKey.Render("▏")
	if m.howText == "" {
		text = sKey.Render("▏") + sDim.Render(i18n.T("ex. : où est sauvée la largeur ? quand la synchro tourne-t-elle ?", "e.g. where is the width saved? when does the sync run?"))
	}
	content := []string{sMuted.Render(i18n.T("↳ ta question : un agent à part lit le code et répond dans cette section", "↳ your question: an agent of its own reads the code and answers in this section"))}
	content = append(content, wrap(text, inner, "")...)
	content = append(content, sKey.Render("↵")+sMuted.Render(i18n.T(" envoyer à l'agent   ", " send to the agent   "))+sKey.Render("esc")+sMuted.Render(i18n.T(" annuler", " cancel")))
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 1).Width(inner + 2).
		Render(strings.Join(content, "\n"))
	return strings.Split(box, "\n")
}

func (m Model) howView() string {
	head := sKey.Render(backLabel()) + "   " + sTitle.Render(howTitle())
	if h := m.howSubj(); h != nil {
		head += sMuted.Render(" · " + h.key + " · " + trunc(h.title, max(10, m.width/2)))
		if !h.archivedAt.IsZero() {
			head += sGood.Render(fmt.Sprintf(i18n.T(" · livré, rangé le %s", " · shipped, put away on %s"), h.archivedAt.Format("02/01/2006")))
		}
	}
	bodyH := m.bodyHeight()
	var box []string
	if m.howAsk {
		box = m.howAskBox(m.width)
	}
	h := max(1, bodyH-len(box))
	lines := m.howLayout(m.width)
	start := min(m.hscroll, max(0, len(lines)-h))
	lines = lines[start:]
	if len(lines) > h {
		lines = append([]string(nil), lines[:h]...)
		lines[h-1] = sMuted.Render(fmt.Sprintf(i18n.T("  ↓ %d lignes de plus · espace", "  ↓ %d more lines · space"), len(m.howLayout(m.width))-start-h+1))
	}
	lines = append(lines, make([]string, max(0, h-len(lines)))...)
	lines = append(lines, box...)
	for i, l := range lines {
		lines[i] = xansi.Truncate(l, m.width, "…")
	}
	body := lipgloss.NewStyle().Width(m.width).Height(bodyH).MaxHeight(bodyH).Render(strings.Join(lines, "\n"))
	return xansi.Truncate(head, m.width, "…") + "\n" + m.viewFlash() + "\n" + body + "\n\n" + m.viewFooter()
}

func (m Model) howButtons() []button {
	h := m.howSubj()
	agent := h.agent() != ""
	explain := i18n.T("Demander l'explication", "Ask for it")
	if h != nil && h.plan.HowItWorks != "" {
		explain = i18n.T("Mettre à jour", "Refresh it")
	}
	return []button{
		{"esc", i18n.T("Retour", "Back"), true},
		{"e", i18n.T("Poser une question", "Ask a question"), m.askDir(h) != ""},
		{"g", explain, agent},
		{"d", "Diff", h != nil && h.c != nil && h.c.Worktree != ""},
	}
}

func (m *Model) scrollHow(delta int) {
	n := len(m.howLayout(m.width))
	m.hscroll = max(0, min(m.hscroll+delta, n-m.bodyHeight()))
}

func (m Model) howKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.flash = ""
	if m.howAsk {
		switch k.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEsc:
			m.howAsk, m.howText = false, ""
		case tea.KeyEnter:
			q := strings.TrimSpace(m.howText)
			if q == "" {
				m.say(i18n.T("Écris ta question, ou esc pour annuler.", "Type your question, or esc to cancel."), nil)
				return m, nil
			}
			m.howAsk, m.howText = false, ""
			return m.askAside(q)
		case tea.KeyBackspace:
			if r := []rune(m.howText); len(r) > 0 {
				m.howText = string(r[:len(r)-1])
			}
		case tea.KeySpace:
			m.howText += " "
		case tea.KeyRunes:
			m.howText += string(k.Runes)
		}
		return m, nil
	}
	switch k.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q", "left", "i":
		m.view, m.howArch = "", ""
	case "up", "k":
		m.scrollHow(-1)
	case "down", "j":
		m.scrollHow(1)
	case "pgup", "u":
		m.scrollHow(-m.bodyHeight() / 2)
	case "pgdown", " ":
		m.scrollHow(m.bodyHeight() / 2)
	case "e":
		if m.askDir(m.howSubj()) != "" {
			m.howAsk, m.howText = true, ""
		} else {
			m.say(i18n.T("Pas de code à lire pour répondre : ni worktree, ni dépôt connu pour ce ticket.", "No code to read to answer: no worktree and no known repository for this ticket."), nil)
		}
	case "g":
		return m.requestHow()
	case "d":
		if h := m.howSubj(); h != nil && h.c != nil {
			return m, m.diff()
		}
	}
	return m, nil
}

func (m Model) howMouse(ev tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch ev.Button {
	case tea.MouseButtonWheelUp:
		m.scrollHow(-3)
		return m, nil
	case tea.MouseButtonWheelDown:
		m.scrollHow(3)
		return m, nil
	}
	if ev.Action == tea.MouseActionMotion {
		_, m.hoverBtn = m.hitTest(ev.X, ev.Y)
		return m, nil
	}
	if ev.Button != tea.MouseButtonLeft || ev.Action != tea.MouseActionPress || m.busy != "" {
		return m, nil
	}
	switch {
	case ev.Y == 0 && ev.X < lipgloss.Width(backLabel())+1:
		m.view, m.howAsk, m.howArch = "", false, ""
	case ev.Y == m.footerRow():
		return m.footerClick(ev.X)
	}
	return m, nil
}

// secHow previews the section in the detail pane; the whole of it is one
// click (or i) away.
func (m Model) secHow(b *detailBuilder, c *chantier.Chantier) {
	s, _ := m.planOf(c)
	if s == nil || (s.HowItWorks == "" && c.Phase < chantier.Build) {
		return
	}
	b.section(howTitle(), "", i18n.T("pour répondre aux questions sur le ticket", "to answer questions on the ticket"))
	if s.HowItWorks == "" {
		b.add(hang("  ", sMuted.Render(i18n.T("Pas encore écrit par l'agent.", "Not written by the agent yet.")), b.w)...)
		b.addT("how", "  "+sImg.Render(i18n.T("› Demander l'explication", "› Ask for it"))+sMuted.Render(i18n.T("  (clic ou i)", "  (click or i)")))
		return
	}
	// The overview, then the mechanisms by name.
	var intro, mechs []string
	for _, l := range strings.Split(s.HowItWorks, "\n") {
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(t, "#"):
			if h := strings.TrimSpace(strings.TrimLeft(t, "#")); h != "" {
				mechs = append(mechs, plainMD(h))
			}
		case t != "" && len(mechs) == 0:
			intro = append(intro, plainMD(t))
		}
	}
	if lines := wrap(strings.Join(intro, " "), b.w, "  "); len(intro) > 0 {
		if len(lines) > 3 {
			lines = append(lines[:3:3], "  "+sMuted.Render("…"))
		}
		for _, l := range lines {
			b.addT("how", sSoft.Render(l))
		}
	}
	if len(mechs) > 0 {
		b.addT("how", hang("  "+sMuted.Render("▸ "), strings.Join(mechs, sMuted.Render(" · ")), b.w)...)
	}
	b.addT("how", "  "+sImg.Render(i18n.T("› Tout lire, poser une question", "› Read it all, ask a question"))+sMuted.Render(i18n.T("  (clic ou i)", "  (click or i)")))
}

// askModel answers questions on the implementation: reading code and
// explaining it does not need the strongest model.
const askModel = "sonnet"

// askDir is where a question can be answered: the chantier's worktree, or for
// a shipped ticket the project's main checkout, where its code was merged.
func (m Model) askDir(h *howSubject) string {
	if h == nil {
		return ""
	}
	if h.c != nil && h.c.Worktree != "" {
		return h.c.Worktree
	}
	prefix, _, _ := strings.Cut(strings.ToUpper(h.key), "-")
	for _, p := range m.projects {
		if strings.EqualFold(p.cfg.Key, prefix) && p.cfg.Repo != "" {
			return p.cfg.Repo
		}
	}
	return ""
}

// askAside has a question answered by a short-lived agent of its own: it
// reads the plan and the code, and cairn writes its answer under "### Questions
// posées". The agent at work is neither interrupted nor weighed down.
func (m Model) askAside(question string) (tea.Model, tea.Cmd) {
	h := m.howSubj()
	dir := m.askDir(h)
	if h == nil || h.plan == nil || dir == "" {
		m.say(i18n.T("Pas de code à lire pour répondre.", "No code to read to answer."), nil)
		return m, nil
	}
	key, path := h.key, h.plan.Path
	prompt := chantier.AskPrompt(key, path, question)
	m.say(fmt.Sprintf(i18n.T("Question posée à un agent à part (%s) : la réponse s'ajoute ici dans une à deux minutes.",
		"Question asked to an agent of its own (%s): the answer shows here in a minute or two."), askModel), nil)
	return m, func() tea.Msg {
		_ = journal.Append(config.ChantiersDir(), key, "question posée à part")
		answer, err := ask(dir, prompt)
		if err != nil {
			return doneMsg{err: fmt.Errorf(i18n.T("la question n'a pas eu de réponse : %w", "the question got no answer: %w"), err)}
		}
		s, err := state.Load(path)
		if err != nil || s == nil {
			return doneMsg{err: fmt.Errorf(i18n.T("réponse reçue, mais le plan est illisible : %v", "answer received, but the plan cannot be read: %v"), err)}
		}
		s.AddAsked(question, answer)
		if err := s.Save(); err != nil {
			return doneMsg{err: err}
		}
		return doneMsg{text: fmt.Sprintf(i18n.T("%s : réponse ajoutée sous « Questions posées ».", "%s: answer added under \"Questions posées\"."), key), refresh: true}
	}
}

// ask runs Claude Code once, headless, in dir: it can read and search, not
// write nor run anything, and answers on its standard output.
var ask = func(dir, prompt string) (string, error) {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return "", fmt.Errorf("%s", i18n.T("claude est introuvable dans le PATH", "claude is not in the PATH"))
	}
	prompted.Record(config.Dir(), prompt) // never read as the human's words by the chat hook
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-p", prompt, "--model", askModel,
		"--allowedTools", "Read,Grep,Glob",
		"--disallowedTools", "Bash,Edit,Write,NotebookEdit,WebFetch,WebSearch")
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", trunc(msg, 200))
	}
	answer := strings.TrimSpace(out.String())
	if answer == "" {
		return "", fmt.Errorf("%s", i18n.T("réponse vide", "empty answer"))
	}
	return answer, nil
}
