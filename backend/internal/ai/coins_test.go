package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/paid"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// costCaller answers like fakeCaller and adds usd per call to the request's
// cost tally, as Client.logUsage does for the real API (never called here).
// With release set, each call first streams its reply (OnText) and then
// waits for release to close.
type costCaller struct {
	mu      sync.Mutex
	replies []string // in order; the last one repeats
	usd     float64
	priced  bool
	fail    error
	calls   int
	release chan struct{}
}

func (c *costCaller) Complete(ctx context.Context, req Completion) (*Response, error) {
	c.mu.Lock()
	reply := c.replies[min(c.calls, len(c.replies)-1)]
	c.calls++
	fail, release := c.fail, c.release
	c.mu.Unlock()
	if release != nil {
		if req.OnText != nil && fail == nil {
			req.OnText(reply)
		}
		<-release
	}
	u := Usage{InputTokens: 10, OutputTokens: 5}
	if t := tallyOf(ctx); t != nil {
		t.add(u, c.usd, c.priced)
	}
	if fail != nil {
		return nil, fail
	}
	return &Response{Text: reply, StopReason: "end_turn", Usage: u}, nil
}

func (c *costCaller) n() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// lockedBuf is a log sink safe for concurrent requests.
type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// Holds of the tests (not real values; prices are not in the repo).
var testCoinReserve = map[string]int64{
	coins.OpGenerate: 50, coins.OpEdit: 30, coins.OpChat: 10, coins.OpText: 5, coins.OpText + "_long": 8,
}

// coinWallets is a coin service where every signed-in account starts with
// balance coins; 1 USD = 100 coins.
func coinWallets(mode coins.Mode, balance int64) (*coins.Service, *lockedBuf) {
	logs := &lockedBuf{}
	return &coins.Service{
		Config: coins.Config{Mode: mode, PerUSD: 100, MinPerCall: 1, HoldTTL: 10 * time.Minute,
			InitialGrant: balance, Reserve: testCoinReserve},
		Store: coins.NewMemory(),
		Log:   slog.New(slog.NewJSONHandler(logs, nil)),
	}, logs
}

func available(t *testing.T, cs *coins.Service, uid string) int64 {
	t.Helper()
	v, err := cs.Wallet(context.Background(), uid, false)
	if err != nil {
		t.Fatal(err)
	}
	return v.Available
}

func reserved(t *testing.T, cs *coins.Service, uid string) int64 {
	t.Helper()
	v, err := cs.Wallet(context.Background(), uid, false)
	if err != nil {
		t.Fatal(err)
	}
	return v.Reserved
}

// coinHandler is NewHandler behind the request id middleware (as in cmd/ai).
func coinHandler(svc *Service) http.Handler { return httpx.WithRequestID(NewHandler(svc)) }

