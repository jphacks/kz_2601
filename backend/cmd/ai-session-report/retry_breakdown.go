package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// retryBreakdown counts why ai.generate attempts were rejected (#1772):
// retry_reasons and the dsl problem codes of validation failures.
type retryBreakdown struct {
	Lines       int            `json:"generateLines"`       // ai.generate lines in the window
	Retried     int            `json:"retriedLines"`        // lines with at least one retry reason
	ByMode      map[string]int `json:"retriedLinesByMode"`  // mode -> lines with a retry
	Reasons     map[string]int `json:"reasons"`             // reason -> rejected attempts
	ReasonLines map[string]int `json:"reasonLines"`         // reason -> lines with it
	Codes       map[string]int `json:"validationCodes"`     // code -> problems
	CodeLines   map[string]int `json:"validationCodeLines"` // code -> lines with it
}

// retryBreakdowns reads the ai.generate lines in [since, until].
func retryBreakdowns(logs []logEvent, since, until time.Time) retryBreakdown {
	r := retryBreakdown{ByMode: map[string]int{}, Reasons: map[string]int{}, ReasonLines: map[string]int{},
		Codes: map[string]int{}, CodeLines: map[string]int{}}
	for _, e := range logs {
		if e.Event != evGenerate || e.At.Before(since) || e.At.After(until) {
			continue
		}
		r.Lines++
		if len(e.RetryReasons) > 0 {
			r.Retried++
			r.ByMode[firstNonEmpty(e.Mode, "unknown")]++
		}
		seen := map[string]bool{}
		for _, x := range e.RetryReasons {
			r.Reasons[x]++
			if !seen[x] {
				seen[x] = true
				r.ReasonLines[x]++
			}
		}
		for c, n := range e.ValidationCodes {
			r.Codes[c] += n
			r.CodeLines[c]++
		}
	}
	return r
}

// byCount orders keys by count (desc), then name.
func byCount(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys
}

// renderRetryBreakdown writes the retry section (labels and counts only).
func renderRetryBreakdown(b *strings.Builder, r retryBreakdown) {
	p := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	p("\n## 作り直しの理由(ai.generate)\n\n")
	if r.Retried == 0 {
		p("retry_reasons の付いた行が無い(作り直しが無い・このログを出す前の期間・ログを渡していない)。\n")
		return
	}
	modes := byCount(r.ByMode)
	parts := make([]string, len(modes))
	for i, m := range modes {
		parts[i] = fmt.Sprintf("%s %d", m, r.ByMode[m])
	}
	p("- 作り直しのあった依頼: %d / %d(%s・%s)\n\n", r.Retried, r.Lines, pct(r.Retried, r.Lines), strings.Join(parts, "・"))
	p("| 理由 | 落ちた回 | 依頼 |\n|---|---|---|\n")
	for _, k := range byCount(r.Reasons) {
		p("| %s | %d | %d |\n", k, r.Reasons[k], r.ReasonLines[k])
	}
	if len(r.Codes) == 0 {
		p("\nvalidation_codes の付いた行が無い(コードを出す前の期間)。\n")
		return
	}
	p("\n| 検証のコード | 問題の数 | 依頼 |\n|---|---|---|\n")
	for _, k := range byCount(r.Codes) {
		p("| %s | %d | %d |\n", k, r.Codes[k], r.CodeLines[k])
	}
	p("\n多いコードから AI の決まり・部品の説明を直す(prompt.go / catalog.go を変える時は持ち主の判断)。\n")
}
