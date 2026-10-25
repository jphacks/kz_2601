package appconfig

// Release requests (#76): the owner asks for an app to be shipped as its own
// standalone app. The API only files the request (status "received") with a
// DSL snapshot; an admin moves status in the Firestore console
// (architecture.md §3.3).

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// Release request field limits (runes).
const (
	maxReleasePurpose = 200
	maxReleaseContact = 100
)

type releaseOut struct {
	ID         string    `json:"id"`
	AppID      string    `json:"appId"`
	Name       string    `json:"name"`
	DSLVersion int       `json:"dslVersion"`
	Purpose    string    `json:"purpose"`
	Contact    string    `json:"contact"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func toReleaseOut(rr *store.ReleaseRequest) releaseOut {
	return releaseOut{ID: rr.ID, AppID: rr.AppID, Name: rr.Name, DSLVersion: rr.Version, Purpose: rr.Purpose,
		Contact: rr.Contact, Status: rr.Status, CreatedAt: rr.CreatedAt, UpdatedAt: rr.UpdatedAt}
}

func writeReleases(w http.ResponseWriter, rs []store.ReleaseRequest) {
	items := make([]releaseOut, 0, len(rs))
	for i := range rs {
		items = append(items, toReleaseOut(&rs[i]))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

// createReleaseRequest: POST /v1/apps/{appId}/release-requests. Owner only
// (the uid-scoped lookup 404s on anyone else's app). Idempotent: while a
// request for the app is open, the existing one comes back with 200.
func (h *Handler) createReleaseRequest(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	var body struct {
		Purpose string `json:"purpose"`
		Contact string `json:"contact"`
	}
	if !httpx.DecodeBody(w, r, &body) {
		return
	}
	purpose, contact := strings.TrimSpace(body.Purpose), strings.TrimSpace(body.Contact)
	switch {
	case purpose == "":
		err = fmt.Errorf("purpose is required")
	case tooLong(purpose, maxReleasePurpose):
		err = fmt.Errorf("purpose must be at most %d characters", maxReleasePurpose)
	case tooLong(contact, maxReleaseContact):
		err = fmt.Errorf("contact must be at most %d characters", maxReleaseContact)
	}
	if err != nil {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, err.Error()))
		return
	}
	app, err := h.store.GetApp(r.Context(), uidOf(r), id)
	if err != nil {
		fail(w, mapStoreErr(err, "app"))
		return
	}
	rr, created, err := h.store.CreateReleaseRequest(r.Context(), store.ReleaseRequest{
		AppID: id, OwnerUID: uidOf(r), Name: app.Name, Version: app.CurrentVersion, DSL: app.DSL,
		Purpose: purpose, Contact: contact,
	})
	if err != nil {
		fail(w, err)
		return
	}
	code := http.StatusOK
	if created {
		code = http.StatusCreated
	}
	httpx.WriteJSON(w, code, toReleaseOut(rr))
}

// listAppReleaseRequests: GET /v1/apps/{appId}/release-requests (owner only).
func (h *Handler) listAppReleaseRequests(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	if _, err := h.store.GetApp(r.Context(), uidOf(r), id); err != nil {
		fail(w, mapStoreErr(err, "app"))
		return
	}
	rs, err := h.store.ListReleaseRequests(r.Context(), uidOf(r), id)
	if err != nil {
		fail(w, err)
		return
	}
	writeReleases(w, rs)
}

// listMyReleaseRequests: GET /v1/release-requests — every app of the caller,
// so the app switcher can badge all cards with one call.
func (h *Handler) listMyReleaseRequests(w http.ResponseWriter, r *http.Request) {
	rs, err := h.store.ListReleaseRequests(r.Context(), uidOf(r), "")
	if err != nil {
		fail(w, err)
		return
	}
	writeReleases(w, rs)
}
