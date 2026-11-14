package calls

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/coins"
)

func TestConfigFromEnvRejects(t *testing.T) {
	for name, m := range map[string]map[string]string{
		"bad CALLS_ENABLED":    {"CALLS_ENABLED": "yes please"},
		"URL without host":     onEnv(map[string]string{"LIVEKIT_URL": "wss://"}),
		"key only spaces":      onEnv(map[string]string{"LIVEKIT_API_KEY": "   "}),
		"block 0":              onEnv(map[string]string{"CALL_BLOCK_MINUTES": "0"}),
		"block 241":            onEnv(map[string]string{"CALL_BLOCK_MINUTES": "241"}),
		"block not a number":   onEnv(map[string]string{"CALL_BLOCK_MINUTES": "ten"}),
		"participants 1":       onEnv(map[string]string{"CALL_MAX_PARTICIPANTS": "1"}),
		"participants 51":      onEnv(map[string]string{"CALL_MAX_PARTICIPANTS": "51"}),
		"tokens per hour < 0":  onEnv(map[string]string{"CALL_TOKENS_PER_HOUR": "-1"}),
		"grace over a day":     onEnv(map[string]string{"CALL_HOLD_GRACE": "25h"}),
		"grace not a duration": onEnv(map[string]string{"CALL_HOLD_GRACE": "10"}),
		"token ttl under 1m":   onEnv(map[string]string{"CALL_TOKEN_TTL": "30s"}),
		"token ttl over 1h":    onEnv(map[string]string{"CALL_TOKEN_TTL": "61m"}),
		"price not a number":   onEnv(map[string]string{"COIN_PER_CALL_MINUTE": "1.5"}),
		"price huge":           onEnv(map[string]string{"COIN_PER_CALL_MINUTE": strings.Repeat("9", 30)}),
	} {
		if _, err := ConfigFromEnv(env(m)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	// two bad keys: the first one checked is the one reported
	_, err := ConfigFromEnv(env(onEnv(map[string]string{"CALL_BLOCK_MINUTES": "0", "CALL_TOKEN_TTL": "30s"})))
	if err == nil || !strings.Contains(err.Error(), "CALL_BLOCK_MINUTES") || strings.Contains(err.Error(), "CALL_TOKEN_TTL") {
		t.Errorf("first error is reported: got %v", err)
	}
}

func TestConfigFromEnvValues(t *testing.T) {
	for _, off := range []string{"", "0", "false", "OFF", " off "} {
		c, err := ConfigFromEnv(env(map[string]string{"CALLS_ENABLED": off, "CALL_BLOCK_MINUTES": "nonsense"}))
		if err != nil || c.Enabled {
			t.Errorf("%q: off must not read the rest: %+v %v", off, c, err)
		}
	}
	c, err := ConfigFromEnv(env(onEnv(map[string]string{
		"CALLS_ENABLED": " ON ", "LIVEKIT_URL": " https://example.livekit.cloud ",
		"CALL_BLOCK_MINUTES": "240", "CALL_MAX_PARTICIPANTS": "2", "CALL_TOKENS_PER_HOUR": "0",
		"CALL_HOLD_GRACE": "0s", "CALL_TOKEN_TTL": "1h", "COIN_PER_CALL_MINUTE": " 0 ",
	})))
	if err != nil {
		t.Fatal(err)
	}
	if !c.Enabled || c.URL != "https://example.livekit.cloud" || c.BlockMinutes != 240 || c.MaxParticipants != 2 ||
		c.TokensPerHour != 0 || c.HoldGrace != 0 || c.TokenTTL != time.Hour || c.PerMinute != 0 {
		t.Fatalf("got %+v", c)
	}
	if c.BlockDuration() != 4*time.Hour {
		t.Errorf("BlockDuration = %s", c.BlockDuration())
	}
}

// failingStore fails the operations named in fail.
type failingStore struct {
	*Memory
	fail  map[string]bool
	opens int
}

var errStore = errors.New("store unavailable")

func (s *failingStore) Get(ctx context.Context, id string) (Session, error) {
	if s.fail["get"] {
		return Session{}, errStore
	}
	return s.Memory.Get(ctx, id)
}

func (s *failingStore) Update(ctx context.Context, id string, fn func(*Session) error) (Session, error) {
	if s.fail["update"] {
		return Session{}, errStore
	}
	return s.Memory.Update(ctx, id, fn)
}

func (s *failingStore) Open(ctx context.Context, room string) ([]Session, error) {
	s.opens++
	if s.fail["open"] {
		return nil, errStore
	}
	return s.Memory.Open(ctx, room)
}

func TestExpireEdgeCases(t *testing.T) {
	ctx := context.Background()
	f, rooms, timers := limitedFixture(t, 100)
	if err := f.svc.Expire(ctx, "unknown"); err != nil {
		t.Fatalf("an unknown session is nothing to do: %v", err)
	}

	// not joined yet: no PaidUntil, nobody taken out
	j, _ := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1"})
	f.now = f.now.Add(time.Hour)
	if err := f.svc.Expire(ctx, j.SessionID); err != nil {
		t.Fatal(err)
	}
	if s, _ := f.svc.Store.Get(ctx, j.SessionID); s.Settled || rooms.count() != 0 {
		t.Fatalf("a session not joined must not be ended: %+v %v", s, rooms.removed)
	}

	// a store error is returned (the timer tries again)
	st := &failingStore{Memory: f.svc.Store.(*Memory), fail: map[string]bool{"get": true}}
	f.svc.Store = st
	if err := f.svc.Expire(ctx, j.SessionID); !errors.Is(err, errStore) {
		t.Fatalf("store error: %v", err)
	}
	st.fail = nil

	// already settled: only the timer is dropped
	_ = f.svc.Handle(ctx, Event{Kind: KindJoined, Room: j.Room, Identity: j.Identity, JoinedAt: f.now})
	f.now = f.now.Add(time.Minute)
	_ = f.svc.Handle(ctx, Event{Kind: KindLeft, Room: j.Room, Identity: j.Identity, At: f.now})
	f.now = f.now.Add(time.Hour)
	f.svc.arm(j.SessionID, f.now.Add(time.Hour)) // a stale timer left over
	if len(timers.pending()) != 1 {
		t.Fatalf("setup: want 1 armed timer, got %v", timers.pending())
	}
	if err := f.svc.Expire(ctx, j.SessionID); err != nil {
		t.Fatal(err)
	}
	if rooms.count() != 0 || len(timers.pending()) != 0 {
		t.Fatalf("a settled session must not be removed again: %v, timers %v", rooms.removed, timers.pending())
	}
}

// A failed removal keeps the stay open (so it is not settled while the
// participant is still in the room).
func TestExpireRemoveFailureDoesNotSettle(t *testing.T) {
	ctx := context.Background()
	f, rooms, _ := limitedFixture(t, 100)
	j, _ := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1"})
	_ = f.svc.Handle(ctx, Event{Kind: KindJoined, Room: j.Room, Identity: j.Identity, JoinedAt: f.now})
	f.now = f.now.Add(11 * time.Minute)
	rooms.err = errors.New("livekit down")
	if err := f.svc.Expire(ctx, j.SessionID); err == nil {
		t.Fatal("a failed removal must be an error")
	}
	if s, _ := f.svc.Store.Get(ctx, j.SessionID); s.Settled {
		t.Fatal("must not settle while still in the room")
	}
	if got := f.balance(t, "u1"); got != 80 {
		t.Fatalf("balance = %d, want 80 (the block still held)", got)
	}
}

// A sweep that cannot list the room is only logged: the webhook still succeeds.
func TestSweepRoomStoreError(t *testing.T) {
	ctx := context.Background()
	f, rooms, _ := limitedFixture(t, 100)
	j, _ := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1"})
	_ = f.svc.Handle(ctx, Event{Kind: KindJoined, Room: j.Room, Identity: j.Identity, JoinedAt: f.now})
	st := &failingStore{Memory: f.svc.Store.(*Memory), fail: map[string]bool{"open": true}}
	f.svc.Store = st
	f.now = f.now.Add(time.Hour)
	// KindLeft of an unknown identity: settle ignores ErrNotFound, the sweep still runs
	if err := f.svc.Handle(ctx, Event{Kind: KindLeft, Room: j.Room, Identity: "someone-else", At: f.now}); err != nil {
		t.Fatalf("a failed sweep must not fail the webhook: %v", err)
	}
	if st.opens < 1 {
		t.Fatal("the sweep must have tried to list the room")
	}
	if rooms.count() != 0 {
		t.Fatalf("removed = %v", rooms.removed)
	}
	f.svc.sweepRoom(ctx, "") // no room: nothing to list
}

// fakeCoins counts Extend calls and can fail them.
type fakeCoins struct {
	*coins.Service
	extendErr error
	extended  int
}

func (c *fakeCoins) Extend(ctx context.Context, h *coins.Hold, at time.Time) error {
	c.extended++
	if c.extendErr != nil {
		return c.extendErr
	}
	return c.Service.Extend(ctx, h, at)
}

// A renewal still issues a token when keeping an earlier block's hold fails.
func TestRenewalWhenExtendFails(t *testing.T) {
	ctx := context.Background()
	f, _, _ := limitedFixture(t, 100)
	fc := &fakeCoins{Service: f.coins, extendErr: errors.New("firestore contention")}
	f.svc.Coins = fc
	j, err := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1", SessionID: j.SessionID})
	if err != nil || r.SessionID != j.SessionID || r.CoinsReserved != 20 {
		t.Fatalf("renewal = %+v %v", r, err)
	}
	if fc.extended != 1 {
		t.Fatalf("Extend calls = %d, want 1 (the first block)", fc.extended)
	}
	if s, _ := f.svc.Store.Get(ctx, j.SessionID); len(s.Blocks) != 2 {
		t.Fatalf("blocks = %+v", s.Blocks)
	}
}

// A renewal of another room of the same app is refused (the session is
// bound to its room).
func TestRenewalOtherRoomRefused(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, 0, coins.ModeOff, 0)
	j, err := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1", Room: "lobby"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1", Room: "vip", SessionID: j.SessionID}); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("other room: %v", err)
	}
	f.svc.Store = &failingStore{Memory: f.svc.Store.(*Memory), fail: map[string]bool{"get": true}}
	if _, err := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1", Room: "lobby", SessionID: j.SessionID}); !errors.Is(err, errStore) {
		t.Fatalf("store error: %v", err)
	}
}

