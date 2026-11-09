package runner

import "github.com/jphacks/kz_2601/backend/internal/runner/quota"

// ReasonAppLimit: an ok run whose writes or notifications the app's daily
// limit cut (#345). The creator sees "limit", not "ok": later runs of the
// day write or send nothing more.
const ReasonAppLimit = "app_limit"

// appLimited: the scope is one of the app's daily counters that cut part
// of a run (per-run caps and per-person limits are not).
func appLimited(scope string) bool {
	return scope == quota.ScopeAppWrites || scope == quota.ScopeAppNotify
}
