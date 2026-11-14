package ingest

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
)

// Hand-written fixtures in the shape of the App Store Connect betaFeedback
// responses (JSON:API). Not real data. The server returns email and the
// tester even though we never ask for them, to prove they are dropped.
const tfScreenshotPage1 = `{
  "data": [{
    "type": "betaFeedbackScreenshotSubmissions", "id": "shot-1",
    "attributes": {"createdDate": "2026-11-03T09:00:00Z", "comment": "保存ボタンが押せない。連絡は tester@example.com まで",
      "email": "tester@example.com", "deviceModel": "iPhone17,1", "osVersion": "26.1.2", "locale": "ja_JP", "timeZone": "Asia/Tokyo",
      "screenshots": [{"url": "https://example.invalid/shot.png", "width": 1179, "height": 2556}]},
    "relationships": {"build": {"data": {"type": "builds", "id": "b-120"}}, "tester": {"data": {"type": "betaTesters", "id": "t-1"}}}
  }],
  "included": [
    {"type": "builds", "id": "b-120", "attributes": {"version": "120"}},
    {"type": "betaTesters", "id": "t-1", "attributes": {"firstName": "Taro", "lastName": "Tester", "email": "tester@example.com"}}
  ],
  "links": {"next": "%s/v1/apps/123/betaFeedbackScreenshotSubmissions?cursor=2"}
}`

const tfScreenshotPage2 = `{
  "data": [
    {"type": "betaFeedbackScreenshotSubmissions", "id": "shot-2",
     "attributes": {"createdDate": "2026-11-01T01:00:00Z", "comment": "", "osVersion": "26.0"},
     "relationships": {"build": {"data": {"type": "builds", "id": "b-119"}}}},
    {"type": "betaFeedbackScreenshotSubmissions", "id": "shot-old",
     "attributes": {"createdDate": "2026-10-30T00:00:00Z", "comment": "old"}}
  ],
  "included": [{"type": "builds", "id": "b-119", "attributes": {"version": "119"}}],
  "links": {}
}`

const tfCrashPage = `{
  "data": [{
    "type": "betaFeedbackCrashSubmissions", "id": "crash-1",
    "attributes": {"createdDate": "2026-11-02T12:00:00Z", "comment": "", "email": "other@example.com", "osVersion": "26.1",
      "deviceModel": "iPhone16,2"},
    "relationships": {"build": {"data": {"type": "builds", "id": "b-120"}}}
  }],
  "included": [{"type": "builds", "id": "b-120", "attributes": {"version": "120"}}],
  "links": {}
}`

