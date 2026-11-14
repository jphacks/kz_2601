package extapi

import (
	"strconv"
	"testing"
	"time"
)

func TestCFPagesState2596(t *testing.T) {
	cases := []struct {
		stage, status, want string
	}{
		{"deploy", "success", deployReady},
		{"build", "success", deployBuilding},
		{"queued", "success", deployBuilding},
		{"build", "failure", deployError},
		{"deploy", "failure", deployError},
		{"build", "canceled", deployCanceled},
		{"queued", "active", deployQueued},
		{"build", "active", deployBuilding},
		{"deploy", "active", deployBuilding},
		{"queued", "idle", deployQueued},
		{"build", "idle", deployQueued},
		{"deploy", "skipped", ""},
		{"deploy", "", ""},
		{"", "unknown", ""},
	}
	for _, c := range cases {
		if got := cfPagesState(c.stage, c.status); got != c.want {
			t.Errorf("cfPagesState(%q, %q) = %q, want %q", c.stage, c.status, got, c.want)
		}
	}
}

func TestConnectionsTTL2596(t *testing.T) {
	cases := []struct {
		name string
		ttl  time.Duration
		want time.Duration
	}{
		{"default", 0, DefaultUnusedTTL},
		{"negative is default", -time.Hour, DefaultUnusedTTL},
		{"set", 7 * 24 * time.Hour, 7 * 24 * time.Hour},
	}
	for _, c := range cases {
		cn := &Connections{UnusedTTL: c.ttl}
		if got := cn.ttl(); got != c.want {
			t.Errorf("%s: ttl() = %s, want %s", c.name, got, c.want)
		}
	}
	if DefaultUnusedTTL != 90*24*time.Hour {
		t.Errorf("DefaultUnusedTTL = %s, want 90 days", DefaultUnusedTTL)
	}
}

func TestCallCachePutOverwrite2596(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	c := newCallCache()
	c.put("k", Output{Rows: []Row{{Title: "old"}}}, now.Add(time.Minute), now)
	in := Output{Rows: []Row{{Title: "new"}}, Usage: Usage{USD: 0.5, Priced: true}}
	c.put("k", in, now.Add(time.Minute), now)
	in.Rows[0].Title = "changed after put"

	got, ok := c.get("k", now)
	if !ok || len(got.Rows) != 1 || got.Rows[0].Title != "new" {
		t.Fatalf("get = %+v, %v; want the overwritten answer", got, ok)
	}
	if got.Usage != (Usage{}) {
		t.Errorf("cached Usage = %+v, want zero (a cached answer costs nothing)", got.Usage)
	}
	if len(c.m) != 1 {
		t.Errorf("entries = %d, want 1", len(c.m))
	}
}

func TestCallCachePutAtCapacity2596(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	t.Run("expired entries are swept", func(t *testing.T) {
		c := newCallCache()
		for i := range maxCacheEntries - 1 {
			c.put("old"+strconv.Itoa(i), Output{}, now.Add(-time.Second), now)
		}
		c.put("live", Output{}, now.Add(time.Hour), now)
		if len(c.m) != maxCacheEntries {
			t.Fatalf("entries = %d, want %d", len(c.m), maxCacheEntries)
		}
		c.put("next", Output{}, now.Add(time.Hour), now)
		if len(c.m) != 2 {
			t.Errorf("entries = %d, want 2 (live + next)", len(c.m))
		}
		if _, ok := c.get("live", now); !ok {
			t.Error("live entry was dropped")
		}
	})

	t.Run("all live: the cache starts over", func(t *testing.T) {
		c := newCallCache()
		for i := range maxCacheEntries {
			c.put("live"+strconv.Itoa(i), Output{}, now.Add(time.Hour), now)
		}
		c.put("next", Output{}, now.Add(time.Hour), now)
		if len(c.m) != 1 {
			t.Errorf("entries = %d, want 1", len(c.m))
		}
		if _, ok := c.get("next", now); !ok {
			t.Error("new entry missing")
		}
	})
}
