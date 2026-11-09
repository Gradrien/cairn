package guard

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"cairn/internal/i18n"
	"cairn/internal/journal"
	"cairn/internal/prompted"
)

// The chat side of the guard, a Claude Code UserPromptSubmit hook: when the
// human asks the agent to push ("push", "pousse ça"), the push is allowed as
// if pressed in cairn. Only the human's words count: cairn's own prompts
// are remembered (package prompted) and skipped. Opening the PR stays a
// cairn button.

var (
	pushWord = regexp.MustCompile(`(?i)(^|[^\p{L}])(push(e|er|es|ez)?|pouss(e|er|es|ez))([^\p{L}]|$)`)
	// Words that, just before the request, make it something else: a
	// negation, a "before pushing", a question about the past.
	notBefore = regexp.MustCompile(`(?i)(^|[^\p{L}'])(ne|n'|pas|sans|jamais|avant|attends|déjà|as-tu|t'as|été|don't|dont|not|never|no|without|before|wait|already|did|have|has)([^\p{L}]|$)`)
	notAfter  = regexp.MustCompile(`(?i)^\W*(pas|rien|jamais|not|nothing|never)([^\p{L}]|$)`)
	sentences = regexp.MustCompile(`[^.!?;\n]+[.!?;\n]*`)
	// A question about pushing ("why are pushes blocked?") asks nothing.
	whyQuestion = regexp.MustCompile(`(?i)(^|[^\p{L}])(pourquoi|comment|quand|why|how|when|where)([^\p{L}]|$)`)
)

// asksPush reports whether a message from the human asks for a push.
func asksPush(text string) bool {
	for _, sentence := range sentences.FindAllString(text, -1) {
		if strings.Contains(sentence, "?") && whyQuestion.MatchString(sentence) {
			continue
		}
		for _, loc := range pushWord.FindAllStringIndex(sentence, -1) {
			before := strings.Fields(sentence[:loc[0]])
			if len(before) > 4 {
				before = before[len(before)-4:]
			}
			if notBefore.MatchString(strings.Join(before, " ")) || notAfter.MatchString(sentence[loc[1]:]) {
				continue
			}
			return true
		}
	}
	return false
}

type promptInput struct {
	Prompt string `json:"prompt"`
	Cwd    string `json:"cwd"`
}

// CheckPrompt reads a UserPromptSubmit payload; when the human asks for a
// push on a chantier, it allows it and returns what the agent should know.
func CheckPrompt(r io.Reader, keys []string, chantiersDir, cairnDir string) string {
	var in promptInput
	if err := json.NewDecoder(r).Decode(&in); err != nil || in.Cwd == "" || !asksPush(in.Prompt) || prompted.Sent(cairnDir, in.Prompt) {
		return ""
	}
	key, s, head := chantierAt(in.Cwd, keys, chantiersDir)
	if s == nil || (s.PushCovers(head) && s.PushCommit() != "") {
		return ""
	}
	// No commit yet: the human may ask to commit then push. The first push pins it.
	s.Set("push", "autorisé")
	if w := strings.ToLower(s.Waiting); strings.Contains(w, "pousser") || strings.Contains(w, "push") {
		s.Set("attente", "")
	}
	if err := s.Save(); err != nil {
		return ""
	}
	_ = journal.Append(chantiersDir, key, "push autorisé (demandé dans le chat)")
	pr := i18n.T(" L'ouverture de la PR reste à autoriser dans cairn : ne l'ouvre pas.", " Opening the PR is still allowed in cairn only: do not open it.")
	if s.PRAllowed() {
		pr = ""
	}
	return fmt.Sprintf(i18n.T("cairn : l'humain demande le push de %s, il est autorisé (le premier push fixe le commit, les nouvelles tentatives passent).%s",
		"cairn: the human asks to push %s, it is allowed (the first push pins the commit, retries pass).%s"), key, pr)
}
