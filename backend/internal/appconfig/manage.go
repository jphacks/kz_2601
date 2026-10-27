package appconfig

// Organising the app list from 変身一覧 (#277): rename, duplicate, reorder.
// Name and order live on the server (users/{uid}/apps) so they follow the uid
// to a new device (#276 keeps the uid when linking Apple ID).

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

const (
	maxAppNameRunes = 40
	maxReorderIDs   = 400 // one Firestore transaction (500 writes) with room to spare
)

// appName trims and checks a user-typed app name.
func appName(s string) (string, error) {
	name := strings.TrimSpace(s)
	if name == "" || utf8.RuneCountInString(name) > maxAppNameRunes {
		return "", apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "name must be 1-40 characters")
	}
	return name, nil
}

// renamedDSL sets dsl.app.name. The name is kept in the DSL (not only in the
// app document) because every commit copies dsl.app.name back into the app
// document; a meta-only rename would be undone by the next voice edit.
func renamedDSL(raw json.RawMessage, name string) (json.RawMessage, *dsl.App, []dsl.PatchOp, error) {
	v, err := json.Marshal(name)
	if err != nil {
		return nil, nil, nil, err
	}
	ops := []dsl.PatchOp{{Op: "add", Path: "/app/name", Value: v}}
	next, app, err := dsl.ApplyPatch(raw, ops)
	if err != nil {
		return nil, nil, nil, invalidDSL(err)
	}
	return next, app, ops, nil
}

// rename commits dsl.app.name = name as a new version (source.kind=manual),
// so the app list, the in-app title and the next AI edit all agree.
func (h *Handler) rename(r *http.Request, id, name string) error {
	cur, err := h.store.GetApp(r.Context(), uidOf(r), id)
	if err != nil {
		return mapStoreErr(err, "app")
	}
	next, app, ops, err := renamedDSL(cur.DSL, name)
	if err != nil {
		return err
	}
	if cur.Name == name && string(compact(next)) == string(compact(cur.DSL)) {
		return nil
	}
	v, err := h.store.CommitVersion(r.Context(), uidOf(r), id, store.Commit{
		BaseVersion: cur.CurrentVersion, DSL: next, Name: name, Patch: ops,
		Source: &store.Source{Kind: "manual", Mode: "patch", Summary: "名前を「" + name + "」に変更"},
	})
	if err != nil {
		return mapStoreErr(err, "app")
	}
	h.publish(r, id, app, next, v.Version)
	return nil
}

// duplicateApp: POST /v1/apps/{appId}/duplicate (#277). Copies the current
// DSL (screens only, no records) to a new appId in the caller's tree. Unlike
// installCopy the id is always new, even for apps with shared entities: the
// point is "another 家計簿 (for work)", so it starts with an empty board.
func (h *Handler) duplicateApp(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if r.ContentLength != 0 {
		if !httpx.DecodeBody(w, r, &body) {
			return
		}
	}
	src, err := h.store.GetApp(r.Context(), uidOf(r), id)
	if err != nil {
		fail(w, mapStoreErr(err, "app"))
		return
	}
	if body.Name == "" {
		body.Name = src.Name
	}
	name, err := appName(body.Name)
	if err != nil {
		fail(w, err)
		return
	}
	raw, app, _, err := renamedDSL(src.DSL, name)
	if err != nil {
		fail(w, err)
		return
	}
	newID := newAppID()
	v, err := h.seedCopy(r, newID, raw, name, &store.Source{Kind: "import", Mode: "full", Summary: "「" + src.Name + "」を複製"})
	if err != nil {
		fail(w, err)
		return
	}
	h.claim(r, newID)
	h.publish(r, newID, app, raw, v.Version)
	created, err := h.store.GetApp(r.Context(), uidOf(r), newID)
	if err != nil {
		fail(w, err)
		return
	}
	slog.Info("appconfig: app duplicated", slog.String("from", id), slog.String("appId", newID), slog.Int("version", v.Version))
	w.Header().Set("ETag", etag(created.CurrentVersion))
	httpx.WriteJSON(w, http.StatusCreated, appWithDSL{appSummary: summary(created), DSL: created.DSL})
}

// seedCopy creates id at version 1 (initial DSL) and commits raw as version 2,
// so iOS does not treat the copy as a blank, never-generated app.
func (h *Handler) seedCopy(r *http.Request, id string, raw json.RawMessage, name string, src *store.Source) (*store.Version, error) {
	init := dsl.Initial()
	initRaw, err := json.Marshal(init)
	if err != nil {
		return nil, err
	}
	if _, _, err := h.store.EnsureApp(r.Context(), uidOf(r), id, initRaw, init.App.Name); err != nil {
		return nil, err
	}
	v, err := h.store.CommitVersion(r.Context(), uidOf(r), id, store.Commit{BaseVersion: 1, DSL: raw, Name: name, Source: src})
	if err != nil {
		return nil, mapStoreErr(err, "app")
	}
	return v, nil
}

// reorderApps: PUT /v1/apps/order (#277). ids is the list as the user arranged
// it; each gets sortIndex = position+1. Apps not in ids keep their index (new
// apps have none and are listed first by iOS).
func (h *Handler) reorderApps(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if !httpx.DecodeBody(w, r, &body) {
		return
	}
	if len(body.IDs) > maxReorderIDs {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "too many ids"))
		return
	}
	seen := make(map[string]bool, len(body.IDs))
	for _, id := range body.IDs {
		if !validAppID(id) || seen[id] {
			fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "ids must be distinct app ids"))
			return
		}
		seen[id] = true
	}
	if err := h.store.ReorderApps(r.Context(), uidOf(r), body.IDs); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
