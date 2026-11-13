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

func TestInlineMarkdownDropsMarks(t *testing.T) {
	for in, want := range map[string]string{
		"**Existant** : `a.ts:12` (`b`)": "Existant : a.ts:12 (b)",
		"un ` seul":                      "un  seul",
		"__gras__ puis `x`":              "gras puis x",
	} {
		if got := xansi.Strip(md(in, sSoft)); got != want {
			t.Errorf("md(%q) = %q, want %q", in, got, want)
		}
	}
}
