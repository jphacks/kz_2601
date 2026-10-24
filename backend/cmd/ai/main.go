// ai service: transcript -> Claude -> DSL (full or JSON Patch) -> appconfig.
package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/ai"
	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
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
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           ai.NewHandler(svc),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      time.Duration(timeoutSec)*time.Second*3 + 10*time.Second,
	}
	log.Printf("ai listening on :%s (model=%s effort=%s logicPass=%v)", port, model, effort, svc.LogicPass)
	log.Fatal(srv.ListenAndServe())
}
