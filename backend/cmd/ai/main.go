// ai service: transcript -> Claude -> DSL (full or JSON Patch) -> appconfig.
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"cloud.google.com/go/firestore"

	"github.com/jphacks/kz_2601/backend/internal/ai"
	"github.com/jphacks/kz_2601/backend/internal/appcheck"
	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/books"
	"github.com/jphacks/kz_2601/backend/internal/entitlement"
	"github.com/jphacks/kz_2601/backend/internal/fsenv"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/paid"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
	fsstore "github.com/jphacks/kz_2601/backend/internal/store"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	// Cloud Logging JSON for slog and the log package alike (#295)
	logger := httpx.NewLogger()
	slog.SetDefault(logger)

	// stg must not use the production Firestore unless ALLOW_SHARED_PROD_DB=1 (#290)
	if shared, err := fsenv.Check(fsenv.FromEnv(os.Getenv), os.Getenv); err != nil {
		log.Fatalf("ai: refusing to start: %v", err)
	} else if shared {
		log.Printf("ai: warning: stg shares the production Firestore (ALLOW_SHARED_PROD_DB=1)")
	}
	port := env("PORT", "8081")
	model := env("CLAUDE_MODEL", "claude-sonnet-5-5")
	effort := env("CLAUDE_EFFORT", "low")
	timeoutSec, _ := strconv.Atoi(env("GENERATE_TIMEOUT_SEC", "45"))
	if timeoutSec <= 0 {
		timeoutSec = 45
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Printf("warning: ANTHROPIC_API_KEY is not set; /v1/generate will fail")
	}

	var store appconfigclient.Store
	if base := os.Getenv("APPCONFIG_BASE_URL"); base != "" {
		store = appconfigclient.NewHTTP(base)
		log.Printf("ai: appconfig at %s", base)
	} else {
		store = appconfigclient.NewMemory()
		log.Printf("ai: APPCONFIG_BASE_URL unset; using in-memory DSL store")
	}

	// one verifier for /v1/chat callers (#286) and the rate limiter (#280)
	authMode := env("AUTH_MODE", auth.ModeFirebase)
	verifier := auth.VerifierFromEnv(context.Background(), authMode, os.Getenv("GOOGLE_CLOUD_PROJECT"))

	proMode, err := paid.ModeFromEnv(os.Getenv) // PRO_GATE_MODE: enforce (default) | log | off
	if err != nil {
		log.Fatalf("ai: %v", err)
	}
	log.Printf("ai: pro gate mode=%s", proMode)
	fsClient := newFirestore()
	pro := newProChecker(fsClient)

	claude := ai.NewClient(apiKey, model, effort)
	claude.StructuredOutput = os.Getenv("CLAUDE_STRUCTURED_OUTPUT") == "1"
	claude.Logger = logger // ai.usage JSON lines for the cost sink (#298)
	// free plan pause on the day's AI cost (#317): ai.usage also adds to the day's total
	breaker := newBudget(fsClient, pro)
	claude.Logger = slog.New(breaker.Tap(logger.Handler(), ai.UsageEvent))
	svc := &ai.Service{
		Claude:          claude,
		Store:           store,
		Model:           model,
		CallTimeout:     time.Duration(timeoutSec) * time.Second,
		LogicPass:       os.Getenv("AI_LOGIC_PASS") == "1",   // UI pass + calculations pass (#100)
		ServerRulesPass: os.Getenv("AI_SERVER_RULES") == "1", // server rules for automatic actions (#343)
		LintRetry:       os.Getenv("AI_LINT_RETRY") == "1",   // off: lint warnings are reported only
		TextImage:       os.Getenv("AI_TEXT_IMAGE") == "1",   // aiButton inputPhoto; off until the privacy policy names photos
		NoBookLookup:    !books.EnabledFromEnv(os.Getenv),    // lookupBook only while appconfig has BOOK_LOOKUP_ENABLED (openBD terms)
		NoCalls:         !ai.CallsEnabled(os.Getenv),         // callButton only while appconfig has CALLS_ENABLED (#447)
		Logger:          claude.Logger,                       // ai.generate per request (#322)
		// clarify turn (#147): its own model/effort/thinking; empty = ai.DefaultChat*
		ChatConfig: ai.ChatConfig{
			Model:    os.Getenv("AI_CHAT_MODEL"),
			Effort:   os.Getenv("AI_CHAT_EFFORT"),
			Thinking: os.Getenv("AI_CHAT_THINKING"),
		},
		// JPHACKS product recipes only when the request needs them (keywords + selector
		// call); AI_ON_DEMAND_RULES=0 sends them all as before. AI_RECIPE_MODEL: the selector
		OnDemandRules: os.Getenv("AI_ON_DEMAND_RULES") != "0",
		RecipeModel:   os.Getenv("AI_RECIPE_MODEL"),
		// /v1/chat callers (#286): AUTH_MODE=firebase (default) | dev
		Verifier: verifier,
		AuthMode: authMode,
		// Pro features (#316): 402 pro_required on a paid save / POST /v1/text
		Pro:     pro,
		ProMode: proMode,
	}
	if fsClient != nil {
		// a participant's /v1/text is covered by the shared app's creator (read-only lookups)
		svc.Members = fsstore.NewFirestoreFromClient(fsClient)
	}
	// creator-paid /v1/text calls per app and day (docs/backend/rate-limit.md)
	creatorDaily, err := ai.CreatorPaidDailyFromEnv(os.Getenv)
	if err != nil {
		log.Fatalf("ai: %v", err)
	}
	creatorMemberDaily, err := ai.CreatorPaidMemberDailyFromEnv(os.Getenv)
	if err != nil {
		log.Fatalf("ai: %v", err)
	}
	svc.CreatorPaid = &ai.CreatorPaidCap{Daily: creatorDaily, MemberDaily: creatorMemberDaily, Env: os.Getenv("APP_ENV")}
	if fsClient != nil && os.Getenv("RATE_LIMIT_STORE") != "memory" {
		svc.CreatorPaid.Store = ratelimit.NewFirestore(fsClient, os.Getenv("RATE_LIMIT_COLLECTION"))
	}
	// COIN_BILLING_MODE (#630): generate / chat / text hold and charge coins
	if cs := newCoins(fsClient); cs != nil {
		svc.Coins = cs
	}
	// AI_PAUSED=1 stops only the Claude-calling routes with 503 ai_paused,
	// before the rate limiter (#511). docs/infra/ai-pause.md
	paused := ai.PausedFromEnv(os.Getenv)
	if paused {
		log.Printf("ai: AI_PAUSED is on; generate/chat/text answer 503 ai_paused")
	}
	handler := newAIHandler(ai.NewHandler(svc), logger,
		newAppCheck(),
		ai.Pause(paused, logger),
		withBudget(newRateLimit(verifier, fsClient, pro), breaker),
	)
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      time.Duration(timeoutSec)*time.Second*3 + 10*time.Second,
	}
	log.Printf("ai listening on :%s (model=%s effort=%s logicPass=%v chat=%+v)", port, model, effort, svc.LogicPass, svc.ChatConfig)
	log.Fatal(srv.ListenAndServe())
}

