package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestMarkdownKeepsImageMarkers(t *testing.T) {
	out := strings.Join(renderMarkdown("Avant :\n\n[image 1]\n\n- puce [image 2] fin\n\n| a | b |\n|---|---|\n| [image 3] | x |", 80), "\n")
	plain := ansi.Strip(out)
	for _, want := range []string{"[image 1]", "[image 2]", "[image 3]"} {
		if !strings.Contains(plain, want) {
			t.Errorf("missing %s in:\n%s", want, plain)
		}
	}
}
