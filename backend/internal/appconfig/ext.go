package appconfig

// External APIs (#1493): the device's door to the runner's
// /internal/ext/*. appconfig checks the person (ID token, App Check), the
// app (whose `runs` pay: the creator of a shared app, else the caller), the
// provider, op and argument names and values, then calls the runner with
// its service identity. The runner holds the keys and the tokens, counts
// the call and answers a reason that becomes the error code here.
// docs/backend/features/external-apis.md

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/extapi"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// DefaultExtPerHour is the per-user cap of external calls (EXT_USER_PER_HOUR).
const DefaultExtPerHour = 120

// extWait bounds one call through the runner (its own timeout is EXT_TIMEOUT, 8 s).
const extWait = 15 * time.Second

// ExtRunner is the runner's /internal/ext/* (*extapi.Client).
type ExtRunner interface {
	Call(ctx context.Context, b extapi.CallBody) (extapi.CallAnswer, error)
	Start(ctx context.Context, uid, provider string, scopes []string) (string, error)
	Revoke(ctx context.Context, uid, provider string) error
	RevokeAll(ctx context.Context, uid string) (int, error)
	List(ctx context.Context, uid string) (extapi.ConnectionList, error)
}

func init() {
	registerRoutes(func(h *Handler, mux *http.ServeMux) {
		if h.extUsers == nil {
			h.extUsers = newFeedLimiter(extPerHour(os.Getenv), time.Now)
		}
		mux.HandleFunc("POST /v1/apps/{appId}/ext/{provider}/{op}", h.extCall)
		mux.HandleFunc("GET /v1/me/connections", h.listConnections)
		mux.HandleFunc("POST /v1/me/connections/{provider}/start", h.startConnection)
		mux.HandleFunc("DELETE /v1/me/connections/{provider}", h.revokeConnection)
	})
}

// WithExt sets the runner's external API door (nil = 503 ext_unavailable).
func WithExt(r ExtRunner) Option { return func(h *Handler) { h.ext = r } }

// WithExtLimit replaces the per-user cap (tests).
func WithExtLimit(perHour int, now func() time.Time) Option {
	return func(h *Handler) { h.extUsers = newFeedLimiter(perHour, now) }
}

func extPerHour(getenv func(string) string) int {
	if n, err := strconv.Atoi(getenv("EXT_USER_PER_HOUR")); err == nil && n >= 0 {
		return n
	}
	return DefaultExtPerHour
}

func extOff() *apiError {
	return apiErr(http.StatusServiceUnavailable, httpx.CodeExtUnavailable, "このサービスは今は使えません。")
}

// extProvider is the path's provider (404 when it is not on the list).
func extProvider(r *http.Request, oauthOnly bool) (extapi.Provider, error) {
	p, ok := extapi.Lookup(r.PathValue("provider"))
	if !ok || (oauthOnly && !p.NeedsOAuth) {
		return p, apiErr(http.StatusNotFound, httpx.CodeNotFound, "unknown provider")
	}
	return p, nil
}

