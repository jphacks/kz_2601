package extapi

import (
	"context"
	"strings"
	"testing"
	"time"
)

// A connection cut while its token was being renewed stays cut: the
// renewed token is not stored and the call is not made.
func TestRevokeDuringRefreshStaysRevoked(t *testing.T) {
	e := newEnv(t, opts{})
	rc := expiredConnection(t, e)
	rc.before = func() {
		if err := e.svc.Revoke(context.Background(), user, FakeOAuth); err != nil {
			t.Errorf("revoke: %v", err)
		}
	}
	if _, err := e.call(e.req(FakeOAuth, "listEvents", nil)); Reason(err) != ReasonNotConnected {
		t.Fatalf("err %v", err)
	}
	c, ok := e.conn(user)
	if !ok || c.Status != StatusRevoked || len(c.Sealed) != 0 {
		t.Fatalf("connection %+v", c)
	}
	if e.runs() != 0 {
		t.Fatalf("runs = %d", e.runs())
	}
}

// The provider refusing an old token does not mark a connection the person
// made again meanwhile.
func TestRefusedOldTokenKeepsNewConnection(t *testing.T) {
	e := newEnv(t, opts{})
	e.connect(user)
	e.svc.ops[FakeOAuth] = map[string]Operation{"listEvents": OperationFunc(func(context.Context, Input) (Output, error) {
		e.connect(user) // connected again while the call was out
		return Output{}, ErrInvalidGrant
	})}
	if _, err := e.call(e.req(FakeOAuth, "listEvents", nil)); Reason(err) != ReasonNotConnected {
		t.Fatalf("err %v", err)
	}
	if c, _ := e.conn(user); c.Status != StatusActive {
		t.Fatalf("connection %q", c.Status)
	}
	// the same refusal of the token still in use marks it
	e.svc.ops[FakeOAuth] = map[string]Operation{"listEvents": OperationFunc(func(context.Context, Input) (Output, error) {
		return Output{}, ErrInvalidGrant
	})}
	_, _ = e.call(e.req(FakeOAuth, "listEvents", nil))
	if c, _ := e.conn(user); c.Status != StatusError {
		t.Fatalf("connection %q", c.Status)
	}
}

// hookStore runs hook once, right after the next Get has read.
type hookStore struct {
	*Memory
	hook func()
}

func (h *hookStore) Get(ctx context.Context, uid, provider string) (Connection, bool, error) {
	c, ok, err := h.Memory.Get(ctx, uid, provider)
	if f := h.hook; f != nil {
		h.hook = nil
		f()
	}
	return c, ok, err
}

// Noting a connection's use only moves its dates: a token renewed (or a
// connection cut) between the read and the note is not overwritten.
func TestNotingUseKeepsNewerToken(t *testing.T) {
	for _, tc := range []string{"renewed", "revoked"} {
		t.Run(tc, func(t *testing.T) {
			ctx := context.Background()
			c := &clock{t: time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)}
			sealer, err := NewLocalSealer(nil)
			if err != nil {
				t.Fatal(err)
			}
			st := &hookStore{Memory: NewMemory()}
			conns := &Connections{Store: st, Sealer: sealer, Now: c.now}
			if err := conns.Save(ctx, user, FakeOAuth, Token{AccessToken: "old", RefreshToken: "r1"}); err != nil {
				t.Fatal(err)
			}
			c.t = c.t.Add(2 * time.Hour) // past the once-an-hour note
			st.hook = func() {
				if tc == "renewed" {
					cur, _, _ := st.Memory.Get(ctx, user, FakeOAuth)
					if _, err := conns.SaveRefreshed(ctx, user, FakeOAuth, cur.Sealed, Token{AccessToken: "new", RefreshToken: "r2"}); err != nil {
						t.Error(err)
					}
					return
				}
				if err := conns.Revoke(ctx, user, FakeOAuth, nil); err != nil {
					t.Error(err)
				}
			}
			if tok, err := conns.Token(ctx, user, FakeOAuth); err != nil || tok.AccessToken != "old" {
				t.Fatalf("token %v %v", tok.AccessToken, err)
			}
			got, ok, _ := st.Memory.Get(ctx, user, FakeOAuth)
			if tc == "revoked" {
				if !ok || got.Status != StatusRevoked || len(got.Sealed) != 0 {
					t.Fatalf("revoked connection came back: %+v", got)
				}
				return
			}
			tok, err := conns.Token(ctx, user, FakeOAuth)
			if err != nil || tok.RefreshToken != "r2" || !got.LastUsedAt.Equal(c.t) {
				t.Fatalf("stored %v %v %+v", tok.RefreshToken, err, got)
			}
		})
	}
}

// Update writes only when fn says so, and names the document's provider.
func TestStoreUpdate(t *testing.T) {
	eachStore(t, func(t *testing.T, st stores) {
		ctx := context.Background()
		u := "u-upd-" + strings.ReplaceAll(t.Name(), "/", "_") + "-" + time.Now().Format("150405.000000000")
		if err := st.Update(ctx, u, FakeOAuth, func(c Connection, ok bool) (Connection, bool) {
			if ok {
				t.Error("found a connection that is not there")
			}
			return c, false
		}); err != nil {
			t.Fatal(err)
		}
		if _, ok, _ := st.Get(ctx, u, FakeOAuth); ok {
			t.Fatal("written without being asked")
		}
		now := time.Now().UTC().Truncate(time.Millisecond)
		if err := st.Update(ctx, u, FakeOAuth, func(Connection, bool) (Connection, bool) {
			return Connection{Status: StatusActive, Sealed: []byte{1}, DeleteAt: now.Add(time.Hour)}, true
		}); err != nil {
			t.Fatal(err)
		}
		if err := st.Update(ctx, u, FakeOAuth, func(c Connection, ok bool) (Connection, bool) {
			if !ok || c.Provider != FakeOAuth || string(c.Sealed) != "\x01" {
				t.Errorf("read %+v %v", c, ok)
			}
			c.Status = StatusError
			return c, true
		}); err != nil {
			t.Fatal(err)
		}
		if c, ok, _ := st.Get(ctx, u, FakeOAuth); !ok || c.Status != StatusError || string(c.Sealed) != "\x01" {
			t.Fatalf("get %+v %v", c, ok)
		}
		_ = st.Delete(ctx, u, FakeOAuth)
	})
}
