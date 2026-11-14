package expreval

import "github.com/jphacks/kz_2601/backend/pkg/dsl"

// setting is ExprEvaluator.setting (#267): the field of the first single
// entity (entity order) that has it, read from that entity's one record
// (SingleRecord). Before the first save there is no record: a stored field
// reads its defaultValue ("now" on a date field is not applied), a computed
// field is evaluated on an empty record.
func (ev *Evaluator) setting(id string) Value {
	for _, e := range ev.singles {
		f := field(e, id)
		if f == nil || id == "" || id[0] == '$' {
			continue
		}
		r, ok := SingleRecord(ev.records(e.ID))
		if !ok {
			r = Record{Data: map[string]any{}}
			for i := range e.Fields {
				if d := &e.Fields[i]; d.Computed == nil {
					if v := ev.defaultValue(d); !v.IsNil() {
						r.Data[d.ID] = normalizeJSON(d.DefaultValue)
					}
				}
			}
		}
		v := ev.FieldValue(id, &Row{Entity: e, Record: r})
		if v.IsNil() && f.Computed == nil {
			return ev.defaultValue(f)
		}
		return v
	}
	return Value{}
}

func (ev *Evaluator) defaultValue(f *dsl.Field) Value {
	d := normalizeJSON(f.DefaultValue)
	if s, ok := d.(string); ok && s == "now" && isDate(f.Type) {
		return Value{}
	}
	return ev.stored(d, f)
}

// SingleRecord is SingleRecord.pick on iOS: the one record of a single entity.
// Normally there is at most one; when an older build added more, the newest
// wins (greatest createdAt string, then the later one in order).
func SingleRecord(rows []Record) (Record, bool) {
	best := -1
	for i, r := range rows {
		if best < 0 || r.CreatedAt >= rows[best].CreatedAt {
			best = i
		}
	}
	if best < 0 {
		return Record{}, false
	}
	return rows[best], true
}
