package appconfig

// Reports, blocks and operator unpublishing (#321, App Review 1.2). Reports
// are stored as reports/{id} and logged as "moderation: report" for the
// operator's alert (docs/infra/moderation.md); the operator unpublishes with
// cmd/moderate. Blocked users' listings and shared posts are left out of the
// blocker's GET /public-apps and GET .../records.

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// ReportsPerDay caps new reports per reporter per JST day.
const ReportsPerDay = 30

// CodeUnpublished is the error code for content the operator unpublished.
const CodeUnpublished = "unpublished"

// reportReasons are the accepted report reasons (iOS shows one per row).
var reportReasons = map[string]bool{
	"spam": true, "sexual": true, "violence": true, "harassment": true,
	"illegal": true, "privacy": true, "other": true,
}

// unpublishedErr tells everyone that the operator stopped this; the owner
// sees why (the reason is written for the author).
func unpublishedErr(what string, st store.ModerationState) *apiError {
	return apiErr(http.StatusForbidden, CodeUnpublished, what+" was unpublished by the operator").With("reason", st.HiddenReason)
}

// ---- reports ----------------------------------------------------------------

// createReport: POST /v1/reports.
//
//	{"kind":"listing","id":"<public app id>","reason":"spam"}
//	{"kind":"share","id":"<token>","reason":...}
//	{"kind":"app","appId":"...","reason":...}           (a shared app)
//	{"kind":"record","appId":"...","entityId":"...","recordId":"...","reason":...}
//
// 201 {id, created:true}; reporting the same target again is 200 created:false.
func (h *Handler) createReport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind     string `json:"kind"`
		ID       string `json:"id"`
		AppID    string `json:"appId"`
		EntityID string `json:"entityId"`
		RecordID string `json:"recordId"`
		Reason   string `json:"reason"`
	}
	if !httpx.DecodeBody(w, r, &body) {
		return
	}
	if !reportReasons[body.Reason] {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "unknown reason"))
		return
	}
	target, owner, err := h.reportTarget(r, body.Kind, body.ID, body.AppID, body.EntityID, body.RecordID)
	if err != nil {
		fail(w, err)
		return
	}
	uid := uidOf(r)
	if owner == uid {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "cannot report your own content"))
		return
	}
	now := time.Now().UTC()
	rep := store.Report{ID: store.ReportID(uid, body.Kind, target), ReporterUID: uid, Kind: body.Kind, TargetID: target,
		TargetUID: owner, Reason: body.Reason, Status: "open", CreatedAt: now}
	created, err := h.store.AddReport(r.Context(), rep, now.In(ratelimit.JST).Format(time.DateOnly), ReportsPerDay)
	if errors.Is(err, store.ErrRateLimited) {
		fail(w, apiErr(http.StatusTooManyRequests, httpx.CodeRateLimited, "too many reports today"))
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if created {
		// The operator's alert keys on this message. No uid, no content.
		slog.Warn("moderation: report", slog.String("reportId", rep.ID), slog.String("kind", rep.Kind),
			slog.String("targetId", rep.TargetID), slog.String("reason", rep.Reason))
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httpx.WriteJSON(w, status, map[string]any{"id": rep.ID, "created": created})
}

// reportTarget checks that the reported thing exists (and that the caller
// can see it) and returns its target id and owner / author uid.
func (h *Handler) reportTarget(r *http.Request, kind, id, appID, entityID, recordID string) (string, string, error) {
	ctx := r.Context()
	bad := func(msg string) (string, string, error) {
		return "", "", apiErr(http.StatusBadRequest, httpx.CodeBadRequest, msg)
	}
	switch kind {
	case store.ReportListing:
		if !idRe.MatchString(id) {
			return bad("invalid id")
		}
		c, err := h.store.GetCatalogApp(ctx, id)
		if err != nil {
			return "", "", mapStoreErr(err, "public app")
		}
		return id, c.OwnerUID, nil
	case store.ReportShare:
		if !idRe.MatchString(id) {
			return bad("invalid id")
		}
		s, err := h.store.GetShare(ctx, id)
		if err != nil {
			return "", "", mapStoreErr(err, "share")
		}
		return id, s.OwnerUID, nil
	case store.ReportApp:
		if !validAppID(appID) || !idRe.MatchString(appID) {
			return bad("invalid appId")
		}
		p, err := h.store.GetPublicApp(ctx, appID)
		if err != nil {
			return "", "", mapStoreErr(err, "shared app")
		}
		return appID, p.OwnerUID, nil
	case store.ReportRecord:
		if !validAppID(appID) || !idRe.MatchString(appID) || !idRe.MatchString(entityID) || !idRe.MatchString(recordID) {
			return bad("invalid appId / entityId / recordId")
		}
		rec, err := h.store.GetRecord(ctx, store.PublicRecords(appID, entityID), recordID)
		if err != nil {
			// only posts in shared apps can be reported: private records are the caller's own
			return "", "", mapStoreErr(err, "record")
		}
		return appID + "/" + entityID + "/" + recordID, rec.Author, nil
	}
	return bad("kind must be listing, share, app or record")
}

// ---- blocks -----------------------------------------------------------------

type blockOut struct {
	UID       string    `json:"uid"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

func toBlockOut(b store.Block) blockOut {
	name := b.Name
	if name == "" {
		name = defaultAuthorName
	}
	return blockOut{UID: b.UID, Name: name, CreatedAt: b.CreatedAt}
}

// listBlocks: GET /v1/me/blocks.
func (h *Handler) listBlocks(w http.ResponseWriter, r *http.Request) {
	bs, err := h.store.ListBlocks(r.Context(), uidOf(r))
	if err != nil {
		fail(w, err)
		return
	}
	items := make([]blockOut, 0, len(bs))
	for _, b := range bs {
		items = append(items, toBlockOut(b))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

// createBlock: POST /v1/me/blocks {"uid","name"} (a post's author) or
// {"listingId"} (the publisher of a 探す listing; the uid is resolved here).
func (h *Handler) createBlock(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UID       string `json:"uid"`
		Name      string `json:"name"`
		ListingID string `json:"listingId"`
	}
	if !httpx.DecodeBody(w, r, &body) {
		return
	}
	b := store.Block{UID: body.UID, Name: strings.TrimSpace(body.Name)}
	switch {
	case body.ListingID != "" && body.UID == "":
		if !idRe.MatchString(body.ListingID) {
			fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "invalid listingId"))
			return
		}
		c, err := h.store.GetCatalogApp(r.Context(), body.ListingID)
		if err != nil {
			fail(w, mapStoreErr(err, "public app"))
			return
		}
		b.UID = c.OwnerUID
		if b.Name == "" {
			b.Name = c.AuthorName
		}
	case body.ListingID == "" && idRe.MatchString(body.UID):
	default:
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "give either uid or listingId"))
		return
	}
	if b.UID == uidOf(r) {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "cannot block yourself"))
		return
	}
	if tooLong(b.Name, maxAuthorNameLen) {
		b.Name = string([]rune(b.Name)[:maxAuthorNameLen])
	}
	bs, err := h.store.ListBlocks(r.Context(), uidOf(r))
	if err != nil {
		fail(w, err)
		return
	}
	if len(bs) >= store.MaxBlocks && !store.BlockedSet(bs)[b.UID] {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "too many blocked users"))
		return
	}
	b.CreatedAt = time.Now().UTC()
	if err := h.store.PutBlock(r.Context(), uidOf(r), b); err != nil {
		fail(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toBlockOut(b))
}

// deleteBlock: DELETE /v1/me/blocks/{uid} (idempotent).
func (h *Handler) deleteBlock(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "uid")
	if err != nil {
		fail(w, err)
		return
	}
	if err := h.store.DeleteBlock(r.Context(), uidOf(r), id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// blockedBy returns the caller's blocked uids (nil when none).
func (h *Handler) blockedBy(r *http.Request) (map[string]bool, error) {
	bs, err := h.store.ListBlocks(r.Context(), uidOf(r))
	if err != nil || len(bs) == 0 {
		return nil, err
	}
	return store.BlockedSet(bs), nil
}

// ---- operator unpublish -----------------------------------------------------

// checkSharedApp refuses shared / public data of an app the operator unpublished.
func (h *Handler) checkSharedApp(r *http.Request, appID string) error {
	p, err := h.store.GetPublicApp(r.Context(), appID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return nil
	case err != nil:
		return err
	case p.Hidden:
		return unpublishedErr("this shared app", p.ModerationState)
	}
	return nil
}
