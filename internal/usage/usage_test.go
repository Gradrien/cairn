package usage

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func line(id, model, cwd, branch, at string, in, out, read, write int, tool string) string {
	content := `{"type":"text","text":"ok"}`
	if tool != "" {
		content = fmt.Sprintf(`{"type":"tool_use","name":%q,"input":{"file_path":"/w/x/%s"}}`, tool, "detail.go")
	}
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"cwd":%q,"gitBranch":%q,"message":{"id":%q,"model":%q,"content":[%s],"usage":{"input_tokens":%d,"output_tokens":%d,"cache_read_input_tokens":%d,"cache_creation_input_tokens":%d,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":%d}}}}`+"\n",
		at, cwd, branch, id, model, content, in, out, read, write, write)
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRead(t *testing.T) {
	root := t.TempDir()
	wt := "/w/yak-861"
	dir := filepath.Join(root, dirName(wt))
	main := filepath.Join(dir, "s1.jsonl")
	write(t, main,
		`{"type":"api-request","params":{"model":"claude-opus-5-5"},"message":{"role":"assistant","usage":{}}}`+"\n"+
			line("m1", "claude-opus-5-5", wt, "yak-861", "2026-11-10T10:00:00Z", 10, 100, 1000, 0, "")+
			// One message streamed as two lines: it counts once, with its last usage.
			line("m2", "claude-opus-5-5", wt, "yak-861", "2026-11-10T10:01:00Z", 2, 5, 2000, 500, "Edit")+
			line("m2", "claude-opus-5-5", wt, "yak-861", "2026-11-10T10:01:00Z", 2, 50, 2000, 500, "")+
			line("m3", "claude-opus-5-5", "/elsewhere", "main", "2026-11-10T10:02:00Z", 1, 1, 1, 1, ""))
	sub := filepath.Join(dir, "s1", "subagents", "agent-a1.jsonl")
	write(t, sub, line("s1", "claude-haiku-5-5", wt+"/api", "yak-861", "2026-11-10T10:03:00Z", 5, 200, 30000, 0, "Grep"))
	write(t, filepath.Join(dir, "s1", "subagents", "agent-a1.meta.json"), `{"agentType":"cairn-explorer","description":"Effets de bord","finished":true}`)
	// A sibling worktree whose name starts the same way is not this chantier.
	write(t, filepath.Join(root, dirName(wt+"-bis"), "s2.jsonl"), line("x", "claude-opus-5-5", wt+"-bis", "b", "2026-11-10T10:00:00Z", 9, 9, 9, 9, ""))

	rep := Read(root, wt, "")
	if rep.Sessions != 1 || rep.Subs != 1 || len(rep.Entries) != 3 {
		t.Fatalf("sessions %d subs %d entries %d", rep.Sessions, rep.Subs, len(rep.Entries))
	}
	s := Summarize(rep.Entries, DefaultPrices, time.Time{}, time.Time{})
	if s.Tokens.Output != 350 || s.Tokens.CacheRead != 33000 || s.Tokens.CacheWrite1h != 500 {
		t.Errorf("tokens %+v", s.Tokens)
	}
	want := (10*4+100*20+1000*0.2)/1e6 + (2*4+50*20+2000*0.2+500*8)/1e6 + (5*0.1+200*0.5+30000*0.01)/1e6
	if math.Abs(s.Cost-want) > 1e-9 {
		t.Errorf("cost %f, want %f", s.Cost, want)
	}
	if len(s.ByAgent) != 2 || s.ByAgent[0].Name != Main || s.ByAgent[1].Name != "cairn-explorer" {
		t.Errorf("by agent %+v", s.ByAgent)
	}
	if len(rep.Last) != 2 || rep.Last[0].Agent != "cairn-explorer" || rep.Last[0].Tool != "Grep" || rep.Last[1].Tool != "Edit" || rep.Last[1].Target != "detail.go" {
		t.Errorf("last actions %+v", rep.Last)
	}

	if len(rep.Subagent) != 1 || rep.Subagent[0].Desc != "Effets de bord" || !rep.Subagent[0].Finished || rep.Subagent[0].Model != "claude-haiku-5-5" {
		t.Errorf("subagents %+v", rep.Subagent)
	}
	if len(rep.Recent) != 2 || rep.Recent[0].Tool != "Grep" {
		t.Errorf("recent %+v", rep.Recent)
	}

	// A partial line is left for later; appended lines are read incrementally.
	f, _ := os.OpenFile(main, os.O_APPEND|os.O_WRONLY, 0o644)
	full := line("m4", "claude-sonnet-5-5", wt, "yak-861", "2026-11-10T11:00:00Z", 1, 1000, 0, 0, "")
	f.WriteString(full[:40])
	f.Close()
	if n := len(Read(root, wt, "").Entries); n != 3 {
		t.Errorf("partial line counted: %d", n)
	}
	f, _ = os.OpenFile(main, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString(full[40:])
	f.Close()
	if n := len(Read(root, wt, "").Entries); n != 4 {
		t.Errorf("appended line missed: %d", n)
	}

	// On the main checkout, only the chantier's branch counts.
	if n := len(Read(root, wt, "other").Entries); n != 0 {
		t.Errorf("branch filter: %d", n)
	}
}

func TestPrices(t *testing.T) {
	if p, ok := DefaultPrices.For("claude-sonnet-5-5"); !ok || p.Output != 10 {
		t.Errorf("sonnet 5.5: %+v", p)
	}
	if p, _ := DefaultPrices.For("claude-sonnet-5"); p.Output != 10 || p.CacheRead != 0.2 {
		t.Errorf("sonnet 5 must not take 5.5's cache rate: %+v", p)
	}
	if _, ok := DefaultPrices.For("gpt-5"); ok {
		t.Error("unknown model priced")
	}
	custom := DefaultPrices.With(map[string]Price{"claude-opus-5-5": {Input: 1}})
	if p, _ := custom.For("claude-opus-5-5"); p.Input != 1 {
		t.Error("config overrides defaults")
	}
	for in, want := range map[string]string{"claude-opus-5-5": "opus 5.5", "claude-haiku-4-5-20251001": "haiku 4.5", "claude-fable-5": "fable 5"} {
		if got := ShortModel(in); got != want {
			t.Errorf("ShortModel(%s) = %s", in, got)
		}
	}
}
