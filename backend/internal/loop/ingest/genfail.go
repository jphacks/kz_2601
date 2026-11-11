package ingest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
)

// Generation-failure spikes (#368, #297): a failure code whose share of the
// ai.generate lines in the last window jumps against the window before it
// becomes one inbox item. Only codes and counts are kept: no user text, uid,
// uid_hash, app id or app_hash ever leave the log reader.

// EnvGenFailOn turns the generation-failure source on.
const EnvGenFailOn = "LOOP_SOURCE_GENFAIL"

// Generation-failure settings (all optional except the logs).
const (
	EnvGenFailLogs     = "LOOP_GENFAIL_LOGS"      // comma-separated `gcloud logging read --format=json` exports
	EnvGenFailWindow   = "LOOP_GENFAIL_WINDOW"    // Go duration, default 24h
	EnvGenFailBaseline = "LOOP_GENFAIL_BASELINE"  // Go duration, default 168h
	EnvGenFailMinCount = "LOOP_GENFAIL_MIN_COUNT" // default 5
	EnvGenFailMinRate  = "LOOP_GENFAIL_MIN_RATE"  // default 0.05
	EnvGenFailFactor   = "LOOP_GENFAIL_FACTOR"    // default 2
)

// ErrGenFailOff is returned by GenFailConfigFromEnv when the source is off.
var ErrGenFailOff = errors.New("generation-failure spikes are off (" + EnvGenFailOn + " != on)")

// GenEvent is the part of one ai.generate log line the detector reads.
// Labels only: the decoder never reads uid_hash or app_hash.
type GenEvent struct {
	At              time.Time
	Mode            string
	Outcome         string // validated / regenerated / unchanged / restored / refused / bad_request / failed
	ErrorCode       string
	RetryReasons    []string
	ValidationCodes map[string]int
}

// GenLogSource reads ai.generate lines in [since, until]. The real reader is
// a file the operator exported with `gcloud logging read` (the same input as
// cmd/ai-session-report); this package never calls the Logging API.
type GenLogSource interface {
	GenerateEvents(ctx context.Context, since, until time.Time) ([]GenEvent, error)
}

// GenLogFiles reads `gcloud logging read --format=json` exports (a JSON array
// of entries, or JSON lines of entries or bare slog records).
type GenLogFiles struct{ Paths []string }

// GenerateEvents implements GenLogSource.
func (g GenLogFiles) GenerateEvents(_ context.Context, since, until time.Time) ([]GenEvent, error) {
	var out []GenEvent
	for _, p := range g.Paths {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		evs, err := ParseGenLogs(f)
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		out = append(out, evs...)
	}
	return genInRange(out, since, until), nil
}

// StaticGenLogs is a GenLogSource over a fixed list (tests).
type StaticGenLogs []GenEvent

// GenerateEvents implements GenLogSource.
func (s StaticGenLogs) GenerateEvents(_ context.Context, since, until time.Time) ([]GenEvent, error) {
	return genInRange(s, since, until), nil
}

// genInRange keeps the events in [since, until].
func genInRange(evs []GenEvent, since, until time.Time) []GenEvent {
	out := []GenEvent{}
	for _, e := range evs {
		if !e.At.Before(since) && !e.At.After(until) {
			out = append(out, e)
		}
	}
	return out
}

const genEvent = "ai.generate"

// genLabel is a plain label (same rule as cmd/ai-session-report).
var genLabel = regexp.MustCompile(`^[a-z][a-z0-9_]{0,40}$`)

