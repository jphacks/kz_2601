package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/ingest"
)

type failingFetcher struct{ src loop.Source }

func (f failingFetcher) Source() loop.Source { return f.src }
func (f failingFetcher) Fetch(context.Context, time.Time) ([]ingest.Raw, error) {
	return nil, errors.New("upstream 503")
}

func TestIngestSourceAll(t *testing.T) {
	ctx := context.Background()
	// gen_failure on without LOOP_GENFAIL_LOGS: a config failure
	ta := newTestApp(t, env{ingest.EnvAppStoreOn: "on", ingest.EnvTestFlightOn: "on", ingest.EnvGenFailOn: "on"})
	ta.appStore = failingFetcher{src: loop.SourceStoreReview}
	ta.testFlight = fakeFetcher{src: loop.SourceTestFlight, raws: []ingest.Raw{{Source: loop.SourceTestFlight, SourceID: "s:1", Text: "保存できない", CreatedAt: time.Now()}}}
	ta.buf.Reset()
	err := ta.run(ctx, []string{"ingest", "-source", "all"})
	if err == nil || !strings.Contains(err.Error(), "2 source(s) failed (app_store_review, gen_failure)") {
		t.Fatalf("err: %v", err)
	}
	out := ta.buf.String()
	for _, want := range []string{"ingest in_app: fetched 0", "ingest app_store_review: failed: fetch app_store_review: upstream 503", "ingest testflight: fetched 1, new 1", "ingest gen_failure: failed:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	evs, _ := ta.ctl.ListAudit(ctx, loop.AuditQuery{})
	got := map[string]string{}
	for _, e := range evs {
		if e.Action == "source_done" || e.Action == "source_failed" {
			got[strings.Fields(e.Reason)[0]] = e.Action + " " + e.Reason
		}
	}
	want := map[string]string{
		"in_app":           "source_done in_app fetched=0 new=0 held=0 skipped=0",
		"testflight":       "source_done testflight fetched=1 new=1 held=1 skipped=0",
		"app_store_review": "source_failed app_store_review",
		"gen_failure":      "source_failed gen_failure",
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("audit %s: got %q want %q (all: %v)", k, got[k], v, got)
		}
	}

	// sources that are off are skipped; nothing failing = success
	ta2 := newTestApp(t, env{})
	if out := ta2.must(t, "ingest", "-source", "all"); !strings.Contains(out, "ingest in_app:") || strings.Contains(out, "testflight") {
		t.Fatalf("off sources: %s", out)
	}
	// the default (no -source) still stops at the first failure
	if err := ta.run(ctx, []string{"ingest"}); err == nil || strings.Contains(err.Error(), "source(s) failed") {
		t.Fatalf("default: %v", err)
	}
}
