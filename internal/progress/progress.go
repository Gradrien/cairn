// Package progress measures where the code and the pull request stand: git
// in the worktree, GitHub through the gh CLI.
package progress

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"cairn/internal/i18n"
)

type Git struct {
	Ahead       int // commits on the branch, not on base
	Files       int
	Added       int
	Removed     int
	Uncommitted int    // changed or untracked paths
	Head        string // commit checked out
	LastSubject string
	LastAt      time.Time
	Err         string
}

type Checks struct {
	Passed, Failed, Pending int
	FailedNames             []string
}

type PR struct {
	URL            string
	HeadOid        string // last commit of the PR
	State          string // OPEN, MERGED, CLOSED
	Draft          bool
	ReviewDecision string // APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED, ""
	Mergeable      string // MERGEABLE, CONFLICTING, UNKNOWN
	Checks         Checks
	Comments       int
	Author         string     // who opened the PR
	Activity       []Activity // comments and reviews, oldest first
	Err            string
}

// Activity is a comment or a review left on a PR.
type Activity struct {
	Who    string
	At     time.Time
	Review bool
	State  string // review state: APPROVED, CHANGES_REQUESTED, COMMENTED
	Body   string
}

func git(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
}

var statRe = regexp.MustCompile(`(\d+) files? changed(?:, (\d+) insertions?\(\+\))?(?:, (\d+) deletions?\(-\))?`)

func ReadGit(dir, base string) Git {
	var g Git
	if out, err := git(dir, "rev-list", "--count", base+"..HEAD"); err == nil {
		g.Ahead, _ = strconv.Atoi(out)
	} else {
		g.Err = fmt.Sprintf(i18n.T("base %s introuvable", "base %s not found"), base)
	}
	if out, err := git(dir, "diff", "--shortstat", base+"...HEAD"); err == nil {
		if m := statRe.FindStringSubmatch(out); m != nil {
			g.Files, _ = strconv.Atoi(m[1])
			g.Added, _ = strconv.Atoi(m[2])
			g.Removed, _ = strconv.Atoi(m[3])
		}
	}
	if out, err := git(dir, "rev-parse", "HEAD"); err == nil {
		g.Head = out
	}
	if out, err := git(dir, "status", "--porcelain"); err == nil && out != "" {
		g.Uncommitted = len(strings.Split(out, "\n"))
	}
	if out, err := git(dir, "log", "-1", "--format=%ct%x00%s"); err == nil {
		if ts, subj, ok := strings.Cut(out, "\x00"); ok {
			if n, err := strconv.ParseInt(ts, 10, 64); err == nil {
				g.LastAt = time.Unix(n, 0)
			}
			g.LastSubject = subj
		}
	}
	return g
}

// ReadPR asks gh about a pull request URL.
func ReadPR(url string) PR {
	out, err := exec.Command("gh", "pr", "view", url, "--json",
		"state,isDraft,reviewDecision,mergeable,statusCheckRollup,comments,reviews,author,headRefOid").Output()
	if err != nil {
		return PR{Err: i18n.T("gh indisponible", "gh unavailable")}
	}
	var raw struct {
		State          string `json:"state"`
		IsDraft        bool   `json:"isDraft"`
		HeadOid        string `json:"headRefOid"`
		ReviewDecision string `json:"reviewDecision"`
		Mergeable      string `json:"mergeable"`
		Author         struct {
			Login string `json:"login"`
		} `json:"author"`
		Comments []struct {
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			Body      string    `json:"body"`
			CreatedAt time.Time `json:"createdAt"`
		} `json:"comments"`
		Reviews []struct {
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			Body        string    `json:"body"`
			State       string    `json:"state"`
			SubmittedAt time.Time `json:"submittedAt"`
		} `json:"reviews"`
		Checks []struct {
			Name       string `json:"name"`
			Context    string `json:"context"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			State      string `json:"state"`
		} `json:"statusCheckRollup"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return PR{Err: err.Error()}
	}
	pr := PR{URL: url, HeadOid: raw.HeadOid, State: raw.State, Draft: raw.IsDraft, ReviewDecision: raw.ReviewDecision,
		Mergeable: raw.Mergeable, Comments: len(raw.Comments), Author: raw.Author.Login}
	for _, c := range raw.Comments {
		pr.Activity = append(pr.Activity, Activity{Who: c.Author.Login, At: c.CreatedAt, Body: c.Body})
	}
	for _, r := range raw.Reviews {
		pr.Activity = append(pr.Activity, Activity{Who: r.Author.Login, At: r.SubmittedAt, Review: true, State: r.State, Body: r.Body})
	}
	sort.SliceStable(pr.Activity, func(i, j int) bool { return pr.Activity[i].At.Before(pr.Activity[j].At) })
	for _, c := range raw.Checks {
		result := c.Conclusion
		if result == "" {
			result = c.State // commit statuses carry state, not conclusion
		}
		name := c.Name
		if name == "" {
			name = c.Context
		}
		switch strings.ToUpper(result) {
		case "SUCCESS", "NEUTRAL":
			pr.Checks.Passed++
		case "SKIPPED", "":
			if strings.ToUpper(c.Status) != "COMPLETED" && c.Status != "" {
				pr.Checks.Pending++
			}
		case "PENDING", "EXPECTED", "QUEUED", "IN_PROGRESS":
			pr.Checks.Pending++
		default: // FAILURE, ERROR, CANCELLED, TIMED_OUT, ACTION_REQUIRED
			pr.Checks.Failed++
			pr.Checks.FailedNames = append(pr.Checks.FailedNames, name)
		}
	}
	return pr
}

// Ago is a short "il y a 2 h" / "2 h ago".
func Ago(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := Elapsed(t)
	if d == "" {
		return i18n.T("à l'instant", "just now")
	}
	return fmt.Sprintf(i18n.T("il y a %s", "%s ago"), d)
}

// Elapsed is the bare time since t: "5 min", "2 h", "3 j" / "3 d"; "" under a
// minute (or for a zero time).
func Elapsed(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return ""
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + " min"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + " h"
	default:
		return strconv.Itoa(int(d.Hours()/24)) + i18n.T(" j", " d")
	}
}
