package store

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMemoryMembership(t *testing.T) { testMembership(t, NewMemory(), "") }

// Runs against the emulator only (FIRESTORE_EMULATOR_HOST=...).
func TestFirestoreMembership(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	f, err := NewFirestore(context.Background(), "demo-kz2601", "(default)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	testMembership(t, f, time.Now().Format("150405.000000"))
}

func testMembership(t *testing.T, s Store, suffix string) {
	ctx := context.Background()
	app := "FAM" + suffix
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	if _, err := s.GetMember(ctx, app, "a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown member: %v", err)
	}
	if created, err := s.AddMember(ctx, app, Member{UID: "b", Via: MemberViaShare, InvitedBy: "owner", JoinedAt: t0.Add(time.Minute)}); err != nil || !created {
		t.Fatalf("add b: %v %v", created, err)
	}
	if created, err := s.AddMember(ctx, app, Member{UID: "a", DisplayName: "A", Via: MemberViaLegacy, JoinedAt: t0}); err != nil || !created {
		t.Fatalf("add a: %v %v", created, err)
	}
	// re-join keeps JoinedAt / Via, updates a non-empty name only
	if created, err := s.AddMember(ctx, app, Member{UID: "a", DisplayName: "", Via: MemberViaShare, JoinedAt: t0.Add(time.Hour)}); err != nil || created {
		t.Fatalf("re-add a: %v %v", created, err)
	}
	if _, err := s.AddMember(ctx, app, Member{UID: "b", DisplayName: "B"}); err != nil {
		t.Fatal(err)
	}
	a, err := s.GetMember(ctx, app, "a")
	if err != nil || a.DisplayName != "A" || a.Via != MemberViaLegacy || !a.JoinedAt.Equal(t0) {
		t.Fatalf("a: %+v %v", a, err)
	}
	ms, err := s.ListMembers(ctx, app, 10)
	if err != nil || len(ms) != 2 || ms[0].UID != "a" || ms[1].UID != "b" || ms[1].DisplayName != "B" || ms[1].InvitedBy != "owner" {
		t.Fatalf("list: %+v %v", ms, err)
	}
	if ms, _ = s.ListMembers(ctx, app, 1); len(ms) != 1 {
		t.Fatalf("limit: %+v", ms)
	}
	if ms, _ = s.ListMembers(ctx, "OTHER"+suffix, 10); len(ms) != 0 {
		t.Fatalf("other app: %+v", ms)
	}
	if err := s.RemoveMember(ctx, app, "a"); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveMember(ctx, app, "a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("remove twice: %v", err)
	}
	if _, err := s.GetMember(ctx, app, "a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removed: %v", err)
	}
}

// countingGetter counts GetMember calls and can fail them.
type countingGetter struct {
	*Memory
	gets int
	err  error
}

func (c *countingGetter) GetMember(ctx context.Context, appID, uid string) (*Member, error) {
	c.gets++
	if c.err != nil {
		return nil, c.err
	}
	return c.Memory.GetMember(ctx, appID, uid)
}

// #2297: IsMember is one GetMember; a uid that cannot be a document ID is not read.
func TestIsMember(t *testing.T) {
	ctx := context.Background()
	g := &countingGetter{Memory: NewMemory()}
	if _, err := g.AddMember(ctx, "APP", Member{UID: "m", Via: MemberViaShare}); err != nil {
		t.Fatal(err)
	}
	for uid, want := range map[string]bool{"m": true, "x": false} {
		g.gets = 0
		if ok, err := IsMember(ctx, g, "APP", uid); err != nil || ok != want || g.gets != 1 {
			t.Errorf("IsMember(%q) = %v %v after %d reads, want %v after 1", uid, ok, err, g.gets, want)
		}
	}
	if ok, _ := IsMember(ctx, g, "OTHER", "m"); ok {
		t.Error("a member of another app")
	}
	g.gets = 0
	for _, uid := range []string{"", ".", "..", "a/b", "__x__", strings.Repeat("a", 1501)} {
		if ok, err := IsMember(ctx, g, "APP", uid); ok || err != nil {
			t.Errorf("IsMember(%q) = %v %v", uid, ok, err)
		}
	}
	if g.gets != 0 {
		t.Errorf("invalid uids read %d times", g.gets)
	}
	g.err = errors.New("boom")
	if _, err := IsMember(ctx, g, "APP", "m"); err == nil {
		t.Error("a read error is returned")
	}
}
