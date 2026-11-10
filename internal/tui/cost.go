package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"cairn/internal/chantier"
	"cairn/internal/i18n"
	"cairn/internal/journal"
	"cairn/internal/progress"
	"cairn/internal/usage"
)

// What a chantier's agents cost, read from Claude Code's transcripts. At API
// rates: on a subscription, an equivalent that says what weighs.

// money formats dollars the way the interface language writes them.
func money(d float64) string {
	var s string
	switch {
	case d < 0.01:
		s = "< 0.01"
	case d < 10:
		s = fmt.Sprintf("%.2f", d)
	default:
		s = fmt.Sprintf("%.0f", d)
	}
	if !i18n.English() {
		s = strings.Replace(s, ".", ",", 1)
	}
	return s + " $"
}

// shortMoney is the list's rounding: "2,4 $", "38 $".
func shortMoney(d float64) string {
	s := fmt.Sprintf("%.0f", d)
	if d < 10 {
		s = fmt.Sprintf("%.1f", d)
	}
	if !i18n.English() {
		s = strings.Replace(s, ".", ",", 1)
	}
	return "~" + s + " $"
}

// count formats a number of tokens: 840, 41 k, 3,1 M.
func count(n int64) string {
	var s string
	switch {
	case n < 1000:
		return fmt.Sprint(n)
	case n < 1_000_000:
		s = fmt.Sprintf("%.0f k", float64(n)/1e3)
	default:
		s = fmt.Sprintf("%.1f M", float64(n)/1e6)
	}
	if !i18n.English() {
		s = strings.Replace(s, ".", ",", 1)
	}
	return s
}

func pct(part, whole float64) string {
	if whole <= 0 {
		return "0 %"
	}
	return fmt.Sprintf("%.0f %%", 100*part/whole)
}

func agentName(a string) string {
	if a == usage.Main {
		return i18n.T("principal", "main")
	}
	return a
}

// costOf is the chantier's total, ok false when nothing was read.
func (m Model) costOf(c *chantier.Chantier) (usage.Sum, bool) {
	rep, ok := m.usage[c.Key]
	if !ok || len(rep.Entries) == 0 {
		return usage.Sum{}, false
	}
	if s, ok := m.usageSum[c.Key]; ok {
		return s, true
	}
	return usage.Summarize(rep.Entries, m.prices(), time.Time{}, time.Time{}), true
}

// phaseSpans cuts the journal's phase changes into spans; what happened
// before the first change counts in the first phase.
func phaseSpans(events []journal.Event) (names []string, from []time.Time) {
	for _, e := range events {
		p, ok := strings.CutPrefix(e.Text, "phase:")
		if !ok {
			continue
		}
		if len(names) > 0 && names[len(names)-1] == p {
			continue
		}
		names, from = append(names, p), append(from, e.At)
	}
	if len(from) > 0 {
		from[0] = time.Time{}
	}
	return names, from
}

func phaseLabel(name string) string {
	for _, ph := range []chantier.Phase{chantier.ToPlan, chantier.Plan, chantier.Build, chantier.Review, chantier.Shipped} {
		if ph.String() == name {
			return ph.Label()
		}
	}
	return name
}

func (m Model) secCost(b *detailBuilder, c *chantier.Chantier) {
	rep := m.usage[c.Key]
	sum, ok := m.costOf(c)
	if !ok {
		return
	}
	prices := m.prices()
	b.section(i18n.T("Coût", "Cost"), sBold.Render(money(sum.Cost)), i18n.T("équivalent API", "API equivalent"))
	// By model, with its share of the cost.
	for _, l := range sum.ByModel {
		name := usage.ShortModel(l.Name)
		share := sMuted.Render(pct(l.Cost, sum.Cost))
		if _, known := prices.For(l.Name); !known {
			share = sWarn.Render(i18n.T("prix inconnu", "no price"))
		}
		b.add("  " + lbl(name) + fmt.Sprintf("%-9s", money(l.Cost)) + bar(int(1000*l.Cost), 0, int(1000*sum.Cost)+1, 12) + " " + share)
	}
	// By phase, from the journal.
	if names, from := phaseSpans(m.watch.history[c.Key]); len(names) > 1 {
		var parts []string
		for i, n := range names {
			var to time.Time
			if i+1 < len(from) {
				to = from[i+1]
			}
			if s := usage.Summarize(rep.Entries, prices, from[i], to); s.Tokens.Total() > 0 {
				parts = append(parts, phaseLabel(n)+" "+money(s.Cost))
			}
		}
		if len(parts) > 0 {
			b.add(hang("  "+lbl(i18n.T("Phases", "Phases")), strings.Join(parts, " · "), b.w)...)
		}
	}
	// Main conversation and subagents.
	if len(sum.ByAgent) > 1 || rep.Subs > 0 {
		var parts []string
		for _, l := range sum.ByAgent {
			parts = append(parts, agentName(l.Name)+" "+money(l.Cost))
		}
		line := strings.Join(parts, " · ")
		if rep.Subs > 0 {
			line += sMuted.Render(" (" + plural(rep.Subs, i18n.T("sous-agent", "subagent"), i18n.T("sous-agents", "subagents")) + ")")
		}
		b.add(hang("  "+lbl("Agents"), line, b.w)...)
	}
	// What the tokens are: re-read context weighs most, usually.
	t := sum.Tokens
	read := float64(t.CacheRead) / float64(max(1, t.Total()))
	b.add(hang("  "+lbl("Tokens"), fmt.Sprintf(i18n.T("%s, dont %s de contexte relu · %s écrits", "%s, %s of it re-read context · %s written"),
		count(t.Total()), fmt.Sprintf("%.0f %%", 100*read), count(t.Output)), b.w)...)
}

// lastActions are what the chantier's agents did last, when recent.
func (m Model) lastActions(c *chantier.Chantier) []string {
	var out []string
	for _, a := range m.usage[c.Key].Last {
		if time.Since(a.At) > 15*time.Minute {
			continue
		}
		what := a.Tool
		if a.Target != "" {
			what += " " + a.Target
		}
		out = append(out, "    "+sDim.Render("↳ ")+lipgloss.NewStyle().Foreground(soft).Render(agentName(a.Agent)+" · "+what)+sMuted.Render(" · "+progress.Ago(a.At)))
	}
	return out
}
