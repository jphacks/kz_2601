package appconfig

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

type appSummary struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	CurrentVersion int       `json:"currentVersion"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type appWithDSL struct {
	appSummary
	DSL json.RawMessage `json:"dsl"`
}

func summary(a *store.App) appSummary {
	return appSummary{ID: a.ID, Name: a.Name, CurrentVersion: a.CurrentVersion, Status: a.Status, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt}
}

func (h *Handler) listApps(w http.ResponseWriter, r *http.Request) {
	p, err := parsePaging(r)
	if err != nil {
		fail(w, err)
		return
	}
	apps, err := h.store.ListApps(r.Context(), uidOf(r), p.size+1, p.offset)
	if err != nil {
		fail(w, err)
		return
	}
	items := make([]appSummary, 0, len(apps))
	for i := range apps {
		if i == p.size {
			break
		}
		items = append(items, summary(&apps[i]))
	}
	resp := map[string]any{"items": items}
	if nt := p.next(len(apps)); nt != "" {
		resp["nextPageToken"] = nt
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) createApp(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID   string          `json:"id"`
		Name string          `json:"name"`
		DSL  json.RawMessage `json:"dsl"`
	}
	if r.ContentLength != 0 {
		if !httpx.DecodeBody(w, r, &body) {
			return
		}
	}
	var raw json.RawMessage
	var app *dsl.App
	if len(body.DSL) > 0 {
		a, err := dsl.Parse(body.DSL)
		if err != nil {
			fail(w, invalidDSL(err))
			return
		}
		app, raw = a, compact(body.DSL)
	} else {
		init := dsl.Initial()
		b, err := json.Marshal(init)
		if err != nil {
			fail(w, err)
			return
		}
		app, raw = &init, b
	}
	name := body.Name
	if name == "" {
		name = app.App.Name
	}
	id := body.ID
	if id == "" {
		id = store.NewID("app_")
	} else if !validAppID(id) {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "invalid id"))
		return
	}
	created, isNew, err := h.store.EnsureApp(r.Context(), uidOf(r), id, raw, name)
	if err != nil {
		fail(w, err)
		return
	}
	if !isNew {
		fail(w, apiErr(http.StatusConflict, httpx.CodeVersionConflict, "app already exists").With("currentVersion", created.CurrentVersion))
		return
	}
	if len(body.DSL) > 0 {
		h.claim(r, id)
	}
	h.publish(r, id, app, raw, created.CurrentVersion)
	w.Header().Set("ETag", etag(created.CurrentVersion))
	httpx.WriteJSON(w, http.StatusCreated, appWithDSL{appSummary: summary(created), DSL: created.DSL})
}

func (h *Handler) getApp(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	app, err := h.store.GetApp(r.Context(), uidOf(r), id)
	if err != nil {
		fail(w, mapStoreErr(err, "app"))
		return
	}
	w.Header().Set("ETag", etag(app.CurrentVersion))
	httpx.WriteJSON(w, http.StatusOK, appWithDSL{appSummary: summary(app), DSL: app.DSL})
}

func (h *Handler) patchApp(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	var body struct {
		Name   *string `json:"name"`
		Status *string `json:"status"`
	}
	if !httpx.DecodeBody(w, r, &body) {
		return
	}
	if body.Status != nil && *body.Status != "active" && *body.Status != "archived" {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "status must be active or archived"))
		return
	}
	app, err := h.store.UpdateAppMeta(r.Context(), uidOf(r), id, body.Name, body.Status)
	if err != nil {
		fail(w, mapStoreErr(err, "app"))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, summary(app))
}

// deleteApp: DELETE /v1/apps/{appId} (#180). Physically deletes the caller's
// app (document + versions + records) together with what the caller made
// public for it: their share links, their "探す" listing (+ installs) and
// their apps_public mirror (+ shared records). Only documents the caller owns
// are touched, so another user's app is a 404 and another user's listing
// stays. Copies other people installed from the catalog are separate appIds
// and stay; a member who joined the appId only deletes their own copy.
func (h *Handler) deleteApp(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	res, err := h.store.PurgeApp(r.Context(), uidOf(r), id)
	if err != nil {
		fail(w, mapStoreErr(err, "app"))
		return
	}
	slog.Info("appconfig: app deleted", slog.String("appId", id), slog.Int("shares", res.Shares),
		slog.Bool("listing", res.Listing), slog.Bool("mirror", res.Mirror),
		slog.Int("versions", res.Versions), slog.Int("records", res.Records))
	w.WriteHeader(http.StatusNoContent)
}
