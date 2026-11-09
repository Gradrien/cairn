package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"

	"cairn/internal/chantier"
	"cairn/internal/config"
	"cairn/internal/herdr"
	"cairn/internal/i18n"
	"cairn/internal/journal"
)

// The "how it works" view: the chantier file's "## Comment ça marche"
// section, full screen, so you can say what was built and how without
// reading the code. A question typed here goes to the agent, which answers
// in that section: the answer stays with the ticket.

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

// HowPrompt asks the agent to write or refresh the section; with a question,
// to answer it there.
func howPrompt(key, path, question string) string {
	head := fmt.Sprintf(i18n.T("Explique comment %s est implémenté : écris ou mets à jour la section `## Comment ça marche` de %s (avant `## Preuves`).",
		"Explain how %s is implemented: write or update the `## Comment ça marche` section of %s (before `## Preuves`)."), key, path)
	if question != "" {
		head = fmt.Sprintf(i18n.T("Question de l'humain sur l'implémentation de %s :\n« %s »\n\n"+
			"Vérifie dans le code, puis réponds-y dans la section `## Comment ça marche` de %s : complète le mécanisme concerné "+
			"et note la question avec sa réponse courte sous `### Questions posées`. Réponds aussi dans ton message.",
			"The human's question on how %s is implemented:\n\"%s\"\n\n"+
				"Check it in the code, then answer it in the `## Comment ça marche` section of %s: complete the mechanism it is about "+
				"and add the question with its short answer under `### Questions posées`. Answer in your message too."), key, question, path)
	}
	return head + "\n\n" + howFormat() + "\n\n" + i18n.T(
		"Ne modifie pas le code, ni la phase, ni l'attente du plan. Lance `cairn sync`, puis reprends où tu en étais, ou arrête-toi si tu attendais.",
		"Do not change the code, the phase or the plan's waiting line. Run `cairn sync`, then resume where you were, or stop if you were waiting.")
}

func (m Model) openHow() (tea.Model, tea.Cmd) {
	c := m.selected()
	if c == nil || c.State == nil {
		m.say(i18n.T("Pas de plan cairn pour ce chantier.", "No cairn plan for this ticket."), nil)
		return m, nil
	}
	m.view, m.hscroll, m.howAsk, m.howText = "how", 0, false, ""
	return m, nil
}

// requestHow sends the agent a request to explain, or to answer question.
func (m Model) requestHow(question string) (tea.Model, tea.Cmd) {
	c := m.selected()
	if c == nil {
		return m, nil
	}
	pane := agentPane(c)
	if pane == "" {
		m.say(fmt.Sprintf(i18n.T("Aucun agent sur %s pour expliquer : relance-le depuis son espace herdr.", "No agent on %s to explain: restart it from its herdr workspace."), c.Key), nil)
		return m, nil
	}
	prompt, key := howPrompt(c.Key, planPath(c), question), c.Key
	text, event := i18n.T("Explication demandée à l'agent de %s.", "Explanation asked from %s's agent."), "explication demandée"
	if question != "" {
		text, event = i18n.T("Question envoyée à l'agent de %s : la réponse arrivera ici.", "Question sent to %s's agent: the answer will show here."), "question sur l'implémentation"
	}
	return m, func() tea.Msg {
		_ = journal.Append(config.ChantiersDir(), key, event)
		return doneMsg{text: fmt.Sprintf(text, key), err: herdr.Prompt(pane, prompt)}
	}
}

func howTitle() string { return i18n.T("Comment ça marche", "How it works") }

// howLayout renders the section, or says how to get it.
func (m Model) howLayout(w int) []string {
	c := m.selected()
	if c == nil || c.State == nil {
		return nil
	}
	if body := c.State.HowItWorks; body != "" {
		return renderMarkdown(body, w)
	}
	lines := []string{"", sMuted.Render("  " + i18n.T("L'agent n'a pas encore expliqué comment ce ticket est construit.", "The agent has not explained yet how this ticket is built."))}
	if c.Phase < chantier.Build {
		lines = append(lines, sMuted.Render("  "+i18n.T("Il l'écrira pendant le build, avec le code.", "It will write it during the build, along with the code.")))
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
	content := []string{sMuted.Render(i18n.T("↳ ta question, l'agent répond dans cette section", "↳ your question, the agent answers in this section"))}
	content = append(content, wrap(text, inner, "")...)
	content = append(content, sKey.Render("↵")+sMuted.Render(i18n.T(" envoyer à l'agent   ", " send to the agent   "))+sKey.Render("esc")+sMuted.Render(i18n.T(" annuler", " cancel")))
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 1).Width(inner + 2).
		Render(strings.Join(content, "\n"))
	return strings.Split(box, "\n")
}

func (m Model) howView() string {
	head := sKey.Render(backLabel()) + "   " + sTitle.Render(howTitle())
	if c := m.selected(); c != nil {
		head += sMuted.Render(" · " + c.Key + " · " + trunc(c.Title(), max(10, m.width/2)))
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
	return xansi.Truncate(head, m.width, "…") + "\n" + m.viewFlash() + "\n" + body + "\n" + m.viewFooter()
}

func (m Model) howButtons() []button {
	c := m.selected()
	agent := c != nil && agentPane(c) != ""
	explain := i18n.T("Demander l'explication", "Ask for it")
	if c != nil && c.State != nil && c.State.HowItWorks != "" {
		explain = i18n.T("Mettre à jour", "Refresh it")
	}
	return []button{
		{"esc", i18n.T("Retour", "Back"), true},
		{"e", i18n.T("Poser une question", "Ask a question"), agent},
		{"g", explain, agent},
		{"d", "Diff", c != nil && c.Worktree != ""},
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
			return m.requestHow(q)
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
		m.view = ""
	case "up", "k":
		m.scrollHow(-1)
	case "down", "j":
		m.scrollHow(1)
	case "pgup", "u":
		m.scrollHow(-m.bodyHeight() / 2)
	case "pgdown", " ":
		m.scrollHow(m.bodyHeight() / 2)
	case "e":
		if c := m.selected(); c != nil && agentPane(c) != "" {
			m.howAsk, m.howText = true, ""
		} else {
			return m.requestHow("?") // says there is no agent
		}
	case "g":
		return m.requestHow("")
	case "d":
		return m, m.diff()
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
		m.view, m.howAsk = "", false
	case ev.Y == 2+m.bodyHeight():
		return m.footerClick(ev.X)
	}
	return m, nil
}

// secHow previews the section in the detail pane; the whole of it is one
// click (or i) away.
func (m Model) secHow(b *detailBuilder, c *chantier.Chantier) {
	s := c.State
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
