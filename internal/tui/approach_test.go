package tui

import (
	"strings"
	"testing"

	"cairn/internal/chantier"
	"cairn/internal/state"
)

const approachPlan = "---\nphase: plan\nattente: Valider le plan\n---\nAjouter la colonne Modèle à l'export.\n\n" +
	"## Approche\n- **Choix** : une colonne dans le mapper, avec `formatModel`.\n- **Écarté** : le libellé calculé en SQL.\n\n" +
	"- [ ] select\n- [ ] mapper\n\n## Questions\n- Libellé ou code ?\n"

// lineOf returns the index of the first screen line containing s, or -1.
func lineOf(lines []string, s string) int {
	for i, l := range lines {
		if strings.Contains(l, s) {
			return i
		}
	}
	return -1
}

func TestApproachShownBeforeTasksWhenPlanWaits(t *testing.T) {
	m := fixture(t, 140, 60)
	c := m.chantiers[0]
	c.State = state.Parse("/x/YAK-861.md", approachPlan)
	lines := screen(m)
	goal, appr, choice, task := lineOf(lines, "Objectif"), lineOf(lines, "Approche"), lineOf(lines, "Choix : une colonne"), lineOf(lines, "select")
	if appr < 0 || choice < 0 {
		t.Fatalf("approach missing:\n%s", strings.Join(lines, "\n"))
	}
	if !(goal < appr && appr < choice && choice < task) {
		t.Fatalf("want goal < approach < tasks, got %d %d %d %d:\n%s", goal, appr, choice, task, strings.Join(lines, "\n"))
	}
}

func TestApproachFoldedDuringBuild(t *testing.T) {
	m := fixture(t, 140, 60)
	c := m.chantiers[0]
	c.Phase = chantier.Build
	c.State = state.Parse("/x/YAK-861.md", strings.Replace(approachPlan, "phase: plan\nattente: Valider le plan", "phase: build", 1))
	lines := screen(m)
	if lineOf(lines, "Approche") < 0 || lineOf(lines, "2 points") < 0 {
		t.Fatalf("approach should be folded:\n%s", strings.Join(lines, "\n"))
	}
	if lineOf(lines, "Choix : une colonne") >= 0 {
		t.Fatalf("folded approach shows its items")
	}
	m.unfold[c.Key+":approach"] = true
	if lineOf(screen(m), "Choix : une colonne") < 0 {
		t.Fatalf("unfolded approach hides its items")
	}
}
