package main

import (
	"context"
	"io"
	"log/slog"
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
