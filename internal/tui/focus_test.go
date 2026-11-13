package tui

import "testing"

func TestFocusWanted(t *testing.T) {
	t.Setenv("CAIRN_HOME", t.TempDir())
	m := fixture(t, 140, 40)
	m.all = m.chantiers
	m.filter, m.overlay = "zzz", "help"
	m.wantKey = "YAK-788"
	m.focusWanted()
	if c := m.selected(); c == nil || c.Key != "YAK-788" || m.wantKey != "" || m.filter != "" || m.overlay != "" {
		t.Fatalf("YAK-788 selected, filter and overlay cleared: %+v %q %q %q", c, m.wantKey, m.filter, m.overlay)
	}
	m.wantKey = "YAK-999"
	m.focusWanted()
	if m.wantKey != "YAK-999" {
		t.Error("a chantier not listed yet stays wanted until it shows")
	}
}
