package runner

import (
	"context"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

// OutcomeSkippedDisabled: the creator turned the rule off (enabled: false in
// the current DSL, #347). Not counted against any quota; appconfig queues
// nothing for a rule that is off, so this only ends runs queued before.
const OutcomeSkippedDisabled = "skipped_disabled"

func (r *Runner) ruleDisabled(ctx context.Context, ev store.OutboxEvent, ruleID string) string {
	r.logRun(ctx, ev, runLog{ruleID: ruleID, outcome: OutcomeSkippedDisabled})
	return OutcomeSkippedDisabled
}
