package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// The first claimer of an appId stays its creator (#153, #578).
func TestStoreClaimApp(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		app, a, b := "OWN"+sfx, "own-a"+sfx, "own-b"+sfx
		if _, err := s.AppOwner(ctx, app); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unclaimed: %v", err)
		}
		for _, c := range []struct{ name, uid, want string }{
			{"first claim", a, a},
			{"same uid again", a, a},
			{"another uid", b, a},
		} {
			t.Run(c.name, func(t *testing.T) {
				got, err := s.ClaimApp(ctx, c.uid, app)
				if err != nil || got != c.want {
					t.Fatalf("ClaimApp = %q %v, want %q", got, err, c.want)
				}
				if o, err := s.AppOwner(ctx, app); err != nil || o != c.want {
					t.Fatalf("AppOwner = %q %v", o, err)
				}
			})
		}
		// appIds are independent
		if got, err := s.ClaimApp(ctx, b, app+"-2"); err != nil || got != b {
			t.Fatalf("other app: %q %v", got, err)
		}
	})
}

// ReorderApps sets sortIndex = position+1, skips unknown ids, keeps unlisted
// apps and updatedAt (#277, #578).
func TestStoreReorderApps(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		uid, other := "ord"+sfx, "ord-other"+sfx
		ids := []string{"ORD1" + sfx, "ORD2" + sfx, "ORD3" + sfx}
		for _, id := range ids {
			if _, _, err := s.EnsureApp(ctx, uid, id, json.RawMessage(`{}`), id); err != nil {
				t.Fatal(err)
			}
		}
		if _, _, err := s.EnsureApp(ctx, other, ids[0], json.RawMessage(`{}`), "x"); err != nil {
			t.Fatal(err)
		}
		before, err := s.GetApp(ctx, uid, ids[0])
		if err != nil {
			t.Fatal(err)
		}

		steps := []struct {
			name  string
			order []string
			want  map[string]int
		}{
			{"none yet", nil, map[string]int{ids[0]: 0, ids[1]: 0, ids[2]: 0}},
			{"reverse", []string{ids[2], ids[1], ids[0]}, map[string]int{ids[2]: 1, ids[1]: 2, ids[0]: 3}},
			{"unknown skipped, unlisted kept", []string{"ORD-missing" + sfx, ids[0]}, map[string]int{ids[0]: 2, ids[1]: 2, ids[2]: 1}},
			{"empty is a no-op", []string{}, map[string]int{ids[0]: 2, ids[1]: 2, ids[2]: 1}},
		}
		for _, st := range steps {
			t.Run(st.name, func(t *testing.T) {
				if st.order != nil {
					if err := s.ReorderApps(ctx, uid, st.order); err != nil {
						t.Fatal(err)
					}
				}
				for id, want := range st.want {
					a, err := s.GetApp(ctx, uid, id)
					if err != nil || a.SortIndex != want {
						t.Fatalf("%s: %+v %v, want sortIndex %d", id, a, err, want)
					}
				}
			})
		}
		after, err := s.GetApp(ctx, uid, ids[0])
		if err != nil || !after.UpdatedAt.Equal(before.UpdatedAt) {
			t.Fatalf("updatedAt moved: %v -> %v (%v)", before.UpdatedAt, after.UpdatedAt, err)
		}
		// another user's tree is never touched
		if a, err := s.GetApp(ctx, other, ids[0]); err != nil || a.SortIndex != 0 {
			t.Fatalf("other user's app: %+v %v", a, err)
		}
		if _, err := s.GetApp(ctx, uid, "ORD-missing"+sfx); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unknown id was created: %v", err)
		}
	})
}

func TestStoreUpdateAppMeta(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		tickMemoryClock(s)
		uid, id := "meta"+sfx, "META"+sfx
		created, _, err := s.EnsureApp(ctx, uid, id, json.RawMessage(`{"initialScreen":"home"}`), "orig")
		if err != nil {
			t.Fatal(err)
		}
		str := func(v string) *string { return &v }
		last := created.UpdatedAt
		for _, c := range []struct {
			name           string
			setName, setSt *string
			wantName       string
			wantStatus     string
		}{
			{"name only", str("renamed"), nil, "renamed", created.Status},
			{"status only", nil, str("archived"), "renamed", "archived"},
			{"both", str("again"), str("active"), "again", "active"},
			{"neither (touch)", nil, nil, "again", "active"},
		} {
			t.Run(c.name, func(t *testing.T) {
				got, err := s.UpdateAppMeta(ctx, uid, id, c.setName, c.setSt)
				if err != nil || got.ID != id || got.Name != c.wantName || got.Status != c.wantStatus ||
					got.CurrentVersion != created.CurrentVersion || !got.UpdatedAt.After(last) {
					t.Fatalf("got %+v %v", got, err)
				}
				last = got.UpdatedAt
				if g, err := s.GetApp(ctx, uid, id); err != nil || g.Name != c.wantName || g.Status != c.wantStatus {
					t.Fatalf("stored %+v %v", g, err)
				}
			})
		}
		for _, c := range []struct{ name, uid, id string }{
			{"unknown app", uid, "META-missing" + sfx},
			{"another user's app", "meta-other" + sfx, id},
		} {
			t.Run(c.name, func(t *testing.T) {
				if _, err := s.UpdateAppMeta(ctx, c.uid, c.id, str("x"), nil); !errors.Is(err, ErrNotFound) {
					t.Fatalf("err %v", err)
				}
			})
		}
	})
}

// SetAppGlance stores the list card's stat, "" removes it, updatedAt stays (link-10).
func TestStoreSetAppGlance(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		uid, id := "gl"+sfx, "GL1"+sfx
		if _, _, err := s.EnsureApp(ctx, uid, id, json.RawMessage(`{}`), id); err != nil {
			t.Fatal(err)
		}
		before, err := s.GetApp(ctx, uid, id)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range []string{"total", "-", ""} {
			a, err := s.SetAppGlance(ctx, uid, id, v)
			if err != nil || a.GlanceStat != v {
				t.Fatalf("set %q: %+v %v", v, a, err)
			}
			if g, err := s.GetApp(ctx, uid, id); err != nil || g.GlanceStat != v || !g.UpdatedAt.Equal(before.UpdatedAt) {
				t.Fatalf("get after %q: %+v %v", v, g, err)
			}
		}
		if _, err := s.SetAppGlance(ctx, uid, "GL-missing"+sfx, "x"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("missing app: %v", err)
		}
	})
}
