package coinsapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// internal answers 500 with a fixed message and never echoes the cause.
func TestInternalHidesCause_3278(t *testing.T) {
	for _, what := range []string{"wallet", "ledger", "settings", "slot"} {
		t.Run(what, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/v1/me/coins", nil)
			internal(w, r, what, errors.New("firestore: secret-path users/u1"))
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500", w.Code)
			}
			var body httpx.ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("body %q: %v", w.Body.String(), err)
			}
			if body.Error.Code != httpx.CodeInternal || body.Error.Message != "internal error" {
				t.Fatalf("error = %+v", body.Error)
			}
			if strings.Contains(w.Body.String(), "secret-path") {
				t.Fatalf("cause leaked: %s", w.Body.String())
			}
		})
	}
}
