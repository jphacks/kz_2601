package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// A commit with Publish updates apps_public in the same transaction, so the
// mirror's version is always the creator's latest; withdrawing sharing
// updates it too, and moderation fields survive.
func TestStoreCommitPublish(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		app, creator, other := "PUBC"+sfx, "pubc-a"+sfx, "pubc-b"+sfx
		shared := json.RawMessage(`{"v":"shared"}`)
		private := json.RawMessage(`{"v":"private"}`)
		for _, uid := range []string{creator, other} {
			if _, _, err := s.EnsureApp(ctx, uid, app, json.RawMessage(`{}`), "x"); err != nil {
				t.Fatal(err)
			}
		}
		commit := func(uid string, base int, raw json.RawMessage, p *CommitPublish) (*Version, error) {
			return s.CommitVersion(ctx, uid, app, Commit{BaseVersion: base, DSL: raw, Name: "n" + string(raw), Publish: p})
		}
		mirror := func() *PublicApp {
			t.Helper()
			p, err := s.GetPublicApp(ctx, app)
			if err != nil {
				t.Fatalf("GetPublicApp: %v", err)
			}
			return p
		}

		// without Claim nothing is claimed or published
		if _, err := commit(creator, 1, shared, &CommitPublish{Shared: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetPublicApp(ctx, app); !errors.Is(err, ErrNotFound) {
			t.Fatalf("published without a creator: %v", err)
		}
		if _, err := s.AppOwner(ctx, app); !errors.Is(err, ErrNotFound) {
			t.Fatalf("claimed without Claim: %v", err)
		}

		// Claim + Shared: the creator is recorded and the mirror created
		if _, err := commit(creator, 2, shared, &CommitPublish{Shared: true, Claim: true}); err != nil {
			t.Fatal(err)
		}
		if o, err := s.AppOwner(ctx, app); err != nil || o != creator {
			t.Fatalf("owner = %q %v", o, err)
		}
		if p := mirror(); p.Version != 3 || p.OwnerUID != creator || string(p.DSL) != string(shared) {
			t.Fatalf("created mirror: %+v", p)
		}

		// another uid's copy (a participant) never touches the mirror
		if _, err := commit(other, 1, private, &CommitPublish{Claim: true}); err != nil {
			t.Fatal(err)
		}
		if p := mirror(); p.Version != 3 || string(p.DSL) != string(shared) {
			t.Fatalf("participant changed the mirror: %+v", p)
		}

		// hidden by the operator: kept across publishes
		if err := s.SetPublicAppHidden(ctx, app, true, "test"); err != nil {
			t.Fatal(err)
		}

		// sharing withdrawn: the mirror follows (no stale shared DSL left)
		if _, err := commit(creator, 3, private, &CommitPublish{Claim: true}); err != nil {
			t.Fatal(err)
		}
		p := mirror()
		if p.Version != 4 || string(p.DSL) != string(private) || p.Name != "n"+string(private) {
			t.Fatalf("withdrawn: mirror %+v", p)
		}
		if !p.Hidden {
			t.Fatal("publish cleared hidden")
		}

		// a conflicting commit changes neither the copy nor the mirror
		var ce *ConflictError
		if _, err := commit(creator, 1, shared, &CommitPublish{Shared: true}); !errors.As(err, &ce) {
			t.Fatalf("conflict: %v", err)
		}
		if p := mirror(); p.Version != 4 {
			t.Fatalf("conflict published: %+v", p)
		}

		// a mirror first published by someone else is not overwritten
		app2 := "PUBC2" + sfx
		if _, err := s.PublishApp(ctx, other, app2, shared, "other", 1); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.EnsureApp(ctx, creator, app2, json.RawMessage(`{}`), "x"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.CommitVersion(ctx, creator, app2, Commit{BaseVersion: 1, DSL: private, Publish: &CommitPublish{Shared: true, Claim: true}}); err != nil {
			t.Fatal(err)
		}
		if p, err := s.GetPublicApp(ctx, app2); err != nil || p.OwnerUID != other || string(p.DSL) != string(shared) {
			t.Fatalf("overwrote another uid's mirror: %+v %v", p, err)
		}
	})
}
