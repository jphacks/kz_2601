package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

// What moderation_test.go leaves out: ordering, limits, quota scope and the
// block timestamps.
func TestStoreModerationReports(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		a, b := "modx-a"+sfx, "modx-b"+sfx
		// in the future so they are the newest reports on a shared emulator
		base := time.Now().UTC().Add(time.Hour).Truncate(time.Millisecond)
		rep := func(by, target string, at time.Time) Report {
			return Report{ID: ReportID(by, ReportRecord, target), ReporterUID: by, Kind: ReportRecord, TargetID: target,
				TargetUID: "author" + sfx, Reason: "spam", Status: "open", CreatedAt: at}
		}
		for i, r := range []Report{rep(a, "t1", base), rep(a, "t2", base.Add(2*time.Second)), rep(b, "t1", base.Add(time.Second))} {
			if c, err := s.AddReport(ctx, r, "d1", 0); err != nil || !c { // limit 0: no quota
				t.Fatalf("report %d: %v %v", i, c, err)
			}
		}
		want := []string{rep(a, "t2", base).ID, rep(b, "t1", base).ID, rep(a, "t1", base).ID}

		// a's quota does not count b's reports
		if c, err := s.AddReport(ctx, rep(b, "t9", base), "d1", 2); err != nil || !c {
			t.Fatalf("b within its own quota: %v %v", c, err)
		}
		if _, err := s.AddReport(ctx, rep(b, "t10", base), "d1", 2); !errors.Is(err, ErrRateLimited) {
			t.Fatalf("b over quota: %v", err)
		}
		if _, err := s.AddReport(ctx, rep(b, "t1", base), "d1", 2); err != nil {
			t.Fatalf("a duplicate is not rate limited: %v", err)
		}

		must := func(err error) {
			t.Helper()
			if err != nil {
				t.Fatal(err)
			}
		}
		must(s.SetReportStatus(ctx, rep(a, "t1", base).ID, "dismissed"))
		all, err := s.ListReports(ctx, 0)
		must(err)
		mine := map[string]Report{}
		var order []string
		for _, r := range all {
			if r.ReporterUID == a || (r.ReporterUID == b && r.TargetID == "t1") {
				order = append(order, r.ID)
				mine[r.ID] = r
			}
		}
		if fmt.Sprint(order) != fmt.Sprint(want) {
			t.Fatalf("newest first: %v, want %v", order, want)
		}
		if got := mine[rep(a, "t1", base).ID]; got.Status != "dismissed" || got.TargetUID != "author"+sfx ||
			got.Kind != ReportRecord || got.Reason != "spam" || !got.CreatedAt.Equal(base) {
			t.Fatalf("stored report: %+v", got)
		}
		if mine[rep(a, "t2", base).ID].Status != "open" {
			t.Fatalf("other reports keep their status: %+v", mine)
		}
		for _, limit := range []int{1, 2} {
			got, err := s.ListReports(ctx, limit)
			if err != nil || len(got) != limit || got[0].ID != want[0] {
				t.Fatalf("limit %d: %+v %v", limit, got, err)
			}
		}
	})
}

func TestStoreModerationBlocks(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		a, b := "blk-a"+sfx, "blk-b"+sfx
		t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		for _, blk := range []Block{
			{UID: "u3", Name: "U3", CreatedAt: t0.Add(2 * time.Minute)},
			{UID: "u1", Name: "U1", CreatedAt: t0},
			{UID: "u2", CreatedAt: t0.Add(time.Minute)},
			{UID: "u1", Name: "renamed", CreatedAt: t0.Add(time.Hour)}, // keeps the first createdAt
		} {
			if err := s.PutBlock(ctx, a, blk); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.PutBlock(ctx, b, Block{UID: "u9"}); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			name, uid string
			want      string
		}{
			{"oldest first", a, "[u1:renamed u2: u3:U3]"},
			{"per user", b, "[u9:]"},
			{"nobody", "blk-none" + sfx, "[]"},
		} {
			bs, err := s.ListBlocks(ctx, tc.uid)
			got := []string{}
			for _, x := range bs {
				got = append(got, x.UID+":"+x.Name)
			}
			if err != nil || bs == nil || fmt.Sprint(got) != tc.want {
				t.Errorf("%s: %v %v, want %s", tc.name, got, err, tc.want)
			}
		}
		bs, _ := s.ListBlocks(ctx, a)
		if len(bs) == 3 && !bs[0].CreatedAt.Equal(t0) {
			t.Errorf("re-block keeps createdAt: %v", bs[0].CreatedAt)
		}
		if bs, _ := s.ListBlocks(ctx, b); len(bs) != 1 || bs[0].CreatedAt.IsZero() {
			t.Errorf("createdAt is filled in: %+v", bs)
		}
		if err := s.DeleteBlock(ctx, a, "never"+sfx); err != nil {
			t.Errorf("unblocking an unknown uid: %v", err)
		}
	})
}

// Hiding twice keeps the listing hidden; unhiding clears the reason and time.
func TestStoreModerationHiddenToggle(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		owner, id := "hid-owner"+sfx, "HID"+sfx
		if _, err := s.PutCatalogApp(ctx, owner, CatalogApp{ID: id, Name: "x", DSL: json.RawMessage(`{}`)}); err != nil {
			t.Fatal(err)
		}
		for _, st := range []struct {
			hidden bool
			reason string
		}{{true, "spam"}, {true, "abuse"}, {false, ""}, {false, ""}} {
			if err := s.SetCatalogHidden(ctx, id, st.hidden, st.reason); err != nil {
				t.Fatal(err)
			}
			c, err := s.GetCatalogApp(ctx, id)
			if err != nil || c.Hidden != st.hidden || c.HiddenReason != st.reason || c.HiddenAt.IsZero() == st.hidden {
				t.Fatalf("after %+v: %+v %v", st, c.ModerationState, err)
			}
		}
	})
}
