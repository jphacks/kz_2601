package scope

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

var errRead = errors.New("read failed")

// readBackend can fail the app reads or hand back a version of another app.
type readBackend struct {
	*store.Memory
	failOwner, failApp bool
	versionApp         string // non-empty: GetVersion reports this appId
}

func (b *readBackend) AppOwner(ctx context.Context, appID string) (string, error) {
	if b.failOwner {
		return "", errRead
	}
	return b.Memory.AppOwner(ctx, appID)
}

func (b *readBackend) GetApp(ctx context.Context, uid, appID string) (*store.App, error) {
	if b.failApp {
		return nil, errRead
	}
	return b.Memory.GetApp(ctx, uid, appID)
}

func (b *readBackend) GetVersion(ctx context.Context, uid, appID string, n int) (*store.Version, error) {
	v, err := b.Memory.GetVersion(ctx, uid, appID, n)
	if err == nil && b.versionApp != "" {
		v.AppID = b.versionApp
	}
	return v, err
}

func readHandle(t *testing.T, b Backend, ev store.OutboxEvent) *Handle {
	t.Helper()
	h, err := New(b, "test", ev, nil)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

var (
	sharedS  = store.OutboxEvent{AppID: "S", Scope: "shared"}
	privateP = store.OutboxEvent{AppID: "P", Scope: "private", UID: "u"}
)

// readSeed: S is a shared app created by "creator" (versions 1-2,
// published), P is u's private app (versions 1-2); "orphan" has an owner
// whose app is gone.
func readSeed(t *testing.T) *readBackend {
	t.Helper()
	ctx := context.Background()
	b := &readBackend{Memory: store.NewMemory()}
	for _, x := range []struct{ uid, app string }{{"creator", "S"}, {"u", "P"}, {"u", "S"}} {
		if _, _, err := b.EnsureApp(ctx, x.uid, x.app, json.RawMessage(`{"v":1}`), x.app); err != nil {
			t.Fatal(err)
		}
	}
	for _, x := range []struct{ uid, app string }{{"creator", "S"}, {"u", "P"}} {
		if _, err := b.CommitVersion(ctx, x.uid, x.app, store.Commit{BaseVersion: 1, DSL: json.RawMessage(`{"v":2}`)}); err != nil {
			t.Fatal(err)
		}
	}
	for _, x := range []struct{ uid, app string }{{"creator", "S"}, {"gone", "orphan"}} {
		if _, err := b.ClaimApp(ctx, x.uid, x.app); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := b.PublishApp(ctx, "creator", "S", json.RawMessage(`{"v":2}`), "Shared", 2); err != nil {
		t.Fatal(err)
	}
	return b
}

// AppID / Shared: what New bound the handle to; a private event needs a uid.
func TestHandleAppIDShared(t *testing.T) {
	b := store.NewMemory()
	for _, c := range []struct {
		name   string
		ev     store.OutboxEvent
		shared bool
	}{
		{"shared", store.OutboxEvent{AppID: "S", Scope: "shared"}, true},
		{"public", store.OutboxEvent{AppID: "S", Scope: "public"}, true},
		{"shared ignores uid", store.OutboxEvent{AppID: "S", Scope: "shared", UID: "u"}, true},
		{"private", store.OutboxEvent{AppID: "P", Scope: "private", UID: "u"}, false},
		{"no scope is private", store.OutboxEvent{AppID: "P", UID: "u"}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := readHandle(t, b, c.ev)
			if h.AppID() != c.ev.AppID || h.Shared() != c.shared {
				t.Fatalf("appId %q shared %v", h.AppID(), h.Shared())
			}
		})
	}
	for _, ev := range []store.OutboxEvent{
		{Scope: "shared"},
		{AppID: "P", Scope: "private"},
		{AppID: "P"},
	} {
		if _, err := New(b, "test", ev, nil); err == nil {
			t.Fatalf("accepted %+v", ev)
		}
	}
}

// PublicApp: apps_public of the handle's app only.
func TestHandlePublicApp(t *testing.T) {
	ctx := context.Background()
	b := readSeed(t)
	p, err := readHandle(t, b, sharedS).PublicApp(ctx)
	if err != nil || p.ID != "S" || p.OwnerUID != "creator" || p.Version != 2 || p.Name != "Shared" {
		t.Fatalf("public app: %+v %v", p, err)
	}
	if _, err := readHandle(t, b, store.OutboxEvent{AppID: "unpublished", Scope: "shared"}).PublicApp(ctx); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unpublished: %v", err)
	}
}

