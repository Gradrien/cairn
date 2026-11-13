package mux

import (
	"errors"
	"testing"
)

func TestAppKind(t *testing.T) {
	str := func(s string) *string { return &s }
	argv := func(a ...string) func() (hcTerminal, error) {
		return func() (hcTerminal, error) {
			var d hcTerminal
			d.Foreground = &struct {
				Argv []string `json:"argv"`
				Cwd  string   `json:"cwd"`
				Name string   `json:"name"`
			}{Argv: a}
			return d, nil
		}
	}
	never := func() (hcTerminal, error) { return hcTerminal{}, errors.New("not asked") }
	cases := []struct {
		name    string
		t       appTerminal
		program string
		get     func() (hcTerminal, error)
		want    string
	}{
		{"reported", appTerminal{App: str("claude-code")}, "2.1.323", never, "claude"},
		{"by program", appTerminal{}, "codex", never, "codex"},
		{"claude before it reports", appTerminal{}, "2.1.323", argv("claude", "--model", "opus"), "claude"},
		{"shell", appTerminal{}, "zsh", never, ""},
		{"login shell", appTerminal{}, "-zsh", never, ""},
		{"other program", appTerminal{}, "nvim", argv("nvim"), ""},
	}
	for _, c := range cases {
		if got, _ := appKind(c.t, c.program, c.get); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
