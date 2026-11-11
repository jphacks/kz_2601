package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appcheck"
	"github.com/jphacks/kz_2601/backend/internal/appconfig"
	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

func testHandler() http.Handler {
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	return newHandler(auth.Middleware(nil, auth.ModeDev), logger, appconfig.New(store.NewMemory()), nil)
}

func TestRoutes(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		debugUID   string
		wantStatus int
	}{
		{name: "health is public", path: "/health", wantStatus: http.StatusOK},
		{name: "healthz is public", path: "/healthz", wantStatus: http.StatusOK},
		{name: "apps requires auth", path: "/v1/apps", wantStatus: http.StatusUnauthorized},
		{name: "apps with debug uid", path: "/v1/apps", debugUID: "u1", wantStatus: http.StatusOK},
		{name: "unknown path is 404", path: "/v1/nope", wantStatus: http.StatusNotFound},
		{name: "client-config is public", path: "/v1/client-config", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.debugUID != "" {
				req.Header.Set(auth.DebugUIDHeader, tt.debugUID)
			}
			rec := httptest.NewRecorder()
			testHandler().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if rec.Header().Get(httpx.RequestIDHeader) == "" {
				t.Fatalf("missing %s header", httpx.RequestIDHeader)
			}
		})
	}
}

var openAPIErrorCodes = []string{
	"bad_request", "unauthenticated", "not_found", "version_conflict",
	"invalid_dsl", "invalid_record", "payload_too_large", "rate_limited", "app_check_failed", "internal",
}

// APP_CHECK_MODE=enforce guards only the authenticated API (#318): health, the
// share page and client-config (old builds learn they must update) stay open.
func TestAppCheckEnforceGuardsAPIOnly(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	ac := appcheck.Middleware(appcheck.Config{Mode: appcheck.ModeEnforce, Verifier: okVerifier{}, Logger: logger})
	authed := func(h http.Handler) http.Handler { return ac(auth.Middleware(nil, auth.ModeDev)(h)) }
	share := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := newHandler(authed, logger, appconfig.New(store.NewMemory()), share, appconfig.MountAccountDeletePage)

	tests := []struct {
		path, token string
		want        int
	}{
		{"/health", "", http.StatusOK},
		{"/s/abc", "", http.StatusOK},
		{"/account/delete", "", http.StatusOK},
		{"/v1/client-config", "", http.StatusOK},
		{"/v1/nope", "", http.StatusNotFound},
		{"/v1/apps", "", http.StatusUnauthorized},
		{"/v1/apps", "bad", http.StatusUnauthorized},
		{"/v1/apps", "ok", http.StatusOK},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		req.Header.Set(auth.DebugUIDHeader, "u1")
		if tt.token != "" {
			req.Header.Set(appcheck.Header, tt.token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tt.want {
			t.Fatalf("%s token=%q: status %d, want %d (%s)", tt.path, tt.token, rec.Code, tt.want, rec.Body.String())
		}
		if tt.want == http.StatusUnauthorized && !strings.Contains(rec.Body.String(), httpx.CodeAppCheckFailed) {
			t.Fatalf("%s: body %s", tt.path, rec.Body.String())
		}
	}
}

type okVerifier struct{}

func (okVerifier) Verify(_ context.Context, token string) (string, error) {
	if token != "ok" {
		return "", errors.New("invalid")
	}
	return "ios", nil
}

func TestUnmatchedRoutesReturnErrorEnvelope(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantCode   string
		wantAllow  bool
	}{
		{
			name:       "unknown path is a not_found envelope",
			method:     http.MethodGet,
			path:       "/v1/nope",
			wantStatus: http.StatusNotFound,
			wantCode:   httpx.CodeNotFound,
		},
		{
			name:       "unknown nested path is a not_found envelope",
			method:     http.MethodGet,
			path:       "/v1/apps/x/nope",
			wantStatus: http.StatusNotFound,
			wantCode:   httpx.CodeNotFound,
		},
		{
			name:       "wrong method is a bad_request envelope",
			method:     http.MethodPut,
			path:       "/v1/apps",
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   httpx.CodeBadRequest,
			wantAllow:  true,
		},
		{
			name:       "wrong method on health is a bad_request envelope",
			method:     http.MethodDelete,
			path:       "/health",
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   httpx.CodeBadRequest,
			wantAllow:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")

			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Header.Set(auth.DebugUIDHeader, "u1")
			rec := httptest.NewRecorder()
			testHandler().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("content-type = %q, want application/json", ct)
			}
			if tt.wantAllow && rec.Header().Get("Allow") == "" {
				t.Fatalf("missing Allow header on 405")
			}

			var got httpx.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("unmarshal %q: %v", rec.Body.String(), err)
			}
			if got.Error.Code != tt.wantCode {
				t.Fatalf("code = %q, want %q", got.Error.Code, tt.wantCode)
			}
			if !slices.Contains(openAPIErrorCodes, got.Error.Code) {
				t.Fatalf("code %q is not in the OpenAPI Error.code enum", got.Error.Code)
			}
			if got.Error.Message == "" {
				t.Fatalf("message is empty: %s", rec.Body.String())
			}
			if got.Error.RequestID == "" || got.Error.RequestID != rec.Header().Get(httpx.RequestIDHeader) {
				t.Fatalf("requestId = %q, header = %q", got.Error.RequestID, rec.Header().Get(httpx.RequestIDHeader))
			}
			if strings.Contains(rec.Body.String(), "page not found") {
				t.Fatalf("stdlib plain text leaked: %s", rec.Body.String())
			}
		})
	}
}

