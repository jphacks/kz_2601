package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestMemoryDeleteUserData(t *testing.T) {
	testDeleteUserData(t, NewMemory(), "")
}

// Runs against the emulator only (make emulator; FIRESTORE_EMULATOR_HOST=localhost:8089).
func TestFirestoreDeleteUserData(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	f, err := NewFirestore(context.Background(), "demo-kz2601")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	testDeleteUserData(t, f, time.Now().Format("150405.000000"))
}

func testDeleteUserData(t *testing.T, s Store, suffix string) {
	ctx := context.Background()
	leaver, stayer := "leaver"+suffix, "stayer"+suffix
	mine, board, theirs := "MINE"+suffix, "BOARD"+suffix, "THEIRS"+suffix
	initial, _ := json.Marshal(dsl.Initial())
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	for _, a := range []struct{ uid, app string }{{leaver, mine}, {leaver, board}, {stayer, board}, {stayer, theirs}} {
		_, _, err := s.EnsureApp(ctx, a.uid, a.app, initial, "x")
		must(err)
	}
	_, err := s.CommitVersion(ctx, leaver, mine, Commit{BaseVersion: 1, DSL: initial, Source: &Source{Kind: "ai", Transcript: "家計簿"}})
	must(err)
	_, _, err = s.CreateRecord(ctx, PrivateRecords(leaver, mine, "expense"), Record{ID: "r1", Data: map[string]any{"amount": 1}})
	must(err)
	_, err = s.PublishApp(ctx, leaver, mine, initial, "mine", 2)
	must(err)
	_, err = s.PublishApp(ctx, stayer, board, initial, "board", 1)
	must(err)
	pub := PublicRecords(board, "post")
	_, _, err = s.CreateRecord(ctx, pub, Record{ID: "p-leaver" + suffix, Author: leaver, Data: map[string]any{}})
	must(err)
	_, _, err = s.CreateRecord(ctx, pub, Record{ID: "p-stayer" + suffix, Author: stayer, Data: map[string]any{}})
	must(err)
	must(s.CreateShare(ctx, Share{Token: "tok-leaver" + suffix, AppID: mine, OwnerUID: leaver, DSL: initial}))
	must(s.CreateShare(ctx, Share{Token: "tok-stayer" + suffix, AppID: theirs, OwnerUID: stayer, DSL: initial}))
	_, err = s.PutCatalogApp(ctx, leaver, CatalogApp{ID: mine, DSL: initial})
	must(err)
	_, err = s.PutCatalogApp(ctx, stayer, CatalogApp{ID: theirs, DSL: initial})
	must(err)

	must(s.DeleteUserData(ctx, leaver))
	must(s.DeleteUserData(ctx, leaver)) // idempotent

	gone := func(what string, err error) {
		t.Helper()
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s must be deleted, got %v", what, err)
		}
	}
	kept := func(what string, err error) {
		t.Helper()
		if err != nil {
			t.Errorf("%s must be kept, got %v", what, err)
		}
	}
	apps, err := s.ListApps(ctx, leaver, 0, 0)
	if err != nil || len(apps) != 0 {
		t.Errorf("leaver apps: %v %v", apps, err)
	}
	_, err = s.GetVersion(ctx, leaver, mine, 2)
	gone("version", err)
	_, err = s.GetRecord(ctx, PrivateRecords(leaver, mine, "expense"), "r1")
	gone("private record", err)
	_, err = s.GetPublicApp(ctx, mine)
	gone("apps_public owned by leaver", err)
	_, err = s.GetRecord(ctx, pub, "p-leaver"+suffix)
	gone("leaver's shared post", err)
	_, err = s.GetShare(ctx, "tok-leaver"+suffix)
	gone("share", err)
	_, err = s.GetCatalogApp(ctx, mine)
	gone("catalog listing", err)

	_, err = s.GetApp(ctx, stayer, board)
	kept("stayer app", err)
	_, err = s.GetPublicApp(ctx, board)
	kept("apps_public owned by stayer", err)
	_, err = s.GetRecord(ctx, pub, "p-stayer"+suffix)
	kept("stayer's shared post", err)
	_, err = s.GetShare(ctx, "tok-stayer"+suffix)
	kept("stayer share", err)
	_, err = s.GetCatalogApp(ctx, theirs)
	kept("stayer listing", err)
}
