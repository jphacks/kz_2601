package appconfig

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/metrics"
)

// captureMetrics swaps slog.Default (not parallel-safe: no t.Parallel) and
// returns the metric events logged by fn.
func captureMetrics(t *testing.T, fn func()) []map[string]any {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(prev)
	fn()
	var out []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) != nil {
			continue
		}
		switch m["event"] {
		case metrics.SessionStart, metrics.AppOpened, metrics.AppShared, metrics.AppPublished, metrics.ShareImported:
			out = append(out, m)
		}
	}
	return out
}

func TestProductMetricEvents(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/OWNER-KAKEIBO"
	e.ownApp("owner", app, kakeibo(t))

	var tok string
	evs := captureMetrics(t, func() {
		e.do("owner", http.MethodGet, "/v1/apps", nil, nil)
		e.do("owner", http.MethodGet, "/v1/apps?pageToken="+base64.RawURLEncoding.EncodeToString([]byte("1")), nil, nil) // later page: no session.start
		e.do("owner", http.MethodGet, app+"/dsl", nil, nil)
		e.do("owner", http.MethodGet, app+"/dsl", nil, map[string]string{"If-None-Match": `"1"`}) // background refresh: no app.opened
		r := e.do("owner", http.MethodPost, app+"/share", nil, nil)
		tok, _ = r.body["token"].(string)
		if r = e.do("friend", http.MethodPost, "/v1/shares/"+tok+"/import", nil, nil); r.code != http.StatusCreated {
			t.Fatalf("import: %d %s", r.code, r.raw)
		}
		if r = e.do("owner", http.MethodPost, app+"/publish", `{}`, nil); r.code != http.StatusOK {
			t.Fatalf("publish: %d %s", r.code, r.raw)
		}
		if r = e.do("finder", http.MethodPost, "/v1/public-apps/OWNER-KAKEIBO/install", nil, nil); r.code != http.StatusCreated {
			t.Fatalf("install: %d %s", r.code, r.raw)
		}
		e.do("other", http.MethodPost, app+"/share", nil, nil) // 404: no event
	})

	owner, appHash := metrics.UIDHash("owner"), metrics.AppHash("OWNER-KAKEIBO")
	want := []struct{ event, uid, app, source string }{
		{metrics.SessionStart, owner, "", ""},
		{metrics.AppOpened, owner, appHash, ""},
		{metrics.AppShared, owner, appHash, ""},
		{metrics.ShareImported, metrics.UIDHash("friend"), "*", "share"},
		{metrics.AppPublished, owner, appHash, ""},
		{metrics.ShareImported, metrics.UIDHash("finder"), "*", "public"},
	}
	if len(evs) != len(want) {
		t.Fatalf("want %d events, got %d: %v", len(want), len(evs), evs)
	}
	for i, w := range want {
		m := evs[i]
		if m["event"] != w.event || m["uid_hash"] != w.uid {
			t.Errorf("#%d: %v, want %s by %s", i, m, w.event, w.uid)
		}
		switch got, _ := m["app_hash"].(string); w.app {
		case "":
			if got != "" {
				t.Errorf("#%d: unexpected app_hash %v", i, m)
			}
		case "*":
			if len(got) != 16 {
				t.Errorf("#%d: app_hash missing: %v", i, m)
			}
		default:
			if got != w.app {
				t.Errorf("#%d: app_hash %q, want %q", i, got, w.app)
			}
		}
		if w.source != "" && (m["source"] != w.source || m["created"] != true) {
			t.Errorf("#%d: %v, want source %s created", i, m, w.source)
		}
	}
	// no raw uid / appId / token / app name / DSL in the metric lines
	var lines []string
	for _, m := range evs {
		b, _ := json.Marshal(m)
		lines = append(lines, string(b))
	}
	all := strings.Join(lines, "\n")
	for _, leak := range []string{`"owner"`, `"friend"`, "OWNER-KAKEIBO", tok, "家計簿", "entities"} {
		if strings.Contains(all, leak) {
			t.Errorf("metric events leak %q: %s", leak, all)
		}
	}
}
