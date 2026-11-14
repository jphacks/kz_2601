package feeds

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Fixtures are real responses recorded on 2026-11-12 (JST):
//   - status_cloudflare_summary.json: https://www.cloudflarestatus.com/api/v2/summary.json
//     cut to 7 components (2 groups), 2 incidents and 2 maintenances with
//     their first update and component only
//   - status_claude_summary.json: https://status.claude.com/api/v2/summary.json (whole)

// statusSource is a fake set of status pages; body overrides a path.
type statusSource struct {
	srv  *httptest.Server
	mu   sync.Mutex
	hits map[string]int
	body map[string][]byte
}

func newStatusSource(t *testing.T) *statusSource {
	s := &statusSource{hits: map[string]int{}, body: map[string][]byte{
		"/cf/api/v2/summary.json":     fixture(t, "status_cloudflare_summary.json"),
		"/claude/api/v2/summary.json": fixture(t, "status_claude_summary.json"),
	}}
	s.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.hits[r.URL.Path]++
		b, ok := s.body[r.URL.Path]
		s.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *statusSource) set(path string, b []byte) {
	s.mu.Lock()
	s.body[path] = b
	s.mu.Unlock()
}

func (s *statusSource) count(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[path]
}

func (s *statusSource) svc(c *clock, o Options) *Service {
	o.Client, o.Now = s.srv.Client(), c.now
	page := func(dir, name string) StatusPage {
		return StatusPage{Name: name, URL: s.srv.URL + "/" + dir + "/api/v2/summary.json", SiteURL: s.srv.URL + "/" + dir}
	}
	o.Endpoints = &Endpoints{Status: map[string]StatusPage{"cloudflare": page("cf", "Cloudflare"), "claude": page("claude", "Claude(Anthropic)")}}
	return New(o)
}

func rows(items []Item) []string {
	var out []string
	for _, it := range items {
		out = append(out, strings.Join([]string{it.Date, it.Icon, it.Title, it.Subtitle, it.URL}, " | "))
	}
	return out
}

