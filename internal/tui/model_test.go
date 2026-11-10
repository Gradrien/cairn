package tui

import (
	"strings"
	"testing"
)

func TestModelArgs(t *testing.T) {
	for _, tc := range []struct{ kind, model, want string }{
		{"claude", "sonnet", "--model sonnet"},
		{"", " opus ", "--model opus"},
		{"claude", "", ""},
		{"codex", "sonnet", ""},
		{"claude", "sonnet; rm -rf", ""},
	} {
		if got := strings.Join(modelArgs(tc.kind, tc.model), " "); got != tc.want {
			t.Errorf("modelArgs(%q, %q) = %q, want %q", tc.kind, tc.model, got, tc.want)
		}
	}
}
