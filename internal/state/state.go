// Package state reads and writes chantier files: ~/.cairn/chantiers/<KEY>.md.
//
//	---
//	ticket: YAK-788
//	phase: plan | build | review
//	attente: Valider le plan        (optional: a decision waiting for a human)
//	push: autorisé [<commit>]       (set by cairn or by the human's request in the chat:
//	                                 pushes pass while HEAD is <commit>; without one, the
//	                                 next push pins it)
//	pr: autorisée                   (set by cairn: the agent may open the pull request)
//	---
//	a short summary (objective, decisions) before the first "##" heading;
//	tasks "- [ ]" todo, "- [~]" in progress, "- [x]" done;
//	"## Effets de bord": tasks that check what the change may break elsewhere;
//	"## Approche": what the plan settled before building (plain items);
//	"## Questions": open questions ("- [x]" once answered);
//	"## Preuves": every list item is a proof;
//	"## Comment ça marche": how the change works, kept as markdown for the
//	human to read (where things live, when they run, what is stored);
//	"## Tester à la main": how to try the change by hand, what to set up
//	first (plain lines), then steps "- [ ] do this → check that", which the
//	human ticks in cairn as they go.
package state

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Task struct {
	Text  string
	Done  bool
	Doing bool   // "- [~]": started, not finished
	Body  []Item // what is written under it: sub-items, paragraphs, code
	Line  int    // body line of its box
}

// Item is a list item of a prose section, with its nesting level (0 for a
// top-level item, 1 under it…).
type Item struct {
	Text  string
	Depth int
	Plain bool // a paragraph, not a list item: shown without a bullet
	Code  bool // a line of a code block
}

type State struct {
	Path        string
	Ticket      string
	Phase       string
	Waiting     string
	Push        string // "autorisé <commit>" once a human allowed pushing that commit
	PR          string // "autorisée" once a human allowed opening the pull request
	Title       string // the first "# " heading, without its "KEY · " prefix
	Tasks       []Task
	SideEffects []Task
	Questions   []Question
	Proofs      []string
	Summary     []string
	Approach    []Item   // "## Approche": what the plan settled (problem, done when, choice…)
	HowItWorks  string   // the "## Comment ça marche" section, raw markdown
	Reprise     string   // the "## Reprise" section: an agent's handoff to the next one
	ManualSetup []string // "## Tester à la main": what to set up before the steps
	ManualSteps []Task   // "## Tester à la main": the steps, ticked by the human
	manualAt    []int    // body line of each manual step, to tick it
	sideAt      []int    // body line of each side effect, to tick it
	UpdatedAt   time.Time
	front       []string // raw frontmatter lines, kept for round-trips
	bodyAt      int      // file line (0-based) where the body starts
	body        string
}

var (
	taskRe  = regexp.MustCompile(`^\s*(?:[-*+]|\d{1,3}[.)]|[a-zA-Z][.)]|\([a-zA-Z0-9]{1,2}\))\s+\[([ xX~-])\]\s+(.*)$`)
	proofHd = regexp.MustCompile(`(?i)^#{1,6}\s+(preuves?|proofs?)\b`)
	sideHd  = regexp.MustCompile(`(?i)^#{1,6}\s+(effets? de bord|side[ -]effects?)`)
	apprHd  = regexp.MustCompile(`(?i)^#{1,6}\s+(approche|approach)\b`)
	questHd = regexp.MustCompile(`(?i)^#{1,6}\s+questions?`)
	testHd  = regexp.MustCompile(`(?i)^#{1,6}\s+(tester [àa] la main|tests? manuels?|manual tests?|test by hand)\b`)
	howHd   = regexp.MustCompile(`(?i)^##\s+(comment [çc]a marche|how it works)\b`)
	repHd   = regexp.MustCompile(`(?i)^##\s+(reprise|handoff)\b`)
	askedHd = regexp.MustCompile(`(?i)^###\s+(questions pos[ée]es|questions asked)\b`)
	headRe  = regexp.MustCompile(`^#{1,6}\s`)
	h2Re    = regexp.MustCompile(`^#{2,6}\s`)
	h12Re   = regexp.MustCompile(`^#{1,2}\s`)
)

func PathFor(dir, key string) string { return filepath.Join(dir, strings.ToUpper(key)+".md") }

