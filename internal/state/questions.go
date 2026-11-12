package state

import (
	"fmt"
	"regexp"
	"strings"
)

// A question of "## Questions" may offer choices, the way an agent asks in
// its own chat: options under it, then its recommendation.
//
//	- [ ] **Spé orpheline** : que fait-on pour un deal sans métier ?
//	  - (a) Garder le ciblage actuel : le point affiche 0.
//	  - (b) Élargir aux expertises de la spé : change le premier envoi.
//	  - Recommandation : (a), parce que…
//
// Options are sub-items marked "(a)", "a)", "1." or "Option A :". The
// recommendation is the sub-item or paragraph starting with "Recommandation"
// or "Je recommande". Anything else under the question is its context.

type Option struct {
	Key  string // "a", "b", "1"…
	Text string // without its key
}

type Question struct {
	Text           string
	Done           bool // "- [x]": settled
	Options        []Option
	Recommended    int    // index in Options, -1 if none
	Recommendation string // the recommendation as written, "" if none
	Body           []Item // context under the question
	Line           int    // body line of the question
	stray          int    // sub-items that are neither an option nor the recommendation
	recoKey        string // the option the recommendation names, even if missing
	single         bool   // one lone option-looking sub-item
}

var (
	optKeyed = regexp.MustCompile(`^(?:\*\*|_)?\(?([a-zA-Z]|\d{1,2})\)(?:\*\*|_)?\s+(.+)$`)
	optWord  = regexp.MustCompile(`(?i)^(?:\*\*|_)?(?:option|choix|choice)\s+([a-zA-Z0-9]{1,2})\b(?:\*\*|_)?\s*[:.)—–-]?\s*(?:\*\*|_)?\s*(.*)$`)
	recoRe   = regexp.MustCompile(`(?i)^(?:\*\*|_)?(?:recommandation|recommand[ée]e?|je recommande|ma recommandation|reco|recommendation|recommended|i recommend|i'd recommend|i would recommend)(?:[^\p{L}\p{N}]|$)`)
	recoTag  = regexp.MustCompile(`(?i)[(\[]\s*(?:recommand[ée]e?|recommended)\s*[)\]]|★`)
	optLabel = regexp.MustCompile(`(?i)^(?:\*\*|_)?(?:options?|choix|alternatives?|choices?)(?:\*\*|_)?\s*:?\s*(?:\*\*|_)?\s*:?$`)
	keyParen = regexp.MustCompile(`\(([a-zA-Z]|\d{1,2})\)`)
	keyWord  = regexp.MustCompile(`(?i)(?:option|choix|choice)\s+([a-zA-Z0-9]{1,2})\b`)
	keyBare  = regexp.MustCompile(`(?i)^(?:\*\*|_)?(?:recommandation|recommand[ée]e?|je recommande|ma recommandation|reco|recommendation|recommended|i recommend)(?:\*\*|_)?\s*(?:[:—–-]\s*)?(?:\*\*|_)?(?:la |le |l'|the )?([a-zA-Z]|\d{1,2})(?:\*\*|_)?(?:$|[\s,.;:)])`)
)

// option reads a sub-item as an option, ok false if it is not one.
func option(n *node) (Option, bool) {
	if !n.item() {
		return Option{}, false
	}
	if n.ordered() {
		return Option{Key: strings.ToLower(strings.Trim(n.Marker, "().")), Text: n.Text}, true
	}
	if m := optKeyed.FindStringSubmatch(n.Text); m != nil {
		return Option{Key: strings.ToLower(m[1]), Text: strings.TrimSpace(m[2])}, true
	}
	if m := optWord.FindStringSubmatch(n.Text); m != nil && strings.TrimSpace(m[2]) != "" {
		return Option{Key: strings.ToLower(m[1]), Text: strings.TrimSpace(m[2])}, true
	}
	return Option{}, false
}

// question reads an item of "## Questions" with what is under it; extra are
// the paragraphs written after it at the section's margin.
func question(n *node, extra []*node) Question {
	q := Question{Text: n.Text, Done: n.done(), Recommended: -1, Line: n.Line}
	kids := append(append([]*node{}, n.Kids...), extra...)
	// The options are the question's own sub-items, or those of one of
	// them ("- **Options** :" with the choices under it).
	opts, optNodes := options(kids)
	var holder *node
	if len(opts) < 2 {
		for _, k := range kids {
			if o, on := options(k.Kids); len(o) >= 2 {
				opts, optNodes, holder = o, on, k
				break
			}
		}
	}
	isOpt := map[*node]int{}
	switch {
	case len(opts) >= 2:
		q.Options = opts
		for j, k := range optNodes {
			isOpt[k] = j
		}
	case len(opts) == 1:
		q.single = true
	}
	var walk func(ns []*node, depth int)
	walk = func(ns []*node, depth int) {
		for _, k := range ns {
			if j, ok := isOpt[k]; ok {
				if recoTag.MatchString(k.Text) {
					if q.Recommended < 0 {
						q.Recommended = j
					}
					q.Options[j].Text = strings.Join(strings.Fields(recoTag.ReplaceAllString(q.Options[j].Text, "")), " ")
				}
				// what sits under an option is its description
				for _, it := range body(k.Kids, 0) {
					q.Options[j].Text += " " + it.Text
				}
				continue
			}
			if !k.Code && depth <= 1 && q.Recommendation == "" && recoRe.MatchString(k.Text) {
				q.Recommendation = strings.TrimSpace(strings.Join(append([]string{k.Text}, itemTexts(body(k.Kids, 0))...), " "))
				continue
			}
			if k == holder && optLabel.MatchString(k.Text) {
				walk(k.Kids, depth) // "Options :" says nothing the list does not
				continue
			}
			if depth == 0 && k.item() && holder == nil {
				q.stray++
			}
			q.Body = append(q.Body, Item{Text: k.label(), Depth: min(depth, 2), Plain: k.plain(), Code: k.Code})
			walk(k.Kids, depth+1)
		}
	}
	walk(kids, 0)
	if q.Recommendation != "" {
		q.recoKey = recommendedKey(q.Recommendation)
		if q.Recommended < 0 {
			for i, o := range q.Options {
				if o.Key == q.recoKey {
					q.Recommended = i
				}
			}
		}
	}
	return q
}

// options reads the options among sibling items.
func options(ns []*node) ([]Option, []*node) {
	var opts []Option
	var at []*node
	for _, k := range ns {
		if o, ok := option(k); ok {
			opts, at = append(opts, o), append(at, k)
		}
	}
	return opts, at
}

func itemTexts(its []Item) []string {
	out := make([]string, len(its))
	for i, it := range its {
		out[i] = it.Text
	}
	return out
}

// recommendedKey finds the option a recommendation names: "(a)", "option
// B", "Recommandation : a".
func recommendedKey(text string) string {
	for _, re := range []*regexp.Regexp{keyParen, keyWord, keyBare} {
		if m := re.FindStringSubmatch(text); m != nil {
			return strings.ToLower(m[1])
		}
	}
	return ""
}

// Answer is what is sent to the agent when option i is chosen.
func (q Question) Answer(i int) string {
	if i < 0 || i >= len(q.Options) {
		return ""
	}
	o := q.Options[i]
	return fmt.Sprintf("(%s) %s", o.Key, o.Text)
}
