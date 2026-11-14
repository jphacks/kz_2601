package appconfig

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/calls"
	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// withCallButton adds callButtons to the first screen of a DSL document.
func withCallButton(t *testing.T, raw []byte, buttons ...map[string]any) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	sc := doc["screens"].([]any)[0].(map[string]any)
	comps, _ := sc["components"].([]any)
	for _, b := range buttons {
		comps = append(comps, b)
	}
	sc["components"] = comps
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func callEnv(t *testing.T, cs *coins.Service, perMinute int64, perHour int) *env {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	cfg, err := calls.ConfigFromEnv(func(k string) string {
		return map[string]string{"CALLS_ENABLED": "1", "LIVEKIT_URL": "wss://example.livekit.cloud", "LIVEKIT_API_KEY": "APIkey",
			"LIVEKIT_API_SECRET": "fake-livekit-secret-for-tests-only-not-real", "COIN_PER_CALL_MINUTE": fmt.Sprint(perMinute), "CALL_BLOCK_MINUTES": "10"}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := &calls.Service{Config: cfg, Store: calls.NewMemory()}
	if cs != nil {
		svc.Coins = cs
	}
	return &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory(), WithCalls(svc), WithCallLimit(perHour, time.Now)))}
}

func TestCallTokenAuthz(t *testing.T) {
	e := callEnv(t, nil, 0, 100)
	const app = "/v1/apps/CALL-1"
	e.ownApp("owner", app, withCallButton(t, familyKakeibo(t),
		map[string]any{"type": "callButton", "label": "話す"},
		map[string]any{"type": "callButton", "label": "ビデオ", "video": true, "room": "video"}))
	tokenURL := app + "/calls/token"

	r := e.do("owner", http.MethodPost, tokenURL, `{"name":"オーナー"}`, nil)
	if r.code != http.StatusOK || r.body["room"] != "app:CALL-1" || r.body["token"] == "" || r.body["url"] != "wss://example.livekit.cloud" || r.body["video"] != false {
		t.Fatalf("owner: %d %s", r.code, r.raw)
	}
	// video only where the callButton allows it
	if r := e.do("owner", http.MethodPost, tokenURL, `{"video":true}`, nil); r.code != http.StatusOK || r.body["video"] != false {
		t.Fatalf("audio room asked for video: %d %s", r.code, r.raw)
	}
	if r := e.do("owner", http.MethodPost, tokenURL, `{"room":"video","video":true}`, nil); r.code != http.StatusOK || r.body["video"] != true || r.body["room"] != "app:CALL-1:video" {
		t.Fatalf("video room: %d %s", r.code, r.raw)
	}
	if r := e.do("owner", http.MethodPost, tokenURL, `{"room":"other"}`, nil); r.code != http.StatusNotFound || r.reason() != reasonNoCallButton {
		t.Fatalf("a room no callButton names: %d %s", r.code, r.raw)
	}
	if r := e.do("owner", http.MethodPost, tokenURL, `{"room":"Bad Room"}`, nil); r.code != http.StatusBadRequest {
		t.Fatalf("bad room: %d %s", r.code, r.raw)
	}

	// knowing the appId is not enough
	wantNotMember(t, e.do("stranger", http.MethodPost, tokenURL, `{}`, nil), "stranger token")
	tok := e.share("owner", app)
	if r := e.do("mom", http.MethodPost, "/v1/shares/"+tok+"/import", `{"displayName":"ママ"}`, nil); r.code != http.StatusCreated {
		t.Fatalf("mom joins: %d %s", r.code, r.raw)
	}
	r = e.do("mom", http.MethodPost, tokenURL, `{}`, nil)
	if r.code != http.StatusOK || r.body["room"] != "app:CALL-1" {
		t.Fatalf("member: %d %s", r.code, r.raw)
	}
	// renewal of mom's session; someone else cannot renew it
	sid := r.body["sessionId"].(string)
	if r := e.do("mom", http.MethodPost, tokenURL, `{"sessionId":"`+sid+`"}`, nil); r.code != http.StatusOK || r.body["sessionId"] != sid {
		t.Fatalf("renew: %d %s", r.code, r.raw)
	}
	if r := e.do("owner", http.MethodPost, tokenURL, `{"sessionId":"`+sid+`"}`, nil); r.code != http.StatusNotFound || r.reason() != reasonCallEnded {
		t.Fatalf("renew someone else's: %d %s", r.code, r.raw)
	}
}

