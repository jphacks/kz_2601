package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// fakeLogging serves fixed pages by token ("" is the first) and records the
// requests.
type fakeLogging struct {
	pages map[string]LoggingPage
	reqs  []LoggingListRequest
	err   error
}

func (f *fakeLogging) ListEntries(_ context.Context, req LoggingListRequest) (LoggingPage, error) {
	f.reqs = append(f.reqs, req)
	if f.err != nil {
		return LoggingPage{}, f.err
	}
	p, ok := f.pages[req.PageToken]
	if !ok {
		return LoggingPage{}, fmt.Errorf("unknown token %q", req.PageToken)
	}
	return p, nil
}

func genEntry(at, outcome string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"timestamp":%q,"jsonPayload":{"event":"ai.generate","outcome":%q,"uid_hash":"u_1"}}`, at, outcome))
}

func TestGenLogLoggingPages(t *testing.T) {
	since := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	until := since.Add(24 * time.Hour)
	f := &fakeLogging{pages: map[string]LoggingPage{
		"":   {Entries: []json.RawMessage{genEntry("2026-11-10T03:00:00Z", "failed"), genEntry("2026-11-10T01:00:00Z", "validated")}, NextPageToken: "p2"},
		"p2": {Entries: []json.RawMessage{json.RawMessage(`{"timestamp":"2026-11-10T02:00:00Z","jsonPayload":{"event":"ai.chat"}}`), genEntry("2026-11-10T02:00:00Z", "refused")}},
	}}
	g := GenLogLogging{Client: f, Project: "proj", Services: []string{"ai"}, PageSize: 2}
	evs, err := g.GenerateEvents(context.Background(), since, until)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range evs {
		got = append(got, e.At.Format("15")+":"+e.Outcome)
	}
	if strings.Join(got, ",") != "01:validated,02:refused,03:failed" {
		t.Fatalf("events %v", got)
	}
	if len(f.reqs) != 2 || f.reqs[0].PageToken != "" || f.reqs[1].PageToken != "p2" {
		t.Fatalf("requests %+v", f.reqs)
	}
	want, _ := GenFailFilter{Since: since, Until: until, Services: []string{"ai"}}.Build()
	for _, r := range f.reqs {
		if r.Project != "proj" || r.PageSize != 2 || r.Filter != want {
			t.Fatalf("request %+v", r)
		}
	}
}

func TestGenLogLoggingLimits(t *testing.T) {
	since := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	ctx := context.Background()
	page := LoggingPage{Entries: []json.RawMessage{genEntry("2026-11-10T01:00:00Z", "failed"), genEntry("2026-11-10T01:00:01Z", "failed")}, NextPageToken: "next"}
	f := &fakeLogging{pages: map[string]LoggingPage{"": page, "next": {Entries: page.Entries}}}
	// 4 lines over a cap of 3: refused, not cut off
	if _, err := (GenLogLogging{Client: f, Project: "p", MaxEntries: 3}).GenerateEvents(ctx, since, since.Add(time.Hour)); !errors.Is(err, ErrGenFailTooMany) {
		t.Fatalf("over the cap: %v", err)
	}
	// exactly the cap is fine; a page size over the API cap is lowered
	f.reqs = nil
	evs, err := GenLogLogging{Client: f, Project: "p", MaxEntries: 4, PageSize: 5000}.GenerateEvents(ctx, since, since.Add(2*time.Hour))
	if err != nil || len(evs) != 4 || f.reqs[0].PageSize != DefaultGenFailPageSize {
		t.Fatalf("at the cap: %d %v %+v", len(evs), err, f.reqs)
	}
	// a token that does not move stops instead of looping
	stuck := &fakeLogging{pages: map[string]LoggingPage{"": {NextPageToken: "x"}, "x": {NextPageToken: "x"}}}
	if _, err := (GenLogLogging{Client: stuck, Project: "p"}).GenerateEvents(ctx, since, since.Add(time.Hour)); err == nil {
		t.Fatal("stuck token accepted")
	}
	// API errors and a bad range surface
	if _, err := (GenLogLogging{Client: &fakeLogging{err: errors.New("403")}, Project: "p"}).GenerateEvents(ctx, since, since.Add(time.Hour)); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("api error: %v", err)
	}
	if _, err := (GenLogLogging{Client: f, Project: "p"}).GenerateEvents(ctx, since, since.Add(-time.Hour)); err == nil {
		t.Fatal("reversed range accepted")
	}
}

// The fields asked of the API are the decoder's, under jsonPayload.
func TestLoggingEntryFields(t *testing.T) {
	got := string(loggingEntryFields())
	if !strings.HasPrefix(got, "nextPageToken,entries(timestamp,jsonPayload/time,") || strings.Contains(got, "uid") || strings.Contains(got, "app") {
		t.Fatal(got)
	}
}

func TestGenFailConfigFromEnvLogging(t *testing.T) {
	on := func(kv ...string) env {
		e := env{EnvGenFailOn: "on"}
		for i := 0; i+1 < len(kv); i += 2 {
			e[kv[i]] = kv[i+1]
		}
		return e
	}
	// the files stay the default
	g, err := GenFailConfigFromEnv(on(EnvGenFailSource, "file", EnvGenFailLogs, "a").get)
	if _, ok := g.Logs.(GenLogFiles); err != nil || !ok {
		t.Fatalf("file: %T %v", g.Logs, err)
	}
	g, err = GenFailConfigFromEnv(on(EnvGenFailSource, "logging", EnvGenFailProject, "proj", EnvGenFailServices, "b, a", EnvGenFailMaxEntries, "10").get)
	if err != nil {
		t.Fatal(err)
	}
	l, ok := g.Logs.(GenLogLogging)
	if !ok || l.Project != "proj" || strings.Join(l.Services, ",") != "a,b" || l.MaxEntries != 10 || l.Client == nil {
		t.Fatalf("logging: %+v", g.Logs)
	}
	for name, e := range map[string]env{
		EnvGenFailProject:    on(EnvGenFailSource, "logging"),
		EnvGenFailMaxEntries: on(EnvGenFailSource, "logging", EnvGenFailProject, "p", EnvGenFailMaxEntries, "0"),
		EnvGenFailSource:     on(EnvGenFailSource, "bigquery", EnvGenFailLogs, "a"),
	} {
		if _, err := GenFailConfigFromEnv(e.get); err == nil || !strings.Contains(err.Error(), name) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
