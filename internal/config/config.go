// Package config loads ~/.cairn/config.json, writing sensible defaults on first run.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Config struct {
	// Repo is the main checkout whose worktrees are chantiers.
	Repo string `json:"repo"`
	// Team is the Sprintosaur team key (YAK, SPI, …).
	Team string `json:"team"`
	// Assignee is the Sprintosaur handle; empty means the local part of git's user.email.
	Assignee string `json:"assignee"`
	// BranchPrefix is put before "yak-123-slug" for new branches.
	BranchPrefix string `json:"branch_prefix"`
	// Base is the ref new branches start from and browsediff compares against.
	Base string `json:"base"`
	// WorktreeParent is where new worktrees go (default: next to Repo).
	WorktreeParent string `json:"worktree_parent"`
	// AgentKind is the herdr agent kind started for a new chantier.
	AgentKind string `json:"agent_kind"`
	// SprintosaurCredentials is a KEY=VALUE file with SPRINTOSAUR_API, _TOKEN, _SHARE_TOKEN.
	SprintosaurCredentials string `json:"sprintosaur_credentials"`
	// SprintosaurWeb is the base URL of issue pages.
	SprintosaurWeb string `json:"sprintosaur_web"`
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

func defaults() Config {
	home, _ := os.UserHomeDir()
	return Config{
		Repo:                   guessRepo(home),
		Team:                   "YAK",
		BranchPrefix:           "",
		Base:                   "origin/main",
		AgentKind:              "claude",
		SprintosaurCredentials: filepath.Join(home, ".claude", "skills", "sprintosaur", "credentials.env"),
		SprintosaurWeb:         "https://sprintosaur.preview.spinup.yalink.fr/issue/",
	}
}

func Load() (Config, error) {
	cfg := defaults()
	path := Path()
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(Dir(), 0o755); err != nil {
			return cfg, err
		}
		out, _ := json.MarshalIndent(cfg, "", "  ")
		_ = os.WriteFile(path, append(out, '\n'), 0o644)
	case err != nil:
		return cfg, err
	default:
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, err
		}
	}
	if cfg.Assignee == "" {
		cfg.Assignee = gitHandle(cfg.Repo)
	}
	if cfg.BranchPrefix == "" && cfg.Assignee != "" {
		cfg.BranchPrefix = cfg.Assignee + "/"
	}
	if cfg.WorktreeParent == "" {
		cfg.WorktreeParent = filepath.Dir(cfg.Repo)
	}
	if err := os.MkdirAll(ChantiersDir(), 0o755); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func gitHandle(repo string) string {
	out, err := exec.Command("git", "-C", repo, "config", "user.email").Output()
	if err != nil {
		return ""
	}
	// prenom.nom@domaine → prenom: Sprintosaur handles are first names.
	local, _, _ := strings.Cut(strings.TrimSpace(string(out)), "@")
	first := strings.FieldsFunc(local, func(r rune) bool { return r == '.' || r == '_' || r == '+' || r == '-' })
	if len(first) == 0 {
		return ""
	}
	return strings.ToLower(first[0])
}

// guessRepo picks the Yak-App checkout on first run: the usual places, else
// ~/Applications/Yak-App (Check then says where to fix it).
func guessRepo(home string) string {
	for _, p := range []string{
		filepath.Join(home, "Applications", "Yak-App"),
		filepath.Join(home, "Yak-App"),
		filepath.Join(home, "code", "Yak-App"),
		filepath.Join(home, "dev", "Yak-App"),
		filepath.Join(home, "Projects", "Yak-App"),
		filepath.Join(home, "Documents", "Yak-App"),
	} {
		if isRepo(p) {
			return p
		}
	}
	return filepath.Join(home, "Applications", "Yak-App")
}

func isRepo(dir string) bool {
	return exec.Command("git", "-C", dir, "rev-parse", "--git-dir").Run() == nil
}

// Path is the config file.
func Path() string { return filepath.Join(Dir(), "config.json") }

// Check reports a config that cannot work, with what to change.
func (c Config) Check() error {
	if !isRepo(c.Repo) {
		return fmt.Errorf("le dépôt %q n'est pas un checkout git : corrige \"repo\" dans %s", c.Repo, Path())
	}
	return nil
}
