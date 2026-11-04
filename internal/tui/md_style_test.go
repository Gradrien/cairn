package tui

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

func TestMarkdownLooksRendered(t *testing.T) {
	src := "### User description\n\n## Theme\n\nThis makes the **Résumé** readable, for [YAK-800](https://x.y/issue/YAK-800).\n\n- the `notes_summary` panel\n\n## Checks\n\n- [ ] freelance mode\n- [x] tested\n\n| A | B |\n|---|---|\n| 1 | 2 |\n"
	out := renderMarkdown(src, 80)
	plain := xansi.Strip(strings.Join(out, "\n"))
	for _, raw := range []string{"## ", "### ", "[ ]", "[x]", "**"} {
		if strings.Contains(plain, raw) {
			t.Errorf("raw markdown %q still shown:\n%s", raw, plain)
		}
	}
}
