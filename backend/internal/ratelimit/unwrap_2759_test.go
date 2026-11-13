package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
)

func TestStatusWriterUnwrap2759(t *testing.T) {
	l, _ := newTestLimiter(Limits{SignedIn: 10})
	rec := httptest.NewRecorder()
	h := l.Middleware(auth.DevVerifier{})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		u, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			t.Errorf("%T has no Unwrap", w)
		} else if u.Unwrap() != http.ResponseWriter(rec) {
			t.Errorf("%T.Unwrap() is not the underlying writer", w)
		}
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("Flush through %T: %v", w, err)
		}
	}))
	r := httptest.NewRequest(http.MethodPost, "/v1/generate", strings.NewReader(`{"mode":"edit"}`))
	r.Header.Set("Authorization", "Bearer u-2759")
	h.ServeHTTP(rec, r)
	if !rec.Flushed {
		t.Fatal("underlying writer not flushed")
	}
}
