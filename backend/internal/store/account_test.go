package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestMemoryDeactivateUser(t *testing.T) {
	testDeactivateUser(t, NewMemory(), "")
}

// Runs against the emulator only (FIRESTORE_EMULATOR_HOST=...).
func TestFirestoreDeactivateUser(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	f, err := NewFirestore(context.Background(), "demo-kz2601", "(default)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	testDeactivateUser(t, f, time.Now().Format("150405.000000"))
}

func testDeactivateUser(t *testing.T, s Store, suffix string) {
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
	_, _, err := s.CreateRecord(ctx, PrivateRecords(leaver, mine, "expense"), Record{ID: "r1", Data: map[string]any{"amount": 1}})
	must(err)
	pub := PublicRecords(board, "post")
	_, _, err = s.CreateRecord(ctx, pub, Record{ID: "p-leaver" + suffix, Author: leaver, Data: map[string]any{}})
	must(err)
	must(s.CreateShare(ctx, Share{Token: "tok-leaver" + suffix, AppID: mine, OwnerUID: leaver, DSL: initial}))
	must(s.CreateShare(ctx, Share{Token: "tok-stayer" + suffix, AppID: theirs, OwnerUID: stayer, DSL: initial}))
	_, err = s.PutCatalogApp(ctx, leaver, CatalogApp{ID: mine, DSL: initial})
	must(err)
	_, err = s.PutCatalogApp(ctx, stayer, CatalogApp{ID: theirs, DSL: initial})
	must(err)

	if gone, err := s.UserDeleted(ctx, leaver); err != nil || gone {
		t.Fatalf("before: %v %v", gone, err)
	}
	must(s.DeactivateUser(ctx, leaver))
	must(s.DeactivateUser(ctx, leaver)) // idempotent

	if gone, err := s.UserDeleted(ctx, leaver); err != nil || !gone {
		t.Errorf("leaver must be deleted: %v %v", gone, err)
	}
	if gone, err := s.UserDeleted(ctx, stayer); err != nil || gone {
		t.Errorf("stayer must not be deleted: %v %v", gone, err)
	}
	for _, id := range []string{mine, board} {
		if a, err := s.GetApp(ctx, leaver, id); err != nil || a.Status != "archived" {
			t.Errorf("leaver %s must be archived (kept): %+v %v", id, a, err)
		}
	}
	if _, err := s.GetRecord(ctx, PrivateRecords(leaver, mine, "expense"), "r1"); err != nil {
		t.Errorf("private record is kept (logical delete): %v", err)
	}
	if _, err := s.GetRecord(ctx, pub, "p-leaver"+suffix); err != nil {
		t.Errorf("shared post is kept: %v", err)
	}
	if sh, err := s.GetShare(ctx, "tok-leaver"+suffix); err != nil || !sh.Revoked {
		t.Errorf("leaver share must be revoked: %+v %v", sh, err)
	}
	if _, err := s.GetCatalogApp(ctx, mine); err == nil {
		t.Errorf("leaver listing must be taken down")
	}

	if a, err := s.GetApp(ctx, stayer, board); err != nil || a.Status == "archived" {
		t.Errorf("stayer app: %+v %v", a, err)
	}
	if sh, err := s.GetShare(ctx, "tok-stayer"+suffix); err != nil || sh.Revoked {
		t.Errorf("stayer share: %+v %v", sh, err)
	}
	if _, err := s.GetCatalogApp(ctx, theirs); err != nil {
		t.Errorf("stayer listing: %v", err)
	}
}
