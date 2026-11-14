package ingest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
)

// Hand-written ai.generate lines in the shapes the backend writes (a
// `gcloud logging read --format=json` array and bare slog JSON lines). Not
// real data. uid_hash, app_hash and a stray free-text field are present to
// prove they never reach an item.
const genLogArray = `[
 {"timestamp": "2026-11-10T01:00:00Z", "jsonPayload": {"event": "ai.generate", "mode": "full", "outcome": "regenerated",
   "retry_reasons": "validation,validation", "validation_codes": "unknown_component:2,missing_field:1",
   "uid_hash": "u_8c1f", "app_hash": "a_77d0", "message": "ai.generate"}},
 {"timestamp": "2026-11-10T02:00:00Z", "jsonPayload": {"event": "ai.generate", "mode": "edit", "outcome": "failed",
   "error_code": "generation_failed", "retry_reasons": "truncated", "uid_hash": "u_8c1f"}},
 {"timestamp": "2026-11-10T02:30:00Z", "jsonPayload": {"event": "http.request", "path": "/v1/generate", "status": 500}},
 {"timestamp": "2026-11-10T02:40:00Z", "jsonPayload": {"event": "ai.generate", "outcome": "failed",
   "error_code": "山田太郎 090-1234-5678", "retry_reasons": "Hello World,share", "validation_codes": "bad code:3,ok_code:x"}}
]`

const genLogLines = `{"time":"2026-11-10T00:10:00.5Z","level":"INFO","msg":"ai.generate","event":"ai.generate","mode":"full","outcome":"validated","uid_hash":"u_1"}
not json
{"time":"2026-11-10T00:20:00Z","event":"ai.chat","outcome":"replied"}
`

func TestParseGenLogs(t *testing.T) {
	evs, err := ParseGenLogs(strings.NewReader(genLogArray))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 3 {
		t.Fatalf("want 3 ai.generate lines, got %d: %+v", len(evs), evs)
	}
	if got := strings.Join(evs[0].FailureCodes(), " "); got != "retry:validation validation:missing_field validation:unknown_component" {
		t.Fatalf("codes[0] = %s", got)
	}
	if got := strings.Join(evs[1].FailureCodes(), " "); got != "error:generation_failed outcome:failed retry:truncated" {
		t.Fatalf("codes[1] = %s", got)
	}
	// free text in a label field is dropped, not turned into a code
	if got := strings.Join(evs[2].FailureCodes(), " "); got != "outcome:failed retry:share" {
		t.Fatalf("codes[2] = %s", got)
	}
	lines, err := ParseGenLogs(strings.NewReader(genLogLines))
	if err != nil || len(lines) != 1 || lines[0].Outcome != "validated" || len(lines[0].FailureCodes()) != 0 {
		t.Fatalf("json lines: %+v %v", lines, err)
	}
	if _, err := ParseGenLogs(strings.NewReader("[not json")); err == nil {
		t.Fatal("broken array accepted")
	}
}

var genEnd = time.Date(2026, 11, 10, 3, 0, 0, 0, time.UTC)

// genLines builds n lines at at, failing ones carrying code (a retry reason).
func genLines(at time.Time, n, failing int, reason string) []GenEvent {
	out := make([]GenEvent, 0, n)
	for i := 0; i < n; i++ {
		e := GenEvent{At: at, Outcome: "validated"}
		if i < failing {
			e.Outcome, e.RetryReasons = "regenerated", []string{reason, reason}
		}
		out = append(out, e)
	}
	return out
}

func codes(sp []GenFailSpike) string {
	parts := make([]string, len(sp))
	for i, s := range sp {
		parts[i] = fmt.Sprintf("%s=%d/%d", s.Code, s.Count, s.Lines)
	}
	return strings.Join(parts, " ")
}

