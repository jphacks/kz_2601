package scope

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

// stub counts owner reads and serves versions of any appId.
type stub struct {
	*store.Memory
	ownerReads int
	versionApp string
	versionUID string
}

func (s *stub) AppOwner(ctx context.Context, appID string) (string, error) {
	s.ownerReads++
	return s.Memory.AppOwner(ctx, appID)
}

func (s *stub) GetVersion(_ context.Context, uid, _ string, n int) (*store.Version, error) {
	s.versionUID = uid
	return &store.Version{AppID: s.versionApp, Version: n}, nil
}

func shared(t *testing.T, b Backend, appID string) *Handle {
	t.Helper()
	h, err := New(b, "test", store.OutboxEvent{AppID: appID, Scope: "shared"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// Owner is read once per handle; a missing owner is "" (Version: not found),
// and the rules of shared data come from the creator's versions of this app.
func TestHandleOwnerAndVersion(t *testing.T) {
	ctx := context.Background()
	s := &stub{Memory: store.NewMemory(), versionApp: "X"}

	h := shared(t, s, "X")
	if o, err := h.Owner(ctx); err != nil || o != "" {
		t.Fatalf("no owner: %q %v", o, err)
	}
	if _, err := h.Version(ctx, 1); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("version without owner: %v", err)
	}
	if at, err := h.CreatedAt(ctx); err != nil || !at.IsZero() {
		t.Fatalf("created without owner: %v %v", at, err)
	}

	if _, err := s.ClaimApp(ctx, "creator", "X"); err != nil {
		t.Fatal(err)
	}
	h = shared(t, s, "X")
	s.ownerReads = 0
	for range 3 {
		if o, err := h.Owner(ctx); err != nil || o != "creator" {
			t.Fatalf("owner: %q %v", o, err)
		}
	}
	if s.ownerReads != 1 {
		t.Fatalf("owner read %d times", s.ownerReads)
	}
	if v, err := h.Version(ctx, 2); err != nil || v.Version != 2 || s.versionUID != "creator" {
		t.Fatalf("version: %+v %v (uid %q)", v, err, s.versionUID)
	}
	// the creator's app is gone: zero time, no error
	if at, err := h.CreatedAt(ctx); err != nil || !at.IsZero() {
		t.Fatalf("created of a gone app: %v %v", at, err)
	}

	// a version of another app is a violation
	s.versionApp = "Y"
	if _, err := h.Version(ctx, 2); !errors.Is(err, ErrViolation) {
		t.Fatalf("version of Y: %v", err)
	}

	// private: the user's own versions
	p, err := New(s, "test", store.OutboxEvent{AppID: "X", Scope: "private", UID: "u"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.versionApp = "X"
	if _, err := p.Version(ctx, 1); err != nil || s.versionUID != "u" {
		t.Fatalf("private version: %v (uid %q)", err, s.versionUID)
	}
}

// Members, Blocks and Allows read only this app's members / the asked uid.
func TestHandleMembersAndAccounts(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	for _, m := range []struct{ app, uid string }{{"X", "m1"}, {"X", "m2"}, {"Y", "m3"}} {
		if _, err := st.AddMember(ctx, m.app, store.Member{UID: m.uid, Via: store.MemberViaShare}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.PutBlock(ctx, "m1", store.Block{UID: "m2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetNotifyConsent(ctx, "m1", "Y", true); err != nil {
		t.Fatal(err)
	}
	h := shared(t, st, "X")

	ms, err := h.Members(ctx)
	if err != nil || strings.Join(sorted(ms), ",") != "m1,m2" {
		t.Fatalf("members: %v %v", ms, err)
	}
	// #2297: one uid at a time, this app only; a private handle has no members
	for uid, want := range map[string]bool{"m1": true, "m2": true, "m3": false, "x": false, "": false, "a/b": false} {
		if ok, err := h.IsMember(ctx, uid); err != nil || ok != want {
			t.Errorf("IsMember(%q) = %v %v, want %v", uid, ok, err, want)
		}
	}
	priv, err := New(st, "test", store.OutboxEvent{AppID: "X", UID: "m1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := priv.IsMember(ctx, "m1"); err != nil || ok {
		t.Errorf("private IsMember: %v %v", ok, err)
	}
	if bs, err := h.Blocks(ctx, "m1"); err != nil || len(bs) != 1 || bs[0] != "m2" {
		t.Fatalf("blocks: %v %v", bs, err)
	}
	if bs, err := h.Blocks(ctx, "m2"); err != nil || len(bs) != 0 {
		t.Fatalf("blocks of m2: %v %v", bs, err)
	}
	// consent for Y does not allow X
	if ok, err := h.Allows(ctx, "m1"); err != nil || ok {
		t.Fatalf("consent of another app: %v %v", ok, err)
	}
	if _, err := st.SetNotifyConsent(ctx, "m1", "X", true); err != nil {
		t.Fatal(err)
	}
	if ok, err := h.Allows(ctx, "m1"); err != nil || !ok {
		t.Fatalf("consent: %v %v", ok, err)
	}
	if gone, err := h.Deleted(ctx, "m1"); err != nil || gone {
		t.Fatalf("deleted: %v %v", gone, err)
	}
}

// PushDevices: iOS only on the asked APNs environment (all when ""),
// Android always; DropDeviceToken removes that token only.
func TestHandlePushDevices(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	tok := func(c string) string { return strings.Repeat(c, 64) }
	for _, d := range []store.Device{
		{InstallationID: "i-dev", Token: tok("a"), Platform: store.PlatformIOS, Environment: "development"},
		{InstallationID: "i-prod", Token: tok("b"), Platform: store.PlatformIOS, Environment: "production"},
		{InstallationID: "android", Token: tok("c"), Platform: store.PlatformAndroid},
	} {
		if _, err := st.PutDevice(ctx, "u", d); err != nil {
			t.Fatal(err)
		}
	}
	h := shared(t, st, "X")
	ids := func(env string) string {
		t.Helper()
		ds, err := h.PushDevices(ctx, "u", env)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, d := range ds {
			out = append(out, d.InstallationID)
		}
		return strings.Join(sorted(out), ",")
	}
	if got := ids("production"); got != "android,i-prod" {
		t.Fatalf("production: %s", got)
	}
	if got := ids(""); got != "android,i-dev,i-prod" {
		t.Fatalf("all: %s", got)
	}
	if err := h.DropDeviceToken(ctx, "u", tok("b")); err != nil {
		t.Fatal(err)
	}
	if got := ids(""); got != "android,i-dev" {
		t.Fatalf("after drop: %s", got)
	}
	if ds, err := h.PushDevices(ctx, "other", ""); err != nil || len(ds) != 0 {
		t.Fatalf("another user's devices: %v %v", ds, err)
	}
}

func sorted(s []string) []string {
	out := slices.Clone(s)
	slices.Sort(out)
	return out
}
