package calls

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/livekit/protocol/auth"

	"github.com/jphacks/kz_2601/backend/internal/coins"
)

const (
	testKey    = "APItestkey"
	testSecret = "test-secret-that-is-long-enough-for-hs256"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func onEnv(extra map[string]string) map[string]string {
	m := map[string]string{"CALLS_ENABLED": "true", "LIVEKIT_URL": "wss://example.livekit.cloud", "LIVEKIT_API_KEY": testKey, "LIVEKIT_API_SECRET": testSecret}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestConfigFromEnv(t *testing.T) {
	c, err := ConfigFromEnv(env(nil))
	if err != nil || c.Enabled {
		t.Fatalf("default must be off without error: %+v %v", c, err)
	}
	if _, err := ConfigFromEnv(env(map[string]string{"CALLS_ENABLED": "true"})); err == nil {
		t.Error("on without LiveKit keys must be an error")
	}
	if _, err := ConfigFromEnv(env(onEnv(map[string]string{"LIVEKIT_URL": "http://x"}))); err == nil {
		t.Error("a non-wss URL must be an error")
	}
	if _, err := ConfigFromEnv(env(onEnv(map[string]string{"COIN_PER_CALL_MINUTE": "-1"}))); err == nil {
		t.Error("a negative price must be an error")
	}
	c, err = ConfigFromEnv(env(onEnv(map[string]string{"COIN_PER_CALL_MINUTE": "3", "CALL_BLOCK_MINUTES": "20"})))
	if err != nil || !c.Enabled || c.PerMinute != 3 || c.BlockCoins() != 60 || c.TokenTTL != DefaultTokenTTL {
		t.Fatalf("got %+v %v", c, err)
	}
	if c, _ := ConfigFromEnv(env(onEnv(nil))); c.BlockCoins() != 0 {
		t.Error("the price defaults to 0 (not counted)")
	}
}

type fixture struct {
	svc   *Service
	coins *coins.Service
	now   time.Time
}

func newFixture(t *testing.T, perMinute int64, mode coins.Mode, grant int64) *fixture {
	t.Helper()
	f := &fixture{now: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	f.coins = &coins.Service{Config: coins.Config{Mode: mode, InitialGrant: grant, HoldTTL: time.Minute, Reserve: map[string]int64{}},
		Store: coins.NewMemory(), Now: func() time.Time { return f.now },
		Subjects: func(_ context.Context, uid string) []string { return []string{"test:" + uid} }}
	cfg, err := ConfigFromEnv(env(onEnv(map[string]string{"COIN_PER_CALL_MINUTE": fmt.Sprint(perMinute), "CALL_BLOCK_MINUTES": "10"})))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	f.svc = &Service{Config: cfg, Store: NewMemory(), Coins: f.coins, Now: func() time.Time { return f.now },
		NewID: func() string { n++; return fmt.Sprintf("s%d", n) }}
	return f
}

func (f *fixture) balance(t *testing.T, uid string) int64 {
	t.Helper()
	v, err := f.coins.Wallet(context.Background(), uid, false)
	if err != nil {
		t.Fatal(err)
	}
	return v.Available
}

func TestJoinIssuesToken(t *testing.T) {
	f := newFixture(t, 0, coins.ModeOff, 0)
	j, err := f.svc.Join(context.Background(), JoinRequest{UID: "u1", AppID: "a1", Room: "lobby", Video: true, Name: "  ゆう\x07\n "})
	if err != nil {
		t.Fatal(err)
	}
	if j.Room != "app:a1:lobby" || j.Identity != "s1" || j.URL != "wss://example.livekit.cloud" || j.CoinsReserved != 0 {
		t.Fatalf("join = %+v", j)
	}
	v, err := auth.ParseAPIToken(j.Token)
	if err != nil {
		t.Fatal(err)
	}
	_, claims, err := v.Verify(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	g := claims.Video
	if v.Identity() != "s1" || claims.Name != "ゆう" || !g.RoomJoin || g.Room != "app:a1:lobby" || !*g.CanSubscribe || *g.CanPublishData {
		t.Fatalf("claims = %+v / %+v", claims, g)
	}
	if strings.Join(g.CanPublishSources, ",") != "microphone,camera" {
		t.Errorf("sources = %v", g.CanPublishSources)
	}
	if claims.RoomConfig == nil || claims.RoomConfig.MaxParticipants != DefaultMaxParticipants {
		t.Errorf("room config = %+v", claims.RoomConfig)
	}

	audio, _ := f.svc.Join(context.Background(), JoinRequest{UID: "u1", AppID: "a1"})
	v, _ = auth.ParseAPIToken(audio.Token)
	_, claims, _ = v.Verify(testSecret)
	if audio.Room != "app:a1" || strings.Join(claims.Video.CanPublishSources, ",") != "microphone" {
		t.Errorf("audio only: room %q sources %v", audio.Room, claims.Video.CanPublishSources)
	}
}

func TestJoinOff(t *testing.T) {
	s := &Service{Store: NewMemory()}
	if _, err := s.Join(context.Background(), JoinRequest{UID: "u", AppID: "a"}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("err = %v", err)
	}
	mux := http.NewServeMux()
	Public(s)(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, WebhookPath, nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("webhook while off = %d, want 404", rec.Code)
	}
}

func TestJoinReservesCoins(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, 2, coins.ModeEnforce, 50)
	j, err := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if j.CoinsReserved != 20 || f.balance(t, "u1") != 30 {
		t.Fatalf("reserved %d, balance %d", j.CoinsReserved, f.balance(t, "u1"))
	}
	// renewal: a second block on the same session
	r, err := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1", SessionID: j.SessionID})
	if err != nil || r.SessionID != j.SessionID || f.balance(t, "u1") != 10 {
		t.Fatalf("renew = %+v %v, balance %d", r, err, f.balance(t, "u1"))
	}
	// the third block does not fit: 402 and nothing taken
	_, err = f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1", SessionID: j.SessionID})
	var ins *coins.InsufficientError
	if !errors.As(err, &ins) || ins.Required != 20 || ins.Balance != 10 {
		t.Fatalf("err = %v", err)
	}
	// someone else cannot renew it, nor another room
	if _, err := f.svc.Join(ctx, JoinRequest{UID: "u2", AppID: "a1", SessionID: j.SessionID}); !errors.Is(err, ErrSessionClosed) {
		t.Errorf("other uid: %v", err)
	}
	if _, err := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a2", SessionID: j.SessionID}); !errors.Is(err, ErrSessionClosed) {
		t.Errorf("other app: %v", err)
	}
	if _, err := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1", SessionID: "nope"}); !errors.Is(err, ErrSessionClosed) {
		t.Errorf("unknown session: %v", err)
	}
}

// A long call renews block after block; the first blocks are settled only
// at the leave, so their holds must not expire in between.
func TestSettleChargesEarlierBlocksOfALongCall(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, 2, coins.ModeEnforce, 200)
	j, err := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Handle(ctx, Event{Kind: KindJoined, Identity: j.Identity, JoinedAt: f.now}); err != nil {
		t.Fatal(err)
	}
	start := f.now
	for _, at := range []time.Duration{9 * time.Minute, 19 * time.Minute, 29 * time.Minute} { // a minute before each block ends
		f.now = start.Add(at)
		if _, err := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1", SessionID: j.SessionID}); err != nil {
			t.Fatalf("renew at %v: %v", at, err)
		}
	}
	f.now = start.Add(35 * time.Minute)
	if err := f.svc.Handle(ctx, Event{Kind: KindLeft, Identity: j.Identity, At: f.now}); err != nil {
		t.Fatal(err)
	}
	if got := f.balance(t, "u1"); got != 200-70 {
		t.Fatalf("balance = %d, want %d (35 min × 2 over 4 blocks)", got, 200-70)
	}
}

