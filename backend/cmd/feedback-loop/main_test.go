package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/ingest"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

type env map[string]string

func (e env) get(k string) string { return e[k] }

type fakeFetcher struct {
	src  loop.Source
	raws []ingest.Raw
}

func (f fakeFetcher) Source() loop.Source { return f.src }
func (f fakeFetcher) Fetch(context.Context, time.Time) ([]ingest.Raw, error) {
	return f.raws, nil
}

type testApp struct {
	*app
	fs  *store.Memory
	ls  *loop.Memory
	ctl *loop.MemoryControl
	buf *strings.Builder
	env env
}

// newTestApp turns every stage on unless e says otherwise.
func newTestApp(t *testing.T, e env) *testApp {
	t.Helper()
	for _, s := range loop.Stages {
		if _, ok := e[loop.EnvName(s)]; !ok {
			e[loop.EnvName(s)] = "on"
		}
	}
	fs, ls, ctl, buf := store.NewMemory(), loop.NewMemory(), loop.NewMemoryControl(), &strings.Builder{}
	now := time.Date(2026, 11, 10, 3, 0, 0, 0, time.UTC)
	a := &app{loop: ls, control: ctl, feedback: fs, users: fs, getenv: e.get, now: func() time.Time { return now }, out: buf,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	return &testApp{app: a, fs: fs, ls: ls, ctl: ctl, buf: buf, env: e}
}

func (ta *testApp) must(t *testing.T, args ...string) string {
	t.Helper()
	ta.buf.Reset()
	if err := ta.run(context.Background(), args); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, ta.buf.String())
	}
	return ta.buf.String()
}

func TestIngestCommands(t *testing.T) {
	ta := newTestApp(t, env{"LOOP_CONSENT_VERSIONS": "feedback-2027-01", ingest.EnvAppStoreOn: "on"})
	ta.appStore = fakeFetcher{src: loop.SourceStoreReview, raws: []ingest.Raw{{Source: loop.SourceStoreReview, SourceID: "r1", Text: "落ちる", CreatedAt: time.Now()}}}
	if _, err := ta.fs.CreateFeedback(context.Background(), store.Feedback{UID: "u1", Kind: "bug", Message: "写真で落ちる", ConsentVersion: "feedback-2027-01"}, 0); err != nil {
		t.Fatal(err)
	}
	out := ta.must(t, "ingest")
	if !strings.Contains(out, "ingest in_app: fetched 1, new 1") || !strings.Contains(out, "ingest app_store_review: fetched 1, new 1 (held from AI 0)") { // reviews go to the AI by default (2026-11-10)
		t.Fatalf("out:\n%s", out)
	}
	out = ta.must(t, "ingest-crash", "-issue", "abc", "-type", "SIGABRT", "-count", "12", "-version", "1.4.2")
	if !strings.Contains(out, "created=true") {
		t.Fatalf("crash: %s", out)
	}
	items, _ := ta.ls.ListItems(context.Background(), loop.ItemQuery{})
	if len(items) != 3 {
		t.Fatalf("items %d", len(items))
	}
	if err := ta.run(context.Background(), []string{"ingest-crash", "-issue", "x"}); err == nil {
		t.Fatal("incomplete crash accepted")
	}
	if err := ta.run(context.Background(), []string{"nope"}); err == nil {
		t.Fatal("unknown command accepted")
	}
}

func TestIngestAppStoreOffByDefault(t *testing.T) {
	ta := newTestApp(t, env{})
	out := ta.must(t, "ingest")
	if strings.Contains(out, "app_store_review") {
		t.Fatalf("store reviews ran while off: %s", out)
	}
	if err := ta.run(context.Background(), []string{"ingest", "-source", "app_store_review"}); err == nil {
		t.Fatal("explicit source while off should fail")
	}
}

func TestIngestTestFlight(t *testing.T) {
	ta := newTestApp(t, env{ingest.EnvAppStoreOn: "on"})
	ta.appStore = fakeFetcher{src: loop.SourceStoreReview}
	ta.testFlight = fakeFetcher{src: loop.SourceTestFlight, raws: []ingest.Raw{{Source: loop.SourceTestFlight, SourceID: "s:1", Text: "保存できない", CreatedAt: time.Now()}}}
	if out := ta.must(t, "ingest"); strings.Contains(out, "testflight") { // off by default, even with the App Store on
		t.Fatalf("testflight ran while off: %s", out)
	}
	if err := ta.run(context.Background(), []string{"ingest", "-source", "testflight"}); err == nil {
		t.Fatal("explicit source while off should fail")
	}
	ta.env[ingest.EnvTestFlightOn] = "on"
	out := ta.must(t, "ingest", "-source", "testflight")
	if !strings.Contains(out, "ingest testflight: fetched 1, new 1 (held from AI 1)") || strings.Contains(out, "in_app") { // not in LOOP_AI_SOURCES by default
		t.Fatalf("out:\n%s", out)
	}
	if out := ta.must(t, "ingest"); !strings.Contains(out, "ingest testflight:") || !strings.Contains(out, "ingest app_store_review:") {
		t.Fatalf("all sources: %s", out)
	}
	// on without a key fails loudly instead of skipping
	ta.testFlight = nil
	if err := ta.run(context.Background(), []string{"ingest", "-source", "testflight"}); err == nil || !strings.Contains(err.Error(), ingest.EnvTestFlightOn) {
		t.Fatalf("missing key: %v", err)
	}
}

