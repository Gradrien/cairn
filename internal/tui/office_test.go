package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"cairn/internal/chantier"
	"cairn/internal/herdr"
	"cairn/internal/office"
	"cairn/internal/state"
	"cairn/internal/usage"
)

func TestOfficeOpensAndNavigates(t *testing.T) {
	m := fixture(t, 160, 45)
	m = press(t, m, "b")
	if m.view != "office" || m.officeZoom {
		t.Fatalf("b opens the open space: %q %v", m.view, m.officeZoom)
	}
	lines := screen(m)
	for i, l := range lines {
		if w := ansi.StringWidth(l); w > 160 {
			t.Errorf("line %d is %d wide", i, w)
		}
	}
	text := strings.Join(lines, "\n")
	for _, c := range m.chantiers {
		if !strings.Contains(text, c.Key) {
			t.Errorf("no room for %s", c.Key)
		}
	}
	m = press(t, m, "right")
	if m.cursor != 1 {
		t.Errorf("right moves to the next room: %d", m.cursor)
	}
	m = press(t, m, "enter")
	if !m.officeZoom || !strings.Contains(strings.Join(screen(m), "\n"), "‹ esc open space") {
		t.Error("enter steps into the room")
	}
	m = press(t, m, "esc")
	if m.officeZoom || m.view != "office" {
		t.Error("esc goes back to the open space")
	}
	m = press(t, m, "esc")
	if m.view != "" {
		t.Error("esc again goes back to the list")
	}
	// The frames only run while the office shows.
	m.view = "office"
	if _, cmd := m.update(officeTick{}); cmd == nil {
		t.Error("the office ticks")
	}
	m.view = ""
	if next, cmd := m.update(officeTick{}); cmd != nil || next.(Model).officeTicking {
		t.Error("ticks stop once the office is left")
	}
}

func TestOfficeReadsActivity(t *testing.T) {
	now := time.Now()
	c := &chantier.Chantier{Key: "YAK-1", Phase: chantier.Build, Agents: []herdr.Agent{{Name: "yak-1", Status: "working"}},
		State: &state.State{Questions: []state.Task{{Text: "Quoi ?"}}, Tasks: []state.Task{{Text: "a", Done: true}, {Text: "b"}}}}
	m := fixture(t, 160, 45)
	m.chantiers = []*chantier.Chantier{c}
	set := func(rep usage.Report) {
		m.usage = map[string]usage.Report{c.Key: rep}
	}
	act := func(tool, target, cmd string) usage.Action {
		return usage.Action{At: now.Add(-5 * time.Second), Agent: usage.Main, Tool: tool, Target: target, Command: cmd}
	}
	for _, tc := range []struct {
		a    usage.Action
		want office.Kind
	}{
		{act("Read", "detail.go", ""), office.Reading},
		{act("Edit", "detail.go", ""), office.Writing},
		{act("Edit", "YAK-1.md", ""), office.Planning},
		{act("Bash", "Run tests", "cd /w && go test ./..."), office.Testing},
		{act("Bash", "List files", "ls -la"), office.Running},
	} {
		set(usage.Report{Recent: []usage.Action{tc.a}})
		if got := m.officeInput(c, now).Main.Kind; got != tc.want {
			t.Errorf("%s %s: kind %d, want %d", tc.a.Tool, tc.a.Target, got, tc.want)
		}
	}

	// Delegating: the main agent goes to the colleague who just started.
	start := now.Add(-3 * time.Second)
	set(usage.Report{
		Recent:   []usage.Action{{At: start, Agent: usage.Main, Tool: "Agent", Target: "Effets de bord"}, {At: now, Agent: "cairn-explorer", Tool: "Grep", Target: "triggerBulk"}},
		Subagent: []usage.Sub{{Type: "cairn-explorer", Desc: "Effets de bord", Start: start, Last: now}},
	})
	in := m.officeInput(c, now)
	if in.Main.Kind != office.Delegating || len(in.Colleagues) != 1 || in.Main.To != in.Colleagues[0].ID {
		t.Fatalf("delegating: %+v", in)
	}
	if col := in.Colleagues[0]; col.Mood != office.Working || col.Bubble != "Grep triggerBulk" {
		t.Errorf("colleague at work: %+v", col)
	}
	// Done: it brings the result, then leaves.
	set(usage.Report{Subagent: []usage.Sub{{Type: "cairn-explorer", Desc: "Effets de bord", Start: start, Last: now.Add(-5 * time.Second), Finished: true}}})
	if col := m.officeInput(c, now).Colleagues[0]; col.Mood != office.Delivering {
		t.Errorf("delivering: %+v", col)
	}
	set(usage.Report{Subagent: []usage.Sub{{Type: "cairn-explorer", Start: start, Last: now.Add(-time.Minute), Finished: true}}})
	if col := m.officeInput(c, now).Colleagues[0]; col.Mood != office.Leaving {
		t.Errorf("leaving: %+v", col)
	}

	// Waiting for you beats everything.
	c.State.Waiting = "Valider le plan"
	if in := m.officeInput(c, now); in.Main.Kind != office.Waiting || in.Main.Label != "? valider le plan" {
		t.Errorf("waiting: %+v", in.Main)
	}
	c.State.Waiting = ""

	// A plan section that changes shows the agent writing it, on the board too.
	m.notePlanChanges()
	c.State.Questions = append(c.State.Questions, state.Task{Text: "Et ça ?"})
	m.notePlanChanges()
	in = m.officeInput(c, now)
	if in.Main.Kind != office.Planning || in.Main.Label != "rédige les questions" {
		t.Errorf("planning: %+v", in.Main)
	}
	hot := ""
	for _, l := range in.Board {
		if l.Hot {
			hot = l.Text
		}
	}
	if hot != "? 2 questions" {
		t.Errorf("hot board line: %q in %+v", hot, in.Board)
	}

	// No agent and nothing recent: nobody at the office.
	c.Agents = nil
	m.usage = nil
	if in := m.officeInput(c, now); in.Main.Kind != office.Absent {
		t.Errorf("absent: %+v", in.Main)
	}
}

func TestModelSwitch(t *testing.T) {
	for _, tc := range []struct{ kind, model, want string }{
		{"claude", "sonnet", "/model sonnet"},
		{"", " opus ", "/model opus"},
		{"claude", "", ""},
		{"codex", "sonnet", ""},
		{"claude", "sonnet; rm -rf", ""},
	} {
		if got := modelSwitch(tc.kind, tc.model); got != tc.want {
			t.Errorf("modelSwitch(%q, %q) = %q, want %q", tc.kind, tc.model, got, tc.want)
		}
	}
}
