package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"

	"cairn/internal/config"
	"cairn/internal/i18n"
	"cairn/internal/mux"
	"cairn/internal/tracker"
)

// The settings view edits ~/.cairn/config.json: every project (repository,
// key, tracker…), adding and removing projects, and the global settings.
// Each change is checked, saved and applied at once.

type settingsState struct {
	raw     config.Config    // the file as written, without defaults
	filled  []config.Project // its projects with defaults, for display
	cursor  int              // index in rows()
	top     int              // first body line shown
	editing bool
	input   string
	confirm int // row index waiting for a second enter to delete, -1
	hover   int
	checks  map[string]*connCheck // Linear connection of each project, by name
}

// connCheck is a live test of a project's Linear key and team.
type connCheck struct {
	loading bool
	conn    tracker.Connection
	err     error
}

type connCheckMsg struct {
	name string
	conn tracker.Connection
	err  error
}

// checkConnections tests every Linear project that has a key.
func (m Model) checkConnections() tea.Cmd {
	s := m.settings
	if s == nil {
		return nil
	}
	var cmds []tea.Cmd
	for _, p := range m.projects {
		l, ok := p.tr.(*tracker.Linear)
		if !ok {
			delete(s.checks, p.cfg.Name)
			continue
		}
		name := p.cfg.Name
		s.checks[name] = &connCheck{loading: true}
		cmds = append(cmds, func() tea.Msg {
			c, err := l.Check()
			return connCheckMsg{name: name, conn: c, err: err}
		})
	}
	return tea.Batch(cmds...)
}

// setRow is one line of the settings: a field of a project (proj ≥ 0) or a
// global one (proj -1). field "" is a heading, never selected.
type setRow struct {
	proj  int
	field string
}

var trackers = []string{"local", "linear", "sprintosaur"}

func (m Model) openSettings() (tea.Model, tea.Cmd) {
	raw, err := config.Read()
	if err != nil {
		m.say("", err)
		return m, nil
	}
	m.view = "settings"
	m.settings = &settingsState{raw: raw, confirm: -1, hover: -1, checks: map[string]*connCheck{}}
	m.settings.refill()
	// Start on the selected chantier's project.
	if p := m.projectOf(m.selected()); p != nil {
		for i, r := range m.settings.rows() {
			if r.field == "repo" && i < len(m.settings.rows()) && m.settings.filled[r.proj].Name == p.cfg.Name {
				m.settings.cursor = i
			}
		}
	}
	if m.settings.cursor == 0 {
		m.settings.cursor = m.settings.next(0, 1)
	}
	return m, m.checkConnections()
}

func (s *settingsState) refill() {
	s.filled = nil
	for _, p := range s.raw.Projects {
		s.filled = append(s.filled, s.raw.Fill(p))
	}
}

func (s *settingsState) rows() []setRow {
	out := []setRow{{-1, ""}, {-1, "lang"}, {-1, "agent_kind"}, {-1, "model_plan"}, {-1, "model_build"}, {-1, "diff"}}
	for i, p := range s.filled {
		out = append(out, setRow{i, ""}, setRow{i, "name"}, setRow{i, "repo"}, setRow{i, "key"}, setRow{i, "tracker"})
		switch p.Tracker {
		case "linear":
			out = append(out, setRow{i, "api_key"}, setRow{i, "credentials"})
		case "sprintosaur":
			out = append(out, setRow{i, "credentials"}, setRow{i, "assignee"}, setRow{i, "web"})
		}
		out = append(out, setRow{i, "base"}, setRow{i, "branch_prefix"}, setRow{i, "worktree_parent"},
			setRow{i, "agent_kind"}, setRow{i, "model_plan"}, setRow{i, "model_build"}, setRow{i, "diff"}, setRow{i, "delete"})
	}
	return append(out, setRow{-2, ""}, setRow{-2, "add"})
}

// next is the first selectable row from i going in dir.
func (s *settingsState) next(i, dir int) int {
	rows := s.rows()
	for j := i; j >= 0 && j < len(rows); j += dir {
		if rows[j].field != "" {
			return j
		}
	}
	return s.cursor
}

