package engine

import (
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

// Assignment by pick (dsl/server_rules_assign.go): order fewest / most
// compares how many records of pick.count point to each candidate.

// countLoads is candidate id -> how many records of count.entity point to
// it (listed through the run's reader, capped and counted like each / agg).
// The row itself and the records this run creates are not counted (a retry
// counts what the first attempt counted); the rows assigned earlier in this
// run are. nil when the pick has no count.
func countLoads(rule *dsl.ServerRule, ev Trigger, row *expreval.Row, rowKey string, src *refSource, c *runCache) map[string]int {
	cnt := rule.Pick.Count
	if cnt == nil || !dsl.IsCountOrder(rule.Pick.Order) {
		return nil
	}
	recs := src.list(cnt.Entity)
	if src.err != nil {
		return nil
	}
	own := ownCreated(rule, ev, rowKey, cnt.Entity)
	out := map[string]int{}
	for _, r := range recs {
		if own[r.ID] || (row != nil && row.Entity != nil && row.Entity.ID == cnt.Entity && r.ID == row.Record.ID) {
			continue
		}
		for _, id := range refIDs(r.Data[cnt.Field]) {
			out[id]++
		}
	}
	for id, n := range c.assigned[rule.ID] {
		out[id] += n
	}
	return out
}

// loadBetter reports whether r beats best: the fewer (most: the more)
// records, ties to the one created first, then the smaller id, so the
// choice does not depend on the order the records are listed in.
func loadBetter(order string, load map[string]int, r, best *expreval.Record) bool {
	a, b := load[r.ID], load[best.ID]
	if a != b {
		return (a < b) == (order == dsl.PickFewest)
	}
	if r.CreatedAt != best.CreatedAt {
		return r.CreatedAt != "" && (best.CreatedAt == "" || r.CreatedAt < best.CreatedAt)
	}
	return r.ID < best.ID
}

// noteAssigned counts the pick of this row for the later rows of the run
// when the rule writes "$pick" into count.field: of the row (updateRecord
// without ref; the row's previous value is then one less) or of a new
// record of count.entity (createRecord).
func (c *runCache) noteAssigned(rule *dsl.ServerRule, row, picked *expreval.Row) {
	p := rule.Pick
	if p.Count == nil || !dsl.IsCountOrder(p.Order) || picked == nil {
		return
	}
	id, _ := picked.Record.Data[dsl.PickRef].(string)
	if id == "" {
		return
	}
	writesPick := func(values map[string]dsl.Expr) bool {
		v, ok := values[p.Count.Field]
		return ok && v.Kind == dsl.ExprRef && v.Ref == dsl.PickRef
	}
	delta := 0
	prev := ""
	for _, act := range rule.Do {
		switch {
		case act.UpdateRecord != nil && act.UpdateRecord.Ref == "" && row != nil && row.Entity != nil &&
			row.Entity.ID == p.Count.Entity && writesPick(act.UpdateRecord.Values):
			delta++
			prev, _ = row.Record.Data[p.Count.Field].(string)
		case act.CreateRecord != nil && act.CreateRecord.Entity == p.Count.Entity && writesPick(act.CreateRecord.Values):
			delta++
		}
	}
	if delta == 0 {
		return
	}
	if c.assigned == nil {
		c.assigned = map[string]map[string]int{}
	}
	m := c.assigned[rule.ID]
	if m == nil {
		m = map[string]int{}
		c.assigned[rule.ID] = m
	}
	m[id] += delta
	if prev != "" && prev != id {
		m[prev]-- // listed with its old value, now moved
	}
}

// ownCreated is the ids of the records of entity this run's createRecord
// actions create for rowKey.
func ownCreated(rule *dsl.ServerRule, ev Trigger, rowKey, entity string) map[string]bool {
	own := map[string]bool{}
	if ev.CreatedID == nil {
		return own
	}
	for i, act := range rule.Do {
		if act.CreateRecord != nil && act.CreateRecord.Entity == entity {
			own[ev.CreatedID(rule.ID, i, rowKey)] = true
		}
	}
	return own
}

// excludeMatch is exclude.match as a test of one exclude record: its field
// equals the row's (compared like lookup). Without match every record
// counts; without a row (or with an empty row value) none does.
func excludeMatch(app *dsl.App, x *dsl.PickExclude, row *expreval.Row) func(expreval.Record) bool {
	m := x.Match
	if m == nil {
		return func(expreval.Record) bool { return true }
	}
	if row == nil {
		return func(expreval.Record) bool { return false }
	}
	ids := app.PickMatchComparesIDs(x)
	value := func(r expreval.Record, field string) string {
		if field == dsl.LookupID {
			return dsl.LookupValueKey(r.ID, ids)
		}
		return dsl.LookupValueKey(r.Data[field], ids)
	}
	want := value(row.Record, m.Equals)
	return func(r expreval.Record) bool { return want != "" && value(r, m.Field) == want }
}

// refIDs is the ids of a ref (string) or refs ([]any) value.
func refIDs(v any) []string {
	switch x := v.(type) {
	case string:
		if x != "" {
			return []string{x}
		}
	case []any:
		out := make([]string, 0, len(x))
		seen := map[string]bool{}
		for _, id := range x {
			if s, ok := id.(string); ok && s != "" && !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
