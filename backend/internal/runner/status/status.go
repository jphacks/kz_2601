// Package status turns the runner's outcomes into the fixed words the
// creator's screen shows (#347). It holds no runner logic and imports
// nothing of the runner, so appconfig can use it; runner's tests check
// that every outcome and reason it has maps to one of these words.
package status

// Results: what the creator sees for a rule's latest run (openapi
// AutomationResult). Internal codes (limits' scopes, stop levels, report
// counts, operator notes) never leave the backend.
const (
	ResultNone          = "none"           // no run yet (or none within store.RuleStatusTTL)
	ResultOK            = "ok"             // ran
	ResultNotifyPaused  = "notify_paused"  // ran, notifications not sent (a stop of notifications)
	ResultLimit         = "limit"          // today's limit of the app / creator / person was reached
	ResultServiceLimit  = "service_limit"  // the service's overall daily limit was reached
	ResultServicePaused = "service_paused" // the service paused every automation
	ResultPlan          = "plan"           // the creator's plan cannot run it (anonymous, or no Pro: #316)
	ResultStopped       = "stopped"        // the operator stopped the app or the rule (or unpublished the app)
	ResultChanged       = "changed"        // the app was changed and the rule is no longer in it
	ResultUnavailable   = "unavailable"    // the app or its creator's account was not usable
	ResultLate          = "late"           // the trigger was too old to run (not run later)
	ResultInactive      = "inactive"       // nobody opened the app for a while (opening resumes it)
	ResultFailed        = "failed"         // tried several times and gave up
	ResultDisabled      = "disabled"       // the creator turned the rule off
	ResultOther         = "other"          // an outcome this build does not know
)

// Outcomes and reasons as the runner writes them (runner.Outcome*,
// quota.Outcome*, queue.OutcomeGaveUp, runner.Reason*).
const (
	outcomeOK            = "ok"
	outcomeSkippedQuota  = "skipped_quota"
	outcomeSkippedGlobal = "skipped_global"
	outcomeSkippedPaused = "skipped_paused"
	outcomeSkippedPlan   = "skipped_plan"
	outcomeSkippedStop   = "skipped_stopped"
	outcomeSkippedStale  = "skipped_stale"
	outcomeSkippedLate   = "skipped_late"
	outcomeSkippedIdle   = "skipped_inactive"
	outcomeGaveUp        = "gave_up"
	outcomeSkippedOff    = "skipped_disabled"

	reasonAppHidden      = "app_hidden"
	reasonOperatorStop   = "operator_stop"
	reasonRuleRemoved    = "rule_removed"
	reasonVersionMissing = "version_missing"
	reasonAppLimit       = "app_limit" // ok, but the app's daily limit cut writes / notifications
)

// Result is the creator's word for one rule run. notifyPaused: the run was
// ok but sent no notification.
func Result(outcome, reason string, notifyPaused bool) string {
	switch outcome {
	case "":
		return ResultNone
	case outcomeOK:
		if reason == reasonAppLimit {
			return ResultLimit
		}
		if notifyPaused {
			return ResultNotifyPaused
		}
		return ResultOK
	case outcomeSkippedQuota:
		return ResultLimit
	case outcomeSkippedGlobal:
		return ResultServiceLimit
	case outcomeSkippedPaused:
		return ResultServicePaused
	case outcomeSkippedPlan:
		return ResultPlan
	case outcomeSkippedStale, outcomeSkippedLate:
		return ResultLate
	case outcomeSkippedIdle:
		return ResultInactive
	case outcomeGaveUp:
		return ResultFailed
	case outcomeSkippedOff:
		return ResultDisabled
	case outcomeSkippedStop:
		switch reason {
		case reasonOperatorStop, reasonAppHidden:
			return ResultStopped
		case reasonRuleRemoved, reasonVersionMissing:
			return ResultChanged
		}
		return ResultUnavailable
	}
	return ResultOther
}

// Failure reasons: why a failed run gave up, in the fixed words of openapi
// AutomationFailureReason. Only these leave the backend: never the error,
// a path, an id or a scope violation's detail (#347).
const (
	FailureNotifyFailed      = "notify_failed"      // a notification could not be sent
	FailureRecordUnavailable = "record_unavailable" // the record to read or change was gone or not readable
	FailureMismatch          = "mismatch"           // the rule does not fit the app's data any more
	FailureWriteRejected     = "write_rejected"     // a record could not be saved (e.g. a duplicate value)
	FailureSourceUnavailable = "source_unavailable" // an outside source (feed) could not be read
	FailureTimeout           = "timeout"            // the run did not finish in time
	FailureInternal          = "internal"           // a problem on the service's side (or not known)
)

// Error codes as the runner writes them to the outbox (runner.errorCode,
// and "feed_wait" of a deferred run).
const (
	CodeSendFailed     = "send_failed"
	CodeScopeViolation = "scope_violation"
	CodeNotReadable    = "not_readable"
	CodeNotFound       = "not_found"
	CodeMismatch       = "mismatch"
	CodeWriteRejected  = "write_rejected"
	CodeFeedWait       = "feed_wait"
	CodeTimeout        = "timeout"
	CodeError          = "error"
)

// FailureReason is the creator's word for a run that gave up with the
// runner's error code. A scope violation is the service's own problem.
func FailureReason(code string) string {
	switch code {
	case CodeSendFailed:
		return FailureNotifyFailed
	case CodeNotReadable, CodeNotFound:
		return FailureRecordUnavailable
	case CodeMismatch:
		return FailureMismatch
	case CodeWriteRejected:
		return FailureWriteRejected
	case CodeFeedWait:
		return FailureSourceUnavailable
	case CodeTimeout:
		return FailureTimeout
	}
	return FailureInternal
}

// IsFailureReason reports whether w is one of the Failure* words.
func IsFailureReason(w string) bool {
	switch w {
	case FailureNotifyFailed, FailureRecordUnavailable, FailureMismatch, FailureWriteRejected,
		FailureSourceUnavailable, FailureTimeout, FailureInternal:
		return true
	}
	return false
}
