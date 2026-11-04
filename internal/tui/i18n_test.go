package tui

import (
	"strings"
	"testing"
	"time"

	"cairn/internal/chantier"
	"cairn/internal/i18n"
	"cairn/internal/journal"
)

func TestEnglish(t *testing.T) {
	i18n.Set("en")
	t.Cleanup(func() { i18n.Set("fr") })

	at := time.Now()
	lines := historyLines([]journal.Event{
		{At: at, Text: "plan validé"},
		{At: at, Text: "phase:Livré"},
		{At: at, Text: "plan lancé (yak-1)"},
		{At: at, Text: "something else"},
	}, 10)
	want := []string{"something else", "plan started (yak-1)", "→ Shipped", "plan approved"}
	for i, w := range want {
		if !strings.HasSuffix(lines[i], w) {
			t.Errorf("line %d = %q, want suffix %q", i, lines[i], w)
		}
	}
	if got := since(time.Now().Add(-49 * time.Hour)); got != "for 2 d" {
		t.Errorf("since = %q", got)
	}
	if got := chantier.ToPlan.Label(); got != "To plan" {
		t.Errorf("label = %q", got)
	}
	if got := chantier.Shipped.String(); got != "Livré" {
		t.Errorf("String must stay French, got %q", got)
	}
}
