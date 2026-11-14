package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
)

func TestInboxListsEverySourceWithoutText(t *testing.T) {
	ta := newTestApp(t, env{})
	ctx := context.Background()
	at := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	put := func(src loop.Source, sid, status, hold string, i int) loop.Item {
		it := loop.Item{ID: loop.ItemID(src, sid), Source: src, SourceID: sid, Text: "秘密の本文 " + sid, Status: status, HoldReason: hold,
			AppVersion: "1.4.2", OSMajor: "26", CreatedAt: at, IngestedAt: at.Add(time.Duration(i) * time.Minute)}
		if _, err := ta.ls.PutItem(ctx, it); err != nil {
			t.Fatal(err)
		}
		return it
	}
	inApp := put(loop.SourceInApp, "fb1", loop.ItemNew, "", 0)
	review := put(loop.SourceStoreReview, "r1", loop.ItemHeld, loop.HoldConsent, 1)
	tf := put(loop.SourceTestFlight, "tf1", loop.ItemNew, "", 2)
	gen := put(loop.SourceGenFailure, "g1", loop.ItemTriaged, "", 3)

	out := ta.must(t, "inbox")
	for _, it := range []loop.Item{inApp, review, tf, gen} {
		if !strings.Contains(out, it.ID) || !strings.Contains(out, string(it.Source)) {
			t.Fatalf("missing %s:\n%s", it.Source, out)
		}
	}
	for _, want := range []string{"4 item(s)", loop.HoldConsent, "1.4.2", "2026-11-09T12:00:00Z"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "秘密") {
		t.Fatalf("text printed:\n%s", out)
	}
	if strings.Index(out, inApp.ID) > strings.Index(out, gen.ID) {
		t.Fatalf("not oldest ingested first:\n%s", out)
	}

	out = ta.must(t, "inbox", "-source", "testflight")
	if !strings.Contains(out, tf.ID) || strings.Contains(out, inApp.ID) || !strings.Contains(out, "1 item(s)") {
		t.Fatalf("source filter:\n%s", out)
	}
	out = ta.must(t, "inbox", "-status", "new", "-limit", "1")
	if !strings.Contains(out, inApp.ID) || strings.Contains(out, tf.ID) || !strings.Contains(out, "1 item(s)") {
		t.Fatalf("status+limit:\n%s", out)
	}
	out = ta.must(t, "inbox", "-source", "in_app", "-status", "held")
	if !strings.Contains(out, "0 item(s)") {
		t.Fatalf("source+status:\n%s", out)
	}

	for _, bad := range [][]string{
		{"inbox", "-source", "twitter"},
		{"inbox", "-status", "done"},
		{"inbox", "-limit", "0"},
		{"inbox", "extra"},
	} {
		if err := ta.run(ctx, bad); err == nil {
			t.Fatalf("%v accepted", bad)
		}
	}
}
