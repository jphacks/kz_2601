package appcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeVerifier accepts "good-<appID>" tokens.
type fakeVerifier struct{ calls int }

func (f *fakeVerifier) Verify(_ context.Context, token string) (string, error) {
	f.calls++
	if id, ok := strings.CutPrefix(token, "good-"); ok {
		return id, nil
	}
	return "", errors.New("bad signature")
}

func serve(t *testing.T, cfg Config, token string) (*httptest.ResponseRecorder, bool, string) {
	t.Helper()
	var logs bytes.Buffer
	cfg.Logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	reached := false
	h := Middleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/v1/generate", nil)
	if token != "" {
		req.Header.Set(Header, token)
	}
	req.Header.Set("X-Client-Build", "12")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, reached, logs.String()
}

func TestMiddlewareModes(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		token      string
		noVerifier bool
		appIDs     []string
		wantPass   bool
		wantResult string // "" = no log line
	}{
		{name: "off ignores missing", mode: ModeOff, wantPass: true},
		{name: "off ignores invalid", mode: ModeOff, token: "junk", wantPass: true},
		{name: "log passes missing", mode: ModeLog, wantPass: true, wantResult: ResultMissing},
		{name: "log passes invalid", mode: ModeLog, token: "junk", wantPass: true, wantResult: ResultInvalid},
		{name: "log passes valid", mode: ModeLog, token: "good-ios", wantPass: true, wantResult: ResultValid},
		{name: "log passes unconfigured", mode: ModeLog, token: "good-ios", noVerifier: true, wantPass: true, wantResult: ResultUnconfigured},
		{name: "enforce passes valid", mode: ModeEnforce, token: "good-ios", wantPass: true, wantResult: ResultValid},
		{name: "enforce rejects missing", mode: ModeEnforce, wantResult: ResultMissing},
		{name: "enforce rejects invalid", mode: ModeEnforce, token: "junk", wantResult: ResultInvalid},
		{name: "enforce fails closed unconfigured", mode: ModeEnforce, token: "good-ios", noVerifier: true, wantResult: ResultUnconfigured},
		{name: "enforce rejects other app", mode: ModeEnforce, token: "good-web", appIDs: []string{"ios"}, wantResult: ResultWrongApp},
		{name: "enforce accepts listed app", mode: ModeEnforce, token: "good-ios", appIDs: []string{"web", "ios"}, wantPass: true, wantResult: ResultValid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fv := &fakeVerifier{}
			cfg := Config{Mode: tt.mode, AppIDs: tt.appIDs}
			if !tt.noVerifier {
				cfg.Verifier = fv
			}
			rec, reached, logs := serve(t, cfg, tt.token)
			if reached != tt.wantPass {
				t.Fatalf("reached = %v, want %v (status %d)", reached, tt.wantPass, rec.Code)
			}
			if tt.mode == ModeOff && (fv.calls != 0 || logs != "") {
				t.Fatalf("off must not verify or log: calls=%d logs=%q", fv.calls, logs)
			}
			if tt.wantResult != "" && !strings.Contains(logs, `"appCheck":"`+tt.wantResult+`"`) {
				t.Fatalf("logs %q lack result %q", logs, tt.wantResult)
			}
			if tt.wantResult != "" && !strings.Contains(logs, `"clientBuild":"12"`) {
				t.Fatalf("logs %q lack client attrs", logs)
			}
			if tt.wantPass {
				return
			}
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			var body struct {
				Error struct {
					Code    string         `json:"code"`
					Details map[string]any `json:"details"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error.Code != "app_check_failed" || body.Error.Details["reason"] != tt.wantResult {
				t.Fatalf("body = %s", rec.Body.String())
			}
		})
	}
}

func TestParseMode(t *testing.T) {
	for raw, want := range map[string]string{"": ModeOff, "off": ModeOff, " LOG ": ModeLog, "enforce": ModeEnforce} {
		if got, err := ParseMode(raw); err != nil || got != want {
			t.Errorf("ParseMode(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	if _, err := ParseMode("on"); err == nil {
		t.Error("ParseMode(on) should fail")
	}
}

func TestParseAppIDs(t *testing.T) {
	got := ParseAppIDs(" a, ,b ,")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("got %q", got)
	}
}

func TestFromEnv(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

	called := false
	_, mode, err := FromEnv(env(nil), logger, func() (Verifier, error) { called = true; return nil, nil })
	if err != nil || mode != ModeOff || called {
		t.Fatalf("default: mode=%q err=%v called=%v (off must not build a verifier)", mode, err, called)
	}
	if _, _, err := FromEnv(env(map[string]string{"APP_CHECK_MODE": "strict"}), logger, nil); err == nil {
		t.Fatal("unknown mode should fail")
	}
	// enforce with a failed verifier rejects (fails closed)
	mw, mode, err := FromEnv(env(map[string]string{"APP_CHECK_MODE": "enforce"}), logger,
		func() (Verifier, error) { return nil, errors.New("jwks fetch failed") })
	if err != nil || mode != ModeEnforce {
		t.Fatalf("mode=%q err=%v", mode, err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/apps", nil)
	req.Header.Set(Header, "good-ios")
	mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("must not reach") })).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestFirebaseVerifierNeedsProject(t *testing.T) {
	if _, err := NewFirebaseVerifier(context.Background(), ""); err == nil {
		t.Fatal("empty project should fail")
	}
}
