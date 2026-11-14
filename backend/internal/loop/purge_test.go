package loop

import (
	"context"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

type existing map[string]bool

func (e existing) FeedbackExists(_ context.Context, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range ids {
		if e[id] {
			out[id] = true
		}
	}
	return out, nil
}

type deletedUsers map[string]bool

func (d deletedUsers) UserDeleted(_ context.Context, uid string) (bool, error) { return d[uid], nil }

func TestSweep(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	s := NewMemory()
	put := func(sid, uid string, src Source, exp time.Time) string {
		it := Item{ID: ItemID(src, sid), Source: src, SourceID: sid, UID: uid, Status: ItemNew, ExpiresAt: exp}
		if _, err := s.PutItem(ctx, it); err != nil {
			t.Fatal(err)
		}
		return it.ID
	}
	live := put("fb_live", "u1", SourceInApp, now.Add(time.Hour))
	orphan := put("fb_gone", "u1", SourceInApp, now.Add(time.Hour))
	expired := put("fb_old", "u1", SourceInApp, now.Add(-time.Hour))
	review := put("r1", "", SourceStoreReview, now.Add(time.Hour))
	leaver := put("fb_leaver", "u2", SourceInApp, now.Add(time.Hour))
	_, _ = s.PutNotice(ctx, Notice{ID: NoticeID("d", "u2"), UID: "u2", ExpiresAt: now.Add(time.Hour)})

	src := existing{"fb_live": true, "fb_old": true, "fb_leaver": true}
	users := deletedUsers{"u2": true}
	res, err := Sweep(ctx, s, src, users, now, true)
	if err != nil || res.Expired != 1 || res.Orphans != 1 || res.Deleted != 2 {
		t.Fatalf("dry run %+v %v", res, err)
	}
	if _, err := s.GetItem(ctx, orphan); err != nil {
		t.Fatal("dry run deleted")
	}
	if _, err := Sweep(ctx, s, src, users, now, false); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]bool{live: true, review: true, orphan: false, expired: false, leaver: false} {
		_, err := s.GetItem(ctx, id)
		if (err == nil) != want {
			t.Errorf("%s kept=%v want %v", id, err == nil, want)
		}
	}
	if ns, _ := s.ListNotices(ctx, "u2", 10); len(ns) != 0 {
		t.Fatal("deleted user's notice kept")
	}
}

// Runs against the emulator only (FIRESTORE_EMULATOR_HOST=...); never staging/prod.
func TestFirestoreFeedbackExists(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	ctx := context.Background()
	c, err := firestore.NewClient(ctx, "demo-kz2601")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	sfx := time.Now().Format("150405000000")
	there := "fb_exists" + sfx
	if _, err := c.Collection(colFeedback).Doc(there).Set(ctx, map[string]any{"uid": "u"}); err != nil {
		t.Fatal(err)
	}
	ids := []string{there}
	for i := range 40 { // more than one "in" chunk
		ids = append(ids, "fb_missing"+sfx+string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	got, err := NewFirestore(c).FeedbackExists(ctx, ids)
	if err != nil || len(got) != 1 || !got[there] {
		t.Fatalf("%v %v", got, err)
	}
}
