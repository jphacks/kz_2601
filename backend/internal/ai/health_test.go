package ai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// GET /health names the environment and build (#290).
func TestHealthReportsEnv(t *testing.T) {
	t.Setenv("APP_ENV", "stg")
	t.Setenv("GIT_SHA", "fedcba9876543210")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "p")
	t.Setenv("FIRESTORE_DATABASE", "stg")
	svc, _ := textSvc()
	for _, path := range []string{"/health", "/healthz"} {
		rec := httptest.NewRecorder()
		NewHandler(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
		if body["status"] != "ok" || body["env"] != "stg" || body["firestoreDatabase"] != "stg" || body["version"] != "fedcba9" {
			t.Fatalf("%s: %v", path, body)
		}
	}
}
