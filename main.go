// cairn: where each piece of work stands, next to herdr or Rex.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"cairn/internal/chantier"
	"cairn/internal/config"
	"cairn/internal/guard"
	"cairn/internal/home"
	"cairn/internal/i18n"
	"cairn/internal/images"
	"cairn/internal/mux"
	"cairn/internal/state"
	"cairn/internal/tui"
	"cairn/internal/update"
)

// sourceDir is the clone this build was installed from, set by install.sh;
// U updates from it.
var sourceDir string

// buildRev is the commit install.sh built, empty when the clone had edits to
// tracked files: only a clean build of a commit can tell whether main is ahead.
var buildRev string

func usage() string {
	return i18n.T(`cairn — où en est chaque chantier, à côté de herdr ou de Rex.

  cairn            ouvre la TUI (tout se fait au clavier, touches affichées en bas)
  cairn add [dossier] [--tracker local|linear|sprintosaur] [--key EIDO] [--name eido]
                   ajoute le dépôt (par défaut celui où tu es) aux projets suivis ;
                   sans tracker, les tickets naissent dans cairn (EIDO-1, EIDO-2…)
  cairn projects   liste les projets suivis
  cairn open       affiche cairn dans son propre espace (session Rex), ou revient à
                   celui d'avant s'il est déjà devant (touche herdr prefix+a, app Herdr cmd+shift+a,
                   Rex ctrl+b a)
  cairn sync [TICKET]
                   pousse la phase de chaque chantier dans la barre latérale, et
                   vérifie le plan du chantier courant (ou de TICKET)
                   (appelé par les agents après avoir modifié leur fichier de chantier)
  cairn guard      hook Claude Code (PreToolUse) : sur un chantier, bloque git push tant que
                   le push n'est pas autorisé, gh pr create tant que la PR ne l'est pas,
                   et gh pr merge toujours
  cairn guard prompt
                   hook Claude Code (UserPromptSubmit) : quand tu demandes un push dans
                   le chat, l'autorise comme le bouton de cairn
  cairn ensure     crée l'espace cairn s'il manque et y lance cairn, sans changer le focus
                   (hook de démarrage du plugin herdr : le bouton reste dans la barre latérale)

herdr, l'app Herdr ou Rex : celui où cairn tourne, ou "mux" dans la config, ou
CAIRN_MUX=herdr|herdr-app|rex.

Config : ~/.cairn/config.json · Chantiers : ~/.cairn/chantiers/<TICKET>.md
`, `cairn — where each piece of work stands, next to herdr or Rex.

  cairn            opens the TUI (all keyboard driven, keys shown at the bottom)
  cairn add [folder] [--tracker local|linear|sprintosaur] [--key EIDO] [--name eido]
                   adds the repository (default: the one you are in) to the projects;
                   without a tracker, tickets are born in cairn (EIDO-1, EIDO-2…)
  cairn projects   lists the projects
  cairn open       shows cairn in its own workspace (Rex session), or goes back to the
                   previous one if cairn is already in front (herdr prefix+a, Herdr app
                   cmd+shift+a, Rex ctrl+b a)
  cairn sync [TICKET]
                   pushes each ticket's phase to the sidebar, and checks the plan
                   of the current ticket (or of TICKET)
                   (called by agents after they edit their ticket file)
  cairn guard      Claude Code hook (PreToolUse): on a ticket, blocks git push until the
                   push is allowed, gh pr create until the PR is, and gh pr merge always
  cairn guard prompt
                   Claude Code hook (UserPromptSubmit): when you ask for a push in the
                   chat, allows it like cairn's button
  cairn ensure     creates the cairn workspace if missing and runs cairn in it, without
                   moving focus (herdr plugin startup hook: the button stays in the sidebar)

herdr, the Herdr app or Rex: the one cairn runs in, or "mux" in the config, or
CAIRN_MUX=herdr|herdr-app|rex.

Config: ~/.cairn/config.json · Tickets: ~/.cairn/chantiers/<TICKET>.md
`)
}

