package calls

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/livekit/protocol/auth"

	"github.com/jphacks/kz_2601/backend/internal/coins"
)

// fakeRooms is the RoomService of the tests (no LiveKit).
type fakeRooms struct {
	mu      sync.Mutex
	removed []string // room/identity
	err     error
}

func (r *fakeRooms) RemoveParticipant(_ context.Context, room, identity string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.removed = append(r.removed, room+"/"+identity)
	return nil
}

func (r *fakeRooms) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.removed)
}

// fakeTimers records the timers instead of running them.
type fakeTimers struct {
	mu sync.Mutex
	at map[int]time.Duration
	fs map[int]func()
	n  int
}

func (t *fakeTimers) after(d time.Duration, f func()) func() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.at == nil {
		t.at, t.fs = map[int]time.Duration{}, map[int]func(){}
	}
	t.n++
	id := t.n
	t.at[id], t.fs[id] = d, f
	return func() bool {
		t.mu.Lock()
		defer t.mu.Unlock()
		_, ok := t.fs[id]
		delete(t.fs, id)
		return ok
	}
}

// fire runs the pending timers (once each).
func (t *fakeTimers) fire() {
	t.mu.Lock()
	fs := t.fs
	t.fs = map[int]func(){}
	t.mu.Unlock()
	for _, f := range fs {
		f()
	}
}

func (t *fakeTimers) pending() []time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []time.Duration
	for id := range t.fs {
		out = append(out, t.at[id])
	}
	return out
}

func limitedFixture(t *testing.T, grant int64) (*fixture, *fakeRooms, *fakeTimers) {
	t.Helper()
	f := newFixture(t, 2, coins.ModeEnforce, grant)
	rooms, timers := &fakeRooms{}, &fakeTimers{}
	f.svc.Rooms, f.svc.AfterFunc = rooms, timers.after
	return f, rooms, timers
}

