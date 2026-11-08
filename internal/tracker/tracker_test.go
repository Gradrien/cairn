package tracker

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cairn/internal/config"
)

func TestLocal(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "chantiers")
	_ = os.MkdirAll(filepath.Join(root, "archive"), 0o755)
	// An archived EIDO-4: numbers are never reused.
	_ = os.WriteFile(filepath.Join(root, "archive", "EIDO-4-20261101-101010.md"), nil, 0o644)
	l := NewLocal("eido", dir)
	is, err := l.Create("  Export PDF des factures ")
	if err != nil || is.ID != "EIDO-5" || is.Title != "Export PDF des factures" {
		t.Fatalf("create: %+v %v", is, err)
	}
	if _, err := l.Create(" "); err == nil {
		t.Fatal("a ticket needs a title")
	}
	// Another project's file is not ours.
	_ = os.WriteFile(filepath.Join(dir, "YAK-1.md"), []byte("---\nphase: plan\n---\n# YAK-1 · x\n"), 0o644)
	mine, _ := l.Mine()
	if len(mine) != 1 || mine[0].ID != "EIDO-5" || mine[0].Title != "Export PDF des factures" || mine[0].Started {
		t.Fatalf("mine: %+v", mine)
	}
	if got, _ := l.Get("eido-5"); got.ID != "EIDO-5" {
		t.Fatalf("get: %+v", got)
	}
	if got, _ := l.Get("EIDO-9"); got.ID != "" {
		t.Fatalf("missing ticket: %+v", got)
	}
	if next, _ := l.Create("deux"); next.ID != "EIDO-6" {
		t.Fatalf("next: %+v", next)
	}
}

func TestLinear(t *testing.T) {
	var updates []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "lin_api_x" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.Unmarshal(raw, &req)
		q := req.Query
		switch {
		case strings.Contains(q, "isMe"):
			_, _ = io.WriteString(w, `{"data":{"issues":{"nodes":[{"identifier":"ENG-12","title":"Fix login","url":"https://linear.app/x/issue/ENG-12","priority":2,"priorityLabel":"High","estimate":3,"assignee":{"id":"me"},"state":{"name":"Todo","type":"unstarted"},"cycle":{"number":7}}]}}}`)
		case strings.Contains(q, "issue(id"):
			_, _ = io.WriteString(w, `{"data":null,"errors":[{"message":"Entity not found: Issue"}]}`)
		case strings.Contains(q, "viewer"):
			_, _ = io.WriteString(w, `{"data":{"viewer":{"id":"me"}}}`)
		case strings.Contains(q, "workflowStates"):
			_, _ = io.WriteString(w, `{"data":{"workflowStates":{"nodes":[{"id":"review","position":3},{"id":"progress","position":2}]}}}`)
		case strings.Contains(q, "issueUpdate"):
			updates = append(updates, req.Variables["input"].(map[string]any))
			_, _ = io.WriteString(w, `{"data":{"issueUpdate":{"success":true}}}`)
		}
	}))
	defer srv.Close()
	l := &Linear{key: "lin_api_x", team: "ENG", endpoint: srv.URL, http: srv.Client()}

	mine, err := l.Mine()
	if err != nil || len(mine) != 1 {
		t.Fatalf("mine: %+v %v", mine, err)
	}
	is := mine[0]
	if is.ID != "ENG-12" || is.Priority != "High" || is.Estimate != "3 pts" || is.Cycle != 7 || !is.Assigned || is.Started || is.URL == "" {
		t.Fatalf("issue: %+v", is)
	}
	if got, err := l.Get("ENG-99"); err != nil || got.ID != "" {
		t.Fatalf("missing issue: %+v %v", got, err)
	}
	if err := l.Take("ENG-12"); err != nil {
		t.Fatal(err)
	}
	if moved, err := l.Start(is); !moved || err != nil {
		t.Fatalf("start: %v %v", moved, err)
	}
	is.Started = true
	if moved, _ := l.Start(is); moved {
		t.Fatal("a started ticket stays where it is")
	}
	if len(updates) != 2 || updates[0]["assigneeId"] != "me" || updates[1]["stateId"] != "progress" {
		t.Fatalf("updates: %+v", updates)
	}
}

// Without credentials, New fails with a nil Tracker, not a nil pointer
// inside a non-nil interface that crashes on first use.
func TestNewWithoutCredentials(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "")
	t.Setenv("SPRINTOSAUR_API", "")
	t.Setenv("SPRINTOSAUR_TOKEN", "")
	for _, kind := range []string{"linear", "sprintosaur"} {
		tr, err := New(config.Project{Tracker: kind, Key: "ENG", Credentials: "/nope"}, t.TempDir())
		if err == nil || tr != nil {
			t.Errorf("%s: tracker %v, err %v", kind, tr, err)
		}
	}
}

func TestLinearCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "good" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"errors":[{"message":"Authentication required, not authenticated"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"viewer":{"name":"Adrien"},"teams":{"nodes":[{"key":"ENG","name":"Engineering"},{"key":"OPS","name":"Ops"}]}}}`)
	}))
	defer srv.Close()
	l := &Linear{key: "bad", team: "ENG", endpoint: srv.URL, http: srv.Client()}
	if _, err := l.Check(); err != ErrKeyRefused {
		t.Fatalf("refused key: %v", err)
	}
	l.key = "good"
	if c, err := l.Check(); err != nil || c.User != "Adrien" || c.Team == nil || c.Team.Name != "Engineering" {
		t.Fatalf("team found: %+v %v", c, err)
	}
	l.team = "CAIRN"
	if c, _ := l.Check(); c.Team != nil || len(c.Teams) != 2 {
		t.Fatalf("team missing: %+v", c)
	}
}
