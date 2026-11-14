package main

import (
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/firestore"

	"github.com/jphacks/kz_2601/backend/internal/budget"
)

// newBudget is the free plan's pause on the day's AI cost (#317).
//
//	AI_FREE_DAILY_BUDGET_USD   threshold in USD per day (JST); unset/0 = off
//	AI_FREE_BUDGET_RESUME_DAY  YYYY-MM-DD: lift the pause for that day only
//
// The total lives next to the rate limit counters (RATE_LIMIT_COLLECTION,
// RATE_LIMIT_STORE=memory keeps it per instance). docs/backend/rate-limit.md
func newBudget(client *firestore.Client, pro budget.ProChecker) *budget.Breaker {
	cfg, err := budget.ConfigFromEnv(os.Getenv)
	if err != nil {
		log.Fatalf("ai: %v", err)
	}
	var store budget.Store = budget.NewMemory()
	storeName := "memory"
	if client != nil && os.Getenv("RATE_LIMIT_STORE") != "memory" {
		store = budget.NewFirestore(client, os.Getenv("RATE_LIMIT_COLLECTION"))
		storeName = "firestore"
	}
	if cfg.Enabled() {
		log.Printf("ai: free plan budget on (%.2f USD/day, store=%s, resumeDay=%q)", cfg.DailyUSD, storeName, cfg.ResumeDay)
	}
	return budget.New(cfg, store, pro)
}

// withBudget runs the budget check inside the rate limiter: the caller is
// verified by then, and a 503 gives the charged units back (#548).
func withBudget(rateLimit func(http.Handler) http.Handler, b *budget.Breaker) func(http.Handler) http.Handler {
	pause := b.Middleware()
	return func(h http.Handler) http.Handler { return rateLimit(pause(h)) }
}
