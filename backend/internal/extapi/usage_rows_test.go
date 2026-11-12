package extapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func readTestdata(t *testing.T, parts ...string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(append([]string{"testdata"}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// usageTestKeys is the per-user key store's stand-in: the key is Args["_key"].
func usageTestKeys(_ context.Context, in Input) (usageKey, error) {
	if in.Args["_key"] == "" {
		return usageKey{}, ErrNotConnected
	}
	return usageKey{Key: in.Args["_key"]}, nil
}

func usageTestIn(key string, args map[string]string) Input {
	a := map[string]string{"_key": key}
	for k, v := range args {
		a[k] = v
	}
	return Input{Args: a}
}

func TestUsageClientGet(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.Header.Get("X-Test-Key") {
		case "ok":
			_, _ = w.Write([]byte(`{"q":"` + r.URL.Query().Get("q") + `"}`))
		case "limited":
			w.WriteHeader(429)
		case "forbidden":
			w.WriteHeader(403)
		default:
			w.WriteHeader(401)
		}
	}))
	t.Cleanup(srv.Close)
	c := &usageClient{keys: usageTestKeys, base: srv.URL, hc: srv.Client(), now: time.Now}
	auth := func(r *http.Request, key string) { r.Header.Set("X-Test-Key", key) }
	ctx := context.Background()
	body, err := c.get(ctx, usageTestIn("ok", nil), "/x", url.Values{"q": {"1"}}, auth)
	if err != nil || string(body) != `{"q":"1"}` {
		t.Fatalf("%s %v", body, err)
	}
	if _, err := c.get(ctx, Input{}, "/x", url.Values{}, auth); !errors.Is(err, ErrNotConnected) || calls != 1 {
		t.Fatalf("no key: %v calls=%d", err, calls)
	}
	for _, k := range []string{"dead", "forbidden"} {
		if _, err := c.get(ctx, usageTestIn(k, nil), "/x", url.Values{}, auth); !errors.Is(err, ErrNotConnected) {
			t.Fatalf("%s: %v", k, err)
		}
	}
	_, err = c.get(ctx, usageTestIn("limited", nil), "/x", url.Values{}, auth)
	var pe *ProviderError
	if !errors.As(err, &pe) || !pe.Retryable || Reason(err) != ReasonUpstream {
		t.Fatalf("429: %v", err)
	}
}

func TestReadPages(t *testing.T) {
	var asked []string
	rows, err := readPages(func(next string) (usagePage[int], error) {
		asked = append(asked, next)
		if next == "" {
			return usagePage[int]{Rows: []int{1}, HasMore: true, NextPage: "p2"}, nil
		}
		return usagePage[int]{Rows: []int{2}}, nil
	})
	if err != nil || !slices.Equal(rows, []int{1, 2}) || !slices.Equal(asked, []string{"", "p2"}) {
		t.Fatalf("%v %v %v", rows, asked, err)
	}
	n := 0
	_, err = readPages(func(string) (usagePage[int], error) {
		n++
		return usagePage[int]{HasMore: true, NextPage: "again"}, nil
	})
	if err == nil || n != usageMaxPages {
		t.Fatalf("endless: %v after %d", err, n)
	}
}

func TestMergeCostRows(t *testing.T) {
	got := mergeCostRows([]CostRow{
		{Day: "2026-11-11", Currency: "USD", Amount: 1},
		{Day: "2026-11-10", Currency: "USD", Amount: 0.25},
		{Day: "2026-11-11", Currency: "USD", Amount: 0.5},
	})
	want := []CostRow{{Day: "2026-11-10", Currency: "USD", Amount: 0.25}, {Day: "2026-11-11", Currency: "USD", Amount: 1.5}}
	if !slices.Equal(got, want) {
		t.Fatalf("%+v", got)
	}
}

func TestUsageDays(t *testing.T) {
	for a, want := range map[string]int{"": 7, "1": 1, "31": 31, " 14 ": 14} {
		if n, err := usageDays(map[string]string{"days": a}); err != nil || n != want {
			t.Errorf("%q: %d %v", a, n, err)
		}
	}
	for _, a := range []string{"0", "32", "-1", "x", "1.5"} {
		if err := checkUsageDays(map[string]string{"days": a}); !errors.Is(err, ErrBadArgs) {
			t.Errorf("%q: %v", a, err)
		}
	}
	now := time.Date(2026, 11, 12, 23, 30, 0, 0, time.FixedZone("JST", 9*3600)) // 14:30Z
	if got := usageStart(now, 7); !got.Equal(time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("start %v", got)
	}
	c := &usageClient{now: func() time.Time { return now }}
	if got := c.start(1); !got.Equal(time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("today %v", got)
	}
}

func TestUsageOutputs(t *testing.T) {
	u := usageOutput([]UsageRow{{Day: "2026-11-10", Model: "m", InputTokens: 1, OutputTokens: 2, CacheRead: 3, CacheWrite: 4}})
	if u.Rows[0] != (Row{Title: "2026-11-10", Subtitle: "m", Value: "10"}) || !slices.Equal(u.Cells[0], []string{"2026-11-10", "m", "1", "2", "3", "4"}) {
		t.Fatalf("%+v", u)
	}
	c := costOutput([]CostRow{{Day: "2026-11-10", Currency: "USD", Amount: 1.236}})
	if c.Rows[0] != (Row{Title: "2026-11-10", Subtitle: "USD", Value: "1.24"}) || len(c.Cells[0]) != 3 {
		t.Fatalf("%+v", c)
	}
	if e := usageOutput(nil); e.Rows == nil || len(e.Rows) != 0 {
		t.Fatalf("empty: %+v", e)
	}
	k := usageKey{Key: "sk-admin-secret"}
	if s := fmt.Sprintf("%v %+v %#v %s", k, k, k, k); s != "[redacted] [redacted] [redacted] [redacted]" {
		t.Fatalf("key printed: %s", s)
	}
}
