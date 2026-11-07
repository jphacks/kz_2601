package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/ai"
	"github.com/jphacks/kz_2601/backend/internal/appcheck"
	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/budget"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

// #317: the free plan pause sits inside the rate limiter (the caller is
// verified, its Pro checked) and its 503 gives the charged units back.
func TestBudgetInsideRateLimitRefunds(t *testing.T) {
	logger := quietLogger()
	store := budget.NewMemory()
	now := time.Now()
	day := now.In(ratelimit.JST).Format("2006-01-02")
	if err := store.Add(context.Background(), "local_"+day+"_ai_cost", 5, now.Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	breaker := budget.New(budget.Config{DailyUSD: 1}, store, nil)
	limiter := ratelimit.New(ratelimit.Config{Limits: ratelimit.Limits{SignedIn: 1}, Weights: ratelimit.DefaultWeights}, ratelimit.NewMemory(), nil)
	reached := false
	app := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { reached = true })
	h := newAIHandler(app, logger,
		appcheck.Middleware(appcheck.Config{Mode: appcheck.ModeEnforce, Verifier: okVerifier{}, Logger: logger}),
		ai.Pause(false, logger),
		withBudget(limiter.Middleware(auth.DevVerifier{}), breaker),
	)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer u1")
	req.Header.Set(appcheck.Header, "ok")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), httpx.CodeFreePlanPaused) || reached {
		t.Fatalf("want 503 free_plan_paused before the app, got %d %s", rec.Code, rec.Body)
	}
	if d, err := limiter.Allow(context.Background(), auth.Identity{UID: "u1"}, "", 1); err != nil || !d.Allowed {
		t.Fatalf("the unit must be given back: %+v %v", d, err)
	}
}
