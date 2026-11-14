package extapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// failStore3274 fails the writes asked for; reads go to the embedded Memory.
type failStore3274 struct {
	*Memory
	failDelete, failUpdate bool
}

func (f *failStore3274) Delete(ctx context.Context, uid, provider string) error {
	if f.failDelete {
		return errors.New("delete boom")
	}
	return f.Memory.Delete(ctx, uid, provider)
}

func (f *failStore3274) Update(ctx context.Context, uid, provider string, fn func(Connection, bool) (Connection, bool)) error {
	if f.failUpdate {
		return errors.New("update boom")
	}
	return f.Memory.Update(ctx, uid, provider, fn)
}

const (
	uid3274  = "uid-secret-3274"
	prov3274 = "somePrv"
	tok3274  = "access-token-3274"
)

func conns3274(t *testing.T, st ConnectionStore, now *time.Time, buf *bytes.Buffer) *Connections {
	t.Helper()
	sealer, err := NewLocalSealer(nil)
	if err != nil {
		t.Fatal(err)
	}
	lg := slog.New(slog.NewJSONHandler(buf, nil))
	return &Connections{Store: st, Sealer: sealer, Now: func() time.Time { return *now }, Log: lg}
}

func lines3274(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if l == "" {
			continue
		}
		m := map[string]any{}
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("not json: %q", l)
		}
		out = append(out, m)
	}
	return out
}

func TestConnectionsLogChoice_3274(t *testing.T) {
	if got := (&Connections{}).log(); got != slog.Default() {
		t.Error("nil Log must fall back to slog.Default()")
	}
	lg := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	if got := (&Connections{Log: lg}).log(); got != lg {
		t.Error("Log must be used when set")
	}
}

func TestConnectionsLogDeleteFailure_3274(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var buf bytes.Buffer
	st := &failStore3274{Memory: NewMemory()}
	c := conns3274(t, st, &now, &buf)
	ctx := context.Background()
	if err := c.Save(ctx, uid3274, prov3274, Token{AccessToken: tok3274, RefreshToken: "r"}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(c.ttl() + time.Hour) // past DeleteAt
	st.failDelete = true
	if _, _, err := c.read(ctx, uid3274, prov3274); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("err = %v, want ErrNotConnected", err)
	}
	ls := lines3274(t, &buf)
	if len(ls) != 1 {
		t.Fatalf("log lines = %d: %s", len(ls), buf.String())
	}
	l := ls[0]
	if l["level"] != "WARN" || l["provider"] != prov3274 || l["error"] != "delete boom" || !strings.Contains(l["msg"].(string), "deleting") {
		t.Fatalf("line = %v", l)
	}
	assertNoPII3274(t, buf.String())
}

func TestConnectionsLogUseNoteFailure_3274(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var buf bytes.Buffer
	st := &failStore3274{Memory: NewMemory()}
	c := conns3274(t, st, &now, &buf)
	ctx := context.Background()
	if err := c.Save(ctx, uid3274, prov3274, Token{AccessToken: tok3274, RefreshToken: "r"}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour) // over an hour since the last use
	st.failUpdate = true
	tok, _, err := c.read(ctx, uid3274, prov3274)
	if err != nil || tok.AccessToken != tok3274 {
		t.Fatalf("read = %v %v; a failed note must not fail the read", tok, err)
	}
	ls := lines3274(t, &buf)
	if len(ls) != 1 || ls[0]["level"] != "WARN" || ls[0]["provider"] != prov3274 || ls[0]["error"] != "update boom" {
		t.Fatalf("lines = %v", ls)
	}
	assertNoPII3274(t, buf.String())
}

func TestConnectionsLogQuietOnSuccess_3274(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var buf bytes.Buffer
	c := conns3274(t, NewMemory(), &now, &buf)
	ctx := context.Background()
	if err := c.Save(ctx, uid3274, prov3274, Token{AccessToken: tok3274}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	if _, _, err := c.read(ctx, uid3274, prov3274); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatalf("unexpected log: %s", buf.String())
	}
}

// the log carries the provider and the error, never the person or the token.
func assertNoPII3274(t *testing.T, s string) {
	t.Helper()
	for _, leak := range []string{uid3274, tok3274} {
		if strings.Contains(s, leak) {
			t.Fatalf("%q in log: %s", leak, s)
		}
	}
}