func TestSettleChargesMinutesOnce(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, 2, coins.ModeEnforce, 50)
	j, _ := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1"})
	_, _ = f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1", SessionID: j.SessionID})
	joined := f.now.Add(30 * time.Second)
	if err := f.svc.Handle(ctx, Event{Kind: KindJoined, Identity: j.Identity, JoinedAt: joined}); err != nil {
		t.Fatal(err)
	}
	left := joined.Add(12*time.Minute + 5*time.Second) // 13 minutes (a started minute counts)
	for i := 0; i < 3; i++ {                           // LiveKit may send it again
		if err := f.svc.Handle(ctx, Event{Kind: KindLeft, Identity: j.Identity, At: left}); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.balance(t, "u1"); got != 50-26 {
		t.Fatalf("balance = %d, want %d (13 min × 2)", got, 50-26)
	}
	s, _ := f.svc.Store.Get(ctx, j.SessionID)
	if !s.Settled || s.Minutes != 13 || len(s.Blocks) != 2 {
		t.Fatalf("session = %+v", s)
	}
	if s.Charges[s.Blocks[0].HoldID] != 20 || s.Charges[s.Blocks[1].HoldID] != 6 {
		t.Errorf("charges = %v", s.Charges)
	}
	// a settled session cannot be renewed
	if _, err := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1", SessionID: j.SessionID}); !errors.Is(err, ErrSessionClosed) {
		t.Errorf("renew after leave: %v", err)
	}
	es, _, _ := f.coins.Ledger(ctx, "u1", 10, "")
	spent := 0
	for _, e := range es {
		if e.Kind == coins.KindSpendCall {
			spent++
		}
	}
	if spent != 2 {
		t.Errorf("spend_call rows = %d, want 2: %+v", spent, es)
	}
}

