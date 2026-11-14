package appconfig

import (
	"net/http"

	"github.com/jphacks/kz_2601/backend/internal/metrics"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// sharedUsage counts one read or write of a shared entity's records for the
// caller (shared.usage, #2298; off unless SHARED_USAGE_LOG=on). Records of
// a personal entity are not counted.
func sharedUsage(r *http.Request, appID string, ent *dsl.Entity, kind string, n int) {
	if ent == nil || !ent.IsShared() {
		return
	}
	metrics.EmitSharedUsage(r.Context(), appID, uidOf(r), kind, n)
}