// newAIHandler wraps app outermost first. The order is security- and
// cost-relevant (main_test.go): observability sees every response, App Check
// and Pause reject before the rate limiter charges anything.
func newAIHandler(app http.Handler, logger *slog.Logger, appCheck, pause, rateLimit func(http.Handler) http.Handler) http.Handler {
	return httpx.Chain(app,
		// request id / trace on every response (#295)
		httpx.WithRequestID,
		// access log, 5xx as ERROR (#295)
		httpx.Log(logger),
		// panics -> 500 + Error Reporting (#295)
		httpx.Recover(logger),
		// /debug/error answers 5xx/panic on stg only (#295)
		httpx.DebugErrors(os.Getenv("APP_ENV") == "stg"),
		// enforce rejects before the rate limiter, without spending quota (#318)
		appCheck,
		// AI_PAUSED: 503 before the rate limiter, nothing charged (#511)
		pause,
		// daily limits; innermost so only admitted requests are counted (#280)
		rateLimit,
	)
}

// newAppCheck verifies X-Firebase-AppCheck (#318). /health stays open.
//
//	APP_CHECK_MODE=off (default) | log | enforce
//	APP_CHECK_APP_IDS  optional comma-separated Firebase app IDs to accept
//
// docs/infra/app-check.md
func newAppCheck() func(http.Handler) http.Handler {
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	mw, mode, err := appcheck.FromEnv(os.Getenv, httpx.NewLogger(), func() (appcheck.Verifier, error) {
		return appcheck.NewFirebaseVerifier(context.Background(), project)
	})
	if err != nil {
		log.Fatalf("ai: %v", err)
	}
	log.Printf("ai: app check mode=%s", mode)
	return func(h http.Handler) http.Handler {
		checked := mw(h)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" || r.URL.Path == "/healthz" {
				h.ServeHTTP(w, r)
				return
			}
			checked.ServeHTTP(w, r)
		})
	}
}