// Version: the creator's versions for shared data, the user's own for
// private data; a version of another app is a violation.
func TestHandleVersion(t *testing.T) {
	ctx := context.Background()
	b := readSeed(t)
	for _, c := range []struct {
		name string
		ev   store.OutboxEvent
		n    int
		want string
		err  error
	}{
		{"shared: the creator's", sharedS, 2, `{"v":2}`, nil},
		{"shared: older", sharedS, 1, `{"v":1}`, nil},
		{"private: the user's own", privateP, 2, `{"v":2}`, nil},
		{"private: not the creator's", store.OutboxEvent{AppID: "S", Scope: "private", UID: "u"}, 2, "", store.ErrNotFound},
		{"missing version", sharedS, 9, "", store.ErrNotFound},
		{"shared without owner", store.OutboxEvent{AppID: "nobody", Scope: "shared"}, 1, "", store.ErrNotFound},
		{"owner's app gone", store.OutboxEvent{AppID: "orphan", Scope: "shared"}, 1, "", store.ErrNotFound},
	} {
		t.Run(c.name, func(t *testing.T) {
			v, err := readHandle(t, b, c.ev).Version(ctx, c.n)
			if c.err != nil {
				if !errors.Is(err, c.err) {
					t.Fatalf("err %v, want %v", err, c.err)
				}
				return
			}
			if err != nil || v.Version != c.n || string(v.DSL) != c.want {
				t.Fatalf("version: %+v %v", v, err)
			}
		})
	}

	b.versionApp = "elsewhere"
	for _, ev := range []store.OutboxEvent{sharedS, privateP} {
		if _, err := readHandle(t, b, ev).Version(ctx, 1); !errors.Is(err, ErrViolation) {
			t.Fatalf("%s: version of another app: %v", ev.Scope, err)
		}
	}
	b.versionApp = ""
	b.failOwner = true
	if _, err := readHandle(t, b, sharedS).Version(ctx, 1); !errors.Is(err, errRead) {
		t.Fatalf("owner error: %v", err)
	}
}

// RunnerApp: runner/{env}/apps/{TenantKey(appId, uid)}, uid "" for shared data.
func TestHandleRunnerApp(t *testing.T) {
	ctx := context.Background()
	b := store.NewMemory()
	at := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	for _, x := range []struct {
		env, key, tz string
		at           time.Time
	}{
		{"test", store.TenantKey("S", ""), "Asia/Tokyo", at},
		{"test", store.TenantKey("P", "u"), "Europe/Paris", at.Add(time.Hour)},
		{"other", store.TenantKey("S", ""), "UTC", at.Add(2 * time.Hour)},
	} {
		if err := b.TouchAppOpened(ctx, x.env, x.key, x.at); err != nil {
			t.Fatal(err)
		}
		if err := b.SetAppTimeZone(ctx, x.env, x.key, x.tz, 1); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		name string
		ev   store.OutboxEvent
		tz   string
		at   time.Time
	}{
		{"shared", sharedS, "Asia/Tokyo", at},
		{"shared ignores uid", store.OutboxEvent{AppID: "S", Scope: "shared", UID: "u"}, "Asia/Tokyo", at},
		{"private", privateP, "Europe/Paris", at.Add(time.Hour)},
		{"private side of a shared app", store.OutboxEvent{AppID: "S", Scope: "private", UID: "u"}, "", time.Time{}},
		{"never opened", store.OutboxEvent{AppID: "new", Scope: "shared"}, "", time.Time{}},
	} {
		t.Run(c.name, func(t *testing.T) {
			ra, err := readHandle(t, b, c.ev).RunnerApp(ctx)
			if err != nil || ra.TimeZone != c.tz || !ra.LastOpenedAt.Equal(c.at) {
				t.Fatalf("runner app: %+v %v", ra, err)
			}
		})
	}
}

// CreatedAt: when the creator's app was created; zero when there is no
// owner or the owner's app is gone.
func TestHandleCreatedAt(t *testing.T) {
	ctx := context.Background()
	b := readSeed(t)
	created := func(uid, appID string) time.Time {
		t.Helper()
		a, err := b.Memory.GetApp(ctx, uid, appID)
		if err != nil {
			t.Fatal(err)
		}
		return a.CreatedAt
	}
	for _, c := range []struct {
		name string
		ev   store.OutboxEvent
		want time.Time
	}{
		{"shared", sharedS, created("creator", "S")},
		{"private", privateP, created("u", "P")},
		{"private copy of a shared app", store.OutboxEvent{AppID: "S", Scope: "private", UID: "u"}, created("u", "S")},
		{"no owner", store.OutboxEvent{AppID: "nobody", Scope: "shared"}, time.Time{}},
		{"owner's app gone", store.OutboxEvent{AppID: "orphan", Scope: "shared"}, time.Time{}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := readHandle(t, b, c.ev).CreatedAt(ctx)
			if err != nil || !got.Equal(c.want) {
				t.Fatalf("created %v %v, want %v", got, err, c.want)
			}
		})
	}
	if created("creator", "S").IsZero() {
		t.Fatal("seeded app without createdAt")
	}

	b.failApp = true
	if _, err := readHandle(t, b, sharedS).CreatedAt(ctx); !errors.Is(err, errRead) {
		t.Fatalf("app error: %v", err)
	}
	b.failApp, b.failOwner = false, true
	if _, err := readHandle(t, b, sharedS).CreatedAt(ctx); !errors.Is(err, errRead) {
		t.Fatalf("owner error: %v", err)
	}
}
