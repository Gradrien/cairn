// Package journal keeps each chantier's history next to its plan:
// ~/.cairn/chantiers/<KEY>.log, one "RFC3339<TAB>event" line per event.
// Events are "phase:<Phase>" when cairn sees the phase change, and human
// decisions such as "plan validé" or "push autorisé".
package journal

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Event struct {
	At   time.Time
	Text string
}

func path(dir, key string) string { return filepath.Join(dir, strings.ToUpper(key)+".log") }

func Append(dir, key, text string) error {
	f, err := os.OpenFile(path(dir, key), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(time.Now().Format(time.RFC3339) + "\t" + text + "\n")
	return err
}

func Read(dir, key string) []Event {
	f, err := os.Open(path(dir, key))
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []Event
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		ts, text, ok := strings.Cut(sc.Text(), "\t")
		if !ok {
			continue
		}
		at, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			continue
		}
		out = append(out, Event{At: at, Text: text})
	}
	return out
}

// LastPhase returns the last recorded phase and when it started.
func LastPhase(events []Event) (string, time.Time) {
	for i := len(events) - 1; i >= 0; i-- {
		if p, ok := strings.CutPrefix(events[i].Text, "phase:"); ok {
			return p, events[i].At
		}
	}
	return "", time.Time{}
}

// PhaseSince is when the current phase started, if cairn saw it start: the
// first phase recorded is only when cairn first looked, not a real start.
func PhaseSince(events []Event) time.Time {
	n := 0
	var at time.Time
	for _, e := range events {
		if strings.HasPrefix(e.Text, "phase:") {
			n++
			at = e.At
		}
	}
	if n < 2 {
		return time.Time{}
	}
	return at
}

// Archive moves a chantier's plan and journal to dir/../archive.
func Archive(dir, key string) error {
	arch := filepath.Join(filepath.Dir(dir), "archive")
	if err := os.MkdirAll(arch, 0o755); err != nil {
		return err
	}
	for _, ext := range []string{".md", ".log"} {
		src := filepath.Join(dir, strings.ToUpper(key)+ext)
		if _, err := os.Stat(src); err == nil {
			dst := filepath.Join(arch, strings.ToUpper(key)+"-"+time.Now().Format("20060102-150405")+ext)
			if err := os.Rename(src, dst); err != nil {
				return err
			}
		}
	}
	return nil
}
