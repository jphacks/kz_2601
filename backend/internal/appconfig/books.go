package appconfig

// ISBN lookup: GET /v1/lookup/book?isbn= fills a form from a book's ISBN
// (DSL lookupBook). Same model as the external feeds (#348): the server is the
// only party that talks to the source, takes nothing from the request but a
// checked ISBN and sends nothing about the user. internal/books holds the
// source, the address check, the timeout, the body cap and the cache.
// docs/backend/features/book-lookup.md

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/books"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// DefaultBookLookupsPerHour is the per-user request cap (BOOK_LOOKUP_USER_PER_HOUR).
const DefaultBookLookupsPerHour = 60

func init() {
	registerRoutes(func(h *Handler, mux *http.ServeMux) {
		if h.books == nil {
			h.books = books.New(books.OptionsFromEnv(os.Getenv))
		}
		if h.bookUsers == nil {
			h.bookUsers = newFeedLimiter(bookLookupsPerHour(os.Getenv), time.Now)
		}
		mux.HandleFunc("GET /v1/lookup/book", h.lookupBook)
	})
}

// WithBooks replaces the lookup service and the per-user cap (tests).
func WithBooks(s *books.Service, perHour int, now func() time.Time) Option {
	return func(h *Handler) {
		h.books = s
		h.bookUsers = newFeedLimiter(perHour, now)
	}
}

func bookLookupsPerHour(getenv func(string) string) int {
	if n, err := strconv.Atoi(getenv("BOOK_LOOKUP_USER_PER_HOUR")); err == nil && n >= 0 {
		return n
	}
	return DefaultBookLookupsPerHour
}

// lookupBook: GET /v1/lookup/book?isbn=
func (h *Handler) lookupBook(w http.ResponseWriter, r *http.Request) {
	vals := r.URL.Query()
	for k, v := range vals {
		if k != "isbn" || len(v) != 1 {
			fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "unknown or repeated query parameter").With("field", k))
			return
		}
	}
	// Checked before the per-user count and before any fetch.
	if _, ok := books.NormalizeISBN(vals.Get("isbn")); !ok {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "ISBN を確かめてください(10 桁か 13 桁)").With("field", "isbn"))
		return
	}
	if !h.books.Enabled() {
		fail(w, apiErr(http.StatusServiceUnavailable, httpx.CodeUpstreamUnavailable, "本の情報の読み込みは今止めています").With("reason", "disabled"))
		return
	}
	if wait, ok := h.bookUsers.take(uidOf(r)); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(wait))
		httpx.ErrorWithDetails(w, http.StatusTooManyRequests, httpx.CodeRateLimited, "しばらくしてからもう一度試してください。",
			map[string]any{"scope": "user", "limit": h.bookUsers.perHour, "remaining": 0, "retryAfterSec": wait})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	book, err := h.books.Lookup(ctx, vals.Get("isbn"))
	switch {
	case errors.Is(err, books.ErrInvalidISBN):
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "ISBN を確かめてください(10 桁か 13 桁)").With("field", "isbn"))
	case errors.Is(err, books.ErrNotFound):
		fail(w, apiErr(http.StatusNotFound, httpx.CodeNotFound, "この ISBN の本は見つかりませんでした"))
	case errors.Is(err, books.ErrDisabled):
		fail(w, apiErr(http.StatusServiceUnavailable, httpx.CodeUpstreamUnavailable, "本の情報の読み込みは今止めています").With("reason", "disabled"))
	case err != nil:
		w.Header().Set("Retry-After", "60")
		fail(w, apiErr(http.StatusServiceUnavailable, httpx.CodeUpstreamUnavailable, "本の情報の提供元に今つながりません"))
	default:
		w.Header().Set("Cache-Control", "private, max-age=3600")
		httpx.WriteJSON(w, http.StatusOK, book)
	}
}
