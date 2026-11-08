package tracker

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cairn/internal/config"
	"cairn/internal/i18n"
)

// Sprintosaur reads and moves issues through the Sprintosaur API.
type Sprintosaur struct {
	api, token, share string
	team, assignee    string
	web               string
	http              *http.Client
}

type spIssue struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Status        string `json:"status"`
	Priority      string `json:"priority"`
	EstimateLabel string `json:"estimateLabel"`
	PriorityValue int    `json:"priorityValue"`
	Assignee      *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"assignee"`
	PullRequests []PullRequest `json:"pullRequests"`
	Cycle        *struct {
		Number int `json:"number"`
	} `json:"cycle"`
}

func (s *Sprintosaur) issue(is spIssue) Issue {
	out := Issue{
		ID: strings.ToUpper(is.ID), Title: is.Title, Status: is.Status, PriorityValue: is.PriorityValue,
		Estimate: is.EstimateLabel, Assigned: is.Assignee != nil, Done: is.Status == "Done",
		Started:      is.Status != "" && is.Status != "Backlog" && is.Status != "Todo",
		PullRequests: is.PullRequests,
	}
	if is.Priority != "No priority" {
		out.Priority = is.Priority
	}
	if is.Cycle != nil {
		out.Cycle = is.Cycle.Number
	}
	if s.web != "" && out.ID != "" {
		out.URL = s.web + out.ID
	}
	return out
}

// NewSprintosaur reads the credentials file (SPRINTOSAUR_API, _TOKEN and
// _SHARE_TOKEN); the environment variables of the same name win.
func NewSprintosaur(p config.Project) (*Sprintosaur, error) {
	path := p.Credentials
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".claude", "skills", "sprintosaur", "credentials.env")
	}
	vals := readEnvFile(path)
	for _, k := range []string{"SPRINTOSAUR_API", "SPRINTOSAUR_TOKEN", "SPRINTOSAUR_SHARE_TOKEN"} {
		if v := os.Getenv(k); v != "" {
			vals[k] = v
		}
	}
	web := p.Web
	if web == "" {
		web = "https://sprintosaur.preview.spinup.yalink.fr/issue/"
	}
	s := &Sprintosaur{
		api:   strings.TrimRight(vals["SPRINTOSAUR_API"], "/"),
		token: vals["SPRINTOSAUR_TOKEN"],
		share: vals["SPRINTOSAUR_SHARE_TOKEN"],
		team:  p.Key, assignee: p.Assignee, web: web,
		http: &http.Client{Timeout: 15 * time.Second},
	}
	if s.api == "" || s.token == "" {
		return nil, fmt.Errorf(i18n.T("identifiants Sprintosaur introuvables (%s, ou variables SPRINTOSAUR_API et SPRINTOSAUR_TOKEN)",
			"Sprintosaur credentials not found (%s, or variables SPRINTOSAUR_API and SPRINTOSAUR_TOKEN)"), path)
	}
	return s, nil
}

func (s *Sprintosaur) Name() string { return "Sprintosaur" }

func (s *Sprintosaur) Guide(key string) string {
	return i18n.T("Lis le ticket et ses commentaires via le skill sprintosaur", "Read the ticket and its comments with the sprintosaur skill")
}

func (s *Sprintosaur) get(path string, q url.Values, out any) error {
	if q == nil {
		q = url.Values{}
	}
	if s.share != "" {
		q.Set("__sp_pk", s.share)
	}
	req, err := http.NewRequest(http.MethodGet, s.api+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(i18n.T("sprintosaur %s : HTTP %d", "sprintosaur %s: HTTP %d"), path, resp.StatusCode)
	}
	return json.Unmarshal(body, out)
}

func (s *Sprintosaur) patch(key, body string) error {
	q := url.Values{}
	if s.share != "" {
		q.Set("__sp_pk", s.share)
	}
	req, err := http.NewRequest(http.MethodPatch, s.api+"/issues/"+url.PathEscape(key)+"?"+q.Encode(), strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf(i18n.T("sprintosaur %s : HTTP %d", "sprintosaur %s: HTTP %d"), key, resp.StatusCode)
	}
	return nil
}

// cycle lists the team's open issues of the running cycle, assigned to
// assignee when not empty.
func (s *Sprintosaur) cycle(assignee string) ([]Issue, error) {
	q := url.Values{"team": {s.team}, "cycle": {"current"}, "state": {"open"}, "limit": {"100"}}
	if assignee != "" {
		q.Set("assignee", assignee)
	}
	var res struct {
		Issues []spIssue `json:"issues"`
	}
	if err := s.get("/issues", q, &res); err != nil {
		return nil, err
	}
	out := make([]Issue, 0, len(res.Issues))
	for _, is := range res.Issues {
		out = append(out, s.issue(is))
	}
	return out, nil
}

func (s *Sprintosaur) Mine() ([]Issue, error) { return s.cycle(s.assignee) }

func (s *Sprintosaur) Free() ([]Issue, error) {
	all, err := s.cycle("")
	var out []Issue
	for _, is := range all {
		if !is.Assigned {
			out = append(out, is)
		}
	}
	return out, err
}

func (s *Sprintosaur) Get(key string) (Issue, error) {
	var res spIssue
	err := s.get("/issues/"+url.PathEscape(key), nil, &res)
	if err == nil && res.ID == "" {
		// Some endpoints wrap the issue.
		var wrapped struct {
			Issue spIssue `json:"issue"`
		}
		if err2 := s.get("/issues/"+url.PathEscape(key), nil, &wrapped); err2 == nil {
			res = wrapped.Issue
		}
	}
	return s.issue(res), err
}

func (s *Sprintosaur) Take(key string) error {
	if s.assignee == "" {
		return nil
	}
	return s.patch(key, `{"assignee":`+strconv.Quote(s.assignee)+`}`)
}

func (s *Sprintosaur) Start(is Issue) (bool, error) {
	if is.Started || is.Status == "" {
		return false, nil
	}
	return true, s.patch(is.ID, `{"status":"In Progress"}`)
}
