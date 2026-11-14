package loop

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestMemoryListLimitsAndOrder(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := range 600 {
		it := Item{ID: fmt.Sprintf("lin_%04d", i), Status: ItemNew, IngestedAt: base.Add(time.Duration(599-i) * time.Second)}
		if _, err := m.PutItem(ctx, it); err != nil {
			t.Fatal(err)
		}
	}
	// two items ingested at the same instant are ordered by id
	_, _ = m.PutItem(ctx, Item{ID: "lin_tie_b", Status: ItemHeld, IngestedAt: base})
	_, _ = m.PutItem(ctx, Item{ID: "lin_tie_a", Status: ItemHeld, IngestedAt: base})

	for limit, want := range map[int]int{0: 100, -1: 100, 7: 7, 10_000: 500} {
		got, err := m.ListItems(ctx, ItemQuery{Status: ItemNew, Limit: limit})
		if err != nil || len(got) != want {
			t.Fatalf("limit %d: %d items %v", limit, len(got), err)
		}
		// oldest ingested first: the last one put
		if got[0].ID != "lin_0599" {
			t.Fatalf("limit %d: first %s", limit, got[0].ID)
		}
	}
	held, _ := m.ListItems(ctx, ItemQuery{Status: ItemHeld})
	if len(held) != 2 || held[0].ID != "lin_tie_a" {
		t.Fatalf("tie order %+v", held)
	}

	for i := range 3 {
		d := Draft{ID: fmt.Sprintf("drf_%d", i), Status: DraftPending, CreatedAt: base.Add(time.Duration(i) * time.Hour)}
		if i == 2 {
			d.Status = DraftRejected
		}
		_ = m.PutDraft(ctx, d)
	}
	_ = m.PutDraft(ctx, Draft{ID: "drf_9", Status: DraftPending, CreatedAt: base.Add(time.Hour)}) // ties drf_1
	ds, _ := m.ListDrafts(ctx, DraftQuery{Status: DraftPending})
	if ids := draftIDs(ds); fmt.Sprint(ids) != "[drf_9 drf_1 drf_0]" {
		t.Fatalf("drafts newest first, ties by id desc: %v", ids)
	}
	if ds, _ = m.ListDrafts(ctx, DraftQuery{Limit: 2}); fmt.Sprint(draftIDs(ds)) != "[drf_2 drf_9]" {
		t.Fatalf("any status, limit 2: %v", draftIDs(ds))
	}
	if _, err := m.GetDraft(ctx, "drf_missing"); err != ErrNotFound {
		t.Fatalf("missing draft: %v", err)
	}
}

func draftIDs(ds []Draft) []string {
	out := []string{}
	for _, d := range ds {
		out = append(out, d.ID)
	}
	return out
}

func TestMemoryNotices(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := range 60 {
		n := Notice{ID: NoticeID(fmt.Sprint("d", i), "u1"), UID: "u1", CreatedAt: base.Add(time.Duration(i) * time.Minute), Pushed: i == 10}
		_, _ = m.PutNotice(ctx, n)
	}
	// a newer notice that was not pushed does not move LastPush
	_, _ = m.PutNotice(ctx, Notice{ID: "ntc_other", UID: "u2", Pushed: true, CreatedAt: base.Add(time.Hour)})
	last, _ := m.LastPush(ctx, "u1")
	if !last.Equal(base.Add(10 * time.Minute)) {
		t.Fatalf("last push %v", last)
	}
	if last, _ = m.LastPush(ctx, "nobody"); !last.IsZero() {
		t.Fatalf("never pushed %v", last)
	}
	ns, _ := m.ListNotices(ctx, "u1", 0)
	if len(ns) != 50 || !ns[0].CreatedAt.Equal(base.Add(59*time.Minute)) {
		t.Fatalf("default limit / newest first: %d %v", len(ns), ns[0].CreatedAt)
	}
	if ns, _ = m.ListNotices(ctx, "u1", 3); len(ns) != 3 {
		t.Fatalf("limit 3: %d", len(ns))
	}
	if ns, _ = m.ListNotices(ctx, "u2", 10); len(ns) != 1 || ns[0].ID != "ntc_other" {
		t.Fatalf("other uid leaked: %+v", ns)
	}
}

