package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cairn/internal/config"
	"cairn/internal/herdr"
	"cairn/internal/i18n"
	"cairn/internal/journal"
)

// Answering an open question of the plan without leaving cairn: a box opens
// under the question, and the answer goes to the chantier's agent as is.

// startAnswer opens the box under open question i of the selection; i < 0
// picks the first question not answered yet.
func (m Model) startAnswer(i int) (tea.Model, tea.Cmd) {
	c := m.selected()
	if c == nil || c.State == nil {
		return m, nil
	}
	qs := c.State.OpenQuestions()
	if i < 0 {
		for j, q := range qs {
			if _, done := m.answered[answerKey(c.Key, q)]; !done && i < 0 {
				i = j
			}
		}
	}
	if i < 0 || i >= len(qs) {
		m.say(i18n.T("Aucune question ouverte à laquelle répondre.", "No open question to answer."), nil)
		return m, nil
	}
	m.answerQ, m.answerText, m.detailFocus = qs[i], "", true
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

func (m Model) answerKeyPress(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.answerQ, m.answerText = "", ""
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
func (m Model) answerBox(w int) []string {
	inner := max(10, w-10)
	text := m.answerText + sKey.Render("▏")
	if m.answerText == "" {
		text = sKey.Render("▏") + sDim.Render(i18n.T("ta réponse…", "your answer…"))
	}
	content := []string{sMuted.Render(i18n.T("↳ ta réponse, envoyée telle quelle à l'agent", "↳ your answer, sent as is to the agent"))}
	content = append(content, wrap(text, inner, "")...)
	content = append(content, sKey.Render("↵")+sMuted.Render(i18n.T(" envoyer à l'agent   ", " send to the agent   "))+sKey.Render("esc")+sMuted.Render(i18n.T(" annuler", " cancel")))
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
	text := strings.TrimSpace(m.answerText)
	if c == nil || m.answerQ == "" {
		return m, nil
	}
	if text == "" {
		m.say(i18n.T("Écris ta réponse, ou esc pour annuler.", "Type your answer, or esc to cancel."), nil)
		return m, nil
	}
	pane := agentPane(c)
	if pane == "" {
		m.say(fmt.Sprintf(i18n.T("Aucun agent sur %s pour recevoir la réponse.", "No agent on %s to receive the answer."), c.Key), nil)
		return m, nil
	}
	q := m.answerQ
	prompt := fmt.Sprintf(i18n.T("Réponse à ta question du plan de %s :\n« %s »\n\n%s\n\n"+
		"Mets à jour le plan (%s) : coche la question ([x]) en notant la réponse, et ajuste les tâches et les effets de bord. "+
		"Si le plan attendait une validation, garde l'attente et arrête-toi.",
		"Answer to your plan question on %s:\n\"%s\"\n\n%s\n\n"+
			"Update the plan (%s): check the question ([x]) with the answer, and adjust the tasks and side effects. "+
			"If the plan was waiting for approval, keep it waiting and stop."), c.Key, plainMD(q), text, planPath(c))
	m.answered[answerKey(c.Key, q)] = text
	m.answerQ, m.answerText = "", ""
	key := c.Key
	return m, func() tea.Msg {
		_ = journal.Append(config.ChantiersDir(), key, "réponse envoyée à l'agent")
		return doneMsg{text: fmt.Sprintf(i18n.T("Réponse envoyée à l'agent de %s.", "Answer sent to %s's agent."), key), err: herdr.Prompt(pane, prompt)}
	}
}
