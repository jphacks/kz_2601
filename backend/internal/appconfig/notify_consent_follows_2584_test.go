package appconfig

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// consent2584Store fails the calls of putNotifyConsent / listFollows when set.
// brokenDSL makes GetApp return the caller's copy with a DSL that does not parse.
type consent2584Store struct {
	store.Store
	appOwner, setConsent, listFollows error
	brokenDSL                         bool
}

func (s *consent2584Store) GetApp(ctx context.Context, uid, appID string) (*store.App, error) {
	a, err := s.Store.GetApp(ctx, uid, appID)
	if err != nil || !s.brokenDSL {
		return a, err
	}
	c := *a
	c.DSL, c.CurrentVersion = json.RawMessage(`{"broken":true}`), 2584
	return &c, nil
}

func (s *consent2584Store) AppOwner(ctx context.Context, appID string) (string, error) {
	if s.appOwner != nil {
		return "", s.appOwner
	}
	return s.Store.AppOwner(ctx, appID)
}

func (s *consent2584Store) SetNotifyConsent(ctx context.Context, uid, appID string, allowed bool) (*store.NotifyConsent, error) {
	if s.setConsent != nil {
		return nil, s.setConsent
	}
	return s.Store.SetNotifyConsent(ctx, uid, appID, allowed)
}

func (s *consent2584Store) ListFollows(ctx context.Context, appID, uid string) ([]string, error) {
	if s.listFollows != nil {
		return nil, s.listFollows
	}
	return s.Store.ListFollows(ctx, appID, uid)
}

func newConsent2584Env(t *testing.T) (*env, *consent2584Store) {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	st := &consent2584Store{Store: store.NewMemory()}
	return &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st))}, st
}

func want2584(t *testing.T, what string, r resp, code int, errCode string) {
	t.Helper()
	if r.code != code || r.errCode() != errCode {
		t.Fatalf("%s: want %d %s, got %d %s", what, code, errCode, r.code, r.raw)
	}
}

// #2584: bad requests to putNotifyConsent, and turning it on for an app the
// caller does not have.
func TestNotifyConsent2584BadRequests(t *testing.T) {
	e, st := newConsent2584Env(t)
	const consent = "/v1/me/notify-consent/NC2584-BAD"
	want2584(t, "invalid appId", e.do("u", http.MethodPut, "/v1/me/notify-consent/undefined", `{"allowed":true}`, nil), http.StatusBadRequest, httpx.CodeBadRequest)
	want2584(t, "not json", e.do("u", http.MethodPut, consent, `{`, nil), http.StatusBadRequest, httpx.CodeBadRequest)
	want2584(t, "unknown field", e.do("u", http.MethodPut, consent, `{"allowed":true,"all":true}`, nil), http.StatusBadRequest, httpx.CodeBadRequest)
	want2584(t, "allowed not bool", e.do("u", http.MethodPut, consent, `{"allowed":"yes"}`, nil), http.StatusBadRequest, httpx.CodeBadRequest)
	r := e.do("u", http.MethodPut, consent, `{}`, nil)
	if d, _ := r.body["error"].(map[string]any)["details"].(map[string]any); r.code != http.StatusBadRequest || d["field"] != "allowed" {
		t.Fatalf("allowed missing: %d %s", r.code, r.raw)
	}
	want2584(t, "missing app", e.do("u", http.MethodPut, consent, `{"allowed":true}`, nil), http.StatusNotFound, httpx.CodeNotFound)
	if ok, _ := st.NotifyAllowed(t.Context(), "u", "NC2584-BAD"); ok {
		t.Fatal("refused consent was stored")
	}
	// off works without the app
	if r = e.do("u", http.MethodPut, consent, `{"allowed":false}`, nil); r.code != http.StatusOK || r.body["allowed"] != false {
		t.Fatalf("off: %d %s", r.code, r.raw)
	}
}