func TestDetectGenFailSpikes(t *testing.T) {
	th := GenFailThreshold{Window: 24 * time.Hour, Baseline: 7 * 24 * time.Hour, MinCount: 5, MinRate: 0.1, Factor: 2}
	inWin := genEnd.Add(-time.Hour)
	inBase := genEnd.Add(-48 * time.Hour)
	cases := []struct {
		name string
		evs  []GenEvent
		want string
	}{
		{"spike vs low baseline", append(genLines(inWin, 50, 10, "validation"), genLines(inBase, 100, 5, "validation")...), "retry:validation=10/50"},
		{"no spike: same rate as baseline", append(genLines(inWin, 50, 10, "validation"), genLines(inBase, 100, 20, "validation")...), ""},
		{"no baseline: a new code spikes on count and rate", genLines(inWin, 50, 10, "lint"), "retry:lint=10/50"},
		{"edge: exactly MinCount and MinRate", genLines(inWin, 50, 5, "lint"), "retry:lint=5/50"},
		{"edge: one below MinCount", genLines(inWin, 40, 4, "lint"), ""},
		{"edge: rate just below MinRate", genLines(inWin, 51, 5, "lint"), ""},
		{"edge: exactly Factor x baseline", append(genLines(inWin, 50, 10, "validation"), genLines(inBase, 100, 10, "validation")...), "retry:validation=10/50"},
		{"edge: just under Factor x baseline", append(genLines(inWin, 50, 10, "validation"), genLines(inBase, 100, 11, "validation")...), ""},
		{"no logs", nil, ""},
		{"lines outside both windows are ignored", append(genLines(genEnd.Add(time.Minute), 50, 50, "lint"), genLines(genEnd.Add(-9*24*time.Hour), 50, 50, "lint")...), ""},
		{"window end is inclusive, start exclusive", append(genLines(genEnd, 10, 10, "lint"), genLines(genEnd.Add(-24*time.Hour), 10, 10, "share")...), "retry:lint=10/10"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := codes(DetectGenFailSpikes(c.evs, genEnd, th)); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestDetectGenFailSpikesOrderAndCodes(t *testing.T) {
	evs := []GenEvent{}
	for i := 0; i < 20; i++ {
		e := GenEvent{At: genEnd.Add(-time.Hour), Outcome: "validated"}
		if i < 8 {
			e = GenEvent{At: e.At, Outcome: "failed", ErrorCode: "generation_failed"}
		}
		if i >= 8 && i < 14 {
			e.Outcome, e.RetryReasons, e.ValidationCodes = "regenerated", []string{"validation"}, map[string]int{"unknown_component": 4}
		}
		evs = append(evs, e)
	}
	got := codes(DetectGenFailSpikes(evs, genEnd, GenFailThreshold{}))
	want := "error:generation_failed=8/20 outcome:failed=8/20 retry:validation=6/20 validation:unknown_component=6/20"
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
}

func TestGenFailuresFetchIsPIIFree(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	// window: 30 lines, 10 fail with retry:validation; one carries a
	// free-text error_code that must not survive
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < 30; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		p := `"outcome": "validated"`
		if i < 10 {
			p = `"outcome": "regenerated", "retry_reasons": "validation", "validation_codes": "unknown_component:1"`
		}
		if i == 0 {
			p += `, "error_code": "taro@example.com"`
		}
		fmt.Fprintf(&b, `{"timestamp": "%s", "jsonPayload": {"event": "ai.generate", %s, "uid_hash": "u_secret_hash", "app_hash": "a_secret_hash", "app_id": "app_SECRETID01"}}`,
			genEnd.Add(-time.Duration(i+1)*time.Minute).Format(time.RFC3339), p)
	}
	b.WriteString("]")
	path := filepath.Join(dir, "logs.json")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	g, err := GenFailConfigFromEnv(env{EnvGenFailOn: "on", EnvGenFailLogs: path + ", "}.get)
	if err != nil {
		t.Fatal(err)
	}
	g.Now = func() time.Time { return genEnd }
	if g.Source() != loop.SourceGenFailure {
		t.Fatal(g.Source())
	}
	raws, err := g.Fetch(ctx, genEnd.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(raws) != 2 {
		t.Fatalf("want retry:validation and validation:unknown_component, got %+v", raws)
	}
	r := raws[0]
	if r.SourceID != "retry:validation@2026-11-10" || r.Count != 10 || r.Kind != "gen_failure" || !r.CreatedAt.Equal(genEnd) || r.UID != "" {
		t.Fatalf("raw: %+v", r)
	}
	wantText := "生成の失敗の急増: retry:validation が 10 / 30 件(33.3%)・基準の期間にログ無し・直近 24 時間"
	if r.Text != wantText {
		t.Fatalf("text:\n%s\nwant\n%s", r.Text, wantText)
	}
	p := Policy{AISources: map[loop.Source]bool{loop.SourceGenFailure: true}}
	for _, raw := range raws {
		it := Normalize(raw, p, genEnd)
		if it.Text != raw.Text || len(it.MaskCounts) != 0 || it.Status != loop.ItemNew || it.UID != "" || it.AppVersion != "" || it.HasDSL {
			t.Fatalf("item: %+v", it)
		}
		for _, bad := range []string{"u_secret", "a_secret", "app_SECRET", "taro", "example.com"} {
			if strings.Contains(fmt.Sprintf("%+v", it), bad) {
				t.Fatalf("item leaks %q: %+v", bad, it)
			}
		}
	}
	// the window ended before since (an old cursor ahead of the clock): nothing
	if raws, err := g.Fetch(ctx, genEnd.Add(time.Hour)); err != nil || len(raws) != 0 {
		t.Fatalf("future since: %v %v", raws, err)
	}
	// ingest twice the same day: the second run stores nothing new
	ls := loop.NewMemory()
	run := &Runner{Store: ls, Policy: p, Now: g.Now}
	if st, err := run.Run(ctx, g); err != nil || st.Created != 2 {
		t.Fatalf("first run: %+v %v", st, err)
	}
	if st, err := run.Run(ctx, g); err != nil || st.Created != 0 || st.Skipped != 2 {
		t.Fatalf("second run: %+v %v", st, err)
	}
}

type env map[string]string

func (e env) get(k string) string { return e[k] }

func TestGenFailConfigFromEnv(t *testing.T) {
	if _, err := GenFailConfigFromEnv(env{}.get); !errors.Is(err, ErrGenFailOff) {
		t.Fatalf("off by default: %v", err)
	}
	if _, err := GenFailConfigFromEnv(env{EnvGenFailOn: "on"}.get); err == nil || !strings.Contains(err.Error(), EnvGenFailLogs) {
		t.Fatalf("on without logs: %v", err)
	}
	g, err := GenFailConfigFromEnv(env{EnvGenFailOn: "on", EnvGenFailLogs: "b.json,a.json"}.get)
	if err != nil || g.Threshold != DefaultGenFailThreshold || strings.Join(g.Logs.(GenLogFiles).Paths, ",") != "a.json,b.json" {
		t.Fatalf("defaults: %+v %v", g, err)
	}
	g, err = GenFailConfigFromEnv(env{EnvGenFailOn: "on", EnvGenFailLogs: "a", EnvGenFailWindow: "6h", EnvGenFailBaseline: "48h",
		EnvGenFailMinCount: "3", EnvGenFailMinRate: "0.2", EnvGenFailFactor: "1.5"}.get)
	want := GenFailThreshold{Window: 6 * time.Hour, Baseline: 48 * time.Hour, MinCount: 3, MinRate: 0.2, Factor: 1.5}
	if err != nil || g.Threshold != want {
		t.Fatalf("custom: %+v %v", g.Threshold, err)
	}
	for k, v := range map[string]string{EnvGenFailWindow: "-1h", EnvGenFailMinCount: "0", EnvGenFailMinRate: "1.5", EnvGenFailFactor: "0.5"} {
		if _, err := GenFailConfigFromEnv(env{EnvGenFailOn: "on", EnvGenFailLogs: "a", k: v}.get); err == nil || !strings.Contains(err.Error(), k) {
			t.Fatalf("%s=%s accepted: %v", k, v, err)
		}
	}
	// a missing export fails loudly
	if _, err := (GenFailures{Logs: GenLogFiles{Paths: []string{filepath.Join(t.TempDir(), "none.json")}}, Now: func() time.Time { return genEnd }}).Fetch(context.Background(), time.Time{}); err == nil {
		t.Fatal("missing file accepted")
	}
}
