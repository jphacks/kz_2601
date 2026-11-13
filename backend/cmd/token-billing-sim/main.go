// Command token-billing-sim estimates, from today's ai.usage log lines, how
// many tokens each person would spend if usage were counted in tokens
// (#630). It is material for that decision only: it reads a log export and
// a weights file and prints distributions. No prices, no Firestore, no API.
//
//	gcloud logging read 'jsonPayload.event="ai.usage" AND timestamp>="..."' --format=json > usage.json
//	go run ./cmd/token-billing-sim -logs usage.json -weights cmd/token-billing-sim/testdata/weights_by_kind.json
//
// Weights (JSON, one file per way of counting; -weights may repeat to
// compare them side by side; the // notes are not part of the file):
//
//	{
//	  "name": "by kind",
//	  "count_errors": false,   // count calls whose outcome is "error"
//	  "count_retries": false,  // count attempt > 1 (server-side retries)
//	  "operations": {          // ai.usage operation -> weight; "*" = any other
//	    "generate": {"call": 10, "input_per_1k": 0, "output_per_1k": 1,
//	                 "cache_read_per_1k": 0, "cache_write_per_1k": 0}
//	  }
//	}
//
// One line costs call + tokens / 1000 × the per-1k weights. A line is
// charged to payer_uid_hash when present (a shared app's creator pays for
// participants), else uid_hash; lines without either are counted apart.
// Days and months are JST. Only counts leave the process (no hashes).
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

var jst = time.FixedZone("JST", 9*60*60)

const anyOp = "*"

type opWeight struct {
	Call            float64 `json:"call"`
	InputPer1K      float64 `json:"input_per_1k"`
	OutputPer1K     float64 `json:"output_per_1k"`
	CacheReadPer1K  float64 `json:"cache_read_per_1k"`
	CacheWritePer1K float64 `json:"cache_write_per_1k"`
}

type weights struct {
	Name         string              `json:"name"`
	CountErrors  bool                `json:"count_errors"`
	CountRetries bool                `json:"count_retries"`
	Operations   map[string]opWeight `json:"operations"`
}