func fieldLabel(f string) string {
	switch f {
	case "lang":
		return i18n.T("Langue", "Language")
	case "agent_kind":
		return i18n.T("Agent", "Agent")
	case "diff":
		return i18n.T("Commande de diff", "Diff command")
	case "model_plan":
		return i18n.T("Modèle du plan", "Plan model")
	case "model_build":
		return i18n.T("Modèle du build", "Build model")
	case "name":
		return i18n.T("Nom", "Name")
	case "repo":
		return i18n.T("Dépôt", "Repository")
	case "key":
		return i18n.T("Clé des tickets", "Ticket key")
	case "tracker":
		return "Tracker"
	case "api_key":
		return i18n.T("Clé d'API", "API key")
	case "credentials":
		return i18n.T("Fichier d'identifiants", "Credentials file")
	case "assignee":
		return i18n.T("Ton handle", "Your handle")
	case "web":
		return i18n.T("URL des tickets", "Ticket URL")
	case "base":
		return i18n.T("Branche de base", "Base branch")
	case "branch_prefix":
		return i18n.T("Préfixe de branche", "Branch prefix")
	case "worktree_parent":
		return i18n.T("Dossier des worktrees", "Worktrees folder")
	case "delete":
		return i18n.T("Retirer ce projet", "Remove this project")
	case "add":
		return i18n.T("+ Ajouter un projet", "+ Add a project")
	}
	return f
}

// fieldHint explains the selected field.
func fieldHint(r setRow, tracker string) string {
	switch r.field {
	case "lang":
		return i18n.T("fr ou en", "fr or en")
	case "agent_kind":
		return mux.Words(i18n.T("le genre d'agent herdr lancé pour un plan (claude, codex…)", "the herdr agent kind started for a plan (claude, codex…)"))
	case "diff":
		return i18n.T("lancée dans le worktree par d ; {base} est remplacé. Vide : browsediff s'il est installé, sinon git diff",
			"run in the worktree by d; {base} is replaced. Empty: browsediff when installed, else git diff")
	case "model_plan":
		return i18n.T("l'agent Claude qui planifie démarre dessus (claude --model) : opus, sonnet, fable… Vide : le modèle par défaut de Claude",
			"the Claude agent that plans starts on it (claude --model): opus, sonnet, fable… Empty: Claude's default model")
	case "model_build":
		return i18n.T("l'agent neuf qui construit après ta validation démarre dessus : un modèle moins cher, sonnet par exemple, construit ce qu'un plus fort a planifié. Vide : le modèle par défaut de Claude",
			"the new agent that builds once you approve starts on it: a cheaper model, sonnet for instance, builds what a stronger one planned. Empty: Claude's default model")
	case "repo":
		return i18n.T("le checkout principal ; ses worktrees sont les chantiers", "the main checkout; its worktrees are the chantiers")
	case "key":
		if tracker == "local" {
			return i18n.T("préfixe des tickets (EIDO-1). Les chantiers existants gardent leur ancienne clé.", "ticket prefix (EIDO-1). Existing chantiers keep their old key.")
		}
		return i18n.T("la clé d'équipe dans le tracker (ENG). Les chantiers existants gardent leur ancienne clé.", "the team key in the tracker (ENG). Existing chantiers keep their old key.")
	case "tracker":
		return i18n.T("entrée ou ←→ : local (aucun outil), linear, sprintosaur", "enter or ←→: local (no tool), linear, sprintosaur")
	case "api_key":
		return i18n.T("entrée puis colle la clé (lin_api_…) : cairn la range dans ~/.cairn/credentials, lisible par toi seul",
			"enter then paste the key (lin_api_…): cairn keeps it in ~/.cairn/credentials, readable by you only")
	case "credentials":
		if tracker == "linear" {
			return i18n.T("fichier KEY=VALUE contenant LINEAR_API_KEY=lin_api_… (Linear › Settings › Security & access › Personal API keys), ou exporte LINEAR_API_KEY",
				"KEY=VALUE file with LINEAR_API_KEY=lin_api_… (Linear › Settings › Security & access › Personal API keys), or export LINEAR_API_KEY")
		}
		return i18n.T("fichier KEY=VALUE avec SPRINTOSAUR_API et SPRINTOSAUR_TOKEN", "KEY=VALUE file with SPRINTOSAUR_API and SPRINTOSAUR_TOKEN")
	case "base":
		return i18n.T("d'où partent les nouvelles branches, et ce que le diff compare", "where new branches start, and what the diff compares to")
	case "delete":
		return i18n.T("ne touche ni au dépôt ni aux chantiers : cairn arrête seulement de le suivre", "touches neither the repository nor the chantiers: cairn just stops following it")
	case "add":
		return i18n.T("le chemin d'un dépôt git ; il démarre sans tracker, à changer ensuite", "a git repository's path; it starts without a tracker, change it next")
	}
	return i18n.T("vide : la valeur par défaut", "empty: the default")
}