// extCall: POST /v1/apps/{appId}/ext/{provider}/{op} {"args": {...}}
func (h *Handler) extCall(w http.ResponseWriter, r *http.Request) {
	appID, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	p, err := extProvider(r, false)
	if err != nil {
		fail(w, err)
		return
	}
	op := r.PathValue("op")
	if _, ok := p.Op(op); !ok {
		fail(w, apiErr(http.StatusNotFound, httpx.CodeNotFound, "unknown op"))
		return
	}
	var in struct {
		Args map[string]string `json:"args"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	if !httpx.DecodeBody(w, r, &in) {
		return
	}
	if _, err := p.CheckArgs(op, in.Args); err != nil {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, err.Error()))
		return
	}
	if h.ext == nil {
		fail(w, extOff())
		return
	}
	creator, creatorAnon, err := h.extCreator(r, appID)
	if err != nil {
		fail(w, err)
		return
	}
	uid := uidOf(r)
	if wait, ok := h.extUsers.take(uid); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(wait))
		httpx.ErrorWithDetails(w, http.StatusTooManyRequests, httpx.CodeRateLimited, "しばらくしてからもう一度試してください。",
			map[string]any{"scope": "user", "limit": h.extUsers.perHour, "remaining": 0, "retryAfterSec": wait})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), extWait)
	defer cancel()
	ans, err := h.ext.Call(ctx, extapi.CallBody{Provider: p.ID, Op: op, Args: in.Args, AppID: appID,
		CreatorUID: creator, CreatorAnonymous: creatorAnon, UserUID: uid, UserAnonymous: auth.Anonymous(r.Context()),
		RequestID: extRequestID(r)})
	if err != nil {
		e := extErr(err, p.ID)
		if e.code == httpx.CodeProRequired && creator != uid {
			e = e.With("reason", reasonCreatorNeedsPro)
		}
		fail(w, e)
		return
	}
	if p.Billing == extapi.BillingPaid {
		w.Header().Set(coins.HeaderCharged, strconv.FormatInt(ans.CoinsCharged, 10))
		w.Header().Set(coins.HeaderBalance, strconv.FormatInt(ans.CoinsBalance, 10))
	}
	httpx.WriteJSON(w, http.StatusOK, ans.Result)
}

// extCreator returns whose `runs` the call is: the creator of a shared app
// for its members (and anyone, for an app with public data), else the
// caller for their own app or their own copy of a private one.
func (h *Handler) extCreator(r *http.Request, appID string) (string, bool, error) {
	ctx, uid := r.Context(), uidOf(r)
	owner, err := h.appOwner(ctx, appID)
	if err != nil {
		return "", false, err
	}
	if owner != "" && owner != uid {
		a, pub, err := h.publishedDSL(ctx, appID)
		if err != nil {
			return "", false, err
		}
		if a != nil && hasSharedEntity(a) {
			if pub.Hidden {
				return "", false, unpublishedErr("this shared app", pub.ModerationState)
			}
			role, err := h.memberRole(ctx, appID, uid)
			if err != nil {
				return "", false, err
			}
			if role == "" && !hasPublicScope(a) {
				return "", false, notMember()
			}
			return owner, false, nil
		}
	}
	if _, err := h.store.GetApp(ctx, uid, appID); err != nil {
		return "", false, mapStoreErr(err, "app")
	}
	return uid, auth.Anonymous(ctx), nil
}

// extRequestID is the call's idempotency key: the device's X-Request-Id
// (kept by httpx.WithRequestID), else a fresh one.
func extRequestID(r *http.Request) string {
	if id := httpx.RequestID(r.Context()); id != "" {
		return id
	}
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "ext-" + hex.EncodeToString(b)
}

// extErr turns the runner's answer into the error code (the status is the
// contract's, not the runner's).
func extErr(err error, provider string) *apiError {
	var re *extapi.RunnerError
	if !errors.As(err, &re) {
		if errors.Is(err, context.DeadlineExceeded) {
			return apiErr(http.StatusGatewayTimeout, httpx.CodeUpstreamTimeout, "時間内に答えが返りませんでした。")
		}
		return apiErr(http.StatusBadGateway, httpx.CodeUpstreamUnavailable, "今は使えません。しばらくしてからもう一度試してください。")
	}
	switch re.Reason {
	case extapi.ReasonBadRequest:
		return apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "引数が正しくありません。")
	case extapi.ReasonNotConnected:
		return apiErr(http.StatusConflict, httpx.CodeExtNotConnected, "このサービスにつないでいません。").With("provider", provider)
	case extapi.ReasonUnavailable:
		return extOff().With("provider", provider)
	case extapi.ReasonBusy:
		return apiErr(http.StatusServiceUnavailable, httpx.CodeExtBusy, "混み合っています。しばらくしてからもう一度試してください。").With("provider", provider)
	case extapi.ReasonDuplicateRequest:
		return apiErr(http.StatusConflict, httpx.CodeDuplicateRequest, "this request id was already used")
	case extapi.ReasonProRequired:
		return apiErr(http.StatusPaymentRequired, httpx.CodeProRequired, "this needs Tailor It Pro: externalCall")
	case extapi.ReasonAllowanceExhausted:
		return apiErr(http.StatusPaymentRequired, httpx.CodeAllowanceExhausted, "the monthly server allowance is used up")
	case extapi.ReasonOverflowCap:
		return apiErr(http.StatusPaymentRequired, httpx.CodeOverflowCapReached, "the monthly coin cap for overflow is reached")
	case extapi.ReasonCoinsInsufficient:
		return apiErr(http.StatusPaymentRequired, httpx.CodeCoinsInsufficient, "not enough coins").With("payer", "self")
	case extapi.ReasonUpstreamTimeout:
		return apiErr(http.StatusGatewayTimeout, httpx.CodeUpstreamTimeout, "時間内に答えが返りませんでした。").With("provider", provider)
	case extapi.ReasonUpstream:
		return apiErr(http.StatusBadGateway, httpx.CodeUpstreamUnavailable, "サービスが答えませんでした。").With("provider", provider)
	}
	return apiErr(http.StatusInternalServerError, httpx.CodeInternal, "internal server error")
}

// listConnections: GET /v1/me/connections[?supportedFeatures=a,b]
//
//	{"connections": [{provider, status, scopes, dates}] (no tokens),
//	 "providers": [{id, nameJa, nameEn, enabled}]}
//
// providers are the services a person can connect (OAuth providers):
// enabled = the runner has it on now. A fake is listed only when the runner
// has it on (never in production). With supportedFeatures (the build's
// list, as for generate), only providers whose externalCall.<id> is in it
// are listed: the review build sends none of them (決定 6).
func (h *Handler) listConnections(w http.ResponseWriter, r *http.Request) {
	ans := extapi.ConnectionList{Connections: []extapi.ConnectionView{}}
	if h.ext != nil {
		var err error
		if ans, err = h.ext.List(r.Context(), uidOf(r)); err != nil {
			fail(w, extErr(err, ""))
			return
		}
		if ans.Connections == nil {
			ans.Connections = []extapi.ConnectionView{}
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"connections": ans.Connections,
		"providers":   connectable(ans.Enabled, r.URL.Query()["supportedFeatures"]),
	})
}

// connectableProvider is one entry of GET /v1/me/connections providers.
type connectableProvider struct {
	ID      string `json:"id"`
	NameJa  string `json:"nameJa"`
	NameEn  string `json:"nameEn"`
	Enabled bool   `json:"enabled"`
}

// connectable lists the OAuth providers (sorted by id). features is the
// supportedFeatures query (repeated or comma-separated); nil = no filter.
func connectable(enabled, features []string) []connectableProvider {
	var allow map[string]bool
	if features != nil {
		allow = map[string]bool{}
		for _, f := range features {
			for _, k := range strings.Split(f, ",") {
				allow[strings.TrimSpace(k)] = true
			}
		}
	}
	out := []connectableProvider{}
	for _, p := range extapi.Providers() {
		on := slices.Contains(enabled, p.ID)
		if !p.NeedsOAuth || (p.Fake && !on) || (allow != nil && !allow[p.FeatureKey()]) {
			continue
		}
		out = append(out, connectableProvider{ID: p.ID, NameJa: p.NameJa, NameEn: p.NameEn, Enabled: on})
	}
	return out
}

// startConnection: POST /v1/me/connections/{provider}/start {"scopes": [...]} → {"authUrl"}
func (h *Handler) startConnection(w http.ResponseWriter, r *http.Request) {
	p, err := extProvider(r, true)
	if err != nil {
		fail(w, err)
		return
	}
	var in struct {
		Scopes []string `json:"scopes"`
	}
	if r.ContentLength != 0 {
		r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
		if !httpx.DecodeBody(w, r, &in) {
			return
		}
	}
	if len(in.Scopes) > 32 {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "too many scopes").With("field", "scopes"))
		return
	}
	if h.ext == nil {
		fail(w, extOff())
		return
	}
	u, err := h.ext.Start(r.Context(), uidOf(r), p.ID, in.Scopes)
	if err != nil {
		fail(w, extErr(err, p.ID))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"authUrl": u})
}

// revokeConnection: DELETE /v1/me/connections/{provider}
func (h *Handler) revokeConnection(w http.ResponseWriter, r *http.Request) {
	p, err := extProvider(r, true)
	if err != nil {
		fail(w, err)
		return
	}
	if h.ext == nil {
		fail(w, extOff())
		return
	}
	if err := h.ext.Revoke(r.Context(), uidOf(r), p.ID); err != nil {
		fail(w, extErr(err, p.ID))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
