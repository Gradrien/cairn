package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadArchive(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("YAK-854-20261105-100000.md", "---\nticket: YAK-854\n---\n# YAK-854 · Vieux\n")
	write("YAK-854-20261109-120000.md", "---\nticket: YAK-854\n---\n# YAK-854 · Colonnes\n\n## Comment ça marche\nUn hook.\n")
	write("YAK-854-20261109-120000.log", "x")
	write("EIDO-7-20261106-090000.md", "---\nticket: EIDO-7\n---\n# EIDO-7 · Autre\n")
	a := LoadArchive(dir)
	if len(a) != 2 || a[0].Key != "YAK-854" || a[1].Key != "EIDO-7" {
		t.Fatalf("%+v", a)
	}
	if a[0].Title != "Colonnes" || a[0].HowItWorks != "Un hook." || a[0].Body() == "" {
		t.Fatalf("latest wins: %+v", a[0].State)
	}
	if len(LoadArchive(filepath.Join(dir, "absent"))) != 0 {
		t.Fatal("missing dir")
	}
}
