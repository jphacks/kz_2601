package extapi

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	ytTestKey     = "test_key_0000000000000000000000000000"
	ytTestChannel = "UC_x5XG1OV2P6uZZ5FSM9Ttw"
	ytTestUploads = "UU_x5XG1OV2P6uZZ5FSM9Ttw"
)

type ytSeen struct {
	calls atomic.Int64
	query atomic.Value // url.Values of the last call
	path  atomic.Value
}

// ytServer is a fake YouTube Data API: it checks the key header and answers
// channels / playlistItems.
func ytServer(t *testing.T, status *atomic.Int64, seen *ytSeen) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.calls.Add(1)
		q := r.URL.Query()
		seen.query.Store(q)
		seen.path.Store(r.URL.Path)
		if q.Get("key") != "" || r.Header.Get("X-Goog-Api-Key") != ytTestKey {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if s := status.Load(); s != 200 {
			w.WriteHeader(int(s))
			_, _ = w.Write([]byte(`{"error":{"code":403,"message":"quotaExceeded ` + r.Header.Get("X-Goog-Api-Key") + `"}}`))
			return
		}
		switch r.URL.Path {
		case "/youtube/v3/channels":
			switch {
			case q.Get("forHandle") == "@nobody":
				_, _ = w.Write([]byte(`{"kind":"youtube#channelListResponse","pageInfo":{"totalResults":0}}`))
			case q.Get("forHandle") == "@hidden":
				_, _ = w.Write([]byte(`{"items":[{"id":"` + ytTestChannel + `","snippet":{"title":"Hidden"},"statistics":{"viewCount":"10","hiddenSubscriberCount":true,"videoCount":"1"}}]}`))
			case q.Get("forHandle") == "@empty":
				_, _ = w.Write([]byte(`{"items":[{"id":"` + ytTestChannel + `","contentDetails":{"relatedPlaylists":{"uploads":"UUempty0000000000000000000"}}}]}`))
			default:
				_, _ = w.Write([]byte(`{"items":[{"id":"` + ytTestChannel + `","snippet":{"title":"Google for Developers"},
					"statistics":{"viewCount":"123456789","subscriberCount":"2440000","hiddenSubscriberCount":false,"videoCount":"6543"},
					"contentDetails":{"relatedPlaylists":{"uploads":"` + ytTestUploads + `"}}}]}`))
			}
		case "/youtube/v3/playlistItems":
			if q.Get("playlistId") != ytTestUploads {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(`{"items":[
				{"snippet":{"title":"新しい動画"},"contentDetails":{"videoId":"abcdefghijk","videoPublishedAt":"2026-11-10T03:04:05Z"},"status":{"privacyStatus":"public"}},
				{"snippet":{"title":"Private video"},"contentDetails":{"videoId":"privateVid0"},"status":{"privacyStatus":"private"}},
				{"snippet":{"title":"2 本目"},"contentDetails":{"videoId":"ABCDEFGHIJK","videoPublishedAt":"bad"},"status":{"privacyStatus":"public"}},
				{"snippet":{"title":"3 本目"},"contentDetails":{"videoId":"zzzzzzzzzzz","videoPublishedAt":"2026-11-01T00:00:00Z"},"status":{"privacyStatus":"public"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func ytOps(t *testing.T) (map[string]Operation, *atomic.Int64, *ytSeen) {
	t.Helper()
	var status atomic.Int64
	status.Store(200)
	seen := &ytSeen{}
	srv := ytServer(t, &status, seen)
	ops, err := newYouTube(ytTestKey, srv.URL, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return ops, &status, seen
}

func TestYouTubeChannelStats(t *testing.T) {
	ops, _, seen := ytOps(t)
	out, err := ops["channelStats"].Do(t.Context(), Input{Args: map[string]string{"channel": ytTestChannel}})
	if err != nil {
		t.Fatal(err)
	}
	q := seen.query.Load().(url.Values)
	if q.Get("id") != ytTestChannel || q.Get("forHandle") != "" || q.Get("part") != "snippet,statistics" {
		t.Fatalf("query %v", q)
	}
	want := Row{Title: "Google for Developers", Subtitle: "動画 6,543 本・再生 123,456,789 回", Value: "登録者 2,440,000 人", URL: "https://www.youtube.com/channel/" + ytTestChannel}
	if out.Item == nil || *out.Item != want || len(out.Rows) != 0 || out.Usage.Priced {
		t.Fatalf("output %+v %+v", out, out.Item)
	}

	// an @handle goes as forHandle
	if _, err := ops["channelStats"].Do(t.Context(), Input{Args: map[string]string{"channel": "@GoogleDevelopers"}}); err != nil {
		t.Fatal(err)
	}
	if q := seen.query.Load().(url.Values); q.Get("forHandle") != "@GoogleDevelopers" || q.Get("id") != "" {
		t.Fatalf("handle query %v", q)
	}

	// a hidden count stays hidden
	out, err = ops["channelStats"].Do(t.Context(), Input{Args: map[string]string{"channel": "@hidden"}})
	if err != nil || out.Item == nil || out.Item.Value != "登録者 非公開" || out.Item.Subtitle != "動画 1 本・再生 10 回" {
		t.Fatalf("hidden: %+v %v", out.Item, err)
	}

	// no such channel is the caller's argument (bad_request)
	if _, err := ops["channelStats"].Do(t.Context(), Input{Args: map[string]string{"channel": "@nobody"}}); Reason(err) != ReasonBadRequest {
		t.Fatalf("nobody: %s (%v)", Reason(err), err)
	}
}

func TestYouTubeLatestVideos(t *testing.T) {
	ops, _, seen := ytOps(t)
	out, err := ops["latestVideos"].Do(t.Context(), Input{Args: map[string]string{"channel": "@GoogleDevelopers", "limit": "2"}})
	if err != nil {
		t.Fatal(err)
	}
	if q := seen.query.Load().(url.Values); q.Get("playlistId") != ytTestUploads || q.Get("maxResults") != "7" {
		t.Fatalf("query %v", q)
	}
	// private uploads are skipped; a bad date leaves the subtitle empty
	want := []Row{
		{Title: "新しい動画", Subtitle: "2026/11/10 12:04", URL: "https://www.youtube.com/watch?v=abcdefghijk"},
		{Title: "2 本目", URL: "https://www.youtube.com/watch?v=ABCDEFGHIJK"},
	}
	if len(out.Rows) != len(want) {
		t.Fatalf("rows %+v", out.Rows)
	}
	for i := range want {
		if out.Rows[i] != want[i] {
			t.Fatalf("row %d = %+v, want %+v", i, out.Rows[i], want[i])
		}
	}
	out, err = ops["latestVideos"].Do(t.Context(), Input{Args: map[string]string{"channel": ytTestChannel}})
	if err != nil || len(out.Rows) != 3 || seen.query.Load().(url.Values).Get("maxResults") != "10" {
		t.Fatalf("default limit: %+v %v", out, err)
	}
}

func TestYouTubeLatestVideosWithoutUploads(t *testing.T) {
	ops, _, _ := ytOps(t)
	// a malformed uploads id is the provider's fault
	if _, err := ops["latestVideos"].Do(t.Context(), Input{Args: map[string]string{"channel": "@empty"}}); Reason(err) != ReasonUpstream {
		t.Fatalf("empty: %s (%v)", Reason(err), err)
	}
}

func TestYouTubeUpstreamErrorHidesKey(t *testing.T) {
	ops, status, _ := ytOps(t)
	status.Store(403)
	_, err := ops["channelStats"].Do(t.Context(), Input{Args: map[string]string{"channel": ytTestChannel}})
	if err == nil || strings.Contains(err.Error(), ytTestKey) || Reason(err) != ReasonUpstream {
		t.Fatalf("403: %s (%v)", Reason(err), err)
	}
}

func TestYouTubeRefusesBadArgs(t *testing.T) {
	ops, _, seen := ytOps(t)
	p, _ := Lookup(YouTube)
	for _, a := range []map[string]string{
		{},
		{"channel": "GoogleDevelopers"},
		{"channel": "@ab"},
		{"channel": "@has space"},
		{"channel": "UCshort"},
		{"channel": "https://www.youtube.com/@GoogleDevelopers"},
		{"channel": ytTestChannel, "limit": "0"},
		{"channel": ytTestChannel, "limit": "21"},
		{"channel": ytTestChannel, "q": "x"},
	} {
		if _, err := p.CheckArgs("latestVideos", a); Reason(err) != ReasonBadRequest {
			t.Fatalf("check %v: %s (%v)", a, Reason(err), err)
		}
		if _, ok := a["q"]; ok {
			continue // only CheckArgs refuses unknown names
		}
		if _, err := ops["latestVideos"].Do(t.Context(), Input{Args: a}); Reason(err) != ReasonBadRequest {
			t.Fatalf("do %v: %s (%v)", a, Reason(err), err)
		}
	}
	if _, err := p.CheckArgs("channelStats", map[string]string{"channel": "@日本語のハンドル"}); err != nil {
		t.Fatalf("unicode handle: %v", err)
	}
	if seen.calls.Load() != 0 {
		t.Fatal("a bad argument reached the API")
	}
}

func TestYouTubeCount(t *testing.T) {
	for in, want := range map[string]string{"0": "0", "999": "999", "1000": "1,000", "2440000": "2,440,000"} {
		if got, ok := ytCount(in); !ok || got != want {
			t.Fatalf("%s → %q", in, got)
		}
	}
	for _, in := range []string{"", "-1", "1.5", "x"} {
		if _, ok := ytCount(in); ok {
			t.Fatalf("%q accepted", in)
		}
	}
}

func TestYouTubeDefinition(t *testing.T) {
	p, ok := Lookup(YouTube)
	if !ok || p.Billing != BillingFree || p.NeedsOAuth || p.Fake || p.EnvName() != "YOUTUBE" {
		t.Fatalf("%+v", p)
	}
	if len(p.Hosts) != 1 || p.Hosts[0] != youTubeHost {
		t.Fatalf("hosts %v", p.Hosts)
	}
	// cached well inside the 30-day limit of the Developer Policies
	for _, o := range p.Ops {
		if o.Write || o.CacheTTL <= 0 || o.CacheTTL > time.Hour {
			t.Fatalf("op %s cache %v", o.Name, o.CacheTTL)
		}
	}
	for _, k := range []string{"", "short", "has space 0123456789abcdef"} {
		if _, err := p.New(Secrets{APIKey: k}); err == nil {
			t.Fatalf("key %q accepted", k)
		}
	}
	if _, err := p.New(Secrets{APIKey: ytTestKey}); err != nil {
		t.Fatal(err)
	}
	// default off
	c, err := ConfigFromEnv(func(k string) string {
		if k == "EXT_YOUTUBE_API_KEY" {
			return ytTestKey
		}
		return ""
	})
	if err != nil || c.Enabled[YouTube] {
		t.Fatalf("enabled without EXT_YOUTUBE_ENABLED: %v", err)
	}
}
