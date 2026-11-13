package httpx

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// flushHandler2759 checks the wrapper it is given exposes Unwrap and that
// http.ResponseController can flush through it.
func flushHandler2759(t *testing.T, under *httptest.ResponseRecorder) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		u, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			t.Errorf("%T has no Unwrap", w)
		} else if u.Unwrap() != http.ResponseWriter(under) {
			t.Errorf("%T.Unwrap() is not the underlying writer", w)
		}
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("Flush through %T: %v", w, err)
		}
	})
}

func TestResponseRecorderUnwrap2759(t *testing.T) {
	rec := httptest.NewRecorder()
	h := Log(slog.New(slog.DiscardHandler))(flushHandler2759(t, rec))
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if !rec.Flushed {
		t.Fatal("underlying writer not flushed")
	}
}

func TestEnvelopeWriterUnwrap2759(t *testing.T) {
	rec := httptest.NewRecorder()
	ErrorEnvelope(flushHandler2759(t, rec)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if !rec.Flushed {
		t.Fatal("underlying writer not flushed")
	}
}
