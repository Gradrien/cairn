package state

import (
	"os"
	"path/filepath"
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
	if q := s.OpenQuestions(); len(q) != 1 || q[0].Text != "Garder le titre ?" {
		t.Fatalf("questions %+v", q)
	}
	if len(s.SideEffects) != 2 || Count(s.SideEffects, true) != 1 {
		t.Fatalf("side %+v", s.SideEffects)
	}
	if len(s.Proofs) != 1 {
		t.Fatalf("proofs %q", s.Proofs)
	}
}

func TestApproach(t *testing.T) {
	s := Parse("/x", `---
phase: plan
---
# YAK-1 · titre

Ajouter la colonne Modèle.

## Approche
- **Problème** : les admins recopient le modèle.
- **Choix** : une colonne dans le mapper,
  avec formatModel.
- **Écarté** : le SQL.
- **Fini quand** :
  1. une colonne Modèle
  2. vide sans modèle
    - même pour les anciens

- [ ] select
- [ ] mapper

## Questions
- Libellé ou code ?
`)
	if len(s.Summary) != 1 {
		t.Fatalf("summary %q", s.Summary)
	}
	want := []Item{{Text: "**Problème** : les admins recopient le modèle."}, {Text: "**Choix** : une colonne dans le mapper, avec formatModel."},
		{Text: "**Écarté** : le SQL."}, {Text: "**Fini quand** :"}, {Text: "1. une colonne Modèle", Depth: 1}, {Text: "2. vide sans modèle", Depth: 1}, {Text: "même pour les anciens", Depth: 2}}
	if len(s.Approach) != len(want) {
		t.Fatalf("approach %+v", s.Approach)
	}
	for i := range want {
		if s.Approach[i] != want[i] {
			t.Fatalf("approach[%d] = %+v, want %+v", i, s.Approach[i], want[i])
		}
	}
	if len(s.Tasks) != 2 {
		t.Fatalf("tasks under the approach must stay tasks: %+v", s.Tasks)
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

func TestTickSide(t *testing.T) {
	path := filepath.Join(t.TempDir(), "YAK-1.md")
	body := "---\nphase: build\n---\n# YAK-1 · T\n\n- [ ] tâche\n\n## Effets de bord\n- [ ] Autres appelants\n- [~] Quota Novu\n\n## Preuves\n- ok\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	s, _ := Load(path)
	if !s.TickSide(1, true) || !s.SideEffects[1].Done || s.SideEffects[1].Doing {
		t.Fatalf("ticked: %+v", s.SideEffects)
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "- [ ] Autres appelants\n- [x] Quota Novu\n") || !strings.Contains(string(data), "- [ ] tâche") {
		t.Errorf("only that box changes:\n%s", data)
	}
	s, _ = Load(path)
	if !s.TickSide(1, false) || s.SideEffects[1].Done || s.TickSide(5, true) {
		t.Error("untick, and nothing out of range")
	}
}

func TestAddAsked(t *testing.T) {
	front := "---\nphase: build\n---\n# YAK-1 · T\n\n- [ ] tâche\n"
	for name, tc := range map[string]struct{ body, want string }{
		"no section": {
			"\n## Preuves\n- ok\n",
			"- [ ] tâche\n\n## Comment ça marche\n\n### Questions posées\n- **Où ?**\n  Dans `a.go:3`.\n\n## Preuves\n- ok\n",
		},
		"section, no questions": {
			"\n## Comment ça marche\nLe flux.\n\n### Export\n- **Où** : `x.go`\n\n## Preuves\n- ok\n",
			"### Export\n- **Où** : `x.go`\n\n### Questions posées\n- **Où ?**\n  Dans `a.go:3`.\n\n## Preuves\n",
		},
		"questions then a mechanism": {
			"\n## Comment ça marche\nLe flux.\n\n### Questions posées\n- **Avant ?**\n  Oui.\n\n### Autre\nTexte.\n",
			"- **Avant ?**\n  Oui.\n- **Où ?**\n  Dans `a.go:3`.\n\n### Autre\nTexte.\n",
		},
		"section at the end": {
			"\n## Comment ça marche\nLe flux.\n",
			"## Comment ça marche\nLe flux.\n\n### Questions posées\n- **Où ?**\n  Dans `a.go:3`.\n",
		},
	} {
		s := Parse("/p/YAK-1.md", front+tc.body)
		s.AddAsked("Où ?", "Dans `a.go:3`.")
		if got := s.String(); !strings.Contains(got, tc.want) {
			t.Errorf("%s:\n%s\nwant it to contain:\n%s", name, got, tc.want)
		}
		if !strings.Contains(s.HowItWorks, "**Où ?**") {
			t.Errorf("%s: HowItWorks follows: %q", name, s.HowItWorks)
		}
	}
}

func TestReprise(t *testing.T) {
	s := Parse("/p/YAK-1.md", "---\nphase: build\n---\n# YAK-1\n\n- [x] a\n- [~] b\n\n## Reprise\nJ'en suis à b.\n\n### Pièges\n- le cache\n\n## Preuves\n- ok\n")
	if s.Reprise != "J'en suis à b.\n\n### Pièges\n- le cache" || len(s.Proofs) != 1 || len(s.Tasks) != 2 {
		t.Errorf("reprise %q proofs %v tasks %v", s.Reprise, s.Proofs, s.Tasks)
	}
}

// From YAK-838: the options of a question stay under it.
func TestQuestionOptions(t *testing.T) {
	s := Parse("/x", "---\nphase: plan\n---\n# YAK-838 · T\n\n## Questions\n"+
		"- **4. Spé orpheline** : aujourd'hui, la requête n'atteint une spé que si son métier est sur le deal (`freelance.ts:771-795`). Un deal sans métier n'a aucune cible.\n"+
		"  - (a) Garder le ciblage actuel : le point affiche 0 et le bouton est grisé.\n"+
		"  - (b) Élargir l'envoi aux expertises de la spé : cela change le premier envoi.\n"+
		"\n"+
		"  Je recommande (a) : il ne change pas qui reçoit l'email.\n"+
		"- [x] **5. Envoi à 0 destinataire** → oui : on enregistre une ligne.\n")
	qs := s.OpenQuestions()
	if len(s.Questions) != 2 || len(qs) != 1 {
		t.Fatalf("questions %+v", s.Questions)
	}
	q := qs[0]
	if !strings.HasSuffix(q.Text, "n'a aucune cible.") || len(q.Options) != 2 {
		t.Fatalf("question %+v", q)
	}
	if q.Options[0] != (Option{"a", "Garder le ciblage actuel : le point affiche 0 et le bouton est grisé."}) || q.Options[1].Key != "b" {
		t.Fatalf("options %+v", q.Options)
	}
	if q.Recommended != 0 || !strings.HasPrefix(q.Recommendation, "Je recommande (a)") || len(q.Body) != 0 {
		t.Fatalf("recommendation %d %q body %+v", q.Recommended, q.Recommendation, q.Body)
	}
	if p := s.Lint(); len(p) != 0 {
		t.Fatalf("lint %v", p)
	}
	if q.Answer(1) != "(b) Élargir l'envoi aux expertises de la spé : cela change le premier envoi." {
		t.Fatalf("answer %q", q.Answer(1))
	}
}

// Agents do not align sub-items on the parent's text, nor always indent them.
func TestQuestionShapes(t *testing.T) {
	for name, tc := range map[string]struct {
		text string
		keys string
		reco int
	}{
		"numbered question, 2-space options": {"1. Garder ?\n  - (a) oui\n  - (b) non\n  - Recommandation : b\n", "ab", 1},
		"options at the margin":              {"- Garder ?\n- (a) oui\n- (b) non\n\nJe recommande l'option a.\n", "ab", 0},
		"options without a dash":             {"- Garder ?\n  (a) oui\n  (b) non\n", "ab", -1},
		"lettered":                           {"- Garder ?\n  a) oui\n  b) non (recommandé)\n", "ab", 1},
		"numbered options":                   {"- Garder ?\n  1. oui\n  2. non\n  - **Recommandation** : 2\n", "12", 1},
		"option words":                       {"- Garder ?\n  - **Option A** : oui\n  - **Option B** : non\n  - Recommandé : option B\n", "ab", 1},
		"bold keys":                          {"- Garder ?\n  - **(a)** oui\n  - **(b)** non\n", "ab", -1},
		"under an Options item":              {"- Garder ?\n  - **Contexte** : rien n'oblige.\n    - un détail\n  - **Options** :\n    - (a) oui\n    - (b) non\n\n  Je recommande (b).\n", "ab", 1},
	} {
		s := Parse("/x", "## Questions\n"+tc.text)
		if len(s.Questions) != 1 {
			t.Errorf("%s: %d questions %+v", name, len(s.Questions), s.Questions)
			continue
		}
		q, keys := s.Questions[0], ""
		for _, o := range q.Options {
			keys += o.Key
		}
		if keys != tc.keys || q.Recommended != tc.reco || q.Options[0].Text != "oui" {
			t.Errorf("%s: keys %q reco %d %+v", name, keys, q.Recommended, q)
		}
	}
}

// From YAK-801: what is under a task belongs to it, nothing is cut.
func TestTaskBody(t *testing.T) {
	s := Parse("/x", "---\nphase: plan\n---\n# YAK-801 · T\n\n"+
		"- [ ] **5. Hasura action** : handlers sur le modèle de `removeDeal`. Erreurs :\n"+
		"  - deal non publié → `actionError` ;\n"+
		"  - rôle freelance → refus.\n"+
		"\n"+
		"  Tests de route : chaque refus, puis le succès.\n"+
		"  ```sh\n  # pas un titre\n  pnpm test\n  ```\n"+
		"- [ ] **6. UI** : le bouton.\n"+
		"  - [x] sous-tâche\n"+
		"\n## Effets de bord\n- [ ] export\n  qui continue\n  - détail\n")
	if len(s.Tasks) != 3 || s.Tasks[2].Text != "sous-tâche" || !s.Tasks[2].Done {
		t.Fatalf("tasks %+v", s.Tasks)
	}
	want := []Item{{Text: "deal non publié → `actionError` ;"}, {Text: "rôle freelance → refus."},
		{Text: "Tests de route : chaque refus, puis le succès.", Plain: true}, {Text: "  # pas un titre", Code: true}, {Text: "  pnpm test", Code: true}}
	if len(s.Tasks[0].Body) != len(want) {
		t.Fatalf("body %+v", s.Tasks[0].Body)
	}
	for i := range want {
		if s.Tasks[0].Body[i] != want[i] {
			t.Errorf("body[%d] = %+v, want %+v", i, s.Tasks[0].Body[i], want[i])
		}
	}
	if len(s.Tasks[1].Body) != 0 {
		t.Errorf("a sub-task is a task, not body: %+v", s.Tasks[1].Body)
	}
	if len(s.SideEffects) != 1 || s.SideEffects[0].Text != "export qui continue" || len(s.SideEffects[0].Body) != 1 {
		t.Errorf("side %+v", s.SideEffects)
	}
	if !s.TickSide(0, true) || !strings.Contains(s.String(), "- [x] export\n") {
		t.Errorf("tick:\n%s", s.String())
	}
}

func TestLint(t *testing.T) {
	s := Parse("/x", "---\nphase: plan\n---\n# T\n\n- [~] a\n- [~] b\n\n## Questions\n"+
		"- Garder ?\n  - oui\n  - non\n"+
		"- Migrer ?\n  - (a) script\n  - (b) migration\n"+
		"- Seul ?\n  - (a) oui\n"+
		"- Lequel ?\n  - (a) x\n  - (b) y\n  - Recommandation : (c)\n")
	var got []string
	for _, p := range s.Lint() {
		got = append(got, p.String())
	}
	all := strings.Join(got, "\n")
	for _, want := range []string{"ligne 7 : 2 tâches en cours", "ligne 10 : question « Garder ? » : 2 sous-puces sans clé",
		"ligne 13 : question « Migrer ? » : pas de recommandation", "ligne 16 : question « Seul ? » : une seule option",
		"ligne 18 : question « Lequel ? » : la recommandation nomme (c)"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in:\n%s", want, all)
		}
	}
	if len(got) != 5 {
		t.Errorf("%d problems:\n%s", len(got), all)
	}
}