func loadWeights(path string) (weights, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return weights{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var w weights
	if err := dec.Decode(&w); err != nil {
		return weights{}, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if w.Name == "" {
		w.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if len(w.Operations) == 0 {
		return weights{}, fmt.Errorf("%s: operations is empty", filepath.Base(path))
	}
	for op, ow := range w.Operations {
		for _, v := range []float64{ow.Call, ow.InputPer1K, ow.OutputPer1K, ow.CacheReadPer1K, ow.CacheWritePer1K} {
			if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
				return weights{}, fmt.Errorf("%s: operation %q: weights must be >= 0", filepath.Base(path), op)
			}
		}
	}
	return w, nil
}

// usage is one ai.usage line (labels and counts only).
type usage struct {
	At                     time.Time
	Payer                  string
	Operation, Outcome     string
	Attempt                int
	Input, Output          float64
	CacheRead, CacheCreate float64
}

type payload map[string]any

func (p payload) str(k string) string {
	switch v := p[k].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}

func (p payload) num(k string) float64 {
	switch v := p[k].(type) {
	case float64:
		return v
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	return 0
}

// parseUsage reads `gcloud logging read --format=json` (a JSON array of
// entries) or JSON lines (entries, or bare slog records with `time`) and
// keeps the ai.usage lines.
func parseUsage(r io.Reader) ([]usage, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var raws []json.RawMessage
	if t := bytes.TrimSpace(data); len(t) > 0 && t[0] == '[' {
		if err := json.Unmarshal(t, &raws); err != nil {
			return nil, fmt.Errorf("decode JSON array: %w", err)
		}
	} else {
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			if line := bytes.TrimSpace(sc.Bytes()); len(line) > 0 && line[0] == '{' {
				raws = append(raws, append(json.RawMessage(nil), line...))
			}
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}
	var out []usage
	for _, raw := range raws {
		var e struct {
			Timestamp   string  `json:"timestamp"`
			JSONPayload payload `json:"jsonPayload"`
		}
		if json.Unmarshal(raw, &e) != nil {
			continue
		}
		p, ts := e.JSONPayload, e.Timestamp
		if p == nil {
			if json.Unmarshal(raw, &p) != nil {
				continue
			}
			ts = p.str("time")
		}
		if p.str("event") != "ai.usage" {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			continue
		}
		payer := p.str("payer_uid_hash")
		if payer == "" {
			payer = p.str("uid_hash")
		}
		op := p.str("operation")
		if op == "" {
			op = "unknown"
		}
		out = append(out, usage{
			At: at, Payer: payer, Operation: op, Outcome: p.str("outcome"),
			Attempt: int(p.num("attempt")),
			Input:   p.num("input_tokens"), Output: p.num("output_tokens"),
			CacheRead: p.num("cache_read_input_tokens"), CacheCreate: p.num("cache_creation_input_tokens"),
		})
	}
	return out, nil
}

// cost is the line's tokens under w; ok is false when the line is not
// counted (an error or a retry the weights leave out, or no weight for its
// operation).
func (w weights) cost(u usage) (float64, bool) {
	if (!w.CountErrors && u.Outcome == "error") || (!w.CountRetries && u.Attempt > 1) {
		return 0, false
	}
	ow, ok := w.Operations[u.Operation]
	if !ok {
		if ow, ok = w.Operations[anyOp]; !ok {
			return 0, false
		}
	}
	return ow.Call + (u.Input*ow.InputPer1K+u.Output*ow.OutputPer1K+
		u.CacheRead*ow.CacheReadPer1K+u.CacheCreate*ow.CacheWritePer1K)/1000, true
}

type opTotal struct {
	Op     string
	Calls  int
	Tokens float64
}

type result struct {
	Name         string
	Lines        int // ai.usage lines read
	Counted      int // lines charged to someone
	Skipped      int // errors / retries left out by the weights
	Unweighted   map[string]int
	Anonymous    int // counted lines without a payer
	Payers       int
	Day, Month   []float64 // per payer-day / payer-month totals, sorted
	Ops          []opTotal // sorted by tokens, desc
	TotalTokens  float64
	AnonymousTok float64
}

func simulate(lines []usage, w weights) result {
	r := result{Name: w.Name, Lines: len(lines), Unweighted: map[string]int{}}
	day := map[[2]string]float64{}
	month := map[[2]string]float64{}
	ops := map[string]*opTotal{}
	payers := map[string]bool{}
	for _, u := range lines {
		c, ok := w.cost(u)
		if !ok {
			if (!w.CountErrors && u.Outcome == "error") || (!w.CountRetries && u.Attempt > 1) {
				r.Skipped++
			} else {
				r.Unweighted[u.Operation]++
			}
			continue
		}
		r.Counted++
		r.TotalTokens += c
		if ops[u.Operation] == nil {
			ops[u.Operation] = &opTotal{Op: u.Operation}
		}
		ops[u.Operation].Calls++
		ops[u.Operation].Tokens += c
		if u.Payer == "" {
			r.Anonymous++
			r.AnonymousTok += c
			continue
		}
		t := u.At.In(jst)
		payers[u.Payer] = true
		day[[2]string{u.Payer, t.Format(time.DateOnly)}] += c
		month[[2]string{u.Payer, t.Format("2006-01")}] += c
	}
	r.Payers = len(payers)
	for _, v := range day {
		r.Day = append(r.Day, v)
	}
	for _, v := range month {
		r.Month = append(r.Month, v)
	}
	sort.Float64s(r.Day)
	sort.Float64s(r.Month)
	for _, o := range ops {
		r.Ops = append(r.Ops, *o)
	}
	sort.Slice(r.Ops, func(i, j int) bool {
		if r.Ops[i].Tokens != r.Ops[j].Tokens {
			return r.Ops[i].Tokens > r.Ops[j].Tokens
		}
		return r.Ops[i].Op < r.Ops[j].Op
	})
	return r
}

// nearestRank is the q-th percentile of sorted xs (nearest-rank method).
func nearestRank(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(math.Ceil(q*float64(len(sorted)))) - 1
	return sorted[max(i, 0)]
}

func num(v float64) string { return strconv.FormatFloat(math.Round(v*10)/10, 'f', -1, 64) }

func render(w io.Writer, r result, top int) {
	_, _ = fmt.Fprintf(w, "## %s\n\n", r.Name)
	_, _ = fmt.Fprintf(w, "ai.usage lines %d, counted %d, left out by the weights (errors / retries) %d, people %d\n\n", r.Lines, r.Counted, r.Skipped, r.Payers)
	_, _ = fmt.Fprintln(w, "| per person | people × periods | p50 | p90 | max |")
	_, _ = fmt.Fprintln(w, "| --- | ---: | ---: | ---: | ---: |")
	for _, row := range []struct {
		name string
		xs   []float64
	}{{"day (JST)", r.Day}, {"month (JST)", r.Month}} {
		if len(row.xs) == 0 {
			_, _ = fmt.Fprintf(w, "| %s | 0 | - | - | - |\n", row.name)
			continue
		}
		_, _ = fmt.Fprintf(w, "| %s | %d | %s | %s | %s |\n", row.name, len(row.xs),
			num(nearestRank(row.xs, 0.5)), num(nearestRank(row.xs, 0.9)), num(row.xs[len(row.xs)-1]))
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "| operation | calls | tokens | share |")
	_, _ = fmt.Fprintln(w, "| --- | ---: | ---: | ---: |")
	for i, o := range r.Ops {
		if i == top {
			_, _ = fmt.Fprintf(w, "| (%d more) | | | |\n", len(r.Ops)-top)
			break
		}
		share := "-"
		if r.TotalTokens > 0 {
			share = fmt.Sprintf("%.0f%%", 100*o.Tokens/r.TotalTokens)
		}
		_, _ = fmt.Fprintf(w, "| %s | %d | %s | %s |\n", o.Op, o.Calls, num(o.Tokens), share)
	}
	_, _ = fmt.Fprintln(w)
	if r.Anonymous > 0 {
		_, _ = fmt.Fprintf(w, "- without a uid (not signed in): %d calls, %s tokens, not in the per-person rows\n", r.Anonymous, num(r.AnonymousTok))
	}
	if len(r.Unweighted) > 0 {
		names := make([]string, 0, len(r.Unweighted))
		for op, n := range r.Unweighted {
			names = append(names, fmt.Sprintf("%s %d", op, n))
		}
		sort.Strings(names)
		_, _ = fmt.Fprintf(w, "- no weight (not counted; add it or \"*\"): %s\n", strings.Join(names, ", "))
	}
}

type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }
func (l *listFlag) Set(s string) error {
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			*l = append(*l, p)
		}
	}
	return nil
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("token-billing-sim", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var logs, ws listFlag
	fs.Var(&logs, "logs", "`gcloud logging read --format=json` exports of ai.usage (comma-separated or repeated)")
	fs.Var(&ws, "weights", "weights JSON (comma-separated or repeated: one section each)")
	top := fs.Int("top", 10, "operations listed per section")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if len(logs) == 0 || len(ws) == 0 {
		return errors.New("-logs and -weights are required")
	}
	if *top < 1 {
		return errors.New("-top must be positive")
	}
	var all []weights
	for _, p := range ws {
		w, err := loadWeights(p)
		if err != nil {
			return err
		}
		all = append(all, w)
	}
	var lines []usage
	for _, p := range logs {
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		us, err := parseUsage(f)
		_ = f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(p), err)
		}
		lines = append(lines, us...)
	}
	for i, w := range all {
		if i > 0 {
			_, _ = fmt.Fprintln(stdout)
		}
		render(stdout, simulate(lines, w), *top)
	}
	return nil
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "token-billing-sim:", err)
		os.Exit(2)
	}
}
