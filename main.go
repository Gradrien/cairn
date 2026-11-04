// cairn: where each piece of work stands, next to herdr.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"cairn/internal/chantier"
	"cairn/internal/config"
	"cairn/internal/guard"
	"cairn/internal/herdr"
	"cairn/internal/home"
	"cairn/internal/i18n"
	"cairn/internal/images"
	"cairn/internal/tui"
)

func usage() string {
	return i18n.T(`cairn — où en est chaque chantier, à côté de herdr.

  cairn            ouvre la TUI (tout se fait au clavier, touches affichées en bas)
  cairn open       affiche cairn dans son propre espace herdr, ou revient à l'espace
                   d'avant s'il est déjà devant (c'est la touche herdr prefix+a)
  cairn sync       pousse la phase de chaque chantier dans la barre latérale herdr
                   (appelé par les agents après avoir modifié leur fichier de chantier)
  cairn guard      hook Claude Code (PreToolUse) : bloque git push / gh pr create sur un
                   chantier tant que le push n'a pas été autorisé dans cairn
  cairn ensure     crée l'espace cairn s'il manque et y lance cairn, sans changer le focus
                   (hook de démarrage du plugin herdr : le bouton reste dans la barre latérale)

Config : ~/.cairn/config.json · Chantiers : ~/.cairn/chantiers/<TICKET>.md
`, `cairn — where each piece of work stands, next to herdr.

  cairn            opens the TUI (all keyboard driven, keys shown at the bottom)
  cairn open       shows cairn in its own herdr workspace, or goes back to the previous
                   workspace if cairn is already in front (herdr key prefix+a)
  cairn sync       pushes each ticket's phase to herdr's sidebar
                   (called by agents after they edit their ticket file)
  cairn guard      Claude Code hook (PreToolUse): blocks git push / gh pr create on a
                   ticket until the push is allowed in cairn
  cairn ensure     creates the cairn workspace if missing and runs cairn in it, without
                   moving focus (herdr plugin startup hook: the button stays in the sidebar)

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
	if flag.Arg(0) == "guard" {
		// Claude Code PreToolUse hook: exit 2 blocks the command and shows the reason to the agent.
		if reason := guard.Check(os.Stdin, cfg.Team, config.ChantiersDir()); reason != "" {
			fmt.Fprintln(os.Stderr, reason)
			os.Exit(2)
		}
		return
	}
	if err := cfg.Check(); err != nil {
		fail(err)
	}
	if !herdr.Available() {
		fail(fmt.Errorf("%s", i18n.T("herdr est introuvable dans le PATH", "herdr is not in the PATH")))
	}

	switch flag.Arg(0) {
	case "":
		if _, err := tea.NewProgram(tui.New(cfg), tea.WithAltScreen(), tea.WithMouseAllMotion(), tea.WithReportFocus()).Run(); err != nil {
			fail(err)
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
		if err := sync(cfg); err != nil {
			fail(err)
		}
	default:
		flag.Usage()
		os.Exit(2)
	}
}

// sync pushes sidebar tokens from local state only (no network).
func sync(cfg config.Config) error {
	snap, err := herdr.GetSnapshot()
	if err != nil {
		return err
	}
	wts, err := herdr.ListWorktrees(cfg.Repo)
	if err != nil {
		return err
	}
	list, _ := chantier.Gather(chantier.Input{
		Team: cfg.Team, Repo: cfg.Repo, ChantiersDir: config.ChantiersDir(), Worktrees: wts, Snapshot: snap,
	})
	for ws, v := range tui.TokensFor(list, snap) {
		if err := herdr.ReportToken(ws, v); err != nil {
			return err
		}
	}
	return nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, i18n.T("cairn :", "cairn:"), err)
	os.Exit(1)
}
