package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"cairn/internal/chantier"
	"cairn/internal/config"
	"cairn/internal/herdr"
	"cairn/internal/i18n"
	"cairn/internal/progress"
	"cairn/internal/prthread"
	"cairn/internal/state"
	"cairn/internal/tracker"
)

// exampleTracker names the example project's tracker on screen.
type exampleTracker struct{ tracker.Tracker }

func (exampleTracker) Name() string { return "Linear" }

// exampleScreens is cairn with invented example data, in English: the source
// of the README screenshots. Nothing in it comes from a real project.
func exampleScreens(t *testing.T, w, h int) Model {
	t.Helper()
	i18n.Set("en")
	dir := "/home/you/.cairn/chantiers"
	plan := func(key, body string, ago time.Duration) *state.State {
		s := state.Parse(filepath.Join(dir, key+".md"), body)
		s.UpdatedAt = time.Now().Add(-ago)
		return s
	}
	issue := func(key, title, status, prio, est string) *tracker.Issue {
		return &tracker.Issue{ID: key, Title: title, Status: status, Priority: prio, Estimate: est, Assigned: true, Started: status != "Todo", URL: "https://linear.app/example/issue/" + key}
	}
	local := func(key, title string) *tracker.Issue {
		return &tracker.Issue{ID: key, Title: title, Status: "In Progress", Started: true}
	}
	agent := func(name, status, title string) herdr.Agent {
		return herdr.Agent{Name: name, Status: status, PaneID: "p-" + name, Title: title}
	}
	m := Model{
		width: w, height: h, hoverItem: -1, hoverBtn: -1, watch: newWatch(),
		gits: map[string]progress.Git{}, prs: map[string]progress.PR{}, readAt: map[string]time.Time{}, reading: map[string]bool{},
		tokens: map[string]string{}, unfold: map[string]bool{}, answered: map[string]string{}, loaded: true,
	}
	m.projects = []*project{
		{cfg: config.Project{Name: "eng", Key: "ENG", Tracker: "linear", Base: "origin/main"}, tr: exampleTracker{}},
		{cfg: config.Project{Name: "web", Key: "WEB", Tracker: "local", Base: "origin/main"}},
	}
	m.chantiers = []*chantier.Chantier{
		{Key: "ENG-412", Project: "eng", Phase: chantier.Plan, Worktree: "/code/api-412", Branch: "eng-412-rate-limit",
			Issue:  issue("ENG-412", "Rate-limit the public API", "Todo", "High", "3"),
			Agents: []herdr.Agent{agent("eng-412", "idle", "plan written")},
			State: plan("ENG-412", "---\nphase: plan\nwaiting: Approve the plan\n---\n"+
				"Add a token bucket per API key in front of the public routes, with limits read from the customer's plan.\n\n"+
				"## Tasks\n- [ ] Token bucket in middleware/ratelimit.go\n- [ ] Limits per plan, read from billing\n- [ ] Retry-After header on 429\n- [ ] Load test: 500 req/s for 60 s\n\n"+
				"## Questions\n- Per API key or per IP?\n- Keep the 429 body as JSON?\n\n"+
				"## Side effects\n- [ ] Webhooks share the router: exempt them\n- [ ] Status page polls /health every 5 s\n- [ ] SDK retries: check they honour Retry-After\n", 12*time.Minute)},
		{Key: "WEB-7", Project: "web", Phase: chantier.Build, Worktree: "/code/web-7", Branch: "web-7-invoice-pdf",
			Issue:  local("WEB-7", "Export invoices as PDF"),
			Agents: []herdr.Agent{agent("web-7", "blocked", "question")},
			State: plan("WEB-7", "---\nphase: build\nwaiting: Answer the agent\n---\nInvoices download as PDF from the billing page.\n\n"+
				"## Tasks\n- [x] PDF template\n- [~] Download button\n- [ ] Email attachment\n\n## Questions\n- Should the PDF include the VAT breakdown?\n", 3*time.Minute)},
		{Key: "ENG-430", Project: "eng", Phase: chantier.Build, Worktree: "/code/api-430", Branch: "eng-430-csv-export",
			Issue:  issue("ENG-430", "CSV export of monthly reports", "In Progress", "Medium", "2"),
			Agents: []herdr.Agent{agent("eng-430", "idle", "ready")},
			State: plan("ENG-430", "---\nphase: build\nwaiting: Ready to push\n---\nExport the monthly report as CSV.\n\n"+
				"## Tasks\n- [x] Query the report rows\n- [x] Stream the CSV\n- [x] Button on the reports page\n\n"+
				"## Side effects\n- [x] Large accounts: stream, never buffer\n- [x] Report permissions apply to the export\n\n"+
				"## Proofs\n- go test ./reports/... ✓ 24 tests\n- curl /v1/reports/2026-10.csv → 200, 1,204 rows\n", 8*time.Minute)},
		{Key: "ENG-398", Project: "eng", Phase: chantier.Build, Worktree: "/code/api-398", Branch: "eng-398-webhook-retries",
			Issue:  issue("ENG-398", "Retry failed webhooks", "In Progress", "High", "5"),
			Agents: []herdr.Agent{agent("eng-398", "working", "go test")},
			State:  plan("ENG-398", "---\nphase: build\n---\n## Tasks\n- [x] Retry queue\n- [x] Backoff: 1 s, 4 s, 16 s\n- [x] Dead-letter after 3 tries\n- [x] Admin view of dead letters\n- [~] Replay button\n- [ ] Docs\n", 1*time.Minute)},
		{Key: "ENG-377", Project: "eng", Phase: chantier.Review, Worktree: "/code/api-377", Branch: "eng-377-audit-pagination",
			Issue:  issue("ENG-377", "Paginate the audit log", "In Review", "Medium", "2"),
			Agents: []herdr.Agent{agent("eng-377", "idle", "waiting for review")},
			PRs:    []tracker.PullRequest{{Number: 418, State: "open", URL: "https://github.com/example/api/pull/418", Branch: "eng-377-audit-pagination"}},
			State: plan("ENG-377", "---\nphase: review\npush: allowed 4f2a91c\n---\nCursor pagination for the audit log API and page.\n\n"+
				"## Tasks\n- [x] Cursor on (created_at, id)\n- [x] API: ?cursor= and ?limit=\n- [x] Infinite scroll on the audit page\n\n"+
				"## Side effects\n- [x] CSV export still returns every row\n- [ ] Mobile app reads page=: keep it working one release\n\n"+
				"## Proofs\n- go test ./audit/... ✓ 31 tests (14:20)\n- curl '/v1/audit?limit=50' → 50 rows + next cursor (14:31)\n- 2.4 M rows: first page in 38 ms, was 2.1 s (14:44)\n", 2*time.Hour)},
		{Key: "WEB-5", Project: "web", Phase: chantier.Review, Worktree: "/code/web-5", Branch: "web-5-dark-mode",
			Issue:  local("WEB-5", "Dark mode for the editor"),
			Agents: []herdr.Agent{agent("web-5", "idle", "done")},
			PRs:    []tracker.PullRequest{{Number: 31, State: "open", URL: "https://github.com/example/web/pull/31", Branch: "web-5-dark-mode"}},
			State:  plan("WEB-5", "---\nphase: review\n---\n## Tasks\n- [x] Theme tokens\n- [x] Editor colours\n- [x] Toggle in settings\n", 40*time.Minute)},
	}
	m.others = []herdr.Agent{agent("scratch", "idle", "Bump Go to 1.27")}
	m.prs["ENG-377"] = progress.PR{URL: "https://github.com/example/api/pull/418", State: "OPEN", Mergeable: "MERGEABLE", ReviewDecision: "CHANGES_REQUESTED",
		HeadOid: "4f2a91c", Checks: progress.Checks{Passed: 6}, Comments: 3, Author: "you",
		Activity: []progress.Activity{
			{Who: "maya", At: time.Now().Add(-4 * time.Minute), Review: true, State: "CHANGES_REQUESTED", Body: "A fixed window lets a client burst at the boundary."},
		}}
	m.prs["WEB-5"] = progress.PR{URL: "https://github.com/example/web/pull/31", State: "OPEN", Mergeable: "MERGEABLE", Checks: progress.Checks{Passed: 4}}
	m.gits["ENG-377"] = progress.Git{Ahead: 3, Files: 7, Added: 214, Removed: 58, Head: "4f2a91c", LastSubject: "feat(audit): cursor pagination", LastAt: time.Now().Add(-2 * time.Hour)}
	m.gits["ENG-430"] = progress.Git{Ahead: 3, Files: 7, Added: 182, Removed: 12, LastSubject: "feat(reports): CSV export", LastAt: time.Now().Add(-9 * time.Minute)}
	m.gits["ENG-412"] = progress.Git{}
	m.rebuildOrder()
	return m
}

