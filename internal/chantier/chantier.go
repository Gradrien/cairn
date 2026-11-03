// Package chantier joins Sprintosaur issues, git worktrees, herdr agents and
// chantier files into one list of "where is each piece of work".
package chantier

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"cairn/internal/herdr"
	"cairn/internal/sprintosaur"
	"cairn/internal/state"
)

type Phase int

const (
	ToPlan Phase = iota
	Plan
	Build
	Review
	Shipped
)

var phaseNames = map[Phase]string{ToPlan: "À planifier", Plan: "Plan", Build: "Build", Review: "Review", Shipped: "Livré"}

func (p Phase) String() string { return phaseNames[p] }

// Flow is the phase line shown in the detail view.
var Flow = []Phase{Plan, Build, Review, Shipped}

type Chantier struct {
	Key        string
	Issue      *sprintosaur.Issue
	Worktree   string // empty when no worktree yet
	Main       bool   // worktree is the repo's main checkout
	Branch     string
	Workspaces []string
	State      *state.State
	Agents     []herdr.Agent
	Phase      Phase
}

func (c *Chantier) Title() string {
	if c.Issue != nil && c.Issue.Title != "" {
		return c.Issue.Title
	}
	return c.Branch
}

// Waiting is why the chantier needs you now, or "".
func (c *Chantier) Waiting() string {
	if c.State != nil && c.State.Waiting != "" {
		return c.State.Waiting
	}
	for _, a := range c.Agents {
		if a.Status == "blocked" {
			return a.Label() + " attend une réponse"
		}
	}
	return ""
}

// OpenPR returns the first non-closed pull request.
func (c *Chantier) OpenPR() *sprintosaur.PullRequest {
	if c.Issue == nil {
		return nil
	}
	for i, pr := range c.Issue.PullRequests {
		if pr.State == "open" || pr.State == "draft" {
			return &c.Issue.PullRequests[i]
		}
	}
	return nil
}

// Token is the short status shown in herdr's sidebar.
func (c *Chantier) Token() string {
	if w := c.Waiting(); w != "" {
		return "⚠ " + w
	}
	s := c.Key + " · " + c.Phase.String()
	if c.State != nil && len(c.State.Tasks) > 0 {
		s += fmt.Sprintf(" %d/%d", c.State.DoneCount(), len(c.State.Tasks))
	}
	return s
}

func derivePhase(c *Chantier) Phase {
	if c.Issue != nil && len(c.Issue.PullRequests) > 0 {
		merged := true
		for _, pr := range c.Issue.PullRequests {
			if pr.State != "merged" && pr.State != "closed" {
				merged = false
			}
		}
		if merged || c.Issue.Status == "Done" {
			return Shipped
		}
	}
	if c.State != nil {
		switch c.State.Phase {
		case "plan":
			return Plan
		case "build":
			return Build
		case "review":
			return Review
		case "livré", "livre", "shipped", "done":
			return Shipped
		}
	}
	if c.OpenPR() != nil {
		return Review
	}
	if c.Worktree != "" {
		return Build
	}
	return ToPlan
}

