package journal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJournal(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "chantiers")
	_ = os.MkdirAll(dir, 0o755)
	if p, _ := LastPhase(Read(dir, "YAK-1")); p != "" {
		t.Fatal("empty journal has a phase")
	}
	_ = Append(dir, "yak-1", "phase:Plan")
	_ = Append(dir, "YAK-1", "plan validé")
	_ = Append(dir, "YAK-1", "phase:Build")
	_ = Append(dir, "YAK-1", "push autorisé")
	ev := Read(dir, "YAK-1")
	if len(ev) != 4 {
		t.Fatalf("events: %+v", ev)
	}
	if p, at := LastPhase(ev); p != "Build" || at.IsZero() {
		t.Fatalf("last phase %q %v", p, at)
	}
	_ = os.WriteFile(filepath.Join(dir, "YAK-1.md"), []byte("x"), 0o644)
	if err := Archive(dir, "YAK-1"); err != nil {
		t.Fatal(err)
	}
	if Read(dir, "YAK-1") != nil {
		t.Fatal("journal still there after archive")
	}
	got, _ := filepath.Glob(filepath.Join(filepath.Dir(dir), "archive", "YAK-1-*"))
	if len(got) != 2 {
		t.Fatalf("archive: %v", got)
	}
}
