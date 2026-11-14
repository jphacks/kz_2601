package store

import (
	"context"
	"errors"
	"testing"
)

// CreateRecordUnique / UpdateRecordUnique keep one record per claim, and
// Firestore (a transaction with unique/{key} locks) answers like Memory (#273).
func TestStoreRecordUnique(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		rs := PublicRecords("UNQ"+sfx, "codes")
		k1, k2, k3 := UniqueClaim{Key: "k1"}, UniqueClaim{Key: "k2"}, UniqueClaim{Key: "k3"}
		create := func(id string, claim UniqueClaim) (bool, error) {
			t.Helper()
			_, created, err := s.CreateRecordUnique(ctx, rs, Record{ID: id, Data: map[string]any{"code": id}, Author: "u" + sfx}, claim, nil)
			return created, err
		}
		update := func(id string, claim UniqueClaim) error {
			t.Helper()
			_, err := s.UpdateRecordUnique(ctx, rs, id, map[string]any{"code": id + "!"}, claim, nil)
			return err
		}
		mustCreate := func(id string, claim UniqueClaim) {
			t.Helper()
			if created, err := create(id, claim); err != nil || !created {
				t.Fatalf("create %s %q: %v %v", id, claim.Key, created, err)
			}
		}
		key := func(id string) string {
			t.Helper()
			r, err := s.GetRecord(ctx, rs, id)
			if err != nil {
				t.Fatalf("get %s: %v", id, err)
			}
			return r.UniqueKey
		}

		// create; the same values again are refused
		mustCreate("a", k1)
		if key("a") != "k1" {
			t.Fatalf("a holds %q", key("a"))
		}
		if _, err := create("b", k1); !errors.Is(err, ErrDuplicate) {
			t.Fatalf("duplicate create: %v", err)
		}
		if _, err := s.GetRecord(ctx, rs, "b"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("refused record was written: %v", err)
		}
		// idempotent on id: the existing record comes back unchecked
		r, created, err := s.CreateRecordUnique(ctx, rs, Record{ID: "a", Data: map[string]any{"code": "other"}}, k3, nil)
		if err != nil || created || r.Data["code"] != "a" || r.UniqueKey != "k1" {
			t.Fatalf("create again: %+v %v %v", r, created, err)
		}
		// an empty claim reserves nothing
		mustCreate("free1", UniqueClaim{})
		mustCreate("free2", UniqueClaim{})

		// update to new values releases the old ones
		if err := update("a", k2); err != nil {
			t.Fatalf("move a: %v", err)
		}
		if key("a") != "k2" {
			t.Fatalf("a holds %q after move", key("a"))
		}
		mustCreate("b", k1)
		// keeping its own values is not a duplicate
		if err := update("a", k2); err != nil {
			t.Fatalf("update keeping the claim: %v", err)
		}
		// moving onto values another record holds is refused; nothing changes
		mustCreate("c", k3)
		if err := update("c", k2); !errors.Is(err, ErrDuplicate) {
			t.Fatalf("duplicate update: %v", err)
		}
		if c, err := s.GetRecord(ctx, rs, "c"); err != nil || c.Data["code"] != "c" || c.UniqueKey != "k3" {
			t.Fatalf("c after refused update: %+v %v", c, err)
		}
		// clearing the values releases them
		if err := update("b", UniqueClaim{}); err != nil {
			t.Fatalf("clear b: %v", err)
		}
		if key("b") != "" {
			t.Fatalf("b holds %q after clear", key("b"))
		}
		mustCreate("d", k1)
		// delete releases the values
		if err := s.DeleteRecord(ctx, rs, "a"); err != nil {
			t.Fatal(err)
		}
		mustCreate("e", k2)
		if err := update("missing", k3); !errors.Is(err, ErrNotFound) {
			t.Fatalf("update missing: %v", err)
		}
	})
}

// A record written without a claim (before entity.unique, or by a server
// rule) still blocks its values through UniqueClaim.Match: data fields
// (numbers by value) and the author.
func TestStoreRecordUniqueMatch(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		rs := PublicRecords("UNQM"+sfx, "seats")
		author := "u" + sfx
		if _, _, err := s.CreateRecord(ctx, rs, Record{ID: "legacy", Data: map[string]any{"row": "A", "n": 1}, Author: author}); err != nil {
			t.Fatal(err)
		}
		for _, c := range []struct {
			name  string
			match map[string]any
			dup   bool
		}{
			{"same values", map[string]any{"row": "A", "n": 1}, true},
			{"number by value", map[string]any{"row": "A", "n": 1.0}, true},
			{"same author", map[string]any{UniqueAuthorKey: author}, true},
			{"other value", map[string]any{"row": "A", "n": 2}, false},
			{"other author", map[string]any{UniqueAuthorKey: "v" + sfx}, false},
		} {
			t.Run(c.name, func(t *testing.T) {
				id := "new-" + c.name
				claim := UniqueClaim{Key: "m-" + c.name, Match: c.match}
				_, created, err := s.CreateRecordUnique(ctx, rs, Record{ID: id, Data: map[string]any{"row": "B"}}, claim, nil)
				if c.dup {
					if !errors.Is(err, ErrDuplicate) {
						t.Fatalf("create: %v %v, want ErrDuplicate", created, err)
					}
					// the same claim on the legacy record itself is not a duplicate
					if _, err := s.UpdateRecordUnique(ctx, rs, "legacy", map[string]any{"row": "A", "n": 1}, claim, nil); err != nil {
						t.Fatalf("legacy takes its own values: %v", err)
					}
					if _, err := s.UpdateRecordUnique(ctx, rs, "legacy", map[string]any{"row": "A", "n": 1}, UniqueClaim{}, nil); err != nil {
						t.Fatalf("legacy releases: %v", err)
					}
					return
				}
				if err != nil || !created {
					t.Fatalf("create: %v %v", created, err)
				}
			})
		}
	})
}
