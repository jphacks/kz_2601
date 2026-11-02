package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// mine keeps the listings of this run (the emulator's public_apps is shared by runs).
func mine(cs []CatalogApp, sfx string) []CatalogApp {
	var out []CatalogApp
	for _, c := range cs {
		if strings.HasSuffix(c.ID, sfx) && strings.HasPrefix(c.ID, "CAT") {
			out = append(out, c)
		}
	}
	return out
}

func TestStorePutCatalogApp(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		tickMemoryClock(s)
		owner, other, id := "cat-owner"+sfx, "cat-other"+sfx, "CATP"+sfx
		dsl := json.RawMessage(`{"initialScreen":"home"}`)
		in := CatalogApp{ID: id, OwnerUID: "ignored", Name: "N", Description: "d", Tags: []string{"t1", "t2"},
			AuthorName: "A", Components: []string{"list"}, Version: 2, DSL: dsl, Installs: 99}
		first, err := s.PutCatalogApp(ctx, owner, in)
		if err != nil || first.ID != id || first.OwnerUID != owner || first.Installs != 0 || first.DSL != nil ||
			first.CreatedAt.IsZero() || !first.UpdatedAt.Equal(first.CreatedAt) || first.Name != "N" || first.Version != 2 {
			t.Fatalf("create: %+v %v", first, err)
		}
		got, err := s.GetCatalogApp(ctx, id)
		if err != nil || string(got.DSL) != string(dsl) || strings.Join(got.Tags, ",") != "t1,t2" ||
			strings.Join(got.Components, ",") != "list" || got.Description != "d" || got.AuthorName != "A" {
			t.Fatalf("stored: %+v %v", got, err)
		}

		// update by the owner keeps installs, createdAt and the operator's hidden state
		if err := s.IncrementInstalls(ctx, id); err != nil {
			t.Fatal(err)
		}
		if err := s.SetCatalogHidden(ctx, id, true, "spam"); err != nil {
			t.Fatal(err)
		}
		in.Name, in.Version = "N2", 3
		upd, err := s.PutCatalogApp(ctx, owner, in)
		if err != nil || upd.Name != "N2" || upd.Version != 3 || upd.Installs != 1 || !upd.CreatedAt.Truncate(time.Microsecond).Equal(first.CreatedAt.Truncate(time.Microsecond)) ||
			!upd.UpdatedAt.After(first.UpdatedAt) || !upd.Hidden || upd.HiddenReason != "spam" {
			t.Fatalf("update: %+v %v", upd, err)
		}

		// another uid cannot take the listing over
		if _, err := s.PutCatalogApp(ctx, other, in); !errors.Is(err, ErrForbidden) {
			t.Fatalf("other uid: %v", err)
		}
		if got, _ := s.GetCatalogApp(ctx, id); got == nil || got.OwnerUID != owner || got.Name != "N2" {
			t.Fatalf("other uid changed it: %+v", got)
		}
		if _, err := s.GetCatalogApp(ctx, "CAT-missing"+sfx); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unknown: %v", err)
		}
	})
}

func TestStoreDeleteCatalogApp(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		owner, id := "catd-owner"+sfx, "CATD"+sfx
		if _, err := s.PutCatalogApp(ctx, owner, CatalogApp{ID: id, Name: "x", DSL: json.RawMessage(`{}`)}); err != nil {
			t.Fatal(err)
		}
		for _, c := range []struct {
			name, uid, id string
			err           error
		}{
			{"not the owner", "catd-other" + sfx, id, ErrForbidden},
			{"owner", owner, id, nil},
			{"already gone", owner, id, ErrNotFound},
			{"never existed", owner, "CATD-missing" + sfx, ErrNotFound},
		} {
			t.Run(c.name, func(t *testing.T) {
				if err := s.DeleteCatalogApp(ctx, c.uid, c.id); !errors.Is(err, c.err) {
					t.Fatalf("err %v, want %v", err, c.err)
				}
			})
		}
		if _, err := s.GetCatalogApp(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("after delete: %v", err)
		}
		// the owner can list it again from scratch
		again, err := s.PutCatalogApp(ctx, owner, CatalogApp{ID: id, Name: "y"})
		if err != nil || again.Installs != 0 {
			t.Fatalf("republish: %+v %v", again, err)
		}
	})
}

// ListCatalogApps: most installed first, limit applies, DSL omitted, hidden
// listings are still returned (the handler filters them, #321).
func TestStoreListCatalogApps(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		tickMemoryClock(s)
		installs := map[string]int{"CAT0" + sfx: 0, "CAT1" + sfx: 1, "CAT3" + sfx: 3, "CAT2" + sfx: 2}
		for id, n := range installs {
			if _, err := s.PutCatalogApp(ctx, "catl"+sfx, CatalogApp{ID: id, Name: id, DSL: json.RawMessage(`{"x":1}`)}); err != nil {
				t.Fatal(err)
			}
			for range n {
				if err := s.IncrementInstalls(ctx, id); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := s.SetCatalogHidden(ctx, "CAT2"+sfx, true, "spam"); err != nil {
			t.Fatal(err)
		}

		all, err := s.ListCatalogApps(ctx, MaxCatalogApps)
		if err != nil {
			t.Fatal(err)
		}
		for i := 1; i < len(all); i++ {
			if all[i-1].Installs < all[i].Installs {
				t.Fatalf("not installs desc at %d: %d < %d", i, all[i-1].Installs, all[i].Installs)
			}
		}
		got := mine(all, sfx)
		var order []string
		for _, c := range got {
			order = append(order, c.ID)
			if c.DSL != nil {
				t.Errorf("%s: DSL not omitted", c.ID)
			}
			if c.Installs != installs[c.ID] {
				t.Errorf("%s: installs %d, want %d", c.ID, c.Installs, installs[c.ID])
			}
			if c.Hidden != (c.ID == "CAT2"+sfx) {
				t.Errorf("%s: hidden %v", c.ID, c.Hidden)
			}
		}
		if want := "CAT3,CAT2,CAT1,CAT0"; strings.ReplaceAll(strings.Join(order, ","), sfx, "") != want {
			t.Fatalf("order %v, want %s", order, want)
		}

		for _, limit := range []int{1, 2} {
			page, err := s.ListCatalogApps(ctx, limit)
			if err != nil || len(page) != limit {
				t.Fatalf("limit %d: %d items %v", limit, len(page), err)
			}
			for i := range page {
				if page[i].ID != all[i].ID && page[i].Installs != all[i].Installs {
					t.Fatalf("limit %d: item %d is %s, want the top of the full list", limit, i, page[i].ID)
				}
			}
		}
	})
}

// SetCatalogHidden on a missing listing is ErrNotFound (Moderation contract).
func TestStoreSetCatalogHiddenMissing(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		for _, hidden := range []bool{true, false} {
			if err := s.SetCatalogHidden(context.Background(), "CATH-missing"+sfx, hidden, "r"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("hidden=%v: %v", hidden, err)
			}
		}
	})
}