func main() {
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage()) }
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fail(err)
	}
	i18n.Set(cfg.Lang)
	mux.Use(cfg.Mux)
	if flag.Arg(0) == "guard" && flag.Arg(1) == "prompt" {
		// Claude Code UserPromptSubmit hook: what it prints joins the agent's context.
		if note := guard.CheckPrompt(os.Stdin, cfg.Keys(), config.ChantiersDir(), config.Dir()); note != "" {
			fmt.Println(note)
		}
		return
	}
	if flag.Arg(0) == "guard" {
		// Claude Code PreToolUse hook: exit 2 blocks the command and shows the reason to the agent.
		if reason := guard.Check(os.Stdin, cfg.Keys(), config.ChantiersDir()); reason != "" {
			fmt.Fprintln(os.Stderr, reason)
			os.Exit(2)
		}
		return
	}
	switch flag.Arg(0) {
	case "add":
		if err := add(flag.Args()[1:]); err != nil {
			fail(err)
		}
		return
	case "projects":
		listProjects(cfg)
		return
	}
	// The TUI opens anyway, on its settings, to fix what is wrong.
	checkErr := cfg.Check()
	if checkErr != nil && flag.Arg(0) != "" {
		fail(checkErr)
	}
	if !mux.Available() {
		fail(fmt.Errorf("%s", mux.NotFound()))
	}

	switch flag.Arg(0) {
	case "":
		m := tui.New(cfg).WithUpdates(updates())
		if checkErr != nil {
			m = m.StartInSettings(checkErr)
		}
		final, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseAllMotion(), tea.WithReportFocus()).Run()
		if err != nil {
			fail(err)
		}
		if fm, ok := final.(tui.Model); ok && fm.UpdateRequested() {
			selfUpdate()
		}
	case "open":
		if err := home.Toggle(); err != nil {
			fail(err)
		}
	case "img":
		fs := flag.NewFlagSet("img", flag.ExitOnError)
		start := fs.Int("start", 0, i18n.T("première image", "first image"))
		_ = fs.Parse(flag.Args()[1:])
		files := fs.Args()
		caps := strings.Split(os.Getenv("CAIRN_CAPTIONS"), "\n")
		for len(caps) < len(files) {
			caps = append(caps, "")
		}
		if len(files) == 0 {
			fail(fmt.Errorf("%s", i18n.T("aucune image", "no image")))
		}
		if err := images.Show(files, caps, *start); err != nil {
			fail(err)
		}
		return
	case "ensure":
		if err := home.Ensure(); err != nil {
			fail(err)
		}
	case "sync":
		// The plan is checked first: an agent that cannot reach herdr or Rex must
		// still hear that its plan does not show as it meant.
		report := lintPlan(cfg, flag.Arg(1))
		err := sync(cfg)
		if report != "" {
			fmt.Print(report)
			if err != nil {
				fmt.Fprintln(os.Stderr, i18n.T("cairn :", "cairn:"), err)
			}
			os.Exit(1)
		}
		if err != nil {
			fail(err)
		}
	default:
		flag.Usage()
		os.Exit(2)
	}
}

// lintPlan checks the plan of the ticket key, or of the chantier checked out
// in the current directory, and says what to fix; "" when it is fine.
func lintPlan(cfg config.Config, key string) string {
	var s *state.State
	if key != "" {
		key = strings.ToUpper(key)
		s, _ = state.Load(state.PathFor(config.ChantiersDir(), key))
	} else if dir, err := os.Getwd(); err == nil {
		key, s = guard.PlanAt(dir, cfg.Keys(), config.ChantiersDir())
	}
	if s == nil {
		return ""
	}
	problems := s.Lint()
	if len(problems) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, i18n.T("cairn : le plan de %s (%s) ne s'affiche pas comme tu l'as voulu. Corrige ces points, puis relance `cairn sync` :\n",
		"cairn: the plan of %s (%s) does not show the way you meant. Fix these points, then run `cairn sync` again:\n"), key, s.Path)
	for _, p := range problems {
		fmt.Fprintf(&b, "  - %s\n", p)
	}
	return b.String()
}

// sync pushes sidebar tokens from local state only (no network).
func sync(cfg config.Config) error {
	snap, err := mux.GetSnapshot()
	if err != nil {
		return err
	}
	var sources []chantier.Source
	for _, p := range cfg.Projects {
		wts, err := mux.ListWorktrees(p.Repo)
		if err != nil {
			return err
		}
		sources = append(sources, chantier.Source{Project: p.Name, Key: p.Key, Repo: p.Repo, Worktrees: wts})
	}
	list, _ := chantier.Gather(chantier.Input{ChantiersDir: config.ChantiersDir(), Sources: sources, Snapshot: snap})
	for ws, v := range tui.TokensFor(list, snap) {
		if err := mux.ReportToken(ws, v); err != nil {
			return err
		}
	}
	return nil
}