// get is the value as written in the file.
func (s *settingsState) get(r setRow) string {
	if r.proj < 0 {
		switch r.field {
		case "lang":
			return s.raw.Lang
		case "agent_kind":
			return s.raw.AgentKind
		case "diff":
			return s.raw.Diff
		case "model_plan":
			return s.raw.ModelPlan
		case "model_build":
			return s.raw.ModelBuild
		}
		return ""
	}
	return projectField(&s.raw.Projects[r.proj], r.field, nil)
}

// shown is the value in effect: the file's, else the default.
func (s *settingsState) shown(r setRow) (string, bool) {
	if v := s.get(r); v != "" || r.proj < 0 {
		if r.proj < 0 && v == "" {
			switch r.field {
			case "lang":
				return "fr", true
			case "agent_kind":
				return "claude", true
			case "diff":
				return "browsediff / git diff", true
			case "model_plan", "model_build":
				return i18n.T("celui de Claude par défaut", "Claude's default"), true
			}
		}
		return v, false
	}
	if r.field == "api_key" {
		switch src, tail := tracker.LinearKey(s.filled[r.proj]); src {
		case "env":
			return "…" + tail + i18n.T("  (variable LINEAR_API_KEY de ton terminal)", "  (your terminal's LINEAR_API_KEY)"), false
		case "file":
			return "…" + tail, false
		}
		return i18n.T("aucune : entrée pour la coller", "none: enter to paste it"), true
	}
	v := projectField(&s.filled[r.proj], r.field, nil)
	if r.field == "diff" && v == "" {
		v = "browsediff / git diff"
	}
	if (r.field == "model_plan" || r.field == "model_build") && v == "" {
		v = i18n.T("celui de Claude par défaut", "Claude's default")
	}
	return v, true
}

// projectField reads a field, or writes it when set is not nil.
func projectField(p *config.Project, field string, set *string) string {
	var f *string
	switch field {
	case "name":
		f = &p.Name
	case "repo":
		f = &p.Repo
	case "key":
		f = &p.Key
	case "tracker":
		f = &p.Tracker
	case "credentials":
		f = &p.Credentials
	case "assignee":
		f = &p.Assignee
	case "web":
		f = &p.Web
	case "base":
		f = &p.Base
	case "branch_prefix":
		f = &p.BranchPrefix
	case "worktree_parent":
		f = &p.WorktreeParent
	case "agent_kind":
		f = &p.AgentKind
	case "diff":
		f = &p.Diff
	case "model_plan":
		f = &p.ModelPlan
	case "model_build":
		f = &p.ModelBuild
	default:
		return ""
	}
	if set != nil {
		*f = *set
	}
	return *f
}

