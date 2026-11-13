package mux

import (
	"encoding/json"
	"testing"
)

func TestRexBlocksToAgents(t *testing.T) {
	// What rex.lua prints: Lua writes an empty table as {}.
	raw := `{"sessions":[{"session_id":"session:1","label":"YAK-12 · build 2/5"}],"blocks":[
	 {"session_id":"session:1","block_id":"block:a","label":"yak-12","argv0":"claude","name":"2.1.322","cwd":"/r/app-12/web","shell_cwd":"/r/app-12","title":"Tests","records":[{"id":"","state":"working","app":"claude-code"}]},
	 {"session_id":"session:1","block_id":"block:b","label":"shell","argv0":"-zsh","name":"zsh","cwd":"/r/app-12","records":{}},
	 {"session_id":"session:1","block_id":"block:c","label":"shell","argv0":"-zsh","name":"zsh","records":[{"state":"done","app":"claude-code"}]},
	 {"session_id":"session:1","block_id":"block:d","label":"shell","argv0":"codex","records":[{"state":"idle"},{"id":"tool","state":"blocked"}]}
	]}`
	var s rexSnap
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatal(err)
	}
	var agents []Agent
	for _, b := range s.Blocks {
		if b.kind() != "" {
			agents = append(agents, b.agent())
		}
	}
	if len(agents) != 2 {
		t.Fatalf("agents = %+v, want claude and codex only (an exited agent's done record is not an agent)", agents)
	}
	a := agents[0]
	if a.Kind != "claude" || a.Name != "yak-12" || a.Status != "working" || a.Dir() != "/r/app-12/web" || a.WorkspaceID != "session:1" || a.PaneID != "block:a" {
		t.Errorf("claude agent = %+v", a)
	}
	if b := agents[1]; b.Kind != "codex" || b.Name != "" || b.Status != "blocked" {
		t.Errorf("codex agent = %+v: a blocked child record wins, and Rex's default label is no name", b)
	}
	if got := baseLabel(s.Sessions[0].Label); got != "YAK-12" {
		t.Errorf("baseLabel = %q", got)
	}
}

func TestRexStatusWithoutRecord(t *testing.T) {
	if got := (rexBlock{Argv0: "claude"}).status(); got != "idle" {
		t.Errorf("status = %q, want idle", got)
	}
	if got := (rexBlock{Records: rexRecords{{State: "error"}}}).status(); got != "blocked" {
		t.Errorf("error = %q, want blocked: a failed agent needs you", got)
	}
}

func TestWords(t *testing.T) {
	defer Use("")
	Use("rex")
	if got := Words("worktree, espace herdr, agent"); got != "worktree, session Rex, agent" {
		t.Errorf("fr = %q", got)
	}
	if got := Words("← back to herdr"); got != "← back to Rex" {
		t.Errorf("en = %q", got)
	}
	Use("herdr")
	if got := Words("its herdr workspace"); got != "its herdr workspace" {
		t.Errorf("herdr = %q", got)
	}
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{"claude": "claude", "--model": "--model", "a b": "'a b'", "it's": `'it'\''s'`} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}
