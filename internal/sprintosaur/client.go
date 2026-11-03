// Package sprintosaur reads issues from the Sprintosaur API.
package sprintosaur

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type PullRequest struct {
	URL    string `json:"url"`
	Number int    `json:"number"`
	State  string `json:"state"`
	Branch string `json:"branch"`
}

type Issue struct {
	ID            string        `json:"id"`
	Title         string        `json:"title"`
	Status        string        `json:"status"`
	Priority      string        `json:"priority"`
	EstimateLabel string        `json:"estimateLabel"`
	PullRequests  []PullRequest `json:"pullRequests"`
	Cycle         *struct {
		Number int `json:"number"`
	} `json:"cycle"`
}

type Client struct {
	api, token, share string
	http              *http.Client
}

// New reads a KEY=VALUE credentials file (shell style, optional export/quotes).
func New(credentialsPath string) (*Client, error) {
	f, err := os.Open(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("identifiants Sprintosaur : %w", err)
	}
	defer f.Close()
	vals := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(line, "#") {
			continue
		}
		vals[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	c := &Client{
		api:   strings.TrimRight(vals["SPRINTOSAUR_API"], "/"),
		token: vals["SPRINTOSAUR_TOKEN"],
		share: vals["SPRINTOSAUR_SHARE_TOKEN"],
		http:  &http.Client{Timeout: 15 * time.Second},
	}
	if c.api == "" || c.token == "" {
		return nil, fmt.Errorf("identifiants Sprintosaur incomplets dans %s", credentialsPath)
	}
	return c, nil
}

func (c *Client) get(path string, q url.Values, out any) error {
	if q == nil {
		q = url.Values{}
	}
	if c.share != "" {
		q.Set("__sp_pk", c.share)
	}
	req, err := http.NewRequest(http.MethodGet, c.api+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sprintosaur %s : HTTP %d", path, resp.StatusCode)
	}
	return json.Unmarshal(body, out)
}

// CurrentIssues lists the assignee's open issues in the running cycle.
func (c *Client) CurrentIssues(team, assignee string) ([]Issue, error) {
	q := url.Values{"team": {team}, "cycle": {"current"}, "state": {"open"}, "limit": {"100"}}
	if assignee != "" {
		q.Set("assignee", assignee)
	}
	var res struct {
		Issues []Issue `json:"issues"`
	}
	err := c.get("/issues", q, &res)
	return res.Issues, err
}

// Issue fetches one issue by key.
func (c *Client) Issue(key string) (Issue, error) {
	var res Issue
	err := c.get("/issues/"+url.PathEscape(key), nil, &res)
	if err == nil && res.ID == "" {
		// Some endpoints wrap the issue.
		var wrapped struct {
			Issue Issue `json:"issue"`
		}
		if err2 := c.get("/issues/"+url.PathEscape(key), nil, &wrapped); err2 == nil {
			res = wrapped.Issue
		}
	}
	return res, err
}