func TestSettleCapsAtTheBlocks(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, 2, coins.ModeEnforce, 50)
	j, _ := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1"})
	// joined time from the leave event itself; 45 minutes but one 10-minute block
	if err := f.svc.Handle(ctx, Event{Kind: KindLeft, Identity: j.Identity, JoinedAt: f.now, At: f.now.Add(45 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if got := f.balance(t, "u1"); got != 30 {
		t.Fatalf("balance = %d, want 30 (never more than the hold)", got)
	}
}

func TestRoomFinishedSettlesOpenSessions(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, 1, coins.ModeEnforce, 50)
	a, _ := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1"})
	b, _ := f.svc.Join(ctx, JoinRequest{UID: "u2", AppID: "a1"})
	_ = f.svc.Handle(ctx, Event{Kind: KindJoined, Identity: a.Identity, JoinedAt: f.now})
	if err := f.svc.Handle(ctx, Event{Kind: KindFinished, Room: "app:a1", At: f.now.Add(3 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if got := f.balance(t, "u1"); got != 47 {
		t.Errorf("u1 = %d, want 47", got)
	}
	// u2 never joined: nothing charged, the hold went back
	if got := f.balance(t, "u2"); got != 50 {
		t.Errorf("u2 = %d, want 50", got)
	}
	if s, _ := f.svc.Store.Get(ctx, b.SessionID); !s.Settled {
		t.Error("u2's session must be closed")
	}
}

func TestUnsettledHoldExpires(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, 2, coins.ModeEnforce, 50)
	if _, err := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1"}); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(10*time.Minute + DefaultHoldGrace - time.Second)
	if got := f.balance(t, "u1"); got != 30 {
		t.Fatalf("still held = %d", got)
	}
	f.now = f.now.Add(2 * time.Second) // no webhook arrived: the block goes back
	if got := f.balance(t, "u1"); got != 50 {
		t.Fatalf("after the TTL = %d, want 50", got)
	}
}

func TestLogModeTakesNothing(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, 2, coins.ModeLog, 5)
	j, err := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1"})
	if err != nil || j.CoinsReserved != 0 {
		t.Fatalf("join = %+v %v", j, err)
	}
	if err := f.svc.Handle(ctx, Event{Kind: KindLeft, Identity: j.Identity, JoinedAt: f.now, At: f.now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if got := f.balance(t, "u1"); got != 5 {
		t.Errorf("balance = %d", got)
	}
}

func TestMinutes(t *testing.T) {
	for d, want := range map[time.Duration]int64{0: 0, time.Second: 1, time.Minute: 1, time.Minute + time.Millisecond: 2, -time.Minute: 0} {
		if got := Minutes(d); got != want {
			t.Errorf("Minutes(%s) = %d, want %d", d, got, want)
		}
	}
}

// signed returns a webhook request signed like LiveKit Cloud.
func signed(t *testing.T, body, secret string) *http.Request {
	t.Helper()
	sum := sha256.Sum256([]byte(body))
	tok, err := auth.NewAccessToken(testKey, secret).SetValidFor(time.Minute).SetSha256(base64.StdEncoding.EncodeToString(sum[:])).ToJWT()
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, WebhookPath, bytes.NewBufferString(body))
	r.Header.Set("Authorization", tok)
	r.Header.Set("Content-Type", "application/webhook+json")
	return r
}

func TestWebhookSignature(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, 1, coins.ModeEnforce, 50)
	f.now = time.Now().UTC()
	j, _ := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1"})
	joined := f.now.Unix()
	body := fmt.Sprintf(`{"event":"participant_left","id":"EV_1","createdAt":"%d","room":{"name":"app:a1"},"participant":{"identity":%q,"joinedAt":"%d"}}`,
		joined+125, j.Identity, joined)
	mux := http.NewServeMux()
	Public(f.svc)(mux)

	for name, r := range map[string]*http.Request{
		"no header":    httptest.NewRequest(http.MethodPost, WebhookPath, strings.NewReader(body)),
		"wrong secret": signed(t, body, "another-secret-that-is-long-enough-to-sign"),
		"tampered": func() *http.Request {
			r := signed(t, body, testSecret)
			r.Body = http.NoBody
			return r
		}(),
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, r)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d, want 401", name, rec.Code)
		}
	}
	if s, _ := f.svc.Store.Get(ctx, j.SessionID); s.Settled {
		t.Fatal("a rejected webhook must not settle")
	}

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, signed(t, body, testSecret))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("signed: %d %s", rec.Code, rec.Body)
		}
	}
	if got := f.balance(t, "u1"); got != 47 {
		t.Errorf("balance = %d, want 47 (3 minutes once)", got)
	}
}
