package tui

import (
	"os"
	"testing"

	"cairn/internal/chantier"
	"cairn/internal/config"
	"cairn/internal/journal"
	"cairn/internal/progress"
	"cairn/internal/state"
	"cairn/internal/tracker"
)

func TestReadiness(t *testing.T) {
	c := &chantier.Chantier{Key: "YAK-1", PRs: []tracker.PullRequest{{State: "open"}},
		State: state.Parse("/x", "- [x] a\n\n## Effets de bord\n- [x] b\n")}
	m := Model{prs: map[string]progress.PR{}}
	if _, ready := m.readiness(c); ready {
		t.Fatal("ready without PR data")
	}
	m.prs["YAK-1"] = progress.PR{State: "OPEN", ReviewDecision: "APPROVED", Mergeable: "MERGEABLE", Checks: progress.Checks{Passed: 3}}
	if checks, ready := m.readiness(c); !ready {
		t.Fatalf("should be ready: %+v", checks)
	}
	m.prs["YAK-1"] = progress.PR{State: "OPEN", ReviewDecision: "APPROVED", Mergeable: "MERGEABLE", Checks: progress.Checks{Passed: 3, Failed: 1}}
	if _, ready := m.readiness(c); ready {
		t.Fatal("ready with failing CI")
	}
	c.State = state.Parse("/x", "- [x] a\n- [ ] b\n")
	m.prs["YAK-1"] = progress.PR{State: "OPEN", ReviewDecision: "APPROVED", Mergeable: "MERGEABLE", Checks: progress.Checks{Passed: 3}}
	if _, ready := m.readiness(c); ready {
		t.Fatal("ready with open tasks and no side effects listed")
	}
}

// Before the tracker answers, a chantier with a PR looks like Build: nothing
// may be journaled then, or every start logs a false Build → Review.
func TestObserveWaitsForIssues(t *testing.T) {
	t.Setenv("CAIRN_HOME", t.TempDir())
	_ = os.MkdirAll(config.ChantiersDir(), 0o755)
	m := Model{watch: newWatch(), chantiers: []*chantier.Chantier{{Key: "YAK-1", Phase: chantier.Build}}}
	m.observe()
	if ev := journal.Read(config.ChantiersDir(), "YAK-1"); len(ev) != 0 {
		t.Fatalf("journaled before issues loaded: %v", ev)
	}
	m.watch.ready = true
	m.observe()
	if ev := journal.Read(config.ChantiersDir(), "YAK-1"); len(ev) != 1 || ev[0].Text != "phase:Build" {
		t.Fatalf("got %v", ev)
	}
}
