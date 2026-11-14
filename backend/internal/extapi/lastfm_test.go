package extapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// lastfmServer は ws.audioscrobbler.com の代わり(本物は呼ばない)。
type lastfmServer struct {
	*httptest.Server
	hits   atomic.Int64
	last   atomic.Value
	status int
	body   string
}

const lastfmTestKey = "0123456789abcdef0123456789abcdef"

const lastfmRecentBody = `{"recenttracks":{"track":[
{"artist":{"mbid":"","#text":"Perfume"},"name":" ポリリズム ","album":{"#text":"GAME"},"url":"https://www.last.fm/music/Perfume/_/%E3%83%9D%E3%83%AA%E3%83%AA%E3%82%BA%E3%83%A0?x=1","@attr":{"nowplaying":"true"}},
{"artist":{"#text":"YOASOBI"},"name":"夜に駆ける","album":{"#text":""},"url":"https://www.last.fm/music/YOASOBI/_/%E5%A4%9C%E3%81%AB%E9%A7%86%E3%81%91%E3%82%8B","date":{"uts":"1763090400","#text":"14 Nov 2026, 02:00"}},
{"artist":{"#text":"x"},"name":"","url":"https://www.last.fm/x","date":{"uts":"1"}},
{"artist":{"#text":"y"},"name":"外のリンク","url":"http://evil.example.com/","date":{"uts":"bad"}}
],"@attr":{"user":"rj","total":"4"}}}`

const lastfmTopBody = `{"topartists":{"artist":[
{"name":"Perfume","playcount":"1200","url":"https://www.last.fm/music/Perfume","@attr":{"rank":"1"}},
{"name":"YOASOBI","playcount":"abc","url":"https://last.fm/music/YOASOBI","@attr":{"rank":"2"}},
{"name":"","playcount":"3","url":"https://www.last.fm/music/x","@attr":{"rank":"3"}}
],"@attr":{"user":"rj"}}}`

func newLastfmServer(t *testing.T) *lastfmServer {
	t.Helper()
	s := &lastfmServer{status: http.StatusOK, body: lastfmRecentBody}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		s.last.Store(r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.status)
		_, _ = w.Write([]byte(s.body))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *lastfmServer) uri() string { v, _ := s.last.Load().(string); return v }

func (s *lastfmServer) ops(t *testing.T) map[string]Operation {
	t.Helper()
	ops, err := newLastfm(lastfmTestKey, s.URL, s.Client())
	if err != nil {
		t.Fatal(err)
	}
	return ops
}

func TestLastfmRegistered(t *testing.T) {
	p, ok := Lookup(Lastfm)
	if !ok || p.Billing != BillingFree || p.NeedsOAuth || p.Fake || !slices.Equal(p.Hosts, []string{"https://ws.audioscrobbler.com"}) {
		t.Fatalf("lastfm %+v", p)
	}
	if !slices.Contains(ReviewBuildFeatures(), "externalCall.lastfm") {
		t.Fatal("a provider without OAuth belongs to the review build")
	}
	if len(p.Ops) != 2 || p.Ops[0].Name != "recentTracks" || p.Ops[0].CacheTTL != time.Minute || p.Ops[1].Name != "topArtists" || p.Ops[1].CacheTTL != time.Hour {
		t.Fatalf("ops %+v", p.Ops)
	}
	cfg, err := ConfigFromEnv(func(string) string { return "" })
	if err != nil || cfg.Enabled[Lastfm] {
		t.Fatalf("default: %v %v", cfg.Enabled, err)
	}
	cfg, err = ConfigFromEnv(func(k string) string {
		return map[string]string{"EXT_LASTFM_ENABLED": "1", "EXT_LASTFM_API_KEY": lastfmTestKey}[k]
	})
	if err != nil || !cfg.Enabled[Lastfm] || cfg.Secrets[Lastfm].APIKey != lastfmTestKey {
		t.Fatalf("EXT_LASTFM_ENABLED: %v %v", cfg.Enabled, err)
	}
	// 鍵が無い・鍵でない → 動かない
	for _, k := range []string{"", "short", "ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ"} {
		if _, err := p.New(Secrets{APIKey: k}); err == nil {
			t.Fatalf("key %q accepted", k)
		}
	}
	if _, err := p.New(Secrets{APIKey: lastfmTestKey}); err != nil {
		t.Fatal(err)
	}
}

