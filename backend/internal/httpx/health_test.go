package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthInfoFromEnv(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want HealthInfo
	}{
		{"local without firestore", nil, HealthInfo{Status: "ok"}},
		{"stg default database", map[string]string{"APP_ENV": "stg", "GOOGLE_CLOUD_PROJECT": "p", "GIT_SHA": "0123456789abcdef"},
			HealthInfo{Status: "ok", Env: "stg", FirestoreDatabase: "(default)", Version: "0123456"}},
		{"prod named database", map[string]string{"APP_ENV": "prod", "GOOGLE_CLOUD_PROJECT": "p", "FIRESTORE_DATABASE": "restore-20261101", "GIT_SHA": "abc"},
			HealthInfo{Status: "ok", Env: "prod", FirestoreDatabase: "restore-20261101", Version: "abc"}},
		{"emulator", map[string]string{"FIRESTORE_EMULATOR_HOST": "localhost:8080"}, HealthInfo{Status: "ok", FirestoreDatabase: "(default)"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HealthInfoFromEnv(func(k string) string { return c.env[k] }); got != c.want {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

// The body is public: only the allowlisted keys, never project / ops flags.
func TestHealthBody(t *testing.T) {
	env := map[string]string{
		"APP_ENV": "stg", "GOOGLE_CLOUD_PROJECT": "secret-project", "GIT_SHA": "0123456789",
		"ALLOW_SHARED_PROD_DB": "1", "APPCONFIG_BASE_URL": "https://internal.example",
	}
	rec := httptest.NewRecorder()
	Health(HealthInfoFromEnv(func(k string) string { return env[k] }))(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("status=%d type=%q", rec.Code, rec.Header().Get("Content-Type"))
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("not JSON: %s", rec.Body.String())
	}
	want := map[string]string{"status": "ok", "env": "stg", "firestoreDatabase": "(default)", "version": "0123456"}
	if len(body) != len(want) {
		t.Fatalf("keys: %v", body)
	}
	for k, v := range want {
		if body[k] != v {
			t.Fatalf("%s=%q, want %q (%s)", k, body[k], v, rec.Body.String())
		}
	}
	for _, leak := range []string{"secret-project", "internal.example", "ALLOW_SHARED"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Fatalf("leaks %q: %s", leak, rec.Body.String())
		}
	}
	// the Uptime check matches the substring "ok" (infra/monitoring.tf)
	if !strings.Contains(rec.Body.String(), "ok") {
		t.Fatalf("no ok: %s", rec.Body.String())
	}
}
