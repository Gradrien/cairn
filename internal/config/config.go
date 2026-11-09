// Package config loads ~/.cairn/config.json: the projects cairn follows and
// the interface settings.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"cairn/internal/i18n"
)

// Project is one repository and where its tickets come from.
type Project struct {
	// Name is the project's short name, shown on screen.
	Name string `json:"name"`
	// Repo is the main checkout whose worktrees are chantiers.
	Repo string `json:"repo"`
	// Key is the ticket prefix (YAK, EIDO…): YAK-123. With Linear or
	// Sprintosaur, it is the team key.
	Key string `json:"key"`
	// Tracker is where tickets live: "local" (none: the chantier file is the
	// ticket), "linear" or "sprintosaur".
	Tracker string `json:"tracker"`
	// Assignee is your handle in the tracker (Sprintosaur); empty means the
	// local part of git's user.email.
	Assignee string `json:"assignee,omitempty"`
	// Credentials is a KEY=VALUE file with the tracker's token
	// (LINEAR_API_KEY, SPRINTOSAUR_API/_TOKEN…); environment variables win.
	Credentials string `json:"credentials,omitempty"`
	// Web is the base URL of issue pages, when the tracker does not give one.
	Web string `json:"web,omitempty"`
	// BranchPrefix is put before "yak-123-slug" for new branches.
	BranchPrefix string `json:"branch_prefix,omitempty"`
	// Base is the ref new branches start from and diffs compare against.
	Base string `json:"base,omitempty"`
	// WorktreeParent is where new worktrees go (default: next to Repo).
	WorktreeParent string `json:"worktree_parent,omitempty"`
	// AgentKind is the herdr agent kind started for a new chantier.
	AgentKind string `json:"agent_kind,omitempty"`
	// Diff is the command the Diff button runs in the worktree; {base} is
	// replaced by Base. Default: browsediff when installed, else git diff.
	Diff string `json:"diff,omitempty"`
}

type Config struct {
	Projects []Project `json:"projects"`
	// Lang is the interface language: "fr" (default) or "en".
	Lang string `json:"lang"`
	// AgentKind and Diff are the defaults of every project.
	AgentKind string `json:"agent_kind,omitempty"`
	Diff      string `json:"diff,omitempty"`

	// Single-project settings of older versions, read once into Projects.
	LegacyRepo           string `json:"repo,omitempty"`
	LegacyTeam           string `json:"team,omitempty"`
	LegacyAssignee       string `json:"assignee,omitempty"`
	LegacyBranchPrefix   string `json:"branch_prefix,omitempty"`
	LegacyBase           string `json:"base,omitempty"`
	LegacyWorktreeParent string `json:"worktree_parent,omitempty"`
	LegacyCredentials    string `json:"sprintosaur_credentials,omitempty"`
	LegacyWeb            string `json:"sprintosaur_web,omitempty"`
}

// Dir is cairn's home: config and chantier files.
func Dir() string {
	if d := os.Getenv("CAIRN_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cairn")
}

// ChantiersDir holds one <KEY>.md per chantier.
func ChantiersDir() string { return filepath.Join(Dir(), "chantiers") }

// ArchiveDir holds the plans of closed chantiers, "<KEY>-<date>-<time>.md".
func ArchiveDir() string { return filepath.Join(Dir(), "archive") }

// Path is the config file.
func Path() string { return filepath.Join(Dir(), "config.json") }

// Read returns the file as written, without defaults; older single-project
// files come with their project moved into Projects.
func Read() (Config, error) { return read() }

func read() (Config, error) {
	cfg := Config{Lang: "fr"}
	data, err := os.ReadFile(Path())
	switch {
	case errors.Is(err, os.ErrNotExist):
		return cfg, nil
	case err != nil:
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("%s: %w", Path(), err)
	}
	cfg.migrate()
	return cfg, nil
}

// migrate turns the single Sprintosaur project of older versions into a project.
func (c *Config) migrate() {
	if c.LegacyRepo == "" && c.LegacyTeam == "" {
		return
	}
	if len(c.Projects) == 0 {
		team := c.LegacyTeam
		if team == "" {
			team = "YAK"
		}
		c.Projects = append(c.Projects, Project{
			Name: strings.ToLower(team), Repo: c.LegacyRepo, Key: team, Tracker: "sprintosaur",
			Assignee: c.LegacyAssignee, BranchPrefix: c.LegacyBranchPrefix, Base: c.LegacyBase,
			WorktreeParent: c.LegacyWorktreeParent, Credentials: c.LegacyCredentials, Web: c.LegacyWeb,
		})
	}
	c.LegacyRepo, c.LegacyTeam, c.LegacyAssignee, c.LegacyBranchPrefix = "", "", "", ""
	c.LegacyBase, c.LegacyWorktreeParent, c.LegacyCredentials, c.LegacyWeb = "", "", "", ""
}

