package usage

import (
	"sort"
	"strings"
	"time"
)

// Price is in dollars per million tokens, at API rates. On a Claude
// subscription nothing is billed per token: the figure is then an API
// equivalent, useful to compare chantiers and to see what weighs.
type Price struct {
	Input        float64 `json:"input"`
	Output       float64 `json:"output"`
	CacheRead    float64 `json:"cache_read"`
	CacheWrite5m float64 `json:"cache_write_5m"`
	CacheWrite1h float64 `json:"cache_write_1h"`
}

// Prices maps a model id prefix ("claude-opus-5-5") to its price; the longest
// prefix that matches a model wins.
type Prices map[string]Price

// DefaultPrices are Anthropic's published API rates (2026-11). Cache writes
// cost 1.25× the input rate for 5 minutes, 2× for an hour. Haiku 5.5's cache
// read rate is not published here: 0.1× input is an estimate. The config's
// "prices" overrides or completes them.
var DefaultPrices = Prices{
	"claude-fable-5-2":  {8, 40, 0.20, 10, 16},
	"claude-mythos-5-2": {8, 40, 0.20, 10, 16},
	"claude-fable-5-1":  {10, 50, 0.25, 12.5, 20},
	"claude-fable-5":    {10, 50, 1, 12.5, 20},
	"claude-opus-5-5":   {4, 20, 0.20, 5, 8},
	"claude-opus-5":     {5, 25, 0.50, 6.25, 10},
	"claude-opus-4":     {5, 25, 0.50, 6.25, 10},
	"claude-sonnet-5-5": {2, 10, 0.10, 2.5, 4},
	"claude-sonnet-5":   {2, 10, 0.20, 2.5, 4},
	"claude-sonnet-4-6": {3, 15, 0.30, 3.75, 6},
	"claude-haiku-5-5":  {0.10, 0.50, 0.01, 0.125, 0.20},
	"claude-haiku-4-5":  {1, 5, 0.10, 1.25, 2},
}

// With returns p completed by extra, extra winning.
func (p Prices) With(extra map[string]Price) Prices {
	out := Prices{}
	for k, v := range p {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// For finds the price of model; ok is false for an unknown model.
func (p Prices) For(model string) (Price, bool) {
	best := ""
	for k := range p {
		if strings.HasPrefix(model, k) && len(k) > len(best) {
			best = k
		}
	}
	if best == "" {
		return Price{}, false
	}
	return p[best], true
}

// Cost of t at price pr, in dollars.
func (pr Price) Cost(t Tokens) float64 {
	return (float64(t.Input)*pr.Input + float64(t.Output)*pr.Output + float64(t.CacheRead)*pr.CacheRead +
		float64(t.CacheWrite5m)*pr.CacheWrite5m + float64(t.CacheWrite1h)*pr.CacheWrite1h) / 1e6
}

// Line is one row of a breakdown.
type Line struct {
	Name   string
	Tokens Tokens
	Cost   float64
}

// Sum is a set of entries added up.
type Sum struct {
	Tokens  Tokens
	Cost    float64
	Unknown int64 // tokens of models without a price
	ByModel []Line
	ByAgent []Line
}

// Summarize adds up the entries between from and to (zero: unbounded),
// largest cost first.
func Summarize(entries []Entry, prices Prices, from, to time.Time) Sum {
	var s Sum
	models, agents := map[string]*Line{}, map[string]*Line{}
	for _, e := range entries {
		if (!from.IsZero() && e.At.Before(from)) || (!to.IsZero() && !e.At.Before(to)) {
			continue
		}
		cost := 0.0
		if pr, ok := prices.For(e.Model); ok {
			cost = pr.Cost(e.Tokens)
		} else {
			s.Unknown += e.Tokens.Total()
		}
		s.Tokens = s.Tokens.Add(e.Tokens)
		s.Cost += cost
		for key, m := range map[string]map[string]*Line{e.Model: models, e.Agent: agents} {
			l := m[key]
			if l == nil {
				l = &Line{Name: key}
				m[key] = l
			}
			l.Tokens = l.Tokens.Add(e.Tokens)
			l.Cost += cost
		}
	}
	s.ByModel, s.ByAgent = sorted(models), sorted(agents)
	return s
}

func sorted(m map[string]*Line) []Line {
	var out []Line
	for _, l := range m {
		out = append(out, *l)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Cost != out[j].Cost {
			return out[i].Cost > out[j].Cost
		}
		return out[i].Tokens.Total() > out[j].Tokens.Total()
	})
	return out
}

// ShortModel is a model id as people say it: "claude-opus-5-5" → "opus 5.5".
func ShortModel(id string) string {
	s := strings.TrimPrefix(id, "claude-")
	parts := strings.Split(s, "-")
	if len(parts) < 2 {
		return s
	}
	var ver []string
	for _, p := range parts[1:] {
		if len(p) > 2 { // a date suffix
			break
		}
		ver = append(ver, p)
	}
	return parts[0] + " " + strings.Join(ver, ".")
}
