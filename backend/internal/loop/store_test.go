package loop

import (
	"context"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

func TestMemoryStore(t *testing.T) { testStore(t, NewMemory(), "m") }

// Runs against the emulator only (FIRESTORE_EMULATOR_HOST=...); never staging/prod.
func TestFirestoreLoopStore(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	c, err := firestore.NewClient(context.Background(), "demo-kz2601")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	testStore(t, NewFirestore(c), time.Now().Format("150405.000000"))
}

func testStore(t *testing.T, s Store, sfx string) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	uid := "u" + sfx
	it := Item{ID: ItemID(SourceInApp, "fb_"+sfx), Source: SourceInApp, SourceID: "fb_" + sfx, UID: uid, Text: "落ちる",
		MaskCounts: map[string]int{"[メール]": 1}, AIAllowed: true, Status: ItemNew, CreatedAt: now, IngestedAt: now, ExpiresAt: now.Add(Retention)}
	created, err := s.PutItem(ctx, it)
	must(err)
	if !created {
		t.Fatal("first put not created")
	}
	it2 := it
	it2.Text = "changed"
	created, err = s.PutItem(ctx, it2)
	must(err)
	if created {
		t.Fatal("second put created")
	}
	got, err := s.GetItem(ctx, it.ID)
	must(err)
	if got.Text != "落ちる" || got.MaskCounts["[メール]"] != 1 || got.UID != uid {
		t.Fatalf("get %+v", got)
	}
	got.Status, got.Category, got.TriagedAt = ItemTriaged, "bug", now
	must(s.UpdateItem(ctx, *got))
	list, err := s.ListItems(ctx, ItemQuery{Status: ItemTriaged, Limit: 500})
	must(err)
	found := false
	for _, x := range list {
		if x.ID == it.ID {
			found = x.Category == "bug"
		}
	}
	if !found {
		t.Fatal("triaged item not listed")
	}
	if err := s.UpdateItem(ctx, Item{ID: "lin_missing" + sfx}); err != ErrNotFound {
		t.Fatalf("update missing: %v", err)
	}
	if _, err := s.GetItem(ctx, "lin_missing"+sfx); err != ErrNotFound {
		t.Fatalf("get missing: %v", err)
	}

	d := Draft{ID: "drf_" + sfx, ClusterKey: "photo-crash", Title: "t", Body: "b", ItemIDs: []string{it.ID}, Count: 1,
		Sources: []string{"in_app"}, Status: DraftPending, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(Retention)}
	must(s.PutDraft(ctx, d))
	gd, err := s.GetDraft(ctx, d.ID)
	must(err)
	if gd.ClusterKey != "photo-crash" || len(gd.ItemIDs) != 1 {
		t.Fatalf("draft %+v", gd)
	}
	ds, err := s.ListDrafts(ctx, DraftQuery{Status: DraftPending, Limit: 500})
	must(err)
	if len(ds) == 0 {
		t.Fatal("no pending drafts")
	}

	n := Notice{ID: NoticeID(d.ID, uid), UID: uid, DraftID: d.ID, Title: "直りました", Pushed: true, CreatedAt: now, ExpiresAt: now.Add(Retention)}
	created, err = s.PutNotice(ctx, n)
	must(err)
	if !created {
		t.Fatal("notice not created")
	}
	if created, _ = s.PutNotice(ctx, n); created {
		t.Fatal("notice duplicated")
	}
	last, err := s.LastPush(ctx, uid)
	must(err)
	if !last.Equal(now) {
		t.Fatalf("last push %v", last)
	}
	ns, err := s.ListNotices(ctx, uid, 10)
	must(err)
	if len(ns) != 1 || ns[0].ID != n.ID {
		t.Fatalf("notices %+v", ns)
	}

	src := Source("test-" + sfx)
	c, err := s.Cursor(ctx, src)
	must(err)
	if !c.IsZero() {
		t.Fatal("cursor not zero")
	}
	must(s.SetCursor(ctx, src, now))
	if c, _ = s.Cursor(ctx, src); !c.Equal(now) {
		t.Fatalf("cursor %v", c)
	}

	items, notices, err := s.ItemsByUID(ctx, uid)
	must(err)
	if len(items) != 1 || len(notices) != 1 {
		t.Fatalf("by uid %v %v", items, notices)
	}

	// expiry: an expired item goes, the live one stays
	old := it
	old.ID, old.SourceID, old.ExpiresAt = ItemID(SourceInApp, "old"+sfx), "old"+sfx, now.Add(-time.Hour)
	_, err = s.PutItem(ctx, old)
	must(err)
	cnt, err := s.PurgeExpired(ctx, now, 500, true)
	must(err)
	if cnt < 1 {
		t.Fatalf("dry run counted %d", cnt)
	}
	if _, err := s.GetItem(ctx, old.ID); err != nil {
		t.Fatal("dry run deleted")
	}
	_, err = s.PurgeExpired(ctx, now, 500, false)
	must(err)
	if _, err := s.GetItem(ctx, old.ID); err != ErrNotFound {
		t.Fatalf("expired item kept: %v", err)
	}
	if _, err := s.GetItem(ctx, it.ID); err != nil {
		t.Fatalf("live item purged: %v", err)
	}

	must(s.DeleteItems(ctx, items))
	must(s.DeleteNotices(ctx, notices))
	items, notices, err = s.ItemsByUID(ctx, uid)
	must(err)
	if len(items)+len(notices) != 0 {
		t.Fatalf("left %v %v", items, notices)
	}
}
