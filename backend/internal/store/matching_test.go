package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestMemoryMatching(t *testing.T) { testMatching(t, NewMemory(), "") }

// Runs against the emulator only (FIRESTORE_EMULATOR_HOST=...); never staging/prod.
func TestFirestoreMatching(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	f, err := NewFirestore(context.Background(), "demo-kz2601", "(default)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	testMatching(t, f, time.Now().Format("150405.000000"))
}

func testMatching(t *testing.T, s Store, sfx string) {
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	app, a, b, c := "MATCH"+sfx, "ma"+sfx, "mb"+sfx, "mc"+sfx
	if MatchPairID(a, b) != MatchPairID(b, a) {
		t.Fatal("pair id must not depend on order")
	}

	// absent pair: fn sees nil; returning nil writes nothing
	p, err := s.ChangeMatchPair(ctx, app, a, b, func(cur *MatchPair) (*MatchPair, error) {
		if cur != nil {
			t.Fatalf("absent pair: %+v", cur)
		}
		return nil, nil
	})
	if err != nil || p != nil {
		t.Fatalf("no write: %+v %v", p, err)
	}
	p, err = s.ChangeMatchPair(ctx, app, a, b, func(*MatchPair) (*MatchPair, error) {
		return &MatchPair{From: a, Status: MatchPending, Profiles: map[string]string{a: "pa", b: "pb"}}, nil
	})
	must(err)
	if p.From != a || p.Status != MatchPending || p.UIDs != SortedPair(a, b) || p.Profiles[b] != "pb" || p.CreatedAt.IsZero() {
		t.Fatalf("created: %+v", p)
	}
	// b accepts from its side (b, a order)
	p, err = s.ChangeMatchPair(ctx, app, b, a, func(cur *MatchPair) (*MatchPair, error) {
		if cur == nil || cur.Status != MatchPending {
			t.Fatalf("cur: %+v", cur)
		}
		cur.Status, cur.MatchedAt = MatchMatched, time.Now().UTC()
		return cur, nil
	})
	must(err)
	if p.Status != MatchMatched || p.MatchedAt.IsZero() || p.Other(b) != a {
		t.Fatalf("matched: %+v", p)
	}
	errBoom := errors.New("boom")
	if _, err := s.ChangeMatchPair(ctx, app, a, b, func(*MatchPair) (*MatchPair, error) { return nil, errBoom }); !errors.Is(err, errBoom) {
		t.Fatalf("fn error: %v", err)
	}
	_, err = s.ChangeMatchPair(ctx, app, c, a, func(*MatchPair) (*MatchPair, error) {
		return &MatchPair{From: c, Status: MatchPending, Profiles: map[string]string{c: "pc", a: "pa"}}, nil
	})
	must(err)

	ps, err := s.ListMatchPairs(ctx, app, a)
	must(err)
	if len(ps) != 2 || ps[0].From != c {
		t.Fatalf("a's pairs (newest first): %+v", ps)
	}
	if ps, _ = s.ListMatchPairs(ctx, app, b); len(ps) != 1 || ps[0].Status != MatchMatched {
		t.Fatalf("b's pairs: %+v", ps)
	}
	if ps, _ = s.ListMatchPairs(ctx, "OTHER"+sfx, a); len(ps) != 0 {
		t.Fatalf("other app: %+v", ps)
	}

	// quota
	must(s.TakeMatchQuota(ctx, a, "d1", 2))
	must(s.TakeMatchQuota(ctx, a, "d1", 2))
	if err := s.TakeMatchQuota(ctx, a, "d1", 2); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("quota: %v", err)
	}
	must(s.TakeMatchQuota(ctx, a, "d2", 2))

	// blocks both ways
	must(s.PutBlock(ctx, a, Block{UID: b}))
	must(s.PutBlock(ctx, c, Block{UID: a}))
	got, err := s.BlockedEither(ctx, a, []string{b, c, "nobody" + sfx})
	must(err)
	if !got[b] || !got[c] || len(got) != 2 {
		t.Fatalf("blocked either: %v", got)
	}
	if got, _ = s.BlockedEither(ctx, b, []string{a}); !got[a] {
		t.Fatalf("b sees a as blocked: %v", got)
	}
}

func TestMemoryPurgeMatching(t *testing.T) { testPurgeMatching(t, NewMemory(), "") }

// Runs against the emulator only (FIRESTORE_EMULATOR_HOST=...); never staging/prod.
func TestFirestorePurgeMatching(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	f, err := NewFirestore(context.Background(), "demo-kz2601", "(default)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	testPurgeMatching(t, f, time.Now().Format("150405.000000"))
}

// The leaver's pairs, profile and contact go; the stayer's stay.
func testPurgeMatching(t *testing.T, s Store, sfx string) {
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile("../../pkg/dsl/testdata/matching.json")
	must(err)
	app, leaver, stayer, third := "MPURGE"+sfx, "mleaver"+sfx, "mstayer"+sfx, "mthird"+sfx
	for _, u := range []string{leaver, stayer} {
		_, _, err := s.EnsureApp(ctx, u, app, raw, "x")
		must(err)
	}
	_, err = s.ClaimApp(ctx, stayer, app)
	must(err)
	if ok, err := s.PublishApp(ctx, stayer, app, raw, "x", 1); err != nil || !ok {
		t.Fatalf("publish: %v %v", ok, err)
	}
	pair := func(a, b string) {
		_, err := s.ChangeMatchPair(ctx, app, a, b, func(*MatchPair) (*MatchPair, error) {
			return &MatchPair{From: a, Status: MatchMatched, Profiles: map[string]string{}}, nil
		})
		must(err)
	}
	pair(leaver, stayer)
	pair(stayer, third)
	post := func(entity, id, uid string) {
		_, _, err := s.CreateRecord(ctx, PublicRecords(app, entity), Record{ID: id, Entity: entity, Author: uid, AuthorName: "n", Data: map[string]any{}})
		must(err)
	}
	post("contact", "c-l", leaver)
	post("contact", "c-s", stayer)
	post("profile", "p-l", leaver)
	must(s.TakeMatchQuota(ctx, leaver, "d1", 0))
	must(s.DeactivateUser(ctx, leaver))

	res, err := s.PurgeAccount(ctx, leaver)
	must(err)
	if res.MatchPairs != 1 || res.MatchRecords != 2 || res.PostsScrubbed != 0 {
		t.Fatalf("result: %+v", res)
	}
	if _, err := s.GetRecord(ctx, PublicRecords(app, "contact"), "c-l"); !errors.Is(err, ErrNotFound) {
		t.Errorf("leaver contact: %v", err)
	}
	if _, err := s.GetRecord(ctx, PublicRecords(app, "profile"), "p-l"); !errors.Is(err, ErrNotFound) {
		t.Errorf("leaver profile: %v", err)
	}
	if _, err := s.GetRecord(ctx, PublicRecords(app, "contact"), "c-s"); err != nil {
		t.Errorf("stayer contact: %v", err)
	}
	if ps, _ := s.ListMatchPairs(ctx, app, stayer); len(ps) != 1 || ps[0].Other(stayer) != third {
		t.Errorf("stayer pairs: %+v", ps)
	}
	if again, err := s.PurgeAccount(ctx, leaver); err != nil || again.MatchPairs != 0 || again.MatchRecords != 0 {
		t.Errorf("second run: %+v %v", again, err)
	}
}