// ParseGenLogs decodes the ai.generate lines of an export. Other events and
// lines that do not decode are skipped; a label that is not a plain label is
// dropped, so free text never reaches a code.
func ParseGenLogs(r io.Reader) ([]GenEvent, error) {
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
			line := bytes.TrimSpace(sc.Bytes())
			if len(line) == 0 || line[0] != '{' {
				continue
			}
			raws = append(raws, append(json.RawMessage(nil), line...))
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}
	out := []GenEvent{}
	for _, raw := range raws {
		if ev, ok := decodeGenEntry(raw); ok {
			out = append(out, ev)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

// genPayload holds only the fields the detector needs.
type genPayload struct {
	Time            string `json:"time"`
	Event           string `json:"event"`
	Mode            string `json:"mode"`
	Outcome         string `json:"outcome"`
	ErrorCode       string `json:"error_code"`
	RetryReasons    string `json:"retry_reasons"`
	ValidationCodes string `json:"validation_codes"`
}

func decodeGenEntry(raw json.RawMessage) (GenEvent, bool) {
	var e struct {
		Timestamp   string      `json:"timestamp"`
		JSONPayload *genPayload `json:"jsonPayload"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return GenEvent{}, false
	}
	p, ts := e.JSONPayload, e.Timestamp
	if p == nil { // bare slog record
		p = &genPayload{}
		if err := json.Unmarshal(raw, p); err != nil {
			return GenEvent{}, false
		}
		ts = p.Time
	}
	if p.Event != genEvent {
		return GenEvent{}, false
	}
	at, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return GenEvent{}, false
	}
	return GenEvent{
		At:              at.UTC(),
		Mode:            plainLabel(p.Mode),
		Outcome:         plainLabel(p.Outcome),
		ErrorCode:       plainLabel(p.ErrorCode),
		RetryReasons:    genLabels(p.RetryReasons),
		ValidationCodes: genCodeCounts(p.ValidationCodes),
	}, true
}

func plainLabel(s string) string {
	if s = strings.TrimSpace(s); genLabel.MatchString(s) {
		return s
	}
	return ""
}

// genLabels reads "a,b,a" (retry_reasons).
func genLabels(s string) []string {
	var out []string
	for _, l := range strings.Split(s, ",") {
		if l = plainLabel(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// genCodeCounts reads "code:n,code:n" (validation_codes, #1772).
func genCodeCounts(s string) map[string]int {
	var out map[string]int
	for _, part := range strings.Split(s, ",") {
		code, n, ok := strings.Cut(strings.TrimSpace(part), ":")
		c, err := strconv.Atoi(n)
		if !ok || err != nil || c <= 0 || !genLabel.MatchString(code) {
			continue
		}
		if out == nil {
			out = map[string]int{}
		}
		out[code] += c
	}
	return out
}

// FailureCodes are the failure codes of one line, each once:
// outcome:failed / outcome:refused, error:<error_code>, retry:<reason> and
// validation:<dsl problem code>.
func (e GenEvent) FailureCodes() []string {
	seen := map[string]bool{}
	add := func(c string) { seen[c] = true }
	if e.Outcome == "failed" || e.Outcome == "refused" {
		add("outcome:" + e.Outcome)
	}
	if e.ErrorCode != "" {
		add("error:" + e.ErrorCode)
	}
	for _, r := range e.RetryReasons {
		add("retry:" + r)
	}
	for c := range e.ValidationCodes {
		add("validation:" + c)
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// GenFailThreshold says when a code is a spike. A code spikes when, in the
// window, it is on at least MinCount lines, its rate (lines with it / all
// ai.generate lines) is at least MinRate, and that rate is at least Factor
// times its rate in the baseline (the Baseline before the window). A code
// absent from the baseline only needs MinCount and MinRate.
type GenFailThreshold struct {
	Window   time.Duration
	Baseline time.Duration
	MinCount int
	MinRate  float64
	Factor   float64
}

// DefaultGenFailThreshold: a day against the week before it.
var DefaultGenFailThreshold = GenFailThreshold{Window: 24 * time.Hour, Baseline: 7 * 24 * time.Hour, MinCount: 5, MinRate: 0.05, Factor: 2}

func (t GenFailThreshold) withDefaults() GenFailThreshold {
	d := DefaultGenFailThreshold
	if t.Window <= 0 {
		t.Window = d.Window
	}
	if t.Baseline <= 0 {
		t.Baseline = d.Baseline
	}
	if t.MinCount <= 0 {
		t.MinCount = d.MinCount
	}
	if t.MinRate <= 0 {
		t.MinRate = d.MinRate
	}
	if t.Factor < 1 {
		t.Factor = d.Factor
	}
	return t
}

// GenFailSpike is one code that jumped. Counts and codes only.
type GenFailSpike struct {
	Code      string
	Count     int // window lines with the code
	Lines     int // window ai.generate lines
	BaseCount int
	BaseLines int
	Start     time.Time // window start
	End       time.Time // window end
}

// Rate is Count / Lines.
func (s GenFailSpike) Rate() float64 { return ratio(s.Count, s.Lines) }

// BaseRate is BaseCount / BaseLines (0 without a baseline).
func (s GenFailSpike) BaseRate() float64 { return ratio(s.BaseCount, s.BaseLines) }

func ratio(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}

// DetectGenFailSpikes compares (end-Window, end] with the Baseline before it.
// Spikes come sorted by count (desc), then code.
func DetectGenFailSpikes(evs []GenEvent, end time.Time, th GenFailThreshold) []GenFailSpike {
	th = th.withDefaults()
	end = end.UTC()
	start := end.Add(-th.Window)
	baseStart := start.Add(-th.Baseline)
	var lines, baseLines int
	win, base := map[string]int{}, map[string]int{}
	for _, e := range evs {
		switch {
		case e.At.After(start) && !e.At.After(end):
			lines++
			for _, c := range e.FailureCodes() {
				win[c]++
			}
		case e.At.After(baseStart) && !e.At.After(start):
			baseLines++
			for _, c := range e.FailureCodes() {
				base[c]++
			}
		}
	}
	out := []GenFailSpike{}
	for code, n := range win {
		s := GenFailSpike{Code: code, Count: n, Lines: lines, BaseCount: base[code], BaseLines: baseLines, Start: start, End: end}
		if n < th.MinCount || s.Rate() < th.MinRate {
			continue
		}
		if s.BaseRate() > 0 && s.Rate() < th.Factor*s.BaseRate() {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Code < out[j].Code
	})
	return out
}

// GenFailures adapts the log reader and the detector to Fetcher. The window
// ends at Now; run it after exporting the logs of the last Window+Baseline.
type GenFailures struct {
	Logs      GenLogSource
	Threshold GenFailThreshold
	Now       func() time.Time
}

// Source implements Fetcher.
func (GenFailures) Source() loop.Source { return loop.SourceGenFailure }

// Fetch implements Fetcher: the spikes of the window ending now (none when
// that window ends before since).
func (g GenFailures) Fetch(ctx context.Context, since time.Time) ([]Raw, error) {
	th := g.Threshold.withDefaults()
	now := time.Now
	if g.Now != nil {
		now = g.Now
	}
	end := now().UTC()
	if end.Before(since) {
		return []Raw{}, nil
	}
	evs, err := g.Logs.GenerateEvents(ctx, end.Add(-th.Window-th.Baseline), end)
	if err != nil {
		return nil, err
	}
	spikes := DetectGenFailSpikes(evs, end, th)
	out := make([]Raw, 0, len(spikes))
	for _, s := range spikes {
		out = append(out, GenFailRaw(s))
	}
	return out, nil
}

// GenFailRaw is the inbox form of one spike: a fixed template over the code
// and counts. One item per code and day (the window's end, UTC).
func GenFailRaw(s GenFailSpike) Raw {
	day := s.End.UTC().Format("2006-01-02")
	base := "基準の期間にログ無し"
	if s.BaseLines > 0 {
		base = fmt.Sprintf("基準 %d / %d 件(%.1f%%)", s.BaseCount, s.BaseLines, 100*s.BaseRate())
	}
	return Raw{
		Source: loop.SourceGenFailure, SourceID: s.Code + "@" + day,
		Text: fmt.Sprintf("生成の失敗の急増: %s が %d / %d 件(%.1f%%)・%s・直近 %s",
			s.Code, s.Count, s.Lines, 100*s.Rate(), base, windowLabel(s.End.Sub(s.Start))),
		Kind: "gen_failure", Count: s.Count, CreatedAt: s.End,
	}
}

// windowLabel writes 24h as "24 時間" and anything else as a Go duration.
func windowLabel(d time.Duration) string {
	if d > 0 && d%time.Hour == 0 {
		return fmt.Sprintf("%d 時間", d/time.Hour)
	}
	return d.String()
}

// GenFailConfigFromEnv reads LOOP_SOURCE_GENFAIL and the LOOP_GENFAIL_*
// settings. On without logs is an error (not a silent skip).
func GenFailConfigFromEnv(getenv func(string) string) (GenFailures, error) {
	if getenv(EnvGenFailOn) != "on" {
		return GenFailures{}, ErrGenFailOff
	}
	paths := []string{}
	for p := range ParseList(getenv(EnvGenFailLogs)) {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return GenFailures{}, fmt.Errorf("%s=on needs %s (gcloud logging read exports)", EnvGenFailOn, EnvGenFailLogs)
	}
	th := DefaultGenFailThreshold
	var errs []error
	dur := func(k string, dst *time.Duration) {
		if v := getenv(k); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil || d <= 0 {
				errs = append(errs, fmt.Errorf("%s: want a positive duration, got %q", k, v))
				return
			}
			*dst = d
		}
	}
	dur(EnvGenFailWindow, &th.Window)
	dur(EnvGenFailBaseline, &th.Baseline)
	if v := getenv(EnvGenFailMinCount); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			errs = append(errs, fmt.Errorf("%s: want a positive integer, got %q", EnvGenFailMinCount, v))
		}
		th.MinCount = n
	}
	if v := getenv(EnvGenFailMinRate); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f <= 0 || f > 1 {
			errs = append(errs, fmt.Errorf("%s: want a rate in (0, 1], got %q", EnvGenFailMinRate, v))
		}
		th.MinRate = f
	}
	if v := getenv(EnvGenFailFactor); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 1 {
			errs = append(errs, fmt.Errorf("%s: want a factor >= 1, got %q", EnvGenFailFactor, v))
		}
		th.Factor = f
	}
	if err := errors.Join(errs...); err != nil {
		return GenFailures{}, err
	}
	return GenFailures{Logs: GenLogFiles{Paths: paths}, Threshold: th}, nil
}
