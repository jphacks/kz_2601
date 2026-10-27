// Package appconfig implements openapi-appconfig.yaml on top of store.Store.
// Plumbing (error envelope, request id, logging, auth) is internal/httpx and
// internal/auth; this package only adds the routes.
package appconfig

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Limits.
const (
	maxRecordBody = 300 << 10 // per record (photo data URLs included)
	defaultPage   = 50
	maxPage       = 200
)

var idRe = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

// Handler serves /v1/*. Authentication is applied by the caller (auth.Middleware).
type Handler struct {
	store store.Store

	mu    sync.Mutex
	cache map[string]*dsl.App // uid/appId/version -> parsed DSL (records validation)

	eventsMax time.Duration // lifetime of one GET /events stream
}

// New builds the v1 router (a ServeMux so the caller can probe matches).
func New(s store.Store) *http.ServeMux {
	h := &Handler{store: s, cache: map[string]*dsl.App{}, eventsMax: eventsMaxStream()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/apps", h.listApps)
	mux.HandleFunc("POST /v1/apps", h.createApp)
	mux.HandleFunc("GET /v1/apps/{appId}", h.getApp)
	mux.HandleFunc("PATCH /v1/apps/{appId}", h.patchApp)
	mux.HandleFunc("DELETE /v1/apps/{appId}", h.deleteApp)
	mux.HandleFunc("PUT /v1/apps/order", h.reorderApps)               // #277 manage.go
	mux.HandleFunc("POST /v1/apps/{appId}/duplicate", h.duplicateApp) // #277 manage.go

	mux.HandleFunc("GET /v1/apps/{appId}/dsl", h.getDSL)
	mux.HandleFunc("GET /v1/apps/{appId}/events", h.events)
	mux.HandleFunc("PUT /v1/apps/{appId}/dsl", h.putDSL)
	mux.HandleFunc("PATCH /v1/apps/{appId}/dsl", h.patchDSL)
	mux.HandleFunc("GET /v1/apps/{appId}/versions", h.listVersions)
	mux.HandleFunc("GET /v1/apps/{appId}/versions/{version}", h.getVersion)
	mux.HandleFunc("POST /v1/apps/{appId}/versions/{version}/restore", h.restoreVersion)

	mux.HandleFunc("GET /v1/apps/{appId}/entities", h.listEntities)
	mux.HandleFunc("GET /v1/apps/{appId}/entities/{entityId}", h.getEntity)

	mux.HandleFunc("GET /v1/apps/{appId}/entities/{entityId}/records", h.listRecords)
	mux.HandleFunc("POST /v1/apps/{appId}/entities/{entityId}/records", h.createRecord)
	mux.HandleFunc("DELETE /v1/apps/{appId}/entities/{entityId}/records", h.deleteAllRecords)
	mux.HandleFunc("GET /v1/apps/{appId}/entities/{entityId}/records/{recordId}", h.getRecord)
	mux.HandleFunc("PATCH /v1/apps/{appId}/entities/{entityId}/records/{recordId}", h.patchRecord)
	mux.HandleFunc("DELETE /v1/apps/{appId}/entities/{entityId}/records/{recordId}", h.deleteRecord)

	// share links (#49) / public catalog (#50): share.go
	mux.HandleFunc("POST /v1/apps/{appId}/share", h.createShare)
	mux.HandleFunc("GET /v1/shares/{token}", h.getShare)
	mux.HandleFunc("DELETE /v1/shares/{token}", h.revokeShare)
	mux.HandleFunc("POST /v1/shares/{token}/import", h.importShare)
	mux.HandleFunc("POST /v1/apps/{appId}/publish", h.publishApp)
	mux.HandleFunc("DELETE /v1/apps/{appId}/publish", h.unpublishApp)
	mux.HandleFunc("GET /v1/public-apps", h.listPublicApps)
	mux.HandleFunc("POST /v1/public-apps/{id}/install", h.installPublicApp)

	mux.HandleFunc("DELETE /v1/me", h.deleteMe) // account deletion (#119): account.go
	// release requests (#76): release.go
	mux.HandleFunc("POST /v1/apps/{appId}/release-requests", h.createReleaseRequest)
	mux.HandleFunc("GET /v1/apps/{appId}/release-requests", h.listAppReleaseRequests)
	mux.HandleFunc("GET /v1/release-requests", h.listMyReleaseRequests)
	return mux
}

// ---- errors -----------------------------------------------------------------

// apiError carries an HTTP status + openapi error code until it reaches fail().
type apiError struct {
	status  int
	code    string
	msg     string
	details map[string]any
}

func (e *apiError) Error() string { return e.code + ": " + e.msg }

func apiErr(status int, code, msg string) *apiError {
	return &apiError{status: status, code: code, msg: msg}
}

// With attaches a details key.
func (e *apiError) With(k string, v any) *apiError {
	if e.details == nil {
		e.details = map[string]any{}
	}
	e.details[k] = v
	return e
}

// fail renders err through httpx: *apiError as-is, anything else as 500.
func fail(w http.ResponseWriter, err error) {
	var ae *apiError
	if errors.As(err, &ae) {
		if ae.status == http.StatusBadRequest || ae.status == http.StatusUnprocessableEntity {
			// 理由(field と msg)を残す: クライアントの保存失敗をログだけで追えるように
			slog.Warn("appconfig: request rejected", slog.Int("status", ae.status), slog.String("code", ae.code),
				slog.String("msg", ae.msg), slog.Any("details", ae.details), slog.String("requestId", w.Header().Get(httpx.RequestIDHeader)))
		}
		httpx.ErrorWithDetails(w, ae.status, ae.code, ae.msg, ae.details)
		return
	}
	slog.Error("appconfig: internal error", slog.Any("error", err), slog.String("requestId", w.Header().Get(httpx.RequestIDHeader)))
	httpx.Error(w, http.StatusInternalServerError, httpx.CodeInternal, "internal server error")
}

// mapStoreErr converts store sentinel errors into API errors.
func mapStoreErr(err error, what string) error {
	var ce *store.ConflictError
	switch {
	case errors.Is(err, store.ErrNotFound):
		return apiErr(http.StatusNotFound, httpx.CodeNotFound, what+" not found")
	case errors.As(err, &ce):
		return apiErr(http.StatusConflict, httpx.CodeVersionConflict, fmt.Sprintf("current version is %d", ce.Current)).With("currentVersion", ce.Current)
	}
	return err
}

func invalidDSL(err error) error {
	var ve *dsl.ValidationError
	if errors.As(err, &ve) {
		return apiErr(http.StatusUnprocessableEntity, httpx.CodeInvalidDSL, fmt.Sprintf("%d problem(s)", len(ve.Problems))).With("problems", ve.Problems)
	}
	return apiErr(http.StatusUnprocessableEntity, httpx.CodeInvalidDSL, err.Error())
}

// ---- request helpers --------------------------------------------------------

func uidOf(r *http.Request) string { return auth.UID(r.Context()) }

func pathID(r *http.Request, name string) (string, error) {
	v := r.PathValue(name)
	if !idRe.MatchString(v) || (name == "appId" && !validAppID(v)) {
		return "", apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "invalid "+name)
	}
	return v, nil
}

