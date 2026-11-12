package extapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// connect runs start → consent (the fake consents at once) → callback for uid.
func (e *env) connect(uid string) {
	e.t.Helper()
	ctx := context.Background()
	auth, err := e.svc.Start(ctx, uid, FakeOAuth, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	u, err := url.Parse(auth)
	if err != nil || !strings.HasPrefix(auth, "https://runner.example/oauth/callback/"+FakeOAuth+"?") {
		e.t.Fatalf("auth url %q %v", auth, err)
	}
	back := e.svc.Callback(ctx, FakeOAuth, u.Query().Get("state"), u.Query().Get("code"))
	if back != "tailorit://connected?provider="+FakeOAuth+"&status=ok" {
		e.t.Fatalf("callback → %q", back)
	}
}

func (e *env) conn(uid string) (Connection, bool) {
	e.t.Helper()
	c, ok, err := e.mem.Get(context.Background(), uid, FakeOAuth)
	if err != nil {
		e.t.Fatal(err)
	}
	return c, ok
}

// OAuth: not connected is 409 and counts nothing; once connected the
// person's token reaches the op.
func TestOAuthConnectAndCall(t *testing.T) {
	e := newEnv(t, opts{})
	_, err := e.call(e.req(FakeOAuth, "listEvents", nil))
	if !errors.Is(err, ErrNotConnected) || StatusOf(Reason(err)) != http.StatusConflict || e.runs() != 0 {
		t.Fatalf("not connected: %v runs=%d", err, e.runs())
	}
	e.connect(user)
	c, ok := e.conn(user)
	if !ok || c.Status != StatusActive || len(c.Sealed) == 0 || !slicesEqual(c.Scopes, []string{"fake.read"}) {
		t.Fatalf("connection %+v", c)
	}
	if bytes.Contains(c.Sealed, []byte("fake-access")) || bytes.Contains(c.Sealed, []byte("fake-refresh")) {
		t.Fatal("token stored in the clear")
	}
	res, err := e.call(e.req(FakeOAuth, "listEvents", nil))
	if err != nil || len(res.Output.Rows) != 1 || e.runs() != 1 {
		t.Fatalf("connected call: %+v %v runs=%d", res, err, e.runs())
	}
	// someone else's token is never used: the creator has none
	r := e.req(FakeOAuth, "listEvents", nil)
	r.UserUID = ""
	if _, err := e.call(r); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("server rule with the creator's (missing) token: %v", err)
	}
}

// A state works once, for its provider, for 10 minutes.
func TestOAuthState(t *testing.T) {
	e := newEnv(t, opts{})
	ctx := context.Background()
	auth, err := e.svc.Start(ctx, user, FakeOAuth, nil)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	state := u.Query().Get("state")
	if got := e.svc.Callback(ctx, FakeFree, state, "fake-code"); !strings.HasSuffix(got, "status=error") {
		t.Fatalf("other provider: %q", got)
	}
	if got := e.svc.Callback(ctx, FakeOAuth, state, "bad-code"); !strings.HasSuffix(got, "status=error") {
		t.Fatalf("bad code: %q", got)
	}
	// the failed exchange used the state up
	if got := e.svc.Callback(ctx, FakeOAuth, state, "fake-code"); !strings.HasSuffix(got, "status=error") {
		t.Fatalf("reused state: %q", got)
	}
	auth, _ = e.svc.Start(ctx, user, FakeOAuth, nil)
	u, _ = url.Parse(auth)
	e.c.t = e.c.t.Add(StateTTL + time.Second)
	if got := e.svc.Callback(ctx, FakeOAuth, u.Query().Get("state"), "fake-code"); !strings.HasSuffix(got, "status=error") {
		t.Fatalf("expired state: %q", got)
	}
	if _, ok := e.conn(user); ok {
		t.Fatal("connected with a bad state")
	}
	if _, err := e.svc.Start(ctx, user, FakeOAuth, []string{"other.scope"}); !errors.Is(err, ErrUnknown) {
		t.Fatalf("unknown scope: %v", err)
	}
}

