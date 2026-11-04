package prthread

import (
	"strings"
	"testing"
)

func TestCleanKeepsCode(t *testing.T) {
	body := "Test:\n\n```sh\npsql <<'SQL'\nselect 1 where score > 70;\n    indented\nSQL\n```\n\nKeep `a<b>c` and <b>bold</b><br>![x](https://h/i.png)"
	got := (&Thread{}).clean(body)
	for _, want := range []string{"psql <<'SQL'\nselect 1 where score > 70;\n    indented\nSQL\n```", "`a<b>c`", "bold\n[image 1]"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "<b>bold") {
		t.Errorf("HTML left outside code:\n%s", got)
	}
}
