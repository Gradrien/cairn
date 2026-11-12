package state

import (
	"fmt"
	"strings"

	"cairn/internal/i18n"
)

// Problem is something in the plan that cairn cannot show as the agent
// meant it. `cairn sync` reports them so the agent fixes the file, the way a
// tool call that does not match its schema is sent back to the model.
type Problem struct {
	Line int // in the file, 1-based
	Msg  string
}

func (p Problem) String() string {
	return fmt.Sprintf(i18n.T("ligne %d : %s", "line %d: %s"), p.Line, p.Msg)
}

// Lint lists the problems of the plan, in file order.
func (s *State) Lint() []Problem {
	var out []Problem
	at := func(bodyLine int) int { return s.bodyAt + bodyLine + 1 }
	switch s.Phase {
	case "", "plan", "build", "review", "livré", "livre", "shipped", "done":
	default:
		out = append(out, Problem{1, fmt.Sprintf(i18n.T("phase « %s » inconnue : plan, build ou review", "unknown phase \"%s\": plan, build or review"), s.Phase)})
	}
	for _, q := range s.Questions {
		if q.Done {
			continue
		}
		name := short(q.Text)
		switch {
		case q.single:
			out = append(out, Problem{at(q.Line), fmt.Sprintf(i18n.T(
				"question « %s » : une seule option ; propose au moins deux choix (a), (b)… ou écris-la comme une simple sous-puce",
				"question \"%s\": a single option; offer at least two choices (a), (b)… or write it as a plain sub-item"), name)})
		case len(q.Options) == 0 && q.stray >= 2:
			out = append(out, Problem{at(q.Line), fmt.Sprintf(i18n.T(
				"question « %s » : %d sous-puces sans clé ; si ce sont des choix, écris-les « (a) … », « (b) … » pour que l'humain puisse en sélectionner un, sinon mets ce contexte dans un paragraphe sous la question",
				"question \"%s\": %d sub-items without a key; if they are choices, write them \"(a) …\", \"(b) …\" so the human can pick one, otherwise put this context in a paragraph under the question"), name, q.stray)})
		case len(q.Options) > 0 && q.stray > 0:
			out = append(out, Problem{at(q.Line), fmt.Sprintf(i18n.T(
				"question « %s » : des sous-puces sans clé au milieu des options ; mets le contexte dans la question ou dans un paragraphe, et seulement les choix en (a), (b)…",
				"question \"%s\": sub-items without a key among the options; put the context in the question or a paragraph, and only the choices as (a), (b)…"), name)})
		}
		if len(q.Options) == 0 {
			continue
		}
		switch {
		case q.Recommendation == "" && q.Recommended < 0:
			out = append(out, Problem{at(q.Line), fmt.Sprintf(i18n.T(
				"question « %s » : pas de recommandation ; ajoute une sous-puce « Recommandation : (x), parce que… »",
				"question \"%s\": no recommendation; add a sub-item \"Recommandation : (x), because…\""), name)})
		case q.Recommended < 0 && q.recoKey != "":
			out = append(out, Problem{at(q.Line), fmt.Sprintf(i18n.T(
				"question « %s » : la recommandation nomme (%s), qui n'est pas une des options",
				"question \"%s\": the recommendation names (%s), which is not one of the options"), name, q.recoKey)})
		case q.Recommended < 0:
			out = append(out, Problem{at(q.Line), fmt.Sprintf(i18n.T(
				"question « %s » : la recommandation ne dit pas quelle option ; écris « Recommandation : (a), parce que… »",
				"question \"%s\": the recommendation does not say which option; write \"Recommandation : (a), because…\""), name)})
		}
		seen := map[string]bool{}
		for _, o := range q.Options {
			if seen[o.Key] {
				out = append(out, Problem{at(q.Line), fmt.Sprintf(i18n.T(
					"question « %s » : deux options ont la clé (%s)", "question \"%s\": two options share the key (%s)"), name, o.Key)})
			}
			seen[o.Key] = true
		}
	}
	var doing []Task
	for _, t := range s.Tasks {
		if t.Doing {
			doing = append(doing, t)
		}
	}
	if len(doing) > 1 {
		out = append(out, Problem{at(doing[1].Line), fmt.Sprintf(i18n.T(
			"%d tâches en cours ([~]) : une seule à la fois, celle sur laquelle tu travailles",
			"%d tasks in progress ([~]): one at a time, the one you are working on"), len(doing))})
	}
	return out
}

func short(s string) string {
	s = strings.NewReplacer("**", "", "`", "").Replace(s)
	if r := []rune(s); len(r) > 50 {
		return string(r[:49]) + "…"
	}
	return s
}
