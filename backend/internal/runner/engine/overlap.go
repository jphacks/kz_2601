package engine

import (
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

// overlapHolds evaluates an overlaps / notOverlaps lookup for row
// (dsl/server_rules_overlap.go): the entity is listed once per run through
// the run's reader (capped and counted like each / agg, shared by the rows of
// PlanEach) and the periods are compared in memory. The row itself is never
// its own match; a row without a period fires neither op, a listed record
// without one is skipped.
func overlapHolds(l *dsl.RuleLookup, row *expreval.Row, src *refSource, loc *time.Location) bool {
	if row == nil {
		return false
	}
	from, to, ok := expreval.Period(row.Record.Data[l.Equals], endValue(row.Record.Data, l.EqualsEnd), loc)
	if !ok {
		return false
	}
	recs := src.list(l.Entity)
	if src.err != nil {
		return false
	}
	found := false
	for _, rec := range recs {
		if l.Entity == row.Entity.ID && rec.ID == row.Record.ID {
			continue
		}
		rf, rt, ok := expreval.Period(rec.Data[l.Field], endValue(rec.Data, l.FieldEnd), loc)
		if ok && expreval.Overlaps(from, to, rf, rt) {
			found = true
			break
		}
	}
	return found == (l.Op == dsl.LookupOverlaps)
}

func endValue(data map[string]any, field string) any {
	if field == "" {
		return nil
	}
	return data[field]
}
