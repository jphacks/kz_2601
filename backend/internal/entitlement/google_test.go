package entitlement

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/api/idtoken"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/googleplay"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

const playPackage = "jp.kz2601.metamon"

// fakePlay is the Play Developer API in tests (token -> purchase).
type fakePlay struct {
	mu     sync.Mutex
	subs   map[string]*googleplay.Subscription
	err    error // returned by Subscription when set
	ackErr error
	acks   []string // tokens acknowledged
}

func newFakePlay() *fakePlay { return &fakePlay{subs: map[string]*googleplay.Subscription{}} }

func (f *fakePlay) Subscription(_ context.Context, pkg, token string) (*googleplay.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	s, ok := f.subs[token]
	if !ok || pkg != playPackage {
		return nil, googleplay.ErrNotFound
	}
	c := *s
	return &c, nil
}

func (f *fakePlay) Acknowledge(_ context.Context, pkg, productID, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ackErr != nil {
		return f.ackErr
	}
	if pkg != playPackage || productID != product {
		return errors.New("unexpected acknowledge")
	}
	f.acks = append(f.acks, token)
	if s, ok := f.subs[token]; ok {
		s.AcknowledgementState = googleplay.AckAcknowledged
	}
	return nil
}

func (f *fakePlay) set(token string, s *googleplay.Subscription) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subs[token] = s
}

func (f *fakePlay) ackCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.acks)
}

// playSub is an active monthly purchase made by uid, not yet acknowledged.
func playSub(uid, order string, expires time.Time) *googleplay.Subscription {
	return &googleplay.Subscription{
		State:                googleplay.StateActive,
		AcknowledgementState: googleplay.AckPending,
		ObfuscatedAccountID:  AccountToken(uid),
		LineItems:            []googleplay.LineItem{{ProductID: product, ExpiryTime: expires, AutoRenew: true, OrderID: order}},
	}
}

func newPlayEnv(t *testing.T, st Store) (*env, *fakePlay) {
	e := newEnv(t, st)
	fp := newFakePlay()
	e.svc.GooglePlay = &GooglePlay{API: fp, Policy: googleplay.Policy{PackageName: playPackage, ProductIDs: []string{product}}}
	return e, fp
}

func TestSubmitGoogleGrantsProAndAcknowledges(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		e, fp := newPlayEnv(t, st)
		ctx := context.Background()
		alice, token := uid(t, "alice"), uid(t, "token")
		fp.set(token, playSub(alice, "GPA.1", e.now.Add(30*24*time.Hour)))

		got, err := e.svc.SubmitGoogle(ctx, alice, product, token)
		if err != nil {
			t.Fatal(err)
		}
		if !got.IsPro || got.Pro.Platform != PlatformGooglePlay || got.Pro.Environment != "Production" ||
			got.Pro.TransactionID != "GPA.1" || got.Pro.OriginalTransactionID != PlayKey(token) {
			t.Fatalf("status %+v", got.Pro)
		}
		if strings.Contains(got.Pro.OriginalTransactionID, token) {
			t.Fatal("the purchase token must not be stored")
		}
		if fp.ackCount() != 1 {
			t.Fatalf("pending purchase must be acknowledged once: %d", fp.ackCount())
		}
		// GET /v1/me/entitlements reads the same document
		if st2, err := e.svc.Get(ctx, alice); err != nil || !st2.IsPro || st2.Pro.Platform != PlatformGooglePlay {
			t.Fatalf("get %+v %v", st2, err)
		}
		// already acknowledged: sent again (restore) without a second acknowledge
		if _, err := e.svc.SubmitGoogle(ctx, alice, product, token); err != nil {
			t.Fatal(err)
		}
		if fp.ackCount() != 1 {
			t.Fatalf("acknowledged purchase acknowledged again: %d", fp.ackCount())
		}
	})
}

func TestSubmitGoogleAckFailureStillGrants(t *testing.T) {
	e, fp := newPlayEnv(t, NewMemory())
	fp.ackErr = errors.New("HTTP 500")
	fp.set("tok", playSub("u", "GPA.1", e.now.Add(time.Hour)))
	got, err := e.svc.SubmitGoogle(context.Background(), "u", product, "tok")
	if err != nil || !got.IsPro {
		t.Fatalf("%+v %v (the app acknowledges too)", got, err)
	}
}

