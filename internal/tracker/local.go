package tracker

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"cairn/internal/i18n"
	"cairn/internal/state"
)

// Local needs no tool: a ticket is its chantier file, ~/.cairn/chantiers/KEY-n.md,
// created from a title. Numbers are never reused, archived chantiers included.
type Local struct {
	key, dir string
}

func NewLocal(key, chantiersDir string) *Local {
	return &Local{key: strings.ToUpper(key), dir: chantiersDir}
}

func (l *Local) Name() string { return i18n.T("local", "local") }

func (l *Local) Guide(key string) string {
	return i18n.T("Le ticket n'existe que dans ce fichier : son titre est tout ce qu'il dit. S'il manque l'essentiel, pose la question sous « ## Questions »",
		"The ticket only exists in that file: its title is all it says. If something essential is missing, ask under \"## Questions\"")
}

// todo is the phase of a ticket created but not planned yet.
const todo = "todo"

func (l *Local) issue(path string) (Issue, bool) {
	s, err := state.Load(path)
	if err != nil || s == nil {
		return Issue{}, false
	}
	key := strings.ToUpper(strings.TrimSuffix(filepath.Base(path), ".md"))
	is := Issue{ID: key, Title: s.Title, Assigned: true, Started: s.Phase != todo && s.Phase != ""}
	return is, true
}

func (l *Local) Mine() ([]Issue, error) {
	files, err := filepath.Glob(filepath.Join(l.dir, l.key+"-*.md"))
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(l.key) + `-\d+\.md$`)
	var out []Issue
	for _, f := range files {
		if !re.MatchString(filepath.Base(f)) {
			continue
		}
		if is, ok := l.issue(f); ok {
			out = append(out, is)
		}
	}
	return out, nil
}

func (l *Local) Free() ([]Issue, error) { return nil, nil }

func (l *Local) Get(key string) (Issue, error) {
	is, _ := l.issue(state.PathFor(l.dir, key))
	return is, nil
}

func (l *Local) Take(string) error         { return nil }
func (l *Local) Start(Issue) (bool, error) { return false, nil }

// Create writes a new chantier file, KEY-<next number>, waiting to be planned.
func (l *Local) Create(title string) (Issue, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Issue{}, fmt.Errorf("%s", i18n.T("le ticket a besoin d'un titre", "the ticket needs a title"))
	}
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return Issue{}, err
	}
	key := fmt.Sprintf("%s-%d", l.key, l.next())
	s := state.New(state.PathFor(l.dir, key), key, title)
	s.Set("phase", todo)
	if err := s.Save(); err != nil {
		return Issue{}, err
	}
	return Issue{ID: key, Title: title, Assigned: true}, nil
}

// next is one more than the highest number used, live or archived.
func (l *Local) next() int {
	re := regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(l.key) + `-(\d+)(?:-\d{8}-\d{6})?\.(md|log)$`)
	n := 0
	for _, dir := range []string{l.dir, filepath.Join(filepath.Dir(l.dir), "archive")} {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if m := re.FindStringSubmatch(e.Name()); m != nil {
				if v, _ := strconv.Atoi(m[1]); v > n {
					n = v
				}
			}
		}
	}
	return n + 1
}