// tokenLife is exp - iat of a token (LiveKit stamps them from the wall clock).
func tokenLife(t *testing.T, token string) time.Duration {
	t.Helper()
	v, err := auth.ParseAPIToken(token)
	if err != nil {
		t.Fatal(err)
	}
	reg, _, err := v.Verify(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	return reg.ExpiresAt.Sub(reg.IssuedAt.Time)
}

// The token is never usable past what the holds pay for.
func TestTokenTTLBoundedByHolds(t *testing.T) {
	ctx := context.Background()
	f, _, _ := limitedFixture(t, 100)
	f.svc.Config.TokenTTL = time.Hour // longer than a 10-minute block
	j, err := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if want := f.now.Add(10 * time.Minute); !j.ExpiresAt.Equal(want) || tokenLife(t, j.Token) > 10*time.Minute {
		t.Fatalf("expiresAt %v / jwt life %v, want %v", j.ExpiresAt, tokenLife(t, j.Token), want)
	}
	_ = f.svc.Handle(ctx, Event{Kind: KindJoined, Room: j.Room, Identity: j.Identity, JoinedAt: f.now})
	f.now = f.now.Add(9 * time.Minute)
	r, err := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1", SessionID: j.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	// two blocks from the join: 20 minutes after it, 11 from now
	if want := f.now.Add(11 * time.Minute); !r.ExpiresAt.Equal(want) || tokenLife(t, r.Token) > 11*time.Minute {
		t.Fatalf("renewal expiresAt %v, want %v", r.ExpiresAt, want)
	}

	// not charged: CALL_TOKEN_TTL as before
	free := newFixture(t, 0, coins.ModeOff, 0)
	free.svc.Config.TokenTTL = time.Hour
	fj, _ := free.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1"})
	if !fj.ExpiresAt.Equal(free.now.Add(time.Hour)) {
		t.Errorf("free expiresAt = %v", fj.ExpiresAt)
	}
}

// When the holds run out the participant is taken out and the stay is settled.
func TestRemovedWhenHoldsRunOut(t *testing.T) {
	ctx := context.Background()
	f, rooms, timers := limitedFixture(t, 50)
	j, _ := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1"})
	if err := f.svc.Handle(ctx, Event{Kind: KindJoined, Room: j.Room, Identity: j.Identity, JoinedAt: f.now}); err != nil {
		t.Fatal(err)
	}
	if p := timers.pending(); len(p) != 1 || p[0] != 10*time.Minute {
		t.Fatalf("timers = %v, want one at 10m", p)
	}
	s, _ := f.svc.Store.Get(ctx, j.SessionID)
	if !s.PaidUntil.Equal(f.now.Add(10 * time.Minute)) {
		t.Fatalf("paidUntil = %v", s.PaidUntil)
	}
	start := f.now
	f.now = start.Add(5 * time.Minute)
	timers.fire() // early (another instance moved it): only set again
	if rooms.count() != 0 || len(timers.pending()) != 1 {
		t.Fatalf("early: removed %v, timers %v", rooms.removed, timers.pending())
	}
	f.now = start.Add(10*time.Minute + time.Second)
	timers.fire()
	if rooms.count() != 1 || rooms.removed[0] != "app:a1/"+j.Identity {
		t.Fatalf("removed = %v", rooms.removed)
	}
	s, _ = f.svc.Store.Get(ctx, j.SessionID)
	if !s.Settled || s.Minutes != 10 || !s.EndedAt.Equal(start.Add(10*time.Minute)) {
		t.Fatalf("session = %+v", s)
	}
	if got := f.balance(t, "u1"); got != 30 {
		t.Errorf("balance = %d, want 30 (the one block)", got)
	}
	// LiveKit then sends participant_left: nothing changes
	if err := f.svc.Handle(ctx, Event{Kind: KindLeft, Room: j.Room, Identity: j.Identity, At: f.now}); err != nil {
		t.Fatal(err)
	}
	if got := f.balance(t, "u1"); got != 30 || len(timers.pending()) != 0 {
		t.Errorf("after left: balance %d, timers %v", got, timers.pending())
	}
}

// A renewal moves the end; a failed removal is tried again.
func TestRenewalMovesTheEnd(t *testing.T) {
	ctx := context.Background()
	f, rooms, timers := limitedFixture(t, 100)
	j, _ := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1"})
	_ = f.svc.Handle(ctx, Event{Kind: KindJoined, Room: j.Room, Identity: j.Identity, JoinedAt: f.now})
	start := f.now
	f.now = start.Add(9 * time.Minute)
	if _, err := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1", SessionID: j.SessionID}); err != nil {
		t.Fatal(err)
	}
	if p := timers.pending(); len(p) != 1 || p[0] != 11*time.Minute {
		t.Fatalf("timers = %v, want one at 11m", p)
	}
	f.now = start.Add(20 * time.Minute)
	rooms.err = errors.New("unavailable")
	timers.fire()
	if s, _ := f.svc.Store.Get(ctx, j.SessionID); s.Settled {
		t.Fatal("must not settle while the participant is still in the room")
	}
	if p := timers.pending(); len(p) != 1 || p[0] != retryRemove {
		t.Fatalf("retry timers = %v", p)
	}
	rooms.err = nil
	timers.fire()
	if s, _ := f.svc.Store.Get(ctx, j.SessionID); !s.Settled || s.Minutes != 20 || rooms.count() != 1 {
		t.Fatalf("session = %+v, removed %v", s, rooms.removed)
	}
	if got := f.balance(t, "u1"); got != 60 {
		t.Errorf("balance = %d, want 60", got)
	}
}

// A renewal after the holds ran out ends the stay instead.
func TestLateRenewalEnds(t *testing.T) {
	ctx := context.Background()
	f, rooms, _ := limitedFixture(t, 100)
	j, _ := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1"})
	_ = f.svc.Handle(ctx, Event{Kind: KindJoined, Room: j.Room, Identity: j.Identity, JoinedAt: f.now})
	f.now = f.now.Add(12 * time.Minute)
	if _, err := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1", SessionID: j.SessionID}); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("late renewal: %v", err)
	}
	if s, _ := f.svc.Store.Get(ctx, j.SessionID); !s.Settled || len(s.Blocks) != 1 || rooms.count() != 1 {
		t.Fatalf("session = %+v, removed %v", s, rooms.removed)
	}
	if got := f.balance(t, "u1"); got != 80 {
		t.Errorf("balance = %d, want 80", got)
	}
}

