package books

import (
	"fmt"
	"testing"
	"time"
)

// store018Fill makes a full cache: the first `expired` entries are past
// their expiry at now, the rest are still fresh.
func store018Fill(now time.Time, expired int) *Service {
	s := &Service{cache: make(map[string]*entry, maxEntries+1)}
	for i := range maxEntries {
		exp := now.Add(time.Hour)
		if i < expired {
			exp = now.Add(-time.Second)
		}
		s.cache[fmt.Sprintf("old-%05d", i)] = &entry{expires: exp}
	}
	return s
}

func TestServiceStoreEvicts(t *testing.T) {
	now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)

	t.Run("expired first", func(t *testing.T) {
		s := store018Fill(now, 10)
		e := &entry{expires: now.Add(time.Hour)}
		s.store("9784000000018", e, now)
		if s.cache["9784000000018"] != e {
			t.Fatal("the new isbn was not stored")
		}
		if got, want := len(s.cache), maxEntries-10+1; got != want {
			t.Fatalf("len = %d, want %d (only the expired ones dropped)", got, want)
		}
		for k, v := range s.cache {
			if !now.Before(v.expires) {
				t.Fatalf("expired %s kept", k)
			}
		}
	})

	t.Run("expiry equal to now counts as expired", func(t *testing.T) {
		s := store018Fill(now, 0)
		s.cache["old-00000"].expires = now
		s.store("9784000000019", &entry{expires: now.Add(time.Hour)}, now)
		if _, ok := s.cache["old-00000"]; ok {
			t.Fatal("entry expiring at now kept")
		}
		if len(s.cache) != maxEntries {
			t.Fatalf("len = %d, want %d", len(s.cache), maxEntries)
		}
	})

	t.Run("nothing expired still goes under the cap", func(t *testing.T) {
		s := store018Fill(now, 0)
		e := &entry{expires: now.Add(time.Hour)}
		s.store("9784000000020", e, now)
		if s.cache["9784000000020"] != e {
			t.Fatal("the new isbn was not stored")
		}
		if len(s.cache) > maxEntries {
			t.Fatalf("len = %d, over the cap %d", len(s.cache), maxEntries)
		}
	})

	t.Run("same isbn replaces", func(t *testing.T) {
		s := store018Fill(now, 0)
		e := &entry{expires: now.Add(2 * time.Hour)}
		s.store("old-00001", e, now)
		if len(s.cache) > maxEntries {
			t.Fatalf("len = %d, over the cap %d", len(s.cache), maxEntries)
		}
		if s.cache["old-00001"] != e {
			t.Fatal("the stored entry is not the new one")
		}
	})

	t.Run("under the cap keeps everything", func(t *testing.T) {
		s := &Service{cache: map[string]*entry{"a": {expires: now.Add(-time.Hour)}}}
		s.store("b", &entry{expires: now.Add(time.Hour)}, now)
		if len(s.cache) != 2 {
			t.Fatalf("len = %d, want 2 (no eviction under the cap)", len(s.cache))
		}
	})
}
