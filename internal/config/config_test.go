package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitRepo(t *testing.T, dir string) {
	t.Helper()
	if err := exec.Command("git", "init", "-q", dir).Run(); err != nil {
		t.Skip("git unavailable")
	}
}

// A config of the single-project era keeps working, as a Sprintosaur project.
func TestLegacyConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAIRN_HOME", home)
	repo := filepath.Join(home, "Yak-App")
	_ = os.WriteFile(filepath.Join(home, "config.json"), []byte(`{"repo": "`+repo+`", "assignee": "adrien", "lang": "en"}`), 0o644)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Projects) != 1 {
		t.Fatalf("projects: %+v", cfg.Projects)
	}
	p := cfg.Projects[0]
	if p.Key != "YAK" || p.Tracker != "sprintosaur" || p.Repo != repo || p.BranchPrefix != "adrien/" || p.Base != "origin/main" || cfg.Lang != "en" {
		t.Fatalf("project: %+v", p)
	}
	if cfg.ProjectFor("yak-12") == nil || cfg.ProjectFor("EIDO-1") != nil {
		t.Fatal("ProjectFor")
	}
}

func TestAddProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAIRN_HOME", home)
	eido := filepath.Join(home, "eido")
	gitRepo(t, eido)
	_ = os.WriteFile(filepath.Join(home, "config.json"), []byte(`{"repo": "/r/Yak-App", "lang": "fr"}`), 0o644)

	p, err := AddProject(Project{Repo: eido, Tracker: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Key != "EIDO" || p.Name != "eido" || p.Tracker != "local" {
		t.Fatalf("added: %+v", p)
	}
	if _, err := AddProject(Project{Repo: eido}); err == nil {
		t.Fatal("same repository twice")
	}
	data, _ := os.ReadFile(filepath.Join(home, "config.json"))
	// The old project moved into "projects", next to the new one.
	if s := string(data); !strings.Contains(s, `"projects"`) || strings.Contains(s, `"repo": "/r/Yak-App",
  "lang"`) || !strings.Contains(s, `"tracker": "sprintosaur"`) {
		t.Fatalf("file:\n%s", s)
	}
	cfg, _ := Load()
	if len(cfg.Projects) != 2 || cfg.Projects[1].Key != "EIDO" {
		t.Fatalf("reloaded: %+v", cfg.Projects)
	}
}

func TestKeyFor(t *testing.T) {
	for in, want := range map[string]string{"eido": "EIDO", "Yak-App": "YAKAPP", "2048": "T2048", "very-long-name": "VERYLONG"} {
		if got := KeyFor(in); got != want {
			t.Errorf("%s → %s, want %s", in, got, want)
		}
	}
}
