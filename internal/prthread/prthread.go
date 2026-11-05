// Package prthread reads a pull request's conversation through gh — the
// description, comments, reviews and inline review comments — as plain text,
// with every image pulled out and numbered.
package prthread

import (
	"encoding/json"
	"fmt"
	"html"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Entry struct {
	Author string
	At     time.Time
	Kind   string // "description", "commentaire", "review", "inline"
	State  string // review state: APPROVED, CHANGES_REQUESTED, COMMENTED
	Path   string // inline comments: file and line
	Line   int
	Body   string // plain text; images appear as "[image N]"
	Bot    bool
}

type Thread struct {
	Owner, Repo string
	Number      int
	HeadRef     string
	Entries     []Entry
	Images      []string     // URLs or repo-relative paths, numbered from 1 in bodies
	Broken      map[int]bool // 1-based numbers of images that cannot be fetched
}

// Relative reports whether src is a repo-relative path (e.g. "./before.png"):
// GitHub resolves those against /pull/<n>/, so in a PR they are dead links
// unless the file happens to exist on the branch.
func Relative(src string) bool {
	return !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") && !strings.HasPrefix(src, "/")
}

var prURL = regexp.MustCompile(`github\.com/([^/]+)/([^/]+)/pull/(\d+)`)

func gh(args ...string) ([]byte, error) {
	out, err := exec.Command("gh", args...).Output()
	if ee, ok := err.(*exec.ExitError); ok {
		return nil, fmt.Errorf("gh : %s", strings.TrimSpace(string(ee.Stderr)))
	}
	return out, err
}

func Fetch(url string) (*Thread, error) {
	m := prURL.FindStringSubmatch(url)
	if m == nil {
		return nil, fmt.Errorf("URL de PR inattendue : %s", url)
	}
	t := &Thread{Owner: m[1], Repo: m[2]}
	fmt.Sscan(m[3], &t.Number)

	out, err := gh("pr", "view", url, "--json", "author,body,createdAt,headRefName,comments,reviews")
	if err != nil {
		return nil, err
	}
	type author struct {
		Login string `json:"login"`
	}
	var pr struct {
		Author    author    `json:"author"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"createdAt"`
		HeadRef   string    `json:"headRefName"`
		Comments  []struct {
			Author    author    `json:"author"`
			Body      string    `json:"body"`
			CreatedAt time.Time `json:"createdAt"`
		} `json:"comments"`
		Reviews []struct {
			Author      author    `json:"author"`
			Body        string    `json:"body"`
			State       string    `json:"state"`
			SubmittedAt time.Time `json:"submittedAt"`
		} `json:"reviews"`
	}
	if err := json.Unmarshal(out, &pr); err != nil {
		return nil, err
	}
	t.HeadRef = pr.HeadRef

	var raw []Entry
	raw = append(raw, Entry{Author: pr.Author.Login, At: pr.CreatedAt, Kind: "description", Body: pr.Body})
	for _, c := range pr.Comments {
		raw = append(raw, Entry{Author: c.Author.Login, At: c.CreatedAt, Kind: "commentaire", Body: c.Body})
	}
	for _, r := range pr.Reviews {
		if strings.TrimSpace(r.Body) == "" && r.State == "COMMENTED" {
			continue // the inline comments carry the content
		}
		raw = append(raw, Entry{Author: r.Author.Login, At: r.SubmittedAt, Kind: "review", State: r.State, Body: r.Body})
	}
	if out, err := gh("api", "--paginate", fmt.Sprintf("repos/%s/%s/pulls/%d/comments", t.Owner, t.Repo, t.Number)); err == nil {
		var inline []struct {
			User         author    `json:"user"`
			Body         string    `json:"body"`
			Path         string    `json:"path"`
			Line         int       `json:"line"`
			OriginalLine int       `json:"original_line"`
			CreatedAt    time.Time `json:"created_at"`
		}
		// --paginate concatenates JSON arrays: "][" between pages.
		fixed := strings.ReplaceAll(strings.TrimSpace(string(out)), "][", ",")
		if json.Unmarshal([]byte(fixed), &inline) == nil {
			for _, c := range inline {
				line := c.Line
				if line == 0 {
					line = c.OriginalLine
				}
				raw = append(raw, Entry{Author: c.User.Login, At: c.CreatedAt, Kind: "inline", Path: c.Path, Line: line, Body: c.Body})
			}
		}
	}
	// The description first, then the conversation in time order.
	sort.SliceStable(raw[1:], func(i, j int) bool { return raw[i+1].At.Before(raw[j+1].At) })
	for _, e := range raw {
		e.Bot = isBot(e.Author)
		e.Body = t.clean(e.Body)
		if e.Body == "" && e.Kind != "review" {
			continue
		}
		t.Entries = append(t.Entries, e)
	}
	return t, nil
}

// IsBot tells automated accounts (CI, linters, deploy previews) from people.
func IsBot(login string) bool { return isBot(login) }

func isBot(login string) bool {
	l := strings.ToLower(login)
	for _, b := range []string{"[bot]", "-bot", "github-actions", "copilot", "linear", "vercel", "codecov", "sonar", "dependabot", "renovate"} {
		if strings.Contains(l, b) {
			return true
		}
	}
	return false
}

var (
	mdImage     = regexp.MustCompile(`!\[[^\]]*\]\(\s*<?([^)\s>]+)>?(?:\s+"[^"]*")?\s*\)`)
	htmlImage   = regexp.MustCompile(`(?i)<img[^>]*?src=["']([^"']+)["'][^>]*>`)
	comments    = regexp.MustCompile(`(?s)<!--.*?-->`)
	breaks      = regexp.MustCompile(`(?i)<br\s*/?>|</(p|div|tr|li|h[1-6]|summary|details|table|thead|tbody|ul|ol|pre|blockquote)>`)
	cells       = regexp.MustCompile(`(?i)</?t[dh][^>]*>`)
	items       = regexp.MustCompile(`(?i)<li[^>]*>`)
	tags        = regexp.MustCompile(`</?[a-zA-Z][a-zA-Z0-9-]*(?:\s[^<>]*)?/?>`)
	fence       = regexp.MustCompile("(?ms)^[ \t]*```.*?^[ \t]*```[ \t]*$|^[ \t]*~~~.*?^[ \t]*~~~[ \t]*$")
	inline      = regexp.MustCompile("`[^`\n]+`")
	blank       = regexp.MustCompile(`\n{3,}`)
	spaces      = regexp.MustCompile(`[ \t]{2,}`)
	placeholder = regexp.MustCompile("\x00\\d+\x00")
)

// clean turns markdown/HTML into readable text and numbers images.
func (t *Thread) clean(body string) string {
	number := func(src string) string {
		for i, u := range t.Images {
			if u == src {
				return fmt.Sprintf("[image %d]", i+1)
			}
		}
		t.Images = append(t.Images, src)
		return fmt.Sprintf("[image %d]", len(t.Images))
	}
	s := strings.ReplaceAll(body, "\r\n", "\n")
	// Code is shown as is: set it aside so the HTML cleanup cannot eat a
	// heredoc or a comparison ("<<'SQL' … > 70").
	var kept []string
	keep := func(m string) string {
		kept = append(kept, m)
		return fmt.Sprintf("\x00%d\x00", len(kept)-1)
	}
	s = fence.ReplaceAllStringFunc(s, keep)
	s = inline.ReplaceAllStringFunc(s, keep)
	s = comments.ReplaceAllString(s, "")
	s = mdImage.ReplaceAllStringFunc(s, func(m string) string { return number(mdImage.FindStringSubmatch(m)[1]) })
	s = htmlImage.ReplaceAllStringFunc(s, func(m string) string { return number(htmlImage.FindStringSubmatch(m)[1]) })
	s = breaks.ReplaceAllString(s, "\n")
	s = items.ReplaceAllString(s, "• ")
	s = cells.ReplaceAllString(s, "  ")
	s = tags.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(spaces.ReplaceAllString(l, "  "), " ")
	}
	s = blank.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
	s = placeholder.ReplaceAllStringFunc(s, func(m string) string {
		var i int
		fmt.Sscan(strings.Trim(m, "\x00"), &i)
		return kept[i]
	})
	return strings.TrimSpace(s)
}

// ImageRe finds "[image N]" markers in cleaned bodies.
var ImageRe = regexp.MustCompile(`\[image (\d+)\]`)
