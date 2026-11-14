package feeds

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// logLines parses the JSON lines written to buf.
func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if l == "" {
			continue
		}
		m := map[string]any{}
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("log line %q: %v", l, err)
		}
		out = append(out, m)
	}
	return out
}

// noTerm fails when the search words show up in the log in any spelling.
func noTerm(t *testing.T, buf *bytes.Buffer, term string) {
	t.Helper()
	for _, s := range []string{term, url.QueryEscape(term), "any=", "/kokkai/speech"} {
		if strings.Contains(buf.String(), s) {
			t.Errorf("log carries %q:\n%s", s, buf.String())
		}
	}
}

func TestFetchLogDiet(t *testing.T) {
	const term = "こども家庭庁"
	var buf bytes.Buffer
	src := newSource(t)
	s := src.svc(&clock{t: testNow}, Options{Logger: slog.New(slog.NewJSONHandler(&buf, nil))})
	if _, err := s.Get(context.Background(), Query{Provider: dsl.FeedDiet, Q: term}, 5); err != nil {
		t.Fatal(err)
	}
	// a cache hit does not go out and writes no line
	if _, err := s.Get(context.Background(), Query{Provider: dsl.FeedDiet, Q: term}, 5); err != nil {
		t.Fatal(err)
	}
	lines := logLines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("lines = %d, want 1:\n%s", len(lines), buf.String())
	}
	l := lines[0]
	if l["msg"] != FetchEvent || l["event"] != FetchEvent || l["level"] != "INFO" || l["provider"] != dsl.FeedDiet ||
		l["status"] != "ok" || l["q_runes"] != float64(len([]rune(term))) {
		t.Errorf("line = %v", l)
	}
	if n, _ := l["bytes"].(float64); n <= 0 {
		t.Errorf("bytes = %v", l["bytes"])
	}
	if _, ok := l["duration_ms"].(float64); !ok {
		t.Errorf("duration_ms = %v", l["duration_ms"])
	}
	noTerm(t, &buf, term)
}

// A transport error (*url.Error) holds the URL with the search words: only
// its class may reach the log.
func TestFetchLogFailuresCarryNoTerm(t *testing.T) {
	const term = "予算委員会"
	down := httptest.NewServer(http.NotFoundHandler())
	downURL := down.URL
	down.Close()
	status := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(status.Close)
	garbage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json " + term))
	}))
	t.Cleanup(garbage.Close)
	for _, c := range []struct {
		name, base, want string
		client           *http.Client
	}{
		{"network", downURL, "network", http.DefaultClient},
		{"status", status.URL, "503", status.Client()},
		{"bad body", garbage.URL, "bad_response", garbage.Client()},
	} {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			s := New(Options{Client: c.client, Now: (&clock{t: testNow}).now,
				Endpoints: &Endpoints{Diet: c.base + "/kokkai/speech"}, Logger: slog.New(slog.NewJSONHandler(&buf, nil))})
			if _, err := s.Get(context.Background(), Query{Provider: dsl.FeedDiet, Q: term}, 5); !errors.Is(err, ErrUpstream) {
				t.Fatalf("err = %v", err)
			}
			lines := logLines(t, &buf)
			if len(lines) != 1 || lines[0]["status"] != c.want || lines[0]["level"] != "WARN" || lines[0]["provider"] != dsl.FeedDiet {
				t.Fatalf("lines = %v", lines)
			}
			noTerm(t, &buf, term)
		})
	}
}

func TestFetchStatus(t *testing.T) {
	for _, c := range []struct {
		err  error
		want string
	}{
		{nil, "ok"},
		{&StatusError{Code: 429}, "429"},
		{fmt.Errorf("x: %w", &StatusError{Code: 500}), "500"},
		{context.DeadlineExceeded, "timeout"},
		{&url.Error{Op: "Get", URL: "https://x/?any=secret", Err: context.Canceled}, "canceled"},
		{fmt.Errorf("%w (9 bytes)", ErrTooLarge), "too_large"},
		{fmt.Errorf("%w: eof", ErrBadResponse), "bad_response"},
		{&url.Error{Op: "Get", URL: "https://x/", Err: fmt.Errorf("%w: y", ErrRedirectBlocked)}, "redirect_blocked"},
		{&url.Error{Op: "Get", URL: "https://x/", Err: fmt.Errorf("%w: 10.0.0.1", ErrBlockedAddress)}, "blocked_address"},
		{ErrUnknown, "not_allowed"},
		{&url.Error{Op: "Get", URL: "https://x/", Err: errors.New("connection refused")}, "network"},
		{errors.New("other"), "error"},
	} {
		if got := FetchStatus(c.err); got != c.want {
			t.Errorf("FetchStatus(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}
