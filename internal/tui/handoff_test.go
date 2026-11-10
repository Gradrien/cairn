package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"cairn/internal/chantier"
	"cairn/internal/herdr"
	"cairn/internal/state"
	"cairn/internal/usage"
)

func TestHandoff(t *testing.T) {
	t.Setenv("CAIRN_HOME", t.TempDir())
	m := selectKey(t, fixture(t, 160, 45), "YAK-712")
	c := m.selected()
	c.Phase = chantier.Build
	c.Agents = []herdr.Agent{{Name: "yak-712", Kind: "claude", Status: "working", PaneID: "p1"}}
	c.State = state.Parse("/c/YAK-712.md", "---\nphase: build\n---\n# YAK-712 · T\n\n- [x] a\n- [~] b\n")
	c.State.UpdatedAt = time.Now()
	m.usage = map[string]usage.Report{c.Key: {Context: 167_000, ContextAt: time.Now()}}

	text := ansi.Strip(strings.Join(m.detailLayout(m.detailWidth()).lines, "\n"))
	if !strings.Contains(text, "contexte 167 k, relu à chaque tour · R repartir à neuf") {
		t.Errorf("the detail says the context is heavy:\n%s", text)
	}
	if list := strings.Join(screen(m), "\n"); !strings.Contains(list, "contexte 167 k") {
		t.Errorf("so does the list:\n%s", list)
	}
	if b := m.handoffButton(c); !b.enabled || b.label != "Repartir à neuf" {
		t.Errorf("button %+v", b)
	}

	// R asks for the handoff; the button waits until it is written and the
	// agent stopped.
	next, cmd := m.handoff()
	m = next.(Model)
	if cmd == nil || !m.handoffAsked(c) || m.handoffReady(c) {
		t.Fatalf("asked %v ready %v", m.handoffAsked(c), m.handoffReady(c))
	}
	if next, _ := m.handoff(); !strings.Contains(next.(Model).flash, "écrit sa reprise") {
		t.Errorf("a second R before it is written says so: %q", next.(Model).flash)
	}
	c.State = state.Parse("/c/YAK-712.md", "---\nphase: build\n---\n# YAK-712 · T\n\n- [x] a\n- [~] b\n\n## Reprise\nJ'en suis à b.\n")
	c.State.UpdatedAt = time.Now().Add(time.Second)
	if m.handoffReady(c) {
		t.Error("not while the agent still works")
	}
	c.Agents[0].Status = "idle"
	if !m.handoffReady(c) {
		t.Fatal("written and stopped: ready")
	}
	cd := m.cardFor(c)
	if cd == nil || !cd.primary || cd.actions[0].key != "R" || !strings.Contains(cd.title, "Reprise écrite") {
		t.Errorf("the card leads to the new agent: %+v", cd)
	}

	// Without a Claude agent, nothing to hand over.
	c.Agents[0].Kind = "codex"
	if m.canHandoff(c) {
		t.Error("only Claude agents are handed over")
	}
}

func TestValidateStartsANewAgent(t *testing.T) {
	t.Setenv("CAIRN_HOME", t.TempDir())
	m := selectKey(t, fixture(t, 160, 45), "YAK-861")
	c := m.selected()
	path := t.TempDir() + "/YAK-861.md"
	c.State = state.Parse(path, "---\nphase: plan\nattente: Valider le plan\n---\n# YAK-861 · T\n\n- [ ] a\n")
	c.Agents = []herdr.Agent{{Name: "yak-861", Kind: "claude", Status: "idle", PaneID: "p1"}}
	next, cmd := m.validate()
	m = next.(Model)
	if cmd == nil || !strings.Contains(m.busy, "Nouvel agent pour le build") {
		t.Errorf("validating the plan hands the build to a new agent: busy %q", m.busy)
	}
}
