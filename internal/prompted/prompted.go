// Package prompted remembers the prompts cairn itself typed into an agent, so
// the chat hook (cairn guard prompt) never takes them for the human's words.
package prompted

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// keep is how long a sent prompt is remembered: it reaches the agent at once,
// or once the agent finishes its turn.
const keep = 24 * time.Hour

func path(dir string) string { return filepath.Join(dir, "cache", "prompted") }

// pasteTag is the wrapper Claude Code may put around a long prompt typed in
// one go (<pasted_content id="…">…</pasted_content id="…">): what cairn sent
// is inside it.
var pasteTag = regexp.MustCompile(`</?pasted_content[^>]*>`)

func sum(text string) string {
	text = pasteTag.ReplaceAllString(text, " ")
	h := sha256.Sum256([]byte(strings.Join(strings.Fields(text), " ")))
	return hex.EncodeToString(h[:])
}

// Record notes that cairn sends text. Errors are ignored: at worst, a cairn
// prompt mentioning a push reads as a request.
func Record(dir, text string) {
	lines := recent(dir, time.Now())
	lines = append(lines, strconv.FormatInt(time.Now().Unix(), 10)+" "+sum(text))
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}
	_ = os.MkdirAll(filepath.Dir(path(dir)), 0o755)
	_ = os.WriteFile(path(dir), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// Sent reports whether cairn sent text lately.
func Sent(dir, text string) bool {
	h := sum(text)
	for _, l := range recent(dir, time.Now()) {
		if strings.HasSuffix(l, " "+h) {
			return true
		}
	}
	return false
}

func recent(dir string, now time.Time) []string {
	data, _ := os.ReadFile(path(dir))
	var out []string
	for _, l := range strings.Split(string(data), "\n") {
		at, _, ok := strings.Cut(l, " ")
		if !ok {
			continue
		}
		if t, err := strconv.ParseInt(at, 10, 64); err == nil && now.Sub(time.Unix(t, 0)) < keep {
			out = append(out, l)
		}
	}
	return out
}