func TestSubmitGoogleExpiredAndStates(t *testing.T) {
	e, fp := newPlayEnv(t, NewMemory())
	ctx := context.Background()
	exp := playSub("u", "GPA.1", e.now.Add(-time.Hour))
	exp.State = googleplay.StateExpired
	fp.set("old", exp)
	got, err := e.svc.SubmitGoogle(ctx, "u", product, "old")
	if err != nil || got.IsPro || got.Pro == nil {
		t.Fatalf("expired is stored as not pro: %+v %v", got, err)
	}
	if fp.ackCount() != 0 {
		t.Fatal("an expired purchase is not acknowledged")
	}

	// on hold with a later expiry: no Pro
	hold := playSub("u", "GPA.2", e.now.Add(time.Hour))
	hold.State = googleplay.StateOnHold
	fp.set("hold", hold)
	if got, err := e.svc.SubmitGoogle(ctx, "u", product, "hold"); err != nil || got.IsPro {
		t.Fatalf("on hold: %+v %v", got, err)
	}

	// canceled (auto-renew off) stays Pro until the expiry
	canceled := playSub("u", "GPA.3", e.now.Add(time.Hour))
	canceled.State = googleplay.StateCanceled
	fp.set("canceled", canceled)
	got, err = e.svc.SubmitGoogle(ctx, "u", product, "canceled")
	if err != nil || !got.IsPro || got.Pro.WillRenew == nil || *got.Pro.WillRenew {
		t.Fatalf("canceled: %+v %v", got, err)
	}

	// the expired one sent again does not replace the active one
	if got, err := e.svc.SubmitGoogle(ctx, "u", product, "old"); err != nil || !got.IsPro {
		t.Fatalf("older purchase replaced a newer one: %+v %v", got, err)
	}

	// pending (not paid yet)
	pending := playSub("u", "", time.Time{})
	pending.State = googleplay.StatePending
	fp.set("pending", pending)
	var perr *googleplay.Error
	if _, err := e.svc.SubmitGoogle(ctx, "u", product, "pending"); !errors.As(err, &perr) || perr.Reason != "pending" {
		t.Fatalf("pending: %v", err)
	}
}

func TestSubmitGoogleTestPurchaseIsSandbox(t *testing.T) {
	e, fp := newPlayEnv(t, NewMemory())
	s := playSub("u", "GPA.1", e.now.Add(time.Hour))
	s.Test = true
	fp.set("tok", s)
	got, err := e.svc.SubmitGoogle(context.Background(), "u", product, "tok")
	if err != nil || got.Pro.Environment != "Sandbox" || !got.IsPro {
		t.Fatalf("%+v %v", got, err)
	}
	e.svc.Environments = []string{"Production"}
	if st, _ := e.svc.Get(context.Background(), "u"); st.IsPro {
		t.Fatal("a test purchase must not count where only Production is accepted")
	}
}

func TestSubmitGoogleAccountMismatch(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		e, fp := newPlayEnv(t, st)
		ctx := context.Background()
		alice, bob, token := uid(t, "alice"), uid(t, "bob"), uid(t, "token")
		fp.set(token, playSub(alice, "GPA.1", e.now.Add(time.Hour)))
		if _, err := e.svc.SubmitGoogle(ctx, bob, product, token); !errors.Is(err, ErrAccountMismatch) {
			t.Fatalf("want ErrAccountMismatch, got %v", err)
		}
		if p, _ := st.Get(ctx, bob); p != nil {
			t.Fatal("nothing is stored on a mismatch")
		}
		if owner, _ := st.Owner(ctx, PlayKey(token)); owner != "" {
			t.Fatalf("not bound on a mismatch: %q", owner)
		}
		if fp.ackCount() != 0 {
			t.Fatal("not acknowledged on a mismatch")
		}
	})
}

func TestSubmitGoogleTokenReuseAcrossUIDs(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		e, fp := newPlayEnv(t, st)
		ctx := context.Background()
		alice, bob, token := uid(t, "alice"), uid(t, "bob"), uid(t, "token")
		s := playSub(alice, "GPA.1", e.now.Add(time.Hour))
		s.ObfuscatedAccountID = "" // e.g. bought outside the app: the binding still decides
		fp.set(token, s)
		if _, err := e.svc.SubmitGoogle(ctx, alice, product, token); err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.SubmitGoogle(ctx, bob, product, token); !errors.Is(err, ErrOwnedByOther) {
			t.Fatalf("want ErrOwnedByOther, got %v", err)
		}
		if p, _ := st.Get(ctx, bob); p != nil {
			t.Fatal("bob must not get Pro")
		}
		// alice deleted her account: the purchase moves
		e.deleted[alice] = true
		if got, err := e.svc.SubmitGoogle(ctx, bob, product, token); err != nil || !got.IsPro {
			t.Fatalf("transfer from a deleted account: %+v %v", got, err)
		}
		// purge removes the Play binding with the Pro document
		if n, err := st.PurgeUser(ctx, bob); err != nil || n != 2 {
			t.Fatalf("purge %d %v", n, err)
		}
		if owner, _ := st.Owner(ctx, PlayKey(token)); owner != "" {
			t.Fatalf("binding left after purge: %q", owner)
		}
	})
}