func TestLastfmRecentTracks(t *testing.T) {
	s := newLastfmServer(t)
	out, err := s.ops(t)["recentTracks"].Do(context.Background(), Input{Args: map[string]string{"user": " rj ", "limit": "5"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.uri() != "/2.0/?api_key="+lastfmTestKey+"&format=json&limit=5&method=user.getrecenttracks&user=rj" {
		t.Fatalf("request %s", s.uri())
	}
	want := []Row{
		{Title: "ポリリズム", Subtitle: "Perfume・GAME", Value: "再生中", URL: "https://www.last.fm/music/Perfume/_/%E3%83%9D%E3%83%AA%E3%83%AA%E3%82%BA%E3%83%A0"},
		{Title: "夜に駆ける", Subtitle: "YOASOBI", Value: "2025/11/14 12:20", URL: "https://www.last.fm/music/YOASOBI/_/%E5%A4%9C%E3%81%AB%E9%A7%86%E3%81%91%E3%82%8B"},
		{Title: "外のリンク", Subtitle: "y"},
	}
	if !slices.Equal(out.Rows, want) {
		t.Fatalf("rows %+v", out.Rows)
	}
	// limit は行を切る・既定は 10
	out, err = s.ops(t)["recentTracks"].Do(context.Background(), Input{Args: map[string]string{"user": "rj", "limit": "1"}})
	if err != nil || len(out.Rows) != 1 {
		t.Fatalf("limit 1: %+v %v", out.Rows, err)
	}
	if _, err := s.ops(t)["recentTracks"].Do(context.Background(), Input{Args: map[string]string{"user": "rj"}}); err != nil || !strings.Contains(s.uri(), "limit=10") {
		t.Fatalf("default limit: %s %v", s.uri(), err)
	}
	// 1 曲だけの時は配列でなく 1 個
	s.body = `{"recenttracks":{"track":{"artist":{"#text":"A"},"name":"one","url":"https://www.last.fm/music/A/_/one","date":{"uts":"1763090400"}}}}`
	out, err = s.ops(t)["recentTracks"].Do(context.Background(), Input{Args: map[string]string{"user": "rj"}})
	if err != nil || len(out.Rows) != 1 || out.Rows[0].Title != "one" {
		t.Fatalf("single: %+v %v", out.Rows, err)
	}
	// 無い人: 404 / error 6 は 400 bad_request(呼んだ後)
	for _, c := range []struct {
		st   int
		body string
	}{{http.StatusNotFound, `{"error":6,"message":"User not found"}`}, {http.StatusOK, `{"error":6,"message":"Invalid parameters"}`}} {
		s.status, s.body = c.st, c.body
		if _, err := s.ops(t)["recentTracks"].Do(context.Background(), Input{Args: map[string]string{"user": "nobody"}}); !errors.Is(err, ErrUnknown) || Reason(err) != ReasonBadRequest {
			t.Fatalf("%d %s: %v", c.st, c.body, err)
		}
	}
	// 回数超え・5xx・鍵の不備は ProviderError
	for _, c := range []struct {
		st    int
		body  string
		retry bool
	}{{http.StatusOK, `{"error":29,"message":"Rate limit exceeded"}`, true}, {http.StatusTooManyRequests, `{"error":29}`, true},
		{http.StatusBadGateway, `<html>`, true}, {http.StatusForbidden, `{"error":10,"message":"Invalid API key"}`, false}} {
		s.status, s.body = c.st, c.body
		var pe *ProviderError
		if _, err := s.ops(t)["recentTracks"].Do(context.Background(), Input{Args: map[string]string{"user": "rj"}}); !errors.As(err, &pe) || pe.Retryable != c.retry || strings.Contains(err.Error(), lastfmTestKey) {
			t.Fatalf("%d %s: %v", c.st, c.body, err)
		}
	}
}

func TestLastfmTopArtists(t *testing.T) {
	s := newLastfmServer(t)
	s.body = lastfmTopBody
	out, err := s.ops(t)["topArtists"].Do(context.Background(), Input{Args: map[string]string{"user": "rj", "period": "7day", "limit": "3"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.uri() != "/2.0/?api_key="+lastfmTestKey+"&format=json&limit=3&method=user.gettopartists&period=7day&user=rj" {
		t.Fatalf("request %s", s.uri())
	}
	want := []Row{
		{Title: "Perfume", Subtitle: "#1", Value: "1200 回", URL: "https://www.last.fm/music/Perfume"},
		{Title: "YOASOBI", Subtitle: "#2", URL: "https://www.last.fm/music/YOASOBI"},
	}
	if !slices.Equal(out.Rows, want) {
		t.Fatalf("rows %+v", out.Rows)
	}
	// period の既定は overall
	if _, err := s.ops(t)["topArtists"].Do(context.Background(), Input{Args: map[string]string{"user": "rj"}}); err != nil || !strings.Contains(s.uri(), "period=overall") {
		t.Fatalf("default period: %s %v", s.uri(), err)
	}
}

func TestLastfmCheck(t *testing.T) {
	p, _ := Lookup(Lastfm)
	for _, c := range []struct {
		op   string
		args map[string]string
		ok   bool
	}{
		{"recentTracks", map[string]string{"user": "rj"}, true},
		{"recentTracks", map[string]string{"user": "a_b-c123", "limit": "20"}, true},
		{"recentTracks", nil, false},
		{"recentTracks", map[string]string{"user": "r"}, false},
		{"recentTracks", map[string]string{"user": "1abc"}, false},
		{"recentTracks", map[string]string{"user": "abcdefghijklmnop"}, false},
		{"recentTracks", map[string]string{"user": "rj", "limit": "0"}, false},
		{"recentTracks", map[string]string{"user": "rj", "limit": "21"}, false},
		{"recentTracks", map[string]string{"user": "rj", "period": "7day"}, false},
		{"topArtists", map[string]string{"user": "rj"}, true},
		{"topArtists", map[string]string{"user": "rj", "period": "12month", "limit": "1"}, true},
		{"topArtists", map[string]string{"user": "rj", "period": "week"}, false},
		{"topArtists", map[string]string{"user": "https://x.example"}, false},
	} {
		_, err := p.CheckArgs(c.op, c.args)
		if (err == nil) != c.ok {
			t.Errorf("%s %v: err %v want ok=%v", c.op, c.args, err, c.ok)
		}
		if err != nil && !errors.Is(err, ErrUnknown) {
			t.Errorf("%s %v: %v is not ErrUnknown", c.op, c.args, err)
		}
	}
}
