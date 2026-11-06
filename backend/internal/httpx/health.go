package httpx

import "net/http"

// HealthInfo is the public GET /health body (#290): enough to tell stg from
// prod and which build answers. Only non-secret identifiers belong here (no
// project number, service account, URLs or ops flags).
type HealthInfo struct {
	Status            string `json:"status"`                      // always "ok" (Uptime check matches it)
	Env               string `json:"env,omitempty"`               // APP_ENV
	FirestoreDatabase string `json:"firestoreDatabase,omitempty"` // FIRESTORE_DATABASE; empty when Firestore is not used
	Version           string `json:"version,omitempty"`           // GIT_SHA, 7 chars
}

// HealthInfoFromEnv reads HealthInfo once at startup. getenv is os.Getenv
// outside tests.
func HealthInfoFromEnv(getenv func(string) string) HealthInfo {
	info := HealthInfo{Status: "ok", Env: getenv("APP_ENV"), Version: getenv("GIT_SHA")}
	if len(info.Version) > 7 {
		info.Version = info.Version[:7]
	}
	if getenv("GOOGLE_CLOUD_PROJECT") != "" || getenv("FIRESTORE_EMULATOR_HOST") != "" {
		info.FirestoreDatabase = getenv("FIRESTORE_DATABASE")
		if info.FirestoreDatabase == "" {
			info.FirestoreDatabase = "(default)" // fsenv.DefaultDatabase
		}
	}
	return info
}

// Health answers 200 with info as JSON.
func Health(info HealthInfo) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		WriteJSON(w, http.StatusOK, info)
	}
}