func TestSubmitGoogleLinkedPurchase(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		e, fp := newPlayEnv(t, st)
		ctx := context.Background()
		alice, bob := uid(t, "alice"), uid(t, "bob")
		t1, t2, t3 := uid(t, "t1"), uid(t, "t2"), uid(t, "t3")
		first := playSub(alice, "GPA.1", e.now.Add(time.Hour))
		first.ObfuscatedAccountID = ""
		fp.set(t1, first)
		if _, err := e.svc.SubmitGoogle(ctx, alice, product, t1); err != nil {
			t.Fatal(err)
		}
		// resubscribe replacing t1, sent by alice: fine
		next := playSub(alice, "GPA.2", e.now.Add(48*time.Hour))
		next.LinkedPurchaseToken = t1
		fp.set(t2, next)
		got, err := e.svc.SubmitGoogle(ctx, alice, product, t2)
		if err != nil || got.Pro.TransactionID != "GPA.2" {
			t.Fatalf("linked by the same uid: %+v %v", got, err)
		}
		// a purchase replacing alice's t1, sent by bob: refused
		other := playSub(bob, "GPA.3", e.now.Add(72*time.Hour))
		other.LinkedPurchaseToken = t1
		fp.set(t3, other)
		if _, err := e.svc.SubmitGoogle(ctx, bob, product, t3); !errors.Is(err, ErrOwnedByOther) {
			t.Fatalf("want ErrOwnedByOther, got %v", err)
		}
		if owner, _ := st.Owner(ctx, PlayKey(t3)); owner != "" {
			t.Fatalf("t3 must stay unbound: %q", owner)
		}
	})
}

func TestSubmitGoogleErrors(t *testing.T) {
	e, fp := newPlayEnv(t, NewMemory())
	ctx := context.Background()
	var perr *googleplay.Error
	if _, err := e.svc.SubmitGoogle(ctx, "u", "jp.kz2601.metamon.other", "tok"); !errors.As(err, &perr) || perr.Reason != "product_id" {
		t.Fatalf("product: %v", err)
	}
	if _, err := e.svc.SubmitGoogle(ctx, "u", product, "unknown"); !errors.As(err, &perr) || perr.Reason != "not_found" {
		t.Fatalf("unknown token: %v", err)
	}
	fp.err = errors.New("googleplay: HTTP 503")
	if _, err := e.svc.SubmitGoogle(ctx, "u", product, "tok"); !errors.Is(err, ErrPlayUnavailable) {
		t.Fatalf("unavailable: %v", err)
	}
	e.svc.GooglePlay = nil
	if _, err := e.svc.SubmitGoogle(ctx, "u", product, "tok"); !errors.Is(err, ErrPlayNotConfigured) {
		t.Fatalf("not configured: %v", err)
	}
}

