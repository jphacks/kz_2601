package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// usage-cost (#2294): per ai.usage operation (generate, patch, chat_first ...)
// the calls, the share with a cache read / write and est_cost_usd per call
// (median) and in total. Two -period flags print before → after, to compare
// a prompt-caching change (#1770).
func init() {
	registerLogMode("usage-cost", logMode{
		summary: "calls, cache read / write share and cost per ai.usage operation (one period, or before → after)",
		run:     runUsageCost,
	})
}

func runUsageCost(args []string, stdout io.Writer) error {
	fs := newModeFlags("usage-cost", os.Stderr)
	var logs, periods listFlag
	fs.Var(&logs, "logs", "`gcloud logging read --format=json` exports with jsonPayload.event=\"ai.usage\" (comma-separated or repeated)")
	fs.Var(&periods, "period", "YYYY-MM-DD..YYYY-MM-DD in JST, days included; once = one table, twice = before → after")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("usage-cost: unexpected argument %q", fs.Arg(0))
	}
	if len(periods) < 1 || len(periods) > 2 {
		return errors.New("usage-cost: pass -period once (one table) or twice (before → after)")
	}
	ps := make([]period, len(periods))
	for i, s := range periods {
		p, err := parsePeriod(s)
		if err != nil {
			return err
		}
		ps[i] = p
	}
	evs, err := readAllLogs(logs)
	if err != nil {
		return err
	}
	stats := make([]map[string]*usageStat, len(ps))
	for i, p := range ps {
		stats[i] = tallyUsage(evs, p)
	}
	renderUsageCost(stdout, ps, stats)
	return nil
}

// usageStat is one operation's ai.usage lines in a period.
type usageStat struct {
	Calls      int
	CacheRead  int // lines with cache_read_input_tokens > 0
	CacheWrite int // lines with cache_creation_input_tokens > 0
	Costs      []float64
}

func (s *usageStat) add(e logEvent) {
	s.Calls++
	if e.CacheRead > 0 {
		s.CacheRead++
	}
	if e.CacheWrite > 0 {
		s.CacheWrite++
	}
	if e.HasCost {
		s.Costs = append(s.Costs, e.CostUSD)
	}
}

func (s *usageStat) total() float64 {
	var t float64
	for _, c := range s.Costs {
		t += c
	}
	return t
}

// median of the priced lines; ok is false when none had a price.
func (s *usageStat) median() (float64, bool) {
	n := len(s.Costs)
	if n == 0 {
		return 0, false
	}
	c := append([]float64(nil), s.Costs...)
	sort.Float64s(c)
	if n%2 == 1 {
		return c[n/2], true
	}
	return (c[n/2-1] + c[n/2]) / 2, true
}

// allOps is the key of the row that sums every operation.
const allOps = "(all)"

// tallyUsage counts every ai.usage line in p by operation, plus allOps.
// Every attempt is a billed call, so retries count too.
func tallyUsage(evs []logEvent, p period) map[string]*usageStat {
	out := map[string]*usageStat{allOps: {}}
	for _, e := range evs {
		if e.Event != evUsage || !p.has(e.At) {
			continue
		}
		op := e.Operation
		if op == "" {
			op = "(none)"
		}
		if out[op] == nil {
			out[op] = &usageStat{}
		}
		out[op].add(e)
		out[allOps].add(e)
	}
	return out
}

func renderUsageCost(w io.Writer, ps []period, stats []map[string]*usageStat) {
	ops := map[string]bool{}
	for _, st := range stats {
		for op := range st {
			if op != allOps {
				ops[op] = true
			}
		}
	}
	names := make([]string, 0, len(ops)+1)
	for op := range ops {
		names = append(names, op)
	}
	sort.Strings(names)
	names = append(names, allOps)

	labels := make([]string, len(ps))
	for i, p := range ps {
		labels[i] = p.Label
	}
	_, _ = fmt.Fprintf(w, "## ai.usage by operation: %s\n\n", strings.Join(labels, " → "))
	_, _ = fmt.Fprintln(w, "| operation | calls | cache read | cache write | cost / call (median, USD) | cost total (USD) |")
	_, _ = fmt.Fprintln(w, "| --- | ---: | ---: | ---: | ---: | ---: |")
	empty := &usageStat{}
	for _, op := range names {
		cells := make([][]string, 5)
		for _, st := range stats {
			s := st[op]
			if s == nil {
				s = empty
			}
			med := "-"
			if m, ok := s.median(); ok {
				med = fmt.Sprintf("%.4f", m)
			}
			total := "-"
			if len(s.Costs) > 0 {
				total = fmt.Sprintf("%.4f", s.total())
			}
			cells[0] = append(cells[0], fmt.Sprint(s.Calls))
			cells[1] = append(cells[1], share(s.CacheRead, s.Calls))
			cells[2] = append(cells[2], share(s.CacheWrite, s.Calls))
			cells[3] = append(cells[3], med)
			cells[4] = append(cells[4], total)
		}
		row := make([]string, len(cells))
		for i, c := range cells {
			row[i] = strings.Join(c, " → ")
		}
		_, _ = fmt.Fprintf(w, "| %s | %s |\n", op, strings.Join(row, " | "))
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "cache read / write: share of calls with cache_read_input_tokens / cache_creation_input_tokens > 0. cost: est_cost_usd of the lines that have it (models without a price are left out of cost but counted in calls).")
}

// share is "n/total (p%)", or "-" when there is nothing to divide.
func share(n, total int) string {
	if total == 0 {
		return "-"
	}
	return fmt.Sprintf("%d/%d (%.0f%%)", n, total, 100*float64(n)/float64(total))
}
