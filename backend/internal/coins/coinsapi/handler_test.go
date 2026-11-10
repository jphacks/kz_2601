package coinsapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
	"github.com/jphacks/kz_2601/backend/internal/appstore/appstoretest"
	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/coins"
)

const (
	bundle = "jp.kz2601.metamon"
	pack   = "jp.kz2601.metamon.coins.test"
)

type env struct {
	signer *appstoretest.Signer
	svc    *coins.Service
	mux    *http.ServeMux
	now    time.Time
}

func newEnv(t *testing.T, mode coins.Mode) *env {
	t.Helper()
	e := &env{signer: appstoretest.New(t, appstoretest.Options{}), now: time.Now(), mux: http.NewServeMux()}
	e.svc = &coins.Service{
		Config: coins.Config{Mode: mode, PerUSD: 100, MinPerCall: 1, HoldTTL: 10 * time.Minute, InitialGrant: 3,
			Reserve: map[string]int64{coins.OpGenerate: 5}, Packs: []coins.Pack{{ProductID: pack, Coins: 40}}},
		Store: coins.NewMemory(),
		Verifier: &appstore.Verifier{Roots: e.signer.Roots, Policy: appstore.Policy{
			BundleIDs: []string{bundle}, ProductIDs: []string{pack}, Environments: []string{appstore.EnvSandbox, appstore.EnvProduction},
		}},
		Now: func() time.Time { return e.now },
	}
	Register(e.mux, e.svc)
	Public(e.svc.Config)(e.mux)
	return e
}

// do calls the mux as uid (DevVerifier: "anon…" uids are anonymous).
func (e *env) do(t *testing.T, method, path, uid string, body any) (int, map[string]any) {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	r := httptest.NewRequest(method, path, rd)
	if uid != "" {
		r = r.WithContext(auth.ContextWithIdentity(r.Context(), auth.Identity{UID: uid, Anonymous: strings.HasPrefix(uid, "anon")}))
	}
	w := httptest.NewRecorder()
	e.mux.ServeHTTP(w, r)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func (e *env) jws(t *testing.T, txid string) string {
	return e.signer.Sign(t, appstoretest.Consumable(bundle, pack, txid, e.now))
}

func TestWalletAndInitialGrant(t *testing.T) {
	e := newEnv(t, coins.ModeEnforce)
	code, body := e.do(t, "GET", "/v1/me/coins", "anon1", nil)
	if code != 200 || body["mode"] != "enforce" || body["total"] != 0.0 {
		t.Fatalf("anonymous %d %v", code, body)
	}
	code, body = e.do(t, "GET", "/v1/me/coins", "u1", nil)
	if code != 200 || body["initial"] != 3.0 || body["available"] != 3.0 {
		t.Fatalf("signed in %d %v", code, body)
	}
}

func TestCreditApple(t *testing.T) {
	e := newEnv(t, coins.ModeEnforce)
	req := map[string]string{"signedTransaction": e.jws(t, "tx1")}
	code, body := e.do(t, "POST", "/v1/me/coins/apple", "u1", req)
	if code != 200 || body["credited"] != true || body["coins"] != 40.0 {
		t.Fatalf("credit %d %v", code, body)
	}
	if w := body["wallet"].(map[string]any); w["purchased"] != 40.0 || w["initial"] != 3.0 {
		t.Fatalf("wallet %v", w)
	}
	// the same transaction again: nothing more
	code, body = e.do(t, "POST", "/v1/me/coins/apple", "u1", req)
	if code != 200 || body["credited"] != false || body["wallet"].(map[string]any)["purchased"] != 40.0 {
		t.Fatalf("retry %d %v", code, body)
	}
	// replayed by another account
	if code, body = e.do(t, "POST", "/v1/me/coins/apple", "u2", req); code != 409 || errCode(body) != "transaction_owned_by_other" {
		t.Fatalf("other %d %v", code, body)
	}
	// anonymous accounts cannot buy
	if code, body = e.do(t, "POST", "/v1/me/coins/apple", "anon1", map[string]string{"signedTransaction": e.jws(t, "tx2")}); code != 403 || errCode(body) != "forbidden" {
		t.Fatalf("anonymous %d %v", code, body)
	}
	// Xcode-signed (local StoreKit testing) is rejected
	x := appstoretest.Consumable(bundle, pack, "tx3", e.now)
	x.Environment = "Xcode"
	code, body = e.do(t, "POST", "/v1/me/coins/apple", "u1", map[string]string{"signedTransaction": e.signer.Sign(t, x)})
	if code != 422 || errCode(body) != "invalid_transaction" {
		t.Fatalf("xcode %d %v", code, body)
	}
	if code, _ = e.do(t, "POST", "/v1/me/coins/apple", "u1", map[string]string{}); code != 400 {
		t.Fatalf("empty %d", code)
	}
	// history: purchase + initial grant, newest first
	code, body = e.do(t, "GET", "/v1/me/coins/ledger?limit=1", "u1", nil)
	if code != 200 || len(body["entries"].([]any)) != 1 || body["nextCursor"] == "" {
		t.Fatalf("ledger %d %v", code, body)
	}
	entry := body["entries"].([]any)[0].(map[string]any)
	if _, ok := entry["usd"]; ok {
		t.Fatalf("usd in the API: %v", entry)
	}
	code, body = e.do(t, "GET", "/v1/me/coins/ledger?cursor="+body["nextCursor"].(string), "u1", nil)
	if code != 200 || len(body["entries"].([]any)) != 1 || body["nextCursor"] != "" {
		t.Fatalf("page 2 %d %v", code, body)
	}
	if code, _ = e.do(t, "GET", "/v1/me/coins/ledger?limit=0", "u1", nil); code != 400 {
		t.Fatalf("limit 0 %d", code)
	}
	if code, _ = e.do(t, "GET", "/v1/me/coins/ledger?cursor=bad", "u1", nil); code != 400 {
		t.Fatalf("bad cursor %d", code)
	}
}

func TestModeOffAPI(t *testing.T) {
	e := newEnv(t, coins.ModeOff)
	code, body := e.do(t, "GET", "/v1/me/coins", "u1", nil)
	if code != 200 || body["mode"] != "off" || body["total"] != 0.0 {
		t.Fatalf("wallet %d %v", code, body)
	}
	if code, body = e.do(t, "POST", "/v1/me/coins/apple", "u1", map[string]string{"signedTransaction": e.jws(t, "tx1")}); code != 404 {
		t.Fatalf("credit %d %v", code, body)
	}
	if code, body = e.do(t, "GET", "/v1/me/coins/ledger", "u1", nil); code != 200 || len(body["entries"].([]any)) != 0 {
		t.Fatalf("ledger %d %v", code, body)
	}
}

func TestPublicConfig(t *testing.T) {
	e := newEnv(t, coins.ModeLog)
	code, body := e.do(t, "GET", "/v1/coins/config", "", nil)
	if code != 200 || body["mode"] != "log" || body["initialGrant"] != 3.0 {
		t.Fatalf("%d %v", code, body)
	}
	p := body["packs"].([]any)[0].(map[string]any)
	if p["productId"] != pack || p["coins"] != 40.0 {
		t.Fatalf("packs %v", body["packs"])
	}
}

func errCode(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	s, _ := e["code"].(string)
	return s
}