func TestTestFlightFeedback(t *testing.T) {
	key, pemKey := testKey(t)
	since := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	var srv *httptest.Server
	paths := []string{}
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		verifyES256(t, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), &key.PublicKey)
		paths = append(paths, r.URL.Path)
		q := r.URL.Query()
		switch r.URL.Path {
		case "/v1/apps/123/betaFeedbackScreenshotSubmissions":
			if q.Get("cursor") == "" {
				checkTFQuery(t, q.Get("fields[betaFeedbackScreenshotSubmissions]"), q)
				_, _ = fmt.Fprintf(w, tfScreenshotPage1, srv.URL)
			} else {
				_, _ = fmt.Fprint(w, tfScreenshotPage2)
			}
		case "/v1/apps/123/betaFeedbackCrashSubmissions":
			checkTFQuery(t, q.Get("fields[betaFeedbackCrashSubmissions]"), q)
			_, _ = fmt.Fprint(w, tfCrashPage)
		default:
			t.Errorf("path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg, err := TestFlightConfigFromEnv(func(k string) string {
		return map[string]string{EnvTestFlightOn: "on", EnvAppStoreIssuer: "iss", EnvAppStoreKeyID: "kid", EnvAppStoreKey: pemKey, EnvAppStoreAppID: "123"}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	raws, err := TestFlightFeedback{Cfg: cfg, BaseURL: srv.URL}.Fetch(context.Background(), since)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 3 || len(raws) != 3 {
		t.Fatalf("paths %v raws %+v", paths, raws)
	}
	shot, empty, crash := raws[0], raws[1], raws[2]
	if shot.SourceID != "s:shot-1" || shot.Kind != "testflight-screenshot" || !shot.HasScreenshot ||
		shot.AppVersion != "build 120" || shot.OSMajor != "26" || shot.Source != loop.SourceTestFlight {
		t.Fatalf("shot %+v", shot)
	}
	if empty.SourceID != "s:shot-2" || empty.Text != "" || empty.AppVersion != "build 119" {
		t.Fatalf("empty %+v", empty)
	}
	if crash.SourceID != "c:crash-1" || crash.Kind != "testflight-crash" || crash.HasScreenshot || crash.Text == "" || crash.AppVersion != "build 120" {
		t.Fatalf("crash %+v", crash)
	}
	for _, r := range raws {
		if r.UID != "" || strings.Contains(r.Text, "Taro") || strings.Contains(r.Text, "other@example.com") ||
			strings.Contains(r.AppVersion+r.OSMajor+r.Kind, "iPhone") {
			t.Fatalf("tester data kept: %+v", r)
		}
	}

	p := allowAll()
	p.AISources[loop.SourceTestFlight] = true
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	if it := Normalize(shot, p, now); strings.Contains(it.Text, "example.com") || it.MaskCounts == nil {
		t.Fatalf("comment not masked: %+v", it)
	}
	if it := Normalize(empty, p, now); it.HoldReason != loop.HoldEmpty {
		t.Fatalf("empty comment not held: %+v", it)
	}
	if it := Normalize(crash, PolicyFromEnv(func(string) string { return "" }), now); it.HoldReason != loop.HoldSource {
		t.Fatalf("testflight reached the AI by default: %+v", it)
	}
}

func checkTFQuery(t *testing.T, fields string, q map[string][]string) {
	t.Helper()
	if fields != tfFields || strings.Contains(fields, "email") || strings.Contains(fields, "tester") {
		t.Errorf("fields %q", fields)
	}
	if got := q["include"]; len(got) != 1 || got[0] != "build" {
		t.Errorf("include %v", got)
	}
	if got := q["sort"]; len(got) != 1 || got[0] != "-createdDate" {
		t.Errorf("sort %v", got)
	}
}

func TestTestFlightOffAndErrors(t *testing.T) {
	if _, err := TestFlightConfigFromEnv(func(string) string { return "" }); err != ErrTestFlightOff {
		t.Fatalf("off: %v", err)
	}
	// LOOP_SOURCE_APPSTORE alone does not turn TestFlight on.
	if _, err := TestFlightConfigFromEnv(func(k string) string { return map[string]string{EnvAppStoreOn: "on"}[k] }); err != ErrTestFlightOff {
		t.Fatalf("appstore switch leaked: %v", err)
	}
	if _, err := TestFlightConfigFromEnv(func(k string) string { return map[string]string{EnvTestFlightOn: "on"}[k] }); err == nil ||
		!strings.Contains(err.Error(), EnvTestFlightOn) {
		t.Fatalf("missing values: %v", err)
	}

	key, _ := testKey(t)
	cfg := AppStoreConfig{IssuerID: "i", KeyID: "k", Key: key, AppID: "1"}
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"data":[],"links":{"next":"https://elsewhere.example/v1/x"}}`)
	}))
	defer foreign.Close()
	if _, err := (TestFlightFeedback{Cfg: cfg, BaseURL: foreign.URL}).Fetch(context.Background(), time.Time{}); err == nil {
		t.Fatal("followed a foreign link")
	}
	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `{"errors":[{"detail":"tester@example.com"}]}`)
	}))
	defer denied.Close()
	_, err := TestFlightFeedback{Cfg: cfg, BaseURL: denied.URL}.Fetch(context.Background(), time.Time{})
	if err == nil || !strings.Contains(err.Error(), "status 403") || strings.Contains(err.Error(), "example.com") {
		t.Fatalf("error %v", err)
	}
}
