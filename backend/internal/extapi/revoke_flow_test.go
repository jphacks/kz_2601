package extapi

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

// startState runs Start for uid and returns the state (the consent is not
// finished yet).
func (e *env) startState(uid string) string {
	e.t.Helper()
	auth, err := e.svc.Start(context.Background(), uid, FakeOAuth, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	u, err := url.Parse(auth)
	if err != nil {
		e.t.Fatal(err)
	}
	return u.Query().Get("state")
}

func (e *env) callback(state string) string {
	return e.svc.Callback(context.Background(), FakeOAuth, state, "fake-code")
}

// RevokeAll (account deletion) deletes the person's started connections:
// their callback saves nothing. Someone else's goes on.
func TestRevokeAllDeletesStates(t *testing.T) {
	e := newEnv(t, opts{})
	mine, theirs := e.startState(user), e.startState("other")
	e.c.t = e.c.t.Add(time.Second)
	if _, err := e.svc.RevokeAll(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	if got := e.callback(mine); !strings.HasSuffix(got, "status=error") {
		t.Fatalf("callback after revokeAll: %q", got)
	}
	if c, ok := e.conn(user); ok && c.Status != StatusRevoked {
		t.Fatalf("connection saved after revokeAll: %+v", c)
	}
	if got := e.callback(theirs); !strings.HasSuffix(got, "status=ok") {
		t.Fatalf("someone else's callback: %q", got)
	}
}

// A callback that took its state before RevokeAll (the exchange still on
// its way) does not save the connection; a flow started afterwards does.
func TestCallbackAfterRevokeAllSavesNothing(t *testing.T) {
	e := newEnv(t, opts{})
	state := e.startState(user)
	e.mem.mu.Lock()
	taken := e.mem.states[state]
	e.mem.mu.Unlock()
	e.c.t = e.c.t.Add(time.Second)
	if n, err := e.svc.RevokeAll(context.Background(), user); err != nil || n != 0 {
		t.Fatalf("revoke all: %d %v", n, err)
	}
	// the state the in-flight callback already holds
	if err := e.mem.PutState(context.Background(), state, taken); err != nil {
		t.Fatal(err)
	}
	if got := e.callback(state); !strings.HasSuffix(got, "status=error") {
		t.Fatalf("in-flight callback: %q", got)
	}
	c, ok := e.conn(user)
	if !ok || c.Status != StatusRevoked || len(c.Sealed) != 0 {
		t.Fatalf("connection %+v %v", c, ok)
	}
	if _, err := e.call(e.req(FakeOAuth, "listEvents", nil)); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("call: %v", err)
	}
	// connecting again later works
	e.c.t = e.c.t.Add(time.Second)
	e.connect(user)
	if c, _ := e.conn(user); c.Status != StatusActive {
		t.Fatalf("reconnect %+v", c)
	}
}

// Cutting one provider while its consent screen is open: the callback saves nothing.
func TestCallbackAfterRevokeSavesNothing(t *testing.T) {
	e := newEnv(t, opts{})
	e.connect(user)
	state := e.startState(user) // asking for more scopes
	e.c.t = e.c.t.Add(time.Second)
	if err := e.svc.Revoke(context.Background(), user, FakeOAuth); err != nil {
		t.Fatal(err)
	}
	if got := e.callback(state); !strings.HasSuffix(got, "status=error") {
		t.Fatalf("callback after revoke: %q", got)
	}
	if c, _ := e.conn(user); c.Status != StatusRevoked {
		t.Fatalf("connection %+v", c)
	}
	err := e.svc.Conns.SaveSince(context.Background(), user, FakeOAuth, Token{AccessToken: "x"}, e.c.t.Add(-time.Minute))
	if !errors.Is(err, ErrCutSinceStart) || !errors.Is(err, ErrNotConnected) {
		t.Fatalf("SaveSince: %v", err)
	}
}

// savingClient saves a new connection while the provider's revoke runs
// (between Revoke's read and its write).
type savingClient struct {
	fakeOAuthClient
	during func()
}

func (c savingClient) Revoke(context.Context, Token) error {
	c.during()
	return nil
}

// Revoke writes the marker over the connection as it is then (one
// transaction), not over what it read first.
func TestRevokeMarksCurrentConnection(t *testing.T) {
	e := newEnv(t, opts{})
	e.connect(user)
	later := e.c.t.Add(time.Hour).UTC()
	cl := savingClient{during: func() {
		_ = e.mem.Put(context.Background(), user, Connection{Provider: FakeOAuth, Status: StatusActive, Sealed: []byte{9}, ConnectedAt: later})
	}}
	if err := e.svc.Conns.Revoke(context.Background(), user, FakeOAuth, cl); err != nil {
		t.Fatal(err)
	}
	c, _ := e.conn(user)
	if c.Status != StatusRevoked || len(c.Sealed) != 0 || !c.ConnectedAt.Equal(later) {
		t.Fatalf("marker %+v", c)
	}
}
