package appconfig

// ISBN lookup: POST /v1/lookup/book {"isbn"} fills a form from a book's ISBN
// (DSL lookupBook). GET ?isbn= is the same lookup, kept for older iOS builds
// (lookup_body.go: user-entered values stay out of URLs). Same model as the external feeds (#348): the server is the
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
		mux.HandleFunc("GET /v1/lookup/book", h.lookupBook) // deprecated: older iOS builds
		mux.HandleFunc("POST /v1/lookup/book", h.lookupBookBody)
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

// lookupBook: GET /v1/lookup/book?isbn= (deprecated)
func (h *Handler) lookupBook(w http.ResponseWriter, r *http.Request) {
	vals := r.URL.Query()
	for k, v := range vals {
		if k != "isbn" || len(v) != 1 {
			fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "unknown or repeated query parameter").With("field", k))
			return
		}
	}
	h.serveBook(w, r, vals.Get("isbn"))
}

// lookupBookBody: POST /v1/lookup/book {"isbn"}
func (h *Handler) lookupBookBody(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ISBN string `json:"isbn"`
	}
	if !decodeLookupBody(w, r, &in) {
		return
	}
	h.serveBook(w, r, in.ISBN)
}

// serveBook is the lookup shared by GET and POST (checks, cap, fetch, reply).
func (h *Handler) serveBook(w http.ResponseWriter, r *http.Request, isbn string) {
	// Off unless BOOK_LOOKUP_ENABLED (books.EnabledFromEnv): 503 for every
	// call, so a saved lookupBook button shows "not available now".
	if !h.books.Enabled() {
		fail(w, apiErr(http.StatusServiceUnavailable, httpx.CodeUpstreamUnavailable, "本の情報の取得は今使えません").With("reason", "disabled"))
		return
	}
	// Checked before the per-user count and before any fetch.
	if _, ok := books.NormalizeISBN(isbn); !ok {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "ISBN を確かめてください(10 桁か 13 桁)").With("field", "isbn"))
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
	book, err := h.books.Lookup(ctx, isbn)
	switch {
	case errors.Is(err, books.ErrInvalidISBN):
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "ISBN を確かめてください(10 桁か 13 桁)").With("field", "isbn"))
	case errors.Is(err, books.ErrNotFound):
		fail(w, apiErr(http.StatusNotFound, httpx.CodeNotFound, "この ISBN の本は見つかりませんでした"))
	case errors.Is(err, books.ErrDisabled):
		fail(w, apiErr(http.StatusServiceUnavailable, httpx.CodeUpstreamUnavailable, "本の情報の取得は今使えません").With("reason", "disabled"))
	case err != nil:
		w.Header().Set("Retry-After", "60")
		fail(w, apiErr(http.StatusServiceUnavailable, httpx.CodeUpstreamUnavailable, "本の情報の提供元に今つながりません"))
	default:
		w.Header().Set("Cache-Control", "private, max-age=3600")
		httpx.WriteJSON(w, http.StatusOK, book)
	}
}