// An expired access token is refreshed; a refused refresh turns the
// connection to error (and the call to 409).
func TestOAuthRefresh(t *testing.T) {
	e := newEnv(t, opts{})
	e.connect(user)
	// an access token past its expiry
	if err := e.svc.Conns.Save(context.Background(), user, FakeOAuth, Token{AccessToken: "fake-access-1", RefreshToken: "fake-refresh-1",
		Expiry: e.c.t.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(FakeOAuth, "listEvents", nil)); err != nil {
		t.Fatal(err)
	}
	tok, err := e.svc.Conns.Token(context.Background(), user, FakeOAuth)
	if err != nil || tok.AccessToken != "fake-access-2" || tok.RefreshToken != "fake-refresh-1" {
		t.Fatalf("refreshed: %v", err)
	}
	// the provider refuses the token (revoked on its side)
	_, err = e.call(e.req(FakeOAuth, "listEvents", map[string]string{"fail": "grant"}))
	if !errors.Is(err, ErrNotConnected) || e.runs() != 1 {
		t.Fatalf("invalid grant: %v runs=%d", err, e.runs())
	}
	if c, _ := e.conn(user); c.Status != StatusError {
		t.Fatalf("status %q", c.Status)
	}
	if _, err := e.call(e.req(FakeOAuth, "listEvents", nil)); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("after error: %v", err)
	}
}

// Revoke deletes the tokens at once and keeps only the revoked marker
// (決定 7); the marker goes after 90 days.
func TestRevoke(t *testing.T) {
	e := newEnv(t, opts{})
	e.connect(user)
	if err := e.svc.Revoke(context.Background(), user, FakeOAuth); err != nil {
		t.Fatal(err)
	}
	c, ok := e.conn(user)
	if !ok || c.Status != StatusRevoked || len(c.Sealed) != 0 || c.RevokedAt.IsZero() {
		t.Fatalf("marker %+v", c)
	}
	if _, err := e.call(e.req(FakeOAuth, "listEvents", nil)); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("after revoke: %v", err)
	}
	if l, _ := e.svc.Conns.List(context.Background(), user); len(l) != 0 {
		t.Fatalf("list shows the marker: %+v", l)
	}
	e.c.t = e.c.t.Add(DefaultUnusedTTL + time.Hour)
	if n, err := e.svc.Conns.ExpireUnused(context.Background(), 10); err != nil || n != 1 {
		t.Fatalf("expire marker: %d %v", n, err)
	}
	if _, ok := e.conn(user); ok {
		t.Fatal("marker kept past 90 days")
	}
	// account deletion cuts everything
	e.connect(user)
	if n, err := e.svc.RevokeAll(context.Background(), user); err != nil || n != 1 {
		t.Fatalf("revoke all: %d %v", n, err)
	}
}

// 90 days unused → deleted (on the next read, and by ExpireUnused); use
// moves the date.
func TestUnusedExpiry(t *testing.T) {
	e := newEnv(t, opts{})
	e.connect(user)
	e.c.t = e.c.t.Add(80 * 24 * time.Hour)
	if _, err := e.call(e.req(FakeOAuth, "listEvents", nil)); err != nil {
		t.Fatalf("day 80: %v", err)
	}
	c, _ := e.conn(user)
	if !c.DeleteAt.Equal(e.c.t.UTC().Add(DefaultUnusedTTL)) {
		t.Fatalf("use did not move deleteAt: %v", c.DeleteAt)
	}
	e.c.t = e.c.t.Add(DefaultUnusedTTL - time.Hour)
	if n, _ := e.svc.Conns.ExpireUnused(context.Background(), 10); n != 0 {
		t.Fatal("expired too early")
	}
	e.c.t = e.c.t.Add(2 * time.Hour)
	if _, err := e.call(e.req(FakeOAuth, "listEvents", nil)); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("after 90 days: %v", err)
	}
	if _, ok := e.conn(user); ok {
		t.Fatal("unused connection kept")
	}
	// the sweep path
	e.connect("other")
	e.c.t = e.c.t.Add(DefaultUnusedTTL + time.Minute)
	if n, err := e.svc.Conns.ExpireUnused(context.Background(), 10); err != nil || n != 1 {
		t.Fatalf("sweep %d %v", n, err)
	}
}

// A sealed token cannot be opened as another person's or another provider's.
func TestSealBindsOwner(t *testing.T) {
	e := newEnv(t, opts{})
	e.connect(user)
	c, _ := e.conn(user)
	if err := e.mem.Put(context.Background(), "thief", c); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Conns.Token(context.Background(), "thief", FakeOAuth); err == nil {
		t.Fatal("a copied sealed token opened for another uid")
	}
}