// After the stay is settled, joining again with its token is turned away.
func TestRejoinAfterSettleIsRemoved(t *testing.T) {
	ctx := context.Background()
	f, rooms, _ := limitedFixture(t, 50)
	j, _ := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1"})
	joined := f.now
	_ = f.svc.Handle(ctx, Event{Kind: KindJoined, Room: j.Room, Identity: j.Identity, JoinedAt: joined})
	f.now = f.now.Add(2 * time.Minute)
	_ = f.svc.Handle(ctx, Event{Kind: KindLeft, Room: j.Room, Identity: j.Identity, At: f.now})
	if rooms.count() != 0 {
		t.Fatalf("a leave removes nobody: %v", rooms.removed)
	}
	f.now = f.now.Add(time.Minute) // the token is still within its lifetime
	if err := f.svc.Handle(ctx, Event{Kind: KindJoined, Room: j.Room, Identity: j.Identity, JoinedAt: f.now}); err != nil {
		t.Fatal(err)
	}
	if rooms.count() != 1 {
		t.Fatalf("rejoin must be removed: %v", rooms.removed)
	}
	s, _ := f.svc.Store.Get(ctx, j.SessionID)
	if s.Minutes != 2 || !s.JoinedAt.Equal(joined) {
		t.Errorf("the settled stay must not change: %+v", s)
	}
	if got := f.balance(t, "u1"); got != 46 {
		t.Errorf("balance = %d, want 46", got)
	}
}

// Any webhook of a room catches up sessions whose timer was lost.
func TestWebhookSweepsTheRoom(t *testing.T) {
	ctx := context.Background()
	f, rooms, _ := limitedFixture(t, 100)
	a, _ := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1"})
	_ = f.svc.Handle(ctx, Event{Kind: KindJoined, Room: a.Room, Identity: a.Identity, JoinedAt: f.now})
	f.now = f.now.Add(15 * time.Minute)
	f.svc.AfterFunc = (&fakeTimers{}).after // as if on another instance
	b, _ := f.svc.Join(ctx, JoinRequest{UID: "u2", AppID: "a1"})
	if err := f.svc.Handle(ctx, Event{Kind: KindJoined, Room: b.Room, Identity: b.Identity, JoinedAt: f.now}); err != nil {
		t.Fatal(err)
	}
	if rooms.count() != 1 || rooms.removed[0] != "app:a1/"+a.Identity {
		t.Fatalf("removed = %v", rooms.removed)
	}
}

// Log mode and free calls are not bounded.
func TestNotBoundedWithoutCharge(t *testing.T) {
	ctx := context.Background()
	for _, f := range []*fixture{newFixture(t, 2, coins.ModeLog, 5), newFixture(t, 0, coins.ModeEnforce, 5)} {
		rooms, timers := &fakeRooms{}, &fakeTimers{}
		f.svc.Rooms, f.svc.AfterFunc = rooms, timers.after
		j, _ := f.svc.Join(ctx, JoinRequest{UID: "u1", AppID: "a1"})
		_ = f.svc.Handle(ctx, Event{Kind: KindJoined, Room: j.Room, Identity: j.Identity, JoinedAt: f.now})
		if len(timers.pending()) != 0 {
			t.Errorf("timers = %v", timers.pending())
		}
		if s, _ := f.svc.Store.Get(ctx, j.SessionID); !s.PaidUntil.IsZero() {
			t.Errorf("paidUntil = %v", s.PaidUntil)
		}
	}
}

// LiveKitRooms calls RemoveParticipant with an admin token of that room only.
func TestLiveKitRoomsRemove(t *testing.T) {
	var gotAuth, gotPath string
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		body, _ = io.ReadAll(r.Body)
		if bytes.Contains(body, []byte("gone")) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":"not_found","msg":"participant not found"}`))
			return
		}
		w.Header().Set("Content-Type", "application/protobuf") // an empty RemoveParticipantResponse

	}))
	defer srv.Close()
	r := NewLiveKitRooms(Config{URL: srv.URL, APIKey: testKey, APISecret: testSecret})
	if err := r.RemoveParticipant(context.Background(), "app:a1", "s1"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/twirp/livekit.RoomService/RemoveParticipant" || !bytes.Contains(body, []byte("app:a1")) || !bytes.Contains(body, []byte("s1")) {
		t.Fatalf("path %q body %q", gotPath, body)
	}
	v, err := auth.ParseAPIToken(gotAuth[len("Bearer "):])
	if err != nil {
		t.Fatal(err)
	}
	_, claims, err := v.Verify(testSecret)
	if err != nil || !claims.Video.RoomAdmin || claims.Video.Room != "app:a1" || claims.Video.RoomJoin {
		t.Fatalf("grant = %+v %v", claims.Video, err)
	}
	if err := r.RemoveParticipant(context.Background(), "app:a1", "gone"); err != nil {
		t.Errorf("already gone must be nil: %v", err)
	}
}
