package store

import (
	"sync"
	"testing"
	"time"
)

// users/{uid}.firstSeenAt (#339 account age): set once by SeeUser, kept
// through re-sign-in and logical deletion, dropped by the purge tombstone.
func TestStoreFirstSeen(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := t.Context()
		uid, never := "fs-user"+sfx, "fs-never"+sfx

		if _, known, err := s.UserFirstSeen(ctx, never); known || err != nil {
			t.Fatalf("never seen: known=%v err=%v", known, err)
		}

		// concurrent first requests write one time
		var wg sync.WaitGroup
		for range 4 {
			wg.Go(func() {
				if gone, err := s.SeeUser(ctx, uid); gone || err != nil {
					t.Errorf("see: gone=%v err=%v", gone, err)
				}
			})
		}
		wg.Wait()
		first, known, err := s.UserFirstSeen(ctx, uid)
		if err != nil || !known || first.IsZero() {
			t.Fatalf("first seen: %v %v %v", first, known, err)
		}
		if gone, err := s.UserDeleted(ctx, uid); gone || err != nil {
			t.Fatalf("firstSeenAt must not read as deleted: %v %v", gone, err)
		}

		// a later request (re-sign-in) keeps the first time
		time.Sleep(2 * time.Millisecond)
		if gone, err := s.SeeUser(ctx, uid); gone || err != nil {
			t.Fatalf("see again: %v %v", gone, err)
		}
		if at, _, _ := s.UserFirstSeen(ctx, uid); !at.Equal(first) {
			t.Fatalf("firstSeenAt overwritten: %v -> %v", first, at)
		}

		// logical deletion keeps it (merge) and SeeUser reports deleted
		if err := s.DeactivateUser(ctx, uid); err != nil {
			t.Fatal(err)
		}
		if gone, err := s.SeeUser(ctx, uid); !gone || err != nil {
			t.Fatalf("see deleted: %v %v", gone, err)
		}
		if at, known, _ := s.UserFirstSeen(ctx, uid); !known || !at.Equal(first) {
			t.Fatalf("deleted account: %v %v", at, known)
		}

		// the purge tombstone has no firstSeenAt, and SeeUser does not put it back
		if _, err := s.PurgeAccount(ctx, uid); err != nil {
			t.Fatal(err)
		}
		if err := s.MarkAccountPurged(ctx, uid); err != nil {
			t.Fatal(err)
		}
		if gone, err := s.SeeUser(ctx, uid); !gone || err != nil {
			t.Fatalf("see purged: %v %v", gone, err)
		}
		if _, known, err := s.UserFirstSeen(ctx, uid); known || err != nil {
			t.Fatalf("purged account keeps firstSeenAt: known=%v err=%v", known, err)
		}

		// an account deleted before it was ever seen gets no firstSeenAt
		late := "fs-late" + sfx
		if err := s.DeactivateUser(ctx, late); err != nil {
			t.Fatal(err)
		}
		if gone, err := s.SeeUser(ctx, late); !gone || err != nil {
			t.Fatalf("see late: %v %v", gone, err)
		}
		if _, known, _ := s.UserFirstSeen(ctx, late); known {
			t.Fatal("firstSeenAt written into a deleted account")
		}
	})
}