func TestGoogleHandler(t *testing.T) {
	e, fp := newPlayEnv(t, NewMemory())
	var changed []string
	e.svc.Changed = func(uid string) { changed = append(changed, uid) }
	mux := http.NewServeMux()
	Register(mux, e.svc)
	h := auth.Middleware(auth.DevVerifier{}, auth.ModeDev)(mux)
	do := func(uid string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		_ = json.NewEncoder(&buf).Encode(body)
		r := httptest.NewRequest("POST", "/v1/me/entitlements/google", &buf)
		r.Header.Set("Authorization", "Bearer "+uid)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	fp.set("tok", playSub("u1", "GPA.1", e.now.Add(time.Hour)))
	req := map[string]string{"productId": product, "purchaseToken": "tok"}

	if w := do("u1", map[string]string{"productId": product}); w.Code != 400 {
		t.Fatalf("missing token: %d", w.Code)
	}
	if w := do("u2", req); w.Code != 409 || !strings.Contains(w.Body.String(), httpx.CodeAccountMismatch) {
		t.Fatalf("mismatch: %d %s", w.Code, w.Body)
	}
	if w := do("u1", map[string]string{"productId": product, "purchaseToken": "nope"}); w.Code != 422 ||
		!strings.Contains(w.Body.String(), `"reason":"not_found"`) {
		t.Fatalf("unknown: %d %s", w.Code, w.Body)
	}
	if len(changed) != 0 {
		t.Fatalf("Changed after a rejected purchase: %v", changed)
	}
	w := do("u1", req)
	var st Status
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &st) != nil || !st.IsPro || st.Pro.ProductID != product {
		t.Fatalf("ok: %d %s", w.Code, w.Body)
	}
	if len(changed) != 1 || changed[0] != "u1" {
		t.Fatalf("Changed: %v", changed)
	}
	// another account with a purchase made out of the app (no account id)
	s := playSub("u1", "GPA.1", e.now.Add(time.Hour))
	s.ObfuscatedAccountID = ""
	fp.set("tok", s)
	if w := do("u3", req); w.Code != 409 || !strings.Contains(w.Body.String(), httpx.CodeTransactionOwnedByOther) {
		t.Fatalf("owned: %d %s", w.Code, w.Body)
	}
	fp.err = errors.New("googleplay: HTTP 500")
	if w := do("u1", req); w.Code != 503 || !strings.Contains(w.Body.String(), httpx.CodeUpstreamUnavailable) {
		t.Fatalf("unavailable: %d %s", w.Code, w.Body)
	}
	e.svc.GooglePlay = nil
	if w := do("u1", req); w.Code != 503 || !strings.Contains(w.Body.String(), httpx.CodeNotConfigured) {
		t.Fatalf("not configured: %d %s", w.Code, w.Body)
	}
}

type allowPush struct{ ok bool }

func (a allowPush) Verify(context.Context, string) error {
	if !a.ok {
		return errors.New("no")
	}
	return nil
}

func TestPlayNotifications(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		e, fp := newPlayEnv(t, st)
		ctx := context.Background()
		alice, token := uid(t, "alice"), uid(t, "token")
		fp.set(token, playSub(alice, "GPA.1", e.now.Add(time.Hour)))
		if _, err := e.svc.SubmitGoogle(ctx, alice, product, token); err != nil {
			t.Fatal(err)
		}
		var changed []string
		e.svc.Changed = func(uid string) { changed = append(changed, uid) }
		n := &PlayNotifications{Service: e.svc, Verifier: allowPush{ok: true}}
		note := func(tok string, typ int) []byte {
			b, _ := json.Marshal(map[string]any{"packageName": playPackage,
				"subscriptionNotification": map[string]any{"notificationType": typ, "purchaseToken": tok, "subscriptionId": product}})
			return b
		}

		// renewal: a later expiry and a new order
		fp.set(token, playSub(alice, "GPA.1..0", e.now.Add(31*24*time.Hour)))
		if out, err := n.Handle(ctx, note(token, 2)); err != nil || out != "applied" {
			t.Fatalf("renewed: %s %v", out, err)
		}
		if p, _ := st.Get(ctx, alice); p == nil || p.TransactionID != "GPA.1..0" || !p.Active(e.now.Add(30*24*time.Hour), nil) {
			t.Fatalf("after renewal %+v", p)
		}
		if len(changed) != 1 || changed[0] != alice {
			t.Fatalf("Changed: %v", changed)
		}

		// refund / revoke: expired now
		revoked := playSub(alice, "GPA.1..0", e.now.Add(-time.Minute))
		revoked.State = googleplay.StateExpired
		fp.set(token, revoked)
		if out, err := n.Handle(ctx, note(token, 12)); err != nil || out != "applied" {
			t.Fatalf("revoked: %s %v", out, err)
		}
		if st2, _ := e.svc.Get(ctx, alice); st2.IsPro {
			t.Fatal("revoked must end Pro")
		}

		// nobody sent this purchase yet
		other := uid(t, "other")
		fp.set(other, playSub(alice, "GPA.9", e.now.Add(time.Hour)))
		if out, _ := n.Handle(ctx, note(other, 4)); out != "unbound" {
			t.Fatalf("unbound: %s", out)
		}
		if out, _ := n.Handle(ctx, []byte(`{"packageName":"`+playPackage+`","testNotification":{"version":"1.0"}}`)); out != "test" {
			t.Fatalf("test: %s", out)
		}
		if out, _ := n.Handle(ctx, []byte(`{"packageName":"com.example","subscriptionNotification":{"purchaseToken":"x"}}`)); out != "ignored" {
			t.Fatalf("other package: %s", out)
		}
		fp.err = errors.New("googleplay: HTTP 500")
		if _, err := n.Handle(ctx, note(token, 2)); err == nil {
			t.Fatal("a transient API error must be retried")
		}
	})
}

