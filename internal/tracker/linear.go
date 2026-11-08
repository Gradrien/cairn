package tracker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"cairn/internal/config"
	"cairn/internal/i18n"
)

// Linear reads and moves issues through Linear's GraphQL API, with a personal
// API key (Linear › Settings › Security & access › Personal API keys).
type Linear struct {
	key, team, endpoint string
	http                *http.Client

	once  sync.Once
	me    string
	meErr error
}

const linearAPI = "https://api.linear.app/graphql"

// NewLinear takes the key from LINEAR_API_KEY, else from the credentials file.
func NewLinear(p config.Project) (*Linear, error) {
	key := os.Getenv("LINEAR_API_KEY")
	if key == "" && p.Credentials != "" {
		key = readEnvFile(p.Credentials)["LINEAR_API_KEY"]
	}
	if key == "" {
		where := "LINEAR_API_KEY"
		if p.Credentials != "" {
			where = p.Credentials + i18n.T(" ou ", " or ") + where
		}
		return nil, &NeedsKey{Tracker: "Linear", Where: where}
	}
	return &Linear{key: key, team: p.Key, endpoint: linearAPI, http: &http.Client{Timeout: 15 * time.Second}}, nil
}

func (l *Linear) Name() string { return "Linear" }

func (l *Linear) Guide(key string) string {
	return fmt.Sprintf(i18n.T("Lis le ticket %s et ses commentaires dans Linear (outils MCP Linear s'ils sont là)",
		"Read ticket %s and its comments in Linear (with the Linear MCP tools if available)"), key)
}

