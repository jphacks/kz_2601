package entitlement

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/googleplay"
)

// failStore fails the operations named in fail.
type failStore struct {
	*Memory
	fail map[string]bool
}

var errStore = errors.New("store unavailable")

func (s *failStore) Owner(ctx context.Context, key string) (string, error) {
	if s.fail["owner"] {
		return "", errStore
	}
	return s.Memory.Owner(ctx, key)
}

func (s *failStore) Apply(ctx context.Context, uid string, p Pro, from string) (*Pro, error) {
	if s.fail["apply"] {
		return nil, errStore
	}
	return s.Memory.Apply(ctx, uid, p, from)
}

func (s *failStore) Get(ctx context.Context, uid string) (*Pro, error) {
	if s.fail["get"] {
		return nil, errStore
	}
	return s.Memory.Get(ctx, uid)
}

func playNote(t *testing.T, v map[string]any) []byte {
	t.Helper()
	v["packageName"] = playPackage
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// boundPlay is a play env where alice has sent token (active for an hour).
func boundPlay(t *testing.T) (*env, *fakePlay, *failStore, *PlayNotifications) {
	t.Helper()
	st := &failStore{Memory: NewMemory()}
	e, fp := newPlayEnv(t, st)
	fp.set("tok", playSub("alice", "GPA.1", e.now.Add(time.Hour)))
	if _, err := e.svc.SubmitGoogle(context.Background(), "alice", product, "tok"); err != nil {
		t.Fatal(err)
	}
	return e, fp, st, &PlayNotifications{Service: e.svc, Verifier: allowPush{ok: true}}
}

// A voided (refunded) subscription ends Pro; voided one-time products are not ours.
func TestPlayNotificationVoided(t *testing.T) {
	ctx := context.Background()
	e, fp, _, n := boundPlay(t)
	inapp := playNote(t, map[string]any{"voidedPurchaseNotification": map[string]any{"purchaseToken": "tok", "productType": 2}})
	if out, err := n.Handle(ctx, inapp); err != nil || out != "ignored" {
		t.Fatalf("voided in-app product: %s %v", out, err)
	}
	if st, _ := e.svc.Get(ctx, "alice"); !st.IsPro {
		t.Fatal("a voided in-app product must not end Pro")
	}

	refunded := playSub("alice", "GPA.1", e.now.Add(-time.Second))
	refunded.State = googleplay.StateExpired
	fp.set("tok", refunded)
	voided := playNote(t, map[string]any{"voidedPurchaseNotification": map[string]any{"purchaseToken": "tok", "productType": voidedSubscription}})
	if out, err := n.Handle(ctx, voided); err != nil || out != "applied" {
		t.Fatalf("voided subscription: %s %v", out, err)
	}
	if st, _ := e.svc.Get(ctx, "alice"); st.IsPro {
		t.Fatal("a voided subscription must end Pro")
	}
}

func TestPlayNotificationIgnored(t *testing.T) {
	ctx := context.Background()
	e, fp, _, n := boundPlay(t)
	for name, data := range map[string][]byte{
		"not JSON":         []byte(strings.Repeat("{", 8)),
		"no package":       []byte(`{"subscriptionNotification":{"purchaseToken":"tok"}}`),
		"no kind":          playNote(t, map[string]any{"oneTimeProductNotification": map[string]any{"purchaseToken": "tok"}}),
		"empty token":      playNote(t, map[string]any{"subscriptionNotification": map[string]any{"notificationType": 2, "purchaseToken": ""}}),
		"voided no token":  playNote(t, map[string]any{"voidedPurchaseNotification": map[string]any{"productType": voidedSubscription}}),
		"not found by API": playNote(t, map[string]any{"subscriptionNotification": map[string]any{"purchaseToken": "tok"}}),
	} {
		if name == "not found by API" {
			fp.mu.Lock()
			delete(fp.subs, "tok")
			fp.mu.Unlock()
		}
		if out, err := n.Handle(ctx, data); err != nil || out != "ignored" {
			t.Errorf("%s: %s %v, want ignored", name, out, err)
		}
	}

	// a purchase of another product (or still pending) is not applied
	other := playSub("alice", "GPA.2", e.now.Add(48*time.Hour))
	other.LineItems[0].ProductID = "jp.kz2601.metamon.other"
	fp.set("tok", other)
	note := playNote(t, map[string]any{"subscriptionNotification": map[string]any{"notificationType": 2, "purchaseToken": "tok"}})
	if out, err := n.Handle(ctx, note); err != nil || out != "ignored" {
		t.Fatalf("other product: %s %v", out, err)
	}
	pending := playSub("alice", "GPA.3", time.Time{})
	fp.set("tok", pending)
	if out, err := n.Handle(ctx, note); err != nil || out != "ignored" {
		t.Fatalf("pending: %s %v", out, err)
	}
	if p, _ := e.svc.Store.Get(ctx, "alice"); p == nil || p.TransactionID != "GPA.1" {
		t.Fatalf("ignored notifications must not change Pro: %+v", p)
	}
}

// Store errors are transient: Handle fails (Pub/Sub redelivers) and the
// push endpoint answers 500.
func TestPlayNotificationStoreErrors(t *testing.T) {
	ctx := context.Background()
	e, fp, st, n := boundPlay(t)
	fp.set("tok", playSub("alice", "GPA.1..0", e.now.Add(31*24*time.Hour)))
	note := playNote(t, map[string]any{"subscriptionNotification": map[string]any{"notificationType": 2, "purchaseToken": "tok"}})
	for _, op := range []string{"owner", "apply"} {
		st.fail = map[string]bool{op: true}
		if _, err := n.Handle(ctx, note); !errors.Is(err, errStore) {
			t.Errorf("%s: %v, want the store error", op, err)
		}
	}

	mux := http.NewServeMux()
	RegisterPlayNotifications(mux, n)
	body, _ := json.Marshal(map[string]any{"message": map[string]any{"data": note}})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, PlayNotificationsPath, strings.NewReader(string(body))))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("store failure: %d %s", w.Code, w.Body)
	}

	st.fail = nil
	if out, err := n.Handle(ctx, note); err != nil || out != "applied" {
		t.Fatalf("redelivered: %s %v", out, err)
	}
}

