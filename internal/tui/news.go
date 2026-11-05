package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"

	"cairn/internal/chantier"
	"cairn/internal/config"
	"cairn/internal/i18n"
	"cairn/internal/prthread"
)

// What moved on a chantier since you last looked at it: phase changes and
// what people (automated reviewers included) left on its PR. A chantier
// counts as seen when you leave it after looking at it for a moment; until
// then its news stay in view.

var (
	fresh  = lipgloss.AdaptiveColor{Light: "#0e7490", Dark: "#8be9fd"}
	sFresh = lipgloss.NewStyle().Foreground(fresh)
)

const seenDwell = 2 * time.Second // shorter than this, you only passed by

// seenStore keeps, per chantier, when you last looked at it, in
// ~/.cairn/cache/seen.json so news survive a restart.
type seenStore struct {
	mu   sync.Mutex
	path string // "" keeps it in memory (tests)
	at   map[string]time.Time
}

func seenPath() string { return filepath.Join(config.Dir(), "cache", "seen.json") }

func loadSeen(path string) *seenStore {
	s := &seenStore{path: path, at: map[string]time.Time{}}
	s.read()
	return s
}

func (s *seenStore) read() {
	if s.path == "" {
		return
	}
	if b, err := os.ReadFile(s.path); err == nil {
		var at map[string]time.Time
		if json.Unmarshal(b, &at) == nil {
			for k, t := range at {
				if t.After(s.at[k]) {
					s.at[k] = t
				}
			}
		}
	}
}

func (s *seenStore) save() {
	if s.path == "" {
		return
	}
	s.read() // another cairn may have written meanwhile: keep the latest of both
	if b, err := json.MarshalIndent(s.at, "", "  "); err == nil {
		_ = os.MkdirAll(filepath.Dir(s.path), 0o755)
		_ = os.WriteFile(s.path, b, 0o644)
	}
}

func (s *seenStore) get(key string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.at[key]
	return t, ok
}

func (s *seenStore) mark(key string, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.After(s.at[key]) {
		s.at[key] = t
		s.save()
	}
}

// adopt starts tracking chantiers cairn has not seen yet: their past is not
// news.
func (s *seenStore) adopt(list []*chantier.Chantier) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now, changed := time.Now(), false
	for _, c := range list {
		if _, ok := s.at[c.Key]; !ok {
			s.at[c.Key], changed = now, true
		}
	}
	if changed {
		s.save()
	}
}

// leave marks the chantier you are leaving as seen, if you looked at it.
func (m *Model) leave(key string) {
	if m.seen != nil && key != "" && !m.lookedAt.IsZero() && time.Since(m.lookedAt) >= seenDwell {
		m.seen.mark(key, time.Now())
	}
}

type newsItem struct {
	at      time.Time
	text    string
	comment bool // something to read in the PR conversation
}

// news lists what moved on c since you last looked, newest first.
func (m Model) news(c *chantier.Chantier) []newsItem {
	if m.seen == nil || c == nil {
		return nil
	}
	since, ok := m.seen.get(c.Key)
	if !ok {
		return nil
	}
	var out []newsItem
	for _, e := range m.watch.history[c.Key] {
		if strings.HasPrefix(e.Text, "phase:") && e.At.After(since) {
			out = append(out, newsItem{at: e.At, text: historyText(e.Text)})
		}
	}
	// PR activity, one line per person: an automated review often comes as
	// a review plus a burst of inline comments.
	pr := m.prs[c.Key]
	type who struct {
		reviews, comments int
		at                time.Time
		state, verdict    string
	}
	people := map[string]*who{}
	var order []string
	for _, a := range pr.Activity {
		if !a.At.After(since) || a.Who == pr.Author || prthread.IsBot(a.Who) {
			continue
		}
		p := people[a.Who]
		if p == nil {
			p = &who{}
			people[a.Who] = p
			order = append(order, a.Who)
		}
		p.at = a.At
		if a.Review {
			p.reviews++
			if a.State != "COMMENTED" || p.state == "" {
				p.state = a.State
			}
		} else {
			p.comments++
		}
		if v := verdict(a.Body); v != "" {
			p.verdict = v
		}
	}
	for _, name := range order {
		p := people[name]
		var parts []string
		switch p.state {
		case "APPROVED":
			parts = append(parts, sGood.Render(i18n.T("a approuvé", "approved")))
		case "CHANGES_REQUESTED":
			parts = append(parts, sErr.Render(i18n.T("demande des changements", "requests changes")))
		default:
			if p.reviews > 0 {
				parts = append(parts, i18n.T("a laissé une review", "left a review"))
			}
		}
		if p.comments > 0 {
			parts = append(parts, plural(p.comments, i18n.T("commentaire", "comment"), i18n.T("commentaires", "comments")))
		}
		text := sBold.Render(name) + " " + strings.Join(parts, ", ")
		if p.verdict != "" {
			text += sMuted.Render(" · ") + p.verdict
		}
		out = append(out, newsItem{at: p.at, text: text, comment: true})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].at.After(out[j].at) })
	return out
}

var verdictRe = regexp.MustCompile(`(?i)\bverdict\**\s*:\s*\**\s*([^\n]+)`)

// verdict pulls the "Verdict: …" line automated reviews open with.
func verdict(body string) string {
	m := verdictRe.FindStringSubmatch(body)
	if m == nil {
		return ""
	}
	v := strings.Trim(strings.TrimSpace(strings.NewReplacer("**", "", "`", "").Replace(m[1])), " .:")
	return trunc(v, 60)
}

// clock is "15:04" today, "04/11 15:04" before.
func clock(t time.Time) string {
	if y, d := t.YearDay(), time.Now().YearDay(); y == d && t.Year() == time.Now().Year() {
		return t.Format("15:04")
	}
	return t.Format("02/01 15:04")
}

// newsShort is the news of the list line, "" when nothing moved.
func (m Model) newsShort(c *chantier.Chantier) string {
	n := m.news(c)
	switch {
	case len(n) == 0:
		return ""
	case len(n) == 1 && n[0].comment:
		return i18n.T("nouveau sur la PR", "new on the PR")
	case len(n) == 1:
		return n[0].text
	}
	return plural(len(n), i18n.T("nouveauté", "update"), i18n.T("nouveautés", "updates"))
}

func (m Model) newsCount() int {
	n := 0
	for _, c := range m.chantiers {
		if len(m.news(c)) > 0 {
			n++
		}
	}
	return n
}

// secNews is the detail section of what moved since your last visit.
func (m Model) secNews(b *detailBuilder, c *chantier.Chantier) {
	items := m.news(c)
	if len(items) == 0 {
		return
	}
	since, _ := m.seen.get(c.Key)
	b.section(i18n.T("Nouveau depuis ta visite", "New since your visit"), sFresh.Render(fmt.Sprint(len(items))), since.Format("02/01 15:04"))
	comment := false
	for _, it := range items {
		b.add(hang("  "+sFresh.Render("● ")+sMuted.Render(clock(it.at))+"  ", it.text, b.w)...)
		comment = comment || it.comment
	}
	if comment {
		b.addT("comments", "  "+sKey.Render("c")+sMuted.Render(i18n.T(" lire la conversation de la PR", " read the PR conversation")))
	}
}
