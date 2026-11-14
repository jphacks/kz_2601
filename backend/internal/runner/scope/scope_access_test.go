package scope

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

var errBackend = errors.New("backend down")

// accessBackend counts AppOwner calls and can fail the directory reads.
type accessBackend struct {
	*store.Memory
	ownerCalls int
	fail       bool
}

func (b *accessBackend) AppOwner(ctx context.Context, appID string) (string, error) {
	b.ownerCalls++
	if b.fail {
		return "", errBackend
	}
	return b.Memory.AppOwner(ctx, appID)
}

func (b *accessBackend) ListMembers(ctx context.Context, appID string, limit int) ([]store.Member, error) {
	if b.fail {
		return nil, errBackend
	}
	return b.Memory.ListMembers(ctx, appID, limit)
}

func (b *accessBackend) ListBlocks(ctx context.Context, uid string) ([]store.Block, error) {
	if b.fail {
		return nil, errBackend
	}
	return b.Memory.ListBlocks(ctx, uid)
}

func (b *accessBackend) NotifyAllowed(ctx context.Context, uid, appID string) (bool, error) {
	if b.fail {
		return false, errBackend
	}
	return b.Memory.NotifyAllowed(ctx, uid, appID)
}

func sharedHandle(t *testing.T, b Backend, appID string) *Handle {
	t.Helper()
	h, err := New(b, "test", store.OutboxEvent{AppID: appID, Scope: "shared"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// Owner: the creator of a shared app (read once), "" when the app has no
// owner (gone), the user of a private one without reading app_owners.
func TestHandleOwner(t *testing.T) {
	ctx := context.Background()
	b := &accessBackend{Memory: store.NewMemory()}
	if _, err := b.ClaimApp(ctx, "creator", "S"); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		ev    store.OutboxEvent
		want  string
		calls int
	}{
		{"shared", store.OutboxEvent{AppID: "S", Scope: "shared"}, "creator", 1},
		{"public", store.OutboxEvent{AppID: "S", Scope: "public"}, "creator", 1},
		{"shared without owner", store.OutboxEvent{AppID: "gone", Scope: "shared"}, "", 1},
		{"private", store.OutboxEvent{AppID: "S", Scope: "private", UID: "u"}, "u", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b.ownerCalls = 0
			h, err := New(b, "test", c.ev, nil)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				got, err := h.Owner(ctx)
				if err != nil || got != c.want {
					t.Fatalf("owner #%d: %q %v, want %q", i, got, err, c.want)
				}
			}
			if b.ownerCalls != c.calls {
				t.Fatalf("AppOwner calls %d, want %d", b.ownerCalls, c.calls)
			}
		})
	}

	// an error is returned and not cached
	fb := &accessBackend{Memory: b.Memory, fail: true}
	h := sharedHandle(t, fb, "S")
	if _, err := h.Owner(ctx); !errors.Is(err, errBackend) {
		t.Fatalf("owner error: %v", err)
	}
	fb.fail = false
	if got, err := h.Owner(ctx); err != nil || got != "creator" {
		t.Fatalf("owner after error: %q %v", got, err)
	}
}

// Members: the members of a shared app in join order, none for an app
// without members or for private data.
func TestHandleMembers(t *testing.T) {
	ctx := context.Background()
	b := &accessBackend{Memory: store.NewMemory()}
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, uid := range []string{"m2", "m1"} {
		if _, err := b.AddMember(ctx, "S", store.Member{UID: uid, JoinedAt: t0.Add(time.Duration(i) * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := b.AddMember(ctx, "other", store.Member{UID: "x"}); err != nil {
		t.Fatal(err)
	}
	private, err := New(b, "test", store.OutboxEvent{AppID: "S", Scope: "private", UID: "m1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		h    *Handle
		want []string
	}{
		{"shared", sharedHandle(t, b, "S"), []string{"m2", "m1"}},
		{"no members", sharedHandle(t, b, "empty"), []string{}},
		{"private", private, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.h.Members(ctx)
			if err != nil || !reflect.DeepEqual(got, c.want) {
				t.Fatalf("members %#v %v, want %#v", got, err, c.want)
			}
		})
	}
	b.fail = true
	if _, err := sharedHandle(t, b, "S").Members(ctx); !errors.Is(err, errBackend) {
		t.Fatalf("members error: %v", err)
	}
}

// Blocks: the users uid blocked (not who blocked uid), oldest first; the
// list does not depend on the handle's app.
func TestHandleBlocks(t *testing.T) {
	ctx := context.Background()
	b := &accessBackend{Memory: store.NewMemory()}
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, blocked := range []string{"b2", "b1"} {
		if err := b.PutBlock(ctx, "u", store.Block{UID: blocked, CreatedAt: t0.Add(time.Duration(i) * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.PutBlock(ctx, "v", store.Block{UID: "u"}); err != nil {
		t.Fatal(err)
	}
	private, err := New(b, "test", store.OutboxEvent{AppID: "P", Scope: "private", UID: "v"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		h    *Handle
		uid  string
		want []string
	}{
		{"blocker", sharedHandle(t, b, "S"), "u", []string{"b2", "b1"}},
		{"other app", sharedHandle(t, b, "T"), "u", []string{"b2", "b1"}},
		{"private handle", private, "u", []string{"b2", "b1"}},
		{"blocked one", sharedHandle(t, b, "S"), "b1", []string{}},
		{"blocks u", sharedHandle(t, b, "S"), "v", []string{"u"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.h.Blocks(ctx, c.uid)
			if err != nil || !reflect.DeepEqual(got, c.want) {
				t.Fatalf("blocks of %s: %#v %v, want %#v", c.uid, got, err, c.want)
			}
		})
	}
	b.fail = true
	if _, err := sharedHandle(t, b, "S").Blocks(ctx, "u"); !errors.Is(err, errBackend) {
		t.Fatalf("blocks error: %v", err)
	}
}

// Allows: uid's stored consent for the handle's app only; none stored is
// not allowed.
func TestHandleAllows(t *testing.T) {
	ctx := context.Background()
	b := &accessBackend{Memory: store.NewMemory()}
	for _, c := range []struct {
		uid, app string
		allowed  bool
	}{
		{"yes", "S", true},
		{"no", "S", false},
		{"elsewhere", "T", true},
		{"revoked", "S", true},
		{"revoked", "S", false},
	} {
		if _, err := b.SetNotifyConsent(ctx, c.uid, c.app, c.allowed); err != nil {
			t.Fatal(err)
		}
	}
	h := sharedHandle(t, b, "S")
	for _, c := range []struct {
		uid  string
		want bool
	}{
		{"yes", true},
		{"no", false},
		{"elsewhere", false},
		{"revoked", false},
		{"never", false},
	} {
		t.Run(c.uid, func(t *testing.T) {
			got, err := h.Allows(ctx, c.uid)
			if err != nil || got != c.want {
				t.Fatalf("allows %s: %v %v, want %v", c.uid, got, err, c.want)
			}
		})
	}
	b.fail = true
	if _, err := h.Allows(ctx, "yes"); !errors.Is(err, errBackend) {
		t.Fatalf("allows error: %v", err)
	}
}