func TestPlayNotificationsHandlerRejectsBodies(t *testing.T) {
	_, _, _, n := boundPlay(t)
	mux := http.NewServeMux()
	RegisterPlayNotifications(mux, n)
	for name, body := range map[string]string{
		"not JSON":     strings.Repeat("x", 32),
		"no message":   `{}`,
		"data not b64": `{"message":{"data":"%%%"}}`,
		"too large":    `{"message":{"data":"` + strings.Repeat("A", maxPushBytes) + `"}}`,
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, PlayNotificationsPath, strings.NewReader(body)))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, w.Code)
		}
	}
}

// A purchase that replaced a deleted account's purchase can be sent by the
// new account (the same Google account signed up again); store errors fail.
func TestSubmitGoogleLinkedEdgeCases(t *testing.T) {
	ctx := context.Background()
	st := &failStore{Memory: NewMemory()}
	e, fp := newPlayEnv(t, st)
	first := playSub("alice", "GPA.1", e.now.Add(time.Hour))
	first.ObfuscatedAccountID = ""
	fp.set("t1", first)
	if _, err := e.svc.SubmitGoogle(ctx, "alice", product, "t1"); err != nil {
		t.Fatal(err)
	}
	next := playSub("carol", "GPA.2", e.now.Add(48*time.Hour))
	next.LinkedPurchaseToken = "t1"
	fp.set("t2", next)

	st.fail = map[string]bool{"owner": true}
	if _, err := e.svc.SubmitGoogle(ctx, "carol", product, "t2"); !errors.Is(err, errStore) {
		t.Fatalf("owner lookup error: %v", err)
	}
	st.fail = nil

	e.svc.Deleted = func(context.Context, string) (bool, error) { return false, errors.New("auth down") }
	if _, err := e.svc.SubmitGoogle(ctx, "carol", product, "t2"); err == nil || errors.Is(err, ErrOwnedByOther) {
		t.Fatalf("a failed deleted-check must fail, not refuse: %v", err)
	}
	if owner, _ := st.Owner(ctx, PlayKey("t2")); owner != "" {
		t.Fatalf("t2 must stay unbound: %q", owner)
	}

	e.deleted["alice"] = true
	e.svc.Deleted = func(_ context.Context, uid string) (bool, error) { return e.deleted[uid], nil }
	got, err := e.svc.SubmitGoogle(ctx, "carol", product, "t2")
	if err != nil || !got.IsPro || got.Pro.TransactionID != "GPA.2" {
		t.Fatalf("linked from a deleted account: %+v %v", got, err)
	}

	acks := fp.ackCount()
	st.fail = map[string]bool{"apply": true}
	fp.set("t3", playSub("dave", "GPA.3", e.now.Add(time.Hour)))
	if _, err := e.svc.SubmitGoogle(ctx, "dave", product, "t3"); !errors.Is(err, errStore) {
		t.Fatalf("apply error: %v", err)
	}
	if fp.ackCount() != acks {
		t.Fatalf("a purchase that was not stored must not be acknowledged (acks %d -> %d)", acks, fp.ackCount())
	}
	st.fail = map[string]bool{"get": true}
	if _, err := e.svc.Get(ctx, "carol"); !errors.Is(err, errStore) {
		t.Fatalf("get error: %v", err)
	}
}
