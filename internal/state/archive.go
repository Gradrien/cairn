package state

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Archived is a shipped chantier's plan, kept in ~/.cairn/archive.
type Archived struct {
	Key        string
	ArchivedAt time.Time
	*State
}

// archName is "<KEY>-<YYYYMMDD>-<HHMMSS>.md", as journal.Archive writes it.
var archName = regexp.MustCompile(`^(.+)-(\d{8}-\d{6})\.md$`)

// LoadArchive returns the latest archived plan of each ticket in dir, most
// recently archived first. A missing dir is an empty archive.
func LoadArchive(dir string) []Archived {
	entries, _ := os.ReadDir(dir)
	latest := map[string]Archived{}
	for _, e := range entries {
		m := archName.FindStringSubmatch(e.Name())
		if e.IsDir() || m == nil {
			continue
		}
		at, err := time.ParseInLocation("20060102-150405", m[2], time.Local)
		if err != nil {
			continue
		}
		key := strings.ToUpper(m[1])
		if prev, ok := latest[key]; ok && !at.After(prev.ArchivedAt) {
			continue
		}
		s, err := Load(filepath.Join(dir, e.Name()))
		if err != nil || s == nil {
			continue
		}
		latest[key] = Archived{Key: key, ArchivedAt: at, State: s}
	}
	out := make([]Archived, 0, len(latest))
	for _, a := range latest {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ArchivedAt.After(out[j].ArchivedAt) })
	return out
}

// Body is the plan without its frontmatter.
func (s *State) Body() string { return s.body }
