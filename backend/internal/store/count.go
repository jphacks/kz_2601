package store

import (
	"context"
	"fmt"
)

// Counter counts what the plan limits (#317, internal/plan) cap. It is kept
// out of Store so tools and fakes need not implement it; appconfig checks
// the limits only when its store has it (Firestore and Memory do).
type Counter interface {
	// CountApps is the number of apps uid holds (#630 app slots): the
	// users/{uid}/apps past version 1, its own and the shared ones it joined.
	// A blank app (version 1, never generated; iOS does not list it) is not
	// held. Commit.HeldLimit counts the same way.
	CountApps(ctx context.Context, uid string) (int, error)
	// CountRecords is the number of records in rs; with field != "" only
	// those whose data.<field> is set (not missing, not null). A missing
	// private app counts 0.
	CountRecords(ctx context.Context, rs RecordSet, field string) (int, error)
}

// HeldVersion: an app is held once its currentVersion is above this (version
// 1 is the blank app ensureApp seeds; iOS's AppSummary.isBlank).
const HeldVersion = 1

// HeldLimitError: the commit would make a blank app held while uid already
// holds Limit apps (Commit.HeldLimit). Nothing was written.
type HeldLimitError struct{ Held, Limit int }

func (e *HeldLimitError) Error() string {
	return fmt.Sprintf("held app limit: %d of %d", e.Held, e.Limit)
}
