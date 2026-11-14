package store

import (
	"context"
	"encoding/json"
	"testing"
)

// Counter (#317): apps per user, records per entity, records with a field set.
func TestStoreCounter(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		c, ok := s.(Counter)
		if !ok {
			t.Fatal("store does not implement Counter")
		}
		uid, app := "cnt"+sfx, "CNT"+sfx
		if n, err := c.CountApps(ctx, uid); err != nil || n != 0 {
			t.Fatalf("no apps: %d %v", n, err)
		}
		priv := PrivateRecords(uid, app, "e")
		if n, err := c.CountRecords(ctx, priv, ""); err != nil || n != 0 {
			t.Fatalf("missing app: %d %v", n, err)
		}
		for _, id := range []string{app, app + "-2"} {
			if _, _, err := s.EnsureApp(ctx, uid, id, json.RawMessage(`{}`), id); err != nil {
				t.Fatal(err)
			}
		}
		if n, err := c.CountApps(ctx, uid); err != nil || n != 0 {
			t.Fatalf("blank apps are not held: CountApps = %d %v", n, err)
		}
		for _, id := range []string{app, app + "-2"} {
			if _, err := s.CommitVersion(ctx, uid, id, Commit{BaseVersion: 1, DSL: json.RawMessage(`{"v":2}`)}); err != nil {
				t.Fatal(err)
			}
		}
		if n, err := c.CountApps(ctx, uid); err != nil || n != 2 {
			t.Fatalf("CountApps = %d %v, want 2", n, err)
		}
		datas := []map[string]any{
			{"title": "a", "pic": "data:image/jpeg;base64,AA=="},
			{"title": "b"},
			{"title": "c", "pic": "https://example.com/p.jpg"},
		}
		for _, d := range datas {
			if _, _, err := s.CreateRecord(ctx, priv, Record{ID: NewID("rec_"), Data: d}); err != nil {
				t.Fatal(err)
			}
		}
		pub := PublicRecords(app, "e")
		if _, _, err := s.CreateRecord(ctx, pub, Record{ID: NewID("rec_"), Data: map[string]any{"pic": "data:image/png;base64,AA=="}}); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			name  string
			rs    RecordSet
			field string
			want  int
		}{
			{"private all", priv, "", 3},
			{"private with photo", priv, "pic", 2},
			{"private, other entity", PrivateRecords(uid, app, "other"), "", 0},
			{"public all", pub, "", 1},
			{"public with photo", pub, "pic", 1},
			{"field nobody has", priv, "none", 0},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if n, err := c.CountRecords(ctx, tc.rs, tc.field); err != nil || n != tc.want {
					t.Fatalf("CountRecords = %d %v, want %d", n, err, tc.want)
				}
			})
		}
	})
}