// No token, code, refresh token or secret reaches the logs or an answer.
func TestTokensStayOut(t *testing.T) {
	e := newEnv(t, opts{})
	e.connect(user)
	mux := http.NewServeMux()
	e.svc.Routes(mux)
	var bodies bytes.Buffer
	post := func(path string, body any) int {
		b, _ := json.Marshal(body)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b)))
		bodies.Write(rec.Body.Bytes())
		bodies.WriteString(rec.Header().Get("Location"))
		return rec.Code
	}
	if code := post("/internal/ext/call", CallBody{Provider: FakeOAuth, Op: "listEvents", AppID: appID, CreatorUID: creator, UserUID: user, RequestID: "r1"}); code != 200 {
		t.Fatalf("call %d %s", code, bodies.String())
	}
	if code := post("/internal/ext/connections/list", ConnBody{UID: user}); code != 200 {
		t.Fatalf("list %d", code)
	}
	if code := post("/internal/ext/connections/start", ConnBody{UID: user, Provider: FakeOAuth}); code != 200 {
		t.Fatalf("start %d", code)
	}
	if code := post("/internal/ext/call", CallBody{Provider: FakeOAuth, Op: "listEvents", CreatorUID: creator, UserUID: "nobody", RequestID: "r2"}); code != http.StatusConflict {
		t.Fatalf("not connected %d", code)
	}
	if code := post("/internal/ext/connections/revoke", ConnBody{UID: user, Provider: FakeOAuth}); code != 200 {
		t.Fatalf("revoke %d", code)
	}
	tok := Token{AccessToken: "fake-access-1", RefreshToken: "fake-refresh-1"}
	e.svc.Log.Info("check", slog.Any("token", tok), slog.Any("secrets", Secrets{APIKey: "k-secret"}), slog.Any("ptr", &tok))
	if s := fmt.Sprintf("%v %+v %#v", tok, tok, tok); strings.Contains(s, "fake-access") {
		t.Fatalf("token printed: %s", s)
	}
	for _, leak := range []string{"fake-access", "fake-refresh", "fake-code", "k-secret"} {
		if strings.Contains(e.logs.String(), leak) {
			t.Fatalf("%q in logs:\n%s", leak, e.logs.String())
		}
		// the fake's consent URL carries its own code by design
		if leak != "fake-code" && strings.Contains(bodies.String(), leak) {
			t.Fatalf("%q in an answer: %s", leak, bodies.String())
		}
	}
	if !strings.Contains(e.logs.String(), `"event":"ext.call"`) {
		t.Fatal("no ext.call line")
	}
}

// The callback route redirects to the app with no code in the URL.
func TestCallbackRoute(t *testing.T) {
	e := newEnv(t, opts{})
	auth, err := e.svc.Start(context.Background(), user, FakeOAuth, nil)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	e.svc.Routes(mux)
	u, _ := url.Parse(auth)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, u.RequestURI(), nil))
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusFound || loc != "tailorit://connected?provider="+FakeOAuth+"&status=ok" {
		t.Fatalf("%d %q", rec.Code, loc)
	}
}

func slicesEqual(a, b []string) bool { return strings.Join(a, ",") == strings.Join(b, ",") }

// ctxRevoker fails a revoke whose context is already done, like a real
// provider call would.
type ctxRevoker struct {
	fakeOAuthClient
	calls *atomic.Int64
}

func (c ctxRevoker) Revoke(ctx context.Context, _ Token) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.calls.Add(1)
	return nil
}

// The caller hanging up (appconfig stops waiting) does not cut revokeAll
// short: the provider is still asked to revoke and the token is deleted.
func TestRevokeAllOutlivesCaller(t *testing.T) {
	e := newEnv(t, opts{})
	e.connect(user)
	var calls atomic.Int64
	e.svc.oauth[FakeOAuth] = ctxRevoker{calls: &calls}
	mux := http.NewServeMux()
	e.svc.Routes(mux)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the caller is gone before the runner starts
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/internal/ext/connections/revokeAll", strings.NewReader(`{"uid":"`+user+`"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"revoked":1`) {
		t.Fatalf("revokeAll: %d %s", w.Code, w.Body.String())
	}
	if calls.Load() != 1 {
		t.Fatalf("provider revoke calls = %d", calls.Load())
	}
	if c, ok := e.conn(user); !ok || c.Status != StatusRevoked || len(c.Sealed) != 0 {
		t.Fatalf("connection %+v", c)
	}
}
