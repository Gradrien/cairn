// Package usage reads what the agents of a chantier cost, from Claude Code's
// own transcripts: ~/.claude/projects/<cwd as a dir name>/<session>.jsonl,
// and <session>/subagents/agent-*.jsonl (with a .meta.json naming the
// subagent's type) for the subagents it started.
//
// Every assistant message carries its model and its usage (input, output,
// cache reads and writes). A message streamed as several lines repeats its id:
// it counts once. Files are read incrementally: only what was appended since
// the last read is parsed.
package usage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Tokens are counted per kind, each billed at its own rate.
type Tokens struct {
	Input, Output, CacheRead, CacheWrite5m, CacheWrite1h int64
}

func (t Tokens) Add(o Tokens) Tokens {
	return Tokens{t.Input + o.Input, t.Output + o.Output, t.CacheRead + o.CacheRead,
		t.CacheWrite5m + o.CacheWrite5m, t.CacheWrite1h + o.CacheWrite1h}
}

// Total is every token, whatever its rate.
func (t Tokens) Total() int64 {
	return t.Input + t.Output + t.CacheRead + t.CacheWrite5m + t.CacheWrite1h
}

// Main is the Agent of the main conversation; a subagent is named by its type
// (Explore, general-purpose, cairn-explorer…).
const Main = "main"

// Entry is one assistant message.
type Entry struct {
	At     time.Time
	Model  string
	Agent  string
	Tokens Tokens
}

// Action is the last thing an agent did: a tool and what it touched.
type Action struct {
	At     time.Time
	Agent  string
	Tool   string
	Target string
}

// Report is what the agents of one chantier consumed.
type Report struct {
	Entries []Entry  // oldest first
	Last    []Action // the last action of each agent, latest first
	// Context is what the main agent re-reads at each turn now: the size of
	// the last message of the latest session, cache included.
	Context   int64
	ContextAt time.Time
	Sessions  int
	Subs      int // subagents started
}

