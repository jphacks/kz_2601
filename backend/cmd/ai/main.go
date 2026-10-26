// ai service: transcript -> Claude -> DSL (full or JSON Patch) -> appconfig.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/ai"
	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"

	"cloud.google.com/go/firestore"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
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

	claude := ai.NewClient(apiKey, model, effort)
	claude.StructuredOutput = os.Getenv("CLAUDE_STRUCTURED_OUTPUT") == "1"
	svc := &ai.Service{
		Claude:      claude,
		Store:       store,
		Model:       model,
		CallTimeout: time.Duration(timeoutSec) * time.Second,
		LogicPass:   os.Getenv("AI_LOGIC_PASS") == "1", // UI pass + calculations pass (#100)
		LintRetry:   os.Getenv("AI_LINT_RETRY") == "1", // off: lint warnings are reported only
		// clarify turn (#147): its own model/effort/thinking; empty = ai.DefaultChat*
		ChatConfig: ai.ChatConfig{
			Model:    os.Getenv("AI_CHAT_MODEL"),
			Effort:   os.Getenv("AI_CHAT_EFFORT"),
			Thinking: os.Getenv("AI_CHAT_THINKING"),
		},
		// /v1/chat callers (#286): AUTH_MODE=firebase (default) | dev
		Verifier: auth.VerifierFromEnv(context.Background(), env("AUTH_MODE", auth.ModeFirebase), os.Getenv("GOOGLE_CLOUD_PROJECT")),
		AuthMode: env("AUTH_MODE", auth.ModeFirebase),
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           withRateLimit(ai.NewHandler(svc)),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      time.Duration(timeoutSec)*time.Second*3 + 10*time.Second,
	}
	log.Printf("ai listening on :%s (model=%s effort=%s logicPass=%v chat=%+v)", port, model, effort, svc.LogicPass, svc.ChatConfig)
	log.Fatal(srv.ListenAndServe())
}

// withRateLimit puts the per-user / per-IP / global daily limits (#280) in
// front of POST /v1/generate and /v1/chat.
//
//	RATE_LIMIT=off|on   default: on when GOOGLE_CLOUD_PROJECT is set (Cloud Run)
//	                    or AUTH_MODE=dev, off for a bare local run
//	RATE_LIMIT_STORE    firestore (default with GOOGLE_CLOUD_PROJECT) | memory
//	RATE_LIMIT_COLLECTION  Firestore collection (default rateLimits)
//
// Limits/weights: ratelimit.ConfigFromEnv.
func withRateLimit(h http.Handler) http.Handler {
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	mode := env("AUTH_MODE", auth.ModeFirebase)
	switch os.Getenv("RATE_LIMIT") {
	case "off", "0", "false":
		log.Printf("ai: RATE_LIMIT=off; generate/chat are not limited")
		return h
	case "":
		if project == "" && mode != auth.ModeDev {
			log.Printf("ai: rate limit off (no GOOGLE_CLOUD_PROJECT); set RATE_LIMIT=on to force")
			return h
		}
	}
	cfg, err := ratelimit.ConfigFromEnv(os.Getenv)
	if err != nil {
		log.Fatalf("ai: %v", err)
	}
	ctx := context.Background()

	var verifier auth.IdentityVerifier = auth.NotConfigured{}
	if mode == auth.ModeDev {
		log.Printf("warning: AUTH_MODE=dev: bearer tokens are trusted as uids; never use in production")
		verifier = auth.DevVerifier{}
	} else if v, err := auth.NewFirebaseVerifier(ctx, project); err != nil {
		log.Printf("ai: firebase verifier init failed: %v (generate/chat will answer 401)", err)
	} else {
		verifier = v
	}

	var store ratelimit.Store = ratelimit.NewMemory()
	storeName := "memory"
	if project != "" && os.Getenv("RATE_LIMIT_STORE") != "memory" {
		client, err := firestore.NewClient(ctx, project)
		if err != nil {
			log.Printf("ai: firestore init failed: %v (rate limit counts in memory)", err)
		} else {
			store = ratelimit.NewFirestore(client, os.Getenv("RATE_LIMIT_COLLECTION"))
			storeName = "firestore"
		}
	}
	log.Printf("ai: rate limit on (store=%s env=%q limits=%+v weights=%+v)", storeName, cfg.Env, cfg.Limits, cfg.Weights)
	// Pro has no server-side receipt check yet (#280): nil = everyone is anonymous/signed_in.
	return ratelimit.New(cfg, store, nil).Middleware(verifier)(h)
}