// commit writes one change, saves and applies it; on a refusal nothing changes.
func (m Model) settingsCommit(r setRow, value string) (tea.Model, tea.Cmd) {
	s := m.settings
	next := s.raw
	next.Projects = append([]config.Project(nil), s.raw.Projects...)
	value = strings.TrimSpace(value)
	switch {
	case r.field == "add":
		if value == "" {
			return m, nil
		}
		// Name and key are written down: changing one later leaves the other.
		np := config.Project{Repo: absPath(value)}
		full := next.Fill(np)
		np.Name, np.Key, np.Tracker = full.Name, full.Key, full.Tracker
		next.Projects = append(next.Projects, np)
	case r.field == "delete":
		next.Projects = append(next.Projects[:r.proj], next.Projects[r.proj+1:]...)
	case r.field == "api_key":
		if value == "" {
			return m, nil
		}
		path, err := config.SaveSecret(s.filled[r.proj].Name, "LINEAR_API_KEY", value)
		if err != nil {
			m.say("", err)
			return m, nil
		}
		next.Projects[r.proj].Credentials = tildePath(path)
	case r.proj < 0:
		switch r.field {
		case "lang":
			next.Lang = value
		case "agent_kind":
			next.AgentKind = value
		case "diff":
			next.Diff = value
		case "model_plan":
			next.ModelPlan = value
		case "model_build":
			next.ModelBuild = value
		}
	default:
		switch r.field {
		case "key":
			value = strings.ToUpper(value)
		case "repo":
			if value != "" {
				value = absPath(value)
			}
		}
		projectField(&next.Projects[r.proj], r.field, &value)
	}
	if err := config.Save(next); err != nil {
		m.say("", err)
		return m, nil
	}
	s.raw = next
	s.refill()
	// A tracker that needs a key: go to the key.
	if r.field == "tracker" && value == "linear" {
		if src, _ := tracker.LinearKey(s.filled[r.proj]); src == "" {
			for i, row := range s.rows() {
				if row.proj == r.proj && row.field == "api_key" {
					s.cursor = i
				}
			}
		}
	}
	if rows := s.rows(); s.cursor >= len(rows) || rows[s.cursor].field == "" {
		s.cursor = s.next(min(s.cursor, len(rows)-1), -1)
	}
	if r.field == "add" {
		// Show the new project: its tracker is the next thing to choose.
		for i, row := range s.rows() {
			if row.proj == len(s.filled)-1 && row.field == "tracker" {
				s.cursor = i
			}
		}
	}
	m.say(i18n.T("Réglages enregistrés.", "Settings saved."), nil)
	next2, cmd := m.applyConfig()
	nm := next2.(Model)
	cmd = tea.Batch(cmd, nm.checkConnections())
	// Say at once when the tracker cannot work (a missing API key…).
	if r.proj >= 0 && r.proj < len(s.filled) {
		var nk *tracker.NeedsKey
		if err := nm.trackerErr(s.filled[r.proj].Name); err != nil && !errors.As(err, &nk) {
			nm.say(i18n.T("Enregistré, mais ", "Saved, but ")+err.Error(), nil)
			nm.flashErr = true
		}
	}
	return nm, cmd
}

// trackerErr is why a project's tracker cannot open, nil if it can.
func (m Model) trackerErr(name string) error {
	for _, p := range m.projects {
		if p.cfg.Name == name && p.tr == nil {
			return p.err
		}
	}
	return nil
}

func absPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		return p // kept readable; expanded when loaded
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// applyConfig reloads the config and follows the new projects.
func (m Model) applyConfig() (tea.Model, tea.Cmd) {
	cfg, err := config.Load()
	if err != nil {
		m.say("", err)
		return m, nil
	}
	m.cfg = cfg
	if cfg.Lang != i18n.Lang() {
		i18n.Set(cfg.Lang)
	}
	m.projects = newProjects(cfg)
	m.loaded = false
	return m, tea.Batch(m.rebuild(), m.loadLocal(), m.loadIssues())
}

func cycle(list []string, cur string, dir int) string {
	for i, v := range list {
		if v == cur {
			return list[(i+dir+len(list))%len(list)]
		}
	}
	return list[0]
}

func (m Model) settingsKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := m.settings
	rows := s.rows()
	r := rows[s.cursor]
	m.flash = ""
	if s.editing {
		switch k.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEsc:
			s.editing = false
		case tea.KeyEnter:
			s.editing = false
			return m.settingsCommit(r, s.input)
		case tea.KeyBackspace:
			if rs := []rune(s.input); len(rs) > 0 {
				s.input = string(rs[:len(rs)-1])
			}
		case tea.KeyCtrlU:
			s.input = ""
		case tea.KeySpace:
			s.input += " "
		case tea.KeyRunes:
			s.input += string(k.Runes)
		}
		return m, nil
	}
	if k.Type != tea.KeyEnter {
		s.confirm = -1
	}
	switch k.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q", "s":
		m.view = ""
		return m, nil
	case "r":
		m.say(i18n.T("Vérification des connexions…", "Checking the connections…"), nil)
		return m, m.checkConnections()
	case "up", "k":
		s.cursor = s.next(s.cursor-1, -1)
	case "down", "j":
		s.cursor = s.next(s.cursor+1, 1)
	case "left", "right":
		dir := 1
		if k.String() == "left" {
			dir = -1
		}
		switch {
		case r.field == "tracker":
			return m.settingsCommit(r, cycle(trackers, s.filled[r.proj].Tracker, dir))
		case r.field == "lang" && r.proj < 0:
			cur, _ := s.shown(r)
			return m.settingsCommit(r, cycle([]string{"fr", "en"}, cur, dir))
		}
	case "enter", " ":
		return m.settingsActivate(s.cursor)
	}
	return m, nil
}

