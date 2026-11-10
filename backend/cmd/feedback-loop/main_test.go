package main

import (
	"context"
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
	buf *strings.Builder
	env env
}

func newTestApp(t *testing.T, e env) *testApp {
	t.Helper()
	fs, ls, buf := store.NewMemory(), loop.NewMemory(), &strings.Builder{}
	now := time.Date(2026, 11, 10, 3, 0, 0, 0, time.UTC)
	a := &app{loop: ls, feedback: fs, users: fs, getenv: e.get, now: func() time.Time { return now }, out: buf}
	return &testApp{app: a, fs: fs, ls: ls, buf: buf, env: e}
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
	if !strings.Contains(out, "ingest in_app: fetched 1, new 1") || !strings.Contains(out, "ingest app_store_review: fetched 1, new 1 (held from AI 1)") {
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
