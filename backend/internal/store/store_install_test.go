package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// RecordInstall counts each uid once and only when asked (#154, #578).
func TestStoreInstalls(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		id, a, b, c := "INS"+sfx, "ins-a"+sfx, "ins-b"+sfx, "ins-c"+sfx
		if _, err := s.PutCatalogApp(ctx, "ins-owner"+sfx, CatalogApp{ID: id, Name: "x", DSL: json.RawMessage(`{}`)}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetInstall(ctx, id, a); !errors.Is(err, ErrNotFound) {
			t.Fatalf("before install: %v", err)
		}

		steps := []struct {
			name, uid, appID string
			count, wantFirst bool
			wantInstalls     int
		}{
			{"a first", a, "APP-a1", true, true, 1},
			{"a again: not counted, appId replaced", a, "APP-a2", true, false, 1},
			{"b without count (owner / joiner)", b, "APP-b", false, true, 1},
			{"b later with count: already seen", b, "APP-b2", true, false, 1},
			{"c first", c, "APP-c", true, true, 2},
		}
		for _, st := range steps {
			t.Run(st.name, func(t *testing.T) {
				first, err := s.RecordInstall(ctx, id, st.uid, st.appID, st.count)
				if err != nil || first != st.wantFirst {
					t.Fatalf("first=%v %v, want %v", first, err, st.wantFirst)
				}
				if got, err := s.GetInstall(ctx, id, st.uid); err != nil || got != st.appID {
					t.Fatalf("GetInstall = %q %v", got, err)
				}
				if cat, err := s.GetCatalogApp(ctx, id); err != nil || cat.Installs != st.wantInstalls {
					t.Fatalf("installs = %+v %v, want %d", cat, err, st.wantInstalls)
				}
			})
		}

		// installs are per listing
		if _, err := s.GetInstall(ctx, id+"-other", a); !errors.Is(err, ErrNotFound) {
			t.Fatalf("other listing: %v", err)
		}

		// a listing that no longer exists: the marker is still kept, nothing to count
		gone := "INS-gone" + sfx
		if first, err := s.RecordInstall(ctx, gone, a, "APP-g", true); err != nil || !first {
			t.Fatalf("missing listing: %v %v", first, err)
		}
		if got, err := s.GetInstall(ctx, gone, a); err != nil || got != "APP-g" {
			t.Fatalf("missing listing marker: %q %v", got, err)
		}
		if _, err := s.GetCatalogApp(ctx, gone); !errors.Is(err, ErrNotFound) {
			t.Fatalf("missing listing was created: %v", err)
		}
	})
}

func TestStoreIncrementInstalls(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		id := "INC" + sfx
		if _, err := s.PutCatalogApp(ctx, "inc-owner"+sfx, CatalogApp{ID: id, Name: "x"}); err != nil {
			t.Fatal(err)
		}
		for want := 1; want <= 3; want++ {
			if err := s.IncrementInstalls(ctx, id); err != nil {
				t.Fatal(err)
			}
			if c, err := s.GetCatalogApp(ctx, id); err != nil || c.Installs != want {
				t.Fatalf("installs %+v %v, want %d", c, err, want)
			}
		}
		if err := s.IncrementInstalls(ctx, "INC-missing"+sfx); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unknown: %v", err)
		}
	})
}
