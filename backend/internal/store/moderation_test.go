package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
)

func TestMemoryModeration(t *testing.T) { testModeration(t, NewMemory(), "") }

// Runs against the emulator only (FIRESTORE_EMULATOR_HOST=...).
func TestFirestoreModeration(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	f, err := NewFirestore(context.Background(), "demo-kz2601", "(default)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	testModeration(t, f, time.Now().Format("150405.000000"))
}

func testModeration(t *testing.T, s Store, sfx string) {
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	a, b := "mod-a"+sfx, "mod-b"+sfx

	// reports: one per (reporter, target), daily quota
	rep := func(target string) Report {
		return Report{ID: ReportID(a, ReportListing, target), ReporterUID: a, Kind: ReportListing, TargetID: target, Reason: "spam", Status: "open"}
	}
	if c, err := s.AddReport(ctx, rep("L1"+sfx), "d1", 2); err != nil || !c {
		t.Fatalf("first: %v %v", c, err)
	}
	if c, err := s.AddReport(ctx, rep("L1"+sfx), "d1", 2); err != nil || c {
		t.Fatalf("duplicate must not count: %v %v", c, err)
	}
	if c, err := s.AddReport(ctx, rep("L2"+sfx), "d1", 2); err != nil || !c {
		t.Fatalf("second: %v %v", c, err)
	}
	if _, err := s.AddReport(ctx, rep("L3"+sfx), "d1", 2); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("quota: %v", err)
	}
	if c, err := s.AddReport(ctx, rep("L3"+sfx), "d2", 2); err != nil || !c {
		t.Fatalf("next day: %v %v", c, err)
	}
	rs, err := s.ListReports(ctx, 100)
	must(err)
	n := 0
	for _, r := range rs {
		if r.ReporterUID == a {
			n++
		}
	}
	if n != 3 {
		t.Fatalf("reports: %d %+v", n, rs)
	}
	must(s.SetReportStatus(ctx, rep("L1"+sfx).ID, "done"))
	if err := s.SetReportStatus(ctx, "rpt_none"+sfx, "done"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing report: %v", err)
	}

	// blocks
	must(s.PutBlock(ctx, a, Block{UID: b, Name: "B"}))
	must(s.PutBlock(ctx, a, Block{UID: b, Name: "B2"}))
	bs, err := s.ListBlocks(ctx, a)
	must(err)
	if len(bs) != 1 || bs[0].UID != b || bs[0].Name != "B2" || !BlockedSet(bs)[b] {
		t.Fatalf("blocks: %+v", bs)
	}
	must(s.DeleteBlock(ctx, a, b))
	must(s.DeleteBlock(ctx, a, b))
	if bs, _ = s.ListBlocks(ctx, a); len(bs) != 0 {
		t.Fatalf("after unblock: %+v", bs)
	}

	// operator unpublish survives the owner's republish
	dsl := json.RawMessage(`{}`)
	if err := s.SetCatalogHidden(ctx, "none"+sfx, true, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing listing: %v", err)
	}
	id := "mod-app" + sfx
	_, err = s.PutCatalogApp(ctx, a, CatalogApp{ID: id, Name: "x", DSL: dsl})
	must(err)
	must(s.SetCatalogHidden(ctx, id, true, "spam"))
	_, err = s.PutCatalogApp(ctx, a, CatalogApp{ID: id, Name: "y", DSL: dsl})
	must(err)
	c, err := s.GetCatalogApp(ctx, id)
	must(err)
	if !c.Hidden || c.HiddenReason != "spam" || c.HiddenAt.IsZero() {
		t.Fatalf("listing state: %+v", c.ModerationState)
	}
	must(s.SetCatalogHidden(ctx, id, false, ""))
	if c, _ = s.GetCatalogApp(ctx, id); c.Hidden || c.HiddenReason != "" {
		t.Fatalf("unhidden: %+v", c.ModerationState)
	}

	if _, err := s.PublishApp(ctx, a, id, dsl, "x", 1); err != nil {
		t.Fatal(err)
	}
	must(s.SetPublicAppHidden(ctx, id, true, "abuse"))
	if _, err := s.PublishApp(ctx, a, id, dsl, "x", 2); err != nil {
		t.Fatal(err)
	}
	p, err := s.GetPublicApp(ctx, id)
	must(err)
	if !p.Hidden || p.HiddenReason != "abuse" {
		t.Fatalf("public app state: %+v", p.ModerationState)
	}
	if err := s.SetPublicAppHidden(ctx, "none"+sfx, true, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing app: %v", err)
	}
}
