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
// without one is skipped. Whole days or instants follow the field's type,
// not the form of the stored value.
func overlapHolds(app *dsl.App, l *dsl.RuleLookup, row *expreval.Row, src *refSource, c *runCache, loc *time.Location) bool {
	if row == nil {
		return false
	}
	from, to, ok := expreval.Period(row.Record.Data[l.Equals], endValue(row.Record.Data, l.EqualsEnd), isDateField(row.Entity, l.Equals), loc)
	if !ok {
		return false
	}
	periods := c.overlapPeriods(app, l, src, loc)
	if src.err != nil {
		return false
	}
	found := false
	for _, p := range periods {
		if l.Entity == row.Entity.ID && p.id == row.Record.ID {
			continue
		}
		if expreval.Overlaps(from, to, p.from, p.to) {
			found = true
			break
		}
	}
	return found == (l.Op == dsl.LookupOverlaps)
}

type recPeriod struct {
	id       string
	from, to time.Time
}

// overlapPeriods parses (once per run) the periods of the records of
// l.Entity; records without a period are left out.
func (c *runCache) overlapPeriods(app *dsl.App, l *dsl.RuleLookup, src *refSource, loc *time.Location) []recPeriod {
	k := l.Entity + "\x00" + l.Field + "\x00" + l.FieldEnd
	if hit, ok := c.periods[k]; ok {
		return hit
	}
	recs := src.list(l.Entity)
	if src.err != nil {
		return nil
	}
	day := isDateField(app.EntityByID(l.Entity), l.Field)
	out := make([]recPeriod, 0, len(recs))
	for _, rec := range recs {
		if f, t, ok := expreval.Period(rec.Data[l.Field], endValue(rec.Data, l.FieldEnd), day, loc); ok {
			out = append(out, recPeriod{id: rec.ID, from: f, to: t})
		}
	}
	c.periods[k] = out
	return out
}

func isDateField(e *dsl.Entity, id string) bool {
	if e == nil {
		return false
	}
	f := e.FieldByID(id)
	return f != nil && f.Type == dsl.FieldDate
}

func endValue(data map[string]any, field string) any {
	if field == "" {
		return nil
	}
	return data[field]
}
