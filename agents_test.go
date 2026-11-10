package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The subagents install.sh links into ~/.claude/agents: each has the front
// matter Claude Code reads, a light model, and the explorer cannot write.
func TestSubagents(t *testing.T) {
	files, _ := filepath.Glob("agents/*.md")
	if len(files) < 2 {
		t.Fatalf("subagents: %v", files)
	}
	skill, _ := os.ReadFile("skill/SKILL.md")
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		front, body, ok := strings.Cut(strings.TrimPrefix(string(data), "---\n"), "\n---\n")
		if !ok || strings.TrimSpace(body) == "" {
			t.Errorf("%s: front matter then a body", f)
			continue
		}
		fields := map[string]string{}
		for _, l := range strings.Split(front, "\n") {
			k, v, _ := strings.Cut(l, ":")
			fields[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
		name := strings.TrimSuffix(filepath.Base(f), ".md")
		if fields["name"] != name || fields["description"] == "" || fields["tools"] == "" {
			t.Errorf("%s: name, description and tools: %v", f, fields)
		}
		if m := fields["model"]; m != "haiku" && m != "sonnet" {
			t.Errorf("%s: a light model, not %q", f, m)
		}
		if name == "cairn-explorer" && (strings.Contains(fields["tools"], "Bash") || strings.Contains(fields["tools"], "Edit") || strings.Contains(fields["tools"], "Write")) {
			t.Errorf("%s: read only, tools %q", f, fields["tools"])
		}
		if !strings.Contains(string(skill), name) {
			t.Errorf("the skill never says when to use %s", name)
		}
	}
}
