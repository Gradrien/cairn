// Package chantier joins tickets, git worktrees, pull requests, herdr agents
// and chantier files into one list of "where is each piece of work".
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
	"cairn/internal/i18n"
	"cairn/internal/state"
	"cairn/internal/tracker"
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

// String is the stable (French) name, written to the journal: never translate it.
func (p Phase) String() string { return phaseNames[p] }

// Label is the name shown on screen, in the interface language.
func (p Phase) Label() string {
	switch p {
	case ToPlan:
		return i18n.T("À planifier", "To plan")
	case Shipped:
		return i18n.T("Livré", "Shipped")
	}
	return phaseNames[p]
}

// Flow is the phase line shown in the detail view.
var Flow = []Phase{Plan, Build, Review, Shipped}

type Chantier struct {
	Key        string
	Project    string // name of the project it belongs to
	Issue      *tracker.Issue
	PRs        []tracker.PullRequest // oldest first
	Worktree   string                // empty when no worktree yet
	Main       bool                  // worktree is the repo's main checkout
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
	if c.State != nil && c.State.Title != "" {
		return c.State.Title
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
			return a.Label() + i18n.T(" attend une réponse", " is waiting for an answer")
		}
	}
	return ""
}

// OpenPR returns the first non-closed pull request.
func (c *Chantier) OpenPR() *tracker.PullRequest {
	for i, pr := range c.PRs {
		if pr.State == "open" || pr.State == "draft" {
			return &c.PRs[i]
		}
	}
	return nil
}

// LastPR is the open pull request, else the latest one, else nil.
func (c *Chantier) LastPR() *tracker.PullRequest {
	if pr := c.OpenPR(); pr != nil {
		return pr
	}
	if len(c.PRs) == 0 {
		return nil
	}
	return &c.PRs[len(c.PRs)-1]
}

// Token is the short status shown in herdr's sidebar.
func (c *Chantier) Token() string {
	if w := c.Waiting(); w != "" {
		return "⚠ " + w
	}
	s := c.Key + " · " + c.Phase.Label()
	if c.State != nil && len(c.State.Tasks) > 0 {
		s += fmt.Sprintf(" %d/%d", c.State.DoneCount(), len(c.State.Tasks))
	}
	return s
}

