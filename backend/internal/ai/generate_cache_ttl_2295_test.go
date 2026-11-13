package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateCacheTTLFromEnv_2295(t *testing.T) {
	for v, want := range map[string]string{"": CacheTTL5m, "5m": CacheTTL5m, " 1h ": CacheTTL1h} {
		got, err := GenerateCacheTTLFromEnv(func(string) string { return v })
		if err != nil || got != want {
			t.Fatalf("%q: %q %v", v, got, err)
		}
	}
	for _, v := range []string{"60m", "1H", "24h", "0"} {
		if _, err := GenerateCacheTTLFromEnv(func(string) string { return v }); err == nil {
			t.Fatalf("%q: no error", v)
		}
	}
}

// Unset, the request is the same as before (no ttl anywhere); with 1h only a
// create turn's cached system blocks carry ttl 1h, and ai.usage says which.
func TestGenerateCacheTTLOnTheWire_2295(t *testing.T) {
	var sent []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, okBody("claude-test"))
	}))
	defer srv.Close()
	cases := []struct {
		name, setting, op string
		suffix            string
		want              string // the ttl of the cached blocks ("" = none)
	}{
		{"default generate", "", OpGenerate, "", ""},
		{"5m generate", CacheTTL5m, OpGenerate, "", ""},
		{"1h generate", CacheTTL1h, OpGenerate, "", "1h"},
		{"1h generate two blocks", CacheTTL1h, OpGenerate, "\nmore", "1h"},
		{"1h patch", CacheTTL1h, OpPatch, "\nedit", ""},
		{"1h chat", CacheTTL1h, OpChat, "", ""},
		{"1h chat first", CacheTTL1h, OpChatFirst, "", ""},
		{"1h unlabeled", CacheTTL1h, "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, logs := testClient(srv, "claude-test")
			c.GenerateCacheTTL = tc.setting
			ctx := context.Background()
			if tc.op != "" {
				ctx = WithOperation(ctx, tc.op)
			}
			req := Completion{System: "base", SystemSuffix: tc.suffix, SystemTail: "tail", Messages: []Message{{Role: "user", Content: "u"}}}
			if _, err := c.Complete(ctx, req); err != nil {
				t.Fatal(err)
			}
			var wr struct {
				System []struct {
					CacheControl map[string]any `json:"cache_control"`
				} `json:"system"`
			}
			if err := json.Unmarshal(sent, &wr); err != nil {
				t.Fatal(err)
			}
			cached := 0
			for _, b := range wr.System {
				if b.CacheControl == nil {
					continue // the uncached tail
				}
				cached++
				ttl, has := b.CacheControl["ttl"]
				if b.CacheControl["type"] != "ephemeral" || (tc.want == "" && has) || (tc.want != "" && ttl != tc.want) || len(b.CacheControl) > 2 {
					t.Fatalf("cache_control %v, want ttl %q", b.CacheControl, tc.want)
				}
			}
			if cached == 0 {
				t.Fatal("no cached block")
			}
			lines := usageLines(t, logs)
			wantLog := CacheTTL5m
			if tc.want != "" {
				wantLog = tc.want
			}
			if len(lines) != 1 || lines[0]["cache_ttl"] != wantLog {
				t.Fatalf("usage %v, want cache_ttl %q", lines, wantLog)
			}
		})
	}
}

// A call without cached system blocks logs no cache_ttl.
func TestUsageLogNoCacheTTLWithoutSystem_2295(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, okBody("claude-test"))
	}))
	defer srv.Close()
	c, logs := testClient(srv, "claude-test")
	c.GenerateCacheTTL = CacheTTL1h
	req := Completion{SystemTail: "tail", Messages: []Message{{Role: "user", Content: "u"}}}
	if _, err := c.Complete(WithOperation(context.Background(), OpGenerate), req); err != nil {
		t.Fatal(err)
	}
	if l := usageLines(t, logs); len(l) != 1 || l[0]["cache_ttl"] != nil {
		t.Fatalf("usage %v", l)
	}
}