// Load reads the config, creating it on first run, and fills every default.
func Load() (Config, error) {
	cfg, err := read()
	if err != nil {
		return cfg, err
	}
	if _, err := os.Stat(Path()); errors.Is(err, os.ErrNotExist) {
		if err := save(cfg); err != nil {
			return cfg, err
		}
	}
	if cfg.Lang == "" {
		cfg.Lang = "fr"
	}
	for i := range cfg.Projects {
		cfg.Projects[i] = cfg.fill(cfg.Projects[i])
	}
	if err := os.MkdirAll(ChantiersDir(), 0o755); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Fill gives a project its defaults.
func (c Config) Fill(p Project) Project { return c.fill(p) }

func (c Config) fill(p Project) Project {
	p.Repo = expand(p.Repo)
	if p.Key == "" {
		p.Key = KeyFor(filepath.Base(p.Repo))
	}
	p.Key = strings.ToUpper(p.Key)
	if p.Name == "" {
		p.Name = strings.ToLower(p.Key)
	}
	if p.Tracker == "" {
		p.Tracker = "local"
	}
	if p.Assignee == "" {
		p.Assignee = gitHandle(p.Repo)
	}
	if p.BranchPrefix == "" && p.Assignee != "" {
		p.BranchPrefix = p.Assignee + "/"
	}
	if p.Base == "" {
		p.Base = "origin/main"
	}
	if p.WorktreeParent == "" {
		p.WorktreeParent = filepath.Dir(p.Repo)
	}
	p.WorktreeParent = expand(p.WorktreeParent)
	if p.AgentKind == "" {
		p.AgentKind = c.AgentKind
	}
	if p.AgentKind == "" {
		p.AgentKind = "claude"
	}
	if p.Diff == "" {
		p.Diff = c.Diff
	}
	p.Credentials = expand(p.Credentials)
	return p
}

func expand(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// KeyFor turns a folder name into a ticket prefix: "eido" → "EIDO", "Yak-App" → "YAKAPP".
func KeyFor(name string) string {
	k := strings.ToUpper(regexp.MustCompile(`[^A-Za-z0-9]`).ReplaceAllString(name, ""))
	if k == "" || (k[0] >= '0' && k[0] <= '9') {
		k = "T" + k
	}
	if len(k) > 8 {
		k = k[:8]
	}
	return k
}

func gitHandle(repo string) string {
	out, err := exec.Command("git", "-C", repo, "config", "user.email").Output()
	if err != nil {
		return ""
	}
	// prenom.nom@domaine → prenom.
	local, _, _ := strings.Cut(strings.TrimSpace(string(out)), "@")
	first := strings.FieldsFunc(local, func(r rune) bool { return r == '.' || r == '_' || r == '+' || r == '-' })
	if len(first) == 0 {
		return ""
	}
	return strings.ToLower(first[0])
}

func isRepo(dir string) bool {
	return exec.Command("git", "-C", dir, "rev-parse", "--git-dir").Run() == nil
}

// Check reports a config that cannot work, with what to change.
func (c Config) Check() error {
	if len(c.Projects) == 0 {
		return fmt.Errorf("%s", i18n.T("aucun projet : lance « cairn add » dans un dépôt git, ou ajoute-le dans les réglages (s)",
			"no project: run \"cairn add\" in a git repository, or add one in the settings (s)"))
	}
	return c.validate()
}

// validate checks every project, defaults applied: a git checkout, a known
// tracker, a key and a repository nobody else has.
func (c Config) validate() error {
	keys, names, repos := map[string]string{}, map[string]string{}, map[string]string{}
	for _, p := range c.Projects {
		p = c.fill(p)
		if !isRepo(p.Repo) {
			return fmt.Errorf(i18n.T("projet %s : %q n'est pas un dépôt git", "project %s: %q is not a git repository"), p.Name, p.Repo)
		}
		switch p.Tracker {
		case "local", "linear", "sprintosaur":
		default:
			return fmt.Errorf(i18n.T("projet %s : tracker inconnu %q (local, linear ou sprintosaur)", "project %s: unknown tracker %q (local, linear or sprintosaur)"), p.Name, p.Tracker)
		}
		if !regexp.MustCompile(`^[A-Z][A-Z0-9]*$`).MatchString(p.Key) {
			return fmt.Errorf(i18n.T("projet %s : la clé %q doit être en lettres et chiffres (ex. EIDO)", "project %s: key %q must be letters and digits (e.g. EIDO)"), p.Name, p.Key)
		}
		if other, ok := keys[p.Key]; ok {
			return fmt.Errorf(i18n.T("les projets %s et %s ont la même clé %s", "projects %s and %s share the key %s"), other, p.Name, p.Key)
		}
		if other, ok := names[p.Name]; ok {
			return fmt.Errorf(i18n.T("deux projets s'appellent %s (%s)", "two projects are called %s (%s)"), p.Name, other)
		}
		if other, ok := repos[filepath.Clean(p.Repo)]; ok {
			return fmt.Errorf(i18n.T("%s est déjà le dépôt du projet %s", "%s is already project %s's repository"), p.Repo, other)
		}
		keys[p.Key], names[p.Name], repos[filepath.Clean(p.Repo)] = p.Name, p.Name, p.Name
	}
	return nil
}

// Save writes the config as given (defaults are not written) once every
// project checks out; nothing is written otherwise.
func Save(cfg Config) error {
	if err := cfg.validate(); err != nil {
		return err
	}
	return save(cfg)
}

// Keys lists every project's ticket prefix.
func (c Config) Keys() []string {
	var out []string
	for _, p := range c.Projects {
		out = append(out, p.Key)
	}
	return out
}

// ProjectFor returns the project a ticket key belongs to, by its prefix.
func (c Config) ProjectFor(key string) *Project {
	prefix, _, _ := strings.Cut(strings.ToUpper(key), "-")
	for i := range c.Projects {
		if c.Projects[i].Key == prefix {
			return &c.Projects[i]
		}
	}
	return nil
}

// AddProject saves a new project (older single-project files are rewritten
// in the new format on the way) and returns it with its defaults.
func AddProject(p Project) (Project, error) {
	cfg, err := read()
	if err != nil {
		return p, err
	}
	p.Repo = expand(p.Repo)
	if abs, err := filepath.Abs(p.Repo); err == nil {
		p.Repo = abs
	}
	if !isRepo(p.Repo) {
		return p, fmt.Errorf(i18n.T("%q n'est pas un dépôt git", "%q is not a git repository"), p.Repo)
	}
	if top, err := exec.Command("git", "-C", p.Repo, "rev-parse", "--show-toplevel").Output(); err == nil {
		p.Repo = strings.TrimSpace(string(top))
	}
	if p.Base == "" {
		p.Base = defaultBase(p.Repo)
	}
	full := cfg.fill(p)
	for _, q := range cfg.Projects {
		q = cfg.fill(q)
		if filepath.Clean(q.Repo) == filepath.Clean(full.Repo) {
			return q, fmt.Errorf(i18n.T("%s est déjà le projet %s", "%s is already project %s"), full.Repo, q.Name)
		}
		if q.Key == full.Key {
			return q, fmt.Errorf(i18n.T("la clé %s est déjà celle du projet %s : choisis-en une autre avec --key", "key %s is already project %s's: pick another with --key"), full.Key, q.Name)
		}
	}
	// Keep the file short: only what differs from the defaults.
	p.Key, p.Name, p.Tracker = full.Key, full.Name, full.Tracker
	if p.Base == "origin/main" {
		p.Base = ""
	}
	cfg.Projects = append(cfg.Projects, p)
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return full, err
	}
	return full, save(cfg)
}

// defaultBase is the remote's default branch, origin/main when unknown.
func defaultBase(repo string) string {
	out, err := exec.Command("git", "-C", repo, "symbolic-ref", "--short", "refs/remotes/origin/HEAD").Output()
	if b := strings.TrimSpace(string(out)); err == nil && b != "" {
		return b
	}
	return "origin/main"
}

func save(cfg Config) error {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	if cfg.Projects == nil {
		cfg.Projects = []Project{}
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), append(out, '\n'), 0o644)
}

// SetLang saves the interface language, keeping the rest of the file as is.
func SetLang(lang string) error {
	raw := map[string]any{}
	if data, err := os.ReadFile(Path()); err == nil {
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
	}
	raw["lang"] = lang
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), append(out, '\n'), 0o644)
}

// SaveSecret writes NAME=value into ~/.cairn/credentials/<project>.env,
// readable by you only, keeping the file's other lines, and returns its path.
func SaveSecret(project, name, value string) (string, error) {
	dir := filepath.Join(Dir(), "credentials")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, regexp.MustCompile(`[^A-Za-z0-9_-]`).ReplaceAllString(project, "_")+".env")
	var lines []string
	if data, err := os.ReadFile(path); err == nil {
		for _, l := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
			k, _, _ := strings.Cut(strings.TrimPrefix(strings.TrimSpace(l), "export "), "=")
			if l != "" && strings.TrimSpace(k) != name {
				lines = append(lines, l)
			}
		}
	}
	lines = append(lines, name+"="+value)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return "", err
	}
	return path, os.Chmod(path, 0o600)
}
