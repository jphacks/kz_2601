package loop

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestExpiresFrom(t *testing.T) {
	now := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	own := now.Add(Retention)
	day := 24 * time.Hour
	cases := []struct {
		name       string
		srcCreated time.Time
		srcRet     time.Duration
		want       time.Time
	}{
		// the source goes first: the copy must not outlive it (#367)
		{"source expires earlier", now.Add(-100 * day), 180 * day, now.Add(80 * day)},
		{"source already gone", now.Add(-200 * day), 180 * day, now.Add(-20 * day)},
		{"source outlives the copy", now, 180 * day, own},
		{"no source retention", now.Add(-365 * day), 0, own},
		{"negative source retention", now.Add(-365 * day), -time.Hour, own},
		{"no source time", time.Time{}, time.Hour, own},
	}
	for _, c := range cases {
		if got := ExpiresFrom(now, c.srcCreated, c.srcRet); !got.Equal(c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestIDsAreDeterministicAndSeparated(t *testing.T) {
	first, second := ItemID(SourceInApp, "fb_1"), ItemID(SourceInApp, "fb_1")
	if first != second {
		t.Fatalf("ItemID not deterministic: %q vs %q", first, second)
	}
	// the same source id from two sources is two items
	if ItemID(SourceInApp, "x") == ItemID(SourceTestFlight, "x") {
		t.Fatal("sources collide")
	}
	// the separator keeps ("ab","c") and ("a","bc") apart
	if ItemID("ab", "c") == ItemID("a", "bc") || NoticeID("ab", "c") == NoticeID("a", "bc") {
		t.Fatal("concatenation collides")
	}
	// the hashed id does not carry the feedback id (#367)
	src := "fb_" + strings.Repeat("a", 20)
	if id := ItemID(SourceInApp, src); len(id) != len("lin_")+24 || !strings.HasPrefix(id, "lin_") || strings.Contains(id, src) {
		t.Fatalf("item id %q", id)
	}
	if id := NoticeID("drf_1", "u1"); len(id) != len("ntc_")+24 || !strings.HasPrefix(id, "ntc_") {
		t.Fatalf("notice id %q", id)
	}
}

func TestMonthIsUTC(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	// 1 Dec 08:00 JST is still November in UTC
	if got := Month(time.Date(2026, 12, 1, 8, 0, 0, 0, jst)); got != "2026-11" {
		t.Fatalf("month %s", got)
	}
}

func TestFirestoreErrorCodes(t *testing.T) {
	nf := status.Error(codes.NotFound, "x")
	ae := status.Error(codes.AlreadyExists, "x")
	if !isNotFound(nf) || isNotFound(ae) || isNotFound(nil) || isNotFound(errors.New("not found")) {
		t.Fatal("isNotFound")
	}
	if !isAlreadyExists(ae) || isAlreadyExists(nf) || isAlreadyExists(nil) {
		t.Fatal("isAlreadyExists")
	}
	// a wrapped status error is still recognised
	if !isNotFound(fmt.Errorf("get: %w", nf)) {
		t.Fatal("wrapped NotFound")
	}
}

func TestLimitOr(t *testing.T) {
	for _, c := range []struct{ n, def, want int }{{0, 100, 100}, {-1, 50, 50}, {10, 100, 10}, {501, 100, 500}, {10_000, 200, 500}} {
		if got := limitOr(c.n, c.def); got != c.want {
			t.Errorf("limitOr(%d,%d)=%d want %d", c.n, c.def, got, c.want)
		}
	}
}