// Load returns nil, nil when the chantier has no file yet.
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s := Parse(path, string(data))
	if fi, err := os.Stat(path); err == nil {
		s.UpdatedAt = fi.ModTime()
	}
	return s, nil
}

func Parse(path, text string) *State {
	s := &State{Path: path, body: text}
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				s.front = lines[1:i]
				s.body = strings.Join(lines[i+1:], "\n")
				s.bodyAt = i + 1
				break
			}
		}
	}
	for _, l := range s.front {
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "ticket":
			s.Ticket = v
		case "phase":
			s.Phase = strings.ToLower(v)
		case "attente", "waiting":
			s.Waiting = v
		case "push":
			s.Push = v
		case "pr":
			s.PR = v
		}
	}
	s.parseBody()
	return s
}

// parseBody cuts the body into sections at their headings, then reads each
// section as an outline (see outline.go).
func (s *State) parseBody() {
	lines := strings.Split(s.body, "\n")
	type sec struct {
		kind     string // "", "proofs", "side", "questions", "approach", "how", "reprise", "manual", "other"
		from, to int
	}
	var secs []sec
	kind, start, fence := "", 0, ""
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if fence != "" {
			if strings.HasPrefix(t, fence) {
				fence = ""
			}
			continue
		}
		if m := outFence.FindStringSubmatch(l); m != nil {
			fence = m[1]
			continue
		}
		// "Comment ça marche" and "Reprise" are prose with their own "###"
		// headings: each runs to the next "#" or "##" heading, kept whole.
		if !headRe.MatchString(l) || (kind == "how" || kind == "reprise") && !h12Re.MatchString(l) {
			continue
		}
		if t, ok := strings.CutPrefix(l, "# "); ok && s.Title == "" {
			s.Title = strings.TrimSpace(t)
			if k, rest, ok := strings.Cut(s.Title, " · "); ok && strings.EqualFold(k, s.Ticket) {
				s.Title = strings.TrimSpace(rest)
			}
		}
		secs = append(secs, sec{kind, start, i})
		start = i + 1
		switch {
		case howHd.MatchString(l):
			kind = "how"
		case repHd.MatchString(l):
			kind = "reprise"
		case proofHd.MatchString(l):
			kind = "proofs"
		case sideHd.MatchString(l):
			kind = "side"
		case questHd.MatchString(l):
			kind = "questions"
		case apprHd.MatchString(l):
			kind = "approach"
		case testHd.MatchString(l):
			kind = "manual"
		case h2Re.MatchString(l), kind == "how", kind == "reprise":
			kind = "other"
		}
	}
	secs = append(secs, sec{kind, start, len(lines)})

	var how, rep []string
	task := func(n *node) Task {
		return Task{Text: n.Text, Done: n.done(), Doing: n.doing(), Body: body(n.Kids, 0), Line: n.Line}
	}
	addTask := func(n *node) { s.Tasks = append(s.Tasks, task(n)) }
	for _, sc := range secs {
		part := lines[sc.from:sc.to]
		switch sc.kind {
		case "how":
			how = append(how, part...)
			continue
		case "reprise":
			rep = append(rep, part...)
			continue
		}
		nodes := outline(part, sc.from)
		switch sc.kind {
		case "proofs":
			var walk func([]*node)
			walk = func(ns []*node) {
				for _, n := range ns {
					if n.item() {
						s.Proofs = append(s.Proofs, n.Text)
						walk(n.Kids)
					}
				}
			}
			walk(nodes)
		case "side":
			boxed(nodes, func(n *node) {
				s.SideEffects = append(s.SideEffects, task(n))
				s.sideAt = append(s.sideAt, n.Line)
			})
		case "manual":
			var walk func([]*node)
			walk = func(ns []*node) {
				for _, n := range ns {
					switch {
					case n.Box != 0:
						s.ManualSteps = append(s.ManualSteps, task(n))
						s.manualAt = append(s.manualAt, n.Line)
						boxed(n.Kids, func(k *node) {
							s.ManualSteps = append(s.ManualSteps, task(k))
							s.manualAt = append(s.manualAt, k.Line)
						})
						continue
					case n.item():
						s.ManualSetup = append(s.ManualSetup, n.label())
					case n.Code:
						s.ManualSetup = append(s.ManualSetup, strings.TrimSpace(n.Text))
					default:
						s.ManualSetup = append(s.ManualSetup, n.Lines...)
					}
					walk(n.Kids)
				}
			}
			walk(nodes)
		case "questions":
			s.readQuestions(nodes)
		case "approach":
			var walk func([]*node, int)
			walk = func(ns []*node, depth int) {
				for _, n := range ns {
					if n.Box != 0 {
						addTask(n)
						boxed(n.Kids, addTask)
						continue
					}
					s.Approach = append(s.Approach, Item{Text: n.label(), Depth: min(depth, 2), Plain: n.plain(), Code: n.Code})
					walk(n.Kids, depth+1)
				}
			}
			walk(nodes, 0)
		case "":
			for _, n := range nodes {
				if !n.item() && !n.Code && !strings.HasPrefix(n.Text, "(Le plan arrive") {
					s.Summary = append(s.Summary, n.Lines...)
				}
			}
			boxed(nodes, addTask)
		default:
			boxed(nodes, addTask)
		}
	}
	s.HowItWorks = strings.TrimSpace(strings.Join(how, "\n"))
	s.Reprise = strings.TrimSpace(strings.Join(rep, "\n"))
}

