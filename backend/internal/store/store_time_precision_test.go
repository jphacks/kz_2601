package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// sameTime fails unless the time returned by a write equals the one read back.
// No rounding on either side: Firestore keeps microseconds, so a write that
// returned nanoseconds would not match its own read.
func sameTime(t *testing.T, what string, wrote, read time.Time) {
	t.Helper()
	if !wrote.Equal(read) {
		t.Errorf("%s: write returned %s, read returned %s", what, wrote.Format(time.RFC3339Nano), read.Format(time.RFC3339Nano))
	}
}

func TestStoreAppTimesMatchReadBack(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		uid, appID := "tp-owner"+sfx, "TPAPP"+sfx
		dsl := json.RawMessage(`{"initialScreen":"home"}`)

		app, created, err := s.EnsureApp(ctx, uid, appID, dsl, "N")
		if err != nil || !created {
			t.Fatalf("ensure: %+v %v %v", app, created, err)
		}
		got, err := s.GetApp(ctx, uid, appID)
		if err != nil {
			t.Fatal(err)
		}
		sameTime(t, "app createdAt", app.CreatedAt, got.CreatedAt)
		sameTime(t, "app updatedAt", app.UpdatedAt, got.UpdatedAt)

		v, err := s.CommitVersion(ctx, uid, appID, Commit{BaseVersion: 1, DSL: dsl})
		if err != nil {
			t.Fatal(err)
		}
		gotV, err := s.GetVersion(ctx, uid, appID, v.Version)
		if err != nil {
			t.Fatal(err)
		}
		sameTime(t, "version createdAt", v.CreatedAt, gotV.CreatedAt)
	})
}

func TestStoreCatalogReleaseFeedbackMatchTimesMatchReadBack(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		owner, id := "tp-cat"+sfx, "TPCAT"+sfx

		c, err := s.PutCatalogApp(ctx, owner, CatalogApp{ID: id, Name: "x", DSL: json.RawMessage(`{}`)})
		if err != nil {
			t.Fatal(err)
		}
		gotC, err := s.GetCatalogApp(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		sameTime(t, "catalog createdAt", c.CreatedAt, gotC.CreatedAt)
		sameTime(t, "catalog updatedAt", c.UpdatedAt, gotC.UpdatedAt)

		rr, _, err := s.CreateReleaseRequest(ctx, ReleaseRequest{AppID: id, OwnerUID: owner, Name: "N", Version: 1,
			DSL: json.RawMessage(`{}`), Purpose: "p", Contact: "c@example.com"})
		if err != nil {
			t.Fatal(err)
		}
		rrs, err := s.ListReleaseRequests(ctx, owner, id)
		if err != nil || len(rrs) != 1 {
			t.Fatalf("list releases: %+v %v", rrs, err)
		}
		sameTime(t, "release createdAt", rr.CreatedAt, rrs[0].CreatedAt)
		sameTime(t, "release updatedAt", rr.UpdatedAt, rrs[0].UpdatedAt)

		fb, err := s.CreateFeedback(ctx, Feedback{UID: owner, Kind: FeedbackBug, Message: "m", ConsentVersion: "v1"}, 0)
		if err != nil {
			t.Fatal(err)
		}
		gotFB, err := s.(feedbackGetter).GetFeedback(ctx, fb.ID)
		if err != nil {
			t.Fatal(err)
		}
		sameTime(t, "feedback createdAt", fb.CreatedAt, gotFB.CreatedAt)

		// the caller may hand in a nanosecond matchedAt (the handlers used time.Now)
		a, b := "tp-ma"+sfx, "tp-mb"+sfx
		p, err := s.ChangeMatchPair(ctx, id, a, b, func(*MatchPair) (*MatchPair, error) {
			return &MatchPair{From: a, Status: MatchMatched, Profiles: map[string]string{a: "pa", b: "pb"},
				MatchedAt: time.Date(2026, 11, 6, 1, 2, 3, 123456789, time.UTC)}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		ps, err := s.ListMatchPairs(ctx, id, a)
		if err != nil || len(ps) != 1 {
			t.Fatalf("list pairs: %+v %v", ps, err)
		}
		sameTime(t, "match createdAt", p.CreatedAt, ps[0].CreatedAt)
		sameTime(t, "match updatedAt", p.UpdatedAt, ps[0].UpdatedAt)
		sameTime(t, "match matchedAt", p.MatchedAt, ps[0].MatchedAt)
	})
}