// settingsActivate does what enter does on a row: edit, cycle, or delete.
func (m Model) settingsActivate(i int) (tea.Model, tea.Cmd) {
	s := m.settings
	r := s.rows()[i]
	s.cursor = i
	switch {
	case r.field == "":
		return m, nil
	case r.field == "tracker":
		return m.settingsCommit(r, cycle(trackers, s.filled[r.proj].Tracker, 1))
	case r.field == "lang" && r.proj < 0:
		cur, _ := s.shown(r)
		return m.settingsCommit(r, cycle([]string{"fr", "en"}, cur, 1))
	case r.field == "delete":
		if s.confirm == i {
			s.confirm = -1
			return m.settingsCommit(r, "")
		}
		s.confirm = i
		m.say(fmt.Sprintf(i18n.T("Entrée encore pour retirer %s des projets suivis.", "Enter again to remove %s from the projects."), s.filled[r.proj].Name), nil)
		return m, nil
	}
	s.editing, s.input = true, s.get(r)
	if r.field == "api_key" {
		s.input = ""
	}
	return m, nil
}

func tildePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home+"/") {
		return "~/" + p[len(home)+1:]
	}
	return p
}

// guide says what to do for a project's tracker to work, step by step, or
// confirms it works. Empty for a project without a tracker.
func (m Model) guide(i int) []string {
	s := m.settings
	p := s.filled[i]
	ok := func(t string) string { return sGood.Render("  ✓ ") + t }
	warn := func(t string) string { return sWarn.Render("  ⚠ ") + t }
	step := func(n int, t string) string { return sKey.Render(fmt.Sprintf("    %d. ", n)) + t }
	switch p.Tracker {
	case "linear":
		src, _ := tracker.LinearKey(p)
		if src == "" {
			return []string{
				warn(i18n.T("Linear n'est pas encore connecté. Pour le connecter :", "Linear is not connected yet. To connect it:")),
				step(1, i18n.T("dans Linear : Settings › Security & access › Personal API keys › New API key (accès lecture et écriture)",
					"in Linear: Settings › Security & access › Personal API keys › New API key (read and write access)")),
				step(2, i18n.T("ici : sélectionne « Clé d'API », entrée, colle la clé, entrée", "here: select \"API key\", enter, paste the key, enter")),
				step(3, fmt.Sprintf(i18n.T("« Clé des tickets » : la clé de ton équipe Linear, le préfixe de ses tickets (ENG pour ENG-123). Actuellement : %s",
					"\"Ticket key\": your Linear team's key, the prefix of its tickets (ENG for ENG-123). Now: %s"), p.Key)),
			}
		}
		c := s.checks[p.Name]
		switch {
		case c == nil || c.loading:
			return []string{sMuted.Render(i18n.T("  … vérification de la connexion à Linear", "  … checking the Linear connection"))}
		case errors.Is(c.err, tracker.ErrKeyRefused):
			out := []string{warn(i18n.T("Linear refuse cette clé (révoquée, mal copiée ?).", "Linear refuses this key (revoked, badly copied?)."))}
			if src == "env" {
				out = append(out, step(1, i18n.T("elle vient de la variable LINEAR_API_KEY de ton terminal, qui passe avant tout : corrige-la ou retire-la de ton ~/.zshrc",
					"it comes from your terminal's LINEAR_API_KEY, which wins over everything: fix it or remove it from your ~/.zshrc")))
			} else {
				out = append(out, step(1, i18n.T("crée une nouvelle clé dans Linear : Settings › Security & access › Personal API keys",
					"create a new key in Linear: Settings › Security & access › Personal API keys")),
					step(2, i18n.T("colle-la dans « Clé d'API » (entrée)", "paste it in \"API key\" (enter)")))
			}
			return out
		case c.err != nil:
			return []string{warn(i18n.T("Linear ne répond pas : ", "Linear does not answer: ") + c.err.Error()),
				sMuted.Render(i18n.T("    vérifie ta connexion ; cairn réessaie à la prochaine modification (ou r)", "    check your connection; cairn tries again on the next change (or r)"))}
		case c.conn.Team == nil:
			var keys []string
			for _, t := range c.conn.Teams {
				keys = append(keys, t.Key+" ("+t.Name+")")
			}
			out := []string{warn(fmt.Sprintf(i18n.T("Connecté en tant que %s, mais Linear n'a pas d'équipe %s.", "Connected as %s, but Linear has no team %s."), c.conn.User, p.Key))}
			if len(keys) > 0 {
				out = append(out, step(1, fmt.Sprintf(i18n.T("mets l'une de tes équipes dans « Clé des tickets » : %s", "put one of your teams in \"Ticket key\": %s"), strings.Join(keys, ", "))))
			} else {
				out = append(out, step(1, i18n.T("cette clé ne voit aucune équipe : vérifie l'espace de travail où tu l'as créée", "this key sees no team: check the workspace you created it in")))
			}
			return out
		default:
			return []string{ok(fmt.Sprintf(i18n.T("Connecté à Linear en tant que %s · équipe %s (%s)", "Connected to Linear as %s · team %s (%s)"), c.conn.User, c.conn.Team.Key, c.conn.Team.Name))}
		}
	case "sprintosaur":
		if err := m.trackerErr(p.Name); err != nil {
			return []string{
				warn(i18n.T("Sprintosaur n'est pas connecté : ", "Sprintosaur is not connected: ") + err.Error()),
				step(1, i18n.T("installe le skill sprintosaur : ses identifiants sont dans ~/.claude/skills/sprintosaur/credentials.env",
					"install the sprintosaur skill: its credentials live in ~/.claude/skills/sprintosaur/credentials.env")),
				step(2, i18n.T("ou indique dans « Fichier d'identifiants » un fichier avec SPRINTOSAUR_API=… et SPRINTOSAUR_TOKEN=…",
					"or point \"Credentials file\" to a file with SPRINTOSAUR_API=… and SPRINTOSAUR_TOKEN=…")),
			}
		}
		return []string{ok(i18n.T("Sprintosaur : identifiants trouvés", "Sprintosaur: credentials found"))}
	}
	return nil
}

