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
// Fields are grouped per feature (blank lines keep gofmt alignment local).
type Handler struct {
	store store.Store

	mu    sync.Mutex
	cache map[string]*dsl.App // uid/appId/version -> parsed DSL (records validation)

	eventsMax time.Duration // lifetime of one GET /events stream

	legacyBefore time.Time // LegacyMembersEnv: set by members.go

	feedbackPerDay int // FEEDBACK_DAILY_LIMIT: set by feedback.go

	outbox   outboxConfig // RUNNER_OUTBOX / APP_ENV: set by outbox.go (#341)
	activity activity     // lastOpenedAt written today (activity.go, #342)

	runnerEnv string // runner/{env} the automations view reads (automations.go, #347)
}

// routeGroups holds the feature routes. Each feature file registers its own
// from init(), so adding a feature does not edit New().
var routeGroups []func(h *Handler, mux *http.ServeMux)

// registerRoutes adds a feature's routes (and its Handler settings).
func registerRoutes(f func(h *Handler, mux *http.ServeMux)) { routeGroups = append(routeGroups, f) }

// New builds the v1 router (a ServeMux so the caller can probe matches).
// Core routes live here; feature routes come from routeGroups.
func New(s store.Store) *http.ServeMux {
	h := &Handler{store: s, cache: map[string]*dsl.App{}, eventsMax: eventsMaxStream()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/apps", h.listApps)
	mux.HandleFunc("POST /v1/apps", h.createApp)
	mux.HandleFunc("GET /v1/apps/{appId}", h.getApp)
	mux.HandleFunc("PATCH /v1/apps/{appId}", h.patchApp)
	mux.HandleFunc("DELETE /v1/apps/{appId}", h.deleteApp)

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

	for _, register := range routeGroups {
		register(h, mux)
	}
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
// device-fixed UUID as appId and never calls POST /apps first. If the app's
// creator has published that appId (apps_public/{appId}, see publish), the
// seed is their DSL instead of dsl.Initial — this is how a shared app is
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
	pub, err := h.creatorMirror(ctx, appID)
	if err != nil {
		return nil, err
	}
	if pub != nil && !pub.Hidden { // #321: not from an app the operator unpublished
		raw, name = pub.DSL, pub.Name
	}
	app, _, err := h.store.EnsureApp(ctx, uid, appID, raw, name)
	return app, err
}

// commitPublish makes a commit to an existing app also update its mirror
// (apps_public/{appId}) in the same transaction, so the creator's version and
// the published one never drift apart: a commit that cannot publish fails as
// a whole. Only the creator's commits publish (decided in the store from
// app_owners); claim records the committer as the creator when none is known.
// Legacy owners (catalog / mirror) are resolved into app_owners first.
func (h *Handler) commitPublish(r *http.Request, appID string, app *dsl.App, claim bool) (*store.CommitPublish, error) {
	if _, err := h.appOwner(r.Context(), appID); err != nil {
		return nil, err
	}
	return &store.CommitPublish{Shared: app.HasSharedEntity(), Claim: claim}, nil
}

// publish mirrors the first DSL of a newly created appId (POST /apps,
// duplicate) to apps_public/{appId} when it has shared entities, so other
// users opening the same appId get it from ensureApp. Commits to an existing
// app publish through commitPublish instead. Only the app's creator
// (app_owners) publishes; other copies stay private. Failures are logged, not
// returned: the app was already created, and without a mirror nothing of it
// is shared yet (the next commit publishes). An appId first published by
// someone else is left alone.
func (h *Handler) publish(r *http.Request, appID string, app *dsl.App, raw json.RawMessage, version int) {
	if !app.HasSharedEntity() {
		return
	}
	owner, err := h.appOwner(r.Context(), appID)
	if err != nil {
		slog.Error("appconfig: publish owner lookup failed", slog.String("appId", appID), slog.Any("error", err))
		return
	}
	if owner != uidOf(r) {
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