func TestMemoryReturnsCopies(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	it := Item{ID: "lin_1", Status: ItemNew, MaskCounts: map[string]int{"[メール]": 1}}
	_, _ = m.PutItem(ctx, it)
	it.MaskCounts["[メール]"] = 99 // the caller's map after the put
	got, _ := m.GetItem(ctx, "lin_1")
	got.MaskCounts["[電話]"] = 1
	list, _ := m.ListItems(ctx, ItemQuery{})
	list[0].MaskCounts["[URL]"] = 1
	again, _ := m.GetItem(ctx, "lin_1")
	if len(again.MaskCounts) != 1 || again.MaskCounts["[メール]"] != 1 {
		t.Fatalf("stored item shares a map with a caller: %v", again.MaskCounts)
	}

	d := Draft{ID: "drf_1", ItemIDs: []string{"lin_1"}, Sources: []string{"in_app"}, AppVersions: []string{"1.0"}}
	_ = m.PutDraft(ctx, d)
	d.ItemIDs[0] = "changed"
	gd, _ := m.GetDraft(ctx, "drf_1")
	gd.Sources[0] = "changed"
	ds, _ := m.ListDrafts(ctx, DraftQuery{})
	ds[0].AppVersions[0] = "changed"
	gd, _ = m.GetDraft(ctx, "drf_1")
	if gd.ItemIDs[0] != "lin_1" || gd.Sources[0] != "in_app" || gd.AppVersions[0] != "1.0" {
		t.Fatalf("stored draft shares a slice with a caller: %+v", gd)
	}
}

func TestMemoryPurgeEveryCollectionWithLimit(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	now := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Second), now.Add(time.Second)
	for i := range 3 {
		_, _ = m.PutItem(ctx, Item{ID: fmt.Sprint("lin_old", i), ExpiresAt: past})
		_ = m.PutDraft(ctx, Draft{ID: fmt.Sprint("drf_old", i), ExpiresAt: past})
		_, _ = m.PutNotice(ctx, Notice{ID: fmt.Sprint("ntc_old", i), UID: "u", ExpiresAt: past})
	}
	_, _ = m.PutItem(ctx, Item{ID: "lin_live", ExpiresAt: future})
	_ = m.PutDraft(ctx, Draft{ID: "drf_live", ExpiresAt: future})
	// expiresAt == now is not yet expired
	_, _ = m.PutNotice(ctx, Notice{ID: "ntc_edge", UID: "u", ExpiresAt: now})

	if n, _ := m.PurgeExpired(ctx, now, 0, true); n != 9 {
		t.Fatalf("dry run, no limit: %d", n)
	}
	// the limit is per collection
	if n, _ := m.PurgeExpired(ctx, now, 2, false); n != 6 {
		t.Fatalf("limit 2: %d", n)
	}
	if n, _ := m.PurgeExpired(ctx, now, 2, false); n != 3 {
		t.Fatalf("rest: %d", n)
	}
	if n, _ := m.PurgeExpired(ctx, now, 0, false); n != 0 {
		t.Fatalf("nothing left: %d", n)
	}
	if _, err := m.GetItem(ctx, "lin_live"); err != nil {
		t.Fatal("live item purged")
	}
	if _, err := m.GetDraft(ctx, "drf_live"); err != nil {
		t.Fatal("live draft purged")
	}
	if ns, _ := m.ListNotices(ctx, "u", 10); len(ns) != 1 || ns[0].ID != "ntc_edge" {
		t.Fatalf("notices left %+v", ns)
	}
}
