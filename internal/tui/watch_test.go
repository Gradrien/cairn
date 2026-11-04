package tui

import (
	"testing"

	"cairn/internal/chantier"
	"cairn/internal/progress"
	"cairn/internal/sprintosaur"
	"cairn/internal/state"
)

func TestReadiness(t *testing.T) {
	c := &chantier.Chantier{Key: "YAK-1", Issue: &sprintosaur.Issue{PullRequests: []sprintosaur.PullRequest{{State: "open"}}},
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