// newRateLimit returns the per-user / per-IP / global daily limits (#280) for
// POST /v1/generate and /v1/chat (identity when off).
//
//	RATE_LIMIT=off|on   default: on when GOOGLE_CLOUD_PROJECT is set (Cloud Run)
//	                    or AUTH_MODE=dev, off for a bare local run
//	RATE_LIMIT_STORE    firestore (default with GOOGLE_CLOUD_PROJECT) | memory
//	RATE_LIMIT_COLLECTION  Firestore collection (default rateLimits)
//	FIRESTORE_DATABASE  database ID (default "(default)"; internal/fsenv)
//
// Limits/weights: ratelimit.ConfigFromEnv.
func newRateLimit(verifier auth.IdentityVerifier, client *firestore.Client, pro ratelimit.ProChecker) func(http.Handler) http.Handler {
	off := func(h http.Handler) http.Handler { return h }
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	mode := env("AUTH_MODE", auth.ModeFirebase)
	switch os.Getenv("RATE_LIMIT") {
	case "off", "0", "false":
		log.Printf("ai: RATE_LIMIT=off; generate/chat are not limited")
		return off
	case "":
		if project == "" && mode != auth.ModeDev {
			log.Printf("ai: rate limit off (no GOOGLE_CLOUD_PROJECT); set RATE_LIMIT=on to force")
			return off
		}
	}
	cfg, err := ratelimit.ConfigFromEnv(os.Getenv)
	if err != nil {
		log.Fatalf("ai: %v", err)
	}
	var store ratelimit.Store = ratelimit.NewMemory()
	storeName := "memory"
	if client != nil && os.Getenv("RATE_LIMIT_STORE") != "memory" {
		store = ratelimit.NewFirestore(client, os.Getenv("RATE_LIMIT_COLLECTION"))
		storeName = "firestore"
	}
	perOS := ""
	if len(cfg.PlatformLimits) > 0 { // RATE_LIMIT_*_DAILY_<OS> (#2133)
		perOS = fmt.Sprintf(" platformLimits=%v", cfg.PlatformLimits)
	}
	log.Printf("ai: rate limit on (store=%s env=%q limits=%+v%s weights=%+v pro=%v)", storeName, cfg.Env, cfg.Limits, perOS, cfg.Weights, pro != nil)
	return ratelimit.New(cfg, store, pro).Middleware(verifier)
}

// newFirestore connects to Firestore when GOOGLE_CLOUD_PROJECT is set (rate
// limit counters, Pro entitlements). nil for a bare local run or on failure.
func newFirestore() *firestore.Client {
	if os.Getenv("GOOGLE_CLOUD_PROJECT") == "" {
		return nil
	}
	c, err := fsenv.NewClient(context.Background(), fsenv.FromEnv(os.Getenv))
	if err != nil {
		log.Printf("ai: firestore init failed: %v (rate limit counts in memory, nobody is pro)", err)
		return nil
	}
	return c
}

// newProChecker reads users/{uid}/entitlements/pro written by appconfig after
// it verified the StoreKit transaction (#314): the Pro rate limits (#449) and
// the paid features (#316). A bare local run (no GOOGLE_CLOUD_PROJECT) checks
// nothing (nil); a deployed one whose Firestore failed treats nobody as Pro.
func newProChecker(client *firestore.Client) proChecker {
	if client == nil {
		if os.Getenv("GOOGLE_CLOUD_PROJECT") == "" {
			return nil
		}
		return nobodyPro{}
	}
	policy, err := entitlement.ConfigFromEnv(os.Getenv)
	if err != nil {
		log.Fatalf("ai: %v", err)
	}
	return entitlement.NewProChecker(entitlement.NewFirestore(client), policy.Environments)
}

// proChecker is both ratelimit.ProChecker and paid.ProChecker.
type proChecker interface {
	IsPro(ctx context.Context, uid string) bool
}

type nobodyPro struct{}

func (nobodyPro) IsPro(context.Context, string) bool { return false }
