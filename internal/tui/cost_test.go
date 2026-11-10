package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"cairn/internal/journal"
	"cairn/internal/usage"
)

// withCost gives the first chantier a plan phase then a build phase, with a
// subagent on Haiku.
func withCost(m Model) Model {
	key := m.chantiers[0].Key
	if m.usage == nil {
		m.usage = map[string]usage.Report{}
	}
	t0 := time.Now().Add(-2 * time.Hour)
	m.watch.history[key] = []journal.Event{{At: t0, Text: "phase:Plan"}, {At: t0.Add(time.Hour), Text: "phase:Build"}}
	m.usage[key] = usage.Report{
		Entries: []usage.Entry{
			{At: t0.Add(time.Minute), Model: "claude-opus-5-5", Agent: usage.Main, Tokens: usage.Tokens{Output: 20_000, CacheRead: 2_000_000}},
			{At: t0.Add(70 * time.Minute), Model: "claude-opus-5-5", Agent: usage.Main, Tokens: usage.Tokens{Output: 50_000, CacheRead: 5_000_000}},
			{At: t0.Add(80 * time.Minute), Model: "claude-haiku-5-5", Agent: "cairn-explorer", Tokens: usage.Tokens{Output: 10_000, CacheRead: 300_000}},
		},
		Last:     []usage.Action{{At: time.Now().Add(-time.Minute), Agent: usage.Main, Tool: "Edit", Target: "detail.go"}},
		Sessions: 1, Subs: 1,
	}
	return m
}

func TestCostSection(t *testing.T) {
	m := withCost(fixture(t, 160, 45))
	m.cursor = 0
	text := ansi.Strip(strings.Join(m.detailLayout(m.detailWidth()).lines, "\n"))
	// plan: 20k×20 + 2M×0.2 = 0.80 ; build: 50k×20 + 5M×0.2 = 2.00 ; haiku: 10k×0.5 + 300k×0.01 = 0.008
	for _, want := range []string{"Coût 2,81 $", "équivalent API", "opus 5.5", "haiku 5.5", "Plan 0,80 $ · Build 2,01 $",
		"principal 2,80 $ · cairn-explorer < 0,01 $ (1 sous-agent)", "de contexte relu", "↳ principal · Edit detail.go"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
	// A chantier at work shows its cost in the list; one waiting for you shows
	// what to decide instead.
	for _, c := range m.chantiers {
		if m.groupOf(c) == "work" {
			m.usage[c.Key] = m.usage[m.chantiers[0].Key]
		}
	}
	if list := strings.Join(screen(m), "\n"); !strings.Contains(list, "Build 1/3 · ◐ · ~2,8 $") {
		t.Errorf("the list shows the cost\n%s", list)
	}
	if text := ansi.Strip(strings.Join(fixture(t, 160, 45).detailLayout(100).lines, "\n")); strings.Contains(text, "Coût") {
		t.Error("no transcript, no cost section")
	}
}