func etag(v int) string { return fmt.Sprintf("%q", strconv.Itoa(v)) }

// parseETag accepts "7", W/"7" or 7 and returns 7.
func parseETag(s string) (int, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "W/")
	s = strings.Trim(s, `"`)
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func requireIfMatch(r *http.Request) (int, error) {
	raw := r.Header.Get("If-Match")
	if raw == "" {
		return 0, apiErr(http.StatusPreconditionFailed, httpx.CodeBadRequest, "If-Match header is required")
	}
	v, ok := parseETag(raw)
	if !ok {
		return 0, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "If-Match must be a version like \"7\"")
	}
	return v, nil
}

type paging struct{ size, offset int }

func parsePaging(r *http.Request) (paging, error) {
	p := paging{size: defaultPage}
	if s := r.URL.Query().Get("pageSize"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			return p, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "pageSize must be a positive integer")
		}
		p.size = min(n, maxPage)
	}
	if t := r.URL.Query().Get("pageToken"); t != "" {
		b, err := base64.RawURLEncoding.DecodeString(t)
		n, err2 := strconv.Atoi(string(b))
		if err != nil || err2 != nil || n < 0 {
			return p, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "invalid pageToken")
		}
		p.offset = n
	}
	return p, nil
}

// next returns the token for the following page when the store returned
// more than one page (callers fetch size+1).
func (p paging) next(got int) string {
	if got <= p.size {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(p.offset + p.size)))
}

// ensureApp loads the app, seeding version 1 for unknown ids: iOS uses a
// device-fixed UUID as appId and never calls POST /apps first. If another
// user has published that appId (apps_public/{appId}, see publish), the seed
// is their DSL instead of dsl.Initial — this is how a shared app is
// "installed" under the caller's own uid.
func (h *Handler) ensureApp(r *http.Request, appID string) (*store.App, error) {
	ctx, uid := r.Context(), uidOf(r)
	if app, err := h.store.GetApp(ctx, uid, appID); err == nil {
		return app, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	init := dsl.Initial()
	raw, err := json.Marshal(init)
	if err != nil {
		return nil, err
	}
	name := init.App.Name
	if pub, err := h.store.GetPublicApp(ctx, appID); err == nil {
		raw, name = pub.DSL, pub.Name
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	app, _, err := h.store.EnsureApp(ctx, uid, appID, raw, name)
	return app, err
}

// publish mirrors a committed DSL to apps_public/{appId} when it has shared
// entities, so other users opening the same appId get it from ensureApp.
// Failures are logged, not returned: the owner's private commit already
// succeeded, and an appId first published by someone else is left alone.
func (h *Handler) publish(r *http.Request, appID string, app *dsl.App, raw json.RawMessage, version int) {
	if !app.HasSharedEntity() {
		return
	}
	ok, err := h.store.PublishApp(r.Context(), uidOf(r), appID, raw, app.App.Name, version)
	switch {
	case err != nil:
		slog.Error("appconfig: publish failed", slog.String("appId", appID), slog.Any("error", err))
	case !ok:
		slog.Warn("appconfig: appId already published by another user; not overwritten", slog.String("appId", appID))
	}
}

// parsedDSL returns the typed DSL for an app version (cached).
func (h *Handler) parsedDSL(uid string, app *store.App) (*dsl.App, error) {
	key := uid + "/" + app.ID + "/" + strconv.Itoa(app.CurrentVersion)
	h.mu.Lock()
	if a, ok := h.cache[key]; ok {
		h.mu.Unlock()
		return a, nil
	}
	h.mu.Unlock()
	a, err := dsl.Parse(app.DSL)
	if err != nil {
		return nil, fmt.Errorf("stored dsl invalid: %w", err)
	}
	h.mu.Lock()
	if len(h.cache) > 256 {
		h.cache = map[string]*dsl.App{}
	}
	h.cache[key] = a
	h.mu.Unlock()
	return a, nil
}

// compact re-encodes raw as canonical compact JSON (numbers preserved).
func compact(raw json.RawMessage) json.RawMessage {
	var b strings.Builder
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return raw
	}
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return raw
	}
	return json.RawMessage(strings.TrimSuffix(b.String(), "\n"))
}
