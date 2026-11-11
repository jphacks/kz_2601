package appconfig

// The "18 or older" self-declaration of matching apps (#507). The server
// keeps that the person said so (users/{uid}.ageConfirmedAt, no age), and
// the matching endpoints answer only people who did:
//   - interests / matches: 400 age_unconfirmed
//   - records of the profile / matched entities: only the caller's own
//   - a new profile record: 400 age_unconfirmed
//
// A send / accept with "ageConfirmed":true also records it.

import (
	"net/http"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// routes (matching age #507)
func init() {
	registerRoutes(func(h *Handler, mux *http.ServeMux) {
		mux.HandleFunc("GET /v1/me/age-confirmation", h.getAgeConfirmation)
		mux.HandleFunc("PUT /v1/me/age-confirmation", h.putAgeConfirmation)
	})
}

// getAgeConfirmation: GET /v1/me/age-confirmation -> {"ageConfirmed":bool}.
func (h *Handler) getAgeConfirmation(w http.ResponseWriter, r *http.Request) {
	ok, err := h.store.AgeConfirmed(r.Context(), uidOf(r))
	if err != nil {
		fail(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ageConfirmed": ok})
}

// putAgeConfirmation: PUT /v1/me/age-confirmation {"ageConfirmed":true}.
func (h *Handler) putAgeConfirmation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AgeConfirmed bool `json:"ageConfirmed"`
	}
	if !httpx.DecodeBody(w, r, &body) {
		return
	}
	if err := h.requireAge(r, body.AgeConfirmed); err != nil {
		fail(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ageConfirmed": true})
}

// requireAge records the declaration when the request carries it, else
// needs the recorded one.
func (h *Handler) requireAge(r *http.Request, declared bool) error {
	ok, err := h.store.AgeConfirmed(r.Context(), uidOf(r))
	switch {
	case err != nil:
		return err
	case ok:
		return nil
	case declared:
		return h.store.ConfirmAge(r.Context(), uidOf(r))
	}
	return ageUnconfirmed()
}

// checkProfileWrite: posting to the profile entity of a matching app needs
// the declaration (the profile is what strangers see).
func (h *Handler) checkProfileWrite(r *http.Request, appID, eid string) error {
	spec, _, err := h.publishedMatch(r.Context(), appID)
	if err != nil || spec == nil || spec.profile == nil || spec.profile.ID != eid {
		return err
	}
	return h.requireAge(r, false)
}

// sharedFilter returns which authors' records of the shared entity eid the
// caller may read: never people blocked either way (#321), plus the
// profile / matched-only rules of matching apps (matchFilter). authors are
// the authors of the records at hand.
func (h *Handler) sharedFilter(r *http.Request, appID, eid string, authors []string) (func(author string) bool, error) {
	me := uidOf(r)
	blocked, err := h.store.BlockedEither(r.Context(), me, othersOf(authors, me))
	if err != nil {
		return nil, err
	}
	match, err := h.matchFilter(r, appID, eid)
	if err != nil {
		return nil, err
	}
	return func(a string) bool {
		return a == me || (!blocked[a] && (match == nil || match(a)))
	}, nil
}