func coinPost(t *testing.T, h http.Handler, path, uid, reqID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if uid != "" {
		bearer(req, uid)
	}
	if reqID != "" {
		req.Header.Set(httpx.RequestIDHeader, reqID)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

type coinMeta struct {
	Meta struct {
		Coins        *int64 `json:"coins"`
		CoinsBalance *int64 `json:"coinsBalance"`
	} `json:"meta"`
}

func metaOf(t *testing.T, body []byte) coinMeta {
	t.Helper()
	var m coinMeta
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("body %s: %v", body, err)
	}
	return m
}

func errDetails(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var b struct {
		Error struct {
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &b); err != nil {
		t.Fatalf("not an error body: %s", body)
	}
	return b.Error.Details
}

func generateSvc(t *testing.T, cc *costCaller) *Service {
	t.Helper()
	svc, _ := newSvc(&fakeCaller{})
	svc.Claude, svc.Verifier = cc, auth.DevVerifier{}
	return svc
}

const createBody = `{"appId":"a1","transcript":"家計簿アプリを作って"}`

// Each mode of /v1/generate: off touches no wallet, log only logs what it
// would charge, enforce charges ceil(usd × COIN_PER_USD).
func TestCoinsGenerateModes(t *testing.T) {
	for _, mode := range []coins.Mode{coins.ModeOff, coins.ModeLog, coins.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			cc := &costCaller{replies: []string{string(kakeibo(t))}, usd: 0.25, priced: true}
			svc := generateSvc(t, cc)
			cs, logs := coinWallets(mode, 100)
			svc.Coins = cs
			rec := coinPost(t, coinHandler(svc), "/v1/generate", "u1", "", createBody)
			if rec.Code != http.StatusOK {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
			m := metaOf(t, rec.Body.Bytes())
			bal := available(t, cs, "u1")
			switch mode {
			case coins.ModeEnforce:
				if m.Meta.Coins == nil || *m.Meta.Coins != 25 || *m.Meta.CoinsBalance != 75 || bal != 75 {
					t.Fatalf("meta %+v balance %d", m.Meta, bal)
				}
				if rec.Header().Get(coins.HeaderCharged) != "25" || rec.Header().Get(coins.HeaderBalance) != "75" {
					t.Fatalf("headers %v", rec.Header())
				}
			case coins.ModeLog:
				if m.Meta.Coins != nil || bal != 100 || !strings.Contains(logs.String(), coins.EventWouldSpend) {
					t.Fatalf("meta %+v balance %d logs %s", m.Meta, bal, logs)
				}
			case coins.ModeOff:
				if m.Meta.Coins != nil || logs.String() != "" || rec.Header().Get(coins.HeaderCharged) != "" {
					t.Fatalf("off touched coins: %+v %s", m.Meta, logs)
				}
			}
			if reserved(t, cs, "u1") != 0 {
				t.Fatal("a hold was left")
			}
		})
	}
}

// Short of the hold: 402 before any Claude call (also before a stream
// starts), and log mode only logs it.
func TestCoinsGenerateInsufficient(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint("stream=", stream), func(t *testing.T) {
			cc := &costCaller{replies: []string{string(kakeibo(t))}, usd: 0.25, priced: true}
			svc := generateSvc(t, cc)
			cs, _ := coinWallets(coins.ModeEnforce, 10)
			svc.Coins = cs
			path := "/v1/generate"
			if stream {
				path += "?stream=1"
			}
			rec := coinPost(t, coinHandler(svc), path, "u1", "", createBody)
			if rec.Code != http.StatusPaymentRequired || errCode(t, rec) != httpx.CodeCoinsInsufficient {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("content type %q: the 402 must come before the stream", ct)
			}
			d := errDetails(t, rec.Body.Bytes())
			if d["required"] != 50.0 || d["balance"] != 10.0 || d["payer"] != "self" {
				t.Fatalf("details %v", d)
			}
			if cc.n() != 0 || available(t, cs, "u1") != 10 {
				t.Fatalf("calls %d balance %d", cc.n(), available(t, cs, "u1"))
			}
		})
	}
	t.Run("log", func(t *testing.T) {
		cc := &costCaller{replies: []string{string(kakeibo(t))}, usd: 0.25, priced: true}
		svc := generateSvc(t, cc)
		cs, logs := coinWallets(coins.ModeLog, 10)
		svc.Coins = cs
		rec := coinPost(t, coinHandler(svc), "/v1/generate", "u1", "", createBody)
		if rec.Code != http.StatusOK || !strings.Contains(logs.String(), coins.EventWouldBlock) || available(t, cs, "u1") != 10 {
			t.Fatalf("%d %s logs %s", rec.Code, rec.Body.String(), logs)
		}
	})
}

// A failed generation costs nothing: the hold comes back (JSON and stream).
func TestCoinsGenerateFailureReleases(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint("stream=", stream), func(t *testing.T) {
			cc := &costCaller{replies: []string{"x"}, usd: 0.25, priced: true, fail: &APIError{Status: 529, Type: "overloaded_error"}}
			svc := generateSvc(t, cc)
			cs, _ := coinWallets(coins.ModeEnforce, 100)
			svc.Coins = cs
			path := "/v1/generate"
			if stream {
				path += "?stream=1"
			}
			rec := coinPost(t, coinHandler(svc), path, "u1", "", createBody)
			if stream {
				if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "event: error") {
					t.Fatalf("%d %s", rec.Code, rec.Body.String())
				}
			} else if rec.Code < 500 {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
			if cc.n() == 0 || available(t, cs, "u1") != 100 || reserved(t, cs, "u1") != 0 {
				t.Fatalf("calls %d balance %d reserved %d", cc.n(), available(t, cs, "u1"), reserved(t, cs, "u1"))
			}
			ents, _, _ := cs.Ledger(context.Background(), "u1", 20, "")
			for _, e := range ents {
				if e.Kind == coins.KindSpendAI {
					t.Fatalf("charged a failed generation: %+v", e)
				}
			}
		})
	}
}

