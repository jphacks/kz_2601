package extapi

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// failStore_S7Backend005 fails the calls whose flag is set; the rest go to Memory.
type failStore_S7Backend005 struct {
	*Memory
	failDelete, failUpdate, failList bool
}

func (s *failStore_S7Backend005) Delete(ctx context.Context, uid, provider string) error {
	if s.failDelete {
		return errors.New("delete down")
	}
	return s.Memory.Delete(ctx, uid, provider)
}

func (s *failStore_S7Backend005) Update(ctx context.Context, uid, provider string, fn func(Connection, bool) (Connection, bool)) error {
	if s.failUpdate {
		return errors.New("update down")
	}
	return s.Memory.Update(ctx, uid, provider, fn)
}

func (s *failStore_S7Backend005) List(ctx context.Context, uid string) ([]Connection, error) {
	if s.failList {
		return nil, errors.New("list down")
	}
	return s.Memory.List(ctx, uid)
}

// failRevoke_S7Backend005 is an OAuth client whose revoke call fails.
type failRevoke_S7Backend005 struct{ fakeOAuthClient }

func (failRevoke_S7Backend005) Revoke(context.Context, Token) error {
	return errors.New("provider down")
}

type connLogEnv_S7Backend005 struct {
	c     *Connections
	st    *failStore_S7Backend005
	buf   *bytes.Buffer
	clock time.Time
}

func newConnLogEnv_S7Backend005(t *testing.T) *connLogEnv_S7Backend005 {
	t.Helper()
	sealer, err := NewLocalSealer(nil)
	if err != nil {
		t.Fatal(err)
	}
	buf := &bytes.Buffer{}
	e := &connLogEnv_S7Backend005{
		st:    &failStore_S7Backend005{Memory: NewMemory()},
		buf:   buf,
		clock: time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC),
	}
	e.c = &Connections{
		Store:  e.st,
		Sealer: sealer,
		Now:    func() time.Time { return e.clock },
		Log:    slog.New(slog.NewTextHandler(buf, nil)),
	}
	return e
}

func (e *connLogEnv_S7Backend005) save(t *testing.T, uid, provider string) {
	t.Helper()
	if err := e.c.Save(context.Background(), uid, provider, Token{AccessToken: "at-secret", RefreshToken: "rt-secret"}); err != nil {
		t.Fatal(err)
	}
}

// wantLog checks the log has every key and no secret or raw uid.
func (e *connLogEnv_S7Backend005) wantLog(t *testing.T, uid string, keys ...string) {
	t.Helper()
	out := e.buf.String()
	for _, k := range keys {
		if !strings.Contains(out, k) {
			t.Errorf("log lacks %q: %s", k, out)
		}
	}
	for _, bad := range []string{"at-secret", "rt-secret", uid} {
		if strings.Contains(out, bad) {
			t.Errorf("log leaks %q: %s", bad, out)
		}
	}
}

func TestConnectionsLog_S7Backend005(t *testing.T) {
	t.Parallel()
	const uid = "uid-person-1"
	ctx := context.Background()

	t.Run("default logger when Log is nil", func(t *testing.T) {
		t.Parallel()
		if got := (&Connections{}).log(); got != slog.Default() {
			t.Fatal("nil Log must fall back to slog.Default")
		}
		lg := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
		if got := (&Connections{Log: lg}).log(); got != lg {
			t.Fatal("Log must be used when set")
		}
	})

	t.Run("deleting an unused connection fails", func(t *testing.T) {
		t.Parallel()
		e := newConnLogEnv_S7Backend005(t)
		e.save(t, uid, FakeOAuth)
		e.st.failDelete = true
		e.clock = e.clock.Add(DefaultUnusedTTL + time.Hour)
		if _, err := e.c.Token(ctx, uid, FakeOAuth); !errors.Is(err, ErrNotConnected) {
			t.Fatalf("got %v", err)
		}
		e.wantLog(t, uid, "deleting an unused connection failed", "provider="+FakeOAuth, "delete down")
	})

	t.Run("noting a use fails", func(t *testing.T) {
		t.Parallel()
		e := newConnLogEnv_S7Backend005(t)
		e.save(t, uid, FakeOAuth)
		e.st.failUpdate = true
		e.clock = e.clock.Add(2 * time.Hour)
		if _, err := e.c.Token(ctx, uid, FakeOAuth); err != nil {
			t.Fatalf("the token is still returned: %v", err)
		}
		e.wantLog(t, uid, "noting a connection's use failed", "provider="+FakeOAuth, "update down")
	})

	t.Run("provider revoke fails", func(t *testing.T) {
		t.Parallel()
		e := newConnLogEnv_S7Backend005(t)
		e.save(t, uid, FakeOAuth)
		if err := e.c.Revoke(ctx, uid, FakeOAuth, failRevoke_S7Backend005{}); err != nil {
			t.Fatalf("revoke still cuts our side: %v", err)
		}
		e.wantLog(t, uid, "provider revoke failed", "provider="+FakeOAuth, "uid_hash=", "provider down")
		conn, ok, _ := e.st.Get(ctx, uid, FakeOAuth)
		if !ok || conn.Status != StatusRevoked {
			t.Fatalf("not revoked: %+v", conn)
		}
	})

	t.Run("listing for a shared grant fails", func(t *testing.T) {
		t.Parallel()
		e := newConnLogEnv_S7Backend005(t)
		e.save(t, uid, GoogleCalendar)
		e.st.failList = true
		if err := e.c.Revoke(ctx, uid, GoogleCalendar, failRevoke_S7Backend005{}); err != nil {
			t.Fatal(err)
		}
		e.wantLog(t, uid, "listing connections failed", "provider="+GoogleCalendar, "list down")
		if strings.Contains(e.buf.String(), "provider revoke failed") {
			t.Error("the provider revoke must be skipped")
		}
	})
}
