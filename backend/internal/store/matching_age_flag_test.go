package store

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestProfilesToMark(t *testing.T) {
	recs := []Record{
		{ID: "a1", Author: "a", OwnerAgeConfirmed: true},
		{ID: "b1", Author: "b"},
		{ID: "b2", Author: "b"},
		{ID: "c1", Author: "c"},
		{ID: "x1"}, // no author: never marked
	}
	if got := unmarkedAuthors(recs); !reflect.DeepEqual(got, []string{"b", "c"}) {
		t.Fatalf("unmarked authors: %v", got)
	}
	var c ProfileAgeBackfill
	ids := profilesToMark(recs, map[string]bool{"a": true, "b": true}, &c)
	if !reflect.DeepEqual(ids, []string{"b1", "b2"}) {
		t.Fatalf("to mark: %v", ids)
	}
	if c != (ProfileAgeBackfill{Marked: 1, ToMark: 2, Undeclared: 2}) {
		t.Fatalf("counts: %+v", c)
	}
}

func TestMemoryProfileAgeFlag(t *testing.T) { testProfileAgeFlag(t, NewMemory(), "") }

// Runs against the emulator only (FIRESTORE_EMULATOR_HOST=...); never staging/prod.
func TestFirestoreProfileAgeFlag(t *testing.T) {
	f := emulatorFirestore(t)
	sfx := time.Now().Format("150405.000000")
	testProfileAgeFlag(t, f, sfx)
	testProfileAgeBackfill(t, f, sfx)
}

func emulatorFirestore(t *testing.T) *Firestore {
	t.Helper()
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	f, err := NewFirestore(context.Background(), "demo-kz2601", "(default)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// publishMatching publishes testdata/matching.json as app with owner and
// opens it for the other uids.
func publishMatching(t *testing.T, s Store, app, owner string, others ...string) {
	t.Helper()
	ctx := context.Background()
	raw, err := os.ReadFile("../../pkg/dsl/testdata/matching.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range append([]string{owner}, others...) {
		if _, _, err := s.EnsureApp(ctx, u, app, raw, "x"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.ClaimApp(ctx, owner, app); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.PublishApp(ctx, owner, app, raw, "x", 1); err != nil || !ok {
		t.Fatalf("publish: %v %v", ok, err)
	}
}

func postAs(t *testing.T, s Store, app, entity, id, uid string, flag bool) {
	t.Helper()
	rec := Record{ID: id, Entity: entity, Author: uid, AuthorName: "n", Data: map[string]any{}, OwnerAgeConfirmed: flag}
	if _, _, err := s.CreateRecord(context.Background(), PublicRecords(app, entity), rec); err != nil {
		t.Fatal(err)
	}
}

func flagOf(t *testing.T, s Store, app, entity, id string) bool {
	t.Helper()
	rec, err := s.GetRecord(context.Background(), PublicRecords(app, entity), id)
	if err != nil {
		t.Fatal(err)
	}
	return rec.OwnerAgeConfirmed
}

// ConfirmAge flags the person's profile records (only those), and
// AgeConfirmedAmong answers for many people at once.
func testProfileAgeFlag(t *testing.T, s Store, sfx string) {
	ctx := context.Background()
	app, owner, dave := "MAGE"+sfx, "mageowner"+sfx, "magedave"+sfx
	publishMatching(t, s, app, owner, dave)
	postAs(t, s, app, "profile", "p-d", dave, false)
	postAs(t, s, app, "contact", "c-d", dave, false)
	postAs(t, s, app, "profile", "p-o", owner, false)
	if flagOf(t, s, app, "profile", "p-o") {
		t.Fatal("a record keeps the flag it was written with")
	}

	if got, err := s.AgeConfirmedAmong(ctx, []string{dave, owner}); err != nil || len(got) != 0 {
		t.Fatalf("nobody declared: %v %v", got, err)
	}
	if err := s.ConfirmAge(ctx, dave); err != nil {
		t.Fatal(err)
	}
	if got, err := s.AgeConfirmedAmong(ctx, []string{dave, owner}); err != nil || !reflect.DeepEqual(got, map[string]bool{dave: true}) {
		t.Fatalf("dave declared: %v %v", got, err)
	}
	if !flagOf(t, s, app, "profile", "p-d") {
		t.Error("dave's profile is flagged")
	}
	if flagOf(t, s, app, "contact", "c-d") || flagOf(t, s, app, "profile", "p-o") {
		t.Error("only dave's profile records are flagged")
	}
	postAs(t, s, app, "profile", "p-n", dave, true)
	if !flagOf(t, s, app, "profile", "p-n") {
		t.Error("a flag written with the record is kept")
	}
}

// The backfill flags profiles of declared authors only. Other tests' data
// share the emulator, so it checks the records it made rather than totals.
func testProfileAgeBackfill(t *testing.T, f *Firestore, sfx string) {
	ctx := context.Background()
	app, owner, yes, no := "MBACK"+sfx, "mbowner"+sfx, "mbyes"+sfx, "mbno"+sfx
	publishMatching(t, f, app, owner, yes, no)
	postAs(t, f, app, "profile", "p-y", yes, false)
	postAs(t, f, app, "profile", "p-n", no, false)
	postAs(t, f, app, "contact", "c-y", yes, false)
	// declared before the flag existed: only the user document
	if _, err := f.userRef(yes).Set(ctx, map[string]any{"ageConfirmedAt": Now()}); err != nil {
		t.Fatal(err)
	}

	dry, err := f.BackfillProfileAgeFlags(ctx, true)
	if err != nil || dry.ToMark < 1 || dry.Undeclared < 1 {
		t.Fatalf("dry run: %+v %v", dry, err)
	}
	if flagOf(t, f, app, "profile", "p-y") {
		t.Fatal("a dry run writes nothing")
	}
	if _, err := f.BackfillProfileAgeFlags(ctx, false); err != nil {
		t.Fatal(err)
	}
	if !flagOf(t, f, app, "profile", "p-y") {
		t.Error("declared author's profile is flagged")
	}
	if flagOf(t, f, app, "profile", "p-n") || flagOf(t, f, app, "contact", "c-y") {
		t.Error("undeclared profile and non-profile records stay unflagged")
	}
	// again: the same result (totals may move with other packages' data)
	if _, err := f.BackfillProfileAgeFlags(ctx, false); err != nil {
		t.Fatal(err)
	}
	if !flagOf(t, f, app, "profile", "p-y") || flagOf(t, f, app, "profile", "p-n") {
		t.Error("a second run changes nothing")
	}
}
