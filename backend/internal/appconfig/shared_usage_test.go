package appconfig

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/metrics"
)

// sharedUsageLines captures the shared.usage lines with the switch on.
func sharedUsageLines(t *testing.T) func() []map[string]any {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	was := metrics.SetSharedUsage(true)
	t.Cleanup(func() { slog.SetDefault(prev); metrics.SetSharedUsage(was) })
	return func() []map[string]any {
		var out []map[string]any
		for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
			var m map[string]any
			if json.Unmarshal([]byte(l), &m) == nil && m["event"] == metrics.SharedUsage {
				out = append(out, m)
			}
		}
		if s := buf.String(); strings.Contains(s, "SECRET-MEMO") || strings.Contains(s, "USAGE-APP-1") || strings.Contains(s, `"anon1"`) {
			t.Fatalf("body or id in the log: %s", s)
		}
		return out
	}
}

func TestSharedUsageCountsSharedRecords(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/USAGE-APP-1"
	recs := app + "/entities/expense/records"
	e.ownApp("owner", app, sharedKakeibo(t))
	lines := sharedUsageLines(t)

	r := e.do("anon1", http.MethodPost, recs, `{"data":{"amount":5,"memo":"SECRET-MEMO"}}`, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("post: %d %s", r.code, r.raw)
	}
	id := r.body["id"].(string)
	if r = e.do("anon1", http.MethodGet, recs, nil, nil); r.code != http.StatusOK {
		t.Fatalf("list: %d %s", r.code, r.raw)
	}
	if r = e.do("anon1", http.MethodPatch, recs+"/"+id, `{"data":{"amount":6}}`, nil); r.code != http.StatusOK {
		t.Fatalf("patch: %d %s", r.code, r.raw)
	}
	if r = e.do("anon1", http.MethodGet, recs+"/"+id, nil, nil); r.code != http.StatusOK {
		t.Fatalf("get: %d %s", r.code, r.raw)
	}
	if r = e.do("anon1", http.MethodDelete, recs+"/"+id, nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", r.code, r.raw)
	}
	got := lines()
	var kinds []string
	for _, m := range got {
		kinds = append(kinds, m["kind"].(string)+":"+jsonNum(m["count"]))
		if m["app_hash"] != metrics.AppHash("USAGE-APP-1") || m["uid_hash"] != metrics.UIDHash("anon1") {
			t.Fatalf("line %v", m)
		}
	}
	if strings.Join(kinds, ",") != "write:1,read:1,write:1,read:1,write:1" {
		t.Fatalf("kinds %v", kinds)
	}
}

// A personal app's records are not counted.
func TestSharedUsageSkipsPersonalRecords(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/USAGE-APP-2"
	e.ownApp("owner", app, kakeibo(t))
	lines := sharedUsageLines(t)
	if r := e.do("owner", http.MethodPost, app+"/entities/expense/records", `{"data":{"amount":5}}`, nil); r.code != http.StatusCreated {
		t.Fatalf("post: %d %s", r.code, r.raw)
	}
	if got := lines(); len(got) != 0 {
		t.Fatalf("personal counted: %v", got)
	}
}

func jsonNum(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