// settingsLines renders the body and the row index of each line (-1 for none).
func (m Model) settingsLines() ([]string, []int) {
	s := m.settings
	var lines []string
	var owners []int
	labelW := 24
	for i, r := range s.rows() {
		if r.field == "" {
			title := i18n.T("Général", "General")
			switch {
			case r.proj >= 0:
				p := s.filled[r.proj]
				title = i18n.T("Projet ", "Project ") + p.Name + sMuted.Render(fmt.Sprintf("  %s-… · %s", p.Key, p.Tracker))
			case r.proj == -2:
				title = ""
			}
			if len(lines) > 0 {
				lines, owners = append(lines, ""), append(owners, -1)
			}
			if title != "" {
				lines, owners = append(lines, sSection.Render(title)), append(owners, -1)
			}
			if r.proj >= 0 {
				for _, g := range m.guide(r.proj) {
					lines, owners = append(lines, g), append(owners, -1)
				}
			}
			continue
		}
		var line string
		switch r.field {
		case "add":
			line = "  " + sKey.Render(fieldLabel(r.field))
			if s.editing && i == s.cursor {
				line = "  " + sKey.Render(i18n.T("Dépôt : ", "Repository: ")) + s.input + "▏"
			}
		case "delete":
			style := sMuted
			if s.confirm == i {
				style = sErr
			}
			line = "  " + style.Render(fieldLabel(r.field))
		default:
			v, isDefault := s.shown(r)
			if r.field == "repo" || r.field == "credentials" || r.field == "worktree_parent" {
				v = tilde(v)
			}
			val := v
			switch {
			case isDefault && r.field == "api_key":
				val = sWarn.Render(v)
			case isDefault:
				val = sDim.Render(v + i18n.T("  (défaut)", "  (default)"))
			}
			if v == "" {
				val = sDim.Render("—")
			}
			if s.editing && i == s.cursor {
				val = s.input + "▏"
				if r.field == "api_key" {
					val = strings.Repeat("•", min(len(s.input), 40)) + "▏"
				}
			}
			line = "  " + sMuted.Render(padRight(fieldLabel(r.field), labelW)) + val
		}
		switch {
		case i == s.cursor:
			line = sSel.Width(m.width).Render(xansi.Strip(line))
			if s.editing {
				line = sSelDim.Width(m.width).Render(xansi.Strip(line))
			}
		case i == s.hover:
			line = sHover.Width(m.width).Render(xansi.Strip(line))
		}
		lines, owners = append(lines, line), append(owners, i)
	}
	return lines, owners
}

