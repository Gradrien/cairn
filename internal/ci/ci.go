// Package ci gathers the logs of a pull request's failing checks, so an agent
// can fix the CI without anyone opening GitHub.
package ci

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"cairn/internal/i18n"
)

var jobRe = regexp.MustCompile(`github\.com/([^/]+/[^/]+)/actions/runs/\d+/job/(\d+)`)

// tail keeps the last n lines: the error is almost always at the end.
func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = append([]string{fmt.Sprintf(i18n.T("… (%d lignes plus haut coupées)", "… (%d earlier lines cut)"), len(lines)-n)}, lines[len(lines)-n:]...)
	}
	return strings.Join(lines, "\n")
}

// FailedLogs writes the failing checks' logs to a file in dir and returns its
// path and the failing check names.
func FailedLogs(prURL, dir, key string) (string, []string, error) {
	out, err := exec.Command("gh", "pr", "checks", prURL, "--json", "name,bucket,link").Output()
	if err != nil && len(out) == 0 { // gh exits non-zero when checks fail
		return "", nil, fmt.Errorf(i18n.T("gh pr checks : %v", "gh pr checks: %v"), err)
	}
	var checks []struct{ Name, Bucket, Link string }
	if err := json.Unmarshal(out, &checks); err != nil {
		return "", nil, err
	}
	var b strings.Builder
	var names []string
	for _, c := range checks {
		if c.Bucket != "fail" {
			continue
		}
		names = append(names, c.Name)
		fmt.Fprintf(&b, "===== %s =====\n%s\n\n", c.Name, c.Link)
		m := jobRe.FindStringSubmatch(c.Link)
		if m == nil {
			b.WriteString(i18n.T("(pas un job GitHub Actions : voir le lien)\n\n", "(not a GitHub Actions job: see the link)\n\n"))
			continue
		}
		logs, err := exec.Command("gh", "run", "view", "--job", m[2], "--log-failed", "-R", m[1]).Output()
		if err != nil || len(logs) == 0 {
			b.WriteString(i18n.T("(logs indisponibles)\n\n", "(logs unavailable)\n\n"))
			continue
		}
		b.WriteString(tail(string(logs), 250) + "\n\n")
	}
	if len(names) == 0 {
		return "", nil, fmt.Errorf("%s", i18n.T("aucun check en échec", "no failing check"))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.log", strings.ToUpper(key), time.Now().Format("20060102-150405")))
	return path, names, os.WriteFile(path, []byte(b.String()), 0o644)
}
