package ratelimit

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// scriptedStore is Memory whose Consume / Refund can be made to fail, and
// which counts the calls.
type scriptedStore struct {
	*Memory
	consumeErr, refundErr error
	consumes, refunds     int
}

func (s *scriptedStore) Consume(ctx context.Context, cs []Counter, w int, exp time.Time) (Usage, error) {
	s.consumes++
	if s.consumeErr != nil {
		return Usage{}, s.consumeErr
	}
	return s.Memory.Consume(ctx, cs, w, exp)
}

func (s *scriptedStore) Refund(ctx context.Context, cs []Counter, w int, exp time.Time) error {
	s.refunds++
	if s.refundErr != nil {
		return s.refundErr
	}
	return s.Memory.Refund(ctx, cs, w, exp)
}

var errStore = errors.New("store down")

// capLimiter is a limiter at 2026-10-25 22:10 JST whose log goes to the returned buffer.
func capLimiter(lim Limits, env string) (*Limiter, *scriptedStore, *bytes.Buffer) {
	c := &clock{t: time.Date(2026, 10, 25, 22, 10, 0, 0, JST)}
	s := &scriptedStore{Memory: NewMemoryClock(c.now)}
	l := New(Config{Limits: lim, Weights: DefaultWeights, Env: env, Now: c.now}, s, nil)
	var buf bytes.Buffer
	l.log = slog.New(slog.NewTextHandler(&buf, nil))
	return l, s, &buf
}

func peekOne(t *testing.T, m *Memory, id string) int {
	t.Helper()
	got, err := m.Peek(context.Background(), []Counter{{ID: id}})
	if err != nil {
		t.Fatal(err)
	}
	return got[0]
}

func TestTakeUnchangedSkips(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		limit int
		uid   string
		store bool
	}{
		{"limit off", 0, "u1", true},
		{"negative limit", -1, "u1", true},
		{"no uid", 5, "", true},
		{"no store", 5, "u1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l, s, _ := capLimiter(Limits{UnchangedDaily: tc.limit}, "test")
			var store Store
			if tc.store {
				store = s
			}
			if tk, ok := l.takeUnchanged(ctx, store, tc.uid); ok || tk.counter != nil {
				t.Fatalf("took %+v, %v", tk, ok)
			}
			if s.consumes != 0 {
				t.Fatalf("consumes = %d, want 0", s.consumes)
			}
		})
	}
}

func TestTakeUnchangedCountsAndCaps(t *testing.T) {
	ctx := context.Background()
	l, s, buf := capLimiter(Limits{UnchangedDaily: 2}, "test")
	uid := "a/" + strings.Repeat("x", 40)
	id := "test_2026-10-25_unchanged_a_" + strings.Repeat("x", 40) // "/" is not doc-safe
	for i := range 2 {
		tk, ok := l.takeUnchanged(ctx, s, uid)
		if !ok || tk.counter == nil || tk.counter.ID != id || tk.counter.Scope != ScopeUnchanged || tk.counter.Limit != 2 {
			t.Fatalf("#%d took %+v, %v", i, tk.counter, ok)
		}
		if want := time.Date(2026, 10, 27, 0, 0, 0, 0, JST); !tk.expireAt.Equal(want) {
			t.Fatalf("expireAt %v, want %v", tk.expireAt, want)
		}
	}
	if tk, ok := l.takeUnchanged(ctx, s, uid); ok || tk.counter != nil {
		t.Fatalf("past the cap took %+v, %v", tk, ok)
	}
	if n := peekOne(t, s.Memory, id); n != 2 {
		t.Fatalf("count = %d, want 2", n)
	}
	if !strings.Contains(buf.String(), "ratelimit.unchanged_capped") {
		t.Fatalf("log %q", buf.String())
	}
}

func TestTakeUnchangedLocalPrefix(t *testing.T) {
	l, s, _ := capLimiter(Limits{UnchangedDaily: 1}, "")
	tk, ok := l.takeUnchanged(context.Background(), s, "u1")
	if !ok || tk.counter.ID != "local_2026-10-25_unchanged_u1" {
		t.Fatalf("took %+v, %v", tk.counter, ok)
	}
}

func TestTakeUnchangedStoreFailure(t *testing.T) {
	l, s, buf := capLimiter(Limits{UnchangedDaily: 5}, "test")
	s.consumeErr = errStore
	if tk, ok := l.takeUnchanged(context.Background(), s, "u1"); ok || tk.counter != nil {
		t.Fatalf("took %+v, %v", tk, ok)
	}
	if !strings.Contains(buf.String(), "unchanged count failed") {
		t.Fatalf("log %q", buf.String())
	}
}

func TestUntakeUnchanged(t *testing.T) {
	ctx := context.Background()
	l, s, buf := capLimiter(Limits{UnchangedDaily: 5}, "test")
	tk, ok := l.takeUnchanged(ctx, s, "u1")
	if !ok {
		t.Fatal("not taken")
	}
	l.untakeUnchanged(ctx, s, tk)
	if n := peekOne(t, s.Memory, tk.counter.ID); n != 0 {
		t.Fatalf("count after give-back = %d, want 0", n)
	}

	// nothing taken / no store: no Refund call
	s.refunds = 0
	l.untakeUnchanged(ctx, s, unchangedTake{})
	l.untakeUnchanged(ctx, nil, tk)
	if s.refunds != 0 {
		t.Fatalf("refunds = %d, want 0", s.refunds)
	}

	// a failed give-back is logged, not panicked on
	s.refundErr = errStore
	l.untakeUnchanged(ctx, s, tk)
	if s.refunds != 1 || !strings.Contains(buf.String(), "unchanged count give-back failed") {
		t.Fatalf("refunds %d log %q", s.refunds, buf.String())
	}
}

