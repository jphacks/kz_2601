package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Same vocabulary as cmd/ai-session-report/platform_breakdown.go (#2368).
const platformUnknown = "unknown"

var knownPlatforms = map[string]bool{"ios": true, "android": true, "macos": true, "windows": true, "linux": true, "other": true}

func normPlatform(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case s == "":
		return platformUnknown
	case knownPlatforms[s]:
		return s
	}
	return "other"
}

// renderByPlatform returns the per-week, per-OS generation table. It counts
// the same lines as the success-rate table (create/edit, no bad_request).
func renderByPlatform(evs []event) string {
	rows := map[time.Time]map[string]*genRow{}
	seen := map[string]bool{}
	var first, last time.Time
	for _, e := range evs {
		if first.IsZero() {
			first = e.At
		}
		last = e.At
		if e.Event != "ai.generate" || e.Mode != "create" && e.Mode != "edit" || e.Outcome == "bad_request" {
			continue
		}
		w, p := week(e.At), normPlatform(e.Platform)
		if rows[w] == nil {
			rows[w] = map[string]*genRow{}
		}
		g := rows[w][p]
		if g == nil {
			g = &genRow{}
			rows[w][p] = g
		}
		g.N++
		switch e.Outcome {
		case "validated":
			g.Validated++
		case "regenerated":
			g.Regenerated++
		case "unchanged":
			g.Unchanged++
		case "refused":
			g.Refused++
		default:
			g.Failed++
		}
		seen[p] = true
	}
	var b strings.Builder
	f := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	f("\n## OS ごとの生成(週 × clientPlatform)\n\n")
	if len(seen) == 0 {
		f("期間内に ai.generate の行が無い。\n")
		return b.String()
	}
	oses := make([]string, 0, len(seen))
	for p := range seen {
		oses = append(oses, p)
	}
	sort.Strings(oses)
	f("| 週 | OS | 生成 | 成功率 | 断った | 失敗 |\n|---|---|---|---|---|---|\n")
	for w := week(first); !w.After(week(last)); w = w.AddDate(0, 0, 7) {
		for _, p := range oses {
			g := rows[w][p]
			if g == nil {
				continue
			}
			f("| %s | %s | %d | %s | %s | %s |\n", w.Format("2006-01-02"), p, g.N,
				pct(g.Validated+g.Regenerated+g.Unchanged, g.N), pct(g.Refused, g.N), pct(g.Failed, g.N))
		}
	}
	f("\nOS は行の clientPlatform、無ければ同じ requestId の http.request の clientPlatform。分からない行は %s、語彙の外は other。\n", platformUnknown)
	return b.String()
}
