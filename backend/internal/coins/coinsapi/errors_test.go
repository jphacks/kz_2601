package coinsapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The internal 500 says only "internal error": the cause stays in the log.
func TestErrorStrings_S7Backend009(t *testing.T) {
	t.Parallel()
	for _, what := range []string{"wallet", "ledger", "settings", "slot", "credit"} {
		t.Run(what, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/v1/coins/"+what, nil)
			internal(w, r, what, errors.New("firestore: secret-cause-detail"))
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status %d", w.Code)
			}
			if strings.Contains(w.Body.String(), "secret-cause-detail") || strings.Contains(w.Body.String(), what) {
				t.Fatalf("body leaks the cause: %s", w.Body)
			}
			var body struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error.Code != "internal" || body.Error.Message != "internal error" {
				t.Fatalf("body %+v", body)
			}
		})
	}
}
