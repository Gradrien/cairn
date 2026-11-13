// Package native is how a native window (the Herdr app's cairn panels) talks to
// the cairn engine: the engine writes what to draw under ~/.cairn/cache, and
// the window drops requests there with `cairn do`, which the engine runs as
// the keys and clicks they stand for.
package native

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"cairn/internal/config"
)

func cache() string { return filepath.Join(config.Dir(), "cache") }

func BoardFile() string   { return filepath.Join(cache(), "board.json") }
func DetailFile() string  { return filepath.Join(cache(), "detail.json") }
func RequestsDir() string { return filepath.Join(cache(), "requests") }
func lockFile() string    { return filepath.Join(cache(), "engine.lock") }

// Request is what the native window asks for. Key selects the chantier first
// ("" or "-" keeps the selection); Action is then one of:
//   - "key": Arg is a TUI key ("a", "v", "enter", "esc", "up"…), run as if pressed;
//   - "confirm" / "cancel": the open confirmation, as enter / esc;
//   - "overlay": Arg is a key of the open confirmation's buttons;
//   - "answer": open question Index, with option Option, or Text in your words;
//   - "side": tick or untick side effect Index;
//   - "manual-ok" / "manual-ko": manual step Index works, or fails as Text says;
//   - "fold": unfold or fold section Arg ("tasks", "goal", "approach");
//   - "comment": send entry Index of the PR conversation to the agent;
//   - "ask": Text is a question about how the ticket is built, for an agent aside.
type Request struct {
	Key    string `json:"key"`
	Action string `json:"action"`
	Arg    string `json:"arg,omitempty"`
	Index  int    `json:"index,omitempty"`
	Option *int   `json:"option,omitempty"`
	Text   string `json:"text,omitempty"`
}

// Send drops a request; the engine runs it within 100 ms.
func Send(r Request) error {
	if r.Key == "-" {
		r.Key = ""
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(RequestsDir(), 0o755); err != nil {
		return err
	}
	name := fmt.Sprintf("%020d-%d.json", time.Now().UnixNano(), os.Getpid())
	tmp := filepath.Join(RequestsDir(), "."+name)
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(RequestsDir(), name))
}

// Take returns the waiting requests, oldest first, and removes them.
func Take() []Request {
	entries, err := os.ReadDir(RequestsDir())
	if err != nil {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var out []Request
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(RequestsDir(), e.Name())
		data, err := os.ReadFile(path)
		_ = os.Remove(path)
		var r Request
		if err == nil && json.Unmarshal(data, &r) == nil {
			out = append(out, r)
		}
	}
	return out
}

// Lock makes this process the engine. It fails while another engine runs;
// the lock goes with the process.
func Lock() (*os.File, error) {
	if err := os.MkdirAll(cache(), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(lockFile(), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another cairn engine runs")
	}
	_ = f.Truncate(0)
	_, _ = f.WriteString(fmt.Sprint(os.Getpid()))
	return f, nil
}

// Running reports whether an engine holds the lock.
func Running() bool {
	f, err := Lock()
	if err != nil {
		return true
	}
	f.Close()
	return false
}
