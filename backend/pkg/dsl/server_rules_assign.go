package dsl

import "fmt"

// Assignment by pick (割り当て・日程の自動調整): order fewest / most with
// count picks the candidate the fewest (most) records of another entity
// point to, e.g. the staff member with the fewest shifts, the day with the
// fewest tasks, the date most people can attend. The actions write "$pick"
// into the row's ref field (updateRecord) or a new record (createRecord).
// Closed and bounded like pick:
//
//   - count: one entity and its ref / refs field to the picked entity (no
//     hidden or computed field), same app, same side, never addressed;
//   - ties go to the candidate created first (then the smaller id), so
//     assigning one after another fills the candidates in turn (round
//     robin) and a retry of the run picks the same one;
//   - the row's own record is not counted, nor a record this run created
//     (a retry after the write counts what the first attempt counted); in
//     an on.each run the rows assigned earlier in the run are counted;
//   - exclude.match keeps exclude to the records whose field equals the
//     row's (the staff off on the shift's day), compared like lookup;
//   - a rule with count may read MaxAssignReadEntities entities (candidates,
//     count, exclude); the run's read cap and daily quota are unchanged.

// PickCount names the records that count toward each candidate.
type PickCount struct {
	Entity string `json:"entity"`
	Field  string `json:"field"` // ref / refs to the picked entity
}

// PickMatch: exclude only the records whose Field equals the row's Equals
// ("$id" = the record id on either side).
type PickMatch struct {
	Field  string `json:"field"`
	Equals string `json:"equals"`
}

// Assignment orders and limits.
const (
	PickFewest = "fewest"
	PickMost   = "most"
	// MaxAssignReadEntities: entities a rule with pick.count may list.
	MaxAssignReadEntities = MaxRuleReadEntities + 1
)

// IsCountOrder reports whether order compares counts (pick.count).
func IsCountOrder(order string) bool { return order == PickFewest || order == PickMost }

// readCap is how many entities the rule may list.
func (r *ServerRule) readCap() int {
	if r.Pick != nil && r.Pick.Count != nil {
		return MaxAssignReadEntities
	}
	return MaxRuleReadEntities
}

// checkPickCount validates p.Count against p.Order (picked: the candidates).
func checkPickCount(where string, rc *ruleCheck, picked *Entity, p *RulePick) {
	add := rc.add
	c := p.Count
	if c == nil {
		if IsCountOrder(p.Order) {
			add(`%s: order %s needs count {"entity":"<records that count>","field":"<their ref / refs to %s>"}`, where, p.Order, picked.ID)
		}
		return
	}
	where += ".count"
	if !IsCountOrder(p.Order) {
		add("%s: count is only for order fewest / most", where)
	}
	e := rc.entities[c.Entity]
	if e == nil {
		add("%s: entity %q does not exist (entities: %s)", where, c.Entity, entityIDs(rc.entities))
		return
	}
	if e.IsShared() != (rc.scope != ScopePrivate) {
		add("%s: entity %q is %s but the rule works on %s data; pick reads only records on the same side (private with private, shared / public with shared / public)", where, e.ID, e.EffectiveScope(), rc.scope)
	}
	rc.reads[e.ID] = true
	f := e.FieldByID(c.Field)
	switch {
	case f == nil:
		add("%s: field %q is not a field of entity %q (fields: %s)", where, c.Field, e.ID, fieldIDs(e))
	case f.Computed != nil:
		add("%s: field %q is computed on the device and is not stored; use a stored ref field", where, f.ID)
	case f.Hidden:
		add("%s: field %q is hidden from answerers and cannot be read by a pick", where, f.ID)
	case f.Type != FieldRef && f.Type != FieldRefs:
		add("%s: field %q is %s; count needs a ref / refs field to entity %q", where, f.ID, f.Type, picked.ID)
	case f.RefEntity != picked.ID:
		add("%s: field %q points to entity %q, not to the picked entity %q", where, f.ID, f.RefEntity, picked.ID)
	}
}

// checkPickMatch validates exclude.match (e: the exclude entity).
func checkPickMatch(where string, rc *ruleCheck, e *Entity, m *PickMatch) {
	add := rc.add
	if rc.row == nil {
		add("%s: needs a record trigger or on.each (the row whose value is matched)", where)
		return
	}
	tf, tk, tref := lookupSide(e, m.Field)
	switch {
	case m.Field != LookupID && tf == nil:
		add("%s: field %q is not a field of entity %q (fields: %s, or $id)", where, m.Field, e.ID, fieldIDs(e))
		return
	case tf != nil && tf.Computed != nil:
		add("%s: field %q is computed on the device and is not stored; compare a stored field", where, tf.ID)
		return
	case tf != nil && tf.Hidden:
		add("%s: field %q is hidden from answerers and cannot be read by a pick", where, tf.ID)
		return
	case tk == lookupNone:
		add("%s: field %q is %s; match compares a string / text / enum / integer / number / date / ref field or $id", where, tf.ID, tf.Type)
		return
	}
	rf, rk, rref := lookupSide(rc.row, m.Equals)
	switch {
	case m.Equals != LookupID && rf == nil:
		add("%s: equals %q is not a field of entity %q (fields: %s, or $id)", where, m.Equals, rc.row.ID, fieldIDs(rc.row))
	case rf != nil && rf.Computed != nil:
		add("%s: equals %q is computed on the device and is not stored; compare a stored field", where, rf.ID)
	case rk == lookupNone:
		add("%s: equals %q is %s; match compares a string / text / enum / integer / number / date / ref field or $id", where, rf.ID, rf.Type)
	case tk != rk:
		add("%s: field %q and equals %q are not comparable (text / numbers, dates and ids each compare only with their own kind)", where, m.Field, m.Equals)
	case tk == lookupRecordID && tref != rref:
		add("%s: field %q holds ids of entity %q but equals %q holds ids of entity %q", where, m.Field, tref, m.Equals, rref)
	}
}

// PickMatchComparesIDs reports whether exclude.match compares record ids.
func (a *App) PickMatchComparesIDs(x *PickExclude) bool {
	e := a.EntityByID(x.Entity)
	if e == nil || x.Match == nil {
		return false
	}
	_, k, _ := lookupSide(e, x.Match.Field)
	return k == lookupRecordID
}

// describePickCount is the order part of a pick with count, e.g.
// 「シフト」が一番少ないものを.
func describePickCount(p *RulePick, name func(string) string) string {
	most := "一番少ない"
	if p.Order == PickMost {
		most = "一番多い"
	}
	return fmt.Sprintf("「%s」が%sものを", name(p.Count.Entity), most)
}

// pickMatchNote is the exclude.match part, e.g. 同じ「日付」で.
func (a *App) pickMatchNote(x *PickExclude) string {
	if x.Match == nil {
		return ""
	}
	name := x.Match.Field
	if x.Match.Field == LookupID {
		name = "ID"
	} else if e := a.EntityByID(x.Entity); e != nil {
		if f := e.FieldByID(x.Match.Field); f != nil {
			name = f.Name
		}
	}
	return "同じ「" + name + "」で"
}
