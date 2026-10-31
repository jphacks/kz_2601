package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

func TestRun(t *testing.T) {
	ctx := context.Background()
	m := store.NewMemory()
	if _, err := m.PutCatalogApp(ctx, "owner", store.CatalogApp{ID: "L1", DSL: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AddReport(ctx, store.Report{ID: "rpt_1", ReporterUID: "v", Kind: store.ReportListing, TargetID: "L1", Reason: "spam", Status: "open"}, "d", 0); err != nil {
		t.Fatal(err)
	}
	do := func(args ...string) (string, error) {
		var b strings.Builder
		err := run(ctx, m, args, &b)
		return b.String(), err
	}

	if out, err := do("reports"); err != nil || !strings.Contains(out, "rpt_1\topen\tlisting\tL1\tspam") {
		t.Fatalf("reports: %q %v", out, err)
	}
	if _, err := do("hide-listing", "L1"); err == nil {
		t.Fatal("-reason must be required")
	}
	// dry run by default
	if out, err := do("hide-listing", "L1", "-reason", "不適切"); err != nil || !strings.HasPrefix(out, "dry run") {
		t.Fatalf("dry run: %q %v", out, err)
	}
	if c, _ := m.GetCatalogApp(ctx, "L1"); c.Hidden {
		t.Fatal("dry run wrote")
	}
	if _, err := do("hide-listing", "-yes", "L1", "-reason", "不適切"); err != nil {
		t.Fatal(err)
	}
	if c, _ := m.GetCatalogApp(ctx, "L1"); !c.Hidden || c.HiddenReason != "不適切" {
		t.Fatalf("hidden: %+v", c.ModerationState)
	}
	if _, err := do("resolve", "rpt_1", "-yes"); err != nil {
		t.Fatal(err)
	}
	if out, _ := do("reports"); out != "" {
		t.Fatalf("handled reports are hidden by default: %q", out)
	}
	if out, _ := do("reports", "-all"); !strings.Contains(out, "\tdone\t") {
		t.Fatalf("-all: %q", out)
	}
	if _, err := do("hide-app", "NOPE", "-reason", "x", "-yes"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing app: %v", err)
	}
	for _, args := range [][]string{{}, {"bogus"}, {"delete-record", "a"}, {"resolve", "rpt_1", "-status", "x"}} {
		if _, err := do(args...); !errors.Is(err, errUsage) {
			t.Fatalf("%v: %v", args, err)
		}
	}
}
