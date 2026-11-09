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
//	"## Questions": open questions ("- [x]" once answered);
//	"## Preuves": every list item is a proof;
//	"## Comment ça marche": how the change works, kept as markdown for the
//	human to read (where things live, when they run, what is stored).
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
	Doing bool // "- [~]": started, not finished
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
	Questions   []Task
	Proofs      []string
	Summary     []string
	HowItWorks  string // the "## Comment ça marche" section, raw markdown
	UpdatedAt   time.Time
	front       []string // raw frontmatter lines, kept for round-trips
	body        string
}

var (
	taskRe  = regexp.MustCompile(`^\s*[-*] \[([ xX~-])\]\s+(.*)$`)
	itemRe  = regexp.MustCompile(`^\s*[-*]\s+(.*)$`)
	proofHd = regexp.MustCompile(`(?i)^#{1,6}\s+(preuves?|proofs?)\b`)
	sideHd  = regexp.MustCompile(`(?i)^#{1,6}\s+(effets? de bord|side[ -]effects?)`)
	questHd = regexp.MustCompile(`(?i)^#{1,6}\s+questions?`)
	howHd   = regexp.MustCompile(`(?i)^##\s+(comment [çc]a marche|how it works)\b`)
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
	section := "" // "", "proofs", "side", "questions", "how", "other"
	seenH2 := false
	var how []string
	for _, l := range strings.Split(s.body, "\n") {
		// "Comment ça marche" is prose with its own "###" headings: it runs
		// to the next "#" or "##" heading and is kept whole.
		if section == "how" && !h12Re.MatchString(l) {
			how = append(how, l)
			continue
		}
		if howHd.MatchString(l) {
			section, seenH2 = "how", true
			continue
		}
		if headRe.MatchString(l) {
			if t, ok := strings.CutPrefix(l, "# "); ok && s.Title == "" {
				s.Title = strings.TrimSpace(t)
				if k, rest, ok := strings.Cut(s.Title, " · "); ok && strings.EqualFold(k, s.Ticket) {
					s.Title = strings.TrimSpace(rest)
				}
			}
			switch {
			case proofHd.MatchString(l):
				section = "proofs"
			case sideHd.MatchString(l):
				section = "side"
			case questHd.MatchString(l):
				section = "questions"
			case h2Re.MatchString(l):
				section = "other"
			}
			if h2Re.MatchString(l) {
				seenH2 = true
			}
			continue
		}
		if section == "proofs" {
			if m := itemRe.FindStringSubmatch(l); m != nil {
				s.Proofs = append(s.Proofs, m[1])
			}
			continue
		}
		if m := taskRe.FindStringSubmatch(l); m != nil {
			t := Task{Text: m[2], Done: m[1] == "x" || m[1] == "X", Doing: m[1] == "~" || m[1] == "-"}
			switch section {
			case "side":
				s.SideEffects = append(s.SideEffects, t)
			case "questions":
				s.Questions = append(s.Questions, t)
			default:
				s.Tasks = append(s.Tasks, t)
			}
			continue
		}
		if section == "questions" {
			if m := itemRe.FindStringSubmatch(l); m != nil {
				s.Questions = append(s.Questions, Task{Text: m[1]})
			}
			continue
		}
		if t := strings.TrimSpace(l); !seenH2 && t != "" && !itemRe.MatchString(l) && !strings.HasPrefix(t, "(Le plan arrive") {
			s.Summary = append(s.Summary, t)
		}
	}
	s.HowItWorks = strings.TrimSpace(strings.Join(how, "\n"))
	return s
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
func (s *State) OpenQuestions() []string {
	var out []string
	for _, q := range s.Questions {
		if !q.Done {
			out = append(out, q.Text)
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
