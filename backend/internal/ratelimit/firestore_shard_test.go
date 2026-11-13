package ratelimit

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/option"
)

// offlineStore is a Firestore store whose client never connects (docsOf only
// builds references).
func offlineStore(t *testing.T, col string) *Firestore {
	t.Helper()
	c, err := firestore.NewClient(context.Background(), "demo-kz2601", option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return NewFirestore(c, col)
}

// #2565: docsOf is the shards then the legacy document; one document when not sharded.
func TestShardDocsOf(t *testing.T) {
	s := offlineStore(t, "rateLimitsDocsOf")
	cases := []struct {
		name string
		c    Counter
		want []string
	}{
		{"not sharded", Counter{ID: "g"}, []string{"g"}},
		{"one shard is not sharded", Counter{ID: "g", Shards: 1}, []string{"g"}},
		{"two shards", Counter{ID: "g", Shards: 2}, []string{"g_0", "g_1", "g"}},
		{"ten shards", Counter{ID: "day", Shards: 10}, []string{"day_0", "day_1", "day_2", "day_3", "day_4", "day_5", "day_6", "day_7", "day_8", "day_9", "day"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			refs := s.docsOf(c.c)
			if len(refs) != len(c.want) {
				t.Fatalf("docsOf = %d refs, want %v", len(refs), c.want)
			}
			for i, r := range refs {
				if r.ID != c.want[i] || r.Parent.ID != "rateLimitsDocsOf" {
					t.Errorf("ref %d = %s/%s, want rateLimitsDocsOf/%s", i, r.Parent.ID, r.ID, c.want[i])
				}
			}
		})
	}
}

// #2565: sum adds the counts of every document.
func TestShardSum(t *testing.T) {
	cases := []struct {
		name   string
		counts []int
		want   int
	}{
		{"nil", nil, 0},
		{"empty", []int{}, 0},
		{"one", []int{7}, 7},
		{"shards and legacy", []int{1, 0, 2, 0, 5}, 8},
		{"missing documents read as zero", []int{0, 0, 0}, 0},
	}
	for _, c := range cases {
		if got := sum(c.counts); got != c.want {
			t.Errorf("%s: sum(%v) = %d, want %d", c.name, c.counts, got, c.want)
		}
	}
}

// #2565 (emulator only): near the limit (strict path) a sharded counter allows
// up to its limit, counting the legacy document, and refuses beyond it.
func TestFirestoreConsumeShardedStrictLimit(t *testing.T) {
	s, c := newEmulatorStore(t)
	ctx := t.Context()
	g := Counter{ID: "gl_" + testSuffix(t), Scope: ScopeGlobal, Limit: 7, Shards: 4}
	exp := time.Now().Add(48 * time.Hour)
	if _, err := c.Collection("rateLimitsTest").Doc(g.ID).Set(ctx, map[string]any{"count": int64(2), "scope": ScopeGlobal, "expireAt": exp}); err != nil {
		t.Fatal(err)
	}

	steps := []struct {
		name    string
		weight  int
		blocked int
		total   int
	}{
		{"within", 2, -1, 4},
		{"up to the limit", 3, -1, 7},
		{"over", 1, 1, 7}, // g (index 1) blocks
	}
	for _, st := range steps {
		u := Counter{ID: "ul_" + testSuffix(t), Scope: ScopeUser, Limit: 100}
		usage, err := s.Consume(ctx, []Counter{u, g}, st.weight, exp)
		if err != nil {
			t.Fatalf("%s: %v", st.name, err)
		}
		if usage.Blocked != st.blocked {
			t.Fatalf("%s: usage %+v, want blocked %d", st.name, usage, st.blocked)
		}
		if usage.Blocked >= 0 && usage.Charged != nil {
			t.Fatalf("%s: blocked but charged %+v", st.name, usage.Charged)
		}
		if usage.Blocked < 0 && (len(usage.Charged) != 2 || usage.Charged[0] != u || !isShardOf(usage.Charged[1].ID, g)) {
			t.Fatalf("%s: charged %+v", st.name, usage.Charged)
		}
		if got := shardTotal(t, s, g); got != st.total {
			t.Fatalf("%s: total %d, want %d", st.name, got, st.total)
		}
	}
	// the legacy document is read, never written
	snap, err := c.Collection("rateLimitsTest").Doc(g.ID).Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := snap.DataAt("count"); n != int64(2) {
		t.Fatalf("legacy count = %v", n)
	}
}

// #2565 (emulator only): far from the limit (fast path) a plain counter over
// its limit still refuses, and nothing is charged.
func TestFirestoreConsumeShardedFastPathBlock(t *testing.T) {
	s, _ := newEmulatorStore(t)
	ctx := t.Context()
	g := Counter{ID: "gf_" + testSuffix(t), Scope: ScopeGlobal, Limit: 1000, Shards: GlobalShards}
	u := Counter{ID: "uf_" + testSuffix(t), Scope: ScopeUser, Limit: 3}
	exp := time.Now().Add(48 * time.Hour)

	usage, err := s.Consume(ctx, []Counter{u, g}, 2, exp)
	if err != nil || usage.Blocked != -1 || len(usage.Counts) != 2 || usage.Counts[0] != 2 || usage.Counts[1] != 2 {
		t.Fatalf("first: %+v %v", usage, err)
	}
	usage, err = s.Consume(ctx, []Counter{u, g}, 2, exp)
	if err != nil || usage.Blocked != 0 || usage.Charged != nil {
		t.Fatalf("over the user limit: %+v %v", usage, err)
	}
	if got := shardTotal(t, s, g); got != 2 {
		t.Fatalf("global total %d, want 2", got)
	}
	if got, err := s.Peek(ctx, []Counter{u}); err != nil || got[0] != 2 {
		t.Fatalf("user count %v %v", got, err)
	}
}

func isShardOf(id string, g Counter) bool {
	for k := range g.Shards {
		if id == shardID(g.ID, k) {
			return true
		}
	}
	return false
}
