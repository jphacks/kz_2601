package store

import "context"

// Counter counts what the plan limits (#317, internal/plan) cap. It is kept
// out of Store so tools and fakes need not implement it; appconfig checks
// the limits only when its store has it (Firestore and Memory do).
type Counter interface {
	// CountApps is the number of users/{uid}/apps.
	CountApps(ctx context.Context, uid string) (int, error)
	// CountRecords is the number of records in rs; with field != "" only
	// those whose data.<field> is set (not missing, not null). A missing
	// private app counts 0.
	CountRecords(ctx context.Context, rs RecordSet, field string) (int, error)
}
