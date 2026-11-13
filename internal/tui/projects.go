package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"cairn/internal/chantier"
	"cairn/internal/config"
	"cairn/internal/localtest"
	"cairn/internal/mux"
	"cairn/internal/tracker"
)

// project is what cairn knows of one project of the config. The model holds
// pointers: every copy of the model sees the same projects.
type project struct {
	cfg       config.Project
	tr        tracker.Tracker
	err       error                            // the tracker's last error
	issues    []tracker.Issue                  // your tickets, from the tracker
	extra     map[string]tracker.Issue         // tickets out of that list that have a chantier
	worktrees []mux.Worktree                   // the repository's worktrees
	prs       map[string][]tracker.PullRequest // GitHub PRs by branch
}

func newProjects(cfg config.Config) []*project {
	var out []*project
	for _, pc := range cfg.Projects {
		p := &project{cfg: pc, extra: map[string]tracker.Issue{}}
		p.tr, p.err = tracker.New(pc, config.ChantiersDir())
		out = append(out, p)
	}
	return out
}

// projectOf is the project a chantier belongs to, nil if none.
func (m Model) projectOf(c *chantier.Chantier) *project {
	if c == nil {
		return nil
	}
	for _, p := range m.projects {
		if p.cfg.Name == c.Project {
			return p
		}
	}
	return m.projectForKey(c.Key)
}

// projectForKey finds the project by the ticket prefix.
func (m Model) projectForKey(key string) *project {
	prefix, _, _ := strings.Cut(strings.ToUpper(key), "-")
	for _, p := range m.projects {
		if p.cfg.Key == prefix {
			return p
		}
	}
	return nil
}

func (m Model) allWorktrees() []mux.Worktree {
	var out []mux.Worktree
	for _, p := range m.projects {
		out = append(out, p.worktrees...)
	}
	return out
}

func (m Model) sources() []chantier.Source {
	var out []chantier.Source
	for _, p := range m.projects {
		issues := append([]tracker.Issue(nil), p.issues...)
		for _, is := range p.extra {
			issues = append(issues, is)
		}
		out = append(out, chantier.Source{
			Project: p.cfg.Name, Key: p.cfg.Key, Repo: p.cfg.Repo,
			Issues: issues, Worktrees: p.worktrees, BranchPRs: p.prs,
		})
	}
	return out
}

type localMsg struct {
	worktrees map[string][]mux.Worktree   // by project name
	tests     map[string]localtest.Status // main checkouts under test, by project name
	snap      mux.Snapshot
	err       error
}

func (m Model) loadLocal() tea.Cmd {
	repos := map[string]string{}
	for _, p := range m.projects {
		repos[p.cfg.Name] = p.cfg.Repo
	}
	return func() tea.Msg {
		snap, err := mux.GetSnapshot()
		if err != nil {
			return localMsg{err: err}
		}
		out := map[string][]mux.Worktree{}
		tests := map[string]localtest.Status{}
		store := ltStore()
		for name, repo := range repos {
			if st, ok := store.Get(repo); ok {
				tests[name] = st
			}
			wts, err := mux.ListWorktrees(repo)
			if err != nil {
				return localMsg{err: err}
			}
			out[name] = wts
		}
		return localMsg{worktrees: out, tests: tests, snap: snap}
	}
}

type projectIssues struct {
	name   string
	issues []tracker.Issue
	extra  map[string]tracker.Issue
	prs    map[string][]tracker.PullRequest
	err    error
}

type issuesMsg []projectIssues

// loadIssues reads every project's tickets and GitHub PRs, side by side.
func (m Model) loadIssues() tea.Cmd {
	// Copies: the model keeps changing its projects while this runs.
	projects := make([]project, len(m.projects))
	for i, p := range m.projects {
		projects[i] = project{cfg: p.cfg, tr: p.tr, err: p.err, extra: map[string]tracker.Issue{}}
		for k, v := range p.extra {
			projects[i].extra[k] = v
		}
	}
	return func() tea.Msg {
		out := make(issuesMsg, len(projects))
		var wg sync.WaitGroup
		for i, p := range projects {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out[i] = readProject(&p)
			}()
		}
		wg.Wait()
		return out
	}
}

func readProject(p *project) (res projectIssues) {
	res = projectIssues{name: p.cfg.Name, extra: map[string]tracker.Issue{}}
	// This runs outside bubbletea's own recovery: a panic here would kill
	// cairn without restoring the terminal. Report it as the tracker's error.
	defer func() {
		if r := recover(); r != nil {
			res = projectIssues{name: p.cfg.Name, err: fmt.Errorf("%s : %v", p.cfg.Tracker, r)}
		}
	}()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		res.prs = tracker.BranchPRs(p.cfg.Repo)
	}()
	defer wg.Wait()
	if p.tr == nil {
		res.err = p.err
		return res
	}
	issues, err := p.tr.Mine()
	if err != nil {
		res.err = err
		return res
	}
	res.issues = issues
	// Worktrees and chantier files whose ticket is not in your list still
	// deserve a title.
	known := map[string]bool{}
	for _, is := range issues {
		known[is.ID] = true
	}
	var keys []string
	if wts, err := mux.ListWorktrees(p.cfg.Repo); err == nil {
		re := chantier.KeyRe(p.cfg.Key)
		for _, wt := range wts {
			if mm := re.FindStringSubmatch(wt.Branch); mm != nil {
				keys = append(keys, strings.ToUpper(mm[1]))
			}
		}
	}
	for key := range p.extra {
		keys = append(keys, key)
	}
	for _, key := range keys {
		if known[key] || len(res.extra) >= 15 {
			continue
		}
		known[key] = true
		if is, err := p.tr.Get(key); err == nil && is.ID != "" {
			res.extra[key] = is
		} else if old, ok := p.extra[key]; ok {
			res.extra[key] = old
		}
	}
	return res
}

// apply stores what loadIssues read.
func (m *Model) apply(msg issuesMsg) (anyOK bool) {
	for _, r := range msg {
		for _, p := range m.projects {
			if p.cfg.Name != r.name {
				continue
			}
			if r.prs != nil {
				p.prs = r.prs
			}
			p.err = r.err
			if r.err == nil {
				p.issues, p.extra = r.issues, r.extra
				anyOK = true
			}
		}
	}
	return anyOK
}

// openURL opens a link or file with the system's handler.
func openURL(target string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", target).Run()
	}
	return exec.Command("xdg-open", target).Start()
}