// KeyRe finds a ticket key for team in a branch name.
func KeyRe(team string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(?:^|[/_-])(` + regexp.QuoteMeta(team) + `-\d+)(?:$|[^0-9])`)
}

type Input struct {
	Team         string
	Repo         string
	ChantiersDir string
	Issues       []sprintosaur.Issue // may be nil when Sprintosaur is unreachable
	ExtraIssues  map[string]sprintosaur.Issue
	Worktrees    []herdr.Worktree
	Snapshot     herdr.Snapshot
}

// Gather returns the chantiers, most urgent first, and the agents that belong to none.
func Gather(in Input) ([]*Chantier, []herdr.Agent) {
	byKey := map[string]*Chantier{}
	get := func(key string) *Chantier {
		key = strings.ToUpper(key)
		c, ok := byKey[key]
		if !ok {
			c = &Chantier{Key: key}
			byKey[key] = c
		}
		return c
	}
	for i := range in.Issues {
		get(in.Issues[i].ID).Issue = &in.Issues[i]
	}
	re := KeyRe(in.Team)
	repo := filepath.Clean(in.Repo)
	for _, wt := range in.Worktrees {
		m := re.FindStringSubmatch(wt.Branch)
		if m == nil {
			continue
		}
		c := get(m[1])
		if c.Worktree != "" && filepath.Clean(wt.Path) == repo {
			continue // a linked worktree wins over the main checkout
		}
		c.Worktree, c.Branch = wt.Path, wt.Branch
		c.Main = filepath.Clean(wt.Path) == repo
		c.Workspaces = nil
		if wt.OpenWorkspaceID != "" && !c.Main {
			c.Workspaces = append(c.Workspaces, wt.OpenWorkspaceID)
		}
	}
	for key, c := range byKey {
		if c.Issue == nil {
			if is, ok := in.ExtraIssues[key]; ok {
				c.Issue = &is
			}
		}
		if s, err := state.Load(state.PathFor(in.ChantiersDir, key)); err == nil {
			c.State = s
		}
	}

	var others []herdr.Agent
	for _, a := range in.Snapshot.Agents {
		if c := owner(a, byKey); c != nil {
			c.Agents = append(c.Agents, a)
			if !contains(c.Workspaces, a.WorkspaceID) {
				c.Workspaces = append(c.Workspaces, a.WorkspaceID)
			}
		} else {
			others = append(others, a)
		}
	}

	list := make([]*Chantier, 0, len(byKey))
	for _, c := range byKey {
		c.Phase = derivePhase(c)
		list = append(list, c)
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if (a.Waiting() != "") != (b.Waiting() != "") {
			return a.Waiting() != ""
		}
		if rank(a.Phase) != rank(b.Phase) {
			return rank(a.Phase) < rank(b.Phase)
		}
		return a.Key < b.Key
	})
	return list, others
}

func rank(p Phase) int {
	return map[Phase]int{Review: 0, Build: 1, Plan: 2, ToPlan: 3, Shipped: 4}[p]
}

// owner picks the chantier an agent works on: by worktree, or — in the shared
// main checkout — only when its name or title names the ticket.
func owner(a herdr.Agent, byKey map[string]*Chantier) *Chantier {
	dir := filepath.Clean(a.Dir())
	var best *Chantier
	for _, c := range byKey {
		if c.Worktree == "" || c.Main {
			continue
		}
		wt := filepath.Clean(c.Worktree)
		if dir == wt || strings.HasPrefix(dir, wt+string(filepath.Separator)) {
			if best == nil || len(wt) > len(best.Worktree) {
				best = c
			}
		}
	}
	if best != nil {
		return best
	}
	text := strings.ToUpper(a.Name + " " + a.Title)
	for key, c := range byKey {
		if regexp.MustCompile(`\b` + regexp.QuoteMeta(key) + `\b`).MatchString(text) {
			return c
		}
	}
	return nil
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// Slug turns a title into a branch-safe slug of at most max bytes.
func Slug(title string, max int) string {
	var b strings.Builder
	dash := false
	for _, r := range norm.NFD.String(strings.ToLower(title)) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > max {
		s = strings.TrimRight(s[:max], "-")
	}
	return s
}

// PlanPrompt is what a new chantier's agent is asked first.
func PlanPrompt(key, title, statePath string) string {
	return fmt.Sprintf("Planifie le ticket %s « %s » avec le workflow cairn (skill cairn). "+
		"Lis le ticket et ses commentaires via le skill sprintosaur, explore le code, puis écris le plan dans %s, "+
		"à la taille du ticket. Recense les effets de bord : tout ce que le changement peut casser ou modifier ailleurs "+
		"(autres appelants et écrans, autres apps du monorepo, données existantes, permissions, migrations, notifications, "+
		"performances), avec pour chacun une tâche qui le vérifie. Termine par `attente: Valider le plan` et arrête-toi : ne code rien avant ma validation.",
		key, title, statePath)
}

// BuildPrompt is sent when the plan is approved.
func BuildPrompt(statePath string) string {
	return fmt.Sprintf("Plan validé. Passe %s en `phase: build` et implémente-le : passe chaque tâche en [~] quand tu la "+
		"commences et en [x] quand elle est finie, et note les preuves de test sous « ## Preuves ». Ne pousse rien et n'ouvre "+
		"pas de PR toi-même : quand tout est prêt, mets `attente: Prêt à pousser` avec en une ligne ce qui partira, et "+
		"arrête-toi. L'humain relit et autorise le push dans cairn.", statePath)
}
