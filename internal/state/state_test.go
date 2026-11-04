package state

import (
	"strings"
	"testing"
)

const sample = `---
ticket: YAK-788
phase: build
attente: Valider le plan
---
# YAK-788

- [x] retirer les actions
- [ ] trigger SQL
  - [X] sous-tâche faite

## Preuves
- go test ./... ✓
- [x] capture S1

## Notes
- [ ] relire avec Océane
`

func TestParse(t *testing.T) {
	s := Parse("/x/YAK-788.md", sample)
	if s.Ticket != "YAK-788" || s.Phase != "build" || s.Waiting != "Valider le plan" {
		t.Fatalf("frontmatter: %+v", s)
	}
	if len(s.Tasks) != 4 || s.DoneCount() != 2 {
		t.Fatalf("tasks: %d done %d: %+v", len(s.Tasks), s.DoneCount(), s.Tasks)
	}
	if len(s.Proofs) != 2 || s.Proofs[0] != "go test ./... ✓" {
		t.Fatalf("proofs: %q", s.Proofs)
	}
}

func TestSetRoundTrip(t *testing.T) {
	s := Parse("/x", sample)
	s.Set("attente", "")
	s.Set("phase", "review")
	out := s.String()
	if strings.Contains(out, "attente") || !strings.Contains(out, "phase: review") {
		t.Fatalf("unexpected:\n%s", out)
	}
	if !strings.HasSuffix(out, "- [ ] relire avec Océane\n") {
		t.Fatalf("body lost:\n%s", out)
	}
	again := Parse("/x", out)
	if again.Phase != "review" || again.Waiting != "" || len(again.Tasks) != 4 {
		t.Fatalf("reparse: %+v", again)
	}
}

func TestNoFrontmatter(t *testing.T) {
	s := Parse("/x", "- [ ] a\n")
	if len(s.Tasks) != 1 || s.Phase != "" {
		t.Fatalf("%+v", s)
	}
	s.Set("phase", "plan")
	if !strings.HasPrefix(s.String(), "---\nphase: plan\n---\n") {
		t.Fatalf("%q", s.String())
	}
}

func TestSections(t *testing.T) {
	s := Parse("/x", `---
phase: build
---
# YAK-1 · titre

Objectif : afficher la synthèse.
Décision : on garde le titre.

- [x] a
- [~] b
- [ ] c

## Questions
- Garder le titre ?
- [x] Migration ou script ? → script

## Effets de bord
- [ ] export CSV
- [x] recherche sémantique

## Preuves
- go test ✓
`)
	if len(s.Summary) != 2 || s.Summary[0] != "Objectif : afficher la synthèse." {
		t.Fatalf("summary %q", s.Summary)
	}
	if len(s.Tasks) != 3 || Count(s.Tasks, true) != 1 || Count(s.Tasks, false) != 1 {
		t.Fatalf("tasks %+v", s.Tasks)
	}
	if q := s.OpenQuestions(); len(q) != 1 || q[0] != "Garder le titre ?" {
		t.Fatalf("questions %q", q)
	}
	if len(s.SideEffects) != 2 || Count(s.SideEffects, true) != 1 {
		t.Fatalf("side %+v", s.SideEffects)
	}
	if len(s.Proofs) != 1 {
		t.Fatalf("proofs %q", s.Proofs)
	}
}

func TestEnglishAliases(t *testing.T) {
	s := Parse("/x", "---\nwaiting: Ready to push\npush: allowed\n---\nGoal\n\n- [x] a\n\n## Side effects\n- [ ] b\n\n## Proofs\n- tests pass\n")
	if s.Waiting != "Ready to push" || !s.PushAllowed() || len(s.SideEffects) != 1 || len(s.Proofs) != 1 {
		t.Fatalf("%+v", s)
	}
	s.Set("attente", "")
	if s.Waiting != "" || strings.Contains(s.String(), "waiting") {
		t.Fatalf("waiting not cleared:\n%s", s.String())
	}
}
