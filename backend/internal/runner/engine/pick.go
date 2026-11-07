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
func pickRow(app *dsl.App, rule *dsl.ServerRule, ev Trigger, row *expreval.Row, src *refSource, e *expreval.Evaluator) (*expreval.Row, bool) {
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
	if p.Exclude != nil {
		skip = excluded(rule, ev, rowKey, src)
		if src.err != nil {
			return nil, false
		}
	}
	seed := ev.RunID + "\x00" + rule.ID + "\x00" + rowKey
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
		}
	}
	if best == nil {
		return nil, false
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
// not counted, so the retry picks what the first attempt picked.
func excluded(rule *dsl.ServerRule, ev Trigger, rowKey string, src *refSource) map[string]bool {
	x := rule.Pick.Exclude
	own := map[string]bool{}
	if ev.CreatedID != nil {
		for i, act := range rule.Do {
			if act.CreateRecord != nil && act.CreateRecord.Entity == x.Entity {
				own[ev.CreatedID(rule.ID, i, rowKey)] = true
			}
		}
	}
	recs := src.list(x.Entity)
	if src.err != nil {
		return nil
	}
	out := map[string]bool{}
	for _, r := range recs {
		if own[r.ID] {
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