func TestIngestGooglePlay(t *testing.T) {
	ta := newTestApp(t, env{})
	ta.googlePlay = fakeFetcher{src: ingest.SourceGooglePlay, raws: []ingest.Raw{{Source: ingest.SourceGooglePlay, SourceID: "gp:1", Text: "保存できない", CreatedAt: time.Now()}}}
	if out := ta.must(t, "ingest"); strings.Contains(out, "google_play_review") {
		t.Fatalf("google play ran while off: %s", out)
	}
	if err := ta.run(context.Background(), []string{"ingest", "-source", "google_play_review"}); !errors.Is(err, ingest.ErrGooglePlayOff) {
		t.Fatalf("explicit source while off: %v", err)
	}
	ta.env[ingest.EnvGooglePlayOn] = "on"
	if out := ta.must(t, "ingest", "-source", "google_play_review"); !strings.Contains(out, "ingest google_play_review: fetched 1, new 1 (held from AI 1)") {
		t.Fatalf("out:\n%s", out)
	}
	ta.googlePlay = nil
	if err := ta.run(context.Background(), []string{"ingest", "-source", "google_play_review"}); err == nil || !strings.Contains(err.Error(), ingest.EnvGooglePlayPackage) {
		t.Fatalf("missing package: %v", err)
	}
}

func TestIngestGenFailure(t *testing.T) {
	ta := newTestApp(t, env{})
	// hand-written export: 20 ai.generate lines in the last day, 6 failed
	var b strings.Builder
	for i := 0; i < 20; i++ {
		outcome := `"outcome":"validated"`
		if i < 6 {
			outcome = `"outcome":"failed","error_code":"generation_failed"`
		}
		fmt.Fprintf(&b, `{"time":"%s","event":"ai.generate",%s,"uid_hash":"u_x"}`+"\n", ta.now().Add(-time.Duration(i+1)*time.Minute).Format(time.RFC3339), outcome)
	}
	path := filepath.Join(t.TempDir(), "gen.jsonl")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	ta.env[ingest.EnvGenFailLogs] = path
	if out := ta.must(t, "ingest"); strings.Contains(out, "gen_failure") { // off by default
		t.Fatalf("gen_failure ran while off: %s", out)
	}
	if err := ta.run(context.Background(), []string{"ingest", "-source", "gen_failure"}); !errors.Is(err, ingest.ErrGenFailOff) {
		t.Fatalf("explicit source while off: %v", err)
	}
	ta.env[ingest.EnvGenFailOn] = "on"
	out := ta.must(t, "ingest", "-source", "gen_failure")
	if !strings.Contains(out, "ingest gen_failure: fetched 2, new 2 (held from AI 2)") || strings.Contains(out, "in_app") { // not in LOOP_AI_SOURCES by default
		t.Fatalf("out:\n%s", out)
	}
	if out := ta.must(t, "ingest"); !strings.Contains(out, "ingest gen_failure: fetched 2, new 0") {
		t.Fatalf("second run: %s", out)
	}
	delete(ta.env, ingest.EnvGenFailLogs)
	if err := ta.run(context.Background(), []string{"ingest"}); err == nil || !strings.Contains(err.Error(), ingest.EnvGenFailLogs) {
		t.Fatalf("on without logs: %v", err)
	}
}

func TestPurgeCommand(t *testing.T) {
	ctx := context.Background()
	ta := newTestApp(t, env{"LOOP_PAUSED": "1"}) // purge runs even while the loop is stopped
	it := loop.Item{ID: "lin_old", Source: loop.SourceCrash, SourceID: "c", Status: loop.ItemNew, ExpiresAt: ta.now().Add(-time.Hour)}
	if _, err := ta.ls.PutItem(ctx, it); err != nil {
		t.Fatal(err)
	}
	out := ta.must(t, "purge", "-dry-run")
	if !strings.Contains(out, "expired 1") {
		t.Fatalf("%s", out)
	}
	ta.must(t, "purge")
	if _, err := ta.ls.GetItem(ctx, "lin_old"); err == nil {
		t.Fatal("expired item kept")
	}
}