func TestListAppsReturnsEmptyItems(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")

	req := httptest.NewRequest(http.MethodGet, "/v1/apps", nil)
	req.Header.Set(auth.DebugUIDHeader, "u1")
	rec := httptest.NewRecorder()
	testHandler().ServeHTTP(rec, req)

	var got struct {
		Items []any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", rec.Body.String(), err)
	}
	if got.Items == nil || len(got.Items) != 0 {
		t.Fatalf("items = %v, want an empty array", got.Items)
	}
}

func TestEnvOr(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "unset falls back", value: "", want: "8080"},
		{name: "set wins", value: "9090", want: "9090"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PORT", tt.value)
			if got := envOr("PORT", "8080"); got != tt.want {
				t.Fatalf("envOr = %q, want %q", got, tt.want)
			}
		})
	}
}

// countingAccounts counts the account lookups RejectDeleted makes.
type countingAccounts struct {
	store.Accounts
	lookups int
}

func (a *countingAccounts) SeeUser(ctx context.Context, uid string) (bool, error) {
	a.lookups++
	return a.Accounts.SeeUser(ctx, uid)
}

// newAuthed keeps App Check -> auth -> RejectDeleted (#516): rejected requests
// never reach the account lookup, and a deleted uid gets 410.
func TestNewAuthedOrder(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	mem := store.NewMemory()
	if err := mem.DeactivateUser(context.Background(), "gone"); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, token, uid string
		want             int
		wantCode         string
		wantLookups      int
	}{
		{name: "app check first", token: "bad", uid: "u1", want: http.StatusUnauthorized, wantCode: httpx.CodeAppCheckFailed},
		{name: "auth before RejectDeleted", token: "ok", want: http.StatusUnauthorized, wantCode: httpx.CodeUnauthenticated},
		{name: "deleted uid is 410", token: "ok", uid: "gone", want: http.StatusGone, wantCode: "account_deleted", wantLookups: 1},
		{name: "active uid passes", token: "ok", uid: "u1", want: http.StatusOK, wantLookups: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accounts := &countingAccounts{Accounts: mem}
			ac := appcheck.Middleware(appcheck.Config{Mode: appcheck.ModeEnforce, Verifier: okVerifier{}, Logger: logger})
			app := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
			h := newAuthed(ac, auth.Middleware(nil, auth.ModeDev), accounts)(app)

			req := httptest.NewRequest(http.MethodGet, "/v1/apps", nil)
			req.Header.Set(appcheck.Header, tt.token)
			if tt.uid != "" {
				req.Header.Set(auth.DebugUIDHeader, tt.uid)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.want, rec.Body.String())
			}
			if tt.wantCode != "" && !strings.Contains(rec.Body.String(), tt.wantCode) {
				t.Fatalf("body %s, want code %s", rec.Body.String(), tt.wantCode)
			}
			if accounts.lookups != tt.wantLookups {
				t.Fatalf("account lookups = %d, want %d", accounts.lookups, tt.wantLookups)
			}
		})
	}
}

