package httpx

import "net/http"

// DebugErrorPath answers on purpose with a 5xx (default) or a panic
// (?kind=panic), to check the 5xx alert and Error Reporting on stg (#295,
// docs/infra/observability.md). Mounted only by DebugErrors(true).
const DebugErrorPath = "/debug/error"

// DebugErrors serves DebugErrorPath when enabled (APP_ENV=stg); otherwise it
// returns next unchanged. Put it inside Recover.
func DebugErrors(enabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if !enabled {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != DebugErrorPath || r.Method != http.MethodGet {
				next.ServeHTTP(w, r)
				return
			}
			if r.URL.Query().Get("kind") == "panic" {
				panic("debug: intentional panic (" + DebugErrorPath + ")")
			}
			Error(w, http.StatusInternalServerError, CodeInternal, "debug: intentional 5xx")
		})
	}
}