func derivePhase(c *Chantier) Phase {
	if len(c.PRs) > 0 {
		merged := true
		for _, pr := range c.PRs {
			if pr.State != "merged" && pr.State != "closed" {
				merged = false
			}
		}
		if merged || (c.Issue != nil && c.Issue.Done) {
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

// withGitHub corrects PR states from GitHub, which wins over a tracker's
// copy that can lag behind.
func withGitHub(prs []tracker.PullRequest, gh map[string]string) []tracker.PullRequest {
	prs = append([]tracker.PullRequest(nil), prs...)
	for i, pr := range prs {
		switch gh[pr.URL] {
		case "MERGED":
			prs[i].State = "merged"
		case "CLOSED":
			prs[i].State = "closed"
		}
	}
	return prs
}

// KeyRe finds a ticket key of one of the prefixes (YAK, EIDO…) in a branch name.
func KeyRe(prefixes ...string) *regexp.Regexp {
	alt := make([]string, len(prefixes))
	for i, p := range prefixes {
		alt[i] = regexp.QuoteMeta(p)
	}
	return regexp.MustCompile(`(?i)(?:^|[/_-])((?:` + strings.Join(alt, "|") + `)-\d+)(?:$|[^0-9])`)
}

// Source is what one project knows.
type Source struct {
	Project string // project name
	Key     string // ticket prefix
	Repo    string // main checkout
	// Issues are the tickets to show, may be nil when the tracker is unreachable.
	Issues    []tracker.Issue
	Worktrees []herdr.Worktree
	// BranchPRs are the pull requests GitHub has for a branch, newest first.
	BranchPRs map[string][]tracker.PullRequest
}

type Input struct {
	ChantiersDir string
	Sources      []Source
	Snapshot     herdr.Snapshot
	// GitHubPRs is the state GitHub reports for a PR URL (OPEN, MERGED,
	// CLOSED). It wins over the tracker's copy, which can lag behind.
	GitHubPRs map[string]string
}

// Gather returns the chantiers, most urgent first, and the agents that belong to none.
func Gather(in Input) ([]*Chantier, []herdr.Agent) {
	byKey := map[string]*Chantier{}
	get := func(key, project string) *Chantier {
		key = strings.ToUpper(key)
		c, ok := byKey[key]
		if !ok {
			c = &Chantier{Key: key, Project: project}
			byKey[key] = c
		}
		return c
	}
	for _, src := range in.Sources {
		for i := range src.Issues {
			if c := get(src.Issues[i].ID, src.Project); c.Issue == nil {
				is := src.Issues[i]
				c.Issue = &is
			}
		}
		re := KeyRe(src.Key)
		repo := filepath.Clean(src.Repo)
		for _, wt := range src.Worktrees {
			m := re.FindStringSubmatch(wt.Branch)
			if m == nil {
				continue
			}
			c := get(m[1], src.Project)
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
		for _, c := range byKey {
			if c.Project != src.Project {
				continue
			}
			var prs []tracker.PullRequest
			if c.Issue != nil {
				prs = append(prs, c.Issue.PullRequests...)
			}
			// GitHub lists newest first; chantiers keep the oldest first.
			found := src.BranchPRs[c.Branch]
			for i := len(found) - 1; i >= 0; i-- {
				if c.Branch != "" && !hasPR(prs, found[i].URL) {
					prs = append(prs, found[i])
				}
			}
			c.PRs = withGitHub(prs, in.GitHubPRs)
		}
	}
	for key, c := range byKey {
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

func hasPR(prs []tracker.PullRequest, url string) bool {
	for _, p := range prs {
		if p.URL == url {
			return true
		}
	}
	return false
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
// guide says where the agent reads the ticket (see tracker.Tracker.Guide).
func PlanPrompt(key, title, statePath, guide string) string {
	return fmt.Sprintf(i18n.T("Planifie le ticket %s « %s » avec le workflow cairn (skill cairn). "+
		"%s, explore le code, puis écris le plan dans %s, "+
		"à la taille du ticket. Recense les effets de bord : tout ce que le changement peut casser ou modifier ailleurs "+
		"(autres appelants et écrans, autres parties du dépôt, données existantes, permissions, migrations, notifications, "+
		"performances), avec pour chacun une tâche qui le vérifie. Confie ces recherches au sous-agent cairn-explorer (plusieurs en parallèle "+
		"si les questions sont indépendantes) plutôt que de lire le dépôt toi-même. Termine par `attente: Valider le plan` et arrête-toi : ne code rien avant ma validation.",
		"Plan ticket %s \"%s\" with the cairn workflow (cairn skill). "+
			"%s, explore the code, then write the plan in %s, "+
			"sized to the ticket. List the side effects: everything the change can break or alter elsewhere "+
			"(other callers and screens, other parts of the repository, existing data, permissions, migrations, notifications, "+
			"performance), each with a task that checks it. Hand these searches to the cairn-explorer subagent (several in parallel "+
			"when the questions are independent) rather than reading the repository yourself. Finish with `attente: Valider le plan` and stop: write no code before I approve."),
		key, title, guide, statePath)
}

// BuildPrompt is sent when the plan is approved.
func BuildPrompt(statePath string) string {
	return fmt.Sprintf(i18n.T("Plan validé. Passe %s en `phase: build` et implémente-le : passe chaque tâche en [~] quand tu la "+
		"commences et en [x] quand elle est finie, et note les preuves de test sous « ## Preuves ». Fais tourner les tests, le build et "+
		"le lint par le sous-agent cairn-runner, et les recherches dans le dépôt par cairn-explorer : ton contexte reste petit. Tiens aussi la section "+
		"« ## Comment ça marche » (voir le skill cairn) : comment c'est construit, où, quand ça s'exécute et où vivent les données, "+
		"pour que je puisse répondre aux questions sur le ticket sans lire le code. Quand le changement peut s'essayer à la main, "+
		"écris aussi « ## Tester à la main » : ce qu'il faut lancer, puis des étapes courtes « fais ça → vérifie ça ». Ne pousse rien et n'ouvre "+
		"pas de PR toi-même : quand tout est prêt, mets `attente: Prêt à pousser` avec en une ligne ce qui partira, et "+
		"arrête-toi. L'humain relit et autorise le push dans cairn (ou te le demande dans le chat) ; la PR s'autorise à part.",
		"Plan approved. Set %s to `phase: build` and implement it: mark each task [~] when you "+
			"start it and [x] when it is done, and write the test proofs under \"## Preuves\". Run the tests, the build and "+
			"the lint through the cairn-runner subagent, and searches in the repository through cairn-explorer: your context stays small. Also keep the "+
			"\"## Comment ça marche\" section (see the cairn skill): how it is built, where, when it runs and where the data lives, "+
			"so I can answer questions on the ticket without reading the code. When the change can be tried by hand, "+
			"also write \"## Tester à la main\": what to start, then short steps \"do this → check that\". Do not push and do not open "+
			"a PR yourself: when everything is ready, set `attente: Prêt à pousser` followed by one line on what will ship, and "+
			"stop. The human reviews and allows the push in cairn (or asks you in the chat); the PR is allowed separately."), statePath)
}

// FreshBuildPrompt starts the build with a new agent: the one that planned
// stopped, everything it established is in the plan.
func FreshBuildPrompt(key, statePath string) string {
	return fmt.Sprintf(i18n.T("Tu prends la suite de l'agent qui a planifié le ticket %s (workflow cairn, skill cairn). "+
		"Tout ce qu'il a établi est dans %s : lis-le d'abord, en entier. Ne refais pas l'exploration ; pour vérifier un point "+
		"précis, demande au sous-agent cairn-explorer.\n\n",
		"You take over from the agent that planned ticket %s (cairn workflow, cairn skill). "+
			"Everything it established is in %s: read it first, all of it. Do not explore again; to check a specific point, "+
			"ask the cairn-explorer subagent.\n\n"), key, statePath) + BuildPrompt(statePath)
}

// HandoffPrompt asks the agent at work to write what the next one needs.
func HandoffPrompt(statePath string) string {
	return fmt.Sprintf(i18n.T("Un agent neuf va reprendre ce chantier pour alléger le contexte. Termine l'étape en cours si elle "+
		"prend moins d'une minute, sinon arrête-toi là. Mets le plan %s à jour (tâches, preuves, « Comment ça marche »), puis écris "+
		"une section « ## Reprise » de 5 à 15 lignes pour lui : où tu en es exactement, ce qui reste en cours, ce que tu as essayé "+
		"qui n'a pas marché et pourquoi, les pièges du code, la prochaine étape. Pas ce qui est déjà dans le plan ou dans git. "+
		"Lance `cairn sync` et arrête-toi : ne commence rien d'autre.",
		"A new agent will take over this ticket to lighten the context. Finish the current step if it takes under a minute, "+
			"otherwise stop there. Update the plan %s (tasks, proofs, \"Comment ça marche\"), then write a \"## Reprise\" section of "+
			"5 to 15 lines for it: exactly where you are, what is in progress, what you tried that did not work and why, the "+
			"traps in the code, the next step. Not what is already in the plan or in git. Run `cairn sync` and stop: start nothing else."), statePath)
}

// ResumePrompt starts the agent that takes over from a handoff.
func ResumePrompt(key, statePath, base string) string {
	return fmt.Sprintf(i18n.T("Tu reprends le ticket %s (workflow cairn, skill cairn) à la suite d'un autre agent. Lis le plan %s "+
		"en entier, sa section « ## Reprise » d'abord, puis regarde ce qui est déjà fait : `git log --oneline %[3]s..HEAD` et "+
		"`git diff --stat %[3]s...HEAD`, et `git status` pour ce qui n'est pas commité. Ne refais pas l'exploration ; pour vérifier "+
		"un point, demande au sous-agent cairn-explorer. Puis continue le travail là où il s'est arrêté, avec les règles du skill "+
		"(tâches, preuves, tests par cairn-runner, pas de push sans feu vert). La section « ## Reprise » reste : la prochaine "+
		"passation la réécrira.",
		"You take over ticket %s (cairn workflow, cairn skill) from another agent. Read the plan %s in full, its \"## Reprise\" "+
			"section first, then look at what is done: `git log --oneline %[3]s..HEAD` and `git diff --stat %[3]s...HEAD`, and "+
			"`git status` for what is not committed. Do not explore again; to check a point, ask the cairn-explorer subagent. Then "+
			"carry on where it stopped, with the skill's rules (tasks, proofs, tests through cairn-runner, no push without a go-ahead). "+
			"The \"## Reprise\" section stays: the next handoff will rewrite it."), key, statePath, base)
}

// AskPrompt is a question on the implementation, for a short-lived agent
// that reads the code and answers on its standard output.
func AskPrompt(key, statePath, question string) string {
	return fmt.Sprintf(i18n.T("Question de l'humain sur l'implémentation du ticket %s, dans ce dépôt :\n« %s »\n\n"+
		"Le plan du ticket, avec sa section « Comment ça marche », est dans %s : lis-le d'abord. Vérifie la réponse dans le "+
		"code (lis seulement ce qu'il faut), puis réponds en 2 à 8 lignes de markdown, en citant les endroits en `fichier:ligne`. "+
		"Des faits vérifiés, pas des suppositions ; si tu ne trouves pas, dis-le et dis où tu as cherché. Ta réponse est "+
		"recopiée telle quelle dans le plan : pas de préambule ni de formule de politesse, rien d'autre que la réponse.",
		"The human's question on how ticket %s is implemented, in this repository:\n\"%s\"\n\n"+
			"The ticket's plan, with its \"Comment ça marche\" section, is in %s: read it first. Check the answer in the code "+
			"(read only what you need), then answer in 2 to 8 lines of markdown, citing places as `file:line`. Facts you "+
			"checked, not guesses; if you do not find it, say so and say where you looked. Your answer is copied as is into "+
			"the plan: no preamble and no pleasantries, nothing but the answer."), key, question, statePath)
}
