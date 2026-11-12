package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cairn/internal/chantier"
	"cairn/internal/config"
	"cairn/internal/herdr"
	"cairn/internal/i18n"
	"cairn/internal/journal"
	"cairn/internal/state"
)

// Answering an open question of the plan without leaving cairn, the way an
// agent asks in its chat: a question with options opens as a list to pick
// from, its last line to answer in your own words; a question without
// options opens a box. The answer goes to the chantier's agent.

// question is the open question being answered, with its index.
func (m Model) question(c *chantier.Chantier) (state.Question, int, bool) {
	if c == nil || c.State == nil || m.answerQ == "" {
		return state.Question{}, -1, false
	}
	for i, q := range c.State.OpenQuestions() {
		if q.Text == m.answerQ {
			return q, i, true
		}
	}
	return state.Question{}, -1, false
}

// startAnswer opens open question i of the selection; i < 0 picks the first
// question not answered yet.
func (m Model) startAnswer(i int) (tea.Model, tea.Cmd) {
	c := m.selected()
	if c == nil || c.State == nil {
		return m, nil
	}
	qs := c.State.OpenQuestions()
	if i < 0 {
		for j, q := range qs {
			if _, done := m.answered[answerKey(c.Key, q.Text)]; !done && i < 0 {
				i = j
			}
		}
	}
	if i < 0 || i >= len(qs) {
		m.say(i18n.T("Aucune question ouverte à laquelle répondre.", "No open question to answer."), nil)
		return m, nil
	}
	q := qs[i]
	m.answerQ, m.answerText, m.detailFocus = q.Text, "", true
	m.answerFree, m.answerOpt = len(q.Options) == 0, max(q.Recommended, 0)
	// Bring the question into view.
	d := m.detailLayout(m.detailWidth())
	for row, t := range d.targets {
		if t == fmt.Sprintf("answer:%d", i) {
			m.scroll = max(0, min(row-2, len(d.lines)-m.bodyHeight()))
			break
		}
	}
	return m, nil
}

// clickOption: a first click on an option picks it, a second sends it; the
// last line ("another answer") opens the box.
func (m Model) clickOption(i, j int) (tea.Model, tea.Cmd) {
	c := m.selected()
	if c == nil || c.State == nil {
		return m, nil
	}
	qs := c.State.OpenQuestions()
	if i < 0 || i >= len(qs) || j < 0 || j > len(qs[i].Options) {
		return m, nil
	}
	if m.answerQ != qs[i].Text {
		mm, cmd := m.startAnswer(i)
		m = mm.(Model)
		m.answerOpt, m.answerFree = j, j == len(qs[i].Options)
		return m, cmd
	}
	if j == len(qs[i].Options) {
		m.answerOpt, m.answerFree = j, true
		return m, nil
	}
	if m.answerOpt == j && !m.answerFree {
		return m.sendAnswer()
	}
	m.answerOpt, m.answerFree = j, false
	return m, nil
}

func (m Model) answerKeyPress(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	q, _, ok := m.question(m.selected())
	if k.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}
	if !ok {
		m.answerQ, m.answerText = "", ""
		return m, nil
	}
	n := len(q.Options)
	if !m.answerFree {
		switch k.String() {
		case "esc":
			m.answerQ = ""
		case "up", "k", "shift+tab":
			m.answerOpt = (m.answerOpt + n) % (n + 1)
		case "down", "j", "tab":
			m.answerOpt = (m.answerOpt + 1) % (n + 1)
		case "enter":
			if m.answerOpt >= n {
				m.answerFree = true
				return m, nil
			}
			return m.sendAnswer()
		default:
			// the key of an option moves onto it: a, b, 1, 2…
			for j, o := range q.Options {
				if strings.EqualFold(k.String(), o.Key) {
					m.answerOpt = j
				}
			}
		}
		return m, nil
	}
	switch k.Type {
	case tea.KeyEsc:
		if n > 0 {
			m.answerFree, m.answerText = false, "" // back to the options
		} else {
			m.answerQ, m.answerText = "", ""
		}
	case tea.KeyEnter:
		return m.sendAnswer()
	case tea.KeyBackspace:
		if r := []rune(m.answerText); len(r) > 0 {
			m.answerText = string(r[:len(r)-1])
		}
	case tea.KeySpace:
		m.answerText += " "
	case tea.KeyRunes:
		m.answerText += string(k.Runes)
	}
	return m, nil
}

// answerBox is the input drawn under the question being answered.
func (m Model) answerBox(w int, back bool) []string {
	inner := max(10, w-10)
	text := m.answerText + sKey.Render("▏")
	if m.answerText == "" {
		text = sKey.Render("▏") + sDim.Render(i18n.T("ta réponse, ou une question pour l'agent…", "your answer, or a question for the agent…"))
	}
	content := []string{sMuted.Render(i18n.T("↳ ta réponse, envoyée telle quelle à l'agent", "↳ your answer, sent as is to the agent"))}
	content = append(content, wrap(text, inner, "")...)
	content = append(content, sKey.Render("↵")+sMuted.Render(i18n.T(" envoyer à l'agent   ", " send to the agent   "))+sKey.Render("esc")+sMuted.Render(escLabel(back)))
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 1).Width(inner + 2).
		Render(strings.Join(content, "\n"))
	out := strings.Split(box, "\n")
	for i := range out {
		out[i] = "    " + out[i]
	}
	return out
}

func (m Model) sendAnswer() (tea.Model, tea.Cmd) {
	c := m.selected()
	q, _, ok := m.question(c)
	if !ok {
		return m, nil
	}
	choice := !m.answerFree && m.answerOpt < len(q.Options)
	text := strings.TrimSpace(m.answerText)
	if choice {
		text = q.Answer(m.answerOpt)
	}
	if text == "" {
		m.say(i18n.T("Écris ta réponse, ou esc pour revenir.", "Type your answer, or esc to go back."), nil)
		return m, nil
	}
	pane := agentPane(c)
	if pane == "" {
		m.say(fmt.Sprintf(i18n.T("Aucun agent sur %s pour recevoir la réponse.", "No agent on %s to receive the answer."), c.Key), nil)
		return m, nil
	}
	answer := text
	if choice {
		answer = fmt.Sprintf(i18n.T("Option retenue : %s", "Option chosen: %s"), text)
	}
	prompt := fmt.Sprintf(i18n.T("Réponse à ta question du plan de %s :\n« %s »\n\n%s\n\n"+
		"Mets à jour le plan (%s) : coche la question ([x]) en notant la réponse après « → », et ajuste les tâches et les effets de bord. "+
		"Si le plan attendait une validation, garde l'attente et arrête-toi.",
		"Answer to your plan question on %s:\n\"%s\"\n\n%s\n\n"+
			"Update the plan (%s): check the question ([x]) with the answer after \"→\", and adjust the tasks and side effects. "+
			"If the plan was waiting for approval, keep it waiting and stop."), c.Key, plainMD(q.Text), answer, planPath(c))
	m.answered[answerKey(c.Key, q.Text)] = text
	m.answerQ, m.answerText, m.answerFree = "", "", false
	key := c.Key
	return m, func() tea.Msg {
		_ = journal.Append(config.ChantiersDir(), key, "réponse envoyée à l'agent")
		return doneMsg{text: fmt.Sprintf(i18n.T("Réponse envoyée à l'agent de %s.", "Answer sent to %s's agent."), key), err: herdr.Prompt(pane, prompt)}
	}
}

func escLabel(back bool) string {
	if back {
		return i18n.T(" revenir aux choix", " back to the choices")
	}
	return i18n.T(" annuler", " cancel")
}
