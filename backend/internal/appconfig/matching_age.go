package appconfig

// The "18 or older" self-declaration of matching apps (#507). The server
// keeps that the person said so (users/{uid}.ageConfirmedAt, no age), and
// the matching endpoints answer only people who did:
//   - interests / matches: 400 age_unconfirmed
//   - records of the profile / matched entities: only the caller's own
//   - a new profile record: 400 age_unconfirmed
//   - profile records of people who have not declared (made before the
//     check): shown to nobody but their author. The profile record carries
//     ownerAgeConfirmed once its author declared, so a list reads the
//     declaration (users/{uid}) only for records without it.
//
// A send / accept with "ageConfirmed":true also records it.

import (
	"context"
	"net/http"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
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
// the declaration (the profile is what strangers see). profile reports
// that eid is that entity.
func (h *Handler) checkProfileWrite(r *http.Request, appID, eid string) (profile bool, err error) {
	spec, _, err := h.publishedMatch(r.Context(), appID)
	if err != nil || spec == nil || spec.profile == nil || spec.profile.ID != eid {
		return false, err
	}
	return true, h.requireAge(r, false)
}

// sharedFilter returns which of recs (records of the shared entity eid) the
// caller may read: never people blocked either way (#321), plus the
// profile / matched-only rules of matching apps (matchFilter).
func (h *Handler) sharedFilter(r *http.Request, appID, eid string, recs []store.Record) (func(rec *store.Record) bool, error) {
	me := uidOf(r)
	blocked, err := h.store.BlockedEither(r.Context(), me, othersOf(recordAuthors(recs), me))
	if err != nil {
		return nil, err
	}
	match, err := h.matchFilter(r, appID, eid, recs)
	if err != nil {
		return nil, err
	}
	return func(rec *store.Record) bool {
		a := rec.Author
		return a == me || (!blocked[a] && (match == nil || match(rec)))
	}, nil
}

// declaredOwners returns the authors other than me of the records without
// OwnerAgeConfirmed who made the declaration (one batched read; none when
// every record carries it).
func (h *Handler) declaredOwners(ctx context.Context, recs []store.Record, me string) (map[string]bool, error) {
	var unknown []string
	for i := range recs {
		if !recs[i].OwnerAgeConfirmed {
			unknown = append(unknown, recs[i].Author)
		}
	}
	unknown = othersOf(unknown, me)
	if len(unknown) == 0 {
		return map[string]bool{}, nil
	}
	return h.store.AgeConfirmedAmong(ctx, unknown)
}

// ownerDeclared: rec's author made the declaration (declared from declaredOwners).
func ownerDeclared(rec *store.Record, declared map[string]bool) bool {
	return rec.OwnerAgeConfirmed || declared[rec.Author]
}