func TestCallTokenNeedsCallButtonAndSharing(t *testing.T) {
	e := callEnv(t, nil, 0, 100)
	e.ownApp("owner", "/v1/apps/CALL-NOBTN", familyKakeibo(t))
	if r := e.do("owner", http.MethodPost, "/v1/apps/CALL-NOBTN/calls/token", `{}`, nil); r.code != http.StatusNotFound || r.reason() != reasonNoCallButton {
		t.Fatalf("no callButton: %d %s", r.code, r.raw)
	}
	e.ownApp("owner", "/v1/apps/CALL-PRIVATE", kakeibo(t))
	if r := e.do("owner", http.MethodPost, "/v1/apps/CALL-PRIVATE/calls/token", `{}`, nil); r.code != http.StatusNotFound || r.reason() != reasonNoSharedData {
		t.Fatalf("private app: %d %s", r.code, r.raw)
	}
}

func TestCallTokenOff(t *testing.T) {
	e := newEnv(t)
	e.ownApp("owner", "/v1/apps/CALL-OFF", withCallButton(t, familyKakeibo(t), map[string]any{"type": "callButton"}))
	r := e.do("owner", http.MethodPost, "/v1/apps/CALL-OFF/calls/token", `{}`, nil)
	if r.code != http.StatusNotFound || r.reason() != reasonCallsDisabled {
		t.Fatalf("off: %d %s", r.code, r.raw)
	}
}

func TestCallTokenRateLimit(t *testing.T) {
	e := callEnv(t, nil, 0, 2)
	const app = "/v1/apps/CALL-RL"
	e.ownApp("owner", app, withCallButton(t, familyKakeibo(t), map[string]any{"type": "callButton"}))
	for i := 0; i < 2; i++ {
		if r := e.do("owner", http.MethodPost, app+"/calls/token", `{}`, nil); r.code != http.StatusOK {
			t.Fatalf("call %d: %d %s", i, r.code, r.raw)
		}
	}
	r := e.do("owner", http.MethodPost, app+"/calls/token", `{}`, nil)
	if r.code != http.StatusTooManyRequests || r.errCode() != "rate_limited" || r.hdr.Get("Retry-After") == "" {
		t.Fatalf("over the cap: %d %s", r.code, r.raw)
	}
}

func TestCallTokenCoins(t *testing.T) {
	cs := &coins.Service{Config: coins.Config{Mode: coins.ModeEnforce, InitialGrant: 15, HoldTTL: time.Minute, Reserve: map[string]int64{}}, Store: coins.NewMemory()}
	e := callEnv(t, cs, 1, 100) // 10 coins per 10-minute block
	const app = "/v1/apps/CALL-COINS"
	e.ownApp("owner", app, withCallButton(t, familyKakeibo(t), map[string]any{"type": "callButton"}))
	r := e.do("owner", http.MethodPost, app+"/calls/token", `{}`, nil)
	if r.code != http.StatusOK || r.body["coinsReserved"] != 10.0 {
		t.Fatalf("first block: %d %s", r.code, r.raw)
	}
	// 5 left: the renewal is 402 from the caller's own wallet
	r = e.do("owner", http.MethodPost, app+"/calls/token", `{"sessionId":"`+r.body["sessionId"].(string)+`"}`, nil)
	if r.code != http.StatusPaymentRequired || r.errCode() != "coins_insufficient" {
		t.Fatalf("second block: %d %s", r.code, r.raw)
	}
	d := r.body["error"].(map[string]any)["details"].(map[string]any)
	if d["payer"] != "self" || d["required"] != 10.0 || d["balance"] != 5.0 {
		t.Errorf("details = %v", d)
	}
}
