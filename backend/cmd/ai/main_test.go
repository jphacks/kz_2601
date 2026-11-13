package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/ai"
	"github.com/jphacks/kz_2601/backend/internal/appcheck"
	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
}

// recording returns a middleware that appends name to *calls on the way in.
func recording(name string, calls *[]string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*calls = append(*calls, name)
			next.ServeHTTP(w, r)
		})
	}
}

func TestNewAIHandlerOrder(t *testing.T) {
	var calls []string
	app := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls = append(calls, "app")
		w.WriteHeader(http.StatusOK)
	})
	h := newAIHandler(app, quietLogger(),
		recording("appCheck", &calls),
		recording("pause", &calls),
		recording("rateLimit", &calls),
	)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/chat", nil))

	if want := []string{"appCheck", "pause", "rateLimit", "app"}; !slices.Equal(calls, want) {
		t.Fatalf("order = %v, want %v", calls, want)
	}
}

// countingStore counts how often the rate limiter charges a request.
type countingStore struct {
	ratelimit.Store
	n atomic.Int32
}

func (s *countingStore) Consume(ctx context.Context, c []ratelimit.Counter, weight int, exp time.Time) (ratelimit.Usage, error) {
	s.n.Add(1)
	return s.Store.Consume(ctx, c, weight, exp)
}

type okVerifier struct{}

func (okVerifier) Verify(_ context.Context, token string) (string, error) {
	if token != "ok" {
		return "", errors.New("invalid")
	}
	return "ios", nil
}

// The real middlewares in the real order: App Check and Pause reject before
// the rate limiter charges, observability (request id) wraps every answer.
func TestNewAIHandlerRejectsBeforeCharging(t *testing.T) {
	logger := quietLogger()
	tests := []struct {
		name      string
		appCheck  string
		paused    bool
		want      int
		wantCode  string
		wantCount int32
	}{
		{name: "app check rejects without charging", appCheck: "bad", want: http.StatusUnauthorized, wantCode: httpx.CodeAppCheckFailed},
		{name: "app check rejects even while paused", appCheck: "bad", paused: true, want: http.StatusUnauthorized, wantCode: httpx.CodeAppCheckFailed},
		{name: "paused answers 503 without charging", appCheck: "ok", paused: true, want: http.StatusServiceUnavailable, wantCode: httpx.CodeAIPaused},
		{name: "admitted request is charged once", appCheck: "ok", want: http.StatusOK, wantCount: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &countingStore{Store: ratelimit.NewMemory()}
			limiter := ratelimit.New(ratelimit.Config{Limits: ratelimit.DefaultLimits, Weights: ratelimit.DefaultWeights}, store, nil)
			app := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
			h := newAIHandler(app, logger,
				appcheck.Middleware(appcheck.Config{Mode: appcheck.ModeEnforce, Verifier: okVerifier{}, Logger: logger}),
				ai.Pause(tt.paused, logger),
				limiter.Middleware(auth.DevVerifier{}),
			)

			req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{}`))
			req.Header.Set("Authorization", "Bearer u1")
			req.Header.Set(appcheck.Header, tt.appCheck)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.want, rec.Body.String())
			}
			if tt.wantCode != "" && !strings.Contains(rec.Body.String(), tt.wantCode) {
				t.Fatalf("body %s, want code %s", rec.Body.String(), tt.wantCode)
			}
			if got := store.n.Load(); got != tt.wantCount {
				t.Fatalf("rate limiter charged %d times, want %d", got, tt.wantCount)
			}
			if rec.Header().Get(httpx.RequestIDHeader) == "" {
				t.Fatalf("missing %s: observability must be outermost", httpx.RequestIDHeader)
			}
		})
	}
}

// A server-side failure from the app (Claude 5xx / timeout) is refunded through
// the real chain; a client error is not; ai_paused is never charged at all (#548).
func TestNewAIHandlerRefundsServerFailures(t *testing.T) {
	logger := quietLogger()
	tests := []struct {
		name      string
		paused    bool
		appStatus int
		want      int
		wantUsed  bool // the 1-unit daily limit is still used up afterwards
	}{
		{name: "504 from the app is refunded", appStatus: http.StatusGatewayTimeout, want: http.StatusGatewayTimeout},
		{name: "502 from the app is refunded", appStatus: http.StatusBadGateway, want: http.StatusBadGateway},
		{name: "400 from the app is not refunded", appStatus: http.StatusBadRequest, want: http.StatusBadRequest, wantUsed: true},
		{name: "paused 503 is never charged", paused: true, want: http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := ratelimit.New(ratelimit.Config{Limits: ratelimit.Limits{SignedIn: 1}, Weights: ratelimit.DefaultWeights}, ratelimit.NewMemory(), nil)
			app := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tt.appStatus) })
			h := newAIHandler(app, logger,
				appcheck.Middleware(appcheck.Config{Mode: appcheck.ModeEnforce, Verifier: okVerifier{}, Logger: logger}),
				ai.Pause(tt.paused, logger),
				limiter.Middleware(auth.DevVerifier{}),
			)
			call := func() int {
				req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{}`))
				req.Header.Set("Authorization", "Bearer u1")
				req.Header.Set(appcheck.Header, "ok")
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				return rec.Code
			}
			if got := call(); got != tt.want {
				t.Fatalf("status = %d, want %d", got, tt.want)
			}
			d, err := limiter.Allow(context.Background(), auth.Identity{UID: "u1"}, "", 1)
			if err != nil {
				t.Fatal(err)
			}
			if used := !d.Allowed; used != tt.wantUsed {
				t.Fatalf("limit used up = %v, want %v (%+v)", used, tt.wantUsed, d)
			}
		})
	}
}

