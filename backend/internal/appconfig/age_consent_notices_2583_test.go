package appconfig

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// age2583Store fails the age / consent reads and writes when set.
type age2583Store struct {
	store.Store
	ageConfirmed, notifyAllowed, setConsent error
}

func (s *age2583Store) AgeConfirmed(ctx context.Context, uid string) (bool, error) {
	if s.ageConfirmed != nil {
		return false, s.ageConfirmed
	}
	return s.Store.AgeConfirmed(ctx, uid)
}

func (s *age2583Store) NotifyAllowed(ctx context.Context, uid, appID string) (bool, error) {
	if s.notifyAllowed != nil {
		return false, s.notifyAllowed
	}
	return s.Store.NotifyAllowed(ctx, uid, appID)
}

func (s *age2583Store) SetNotifyConsent(ctx context.Context, uid, appID string, allowed bool) (*store.NotifyConsent, error) {
	if s.setConsent != nil {
		return nil, s.setConsent
	}
	return s.Store.SetNotifyConsent(ctx, uid, appID, allowed)
}

func newAge2583Env(t *testing.T) (*env, *age2583Store) {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	st := &age2583Store{Store: store.NewMemory()}
	return &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st))}, st
}

func want2583(t *testing.T, what string, r resp, code int, errCode string) {
	t.Helper()
	if r.code != code || r.errCode() != errCode {
		t.Fatalf("%s: want %d %s, got %d %s", what, code, errCode, r.code, r.raw)
	}
}

const feedbackFixed2583 = "/v1/me/notify-consent/feedback-fixed"

// #2583: the 18+ declaration is false until made, then true; per uid.
func TestAge2583UnsetThenSet(t *testing.T) {
	e, _ := newAge2583Env(t)
	if r := e.do("a2583", http.MethodGet, "/v1/me/age-confirmation", nil, nil); r.code != http.StatusOK || r.body["ageConfirmed"] != false {
		t.Fatalf("unset: %d %s", r.code, r.raw)
	}
	want2583(t, "bad body", e.do("a2583", http.MethodPut, "/v1/me/age-confirmation", `{"age":20}`, nil), http.StatusBadRequest, httpx.CodeBadRequest)
	if r := e.do("a2583", http.MethodPut, "/v1/me/age-confirmation", `{"ageConfirmed":true}`, nil); r.code != http.StatusOK || r.body["ageConfirmed"] != true {
		t.Fatalf("declare: %d %s", r.code, r.raw)
	}
	if r := e.do("a2583", http.MethodGet, "/v1/me/age-confirmation", nil, nil); r.code != http.StatusOK || r.body["ageConfirmed"] != true {
		t.Fatalf("set: %d %s", r.code, r.raw)
	}
	// declared once, kept even when a later PUT says false
	if r := e.do("a2583", http.MethodPut, "/v1/me/age-confirmation", `{"ageConfirmed":false}`, nil); r.code != http.StatusOK || r.body["ageConfirmed"] != true {
		t.Fatalf("kept: %d %s", r.code, r.raw)
	}
	if r := e.do("b2583", http.MethodGet, "/v1/me/age-confirmation", nil, nil); r.code != http.StatusOK || r.body["ageConfirmed"] != false {
		t.Fatalf("another uid: %d %s", r.code, r.raw)
	}
}