// readQuestions reads "## Questions": each item is a question. Paragraphs at
// the margin after a question, and option or recommendation items written
// at the margin right after it, still belong to it.
func (s *State) readQuestions(nodes []*node) {
	for i := 0; i < len(nodes); i++ {
		n := nodes[i]
		if !n.item() {
			continue // text before the first question
		}
		var extra []*node
		for i+1 < len(nodes) {
			k := nodes[i+1]
			_, isOpt := option(k)
			loose := !k.item() || k.Box == 0 && (isOpt && !isDigits(k.Marker) || recoRe.MatchString(k.Text))
			if !loose {
				break
			}
			extra = append(extra, k)
			i++
		}
		s.Questions = append(s.Questions, question(n, extra))
	}
}

func isDigits(marker string) bool { return marker != "" && marker[0] >= '0' && marker[0] <= '9' }

// TickManual checks (or unchecks) manual step i in the file's body; Save
// writes it.
func (s *State) TickManual(i int, done bool) bool {
	if i < 0 || i >= len(s.manualAt) || !s.tick(s.manualAt[i], done) {
		return false
	}
	s.ManualSteps[i].Done = done
	return true
}

// TickSide checks (or unchecks) side effect i: the human verified it.
func (s *State) TickSide(i int, done bool) bool {
	if i < 0 || i >= len(s.sideAt) || !s.tick(s.sideAt[i], done) {
		return false
	}
	s.SideEffects[i].Done, s.SideEffects[i].Doing = done, false
	return true
}

// tick sets the box of the task on body line n.
func (s *State) tick(n int, done bool) bool {
	lines := strings.Split(s.body, "\n")
	if n >= len(lines) {
		return false
	}
	l := lines[n]
	m := taskRe.FindStringSubmatchIndex(l)
	if m == nil {
		return false
	}
	mark := " "
	if done {
		mark = "x"
	}
	lines[n] = l[:m[2]] + mark + l[m[3]:]
	s.body = strings.Join(lines, "\n")
	return true
}

func (s *State) DoneCount() int { return Count(s.Tasks, true) }

// Count counts done tasks (done=true) or in-progress ones (done=false).
func Count(ts []Task, done bool) int {
	n := 0
	for _, t := range ts {
		if (done && t.Done) || (!done && t.Doing) {
			n++
		}
	}
	return n
}

// OpenQuestions are the questions not yet answered.
func (s *State) OpenQuestions() []Question {
	var out []Question
	for _, q := range s.Questions {
		if !q.Done {
			out = append(out, q)
		}
	}
	return out
}

// Set changes one frontmatter key; an empty value removes it.
func (s *State) Set(key, value string) {
	out := s.front[:0:0]
	found := false
	for _, l := range s.front {
		k, _, ok := strings.Cut(l, ":")
		k = strings.TrimSpace(k)
		if ok && (strings.EqualFold(k, key) || key == "attente" && strings.EqualFold(k, "waiting")) {
			found = true
			if value != "" {
				out = append(out, key+": "+value)
			}
			continue
		}
		out = append(out, l)
	}
	if !found && value != "" {
		out = append(out, key+": "+value)
	}
	s.front = out
	switch key {
	case "phase":
		s.Phase = value
	case "attente":
		s.Waiting = value
	case "push":
		s.Push = value
	case "pr":
		s.PR = value
	}
}

