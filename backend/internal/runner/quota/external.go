package quota

import (
	"context"

	"github.com/jphacks/kz_2601/backend/internal/coins"
)

// ScopeExtNeedsPro: an external API call whose creator has no Pro. External
// APIs are Pro only in every PRO_GATE_MODE (docs/decisions/external-apis.md
// 決定 1).
const ScopeExtNeedsPro = "ext_needs_pro"

// ExtRun counts one external API call as one `runs` of the creator's month
// (docs/decisions/external-apis.md「数え方」): the same counter and overflow
// as a run, no new kind. in is the Decision of the server rule run the call
// is part of; nil for a call a person pressed (then the creator's Pro is
// checked here). A non-nil Block means the call must not happen; nothing
// was counted. Give the Charge back with Refund when the call failed.
func (q *Quota) ExtRun(ctx context.Context, r Run, in *Decision) (Charge, *Block, error) {
	var dec Decision
	if in != nil {
		dec = *in
		if !dec.Allowed {
			b := blockOr(dec.Block)
			return Charge{}, &b, nil
		}
		if dec.Plan != PlanPro {
			return Charge{}, &Block{Scope: ScopeExtNeedsPro}, nil
		}
	} else {
		if r.CreatorUID == "" || r.CreatorAnonymous || q.pro == nil || !q.pro.IsPro(ctx, r.CreatorUID) {
			return Charge{}, &Block{Scope: ScopeExtNeedsPro}, nil
		}
		r.Attempt = 1
		dec = Decision{Allowed: true, Outcome: OutcomeOK, Plan: PlanPro, run: r}
		dec.month = q.newMonth(q.cfg.Now(), PlanPro, r)
	}
	part, stop, err := q.allow(ctx, dec, coins.KindRuns, 1)
	if err != nil || stop != nil {
		return Charge{}, stop, err
	}
	return appendPart(Charge{}, part), nil, nil
}