// A stream reserves before it starts and the result event carries the charge.
func TestCoinsGenerateStreamCharges(t *testing.T) {
	cc := &costCaller{replies: []string{string(kakeibo(t))}, usd: 0.25, priced: true}
	svc := generateSvc(t, cc)
	cs, _ := coinWallets(coins.ModeEnforce, 100)
	svc.Coins = cs
	rec := coinPost(t, coinHandler(svc), "/v1/generate?stream=1", "u1", "", createBody)
	i := strings.Index(rec.Body.String(), "event: result\ndata: ")
	if rec.Code != http.StatusOK || i < 0 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	data := strings.SplitN(rec.Body.String()[i+len("event: result\ndata: "):], "\n", 2)[0]
	if m := metaOf(t, []byte(data)); m.Meta.Coins == nil || *m.Meta.Coins != 25 || available(t, cs, "u1") != 75 {
		t.Fatalf("meta %+v balance %d", m.Meta, available(t, cs, "u1"))
	}
}

// Undo calls no Claude and holds nothing; edit holds the edit amount.
func TestCoinsGenerateOps(t *testing.T) {
	cases := map[Mode]string{"": coins.OpGenerate, ModeAuto: coins.OpGenerate, ModeCreate: coins.OpGenerate, ModeEdit: coins.OpEdit, ModeUndo: ""}
	for m, want := range cases {
		if got := generateCoinOp(m); got != want {
			t.Errorf("%q: %q, want %q", m, got, want)
		}
	}
	cc := &costCaller{replies: []string{"x"}, usd: 0.25, priced: true}
	svc := generateSvc(t, cc)
	cs, _ := coinWallets(coins.ModeEnforce, 0)
	svc.Coins = cs
	h := coinHandler(svc)
	if rec := coinPost(t, h, "/v1/generate", "u1", "", `{"appId":"a1","transcript":"元に戻して","mode":"undo"}`); rec.Code == http.StatusPaymentRequired {
		t.Fatalf("undo needs no coins: %s", rec.Body.String())
	}
	rec := coinPost(t, h, "/v1/generate", "u1", "", `{"appId":"a1","transcript":"色を変えて","mode":"edit"}`)
	if rec.Code != http.StatusPaymentRequired || errDetails(t, rec.Body.Bytes())["required"] != 30.0 {
		t.Fatalf("edit: %d %s", rec.Code, rec.Body.String())
	}
}

// mode=auto that resolves to undo (no Claude call) gives the hold back.
func TestCoinsGenerateAutoUndoFree(t *testing.T) {
	cc := &costCaller{replies: []string{string(kakeibo(t))}, usd: 0.25, priced: true}
	svc := generateSvc(t, cc)
	cs, _ := coinWallets(coins.ModeEnforce, 100)
	svc.Coins = cs
	h := coinHandler(svc)
	if rec := coinPost(t, h, "/v1/generate", "u1", "", createBody); rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	rec := coinPost(t, h, "/v1/generate", "u1", "", `{"appId":"a1","transcript":"元に戻して"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"source":"restore"`) {
		t.Fatalf("undo: %d %s", rec.Code, rec.Body.String())
	}
	if m := metaOf(t, rec.Body.Bytes()); m.Meta.Coins != nil || available(t, cs, "u1") != 75 || reserved(t, cs, "u1") != 0 {
		t.Fatalf("undo charged: %+v balance %d", m.Meta, available(t, cs, "u1"))
	}
}