func exampleThread() *prthread.Thread {
	now := time.Now()
	return &prthread.Thread{Owner: "example", Repo: "api", Number: 418, HeadRef: "eng-377-audit-pagination", Entries: []prthread.Entry{
		{Author: "you", At: now.Add(-2 * time.Hour), Kind: "description", Body: "## Cursor pagination for the audit log\n\nThe audit log loaded every row: 2.1 s on large accounts. It now pages with a cursor on `(created_at, id)`.\n\n- API: `?cursor=` and `?limit=` (default 50, max 200)\n- The audit page scrolls infinitely\n- `page=` still works for one release, for the mobile app"},
		{Author: "maya", At: now.Add(-4 * time.Minute), Kind: "review", State: "CHANGES_REQUESTED", Body: "Looks good overall. Two things before we merge."},
		{Author: "maya", At: now.Add(-4 * time.Minute), Kind: "inline", Path: "audit/cursor.go", Line: 42, Body: "Two rows can share `created_at`. Compare on `(created_at, id)` together, or a page can skip a row:\n\n```go\nif row.CreatedAt.Before(c.At) || row.CreatedAt.Equal(c.At) && row.ID < c.ID {\n```"},
		{Author: "maya", At: now.Add(-3 * time.Minute), Kind: "inline", Path: "web/audit/List.tsx", Line: 88, Body: "The spinner shows at the bottom even when there is no next page."},
	}}
}

// TestExampleScreens writes the README screenshots' terminal output (ANSI)
// to $CAIRN_SHOTS: overview, plan, pr, comments, board.
func TestExampleScreens(t *testing.T) {
	dir := os.Getenv("CAIRN_SHOTS")
	if dir == "" {
		t.Skip("CAIRN_SHOTS not set")
	}
	defer i18n.Set("fr")
	lipgloss.SetColorProfile(termenv.TrueColor)
	const w, h = 150, 44
	m := exampleScreens(t, w, h)
	at := func(key string) Model {
		mm := m
		for i, c := range mm.chantiers {
			if c.Key == key {
				mm.cursor = i
			}
		}
		return mm
	}
	comments := at("ENG-377")
	comments.view = "comments"
	comments.thread = exampleThread()
	comments.threadKey = "ENG-377"
	comments.height = 26
	board := press(t, m, "t")
	board.height = 30
	shots := map[string]Model{
		"overview": at("ENG-430"),
		"plan":     at("ENG-412"),
		"pr":       at("ENG-377"),
		"comments": comments,
		"board":    board,
	}
	for name, v := range shots {
		if err := os.WriteFile(filepath.Join(dir, name+".ans"), []byte(v.View()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
