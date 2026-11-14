package appconfig

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

func TestValidShareToken(t *testing.T) {
	for _, tok := range []string{"AbC_12-x", "a.b", "a:b", ".a", strings.Repeat("a", 128)} {
		if !validShareToken(tok) {
			t.Errorf("%q should be valid", tok)
		}
	}
	for _, tok := range []string{"", ".", "..", "...", "a/b", "a b", "é", strings.Repeat("a", 129)} {
		if validShareToken(tok) {
			t.Errorf("%q should be invalid", tok)
		}
	}
}

// The mux cleans dot segments before routing, so call the handlers directly
// to check they reject such tokens on their own.
func TestShareHandlersRejectBadToken(t *testing.T) {
	h := &Handler{store: store.NewMemory()}
	handlers := map[string]http.HandlerFunc{"get": h.getShare, "revoke": h.revokeShare, "import": h.importShare}
	for name, fn := range handlers {
		for _, tok := range []string{".", "..", "a/b"} {
			req := httptest.NewRequest(http.MethodGet, "/v1/shares/x", nil)
			req.SetPathValue("token", tok)
			rec := httptest.NewRecorder()
			fn(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s %q: want 400, got %d %s", name, tok, rec.Code, rec.Body)
			}
		}
	}
}
