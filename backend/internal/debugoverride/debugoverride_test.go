package debugoverride

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/entitlement"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestFromEnv(t *testing.T) {
	for _, tc := range []struct {
		name    string
		env     map[string]string
		on      bool
		wantErr bool
	}{
		{"unset", map[string]string{"APP_ENV": "prod"}, false, false},
		{"unset on stg", map[string]string{"APP_ENV": "stg", UIDsEnv: "u1"}, false, false},
		{"prod refuses", map[string]string{"APP_ENV": "prod", EnabledEnv: "1", UIDsEnv: "u1"}, false, true},
		{"no APP_ENV refuses", map[string]string{EnabledEnv: "1", UIDsEnv: "u1"}, false, true},
		{"staging alias refuses", map[string]string{"APP_ENV": "staging", EnabledEnv: "1", UIDsEnv: "u1"}, false, true},
		{"other value on prod refuses", map[string]string{"APP_ENV": "prod", EnabledEnv: "0"}, false, true},
		{"value must be 1", map[string]string{"APP_ENV": "stg", EnabledEnv: "true", UIDsEnv: "u1"}, false, true},
		{"needs uids", map[string]string{"APP_ENV": "stg", EnabledEnv: "1", UIDsEnv: " , "}, false, true},
		{"stg on", map[string]string{"APP_ENV": "stg", EnabledEnv: "1", UIDsEnv: "u1, u2"}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := FromEnv(envOf(tc.env))
			if (err != nil) != tc.wantErr || (c != nil) != tc.on {
				t.Fatalf("FromEnv = %v, %v; want on=%v err=%v", c, err, tc.on, tc.wantErr)
			}
		})
	}
	c, _ := FromEnv(envOf(map[string]string{"APP_ENV": "stg", EnabledEnv: "1", UIDsEnv: "u1, u2"}))
	if c.Len() != 2 || !c.allows("u2") || c.allows("u3") || c.allows("") {
		t.Fatalf("uids parsed wrong: %+v", c)
	}
}

// writeFails is an entitlement.Store whose writes fail the test; Get counts
// the reads and answers Pro for the uids in pro.
type writeFails struct {
	t     *testing.T
	pro   map[string]bool
	reads atomic.Int32
}

func (s *writeFails) Get(_ context.Context, uid string) (*entitlement.Pro, error) {
	s.reads.Add(1)
	if !s.pro[uid] {
		return nil, nil
	}
	return &entitlement.Pro{ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func (s *writeFails) Owner(context.Context, string) (string, error) { return "", nil }

func (s *writeFails) Apply(context.Context, string, entitlement.Pro, string) (*entitlement.Pro, error) {
	s.t.Error("override wrote an entitlement")
	return nil, nil
}

func (s *writeFails) PurgeUser(context.Context, string) (int, error) {
	s.t.Error("override purged an entitlement")
	return 0, nil
}

var _ entitlement.Store = (*writeFails)(nil)

type rig struct {
	store *writeFails
	h     http.Handler
}

// newRig chains Middleware -> auth.Optional (the ai handler's verification;
// an unverified caller reaches the handler without a uid) -> a handler that
// answers Pro / free for the verified caller, or for ?uid= when given (a
// lookup of another account, e.g. an app's owner).
func newRig(t *testing.T, env map[string]string, storedPro map[string]bool) *rig {
	t.Helper()
	cfg, err := FromEnv(envOf(env))
	if err != nil {
		t.Fatal(err)
	}
	st := &writeFails{t: t, pro: storedPro}
	checker := Wrap(entitlement.NewProChecker(st, nil), cfg, nil)
	app := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := r.URL.Query().Get("uid")
		if uid == "" {
			uid = auth.UID(r.Context())
		}
		if checker.IsPro(r.Context(), uid) {
			_, _ = w.Write([]byte("pro"))
			return
		}
		_, _ = w.Write([]byte("free"))
	})
	return &rig{store: st, h: Middleware(cfg)(auth.Optional(auth.DevVerifier{}, auth.ModeDev)(app))}
}

func (r *rig) ask(t *testing.T, bearer, plan, query string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/x"+query, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if plan != "" {
		req.Header.Set(Header, plan)
	}
	rec := httptest.NewRecorder()
	r.h.ServeHTTP(rec, req)
	return rec.Body.String()
}

