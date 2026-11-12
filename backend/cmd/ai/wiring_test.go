package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appcheck"
	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// clearEnv blanks every variable the ai wiring reads, so a developer's shell
// (GOOGLE_CLOUD_PROJECT etc.) does not change the outcome.
func clearEnv(t *testing.T, set map[string]string) {
	t.Helper()
	for _, k := range []string{
		"GOOGLE_CLOUD_PROJECT", "AUTH_MODE", "RATE_LIMIT", "RATE_LIMIT_STORE", "RATE_LIMIT_COLLECTION",
		"APP_CHECK_MODE", "APP_CHECK_APP_IDS", "COIN_BILLING_MODE", "APP_ENV",
		"AI_FREE_DAILY_BUDGET_USD", "AI_FREE_BUDGET_RESUME_DAY", "FIRESTORE_EMULATOR_HOST", "PRO_GATE_MODE",
		// ratelimit.ConfigFromEnv
		"RATE_LIMIT_ANON_DAILY", "RATE_LIMIT_USER_DAILY", "RATE_LIMIT_PRO_DAILY", "RATE_LIMIT_IP_DAILY",
		"RATE_LIMIT_GLOBAL_DAILY", "RATE_LIMIT_WAIVED_HOURLY", "RATE_LIMIT_WAIVED_IP_HOURLY", "RATE_LIMIT_UNCHANGED_DAILY",
		"RATE_LIMIT_WEIGHT_CREATE", "RATE_LIMIT_WEIGHT_EDIT", "RATE_LIMIT_WEIGHT_CHAT", "RATE_LIMIT_WEIGHT_TEXT",
		"RATE_LIMIT_WEIGHT_TEXT_TASK", "RATE_LIMIT_WEIGHT_TEXT_LONG", "RATE_LIMIT_WEIGHT_TEXT_IMAGE",
	} {
		t.Setenv(k, "")
	}
	for k, v := range set {
		t.Setenv(k, v)
	}
}

func TestEnvDefault(t *testing.T) {
	t.Setenv("AI_TEST_ENV_KEY", "")
	if got := env("AI_TEST_ENV_KEY", "def"); got != "def" {
		t.Fatalf("unset: %q", got)
	}
	t.Setenv("AI_TEST_ENV_KEY", "v")
	if got := env("AI_TEST_ENV_KEY", "def"); got != "v" {
		t.Fatalf("set: %q", got)
	}
}

// newRateLimit is on for every deployed run and for AUTH_MODE=dev; only a
// bare local run, or an explicit RATE_LIMIT=off, leaves generate/chat open.
func TestNewRateLimitModes(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		limited bool
	}{
		{name: "bare local run", env: nil, limited: false},
		{name: "deployed (project set)", env: map[string]string{"GOOGLE_CLOUD_PROJECT": "p"}, limited: true},
		{name: "dev auth", env: map[string]string{"AUTH_MODE": auth.ModeDev}, limited: true},
		{name: "forced on locally", env: map[string]string{"RATE_LIMIT": "on"}, limited: true},
		{name: "off on Cloud Run", env: map[string]string{"GOOGLE_CLOUD_PROJECT": "p", "RATE_LIMIT": "off"}, limited: false},
		{name: "0", env: map[string]string{"GOOGLE_CLOUD_PROJECT": "p", "RATE_LIMIT": "0"}, limited: false},
		{name: "false", env: map[string]string{"GOOGLE_CLOUD_PROJECT": "p", "RATE_LIMIT": "false"}, limited: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t, tt.env)
			mw := newRateLimit(auth.DevVerifier{}, nil, nil)
			reached := 0
			h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				reached++
				w.WriteHeader(http.StatusOK)
			}))

			// no bearer: the limiter answers 401 before the app
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{}`)))
			if limited := rec.Code == http.StatusUnauthorized; limited != tt.limited {
				t.Fatalf("limited = %v (status %d), want %v", limited, rec.Code, tt.limited)
			}
			if !tt.limited {
				return
			}
			// a verified caller gets through, counted in memory (no Firestore client)
			req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{}`))
			req.Header.Set("Authorization", "Bearer u1")
			rec = httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK || reached != 1 {
				t.Fatalf("signed-in caller: status %d reached %d (%s)", rec.Code, reached, rec.Body)
			}
		})
	}
}

