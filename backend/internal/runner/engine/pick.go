package engine

import (
	"crypto/sha256"
	"encoding/binary"
	"maps"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

// pickRow evaluates rule.Pick: the entity (and exclude's) is listed through
// the run's reader (capped and counted like each / agg), filtered and
// ordered in memory. ok=false when no record is left; otherwise the
// returned row is row (or, without one, a row of the results only) with
// "$pick" for the actions.
func pickRow(app *dsl.App, rule *dsl.ServerRule, ev Trigger, row *expreval.Row, src *refSource, e *expreval.Evaluator, c *runCache) (*expreval.Row, bool) {
	p := rule.Pick
	target := app.EntityByID(p.Entity)
	if target == nil {
		return nil, false
	}
	src.listed[p.Entity] = true // Matching and "$pick.<field>" read the listed records
	recs := e.Matching(p.Entity, p.Filter, row)
	if src.err != nil {
		return nil, false
	}
	rowKey := ""
	if rule.On.Each != "" && row != nil {
		rowKey = row.Record.ID
	}
	skip := map[string]bool{}
	// once per run: exclude also skips what earlier rows picked (their
	// creates are not written yet). fewest / most instead count earlier rows
	// (countLoads), so a candidate may be chosen again while least loaded.
	once := p.Exclude != nil && !dsl.IsCountOrder(p.Order)
	if p.Exclude != nil {
		skip = excluded(app, rule, ev, row, rowKey, src)
		if src.err != nil {
			return nil, false
		}
		if once {
			for id := range c.picked[rule.ID] {
				skip[id] = true
			}
		}
	}
	seed := ev.RunID + "\x00" + rule.ID + "\x00" + rowKey
	load := countLoads(rule, ev, row, rowKey, src, c)
	if src.err != nil {
		return nil, false
	}
	var best *expreval.Record
	var bestKey uint64
	for i := range recs {
		r := &recs[i]
		if r.ID == "" || skip[r.ID] || (row != nil && row.Entity != nil && row.Entity.ID == target.ID && r.ID == row.Record.ID) {
			continue
		}
		switch p.Order {
		case dsl.PickRandom:
			// The smallest hash of (seed, id): the same record on a retry of
			// the run, whatever order the records are listed in.
			k := pickHash(seed, r.ID)
			if best == nil || k < bestKey {
				best, bestKey = r, k
			}
		case dsl.PickNewest: // listed newest first; ties keep the first listed
			if best == nil || r.CreatedAt > best.CreatedAt {
				best = r
			}
		case dsl.PickOldest: // ties go to the last listed (the older)
			if best == nil || r.CreatedAt <= best.CreatedAt {
				best = r
			}
		case dsl.PickFewest, dsl.PickMost: // most: at least one record points to it
			if (p.Order == dsl.PickFewest || load[r.ID] > 0) && (best == nil || loadBetter(p.Order, load, r, best)) {
				best = r
			}
		}
	}
	if best == nil {
		return nil, false
	}
	if once {
		if c.picked[rule.ID] == nil {
			c.picked[rule.ID] = map[string]bool{}
		}
		c.picked[rule.ID][best.ID] = true
	}
	out := expreval.Row{Record: expreval.Record{}}
	if row != nil {
		out = *row
	}
	out.Entity = dsl.PickRow(out.Entity, target, p)
	data := maps.Clone(out.Record.Data)
	if data == nil {
		data = map[string]any{}
	}
	data[dsl.PickRef] = best.ID
	out.Record.Data = data
	return &out, true
}

// excluded is the ids exclude.field points to in the records of
// exclude.entity. A record this run creates (a retry after the create) is
// not counted, so the retry picks what the first attempt picked. With
// exclude.match only the records matching the row count.
func excluded(app *dsl.App, rule *dsl.ServerRule, ev Trigger, row *expreval.Row, rowKey string, src *refSource) map[string]bool {
	x := rule.Pick.Exclude
	own := ownCreated(rule, ev, rowKey, x.Entity)
	match := excludeMatch(app, x, row)
	recs := src.list(x.Entity)
	if src.err != nil {
		return nil
	}
	out := map[string]bool{}
	for _, r := range recs {
		if own[r.ID] || !match(r) {
			continue
		}
		switch v := r.Data[x.Field].(type) {
		case string:
			out[v] = true
		case []any:
			for _, id := range v {
				if s, ok := id.(string); ok {
					out[s] = true
				}
			}
		}
	}
	return out
}

func pickHash(seed, id string) uint64 {
	sum := sha256.Sum256([]byte(seed + "\x00" + id))
	return binary.BigEndian.Uint64(sum[:8])
}
