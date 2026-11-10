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

func TestHowItWorks(t *testing.T) {
	s := Parse("/x", `---
ticket: YAK-854
phase: review
---
# YAK-854 · Colonnes

Objectif.

- [x] hook

## Comment ça marche

### Largeur d'une colonne
- Quoi : la largeur vit dans `+"`useColumnWidths`"+` (état React), écrite au relâchement.
- [x] pas une tâche

## Preuves
- go test ✓
`)
	if !strings.HasPrefix(s.HowItWorks, "### Largeur d'une colonne") || !strings.Contains(s.HowItWorks, "pas une tâche") {
		t.Fatalf("how: %q", s.HowItWorks)
	}
	if len(s.Tasks) != 1 || len(s.Proofs) != 1 || len(s.Summary) != 1 {
		t.Fatalf("tasks %d proofs %d summary %q", len(s.Tasks), len(s.Proofs), s.Summary)
	}
	if Parse("/x", "## How it works\nIt syncs on save.\n").HowItWorks != "It syncs on save." {
		t.Fatal("english heading")
	}
}

func TestManualTest(t *testing.T) {
	text := `---
ticket: YAK-854
phase: build
---
# YAK-854 · Colonnes

Objectif.

- [x] hook

## Tester à la main
Lance ` + "`pnpm dev`" + `, ouvre /freelances.
- connecté en admin

- [ ] Glisse le bord de « Nom » → la colonne s'élargit
- [x] Double-clique sur le bord → elle revient à sa largeur

## Preuves
- go test ✓
`
	s := Parse("/x", text)
	if len(s.ManualSetup) != 2 || s.ManualSetup[1] != "connecté en admin" {
		t.Fatalf("setup %q", s.ManualSetup)
	}
	if len(s.ManualSteps) != 2 || s.ManualSteps[0].Done || !s.ManualSteps[1].Done {
		t.Fatalf("steps %+v", s.ManualSteps)
	}
	if len(s.Tasks) != 1 || len(s.Proofs) != 1 {
		t.Fatalf("tasks %d proofs %d", len(s.Tasks), len(s.Proofs))
	}
	if !s.TickManual(0, true) || !s.TickManual(1, false) || s.TickManual(2, true) {
		t.Fatal("tick")
	}
	again := Parse("/x", s.String())
	if !again.ManualSteps[0].Done || again.ManualSteps[1].Done {
		t.Fatalf("round trip:\n%s", s.String())
	}
	if !strings.Contains(s.String(), "- [x] Glisse le bord") || !strings.Contains(s.String(), "- [ ] Double-clique") {
		t.Fatalf("file:\n%s", s.String())
	}
	if len(Parse("/x", "## Manual test\n- [ ] Click → it opens\n").ManualSteps) != 1 {
		t.Fatal("english heading")
	}
}