func TestPlayNotificationsHandler(t *testing.T) {
	e, fp := newPlayEnv(t, NewMemory())
	fp.set("tok", playSub("u", "GPA.1", e.now.Add(time.Hour)))
	if _, err := e.svc.SubmitGoogle(context.Background(), "u", product, "tok"); err != nil {
		t.Fatal(err)
	}
	post := func(v PushVerifier, body string) *httptest.ResponseRecorder {
		mux := http.NewServeMux()
		RegisterPlayNotifications(mux, &PlayNotifications{Service: e.svc, Verifier: v})
		r := httptest.NewRequest("POST", PlayNotificationsPath, strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	data, _ := json.Marshal(map[string]any{"packageName": playPackage,
		"subscriptionNotification": map[string]any{"notificationType": 2, "purchaseToken": "tok"}})
	body, _ := json.Marshal(map[string]any{"message": map[string]any{"data": data, "messageId": "1"}, "subscription": "s"})

	if w := post(allowPush{ok: false}, string(body)); w.Code != 401 {
		t.Fatalf("unauthenticated: %d", w.Code)
	}
	if w := post(allowPush{ok: true}, `{"message":{}}`); w.Code != 400 {
		t.Fatalf("empty: %d", w.Code)
	}
	if w := post(allowPush{ok: true}, string(body)); w.Code != 200 || !strings.Contains(w.Body.String(), `"applied"`) {
		t.Fatalf("ok: %d %s", w.Code, w.Body)
	}
}

func TestOIDCPushVerifier(t *testing.T) {
	claims := map[string]any{"email": "push@demo.iam.gserviceaccount.com", "email_verified": true}
	v := &OIDCPushVerifier{Audience: "aud", Email: "push@demo.iam.gserviceaccount.com",
		Validate: func(_ context.Context, tok, aud string) (*idtoken.Payload, error) {
			if tok != "good" || aud != "aud" {
				return nil, errors.New("bad token")
			}
			return &idtoken.Payload{Claims: claims}, nil
		}}
	ctx := context.Background()
	if err := v.Verify(ctx, "Bearer good"); err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"", "good", "Bearer ", "Bearer bad"} {
		if v.Verify(ctx, h) == nil {
			t.Fatalf("%q must fail", h)
		}
	}
	claims["email"] = "someone@example.com"
	if v.Verify(ctx, "Bearer good") == nil {
		t.Fatal("another account must fail")
	}
}

func TestPlayBindingsStayOutOfAppStoreReconcile(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	now := time.Now()
	if _, err := m.Apply(ctx, "u", Pro{OriginalTransactionID: PlayKey("tok"), TransactionID: "GPA.1", ExpiresAt: now.Add(time.Hour), Platform: PlatformGooglePlay}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply(ctx, "u", Pro{OriginalTransactionID: "apple-1", TransactionID: "t", ExpiresAt: now.Add(2 * time.Hour)}, ""); err != nil {
		t.Fatal(err)
	}
	b, err := m.Bindings(ctx, "", 10)
	if err != nil || len(b) != 1 || b[0].OriginalTransactionID != "apple-1" {
		t.Fatalf("bindings %+v %v", b, err)
	}
	if n, err := m.PurgeUser(ctx, "u"); err != nil || n != 3 {
		t.Fatalf("purge %d %v", n, err)
	}
}

func TestGooglePlayConfigFromEnv(t *testing.T) {
	p := GooglePlayConfigFromEnv(func(string) string { return "" })
	if p.PackageName != playPackage || len(p.ProductIDs) != 1 || p.ProductIDs[0] != product {
		t.Fatalf("%+v", p)
	}
	p = GooglePlayConfigFromEnv(func(k string) string {
		return map[string]string{"GOOGLE_PLAY_PACKAGE_NAME": "jp.kz2601.metamon.stg", "GOOGLE_PLAY_PRODUCT_IDS": "a, b"}[k]
	})
	if p.PackageName != "jp.kz2601.metamon.stg" || len(p.ProductIDs) != 2 {
		t.Fatalf("%+v", p)
	}
}