var stgOn = map[string]string{"APP_ENV": "stg", EnabledEnv: "1", UIDsEnv: "listed,listedPro"}

func TestOverrideAppliesToListedVerifiedCaller(t *testing.T) {
	r := newRig(t, stgOn, map[string]bool{"listedPro": true})
	if got := r.ask(t, "listed", "pro", ""); got != "pro" {
		t.Fatalf("listed + pro: got %s", got)
	}
	if got := r.ask(t, "listedPro", "free", ""); got != "free" {
		t.Fatalf("listed, stored Pro + free: got %s", got)
	}
	if got := r.ask(t, "listed", " PRO ", ""); got != "pro" {
		t.Fatalf("value is trimmed / case-insensitive: got %s", got)
	}
	if n := r.store.reads.Load(); n != 0 {
		t.Fatalf("an overridden answer must not read the store, read %d times", n)
	}
}

func TestOverrideOnlyForThatRequest(t *testing.T) {
	r := newRig(t, stgOn, map[string]bool{"listedPro": true})
	if got := r.ask(t, "listedPro", "free", ""); got != "free" {
		t.Fatalf("override: got %s", got)
	}
	// next request without the header: the stored entitlement, read from the
	// store (the override left nothing in the checker's cache)
	if got := r.ask(t, "listedPro", "", ""); got != "pro" {
		t.Fatalf("after the override: got %s", got)
	}
	if n := r.store.reads.Load(); n != 1 {
		t.Fatalf("want 1 store read after the override, got %d", n)
	}
	if got := r.ask(t, "listed", "pro", ""); got != "pro" {
		t.Fatalf("override pro: got %s", got)
	}
	if got := r.ask(t, "listed", "", ""); got != "free" {
		t.Fatalf("after override pro: got %s", got)
	}
}

func TestHeaderIgnored(t *testing.T) {
	stored := map[string]bool{"listedPro": true, "other": true}
	for _, tc := range []struct {
		name          string
		env           map[string]string
		bearer, plan  string
		query, wantIs string
	}{
		{"flag off", map[string]string{"APP_ENV": "stg", UIDsEnv: "listed"}, "listed", "pro", "", "free"},
		{"flag off, stored Pro", map[string]string{"APP_ENV": "stg", UIDsEnv: "listedPro"}, "listedPro", "free", "", "pro"},
		{"uid not listed", stgOn, "stranger", "pro", "", "free"},
		{"uid not listed, stored Pro", stgOn, "other", "free", "", "pro"},
		{"unauthenticated", stgOn, "", "pro", "", "free"},
		{"unauthenticated claims a listed uid", stgOn, "", "pro", "?uid=listed", "free"},
		{"another account than the caller", stgOn, "listed", "free", "?uid=listedPro", "pro"},
		{"unknown value", stgOn, "listed", "gold", "", "free"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, tc.env, stored)
			if got := r.ask(t, tc.bearer, tc.plan, tc.query); got != tc.wantIs {
				t.Fatalf("got %s, want %s", got, tc.wantIs)
			}
		})
	}
}

func TestOffPassesThrough(t *testing.T) {
	next := entitlement.NewProChecker(&writeFails{t: t}, nil)
	if got := Wrap(next, nil, nil); got != ProChecker(next) {
		t.Fatalf("off must return the checker as is, got %T", got)
	}
	h := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	if got := Middleware(nil)(h); got == nil {
		t.Fatal("off middleware returned nil")
	}
	var none ProChecker
	if Wrap(none, nil, nil) != nil {
		t.Fatal("off with no checker stays nil")
	}
}

func TestMiddlewareCarriesOnlyValidValues(t *testing.T) {
	cfg, _ := FromEnv(envOf(stgOn))
	var got Plan
	h := Middleware(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = Requested(r.Context()) }))
	for v, want := range map[string]Plan{"pro": Pro, "free": Free, "": "", "x": ""} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(Header, v)
		h.ServeHTTP(httptest.NewRecorder(), req)
		if got != want {
			t.Fatalf("%q: carried %q, want %q", v, got, want)
		}
	}
}
