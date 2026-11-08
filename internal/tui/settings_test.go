package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"cairn/internal/config"
	"cairn/internal/tracker"
)

func TestSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAIRN_HOME", home)
	t.Setenv("LINEAR_API_KEY", "")
	repo := filepath.Join(home, "eido")
	if exec.Command("git", "init", "-q", repo).Run() != nil {
		t.Skip("git unavailable")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m.width, m.height = 120, 40
	m = m.StartInSettings(cfg.Check())
	if m.view != "settings" || !strings.Contains(m.flash, "cairn add") {
		t.Fatalf("opens on settings with why: %q", m.flash)
	}
	keys := func(ks ...tea.KeyMsg) {
		t.Helper()
		for _, k := range ks {
			next, _ := m.settingsKey(k)
			m = next.(Model)
		}
	}
	enter, down := tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyDown}
	field := func(proj int, f string) {
		t.Helper()
		for i, r := range m.settings.rows() {
			if r.proj == proj && r.field == f {
				m.settings.cursor = i
				return
			}
		}
		t.Fatalf("no row %d %s", proj, f)
	}

	// Add the repository: it lands on its tracker, local by default.
	field(-2, "add")
	keys(enter, keyForRunes(repo), enter)
	if len(m.projects) != 1 || m.projects[0].cfg.Key != "EIDO" || m.projects[0].cfg.Tracker != "local" {
		t.Fatalf("added: %+v (%s)", m.projects, m.flash)
	}
	if r := m.settings.rows()[m.settings.cursor]; r.field != "tracker" {
		t.Fatalf("cursor on %+v", r)
	}
	// local → linear: the credentials row appears.
	keys(enter)
	if m.projects[0].cfg.Tracker != "linear" {
		t.Fatalf("tracker: %s", m.projects[0].cfg.Tracker)
	}
	keys(down)
	if r := m.settings.rows()[m.settings.cursor]; r.field != "credentials" {
		t.Fatalf("after tracker: %+v", r)
	}
	// A bad key is refused and nothing is written.
	field(0, "key")
	keys(enter, tea.KeyMsg{Type: tea.KeyCtrlU}, keyForRunes("e-1"), enter)
	if m.projects[0].cfg.Key != "EIDO" || !m.flashErr {
		t.Fatalf("bad key kept: %s / %q", m.projects[0].cfg.Key, m.flash)
	}
	keys(enter, tea.KeyMsg{Type: tea.KeyCtrlU}, keyForRunes("eng"), enter)
	if m.projects[0].cfg.Key != "ENG" {
		t.Fatalf("key: %s (%s)", m.projects[0].cfg.Key, m.flash)
	}
	if v := m.settingsView(); !strings.Contains(v, "Projet eido") || !strings.Contains(v, "ENG-… · linear") {
		t.Fatalf("view:\n%s", v)
	}
	data, _ := os.ReadFile(config.Path())
	if s := string(data); !strings.Contains(s, `"key": "ENG"`) || !strings.Contains(s, `"tracker": "linear"`) || strings.Contains(s, "origin/main") {
		t.Fatalf("file:\n%s", s)
	}
	// Without a key, the project says how to connect, step by step; a pasted
	// key goes to a private file the project points to.
	if v := m.settingsView(); !strings.Contains(v, "Personal API keys") || !strings.Contains(v, "colle la clé") {
		t.Fatalf("no guide:\n%s", v)
	}
	field(0, "api_key")
	keys(enter, keyForRunes("lin_api_secret1234"))
	if v := m.settingsView(); strings.Contains(v, "secret") {
		t.Fatal("the key shows while typed")
	}
	keys(enter)
	secret := filepath.Join(home, "credentials", "eido.env")
	if fi, err := os.Stat(secret); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("secret file: %v %v", fi, err)
	}
	if m.projects[0].cfg.Credentials != secret {
		t.Fatalf("credentials: %q", m.projects[0].cfg.Credentials)
	}
	if v := m.settingsView(); !strings.Contains(v, "…1234") || !strings.Contains(v, "vérification de la connexion") {
		t.Fatalf("after the key:\n%s", v)
	}
	m.settings.checks["eido"] = &connCheck{conn: tracker.Connection{User: "Adrien", Teams: []tracker.Team{{Key: "OPS", Name: "Ops"}}}}
	if v := m.settingsView(); !strings.Contains(v, "pas d'équipe ENG") || !strings.Contains(v, "OPS (Ops)") {
		t.Fatalf("team guide:\n%s", v)
	}
	// Removing asks twice.
	field(0, "delete")
	keys(enter)
	if len(m.projects) != 1 {
		t.Fatal("removed on the first enter")
	}
	keys(enter)
	if len(m.projects) != 0 {
		t.Fatal("not removed")
	}
	keys(tea.KeyMsg{Type: tea.KeyEsc})
	if m.view != "" {
		t.Fatal("esc goes back")
	}
}