type linearIssue struct {
	Identifier    string   `json:"identifier"`
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	Priority      int      `json:"priority"`
	PriorityLabel string   `json:"priorityLabel"`
	Estimate      *float64 `json:"estimate"`
	Assignee      *struct {
		ID string `json:"id"`
	} `json:"assignee"`
	State struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"state"`
	Cycle *struct {
		Number int `json:"number"`
	} `json:"cycle"`
}

const linearFields = `identifier title url priority priorityLabel estimate assignee { id } state { name type } cycle { number }`

func (is linearIssue) issue() Issue {
	out := Issue{
		ID: strings.ToUpper(is.Identifier), Title: is.Title, Status: is.State.Name, URL: is.URL,
		PriorityValue: is.Priority, Assigned: is.Assignee != nil,
		Started: is.State.Type == "started" || is.State.Type == "completed" || is.State.Type == "canceled",
		Done:    is.State.Type == "completed",
	}
	if is.Priority != 0 {
		out.Priority = is.PriorityLabel
	}
	if is.Estimate != nil {
		out.Estimate = fmt.Sprintf(i18n.T("%g pts", "%g pts"), *is.Estimate)
	}
	if is.Cycle != nil {
		out.Cycle = is.Cycle.Number
	}
	return out
}

func (l *Linear) query(q string, vars map[string]any, out any) error {
	body, _ := json.Marshal(map[string]any{"query": q, "variables": vars})
	req, err := http.NewRequest(http.MethodPost, l.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	auth := l.key
	if strings.HasPrefix(auth, "lin_oauth") {
		auth = "Bearer " + auth
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var res struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrKeyRefused
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return fmt.Errorf("linear: HTTP %d", resp.StatusCode)
	}
	if len(res.Errors) > 0 {
		if msg := strings.ToLower(res.Errors[0].Message); strings.Contains(msg, "authenticat") || strings.Contains(msg, "api key") {
			return ErrKeyRefused
		}
		return fmt.Errorf("linear: %s", res.Errors[0].Message)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("linear: HTTP %d", resp.StatusCode)
	}
	return json.Unmarshal(res.Data, out)
}

func (l *Linear) list(filter string) ([]Issue, error) {
	var res struct {
		Issues struct {
			Nodes []linearIssue `json:"nodes"`
		} `json:"issues"`
	}
	q := `query($team: String!) { issues(first: 100, filter: { team: { key: { eq: $team } }, ` + filter + ` }) { nodes { ` + linearFields + ` } } }`
	if err := l.query(q, map[string]any{"team": l.team}, &res); err != nil {
		return nil, err
	}
	out := make([]Issue, 0, len(res.Issues.Nodes))
	for _, is := range res.Issues.Nodes {
		out = append(out, is.issue())
	}
	return out, nil
}

// Mine is what you have in Todo or in progress.
func (l *Linear) Mine() ([]Issue, error) {
	return l.list(`assignee: { isMe: { eq: true } }, state: { type: { in: ["unstarted", "started"] } }`)
}

// Free is what nobody has in the backlog or Todo.
func (l *Linear) Free() ([]Issue, error) {
	return l.list(`assignee: { null: true }, state: { type: { in: ["backlog", "unstarted"] } }`)
}

func (l *Linear) Get(key string) (Issue, error) {
	var res struct {
		Issue *linearIssue `json:"issue"`
	}
	err := l.query(`query($id: String!) { issue(id: $id) { `+linearFields+` } }`, map[string]any{"id": strings.ToUpper(key)}, &res)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "not found") {
		return Issue{}, nil
	}
	if err != nil || res.Issue == nil {
		return Issue{}, err
	}
	return res.Issue.issue(), nil
}

func (l *Linear) viewer() (string, error) {
	l.once.Do(func() {
		var res struct {
			Viewer struct {
				ID string `json:"id"`
			} `json:"viewer"`
		}
		l.meErr = l.query(`query { viewer { id } }`, nil, &res)
		l.me = res.Viewer.ID
	})
	return l.me, l.meErr
}

func (l *Linear) update(key string, input map[string]any) error {
	var res struct {
		IssueUpdate struct {
			Success bool `json:"success"`
		} `json:"issueUpdate"`
	}
	err := l.query(`mutation($id: String!, $input: IssueUpdateInput!) { issueUpdate(id: $id, input: $input) { success } }`,
		map[string]any{"id": strings.ToUpper(key), "input": input}, &res)
	if err == nil && !res.IssueUpdate.Success {
		err = fmt.Errorf(i18n.T("linear : %s n'a pas pu être modifié", "linear: %s could not be updated"), key)
	}
	return err
}

func (l *Linear) Take(key string) error {
	me, err := l.viewer()
	if err != nil {
		return err
	}
	return l.update(key, map[string]any{"assigneeId": me})
}

// Start moves the ticket to the team's first "started" state (In Progress).
func (l *Linear) Start(is Issue) (bool, error) {
	if is.Started {
		return false, nil
	}
	var res struct {
		WorkflowStates struct {
			Nodes []struct {
				ID       string  `json:"id"`
				Position float64 `json:"position"`
			} `json:"nodes"`
		} `json:"workflowStates"`
	}
	err := l.query(`query($team: String!) { workflowStates(filter: { team: { key: { eq: $team } }, type: { eq: "started" } }) { nodes { id position } } }`,
		map[string]any{"team": l.team}, &res)
	if err != nil {
		return false, err
	}
	best, pos := "", 0.0
	for _, s := range res.WorkflowStates.Nodes {
		if best == "" || s.Position < pos {
			best, pos = s.ID, s.Position
		}
	}
	if best == "" {
		return false, nil
	}
	return true, l.update(is.ID, map[string]any{"stateId": best})
}

// Team is a Linear team: its key prefixes its tickets (ENG-123).
type Team struct{ Key, Name string }

// Connection is what Check finds: who the key belongs to, the team the
// project names (nil if missing) and every team the key can see.
type Connection struct {
	User  string
	Team  *Team
	Teams []Team
}

// Check tries the key and looks for the project's team.
func (l *Linear) Check() (Connection, error) {
	var res struct {
		Viewer struct {
			Name string `json:"name"`
		} `json:"viewer"`
		Teams struct {
			Nodes []Team `json:"nodes"`
		} `json:"teams"`
	}
	if err := l.query(`query { viewer { name } teams(first: 100) { nodes { key name } } }`, nil, &res); err != nil {
		return Connection{}, err
	}
	c := Connection{User: res.Viewer.Name, Teams: res.Teams.Nodes}
	for i, t := range c.Teams {
		if strings.EqualFold(t.Key, l.team) {
			c.Team = &c.Teams[i]
		}
	}
	return c, nil
}

// LinearKey says where the project's Linear key comes from: "env", "file"
// or "" when there is none, with the key's last characters to recognise it.
func LinearKey(p config.Project) (source, tail string) {
	key := os.Getenv("LINEAR_API_KEY")
	source = "env"
	if key == "" && p.Credentials != "" {
		key, source = readEnvFile(p.Credentials)["LINEAR_API_KEY"], "file"
	}
	if key == "" {
		return "", ""
	}
	if len(key) > 4 {
		tail = key[len(key)-4:]
	}
	return source, tail
}