func (s *State) String() string {
	return "---\n" + strings.Join(s.front, "\n") + "\n---\n" + s.body
}

// PushAllowed reports whether a human allowed pushing. The allowance covers
// a commit (PushCommit), not a number of commands: pushes, PR creation and
// retries all pass while the branch stays on it.
func (s *State) PushAllowed() bool {
	f := strings.Fields(s.Push)
	if len(f) == 0 {
		return false
	}
	return strings.EqualFold(f[0], "autorisé") || strings.EqualFold(f[0], "autorise") || strings.EqualFold(f[0], "allowed")
}

// PRAllowed reports whether a human allowed opening the pull request.
func (s *State) PRAllowed() bool {
	f := strings.Fields(strings.ToLower(s.PR))
	return len(f) > 0 && (strings.HasPrefix(f[0], "autoris") || f[0] == "allowed")
}

// PushCommit is the commit the allowance covers, "" if none was recorded.
func (s *State) PushCommit() string {
	if f := strings.Fields(s.Push); len(f) > 1 && s.PushAllowed() {
		return f[1]
	}
	return ""
}

// PushCovers reports whether the allowance covers head: an allowance with no
// commit recorded covers anything (the guard pins it on first use).
func (s *State) PushCovers(head string) bool {
	if !s.PushAllowed() {
		return false
	}
	c := s.PushCommit()
	return c == "" || head == "" || strings.HasPrefix(head, c) || strings.HasPrefix(c, head)
}

func (s *State) Save() error { return os.WriteFile(s.Path, []byte(s.String()), 0o644) }

// New is the file a chantier starts with, before its agent writes the plan.
func New(path, key, title string) *State {
	return Parse(path, "---\nticket: "+key+"\nphase: plan\n---\n# "+key+" · "+title+"\n\n"+
		"(Le plan arrive : l'agent l'écrit ici.)\n")
}

// AddAsked writes a question and its answer under "### Questions posées" in
// "## Comment ça marche", creating what is missing (the section goes before
// "## Preuves", or at the end). Save writes it.
func (s *State) AddAsked(question, answer string) {
	entry := []string{"- **" + strings.TrimSpace(question) + "**"}
	for _, l := range strings.Split(strings.TrimSpace(answer), "\n") {
		entry = append(entry, strings.TrimRight("  "+l, " "))
	}
	lines := strings.Split(s.body, "\n")
	how, asked, end := -1, -1, len(lines) // end: where the how section stops
	for i, l := range lines {
		switch {
		case howHd.MatchString(l):
			how = i
		case how >= 0 && end == len(lines) && h12Re.MatchString(l):
			end = i
		case how >= 0 && end == len(lines) && askedHd.MatchString(l):
			asked = i
		}
	}
	var out []string
	switch {
	case how < 0:
		at := len(lines)
		for i, l := range lines {
			if proofHd.MatchString(l) && strings.HasPrefix(l, "## ") {
				at = i
				break
			}
		}
		block := append([]string{"## Comment ça marche", "", "### Questions posées"}, entry...)
		block = append(block, "")
		for at > 0 && strings.TrimSpace(lines[at-1]) == "" && at == len(lines) {
			at--
		}
		if at > 0 && strings.TrimSpace(lines[at-1]) != "" {
			block = append([]string{""}, block...)
		}
		out = append(append(append([]string{}, lines[:at]...), block...), lines[at:]...)
	case asked < 0:
		at := end
		for at > how+1 && strings.TrimSpace(lines[at-1]) == "" {
			at--
		}
		block := append([]string{"", "### Questions posées"}, entry...)
		out = append(append(append([]string{}, lines[:at]...), block...), lines[at:]...)
	default:
		at := end
		for i := asked + 1; i < end; i++ {
			if strings.HasPrefix(lines[i], "### ") {
				at = i
				break
			}
		}
		for at > asked+1 && strings.TrimSpace(lines[at-1]) == "" {
			at--
		}
		out = append(append(append([]string{}, lines[:at]...), entry...), lines[at:]...)
	}
	s.body = strings.Join(out, "\n")
	s.HowItWorks = Parse(s.Path, s.String()).HowItWorks
}
