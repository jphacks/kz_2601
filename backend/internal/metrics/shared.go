package metrics

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"
)

// Shared-app usage (#2298, for #1450): one line per read, write or
// addressed notification in a shared app, so the per-participant numbers can
// be seen before deciding what participants pay. Counting only: nothing is
// charged. Labels and hashes only: never the record, the appId or the uid.
const (
	// SharedUsage is the event (and message) of the lines.
	SharedUsage = "shared.usage"
	// EnvSharedUsage turns the lines on ("on"); default off.
	EnvSharedUsage = "SHARED_USAGE_LOG"
)

// Kinds of shared.usage.
const (
	SharedRead   = "read"   // records read from the store for one request
	SharedWrite  = "write"  // records created, changed or deleted
	SharedNotify = "notify" // addressed notifications received
)

var sharedUsageOn atomic.Bool

func init() { sharedUsageOn.Store(os.Getenv(EnvSharedUsage) == "on") }

// SetSharedUsage switches the lines on or off and returns the previous
// setting (tests; the server reads SHARED_USAGE_LOG at start).
func SetSharedUsage(on bool) bool { return sharedUsageOn.Swap(on) }

// EmitSharedUsage writes one shared.usage line: app_hash, uid_hash, kind and
// count. Nothing when off, when count <= 0 or without an app.
func EmitSharedUsage(ctx context.Context, appID, uid, kind string, count int) {
	if !sharedUsageOn.Load() || count <= 0 || appID == "" {
		return
	}
	Emit(ctx, SharedUsage, uid, slog.String("app_hash", AppHash(appID)), slog.String("kind", kind), slog.Int("count", count))
}