func padRight(s string, w int) string {
	if n := lipgloss.Width(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

// settingsTop keeps the cursor on screen.
func (m Model) settingsTop(owners []int, h int) int {
	s := m.settings
	cur := 0
	for i, o := range owners {
		if o == s.cursor {
			cur = i
		}
	}
	top := s.top
	if cur < top+1 {
		top = max(0, cur-2)
	}
	if cur >= top+h-1 {
		top = cur - h + 2
	}
	s.top = max(0, min(top, max(0, len(owners)-h)))
	return s.top
}

func (m Model) settingsView() string {
	s := m.settings
	head := spread(sKey.Render(backLabel())+"   "+sTitle.Render(i18n.T("Réglages", "Settings")), sDim.Render(tilde(config.Path())), m.width)
	bodyH := m.bodyHeight()
	lines, owners := m.settingsLines()
	r := s.rows()[s.cursor]
	tracker := ""
	if r.proj >= 0 {
		tracker = s.filled[r.proj].Tracker
	}
	hint := sMuted.Render("  " + fieldHint(r, tracker))
	listH := bodyH - 2
	top := m.settingsTop(owners, listH)
	shown := lines[top:min(len(lines), top+listH)]
	for i, l := range shown {
		shown[i] = xansi.Truncate(l, m.width, "…")
	}
	body := lipgloss.NewStyle().Width(m.width).Height(listH).MaxHeight(listH).Render(strings.Join(shown, "\n"))
	body += "\n" + sLine.Render(strings.Repeat("─", m.width)) + "\n" + xansi.Truncate(hint, m.width, "…")
	return head + "\n" + m.viewFlash() + "\n" + body + "\n\n" + m.viewFooter()
}

func (m Model) settingsButtons() []button {
	s := m.settings
	if s.editing {
		return []button{
			{"esc", i18n.T("Annuler", "Cancel"), true},
			{"enter", i18n.T("Enregistrer", "Save"), true},
		}
	}
	act := i18n.T("Modifier", "Edit")
	switch r := s.rows()[s.cursor]; r.field {
	case "tracker", "lang":
		act = i18n.T("Changer", "Change")
	case "delete":
		act = i18n.T("Retirer", "Remove")
	case "add":
		act = i18n.T("Ajouter", "Add")
	}
	return []button{
		{"esc", i18n.T("Retour", "Back"), true},
		{"enter", act, true},
	}
}

func (m Model) settingsMouse(ev tea.MouseMsg) (tea.Model, tea.Cmd) {
	s := m.settings
	_, owners := m.settingsLines()
	row := -1
	if y := ev.Y - 2; y >= 0 && y < m.bodyHeight()-2 {
		if i := s.top + y; i < len(owners) {
			row = owners[i]
		}
	}
	if ev.Action == tea.MouseActionMotion {
		_, m.hoverBtn = m.hitTest(ev.X, ev.Y)
		s.hover = row
		return m, nil
	}
	switch ev.Button {
	case tea.MouseButtonWheelUp:
		s.cursor = s.next(s.cursor-1, -1)
		return m, nil
	case tea.MouseButtonWheelDown:
		s.cursor = s.next(s.cursor+1, 1)
		return m, nil
	}
	if ev.Button != tea.MouseButtonLeft || ev.Action != tea.MouseActionPress {
		return m, nil
	}
	switch {
	case ev.Y == 0 && ev.X < lipgloss.Width(backLabel())+1:
		m.view = ""
	case ev.Y == m.footerRow():
		return m.footerClick(ev.X)
	case row >= 0 && !s.editing:
		// A first click selects, a second one acts.
		if row != s.cursor {
			s.cursor, s.confirm = row, -1
			return m, nil
		}
		return m.settingsActivate(row)
	}
	return m, nil
}

// StartInSettings opens cairn on its settings, with why.
func (m Model) StartInSettings(why error) Model {
	next, _ := m.openSettings()
	nm := next.(Model)
	nm.say("", why)
	return nm
}
