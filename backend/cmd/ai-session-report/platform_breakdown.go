package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// platformUnknown is the row of ai.generate lines whose OS is not known
// (no X-Client-Platform, or logs from before it was sent).
const platformUnknown = "unknown"

// knownPlatforms is the X-Client-Platform vocabulary (#2132). Anything
// else is counted as "other" so a free-form value never becomes a row.
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

// platformRow counts the generations of one OS.
type platformRow struct {
	Generations int `json:"generations"`
	Failed      int `json:"failed"` // outcome failed or refused
}

// platformBreakdown counts ai.generate lines per client OS (#2368). The OS
// comes from the line's clientPlatform, or else from the http.request line
// of the same requestId.
type platformBreakdown struct {
	Rows map[string]*platformRow `json:"rows"`
}

func platformBreakdowns(logs []logEvent, since, until time.Time) *platformBreakdown {
	byReq := map[string]string{}
	for _, e := range logs {
		if e.RequestID != "" && e.Platform != "" {
			byReq[e.RequestID] = e.Platform
		}
	}
	r := &platformBreakdown{Rows: map[string]*platformRow{}}
	for _, e := range logs {
		if e.Event != evGenerate || e.Outcome == "restored" || e.At.Before(since) || e.At.After(until) {
			continue
		}
		p := e.Platform
		if p == "" {
			p = byReq[e.RequestID]
		}
		p = normPlatform(p)
		row := r.Rows[p]
		if row == nil {
			row = &platformRow{}
			r.Rows[p] = row
		}
		row.Generations++
		if e.Outcome == "failed" || e.Outcome == "refused" {
			row.Failed++
		}
	}
	return r
}

// renderPlatformBreakdown writes the per-OS section (labels and counts only).
func renderPlatformBreakdown(b *strings.Builder, r *platformBreakdown) {
	p := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	p("\n## OS ごとの生成(ai.generate × X-Client-Platform)\n\n")
	if len(r.Rows) == 0 {
		p("期間内に ai.generate の行が無い。\n")
		return
	}
	keys := make([]string, 0, len(r.Rows))
	for k := range r.Rows {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, c := r.Rows[keys[i]], r.Rows[keys[j]]
		if a.Generations != c.Generations {
			return a.Generations > c.Generations
		}
		return keys[i] < keys[j]
	})
	p("| OS | 生成 | 失敗 | 失敗の割合 |\n|---|---|---|---|\n")
	for _, k := range keys {
		row := r.Rows[k]
		p("| %s | %d | %d | %s |\n", k, row.Generations, row.Failed, pct(row.Failed, row.Generations))
	}
	p("\n失敗は outcome が failed・refused。OS は同じ requestId の http.request の clientPlatform から取る。分からない行は %s。\n", platformUnknown)
}
