package tui

import "testing"

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