func TestNewProChecker(t *testing.T) {
	clearEnv(t, nil)
	if p := newProChecker(nil); p != nil {
		t.Fatalf("bare local run: %T, want nil (no Pro checks)", p)
	}
	// deployed but Firestore failed: nobody is Pro (fail closed), not nil
	clearEnv(t, map[string]string{"GOOGLE_CLOUD_PROJECT": "p"})
	p := newProChecker(nil)
	if p == nil || p.IsPro(context.Background(), "u1") {
		t.Fatalf("deployed without Firestore: %T, want nobodyPro", p)
	}
}

func TestNewFirestoreBareRun(t *testing.T) {
	clearEnv(t, nil)
	if c := newFirestore(); c != nil {
		t.Fatal("no GOOGLE_CLOUD_PROJECT: want nil client")
	}
}

func TestNewCoins(t *testing.T) {
	clearEnv(t, nil)
	if cs := newCoins(nil); cs != nil {
		t.Fatalf("default off: %+v", cs)
	}
	clearEnv(t, map[string]string{"COIN_BILLING_MODE": "log"})
	cs := newCoins(nil)
	if cs == nil || cs.Config.Mode != coins.ModeLog {
		t.Fatalf("log mode: %+v", cs)
	}
	if _, ok := cs.Store.(*coins.Memory); !ok {
		t.Fatalf("no Firestore: store = %T, want memory", cs.Store)
	}
	if cs.ProPeriodEnd != nil {
		t.Fatal("no Firestore: nobody is Pro, ProPeriodEnd must be nil")
	}
	if proPeriodEnd(nil, []string{"Production"}) != nil {
		t.Fatal("proPeriodEnd(nil) must be nil")
	}
}

func TestNewBudgetOffByDefault(t *testing.T) {
	clearEnv(t, nil)
	b := newBudget(nil, nil)
	reached := false
	h := b.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { reached = true }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/chat", nil))
	if !reached {
		t.Fatal("no AI_FREE_DAILY_BUDGET_USD: the budget must not pause")
	}
}

// newAppCheck keeps /health open in every mode; in enforce, a verifier that
// cannot start (no project) rejects every API call (fail closed).
func TestNewAppCheck(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		path string
		want int
	}{
		{name: "off: api open", path: "/v1/generate", want: http.StatusNoContent},
		{name: "enforce: health open", env: map[string]string{"APP_CHECK_MODE": "enforce"}, path: "/health", want: http.StatusNoContent},
		{name: "enforce: healthz open", env: map[string]string{"APP_CHECK_MODE": "enforce"}, path: "/healthz", want: http.StatusNoContent},
		{name: "enforce: unconfigured rejects api", env: map[string]string{"APP_CHECK_MODE": "enforce"}, path: "/v1/generate", want: http.StatusUnauthorized},
		{name: "enforce: health prefix is not health", env: map[string]string{"APP_CHECK_MODE": "enforce"}, path: "/healthz/../v1/generate", want: http.StatusUnauthorized},
		{name: "log: unconfigured passes", env: map[string]string{"APP_CHECK_MODE": "log"}, path: "/v1/chat", want: http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t, tt.env)
			h := newAppCheck()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.URL.Path = tt.path
			req.Header.Set(appcheck.Header, "some-token")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.want, rec.Body)
			}
			if tt.want == http.StatusUnauthorized && !strings.Contains(rec.Body.String(), httpx.CodeAppCheckFailed) {
				t.Fatalf("body = %s", rec.Body)
			}
		})
	}
}