// mapEnv is a getenv over a fixed table; nil is the empty environment.
func mapEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestEnvFrom(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "empty table", env: nil, want: "def"},
		{name: "empty value is unset", env: map[string]string{"K": ""}, want: "def"},
		{name: "set", env: map[string]string{"K": "v"}, want: "v"},
		{name: "whitespace is a value", env: map[string]string{"K": " "}, want: " "},
	}
	for _, tt := range tests {
		if got := envFrom(mapEnv(tt.env), "K", "def"); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestGenerateTimeoutSec(t *testing.T) {
	tests := []struct {
		raw  string
		want int
	}{
		{raw: "", want: 45},
		{raw: "10", want: 10},
		{raw: "0", want: 45},
		{raw: "-5", want: 45},
		{raw: "abc", want: 45},
		{raw: "1.5", want: 45},
	}
	for _, tt := range tests {
		if got := generateTimeoutSec(mapEnv(map[string]string{"GENERATE_TIMEOUT_SEC": tt.raw})); got != tt.want {
			t.Errorf("GENERATE_TIMEOUT_SEC=%q: got %d, want %d", tt.raw, got, tt.want)
		}
	}
	if got := generateTimeoutSec(mapEnv(nil)); got != 45 {
		t.Errorf("empty table: got %d", got)
	}
}

func TestRateLimitPlanFrom(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		hasClient  bool
		on         bool
		store      string
		collection string
	}{
		{name: "empty table", env: nil, on: false},
		{name: "firebase auth without project", env: map[string]string{"AUTH_MODE": auth.ModeFirebase}, on: false},
		{name: "dev auth", env: map[string]string{"AUTH_MODE": auth.ModeDev}, on: true, store: "memory"},
		{name: "forced on", env: map[string]string{"RATE_LIMIT": "on"}, on: true, store: "memory"},
		{name: "unknown value counts as on", env: map[string]string{"RATE_LIMIT": "yes"}, on: true, store: "memory"},
		{name: "off", env: map[string]string{"GOOGLE_CLOUD_PROJECT": "p", "RATE_LIMIT": "off"}, hasClient: true, on: false},
		{name: "0", env: map[string]string{"GOOGLE_CLOUD_PROJECT": "p", "RATE_LIMIT": "0"}, on: false},
		{name: "false", env: map[string]string{"GOOGLE_CLOUD_PROJECT": "p", "RATE_LIMIT": "false"}, on: false},
		{name: "project without client", env: map[string]string{"GOOGLE_CLOUD_PROJECT": "p"}, on: true, store: "memory"},
		{name: "project with client", env: map[string]string{"GOOGLE_CLOUD_PROJECT": "p", "RATE_LIMIT_COLLECTION": "c"}, hasClient: true, on: true, store: "firestore", collection: "c"},
		{name: "memory store forced", env: map[string]string{"GOOGLE_CLOUD_PROJECT": "p", "RATE_LIMIT_STORE": "memory"}, hasClient: true, on: true, store: "memory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := rateLimitPlanFrom(mapEnv(tt.env), tt.hasClient)
			if err != nil {
				t.Fatal(err)
			}
			if plan.On != tt.on || plan.Store != tt.store || plan.Collection != tt.collection {
				t.Fatalf("got %+v, want on=%v store=%q collection=%q", plan, tt.on, tt.store, tt.collection)
			}
			if !plan.On && plan.Reason == "" {
				t.Fatal("off without a reason")
			}
		})
	}
}

func TestRateLimitPlanFromConfig(t *testing.T) {
	plan, err := rateLimitPlanFrom(mapEnv(map[string]string{"RATE_LIMIT": "on", "APP_ENV": "stg", "RATE_LIMIT_ANON_DAILY": "7"}), false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Config.Env != "stg" || plan.Config.Limits.Anonymous != 7 || plan.Config.Limits.SignedIn != ratelimit.DefaultLimits.SignedIn {
		t.Fatalf("config %+v", plan.Config)
	}

	for _, raw := range []string{"-1", "x"} {
		if _, err := rateLimitPlanFrom(mapEnv(map[string]string{"RATE_LIMIT": "on", "RATE_LIMIT_USER_DAILY": raw}), false); err == nil {
			t.Errorf("RATE_LIMIT_USER_DAILY=%q: no error", raw)
		}
	}
	// off never reads the limits, so a bad value does not stop the service
	if _, err := rateLimitPlanFrom(mapEnv(map[string]string{"RATE_LIMIT": "off", "RATE_LIMIT_USER_DAILY": "x"}), false); err != nil {
		t.Errorf("off with a bad limit: %v", err)
	}
}
