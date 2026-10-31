package store

import (
	"context"
	"errors"
	"os"
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
