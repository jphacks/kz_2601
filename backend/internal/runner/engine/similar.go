package engine

import (
	"maps"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

// similarRec is one listed record with the items of the compared fields.
type similarRec struct {
	id, author string
	items      []map[string]bool // per field of RuleSimilar.Fields
}

// similarRow evaluates rule.Similar for row: the entity is listed through
// the run's reader (capped and counted like each / agg) and compared in
// memory. ok=false when no record shares MinShared items; otherwise the
// returned row is row with "$similar" / "$similarScore" for the actions.
func similarRow(app *dsl.App, s *dsl.RuleSimilar, row *expreval.Row, src *refSource, c *runCache) (*expreval.Row, bool) {
	if row == nil || row.Entity == nil {
		return nil, false
	}
	target := app.EntityByID(s.Entity)
	if target == nil {
		return nil, false
	}
	mine := make([]map[string]bool, len(s.Fields))
	has := false
	for i, f := range s.Fields {
		mine[i] = dsl.SimilarItems(row.Entity.FieldByID(f), row.Record.Data[f])
		has = has || len(mine[i]) > 0
	}
	if !has {
		return nil, false
	}
	recs := c.similarRecs(s, target, src)
	if src.err != nil {
		return nil, false
	}
	shared := target.IsShared()
	best, score := "", 0
	for _, r := range recs {
		if (target.ID == row.Entity.ID && r.id == row.Record.ID) || (shared && r.author != "" && r.author == row.Record.Author) {
			continue
		}
		n := 0
		for i := range mine {
			for k := range mine[i] {
				if r.items[i][k] {
					n++
				}
			}
		}
		if n > score {
			best, score = r.id, n
		}
	}
	if best == "" || score < s.MinShared() {
		return nil, false
	}
	data := maps.Clone(row.Record.Data)
	if data == nil {
		data = map[string]any{}
	}
	data[dsl.SimilarRef], data[dsl.SimilarScore] = best, float64(score)
	out := *row
	out.Entity = dsl.SimilarRow(row.Entity, s)
	out.Record.Data = data
	return &out, true
}

// similarRecs lists (once per run) the records of the searched entity with
// their compared items.
func (c *runCache) similarRecs(s *dsl.RuleSimilar, target *dsl.Entity, src *refSource) []similarRec {
	k := target.ID + "\x00" + strings.Join(s.Fields, "\x00")
	if hit, ok := c.similar[k]; ok {
		return hit
	}
	recs := src.list(target.ID)
	if src.err != nil {
		return nil
	}
	out := make([]similarRec, 0, len(recs))
	for _, r := range recs {
		if r.ID == "" {
			continue
		}
		sr := similarRec{id: r.ID, author: r.Author, items: make([]map[string]bool, len(s.Fields))}
		for i, f := range s.Fields {
			sr.items[i] = dsl.SimilarItems(target.FieldByID(f), r.Data[f])
		}
		out = append(out, sr)
	}
	c.similar[k] = out
	return out
}