// GET /health names the environment and build through the full chain (#290).
func TestHealthReportsEnv(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	t.Setenv("GIT_SHA", "0123456789abcdef")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "p")
	rec := httptest.NewRecorder()
	testHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if body["status"] != "ok" || body["env"] != "prod" || body["firestoreDatabase"] != "(default)" || body["version"] != "0123456" {
		t.Fatalf("%v", body)
	}
}

// #362 / #497: a share link opened in a browser through the whole chain
// (envelope included) gets the HTML invalid-link page in the viewer's
// language for every way it can be dead; the API keeps its JSON 404.
func TestSharePageNotFoundIsHTML(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	ctx := t.Context()
	st := store.NewMemory()
	for _, tok := range []string{"tok_ok", "tok_rev", "tok_hid"} {
		if err := st.CreateShare(ctx, store.Share{Token: tok, Name: "n", AppID: tok, OwnerUID: "o"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.RevokeShare(ctx, "tok_rev"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PublishApp(ctx, "o", "tok_hid", []byte(`{}`), "n", 1); err != nil {
		t.Fatal(err)
	}
	if err := st.SetPublicAppHidden(ctx, "tok_hid", true, "r"); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	h := newHandler(auth.Middleware(nil, auth.ModeDev), logger, appconfig.New(st), appconfig.SharePage(st))

	get := func(path, lang string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if lang != "" {
			req.Header.Set("Accept-Language", lang)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := get("/s/tok_ok", ""); rec.Code != http.StatusOK {
		t.Fatalf("live share: %d", rec.Code)
	}
	for _, path := range []string{"/s/missing", "/s/tok_rev", "/s/tok_hid", "/s/bad%20tok", "/s/", "/s/tok_ok/extra"} {
		for lang, want := range map[string]string{"ja": "リンクが無効です", "en-US": "This link is not valid"} {
			rec := get(path, lang)
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s %s: status %d", path, lang, rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
				t.Errorf("%s %s: content-type %q (body %.80s)", path, lang, ct, rec.Body.String())
			}
			if csp := rec.Header().Get("Content-Security-Policy"); !strings.HasPrefix(csp, "default-src 'none'; style-src 'nonce-") {
				t.Errorf("%s %s: CSP %q", path, lang, csp)
			}
			if body := rec.Body.String(); !strings.Contains(body, want) || strings.Contains(body, `"code"`) {
				t.Errorf("%s %s: body lacks %q or is an envelope", path, lang, want)
			}
		}
	}
	// the API's share lookup is still a JSON envelope
	req := httptest.NewRequest(http.MethodGet, "/v1/shares/tok_rev", nil)
	req.Header.Set(auth.DebugUIDHeader, "u1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var env struct {
		Error struct{ Code string } `json:"error"`
	}
	if rec.Code != http.StatusNotFound || json.Unmarshal(rec.Body.Bytes(), &env) != nil || env.Error.Code != httpx.CodeNotFound {
		t.Errorf("api: %d %s", rec.Code, rec.Body.String())
	}
}