// add follows a new repository: cairn add [dir] [--tracker …] [--key …] [--name …].
func add(args []string) error {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	var p config.Project
	fs.StringVar(&p.Tracker, "tracker", "local", i18n.T("d'où viennent les tickets : local, linear ou sprintosaur", "where tickets come from: local, linear or sprintosaur"))
	fs.StringVar(&p.Key, "key", "", i18n.T("préfixe des tickets (clé d'équipe avec Linear ou Sprintosaur) ; par défaut le nom du dossier", "ticket prefix (team key with Linear or Sprintosaur); default: the folder name"))
	fs.StringVar(&p.Name, "name", "", i18n.T("nom court du projet", "short name of the project"))
	fs.StringVar(&p.Credentials, "credentials", "", i18n.T("fichier KEY=VALUE du jeton du tracker", "KEY=VALUE file with the tracker's token"))
	fs.StringVar(&p.Base, "base", "", i18n.T("branche de départ (par défaut celle du remote)", "base branch (default: the remote's)"))
	// Flags may come before or after the folder.
	_ = fs.Parse(args)
	p.Repo = "."
	if rest := fs.Args(); len(rest) > 0 {
		p.Repo = rest[0]
		_ = fs.Parse(rest[1:])
	}
	switch p.Tracker {
	case "local", "linear", "sprintosaur":
	default:
		return fmt.Errorf(i18n.T("tracker inconnu %q (local, linear ou sprintosaur)", "unknown tracker %q (local, linear or sprintosaur)"), p.Tracker)
	}
	if p.Tracker != "local" && p.Key == "" {
		return fmt.Errorf("%s", i18n.T("avec un tracker, --key donne la clé d'équipe (ex. --key ENG)", "with a tracker, --key gives the team key (e.g. --key ENG)"))
	}
	full, err := config.AddProject(p)
	if err != nil {
		return err
	}
	fmt.Printf(i18n.T("✓ projet %s ajouté : %s, tickets %s-…, tracker %s\n", "✓ project %s added: %s, tickets %s-…, tracker %s\n"), full.Name, full.Repo, full.Key, full.Tracker)
	if full.Tracker == "linear" && os.Getenv("LINEAR_API_KEY") == "" && full.Credentials == "" {
		fmt.Println(i18n.T("! Linear : exporte LINEAR_API_KEY (clé personnelle) ou passe --credentials <fichier>",
			"! Linear: export LINEAR_API_KEY (personal key) or pass --credentials <file>"))
	}
	return nil
}

func listProjects(cfg config.Config) {
	if len(cfg.Projects) == 0 {
		fmt.Println(i18n.T("Aucun projet : lance « cairn add » dans un dépôt.", "No project: run \"cairn add\" in a repository."))
		return
	}
	for _, p := range cfg.Projects {
		fmt.Printf("%-10s %-8s %-12s %s\n", p.Name, p.Key+"-", p.Tracker, p.Repo)
	}
}

// updates says how this build checks for updates. CAIRN_NO_UPDATE_CHECK turns
// the check off.
func updates() tui.Updates {
	rev := buildRev
	if sourceDir == "" {
		if r, modified, stamped := update.Build(); stamped && !modified {
			rev = r
		}
	}
	if os.Getenv("CAIRN_NO_UPDATE_CHECK") != "" {
		rev = ""
	}
	return tui.Updates{File: filepath.Join(config.Dir(), "cache", "update.json"), Rev: rev, SourceDir: sourceDir, Compare: update.GitHub}
}

// selfUpdate installs the latest main from the clone, then becomes the new
// cairn in the same pane. On failure the installed cairn is unchanged, and it
// starts again once the error is read.
func selfUpdate() {
	fmt.Println(i18n.T("↑ Mise à jour de cairn depuis ", "↑ Updating cairn from ") + sourceDir + "\n")
	if err := update.Install(sourceDir, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "\n"+i18n.T("✗ mise à jour échouée : ", "✗ update failed: ")+err.Error())
		fmt.Fprint(os.Stderr, i18n.T("cairn n'a pas changé. Entrée pour le relancer… ", "cairn was not changed. Enter to start it again… "))
		fmt.Scanln()
	}
	self, err := os.Executable()
	if err != nil {
		fail(err)
	}
	if err := syscall.Exec(self, os.Args, os.Environ()); err != nil {
		fail(fmt.Errorf(i18n.T("relance cairn à la main (%v)", "start cairn again by hand (%v)"), err))
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, i18n.T("cairn :", "cairn:"), err)
	os.Exit(1)
}
