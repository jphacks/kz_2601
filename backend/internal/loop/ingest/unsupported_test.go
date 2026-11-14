package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
)

var unsupportedNow = time.Date(2026, 11, 13, 20, 0, 0, 0, time.UTC) // the 13th (UTC) is not finished

func TestParseUnsupportedLogsFromExport(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "unsupported", "export.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	evs, err := ParseUnsupportedLogs(f)
	if err != nil {
		t.Fatal(err)
	}
	// 7 records: one ai.generate and one with an empty phrase are skipped
	if len(evs) != 5 {
		t.Fatalf("want 5 events, got %d: %+v", len(evs), evs)
	}
	if evs[0].Requested != "カレンダー表示" || evs[0].Substitute != "日付順の一覧" || evs[0].Platform != "ios" || evs[0].Mode != "create" || evs[0].AppHash != "a_1" {
		t.Fatalf("first %+v", evs[0])
	}
	// bare slog lines decode too; a label that is not plain is dropped
	lines := `{"time":"2026-11-12T06:00:00Z","level":"INFO","msg":"ai.unsupported","event":"ai.unsupported","requested":"順位","platform":"iOS 26 (secret)","app_hash":"a_1"}
not json
{"time":"2026-11-12T06:10:00Z","event":"ai.chat","requested":"x"}
`
	evs, err = ParseUnsupportedLogs(strings.NewReader(lines))
	if err != nil || len(evs) != 1 || evs[0].Requested != "順位" || evs[0].Platform != "" {
		t.Fatalf("lines: %+v %v", evs, err)
	}
}

func TestUnsupportedFetchGroupsFinishedDays(t *testing.T) {
	ctx := context.Background()
	u := Unsupported{Logs: UnsupportedLogFiles{Paths: []string{filepath.Join("testdata", "unsupported", "export.json")}}, Now: func() time.Time { return unsupportedNow }}
	raws, err := u.Fetch(ctx, unsupportedNow.Add(-loop.Retention))
	if err != nil {
		t.Fatal(err)
	}
	// the 12th: two spellings of カレンダー表示 are one group (2 lines, 2 apps, ios+android), the 送金 line is another;
	// the 13th is not finished, so nothing of it yet
	if len(raws) != 2 {
		t.Fatalf("want 2 items, got %d: %+v", len(raws), raws)
	}
	cal := raws[0]
	if cal.Source != loop.SourceUnsupported || cal.Count != 2 || cal.Kind != "unsupported" || !strings.HasSuffix(cal.SourceID, "@2026-11-12") {
		t.Fatalf("calendar raw %+v", cal)
	}
	for _, want := range []string{"「カレンダー表示」", "日付順の一覧 / 日付順のリスト で代替", "2 件", "2 つのアプリ", "android 1 / ios 1"} {
		if !strings.Contains(cal.Text, want) {
			t.Errorf("text lacks %q: %s", want, cal.Text)
		}
	}
	if strings.Contains(cal.Text, "2026-11-12") {
		t.Errorf("text holds the date (masked as a number): %s", cal.Text)
	}
	if cal.CreatedAt != time.Date(2026, 11, 12, 2, 0, 0, 0, time.UTC) {
		t.Errorf("createdAt %v", cal.CreatedAt)
	}
	// no hash reaches an item; the masked phrase is masked again at ingest
	for _, r := range raws {
		if strings.Contains(r.Text, "a_1") || strings.Contains(r.Text, "u_secret") {
			t.Errorf("item holds a hash: %s", r.Text)
		}
	}
	policy := Policy{AISources: map[loop.Source]bool{loop.SourceUnsupported: true}}
	if it := Normalize(raws[0], policy, unsupportedNow); it.Status != loop.ItemNew || !it.AIAllowed {
		t.Fatalf("calendar item held: %+v", it)
	}
	// a phrase with personal data is masked again at ingest (the writer masked it too)
	it := Normalize(raws[1], policy, unsupportedNow)
	if strings.Contains(it.Text, "090-1234") || strings.Contains(it.Text, "田中") || !strings.Contains(it.Text, "[電話]") || !strings.Contains(it.Text, "代替なし") {
		t.Fatalf("normalised %+v", it)
	}

	// the next day both groups of the 13th come out; re-read from the start
	// of the 12th, its items keep the same ids (ingest is idempotent)
	u.Now = func() time.Time { return unsupportedNow.Add(24 * time.Hour) }
	again, err := u.Fetch(ctx, time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC))
	if err != nil || len(again) != 4 || again[0].SourceID != raws[0].SourceID {
		t.Fatalf("next day: %d %v %+v", len(again), err, again)
	}
	if !strings.Contains(again[2].Text, "「Calendar view」") || !strings.Contains(again[2].Text, "macos 1") {
		t.Fatalf("en group: %s", again[2].Text)
	}
}

func TestUnsupportedConfigFromEnv(t *testing.T) {
	get := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if _, err := UnsupportedConfigFromEnv(get(nil)); err != ErrUnsupportedOff {
		t.Fatalf("off: %v", err)
	}
	if _, err := UnsupportedConfigFromEnv(get(map[string]string{EnvUnsupportedOn: "on"})); err == nil || !strings.Contains(err.Error(), EnvUnsupportedLogs) {
		t.Fatalf("on without logs: %v", err)
	}
	u, err := UnsupportedConfigFromEnv(get(map[string]string{EnvUnsupportedOn: "on", EnvUnsupportedLogs: "b.json, a.json"}))
	if err != nil || fmt.Sprint(u.Logs.(UnsupportedLogFiles).Paths) != "[a.json b.json]" {
		t.Fatalf("files: %+v %v", u, err)
	}
	u, err = UnsupportedConfigFromEnv(get(map[string]string{EnvUnsupportedOn: "on", EnvGenFailSource: "logging", "GOOGLE_CLOUD_PROJECT": "p", "APP_ENV": "prod", EnvGenFailMaxLines: "7"}))
	api, ok := u.Logs.(GenLogAPI)
	if err != nil || !ok || api.Project != "p" || fmt.Sprint(api.Services) != "[ai-prod]" || api.MaxEntries != 7 {
		t.Fatalf("logging: %+v %v", u, err)
	}
}

func TestGenLogAPIUnsupportedEvents(t *testing.T) {
	ctx := context.Background()
	entries := []json.RawMessage{
		json.RawMessage(`{"timestamp": "2026-11-12T03:00:00Z", "jsonPayload": {"event": "ai.unsupported", "requested": "順位", "substitute": "降順の表", "platform": "ios", "app_hash": "a_1", "uid_hash": "u_secret"}}`),
		json.RawMessage(`{"timestamp": "2026-11-12T02:00:00Z", "jsonPayload": {"event": "ai.unsupported", "requested": "順位", "app_hash": "a_2"}}`),
	}
	f := &fakeLister{entries: entries}
	api := GenLogAPI{Project: "p", Services: []string{"ai-prod"}, Lister: f}
	since, until := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	evs, err := api.UnsupportedEvents(ctx, since, until)
	if err != nil || len(evs) != 2 || evs[0].At.Hour() != 2 || evs[1].Substitute != "降順の表" {
		t.Fatalf("%+v %v", evs, err)
	}
	if len(f.reqs) != 1 || !strings.Contains(f.reqs[0].Filter, `jsonPayload.event="ai.unsupported"`) || !strings.Contains(f.reqs[0].Filter, `"ai-prod"`) {
		t.Fatalf("filter %+v", f.reqs)
	}
}