func TestStatusCloudflare(t *testing.T) {
	src := newStatusSource(t)
	s := src.svc(&clock{t: testNow}, Options{})
	res, err := s.Get(context.Background(), Query{Provider: dsl.FeedStatus, FeedID: "cloudflare"}, 20)
	if err != nil {
		t.Fatal(err)
	}
	site := src.srv.URL + "/cf"
	if res.Title != "Cloudflareの稼働状況" || res.Source.Name != "Cloudflare の status ページ" || res.Source.URL != site ||
		!strings.Contains(res.Source.License, "承認や提携はありません") {
		t.Errorf("title/source = %q %+v", res.Title, res.Source)
	}
	// Overall, open incidents, then components not operational (groups
	// skipped), then upcoming maintenance. Update bodies are not shown.
	want := []string{
		"2026-11-12T02:06:09+09:00 | exclamationmark.circle | Cloudflare: 一部で低下 | Partially Degraded Service | " + site,
		"2026-11-11T13:33:19+09:00 | exclamationmark.circle | Replicate Input Upload Issue | 障害・原因を特定・影響 小 | " + site + "/incidents/yu63fhh2peb7",
		"2026-11-10T10:53:58+09:00 | exclamationmark.circle | Changes to Cloudflare Workers are not producing Event Subscription events | 障害・原因を特定・影響 小 | " + site + "/incidents/0ocqieaifg7h",
		" | exclamationmark.triangle | Albuquerque, United States - (ABQ) | 一部停止 | ",
		" | exclamationmark.triangle | Arica, Chile - (ARI) | 一部停止 | ",
		" | exclamationmark.triangle | Annaba, Algeria - (AAE) | 一部停止 | ",
		"2026-11-12T11:00:00+09:00 | wrench | CDG (Paris) on 2026-11-12 | メンテナンス・予定 | " + site + "/incidents/d9hqtlddolkz",
		"2026-11-12T15:00:00+09:00 | wrench | MSP (Minneapolis) on 2026-11-12 | メンテナンス・予定 | " + site + "/incidents/ozla8jeh1luu",
	}
	if got := rows(res.Items); !slices.Equal(got, want) {
		t.Fatalf("rows =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, it := range res.Items {
		if strings.Contains(it.Title+it.Subtitle, "We are continuing") {
			t.Errorf("update body shown: %+v", it)
		}
	}
	// Cached for the TTL: a second read does not reach the source.
	if _, err := s.Get(context.Background(), Query{Provider: dsl.FeedStatus, FeedID: "cloudflare"}, 3); err != nil {
		t.Fatal(err)
	}
	if n := src.count("/cf/api/v2/summary.json"); n != 1 {
		t.Errorf("source hits = %d, want 1", n)
	}
}

func TestStatusAllOperational(t *testing.T) {
	src := newStatusSource(t)
	s := src.svc(&clock{t: testNow}, Options{})
	res, err := s.Get(context.Background(), Query{Provider: dsl.FeedStatus, FeedID: "claude"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2026-11-12T01:59:29+09:00 | checkmark.circle | Claude(Anthropic): すべて正常 | All Systems Operational | " + src.srv.URL + "/claude"}
	if got := rows(res.Items); !slices.Equal(got, want) {
		t.Fatalf("rows = %q", got)
	}
}

func TestStatusMaintenanceAndOddValues(t *testing.T) {
	src := newStatusSource(t)
	src.set("/claude/api/v2/summary.json", []byte(`{
"page":{"updated_at":"not a time"},
"status":{"indicator":"maintenance","description":"Service Under Maintenance\u202e"},
"components":[
 {"name":"API","status":"under_maintenance","group":false},
 {"name":"Console","status":"operational","group":false},
 {"name":"Web\nApp","status":"some_new_state","group":false}],
"incidents":[
 {"id":"abc123","name":"Done","status":"resolved","impact":"major","created_at":"2026-11-11T00:00:00Z"},
 {"id":"../../evil","name":"Elevated errors","status":"investigating","impact":"critical","created_at":"2026-11-11T00:00:00Z"}],
"scheduled_maintenances":[
 {"id":"m1","name":"DB upgrade","status":"in_progress","impact":"maintenance","scheduled_for":"2026-11-12T00:00:00Z"},
 {"id":"m2","name":"Old","status":"completed","impact":"maintenance","scheduled_for":"2026-11-01T00:00:00Z"}]}`))
	s := src.svc(&clock{t: testNow}, Options{})
	res, err := s.Get(context.Background(), Query{Provider: dsl.FeedStatus, FeedID: "claude"}, 20)
	if err != nil {
		t.Fatal(err)
	}
	site := src.srv.URL + "/claude"
	want := []string{
		" | wrench | Claude(Anthropic): メンテナンス中 | Service Under Maintenance | " + site,
		"2026-11-11T09:00:00+09:00 | xmark.circle | Elevated errors | 障害・調査中・影響 重大 | ", // id not a Statuspage id: no link
		"2026-11-12T09:00:00+09:00 | wrench | DB upgrade | メンテナンス・実施中 | " + site + "/incidents/m1",
		" | wrench | API | メンテナンス中 | ",
	}
	if got := rows(res.Items); !slices.Equal(got, want) {
		t.Fatalf("rows =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestStatusRejects(t *testing.T) {
	src := newStatusSource(t)
	c := &clock{t: testNow}
	s := src.svc(c, Options{})
	ctx := context.Background()
	for _, q := range []Query{
		{Provider: dsl.FeedStatus},
		{Provider: dsl.FeedStatus, FeedID: "openai"},
		{Provider: dsl.FeedStatus, FeedID: "slack"},
		{Provider: dsl.FeedStatus, FeedID: "https://status.openai.com/api/v2/summary.json"},
		{Provider: dsl.FeedStatus, FeedID: "claude", Region: "130000"},
		{Provider: dsl.FeedStatus, FeedID: "claude", Q: "x"},
	} {
		if _, err := s.Get(ctx, q, 5); !errors.Is(err, ErrUnknown) {
			t.Errorf("%+v: err = %v, want ErrUnknown", q, err)
		}
	}
	// A page that is not a summary (an HTML error page, an empty object)
	// fails, is remembered for FailTTL, and is retried after it.
	src.set("/cf/api/v2/summary.json", []byte(`<html>maintenance</html>`))
	q := Query{Provider: dsl.FeedStatus, FeedID: "cloudflare"}
	if _, err := s.Get(ctx, q, 5); !errors.Is(err, ErrUpstream) {
		t.Errorf("html: err = %v", err)
	}
	if _, err := s.Get(ctx, q, 5); !errors.Is(err, ErrRecentFailure) {
		t.Errorf("cached failure: err = %v", err)
	}
	src.set("/cf/api/v2/summary.json", []byte(`{}`))
	c.add(FailTTL + time.Second)
	if _, err := s.Get(ctx, q, 5); !errors.Is(err, ErrUpstream) {
		t.Errorf("empty: err = %v", err)
	}
	if n := src.count("/cf/api/v2/summary.json"); n != 2 {
		t.Errorf("source hits = %d, want 2", n)
	}
	// Over the body cap.
	src.set("/claude/api/v2/summary.json", []byte(`{"status":{"indicator":"none"},"x":"`+strings.Repeat("a", int(providerLimits[dsl.FeedStatus].maxBody))+`"}`))
	if _, err := s.Get(ctx, Query{Provider: dsl.FeedStatus, FeedID: "claude"}, 5); !errors.Is(err, ErrUpstream) {
		t.Errorf("too large: err = %v", err)
	}
}

func TestStatusDisabled(t *testing.T) {
	src := newStatusSource(t)
	s := src.svc(&clock{t: testNow}, OptionsFromEnv(func(k string) string {
		return map[string]string{"FEEDS_DISABLED": "status"}[k]
	}))
	if _, err := s.Get(context.Background(), Query{Provider: dsl.FeedStatus, FeedID: "claude"}, 5); !errors.Is(err, ErrDisabled) {
		t.Errorf("err = %v, want ErrDisabled", err)
	}
	if n := src.count("/claude/api/v2/summary.json"); n != 0 {
		t.Errorf("source hits = %d", n)
	}
}

// The status allowlist matches the ids the DSL accepts, and every page is a
// fixed https summary.json under its own site.
func TestStatusAllowlistMatchesDSL(t *testing.T) {
	ep := DefaultEndpoints()
	var ids []string
	for id, p := range ep.Status {
		ids = append(ids, id)
		if dsl.StatusPages[id] != p.Name {
			t.Errorf("status %s name %q != dsl %q", id, p.Name, dsl.StatusPages[id])
		}
		if !strings.HasPrefix(p.SiteURL, "https://") || p.URL != p.SiteURL+"/api/v2/summary.json" || !ep.allowedURL(p.URL) {
			t.Errorf("status %s: %+v", id, p)
		}
	}
	slices.Sort(ids)
	if want := slices.Sorted(maps.Keys(dsl.StatusPages)); !slices.Equal(ids, want) {
		t.Errorf("status ids %v != dsl %v", ids, want)
	}
}