// #2583: the consent to 「直りました」 is off until set, then what was set.
func TestFeedbackFixed2583UnsetThenSet(t *testing.T) {
	e, _ := newAge2583Env(t)
	r := e.do("c2583", http.MethodGet, feedbackFixed2583, nil, nil)
	if r.code != http.StatusOK || r.body["enabled"] != false || r.body["updatedAt"] != nil {
		t.Fatalf("unset: %d %s", r.code, r.raw)
	}
	for _, on := range []bool{true, false} {
		body := `{"enabled":false}`
		if on {
			body = `{"enabled":true}`
		}
		if r = e.do("c2583", http.MethodPut, feedbackFixed2583, body, nil); r.code != http.StatusOK || r.body["enabled"] != on || r.body["updatedAt"] == nil {
			t.Fatalf("put %v: %d %s", on, r.code, r.raw)
		}
		if r = e.do("c2583", http.MethodGet, feedbackFixed2583, nil, nil); r.code != http.StatusOK || r.body["enabled"] != on {
			t.Fatalf("get after %v: %d %s", on, r.code, r.raw)
		}
	}
	want2583(t, "enabled missing", e.do("c2583", http.MethodPut, feedbackFixed2583, `{}`, nil), http.StatusBadRequest, httpx.CodeBadRequest)
}

// #2583: without a signed-in user every endpoint is 401.
func TestAgeConsent2583Anonymous(t *testing.T) {
	e, _ := newAge2583Env(t)
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/v1/me/age-confirmation"},
		{http.MethodPut, "/v1/me/age-confirmation"},
		{http.MethodGet, feedbackFixed2583},
		{http.MethodPut, feedbackFixed2583},
	} {
		var body any
		if c.method == http.MethodPut {
			body = `{"ageConfirmed":true,"enabled":true}`
		}
		want2583(t, c.method+" "+c.path, e.do("", c.method, c.path, body, nil), http.StatusUnauthorized, "unauthenticated")
	}
}

// #2583: store failures are 500.
func TestAgeConsent2583StoreErrors(t *testing.T) {
	boom := errors.New("injected store failure")
	e, st := newAge2583Env(t)

	st.ageConfirmed = boom
	want2583(t, "get age", e.do("d2583", http.MethodGet, "/v1/me/age-confirmation", nil, nil), http.StatusInternalServerError, httpx.CodeInternal)
	want2583(t, "put age", e.do("d2583", http.MethodPut, "/v1/me/age-confirmation", `{"ageConfirmed":true}`, nil), http.StatusInternalServerError, httpx.CodeInternal)
	st.ageConfirmed = nil

	st.notifyAllowed = boom
	want2583(t, "get consent", e.do("d2583", http.MethodGet, feedbackFixed2583, nil, nil), http.StatusInternalServerError, httpx.CodeInternal)
	st.notifyAllowed = nil

	st.setConsent = boom
	want2583(t, "put consent", e.do("d2583", http.MethodPut, feedbackFixed2583, `{"enabled":true}`, nil), http.StatusInternalServerError, httpx.CodeInternal)
	st.setConsent = nil

	// nothing was recorded
	if r := e.do("d2583", http.MethodGet, "/v1/me/age-confirmation", nil, nil); r.body["ageConfirmed"] != false {
		t.Fatalf("age after failures: %s", r.raw)
	}
	if r := e.do("d2583", http.MethodGet, feedbackFixed2583, nil, nil); r.body["enabled"] != false {
		t.Fatalf("consent after failures: %s", r.raw)
	}
}

// #2583: notices are read next to Firestore when the store is Firestore, and
// from an empty in-memory reader otherwise (a wrapped store included).
// The client is never dialed, so this needs no emulator.
func TestDefaultNotices2583(t *testing.T) {
	if _, ok := defaultNotices(store.NewFirestoreFromClient(nil)).(*loop.Firestore); !ok {
		t.Fatal("Firestore store: want loop.Firestore")
	}
	for name, st := range map[string]store.Store{
		"memory":  store.NewMemory(),
		"wrapped": &age2583Store{Store: store.NewMemory()},
	} {
		n, ok := defaultNotices(st).(*loop.Memory)
		if !ok {
			t.Fatalf("%s: want loop.Memory", name)
		}
		if got, err := n.ListNotices(t.Context(), "u2583", 10); err != nil || len(got) != 0 {
			t.Fatalf("%s: empty reader: %v %v", name, got, err)
		}
	}
}
