package state

import "testing"

func TestWaits(t *testing.T) {
	cases := []struct {
		wait             string
		push, pr, isPlan bool
	}{
		{"Prêt à pousser", true, false, false},
		{"Prêt à pousser — ouvrir la PR Bruno (commit 3fa2dde)", true, false, false},
		{"Prêt à ouvrir la PR — ce qu'elle contient", false, true, false},
		{"ready to open the PR", false, true, false},
		{"Valider le plan", false, false, true},
		{"Valider le plan E", false, false, true},
		// A word found mid-sentence is not the wait.
		{"Valider le plan : ouvrir l'écran de profil", false, false, true},
		{"Valider le plan (notifications push)", false, false, true},
		{"Choisir entre A et B", false, false, false},
	}
	for _, c := range cases {
		if got := IsPushWait(c.wait); got != c.push {
			t.Errorf("IsPushWait(%q) = %v", c.wait, got)
		}
		if got := IsPRWait(c.wait); got != c.pr {
			t.Errorf("IsPRWait(%q) = %v", c.wait, got)
		}
		if got := IsPlanWait(c.wait); got != c.isPlan {
			t.Errorf("IsPlanWait(%q) = %v", c.wait, got)
		}
	}
}
