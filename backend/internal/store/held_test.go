package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
)

// #630 app slots: a commit that makes a blank app held checks the held count
// in its transaction; other commits are not counted.
func TestCommitHeldLimit(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		uid := "held" + sfx
		blank := func(id string) {
			t.Helper()
			if _, _, err := s.EnsureApp(ctx, uid, id, json.RawMessage(`{}`), id); err != nil {
				t.Fatal(err)
			}
		}
		gen := func(id string, base, limit int) error {
			_, err := s.CommitVersion(ctx, uid, id, Commit{BaseVersion: base, DSL: json.RawMessage(`{"v":1}`), HeldLimit: limit})
			return err
		}
		for _, id := range []string{"H1", "H2", "H3"} {
			blank(id + sfx)
		}
		if err := gen("H1"+sfx, 1, 2); err != nil {
			t.Fatal(err)
		}
		if err := gen("H2"+sfx, 1, 2); err != nil {
			t.Fatal(err)
		}
		var he *HeldLimitError
		if err := gen("H3"+sfx, 1, 2); !errors.As(err, &he) || he.Held != 2 || he.Limit != 2 {
			t.Fatalf("3rd: %v", err)
		}
		// nothing was written: still blank, and the held apps still commit
		if a, err := s.GetApp(ctx, uid, "H3"+sfx); err != nil || a.CurrentVersion != 1 {
			t.Fatalf("H3 after the refusal: %+v %v", a, err)
		}
		if err := gen("H1"+sfx, 2, 2); err != nil {
			t.Fatalf("an edit of a held app is not counted: %v", err)
		}
		if err := gen("H3"+sfx, 1, 3); err != nil {
			t.Fatalf("a bought slot: %v", err)
		}
		if err := gen("H3"+sfx, 2, 0); err != nil {
			t.Fatal(err)
		}
	})
}

// Two blanks of one uid generated at once with one slot left: exactly one
// becomes held (the users/{uid} lock serializes the counts).
func TestCommitHeldLimitConcurrent(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		uid := "heldc" + sfx
		ids := []string{"C0" + sfx, "C1" + sfx, "C2" + sfx, "C3" + sfx}
		for _, id := range ids {
			if _, _, err := s.EnsureApp(ctx, uid, id, json.RawMessage(`{}`), id); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.CommitVersion(ctx, uid, ids[0], Commit{BaseVersion: 1, DSL: json.RawMessage(`{"v":1}`)}); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		errs := make([]error, 3)
		for i := range 3 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = s.CommitVersion(ctx, uid, ids[i+1], Commit{BaseVersion: 1, DSL: json.RawMessage(fmt.Sprintf(`{"v":%d}`, i)), HeldLimit: 2})
			}()
		}
		wg.Wait()
		ok, refused := 0, 0
		for _, err := range errs {
			var he *HeldLimitError
			switch {
			case err == nil:
				ok++
			case errors.As(err, &he):
				refused++
			default:
				t.Fatalf("unexpected: %v", err)
			}
		}
		if ok != 1 || refused != 2 {
			t.Fatalf("ok %d refused %d (%v)", ok, refused, errs)
		}
		if n, err := s.(Counter).CountApps(ctx, uid); err != nil || n != 2 {
			t.Fatalf("held %d %v", n, err)
		}
	})
}