// Without a caller there is no payer: 401 in enforce, nothing in log.
func TestCoinsNeedPayer(t *testing.T) {
	for mode, want := range map[coins.Mode]int{coins.ModeEnforce: http.StatusUnauthorized, coins.ModeLog: http.StatusOK} {
		cc := &costCaller{replies: []string{string(kakeibo(t))}, usd: 0.25, priced: true}
		svc := generateSvc(t, cc)
		svc.Coins, _ = coinWallets(mode, 100)
		if rec := coinPost(t, coinHandler(svc), "/v1/generate", "", "", createBody); rec.Code != want {
			t.Fatalf("%s: %d %s", mode, rec.Code, rec.Body.String())
		}
	}
}

// A model without a price charges the hold (coins.unpriced_model); a cost
// above the hold is capped at it.
func TestCoinsUnpricedAndCap(t *testing.T) {
	for name, cc := range map[string]*costCaller{
		"unpriced": {replies: []string{`{"text":"ok"}`}, usd: 0, priced: false},
		"over":     {replies: []string{`{"text":"ok"}`}, usd: 1, priced: true},
	} {
		t.Run(name, func(t *testing.T) {
			svc, _ := textSvc()
			svc.Claude = cc
			cs, _ := coinWallets(coins.ModeEnforce, 100)
			svc.Coins = cs
			rec := coinPost(t, coinHandler(svc), "/v1/text", "u1", "", `{"instruction":"励ましの一言"}`)
			if m := metaOf(t, rec.Body.Bytes()); rec.Code != http.StatusOK || m.Meta.Coins == nil || *m.Meta.Coins != 5 {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// Chat turns (clarify and chat_first) pay per turn; a failed turn is free.
func TestCoinsChat(t *testing.T) {
	bodies := map[string]string{
		"turn":       `{"appId":"a1","messages":[{"role":"user","text":"家計簿"}]}`,
		"chat_first": `{"appId":"a1","mode":"create","messages":[{"role":"user","text":"家計簿"}]}`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			cc := &costCaller{replies: []string{questionReply}, usd: 0.0625, priced: true}
			svc := generateSvc(t, cc)
			cs, _ := coinWallets(coins.ModeEnforce, 100)
			svc.Coins = cs
			h := coinHandler(svc)
			rec := coinPost(t, h, "/v1/chat", "u1", "", body)
			if m := metaOf(t, rec.Body.Bytes()); rec.Code != http.StatusOK || m.Meta.Coins == nil || *m.Meta.Coins != 7 || available(t, cs, "u1") != 93 {
				t.Fatalf("%d %s balance %d", rec.Code, rec.Body.String(), available(t, cs, "u1"))
			}
			cc.fail = &APIError{Status: 529, Type: "overloaded_error"}
			if rec := coinPost(t, h, "/v1/chat", "u1", "", body); rec.Code < 500 || available(t, cs, "u1") != 93 || reserved(t, cs, "u1") != 0 {
				t.Fatalf("failed turn: %d balance %d", rec.Code, available(t, cs, "u1"))
			}
		})
	}
	t.Run("insufficient", func(t *testing.T) {
		cc := &costCaller{replies: []string{questionReply}, usd: 0.0625, priced: true}
		svc := generateSvc(t, cc)
		svc.Coins, _ = coinWallets(coins.ModeEnforce, 3)
		rec := coinPost(t, coinHandler(svc), "/v1/chat", "u1", "", bodies["turn"])
		if rec.Code != http.StatusPaymentRequired || errCode(t, rec) != httpx.CodeCoinsInsufficient || cc.n() != 0 {
			t.Fatalf("%d %s calls %d", rec.Code, rec.Body.String(), cc.n())
		}
	})
}

// An early question answers before its Claude call ends: the charge waits
// for the call (no coins in meta) and lands afterwards.
func TestCoinsChatEarlyQuestionChargedLater(t *testing.T) {
	cc := &costCaller{replies: []string{questionReply}, usd: 0.0625, priced: true, release: make(chan struct{})}
	svc := generateSvc(t, cc)
	cs, _ := coinWallets(coins.ModeEnforce, 100)
	svc.Coins = cs
	rec := coinPost(t, coinHandler(svc), "/v1/chat", "u1", "", `{"appId":"a1","messages":[{"role":"user","text":"家計簿"}]}`)
	if m := metaOf(t, rec.Body.Bytes()); rec.Code != http.StatusOK || m.Meta.Coins != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if reserved(t, cs, "u1") != 10 {
		t.Fatalf("the hold should stay while the call runs: reserved %d", reserved(t, cs, "u1"))
	}
	close(cc.release)
	eventually(t, func() bool { return available(t, cs, "u1") == 93 && reserved(t, cs, "u1") == 0 })
}

func eventually(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// /v1/text: the caller pays; long / aiChat / image / task hold their own
// amount when set (COIN_RESERVE_TEXT_<KIND>, else TEXT's).
func TestCoinsText(t *testing.T) {
	cc := &costCaller{replies: []string{`{"text":"がんばったね"}`}, usd: 0.03125, priced: true}
	svc, _ := textSvc()
	svc.Claude = cc
	cs, _ := coinWallets(coins.ModeEnforce, 6)
	svc.Coins = cs
	h := coinHandler(svc)
	rec := coinPost(t, h, "/v1/text", "u1", "", `{"instruction":"励ましの一言","length":"long"}`)
	if rec.Code != http.StatusPaymentRequired || errDetails(t, rec.Body.Bytes())["required"] != 8.0 || cc.n() != 0 {
		t.Fatalf("long: %d %s", rec.Code, rec.Body.String())
	}
	rec = coinPost(t, h, "/v1/text", "u1", "", `{"instruction":"励ましの一言"}`)
	if m := metaOf(t, rec.Body.Bytes()); rec.Code != http.StatusOK || *m.Meta.Coins != 4 || available(t, cs, "u1") != 2 {
		t.Fatalf("%d %s balance %d", rec.Code, rec.Body.String(), available(t, cs, "u1"))
	}
	cases := map[string]string{
		"":      `{"instruction":"x"}`,
		"long":  `{"instruction":"x","length":"long"}`,
		"task":  `{"task":"summarize","input":"x"}`,
		"image": `{"instruction":"x","image":{"mediaType":"image/jpeg","data":"AA=="}}`,
	}
	for want, body := range cases {
		var req TextRequest
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Fatal(err)
		}
		if got := textCoinKind(req); got != want {
			t.Errorf("%s: kind %q, want %q", body, got, want)
		}
	}
	if textCoinKind(TextRequest{Task: "chat"}) != "long" {
		t.Error("aiChat holds like long")
	}
}

// A failed /v1/text call is free.
func TestCoinsTextFailureReleases(t *testing.T) {
	cc := &costCaller{replies: []string{"not json"}, usd: 0.03125, priced: true}
	svc, _ := textSvc()
	svc.Claude = cc
	cs, _ := coinWallets(coins.ModeEnforce, 100)
	svc.Coins = cs
	rec := coinPost(t, coinHandler(svc), "/v1/text", "u1", "", `{"instruction":"励ましの一言"}`)
	if rec.Code != http.StatusBadGateway || available(t, cs, "u1") != 100 || reserved(t, cs, "u1") != 0 {
		t.Fatalf("%d %s balance %d", rec.Code, rec.Body.String(), available(t, cs, "u1"))
	}
}

// A participant's in-app AI of a shared app is charged to the creator; a
// creator short of coins is a 402 with payer creator and no Claude call,
// and the creator-paid count comes back.
func TestCoinsTextCreatorPays(t *testing.T) {
	m := store.NewMemory()
	proApp, _ := sharedAppFixture(t, m, "")
	cc := &costCaller{replies: []string{`{"text":"ok"}`}, usd: 0.03125, priced: true}
	svc, _ := textSvc()
	svc.Claude = cc
	svc.Pro, svc.ProMode, svc.Members = proSet{"pro-owner": true}, paid.ModeEnforce, m
	svc.CreatorPaid = &CreatorPaidCap{Daily: 1, Env: "test"}
	cs, _ := coinWallets(coins.ModeEnforce, 20)
	svc.Coins = cs
	h := coinHandler(svc)
	body := `{"instruction":"励ましの一言","appId":"` + proApp + `"}`

	// the creator cannot pay: 402 payer creator, the app's daily count is not used up
	cs.Config.Reserve = map[string]int64{coins.OpText: 50}
	rec := coinPost(t, h, "/v1/text", "member", "", body)
	if rec.Code != http.StatusPaymentRequired || errCode(t, rec) != httpx.CodeCoinsInsufficient || errDetails(t, rec.Body.Bytes())["payer"] != "creator" || cc.n() != 0 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	cs.Config.Reserve = testCoinReserve
	rec = coinPost(t, h, "/v1/text", "member", "", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("after the 402 the daily count must be back: %d %s", rec.Code, rec.Body.String())
	}
	if available(t, cs, "pro-owner") != 16 || available(t, cs, "member") != 20 {
		t.Fatalf("creator %d member %d", available(t, cs, "pro-owner"), available(t, cs, "member"))
	}
}

// The same X-Request-Id holds once: a retry is refused (409) without a
// second Claude call or charge.
func TestCoinsIdempotentRequestID(t *testing.T) {
	cc := &costCaller{replies: []string{`{"text":"ok"}`}, usd: 0.03125, priced: true}
	svc, _ := textSvc()
	svc.Claude = cc
	cs, _ := coinWallets(coins.ModeEnforce, 100)
	svc.Coins = cs
	h := coinHandler(svc)
	if rec := coinPost(t, h, "/v1/text", "u1", "req-1", `{"instruction":"励ましの一言"}`); rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	rec := coinPost(t, h, "/v1/text", "u1", "req-1", `{"instruction":"励ましの一言"}`)
	if rec.Code != http.StatusConflict || errCode(t, rec) != httpx.CodeDuplicateRequest || cc.n() != 1 || available(t, cs, "u1") != 96 {
		t.Fatalf("retry: %d %s calls %d balance %d", rec.Code, rec.Body.String(), cc.n(), available(t, cs, "u1"))
	}
	// another user's same id is a different hold
	if rec := coinPost(t, h, "/v1/text", "u2", "req-1", `{"instruction":"励ましの一言"}`); rec.Code != http.StatusOK {
		t.Fatalf("u2: %d %s", rec.Code, rec.Body.String())
	}
}

// Calls at the same time never hold more than the balance.
func TestCoinsConcurrentHolds(t *testing.T) {
	cc := &costCaller{replies: []string{`{"text":"ok"}`}, usd: 0.25, priced: true, release: make(chan struct{})}
	svc, _ := textSvc()
	svc.Claude = cc
	cs, _ := coinWallets(coins.ModeEnforce, 100)
	cs.Config.Reserve = map[string]int64{coins.OpText: 30}
	svc.Coins = cs
	h := coinHandler(svc)
	var ok, short atomic.Int32
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			rec := coinPost(t, h, "/v1/text", "u1", "", `{"instruction":"励ましの一言"}`)
			switch rec.Code {
			case http.StatusOK:
				ok.Add(1)
			case http.StatusPaymentRequired:
				short.Add(1)
			default:
				t.Errorf("%d %s", rec.Code, rec.Body.String())
			}
		})
	}
	eventually(t, func() bool { return short.Load() == 7 })
	close(cc.release)
	wg.Wait()
	if ok.Load() != 3 || available(t, cs, "u1") != 25 || reserved(t, cs, "u1") != 0 {
		t.Fatalf("ok %d short %d balance %d", ok.Load(), short.Load(), available(t, cs, "u1"))
	}
}

// A wallet that cannot be read stops enforce (500) and never log mode.
func TestCoinsWalletDown(t *testing.T) {
	for mode, want := range map[coins.Mode]int{coins.ModeEnforce: http.StatusInternalServerError, coins.ModeLog: http.StatusOK} {
		cc := &costCaller{replies: []string{`{"text":"ok"}`}, usd: 0.03125, priced: true}
		svc, _ := textSvc()
		svc.Claude = cc
		svc.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		cs, _ := coinWallets(mode, 100)
		cs.Store = downStore{}
		svc.Coins = cs
		if rec := coinPost(t, coinHandler(svc), "/v1/text", "u1", "", `{"instruction":"励ましの一言"}`); rec.Code != want {
			t.Fatalf("%s: %d %s", mode, rec.Code, rec.Body.String())
		}
	}
}

type downStore struct{ coins.Store }

func (downStore) Update(context.Context, coins.Keys, func(*coins.Tx) error) error {
	return fmt.Errorf("firestore unavailable")
}
