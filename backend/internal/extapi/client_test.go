package extapi_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/extapi"
)

// fakeRunner answers every request with status/body and keeps what it got.
type fakeRunner struct {
	status int
	body   string
	path   string
	ctype  string
	got    extapi.ConnBody
}

func (f *fakeRunner) client(t *testing.T) *extapi.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		f.path, f.ctype = r.URL.Path, r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &f.got); err != nil {
			t.Errorf("request body %q: %v", raw, err)
		}
		w.WriteHeader(f.status)
		_, _ = io.WriteString(w, f.body)
	}))
	t.Cleanup(srv.Close)
	// a trailing slash on BaseURL is trimmed
	return &extapi.Client{BaseURL: srv.URL + "/", HTTP: srv.Client()}
}

func (f *fakeRunner) sent(t *testing.T, path string, want extapi.ConnBody) {
	t.Helper()
	if f.path != path || f.ctype != "application/json" || !reflect.DeepEqual(f.got, want) {
		t.Fatalf("sent %s (%s) %+v; want %s %+v", f.path, f.ctype, f.got, path, want)
	}
}

func TestClientStart(t *testing.T) {
	f := &fakeRunner{status: http.StatusOK, body: `{"authUrl":"https://example.test/consent"}`}
	c := f.client(t)
	got, err := c.Start(t.Context(), "u1", "todoist", []string{"read", "write"})
	if err != nil || got != "https://example.test/consent" {
		t.Fatalf("Start = %q, %v", got, err)
	}
	f.sent(t, "/internal/ext/connections/start", extapi.ConnBody{UID: "u1", Provider: "todoist", Scopes: []string{"read", "write"}})

	// an answer without authUrl is not the runner's
	f.body = `{}`
	if _, err := c.Start(t.Context(), "u1", "todoist", nil); !errors.Is(err, extapi.ErrRunnerAnswer) || !strings.Contains(err.Error(), "no authUrl") {
		t.Fatalf("empty authUrl: %v", err)
	}
}

func TestClientRevoke(t *testing.T) {
	f := &fakeRunner{status: http.StatusOK, body: `{"revoked":true}`}
	if err := f.client(t).Revoke(t.Context(), "u1", "slack"); err != nil {
		t.Fatal(err)
	}
	f.sent(t, "/internal/ext/connections/revoke", extapi.ConnBody{UID: "u1", Provider: "slack"})
}

func TestClientRevokeAll(t *testing.T) {
	f := &fakeRunner{status: http.StatusOK, body: `{"revoked":3}`}
	n, err := f.client(t).RevokeAll(t.Context(), "u1")
	if err != nil || n != 3 {
		t.Fatalf("RevokeAll = %d, %v", n, err)
	}
	f.sent(t, "/internal/ext/connections/revokeAll", extapi.ConnBody{UID: "u1"})
}

func TestClientList(t *testing.T) {
	f := &fakeRunner{status: http.StatusOK, body: `{"connections":[{"provider":"github","status":"active","scopes":["repo"]}],"enabled":["github","slack"]}`}
	c := f.client(t)
	got, err := c.List(t.Context(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	want := extapi.ConnectionList{
		Connections: []extapi.ConnectionView{{Provider: "github", Status: "active", Scopes: []string{"repo"}}},
		Enabled:     []string{"github", "slack"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List = %+v, want %+v", got, want)
	}
	f.sent(t, "/internal/ext/connections/list", extapi.ConnBody{UID: "u1"})

	// an older runner: no connections key → [] (not null), no enabled
	f.body = `{}`
	got, err = c.List(t.Context(), "u1")
	if err != nil || got.Connections == nil || len(got.Connections) != 0 || got.Enabled != nil {
		t.Fatalf("empty List = %#v, %v", got, err)
	}
}

// Every method shares the error handling of a non-200 or malformed answer.
func TestClientErrors(t *testing.T) {
	calls := map[string]func(c *extapi.Client) error{
		"Start": func(c *extapi.Client) error {
			_, err := c.Start(t.Context(), "u1", "p", nil)
			return err
		},
		"Revoke": func(c *extapi.Client) error { return c.Revoke(t.Context(), "u1", "p") },
		"RevokeAll": func(c *extapi.Client) error {
			_, err := c.RevokeAll(t.Context(), "u1")
			return err
		},
		"List": func(c *extapi.Client) error {
			_, err := c.List(t.Context(), "u1")
			return err
		},
	}
	cases := []struct {
		name    string
		status  int
		body    string
		want    *extapi.RunnerError // nil: ErrRunnerAnswer
		wantMsg string
	}{
		{"4xx with reason", http.StatusNotFound, `{"reason":"not_connected"}`, &extapi.RunnerError{Status: 404, Reason: "not_connected"}, "runner 404: not_connected"},
		{"429 with scope", http.StatusTooManyRequests, `{"reason":"rate_limited","scope":"user","retryAfterSec":30}`,
			&extapi.RunnerError{Status: 429, Reason: "rate_limited", Scope: "user", RetryAfterSec: 30}, "runner 429: rate_limited"},
		{"5xx with reason", http.StatusBadGateway, `{"reason":"provider_error"}`, &extapi.RunnerError{Status: 502, Reason: "provider_error"}, "runner 502"},
		{"5xx not JSON", http.StatusInternalServerError, `oops`, nil, "status 500"},
		{"4xx without reason", http.StatusBadRequest, `{}`, nil, "status 400"},
		{"200 not JSON", http.StatusOK, `<html>`, nil, "unexpected runner answer"},
		{"200 too large", http.StatusOK, `"` + strings.Repeat("a", 2<<20) + `"`, nil, "body over"},
	}
	for name, call := range calls {
		for _, tc := range cases {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				f := &fakeRunner{status: tc.status, body: tc.body}
				err := call(f.client(t))
				if err == nil || !strings.Contains(err.Error(), tc.wantMsg) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantMsg)
				}
				var re *extapi.RunnerError
				if tc.want == nil {
					if !errors.Is(err, extapi.ErrRunnerAnswer) || errors.As(err, &re) {
						t.Fatalf("err = %#v, want ErrRunnerAnswer", err)
					}
					return
				}
				if !errors.As(err, &re) || *re != *tc.want {
					t.Fatalf("err = %#v, want %#v", err, tc.want)
				}
			})
		}
	}
}

// A runner that cannot be reached is a plain transport error.
func TestClientUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	c := &extapi.Client{BaseURL: url}
	if _, err := c.List(t.Context(), "u1"); err == nil || errors.Is(err, extapi.ErrRunnerAnswer) {
		t.Fatalf("err = %v, want a transport error", err)
	}
}
