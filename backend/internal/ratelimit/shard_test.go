package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
)

func TestShardedIndex(t *testing.T) {
	if g, err := shardedIndex([]Counter{{ID: "u"}, {ID: "g", Shards: 1}}); g != -1 || err != nil {
		t.Fatalf("no sharded: %d %v", g, err)
	}
	if g, err := shardedIndex([]Counter{{ID: "u"}, {ID: "g", Shards: 10}}); g != 1 || err != nil {
		t.Fatalf("one sharded: %d %v", g, err)
	}
	if _, err := shardedIndex([]Counter{{ID: "a", Shards: 2}, {ID: "b", Shards: 2}}); !errors.Is(err, errManySharded) {
		t.Fatalf("two sharded: %v", err)
	}
}

func TestNearLimit(t *testing.T) {
	cases := []struct {
		seen, weight, limit int
		want                bool
	}{
		{0, 3, 2000, false},
		{2000 - ShardStrictMargin - 3, 3, 2000, false}, // exactly at the margin: fast
		{2000 - ShardStrictMargin - 2, 3, 2000, true},
		{2000, 1, 2000, true},
		{0, 1, ShardStrictMargin, true}, // limit within the margin: always strict
	}
	for _, c := range cases {
		if got := nearLimit(c.seen, c.weight, c.limit); got != c.want {
			t.Errorf("nearLimit(%d,%d,%d)=%v", c.seen, c.weight, c.limit, got)
		}
	}
}

// shardingStore stands in for Firestore: it charges Memory and reports the
// sharded counter as charged to shard 3.
type shardingStore struct {
	*Memory
	consumed []Counter
	refunded []Counter
}

func (s *shardingStore) Consume(ctx context.Context, c []Counter, w int, exp time.Time) (Usage, error) {
	s.consumed = c
	u, err := s.Memory.Consume(ctx, c, w, exp)
	if err == nil && u.Blocked < 0 {
		u.Charged = append([]Counter(nil), c...)
		for i := range u.Charged {
			if u.Charged[i].Shards > 1 {
				u.Charged[i] = Counter{ID: shardID(c[i].ID, 3), Scope: c[i].Scope, Limit: c[i].Limit}
			}
		}
	}
	return u, err
}

func (s *shardingStore) Refund(ctx context.Context, c []Counter, w int, exp time.Time) error {
	s.refunded = c
	return s.Memory.Refund(ctx, c, w, exp)
}

// Allow shards only the global counter, and Refund uses Usage.Charged.
func TestLimiterShardsGlobalOnly(t *testing.T) {
	c := &clock{t: time.Date(2026, 10, 25, 23, 0, 0, 0, JST)}
	s := &shardingStore{Memory: NewMemory()}
	l := New(Config{Limits: Limits{Anonymous: 30, PerIP: 300, Global: 2000}, Weights: DefaultWeights, Env: "test", Now: c.now}, s, nil)
	dec, err := l.Allow(context.Background(), auth.Identity{UID: "a", Anonymous: true}, "1.2.3.4", 3)
	if err != nil || !dec.Allowed {
		t.Fatalf("allow: %+v %v", dec, err)
	}
	if len(s.consumed) != 3 {
		t.Fatalf("counters: %+v", s.consumed)
	}
	for _, ctr := range s.consumed {
		want := 0
		if ctr.Scope == ScopeGlobal {
			want = GlobalShards
		}
		if ctr.Shards != want {
			t.Fatalf("%s shards = %d, want %d", ctr.Scope, ctr.Shards, want)
		}
	}
	if err := l.Refund(context.Background(), dec, 3); err != nil {
		t.Fatal(err)
	}
	if got, want := s.refunded[2], (Counter{ID: "test_2026-10-25_global_3", Scope: ScopeGlobal, Limit: 2000}); got != want {
		t.Fatalf("refunded global = %+v, want %+v", got, want)
	}
	if s.refunded[0] != s.consumed[0] || s.refunded[1] != s.consumed[1] {
		t.Fatalf("user/ip refund changed: %+v", s.refunded)
	}
}
