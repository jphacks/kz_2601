package extapi

import (
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestStatusOf_3176(t *testing.T) {
	t.Parallel()
	tests := []struct {
		reason string
		want   int
	}{
		{ReasonOK, http.StatusOK},
		{ReasonBadRequest, http.StatusBadRequest},
		{ReasonNotConnected, http.StatusConflict},
		{ReasonDuplicateRequest, http.StatusConflict},
		{ReasonUnavailable, http.StatusServiceUnavailable},
		{ReasonBusy, http.StatusServiceUnavailable},
		{ReasonProRequired, http.StatusPaymentRequired},
		{ReasonAllowanceExhausted, http.StatusPaymentRequired},
		{ReasonOverflowCap, http.StatusPaymentRequired},
		{ReasonCoinsInsufficient, http.StatusPaymentRequired},
		{ReasonRateLimited, http.StatusTooManyRequests},
		{ReasonUpstreamTimeout, http.StatusGatewayTimeout},
		{ReasonUpstream, http.StatusBadGateway},
		{ReasonInternal, http.StatusInternalServerError},
		{"知らない理由", http.StatusInternalServerError},
		{"", http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.reason, func(t *testing.T) {
			t.Parallel()
			if got := StatusOf(tc.reason); got != tc.want {
				t.Errorf("StatusOf(%q) = %d, want %d", tc.reason, got, tc.want)
			}
		})
	}
}

// 上限に着いたら、期限切れだけを消す。期限切れが無ければ全部捨てて新しい 1 件だけ残す。
func TestCachePutEvicts_3176(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	fill := func(c *callCache, expires time.Time) {
		for i := 0; i < maxCacheEntries; i++ {
			c.m["k"+strconv.Itoa(i)] = cacheEntry{expires: expires}
		}
	}

	t.Run("上限の手前では消さない", func(t *testing.T) {
		t.Parallel()
		c := newCallCache()
		fill(c, now.Add(time.Hour))
		delete(c.m, "k0")
		c.put("new", Output{ResultID: "n"}, now.Add(time.Hour), now)
		if len(c.m) != maxCacheEntries {
			t.Errorf("len = %d, want %d", len(c.m), maxCacheEntries)
		}
	})

	t.Run("上限で期限切れだけ消える", func(t *testing.T) {
		t.Parallel()
		c := newCallCache()
		fill(c, now.Add(time.Hour))
		c.m["old"] = cacheEntry{expires: now.Add(-time.Minute)}
		delete(c.m, "k0")
		c.put("new", Output{ResultID: "n"}, now.Add(time.Hour), now)
		if _, ok := c.m["old"]; ok {
			t.Error("期限切れが残っている")
		}
		if _, ok := c.m["k1"]; !ok {
			t.Error("期限内が消えた")
		}
		if _, ok := c.m["new"]; !ok {
			t.Error("新しい 1 件が無い")
		}
	})

	t.Run("上限で全部が期限内なら全部捨てて新しい 1 件", func(t *testing.T) {
		t.Parallel()
		c := newCallCache()
		fill(c, now.Add(time.Hour))
		c.put("new", Output{ResultID: "n"}, now.Add(time.Hour), now)
		if len(c.m) != 1 {
			t.Fatalf("len = %d, want 1", len(c.m))
		}
		if e, ok := c.m["new"]; !ok || e.out.ResultID != "n" {
			t.Errorf("new = %+v, %v", e, ok)
		}
	})

	t.Run("上限で全部が期限切れなら新しい 1 件だけ", func(t *testing.T) {
		t.Parallel()
		c := newCallCache()
		fill(c, now.Add(-time.Second))
		c.put("new", Output{}, now.Add(time.Hour), now)
		if len(c.m) != 1 {
			t.Errorf("len = %d, want 1", len(c.m))
		}
	})

	t.Run("使った額は覚えない", func(t *testing.T) {
		t.Parallel()
		c := newCallCache()
		c.put("k", Output{Usage: Usage{USD: 1, Priced: true}}, now.Add(time.Hour), now)
		got, ok := c.get("k", now)
		if !ok || got.Usage != (Usage{}) {
			t.Errorf("get = %+v, %v; Usage を消すはず", got, ok)
		}
	})
}

func TestCacheCloneIsolated_3176(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	c := newCallCache()
	in := Output{
		Rows: []Row{{Title: "a"}, {Title: "b"}},
		Item: &Row{Title: "item"},
	}
	c.put("k", in, now.Add(time.Hour), now)

	// 入れた物を書き換えても、覚えの中は変わらない
	in.Rows[0].Title = "changed-in"
	in.Item.Title = "changed-in"

	got, ok := c.get("k", now)
	if !ok {
		t.Fatal("miss")
	}
	if got.Rows[0].Title != "a" || got.Item.Title != "item" {
		t.Fatalf("入れた物の書き換えが覚えに届いた: %+v / %+v", got.Rows, got.Item)
	}

	// 返した値を書き換えても、覚えの中は変わらない
	got.Rows[0].Title = "changed-out"
	got.Rows[1] = Row{Title: "z"}
	got.Item.Title = "changed-out"

	again, ok := c.get("k", now)
	if !ok {
		t.Fatal("miss")
	}
	if again.Rows[0].Title != "a" || again.Rows[1].Title != "b" || again.Item.Title != "item" {
		t.Errorf("返した値の書き換えが覚えに届いた: %+v / %+v", again.Rows, again.Item)
	}
	if &got.Rows[0] == &again.Rows[0] || got.Item == again.Item {
		t.Error("同じ物を返している")
	}
}

func TestCloneEmpty_3176(t *testing.T) {
	t.Parallel()
	got := Output{ResultID: "r"}.clone()
	if got.Rows != nil || got.Item != nil || got.ResultID != "r" {
		t.Errorf("clone = %+v", got)
	}
}
