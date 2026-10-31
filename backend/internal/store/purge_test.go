package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"cloud.google.com/go/firestore"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// purgeFixture builds an app for owner with versions, private and shared
// records, two shares, a catalog listing with an install, plus a share and a
// copy of other's that must survive.
func purgeFixture(t *testing.T, s Store, owner, other, appID string) {
	t.Helper()
	ctx := context.Background()
	initial, _ := json.Marshal(dsl.Initial())
	if _, _, err := s.EnsureApp(ctx, owner, appID, initial, "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CommitVersion(ctx, owner, appID, Commit{BaseVersion: 1, DSL: initial, Name: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.EnsureApp(ctx, other, appID, initial, "x"); err != nil { // a joined member's copy
		t.Fatal(err)
	}
	for _, rs := range []RecordSet{PrivateRecords(owner, appID, "e1"), PrivateRecords(owner, appID, "e2"), {AppID: appID, Entity: "board", Public: true}} {
		if _, _, err := s.CreateRecord(ctx, rs, Record{ID: NewID("r_"), Entity: rs.Entity, Data: map[string]any{"v": 1}, Author: owner}); err != nil {
			t.Fatal(err)
		}
	}
	if ok, err := s.PublishApp(ctx, owner, appID, initial, "x", 2); err != nil || !ok {
		t.Fatalf("publish mirror: %v %v", ok, err)
	}
	now := time.Now().UTC()
	for _, tok := range []string{appID + "-t1", appID + "-t2"} {
		if err := s.CreateShare(ctx, Share{Token: tok, AppID: appID, OwnerUID: owner, Name: "x", DSL: initial, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CreateShare(ctx, Share{Token: appID + "-other", AppID: appID, OwnerUID: other, Name: "x", DSL: initial, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutCatalogApp(ctx, owner, CatalogApp{ID: appID, Name: "x", DSL: initial}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordInstall(ctx, appID, "installer", "copy-"+appID, true); err != nil {
		t.Fatal(err)
	}
}

func checkPurged(t *testing.T, s Store, owner, other, appID string) {
	t.Helper()
	ctx := context.Background()
	res, err := s.PurgeApp(ctx, owner, appID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Shares != 2 || !res.Listing || !res.Mirror || res.Versions != 2 || res.Records != 2 {
		t.Fatalf("result: %+v", res)
	}
	if _, err := s.GetApp(ctx, owner, appID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("app: %v", err)
	}
	for _, tok := range []string{appID + "-t1", appID + "-t2"} {
		if _, err := s.GetShare(ctx, tok); !errors.Is(err, ErrNotFound) {
			t.Fatalf("share %s: %v", tok, err)
		}
	}
	if _, err := s.GetShare(ctx, appID+"-other"); err != nil {
		t.Fatalf("another uid's share must stay: %v", err)
	}
	if _, err := s.GetCatalogApp(ctx, appID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("listing: %v", err)
	}
	if _, err := s.GetInstall(ctx, appID, "installer"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("install: %v", err)
	}
	if _, err := s.GetPublicApp(ctx, appID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("mirror: %v", err)
	}
	if _, err := s.GetApp(ctx, other, appID); err != nil {
		t.Fatalf("member's copy must stay: %v", err)
	}
	if _, err := s.PurgeApp(ctx, owner, appID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("purge again: %v", err)
	}
}

func TestMemoryPurgeApp(t *testing.T) {
	m := NewMemory()
	purgeFixture(t, m, "owner", "member", "app1")
	checkPurged(t, m, "owner", "member", "app1")
}

// Runs against the local emulator only (make emulator); never staging/prod.
func TestFirestorePurgeApp(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	ctx := context.Background()
	f, err := NewFirestore(ctx, "demo-kz2601", firestore.DefaultDatabaseID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	sfx := strconv.FormatInt(time.Now().UnixNano(), 36)
	owner, member, appID := "owner-"+sfx, "member-"+sfx, "purge-"+sfx
	purgeFixture(t, f, owner, member, appID)
	checkPurged(t, f, owner, member, appID)

	// no orphaned subcollection documents are left behind
	empty := func(name string, q *firestore.CollectionRef) {
		t.Helper()
		docs, err := q.DocumentRefs(ctx).GetAll()
		if err != nil {
			t.Fatal(err)
		}
		if len(docs) != 0 {
			t.Fatalf("%s: %d documents left", name, len(docs))
		}
	}
	app := f.appRef(owner, appID)
	empty("versions", app.Collection("versions"))
	empty("entities", app.Collection("entities"))
	empty("records e1", app.Collection("entities").Doc("e1").Collection("records"))
	empty("records e2", app.Collection("entities").Doc("e2").Collection("records"))
	empty("public records", f.publicRef(appID).Collection("entities").Doc("board").Collection("records"))
	empty("installs", f.catalogRef(appID).Collection("installs"))
}
