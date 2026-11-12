package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"slices"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
)

var (
	inboxSources  = []loop.Source{loop.SourceInApp, loop.SourceStoreReview, loop.SourceCrash, loop.SourceTestFlight, loop.SourceGenFailure}
	inboxStatuses = []string{loop.ItemNew, loop.ItemHeld, loop.ItemTriaged, loop.ItemSpam, loop.ItemDrafted}
)

// inbox lists every source in one list (#368), oldest ingested first.
// Metadata only: the text is never printed. Read-only, so not behind a stop switch.
func (a *app) inbox(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("inbox", flag.ContinueOnError)
	src := fs.String("source", "", "only this source (in_app / app_store_review / crash / testflight / gen_failure)")
	status := fs.String("status", "", "only this status (new / held / triaged / spam / drafted)")
	limit := fs.Int("limit", 100, "max items (1-2000)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("inbox takes no arguments: %v", fs.Args())
	}
	if *src != "" && !slices.Contains(inboxSources, loop.Source(*src)) {
		return fmt.Errorf("unknown source %q", *src)
	}
	if *status != "" && !slices.Contains(inboxStatuses, *status) {
		return fmt.Errorf("unknown status %q", *status)
	}
	if *limit < 1 || *limit > 2000 {
		return errors.New("-limit must be 1-2000")
	}
	items, err := a.loop.ListItems(ctx, loop.ItemQuery{Source: loop.Source(*src), Status: *status, Limit: *limit})
	if err != nil {
		return err
	}
	const row = "%-28s %-16s %-8s %-22s %-10s %-4s %s\n"
	fmt.Fprintf(a.out, row, "ID", "SOURCE", "STATUS", "HOLD", "VERSION", "OS", "CREATED")
	for _, it := range items {
		fmt.Fprintf(a.out, row, it.ID, it.Source, it.Status, dash(it.HoldReason), dash(it.AppVersion), dash(it.OSMajor), createdAt(it.CreatedAt))
	}
	fmt.Fprintf(a.out, "%d item(s)\n", len(items))
	return nil
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func createdAt(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}