func TestTakeWaiveSkips(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		hourly int
		uid    string
	}{
		{"limit off", 0, "u1"},
		{"no uid", 5, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l, s, _ := capLimiter(Limits{WaivedHourly: tc.hourly}, "test")
			tk, ok := l.takeWaive(ctx, s, tc.uid, "")
			if !ok || len(tk.counters) != 0 {
				t.Fatalf("took %+v, %v (want a free pass)", tk, ok)
			}
			if s.consumes != 0 {
				t.Fatalf("consumes = %d, want 0", s.consumes)
			}
		})
	}
}

func TestTakeWaiveCountsAndCaps(t *testing.T) {
	ctx := context.Background()
	l, s, buf := capLimiter(Limits{WaivedHourly: 2}, "")
	uid := strings.Repeat("w", 32)
	id := "local_2026-10-25T22_waived_" + uid
	for i := range 2 {
		tk, ok := l.takeWaive(ctx, s, uid, "")
		if !ok || len(tk.counters) != 1 || tk.counters[0].ID != id || tk.counters[0].Scope != ScopeWaived {
			t.Fatalf("#%d took %+v, %v", i, tk, ok)
		}
		if want := time.Date(2026, 10, 26, 23, 0, 0, 0, JST); !tk.expireAt.Equal(want) {
			t.Fatalf("expireAt %v, want %v", tk.expireAt, want)
		}
	}
	if tk, ok := l.takeWaive(ctx, s, uid, ""); ok || len(tk.counters) != 0 {
		t.Fatalf("past the cap took %+v, %v", tk, ok)
	}
	if n := peekOne(t, s.Memory, id); n != 2 {
		t.Fatalf("count = %d, want 2", n)
	}
	if !strings.Contains(buf.String(), "ratelimit.waive_capped") || !strings.Contains(buf.String(), "ip=false") {
		t.Fatalf("log %q", buf.String())
	}
}

// A store failure lets the waive through without anything to give back.
func TestTakeWaiveStoreFailure(t *testing.T) {
	l, s, buf := capLimiter(Limits{WaivedHourly: 5}, "test")
	s.consumeErr = errStore
	tk, ok := l.takeWaive(context.Background(), s, "u1", "")
	if !ok || len(tk.counters) != 0 {
		t.Fatalf("took %+v, %v", tk, ok)
	}
	if !strings.Contains(buf.String(), "waive count failed") {
		t.Fatalf("log %q", buf.String())
	}
}

func TestUntakeWaive(t *testing.T) {
	ctx := context.Background()
	l, s, buf := capLimiter(Limits{WaivedHourly: 5}, "test")
	tk, ok := l.takeWaive(ctx, s, "u1", "")
	if !ok || len(tk.counters) != 1 {
		t.Fatalf("took %+v, %v", tk, ok)
	}
	l.untakeWaive(ctx, s, tk)
	if n := peekOne(t, s.Memory, tk.counters[0].ID); n != 0 {
		t.Fatalf("count after give-back = %d, want 0", n)
	}

	s.refunds = 0
	l.untakeWaive(ctx, s, waiveTake{})
	if s.refunds != 0 {
		t.Fatalf("refunds = %d, want 0", s.refunds)
	}

	s.refundErr = errStore
	l.untakeWaive(ctx, s, tk)
	if s.refunds != 1 || !strings.Contains(buf.String(), "waive count give-back failed") {
		t.Fatalf("refunds %d log %q", s.refunds, buf.String())
	}
}

// Config returns what New filled in (defaults for Location / Now).
func TestLimiterConfig(t *testing.T) {
	in := Config{Limits: Limits{SignedIn: 7}, Weights: DefaultWeights, Env: "test"}
	got := New(in, nil, nil).Config()
	if got.Limits != in.Limits || got.Weights != in.Weights || got.Env != "test" {
		t.Fatalf("config %+v", got)
	}
	if got.Location != JST || got.Now == nil {
		t.Fatalf("defaults not filled: location %v now nil %v", got.Location, got.Now == nil)
	}
	loc := time.FixedZone("X", 3600)
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, loc)
	got = New(Config{Location: loc, Now: func() time.Time { return fixed }}, nil, nil).Config()
	if got.Location != loc || !got.Now().Equal(fixed) {
		t.Fatalf("explicit location/now not kept: %v %v", got.Location, got.Now())
	}
}

// NewMemoryClock expires entries by the given clock; nil keeps time.Now.
func TestNewMemoryClock(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, JST)
	m := NewMemoryClock(func() time.Time { return now })
	c := []Counter{{ID: "k", Limit: 10}}
	if _, err := m.Consume(ctx, c, 3, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n := peekOne(t, m, "k"); n != 3 {
		t.Fatalf("count = %d, want 3", n)
	}
	now = now.Add(2 * time.Hour)
	if n := peekOne(t, m, "k"); n != 0 {
		t.Fatalf("count after expiry = %d, want 0", n)
	}

	d := NewMemoryClock(nil)
	if d.now == nil {
		t.Fatal("nil clock left now unset")
	}
	// expiry in the real future: still counted under time.Now
	if _, err := d.Consume(ctx, c, 1, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n := peekOne(t, d, "k"); n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
}