// Dir is where Claude Code keeps transcripts.
func Dir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, "projects")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]`)

// dirName is the directory Claude Code names after a working directory.
func dirName(cwd string) string { return nonAlnum.ReplaceAllString(cwd, "-") }

// Read gathers the usage of the agents that worked in worktree (or below
// it). When branch is set, only messages written on that branch count: a
// chantier on the main checkout shares its directory with other work.
func Read(root, worktree, branch string) Report {
	var rep Report
	if worktree == "" {
		return rep
	}
	worktree = filepath.Clean(worktree)
	enc := dirName(worktree)
	dirs, _ := os.ReadDir(root)
	last := map[string]Action{}
	for _, d := range dirs {
		if !d.IsDir() || (d.Name() != enc && !strings.HasPrefix(d.Name(), enc+"-")) {
			continue
		}
		dir := filepath.Join(root, d.Name())
		sessions, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
		for _, s := range sessions {
			f := cached(s)
			if !f.keep(worktree, branch) {
				continue
			}
			rep.Sessions++
			rep.add(f, Main, worktree, branch, last)
			for _, m := range f.msgs {
				if !m.sidechain && inside(m.cwd, worktree) && (branch == "" || m.branch == "" || m.branch == branch) && m.at.After(rep.ContextAt) {
					t := m.tokens
					rep.Context, rep.ContextAt = t.Input+t.CacheRead+t.CacheWrite5m+t.CacheWrite1h, m.at
				}
			}
			subs, _ := filepath.Glob(filepath.Join(strings.TrimSuffix(s, ".jsonl"), "subagents", "agent-*.jsonl"))
			for _, sp := range subs {
				sf := cached(sp)
				if !sf.keep(worktree, branch) {
					continue
				}
				rep.Subs++
				rep.add(sf, subType(sp), worktree, branch, last)
			}
		}
	}
	sort.SliceStable(rep.Entries, func(i, j int) bool { return rep.Entries[i].At.Before(rep.Entries[j].At) })
	for _, a := range last {
		rep.Last = append(rep.Last, a)
	}
	sort.Slice(rep.Last, func(i, j int) bool { return rep.Last[i].At.After(rep.Last[j].At) })
	return rep
}

func (r *Report) add(f *file, agent, worktree, branch string, last map[string]Action) {
	for _, m := range f.msgs {
		if !inside(m.cwd, worktree) || (branch != "" && m.branch != "" && m.branch != branch) {
			continue
		}
		who := agent
		if m.sidechain && who == Main {
			who = "subagent" // older transcripts kept subagents inline
		}
		r.Entries = append(r.Entries, Entry{m.at, m.model, who, m.tokens})
		if m.tool != "" {
			if a, ok := last[who]; !ok || m.at.After(a.At) {
				last[who] = Action{m.at, who, m.tool, m.target}
			}
		}
	}
}

func inside(cwd, worktree string) bool {
	return cwd == "" || cwd == worktree || strings.HasPrefix(cwd, worktree+string(filepath.Separator))
}

// subType reads the subagent's type from the .meta.json next to its transcript.
func subType(transcript string) string {
	var meta struct {
		AgentType string `json:"agentType"`
	}
	data, err := os.ReadFile(strings.TrimSuffix(transcript, ".jsonl") + ".meta.json")
	if err == nil && json.Unmarshal(data, &meta) == nil && meta.AgentType != "" {
		return meta.AgentType
	}
	return "subagent"
}

// ── incremental parsing ─────────────────────────────────────────────────────

type msg struct {
	id, model, cwd, branch string
	at                     time.Time
	sidechain              bool
	tokens                 Tokens
	tool, target           string // the message's last tool call
}

type file struct {
	size   int64
	mod    time.Time
	offset int64
	msgs   []msg
	byID   map[string]int
}

// keep: the file has at least one message from the worktree and branch.
func (f *file) keep(worktree, branch string) bool {
	for _, m := range f.msgs {
		if inside(m.cwd, worktree) && (branch == "" || m.branch == "" || m.branch == branch) {
			return true
		}
	}
	return false
}

var (
	mu    sync.Mutex
	files = map[string]*file{}
)

// cached returns the parsed file, reading only what was appended since last time.
func cached(path string) *file {
	mu.Lock()
	defer mu.Unlock()
	st, err := os.Stat(path)
	if err != nil {
		delete(files, path)
		return &file{}
	}
	f := files[path]
	if f == nil || st.Size() < f.offset {
		f = &file{byID: map[string]int{}} // new, or rewritten
		files[path] = f
	}
	if st.Size() == f.size && st.ModTime().Equal(f.mod) {
		return f
	}
	if fh, err := os.Open(path); err == nil {
		if _, err := fh.Seek(f.offset, io.SeekStart); err == nil {
			f.offset += parse(fh, f)
		}
		fh.Close()
	}
	f.size, f.mod = st.Size(), st.ModTime()
	return f
}

var (
	assistantMark = []byte(`"assistant"`)
	usageMark     = []byte(`"usage"`)
)

// parse reads whole lines from r into f and returns how many bytes it used:
// a line still being written is left for the next read.
func parse(r io.Reader, f *file) int64 {
	br := bufio.NewReaderSize(r, 1<<16)
	var used int64
	for {
		line, err := br.ReadBytes('\n')
		if err != nil {
			return used // EOF: a partial line is read again next time
		}
		used += int64(len(line))
		// Request dumps are huge and never hold usage: skip them unparsed.
		if !bytes.Contains(line, assistantMark) || !bytes.Contains(line, usageMark) {
			continue
		}
		var l struct {
			Type      string    `json:"type"`
			Timestamp time.Time `json:"timestamp"`
			Cwd       string    `json:"cwd"`
			GitBranch string    `json:"gitBranch"`
			Sidechain bool      `json:"isSidechain"`
			Message   struct {
				ID      string `json:"id"`
				Model   string `json:"model"`
				Content []struct {
					Type  string          `json:"type"`
					Name  string          `json:"name"`
					Input json.RawMessage `json:"input"`
				} `json:"content"`
				Usage *struct {
					Input         int64 `json:"input_tokens"`
					Output        int64 `json:"output_tokens"`
					CacheRead     int64 `json:"cache_read_input_tokens"`
					CacheCreation int64 `json:"cache_creation_input_tokens"`
					Split         *struct {
						M5 int64 `json:"ephemeral_5m_input_tokens"`
						H1 int64 `json:"ephemeral_1h_input_tokens"`
					} `json:"cache_creation"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &l) != nil || l.Type != "assistant" || l.Message.Usage == nil {
			continue
		}
		u := l.Message.Usage
		t := Tokens{Input: u.Input, Output: u.Output, CacheRead: u.CacheRead, CacheWrite5m: u.CacheCreation}
		if u.Split != nil && u.Split.M5+u.Split.H1 == u.CacheCreation {
			t.CacheWrite5m, t.CacheWrite1h = u.Split.M5, u.Split.H1
		}
		m := msg{id: l.Message.ID, model: l.Message.Model, cwd: l.Cwd, branch: l.GitBranch,
			at: l.Timestamp, sidechain: l.Sidechain, tokens: t}
		for _, c := range l.Message.Content {
			if c.Type == "tool_use" {
				m.tool, m.target = c.Name, target(c.Input)
			}
		}
		if i, ok := f.byID[m.id]; ok && m.id != "" {
			// The same message, one more block: newer usage, keep its tool.
			if m.tool == "" {
				m.tool, m.target = f.msgs[i].tool, f.msgs[i].target
			}
			f.msgs[i] = m
			continue
		}
		if m.model == "<synthetic>" {
			continue
		}
		f.byID[m.id] = len(f.msgs)
		f.msgs = append(f.msgs, m)
	}
}

// target is the short thing a tool call is about: a file, a command, a pattern.
func target(input json.RawMessage) string {
	var in map[string]any
	if json.Unmarshal(input, &in) != nil {
		return ""
	}
	for _, k := range []string{"file_path", "path", "description", "command", "pattern", "prompt", "query", "url"} {
		if s, ok := in[k].(string); ok && s != "" {
			if k == "file_path" || k == "path" {
				return filepath.Base(s)
			}
			s = strings.Join(strings.Fields(s), " ")
			if r := []rune(s); len(r) > 60 {
				s = string(r[:59]) + "…"
			}
			return s
		}
	}
	return ""
}
