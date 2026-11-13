package appconfig

import (
	"errors"
	"testing"
	"time"
)

func TestTTLCache_2605(t *testing.T) {
	now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	loads := 0
	loader := func(v int) func() (int, error) {
		return func() (int, error) { loads++; return v, nil }
	}

	t.Run("inside ttl is not reloaded, expiry reloads", func(t *testing.T) {
		loads = 0
		c := newTTLCache[int](10*time.Second, 8, clock)
		if v, _ := c.get("k", loader(1)); v != 1 {
			t.Fatalf("first: %d", v)
		}
		now = now.Add(9 * time.Second)
		if v, _ := c.get("k", loader(2)); v != 1 || loads != 1 {
			t.Fatalf("within ttl: v=%d loads=%d", v, loads)
		}
		now = now.Add(time.Second) // exactly at until: expired
		if v, _ := c.get("k", loader(3)); v != 3 || loads != 2 {
			t.Fatalf("after ttl: v=%d loads=%d", v, loads)
		}
	})

	t.Run("keys are independent", func(t *testing.T) {
		loads = 0
		c := newTTLCache[int](time.Minute, 8, clock)
		c.get("a", loader(1))
		if v, _ := c.get("b", loader(2)); v != 2 || loads != 2 {
			t.Fatalf("b: v=%d loads=%d", v, loads)
		}
		if v, _ := c.get("a", loader(9)); v != 1 || loads != 2 {
			t.Fatalf("a cached: v=%d loads=%d", v, loads)
		}
	})

	t.Run("load error is returned and not cached", func(t *testing.T) {
		c := newTTLCache[int](time.Minute, 8, clock)
		boom := errors.New("boom")
		if _, err := c.get("k", func() (int, error) { return 0, boom }); !errors.Is(err, boom) {
			t.Fatalf("err %v", err)
		}
		if len(c.m) != 0 {
			t.Fatalf("error cached: %v", c.m)
		}
		if v, err := c.get("k", func() (int, error) { return 7, nil }); err != nil || v != 7 {
			t.Fatalf("retry: %d %v", v, err)
		}
	})

	t.Run("full cache drops everything then stores the new key", func(t *testing.T) {
		loads = 0
		c := newTTLCache[int](time.Minute, 2, clock)
		c.get("a", loader(1))
		c.get("b", loader(2))
		c.get("c", loader(3)) // len==max: reset, only c remains
		if len(c.m) != 1 {
			t.Fatalf("size %d", len(c.m))
		}
		if _, ok := c.m["c"]; !ok {
			t.Fatalf("new key missing: %v", c.m)
		}
		before := loads
		c.get("a", loader(10)) // dropped earlier: loads again
		if loads != before+1 {
			t.Fatalf("a should reload, loads=%d", loads)
		}
	})
}
