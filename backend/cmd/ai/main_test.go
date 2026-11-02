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