// A settlement that cannot be stored is 500 so LiveKit sends it again.
func TestWebhookInternalErrorAndSize(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, 1, coins.ModeEnforce, 50)
	f.now = time.Now().UTC()
	j, _ := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1"})
	body := fmt.Sprintf(`{"event":"participant_left","id":"EV_2","room":{"name":"app:a1"},"participant":{"identity":%q,"joinedAtMs":"%d"}}`,
		j.Identity, f.now.Add(-2*time.Minute).UnixMilli())
	f.svc.Store = &failingStore{Memory: f.svc.Store.(*Memory), fail: map[string]bool{"update": true}}
	mux := http.NewServeMux()
	Public(f.svc)(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, signed(t, body, testSecret))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("store failure: %d %s", rec.Code, rec.Body)
	}

	// a body past maxWebhookBytes is not read (and is not a valid event)
	big := `{"event":"participant_left","pad":"` + strings.Repeat("a", maxWebhookBytes) + `"}`
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, signed(t, big, testSecret))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("oversized: %d", rec.Code)
	}
}

func TestPublicOffMountsNothing(t *testing.T) {
	mux := http.NewServeMux()
	Public(&Service{})(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, WebhookPath, strings.NewReader("{}")))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("calls off: %d, want 404", rec.Code)
	}
	Public(nil)(mux) // a nil service is off too
}

func TestUnix(t *testing.T) {
	at := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	if got := unix(at.Unix(), 0); !got.Equal(at) {
		t.Errorf("seconds: %s", got)
	}
	if got := unix(1, at.Add(250*time.Millisecond).UnixMilli()); !got.Equal(at.Add(250 * time.Millisecond)) {
		t.Errorf("milliseconds win: %s", got)
	}
	if got := unix(0, 0); !got.IsZero() {
		t.Errorf("none: %s", got)
	}
	if got := unix(-5, -5); !got.IsZero() {
		t.Errorf("negative: %s", got)
	}
}

func TestNewIDIsRandomHex(t *testing.T) {
	s := &Service{}
	a, b := s.newID(), s.newID()
	if len(a) != 32 || strings.Trim(a, "0123456789abcdef") != "" || a == b {
		t.Fatalf("ids %q %q", a, b)
	}
}
