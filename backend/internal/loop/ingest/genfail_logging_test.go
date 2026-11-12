package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/option"
)

// fakeLister serves entries newest first in pages of the requested size.
type fakeLister struct {
	entries []json.RawMessage
	reqs    []LogListRequest
	err     error
}

func (f *fakeLister) ListEntries(_ context.Context, req LogListRequest) (LogListPage, error) {
	f.reqs = append(f.reqs, req)
	if f.err != nil {
		return LogListPage{}, f.err
	}
	start := 0
	if req.PageToken != "" {
		_, _ = fmt.Sscanf(req.PageToken, "p%d", &start)
	}
	end := min(start+req.PageSize, len(f.entries))
	page := LogListPage{Entries: f.entries[start:end]}
	if end < len(f.entries) {
		page.Next = fmt.Sprintf("p%d", end)
	}
	return page, nil
}

// genEntries makes n ai.generate entries, one a minute back from end; the
// first `fail` of them fail. Each carries a uid_hash and app_hash.
func genEntries(end time.Time, n, fail int) []json.RawMessage {
	out := make([]json.RawMessage, 0, n)
	for i := 0; i < n; i++ {
		p := `"outcome": "validated"`
		if i < fail {
			p = `"outcome": "failed", "error_code": "generation_failed"`
		}
		out = append(out, json.RawMessage(fmt.Sprintf(`{"timestamp": %q, "jsonPayload": {"event": "ai.generate", %s, "uid_hash": "u_secret", "app_hash": "a_secret"}}`,
			end.Add(-time.Duration(i+1)*time.Minute).Format(time.RFC3339), p)))
	}
	return out
}

func TestGenLogAPIPagesAndFilter(t *testing.T) {
	ctx := context.Background()
	f := &fakeLister{entries: genEntries(genEnd, 25, 10)}
	// a stray line of another event and a broken line are skipped
	f.entries = append(f.entries, json.RawMessage(`{"timestamp": "2026-11-10T00:00:00Z", "jsonPayload": {"event": "ai.chat"}}`), json.RawMessage(`{`))
	api := GenLogAPI{Project: "proj", Services: []string{"ai-prod"}, Lister: f, PageSize: 10}
	since := genEnd.Add(-48 * time.Hour)
	evs, err := api.GenerateEvents(ctx, since, genEnd)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 25 || len(f.reqs) != 3 {
		t.Fatalf("events %d, calls %d", len(evs), len(f.reqs))
	}
	for i := 1; i < len(evs); i++ {
		if evs[i].At.Before(evs[i-1].At) {
			t.Fatal("events not sorted oldest first")
		}
	}
	r := f.reqs[0]
	if len(r.ResourceNames) != 1 || r.ResourceNames[0] != "projects/proj" || r.OrderBy != "timestamp desc" || r.PageSize != 10 || r.PageToken != "" {
		t.Fatalf("request %+v", r)
	}
	for _, want := range []string{`resource.labels.service_name=("ai-prod")`, `jsonPayload.event="ai.generate"`,
		`timestamp>="2026-11-08T03:00:00Z"`, `timestamp<="2026-11-10T03:00:00Z"`} {
		if !strings.Contains(r.Filter, want) {
			t.Errorf("filter %q lacks %q", r.Filter, want)
		}
	}
	if f.reqs[1].PageToken != "p10" || f.reqs[2].PageToken != "p20" {
		t.Fatalf("page tokens %q %q", f.reqs[1].PageToken, f.reqs[2].PageToken)
	}
	// end to end: the spike comes out with codes and counts only
	raws, err := GenFailures{Logs: api, Threshold: GenFailThreshold{Window: time.Hour, Baseline: time.Hour}, Now: func() time.Time { return genEnd }}.
		Fetch(ctx, genEnd.Add(-time.Hour))
	if err != nil || len(raws) == 0 {
		t.Fatalf("fetch %+v %v", raws, err)
	}
	for _, r := range raws {
		if strings.Contains(r.Text+r.SourceID, "secret") {
			t.Fatalf("hash leaked: %+v", r)
		}
	}
}

func TestGenLogAPICapAndErrors(t *testing.T) {
	ctx := context.Background()
	f := &fakeLister{entries: genEntries(genEnd, 50, 0)}
	api := GenLogAPI{Project: "p", Services: []string{"ai-stg"}, Lister: f, PageSize: 10, MaxEntries: 15}
	evs, err := api.GenerateEvents(ctx, genEnd.Add(-24*time.Hour), genEnd)
	if err != nil {
		t.Fatal(err)
	}
	// newest first: the cut keeps the 15 newest lines
	if len(evs) != 15 || len(f.reqs) != 2 || f.reqs[1].PageSize != 5 || !evs[0].At.Equal(genEnd.Add(-15*time.Minute)) {
		t.Fatalf("events %d calls %d %+v", len(evs), len(f.reqs), f.reqs)
	}
	boom := errors.New("status 403")
	if _, err := (GenLogAPI{Project: "p", Services: []string{"ai"}, Lister: &fakeLister{err: boom}}).GenerateEvents(ctx, time.Time{}, genEnd); !errors.Is(err, boom) {
		t.Fatalf("error %v", err)
	}
	if _, err := (GenLogAPI{Lister: f}).GenerateEvents(ctx, time.Time{}, genEnd); err == nil {
		t.Fatal("no project accepted")
	}
}