// #2584: a private app needs no participation; store failures are 500.
func TestNotifyConsent2584StoreErrors(t *testing.T) {
	boom := errors.New("injected store failure")
	e, st := newConsent2584Env(t)
	const id = "NC2584-FAM"
	e.ownApp("owner", "/v1/apps/"+id, familyKakeibo(t))
	e.ownApp("owner", "/v1/apps/NC2584-SOLO", walk(t))
	consent := "/v1/me/notify-consent/" + id

	if r := e.do("owner", http.MethodPut, "/v1/me/notify-consent/NC2584-SOLO", `{"allowed":true}`, nil); r.code != http.StatusOK || r.body["allowed"] != true {
		t.Fatalf("private app: %d %s", r.code, r.raw)
	}

	st.appOwner = boom
	want2584(t, "memberRole", e.do("owner", http.MethodPut, consent, `{"allowed":true}`, nil), http.StatusInternalServerError, httpx.CodeInternal)
	st.appOwner = nil

	st.brokenDSL = true
	want2584(t, "stored dsl invalid", e.do("owner", http.MethodPut, consent, `{"allowed":true}`, nil), http.StatusInternalServerError, httpx.CodeInternal)
	st.brokenDSL = false

	st.setConsent = boom
	want2584(t, "SetNotifyConsent on", e.do("owner", http.MethodPut, consent, `{"allowed":true}`, nil), http.StatusInternalServerError, httpx.CodeInternal)
	want2584(t, "SetNotifyConsent off", e.do("owner", http.MethodPut, consent, `{"allowed":false}`, nil), http.StatusInternalServerError, httpx.CodeInternal)
	st.setConsent = nil

	if ok, _ := st.NotifyAllowed(t.Context(), "owner", id); ok {
		t.Fatal("a failed consent was stored")
	}
}

// #2584: listFollows refuses an invalid appId, an app without shared data,
// someone who did not join and an unpublished app; no follows is [].
func TestListFollows2584Refused(t *testing.T) {
	e, st := newConsent2584Env(t)
	const id = "FO2584-SH"
	app := "/v1/apps/" + id
	e.ownApp("owner", app, []byte(strings.Replace(string(timelineDSL(t)), `"scope": "public"`, `"scope": "shared"`, 1)))
	e.ownApp("owner", "/v1/apps/FO2584-PRIV", walk(t))

	want2584(t, "invalid appId", e.do("owner", http.MethodGet, "/v1/apps/undefined/follows", nil, nil), http.StatusBadRequest, httpx.CodeBadRequest)
	if r := e.do("owner", http.MethodGet, "/v1/apps/FO2584-PRIV/follows", nil, nil); r.code != http.StatusNotFound || r.reason() != reasonNoSharedData {
		t.Fatalf("private app: %d %s", r.code, r.raw)
	}
	if r := e.do("owner", http.MethodGet, "/v1/apps/FO2584-NONE/follows", nil, nil); r.code != http.StatusNotFound || r.reason() != reasonNoSharedData {
		t.Fatalf("missing app: %d %s", r.code, r.raw)
	}
	wantNotMember(t, e.do("stranger", http.MethodGet, app+"/follows", nil, nil), "stranger")

	r := e.do("owner", http.MethodGet, app+"/follows", nil, nil)
	if uids, ok := r.body["uids"].([]any); r.code != http.StatusOK || !ok || len(uids) != 0 {
		t.Fatalf("no follows: %d %s", r.code, r.raw)
	}

	if err := st.SetPublicAppHidden(t.Context(), id, true, "違法な内容のため"); err != nil {
		t.Fatal(err)
	}
	want2584(t, "unpublished", e.do("owner", http.MethodGet, app+"/follows", nil, nil), http.StatusForbidden, httpx.CodeUnpublished)
}

// #2584: store failures in listFollows are 500.
func TestListFollows2584StoreErrors(t *testing.T) {
	boom := errors.New("injected store failure")
	e, st := newConsent2584Env(t)
	app := "/v1/apps/FO2584-ERR"
	e.ownApp("owner", app, []byte(strings.Replace(string(timelineDSL(t)), `"scope": "public"`, `"scope": "shared"`, 1)))

	st.appOwner = boom
	want2584(t, "memberRole", e.do("owner", http.MethodGet, app+"/follows", nil, nil), http.StatusInternalServerError, httpx.CodeInternal)
	st.appOwner = nil

	st.listFollows = boom
	want2584(t, "ListFollows", e.do("owner", http.MethodGet, app+"/follows", nil, nil), http.StatusInternalServerError, httpx.CodeInternal)
}
