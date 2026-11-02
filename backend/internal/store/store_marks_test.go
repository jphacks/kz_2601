package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestStoreMarks(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		rs := PublicRecords("MARKS"+sfx, "post")
		a, b := "mk-a"+sfx, "mk-b"+sfx
		for _, id := range []string{"r1", "r2"} {
			if _, _, err := s.CreateRecord(ctx, rs, Record{ID: id, Entity: rs.Entity, Author: a, Data: map[string]any{}}); err != nil {
				t.Fatal(err)
			}
		}

		for _, st := range []struct {
			name      string
			uid, mark string
			on        bool
			want      MarkState
		}{
			{"off before any is a no-op", a, "like", false, MarkState{Count: 0, Mine: false}},
			{"a likes", a, "like", true, MarkState{Count: 1, Mine: true}},
			{"a likes again (idempotent)", a, "like", true, MarkState{Count: 1, Mine: true}},
			{"b likes", b, "like", true, MarkState{Count: 2, Mine: true}},
			{"another mark counts apart", a, "save", true, MarkState{Count: 1, Mine: true}},
			{"a unlikes", a, "like", false, MarkState{Count: 1, Mine: false}},
			{"a unlikes again (idempotent)", a, "like", false, MarkState{Count: 1, Mine: false}},
		} {
			got, err := s.SetMark(ctx, rs, "r1", st.mark, st.uid, st.on)
			if err != nil || got != st.want {
				t.Fatalf("%s: %+v %v, want %+v", st.name, got, err, st.want)
			}
		}
		r, err := s.GetRecord(ctx, rs, "r1")
		if err != nil || r.MarkCounts["like"] != 1 || r.MarkCounts["save"] != 1 {
			t.Fatalf("counts on the record: %+v %v", r, err)
		}

		for _, tc := range []struct {
			name string
			rs   RecordSet
			id   string
		}{
			{"missing record", rs, "none"},
			{"missing app", PublicRecords("NOAPP"+sfx, "post"), "r1"},
			{"other entity", PublicRecords(rs.AppID, "other"), "r1"},
			{"missing private app", PrivateRecords(a, "NOAPP"+sfx, "post"), "r1"},
		} {
			if _, err := s.SetMark(ctx, tc.rs, tc.id, "like", a, true); !errors.Is(err, ErrNotFound) {
				t.Errorf("%s: want ErrNotFound, got %v", tc.name, err)
			}
		}

		for _, tc := range []struct {
			name  string
			uid   string
			ids   []string
			marks []string
			want  string
		}{
			{"a", a, []string{"r1", "r2", "none"}, []string{"like", "save"}, "map[r1:map[save:true]]"},
			{"b", b, []string{"r1", "r2"}, []string{"like", "save"}, "map[r1:map[like:true]]"},
			{"only asked marks", b, []string{"r1"}, []string{"save"}, "map[]"},
			{"nobody", "mk-nobody" + sfx, []string{"r1"}, []string{"like"}, "map[]"},
			{"no records", a, nil, []string{"like"}, "map[]"},
			{"no marks", a, []string{"r1"}, nil, "map[]"},
		} {
			got, err := s.MarkedBy(ctx, rs, tc.ids, tc.marks, tc.uid)
			if err != nil || got == nil || fmt.Sprint(got) != tc.want {
				t.Errorf("MarkedBy %s: %v %v, want %s", tc.name, got, err, tc.want)
			}
		}

		// a deleted record takes its marks with it: the same id starts over
		if err := s.DeleteRecord(ctx, rs, "r1"); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.CreateRecord(ctx, rs, Record{ID: "r1", Entity: rs.Entity, Author: a, Data: map[string]any{}}); err != nil {
			t.Fatal(err)
		}
		if got, err := s.MarkedBy(ctx, rs, []string{"r1"}, []string{"like", "save"}, b); err != nil || len(got) != 0 {
			t.Fatalf("marks after re-create: %v %v", got, err)
		}
		if got, err := s.SetMark(ctx, rs, "r1", "like", b, true); err != nil || got != (MarkState{Count: 1, Mine: true}) {
			t.Fatalf("like after re-create: %+v %v", got, err)
		}
	})
}

// MarkedBy answers for a whole page of records (more lookups than one batch read).
func TestStoreMarkedByManyRecords(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		rs := PublicRecords("MARKSMANY"+sfx, "post")
		uid := "mk-many" + sfx
		ids := make([]string, 0, 400)
		for i := range 400 {
			ids = append(ids, fmt.Sprintf("r%03d", i))
		}
		liked := []string{"r000", "r150", "r299", "r300", "r399"} // around the 300-lookup chunk edge
		for _, id := range liked {
			if _, _, err := s.CreateRecord(ctx, rs, Record{ID: id, Entity: rs.Entity, Data: map[string]any{}}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.SetMark(ctx, rs, id, "like", uid, true); err != nil {
				t.Fatal(err)
			}
		}
		got, err := s.MarkedBy(ctx, rs, ids, []string{"like", "save"}, uid)
		if err != nil || len(got) != len(liked) {
			t.Fatalf("got %v %v", got, err)
		}
		for _, id := range liked {
			if !got[id]["like"] || got[id]["save"] {
				t.Errorf("%s: %v", id, got[id])
			}
		}
	})
}
