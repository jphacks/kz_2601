package engine

import (
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

// runCache is what one run reads once and shares between the rows of
// PlanEach: the listed records of an entity and the lookup indexes built
// from them (one list per entity, one index per entity + field).
type runCache struct {
	lists map[string][]expreval.Record
	index map[string]map[string][]string // entity\x00field -> value key -> record ids (at most 2)
	// assigned: rule id -> candidate id -> assignments planned by earlier
	// rows of this run (pick.count; written after planning). nil until used.
	assigned map[string]map[string]int
	// similar: entity\x00fields -> the records with their compared items.
	similar map[string][]similarRec
	// picked: rule id -> the records pick with exclude chose for earlier
	// rows of this run (the writes come after planning, so exclude's list
	// does not see them yet).
	picked map[string]map[string]bool
}

func newRunCache() *runCache {
	return &runCache{lists: map[string][]expreval.Record{}, index: map[string]map[string][]string{}, similar: map[string][]similarRec{}, picked: map[string]map[string]bool{}}
}

// lookupHolds evaluates rule.Lookup for row: the entity is listed through
// the run's reader (capped and counted like each / agg) and matched in
// memory. The row itself is never its own match; a row whose value is empty
// fires neither op.
func lookupHolds(app *dsl.App, l *dsl.RuleLookup, row *expreval.Row, src *refSource, c *runCache) bool {
	if row == nil {
		return false
	}
	ids := app.LookupComparesIDs(l)
	var v any
	if l.Equals == dsl.LookupID {
		v = row.Record.ID
	} else {
		v = row.Record.Data[l.Equals]
	}
	key := dsl.LookupValueKey(v, ids)
	if key == "" {
		return false
	}
	idx := c.lookupIndex(l, ids, src)
	if src.err != nil {
		return false
	}
	found := false
	for _, id := range idx[key] {
		if l.Entity != row.Entity.ID || id != row.Record.ID {
			found = true
		}
	}
	return found == (l.Op == dsl.LookupExists)
}

// lookupIndex builds (once per run) value key -> ids of the records of
// l.Entity. Two ids per key are enough to know whether a record other than
// the row matches.
func (c *runCache) lookupIndex(l *dsl.RuleLookup, ids bool, src *refSource) map[string][]string {
	k := l.Entity + "\x00" + l.Field
	if hit, ok := c.index[k]; ok {
		return hit
	}
	recs := src.list(l.Entity)
	if src.err != nil {
		return nil
	}
	idx := make(map[string][]string, len(recs))
	for _, rec := range recs {
		var v any
		if l.Field == dsl.LookupID {
			v = rec.ID
		} else {
			v = rec.Data[l.Field]
		}
		key := dsl.LookupValueKey(v, ids)
		if key == "" || len(idx[key]) >= 2 {
			continue
		}
		idx[key] = append(idx[key], rec.ID)
	}
	c.index[k] = idx
	return idx
}
