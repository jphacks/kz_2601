package books

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/feeds"
)

// One feeds.fetch line per outbound lookup, success or not, never the ISBN.
func TestFetchLogCarriesNoISBN(t *testing.T) {
	const raw, isbn = "4-7808-0204-0", "9784780802047"
	const isbn10 = "4780802040"
	down := httptest.NewServer(http.NotFoundHandler())
	downURL := down.URL
	down.Close()
	for _, c := range []struct {
		name    string
		handler func(http.ResponseWriter, string)
		base    string // overrides the fake's URL
		status  string
		level   string
		found   bool
	}{
		{name: "found", handler: recorded(t), status: "ok", level: "INFO", found: true},
		{name: "no record", handler: func(w http.ResponseWriter, _ string) { _, _ = w.Write([]byte("[null]")) }, status: "ok", level: "INFO"},
		{name: "status", handler: func(w http.ResponseWriter, _ string) { w.WriteHeader(http.StatusBadGateway) }, status: "502", level: "WARN"},
		{name: "bad body", handler: func(w http.ResponseWriter, i string) { _, _ = w.Write([]byte("oops " + i)) }, status: "bad_response", level: "WARN"},
		{name: "network", handler: recorded(t), base: downURL, status: "network", level: "WARN"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			f := newSource(t, c.handler)
			s := newService(f, &clock{t: time.Date(2026, 11, 8, 3, 0, 0, 0, time.UTC)},
				Options{Logger: slog.New(slog.NewJSONHandler(&buf, nil))})
			if c.base != "" {
				s.ep.GetURL = c.base + "/v1/get"
			}
			_, _ = s.Lookup(context.Background(), raw)
			_, _ = s.Lookup(context.Background(), raw) // cached: no second line
			var lines []map[string]any
			for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
				m := map[string]any{}
				if err := json.Unmarshal([]byte(l), &m); err != nil {
					t.Fatalf("log line %q: %v", l, err)
				}
				lines = append(lines, m)
			}
			if len(lines) != 1 {
				t.Fatalf("lines = %d:\n%s", len(lines), buf.String())
			}
			l := lines[0]
			if l["event"] != feeds.FetchEvent || l["provider"] != Provider || l["status"] != c.status ||
				l["level"] != c.level || l["found"] != c.found {
				t.Errorf("line = %v", l)
			}
			if _, ok := l["bytes"].(float64); !ok {
				t.Errorf("bytes = %v", l["bytes"])
			}
			if _, ok := l["duration_ms"].(float64); !ok {
				t.Errorf("duration_ms = %v", l["duration_ms"])
			}
			for _, s := range []string{isbn, isbn10, raw, "isbn="} {
				if strings.Contains(buf.String(), s) {
					t.Errorf("log carries %q:\n%s", s, buf.String())
				}
			}
		})
	}
}
