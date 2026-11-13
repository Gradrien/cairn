package tui

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"cairn/internal/native"
	"cairn/internal/state"
)

func TestNativeDetail(t *testing.T) {
	t.Setenv("CAIRN_HOME", t.TempDir())
	m := fixture(t, 140, 40)
	m.all = m.chantiers
	m.cursor = 0
	d := m.nativeDetail()
	if d.Key != "YAK-861" || d.Card == nil || d.Card.Tone != "warn" || d.Card.Actions[0].Key != "v" {
		t.Fatalf("the plan to approve leads with its card and Valider: %+v", d.Card)
	}
	var ids []string
	for _, s := range d.Sections {
		ids = append(ids, s.ID)
	}
	if got := strings.Join(ids, " "); !strings.HasPrefix(got, "questions goal tasks side") {
		t.Errorf("a plan to approve shows questions, goal, tasks, side effects first: %s", got)
	}
	m.cursor = 3
	d = m.nativeDetail()
	if d.Card.Tone != "danger" || d.Sections[0].ID != "pr" || !d.Sections[0].PR.Checklist || len(d.Sections[0].PR.Failed) != 2 {
		t.Errorf("a broken CI shows in the card and the PR leads with its checklist: %+v", d.Sections[0])
	}
}

func TestNativeRequests(t *testing.T) {
	t.Setenv("CAIRN_HOME", t.TempDir())
	m := fixture(t, 140, 40)
	m.all = m.chantiers
	c := m.chantiers[0]
	if err := os.WriteFile(c.State.Path, []byte("---\nphase: plan\n---\n- [ ] Une\n\n## Effets de bord\n- [ ] Autres appelants\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := native.Send(native.Request{Key: "yak-861", Action: "side"}); err != nil {
		t.Fatal(err)
	}
	if err := native.Send(native.Request{Key: "YAK-788", Action: "key", Arg: "x"}); err != nil {
		t.Fatal(err)
	}
	reqs := native.Take()
	if len(reqs) != 2 || reqs[0].Action != "side" {
		t.Fatalf("requests come back oldest first: %+v", reqs)
	}
	if left, _ := os.ReadDir(native.RequestsDir()); len(left) != 0 {
		t.Error("taken requests are removed")
	}
	m, _ = m.runRequest(reqs[0])
	s, _ := state.Load(c.State.Path)
	if s == nil || !s.SideEffects[0].Done {
		t.Error("side ticks the side effect in the plan file")
	}
	m, _ = m.runRequest(reqs[1])
	if m.selected().Key != "YAK-788" || m.overlay != "close" {
		t.Errorf("key x selects YAK-788 and asks to confirm the close: %q %q", m.selected().Key, m.overlay)
	}
	if d := m.nativeDetail(); d.Overlay == nil || len(d.Overlay.Buttons) == 0 {
		t.Errorf("the confirmation is exported with its buttons: %+v", d.Overlay)
	} else {
		js, _ := json.Marshal(d.Overlay)
		t.Log(string(js))
	}
	m, _ = m.runRequest(native.Request{Action: "cancel"})
	if m.overlay != "" {
		t.Error("cancel closes the confirmation")
	}
	m, _ = m.runRequest(native.Request{Key: "YAK-1", Action: "key", Arg: "v"})
	if !strings.Contains(m.flash, "YAK-1") {
		t.Errorf("an unknown chantier is said, not acted on: %q", m.flash)
	}
}