// TestLoggingListerHTTP drives the real client against a local server.
func TestLoggingListerHTTP(t *testing.T) {
	var got struct {
		ResourceNames []string `json:"resourceNames"`
		Filter        string   `json:"filter"`
		OrderBy       string   `json:"orderBy"`
		PageSize      int      `json:"pageSize"`
		PageToken     string   `json:"pageToken"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v2/entries:list" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Error(err)
		}
		if got.PageToken == "" {
			_, _ = fmt.Fprint(w, `{"entries": [{"timestamp": "2026-11-10T02:00:00Z", "insertId": "x", "jsonPayload": {"event": "ai.generate", "outcome": "failed", "uid_hash": "u_1"}}], "nextPageToken": "t2"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"entries": [{"timestamp": "2026-11-10T01:00:00Z", "jsonPayload": {"event": "ai.generate", "outcome": "validated"}}]}`)
	}))
	defer srv.Close()
	ctx := context.Background()
	l, err := NewLoggingLister(ctx, option.WithEndpoint(srv.URL+"/"), option.WithoutAuthentication(), option.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	evs, err := GenLogAPI{Project: "proj", Services: []string{"ai-prod"}, Lister: l}.GenerateEvents(ctx, genEnd.Add(-24*time.Hour), genEnd)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 || evs[1].Outcome != "failed" || evs[0].Outcome != "validated" {
		t.Fatalf("events %+v", evs)
	}
	if got.PageToken != "t2" || got.OrderBy != "timestamp desc" || got.PageSize != 1000 || len(got.ResourceNames) != 1 {
		t.Fatalf("request %+v", got)
	}

	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `{"error": {"code": 403, "message": "denied"}}`)
	}))
	defer denied.Close()
	l, _ = NewLoggingLister(ctx, option.WithEndpoint(denied.URL+"/"), option.WithoutAuthentication(), option.WithHTTPClient(denied.Client()))
	if _, err := l.ListEntries(ctx, LogListRequest{}); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("error %v", err)
	}
}

func TestGenFailConfigLogging(t *testing.T) {
	g, err := GenFailConfigFromEnv(env{EnvGenFailOn: "on", EnvGenFailSource: "logging", "GOOGLE_CLOUD_PROJECT": "proj", "APP_ENV": "prod"}.get)
	if err != nil {
		t.Fatal(err)
	}
	api, ok := g.Logs.(GenLogAPI)
	if !ok || api.Project != "proj" || len(api.Services) != 1 || api.Services[0] != "ai-prod" || api.MaxEntries != 0 || api.Lister != nil {
		t.Fatalf("logs %#v", g.Logs)
	}
	g, err = GenFailConfigFromEnv(env{EnvGenFailOn: "on", EnvGenFailSource: "logging", EnvGenFailProject: "other",
		"GOOGLE_CLOUD_PROJECT": "proj", EnvGenFailServices: "b, a", EnvGenFailMaxLines: "100"}.get)
	if err != nil {
		t.Fatal(err)
	}
	if api := g.Logs.(GenLogAPI); api.Project != "other" || strings.Join(api.Services, ",") != "a,b" || api.MaxEntries != 100 {
		t.Fatalf("logs %#v", api)
	}
	// the file reader stays the default
	if g, err := GenFailConfigFromEnv(env{EnvGenFailOn: "on", EnvGenFailLogs: "a.json"}.get); err != nil {
		t.Fatal(err)
	} else if _, ok := g.Logs.(GenLogFiles); !ok {
		t.Fatalf("default reader %#v", g.Logs)
	}
	for _, e := range []env{
		{EnvGenFailOn: "on", EnvGenFailSource: "bigquery", EnvGenFailLogs: "a"},
		{EnvGenFailOn: "on", EnvGenFailSource: "logging", "APP_ENV": "prod"},
		{EnvGenFailOn: "on", EnvGenFailSource: "logging", "GOOGLE_CLOUD_PROJECT": "p"},
		{EnvGenFailOn: "on", EnvGenFailSource: "logging", "GOOGLE_CLOUD_PROJECT": "p", "APP_ENV": "prod", EnvGenFailMaxLines: "0"},
	} {
		if _, err := GenFailConfigFromEnv(e.get); err == nil {
			t.Errorf("accepted %v", e)
		}
	}
	// still off without the switch, even with the source set
	if _, err := GenFailConfigFromEnv(env{EnvGenFailSource: "logging"}.get); !errors.Is(err, ErrGenFailOff) {
		t.Fatalf("off: %v", err)
	}
}
