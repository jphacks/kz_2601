package dsl

import "fmt"

// Overlap of periods in a server rule's lookup: the rule runs only when a
// record of the entity has a period (field .. fieldEnd) that overlaps the
// row's period (equals .. equalsEnd) (overlaps), or when none does
// (notOverlaps), e.g. a booking that clashes with another booking, or a work
// plan that covers a day off. It keeps the lookup's bounds (one per rule,
// same side, a counted read listed once per run) and only compares dates:
//
//   - a date covers its whole day in the creator's time zone and the end day
//     is included; a datetime period is [start, end) (back-to-back periods
//     do not overlap) and a datetime without an end is the instant itself;
//   - no end (fieldEnd / equalsEnd omitted, or the value empty) = the start
//     only; an end before the start, or an empty start, is not a period (the
//     row then fires neither op, a listed record is skipped);
//   - the row itself is never its own match (same entity).

// Overlap ops.
const (
	LookupOverlaps    = "overlaps"
	LookupNotOverlaps = "notOverlaps"
)

// IsOverlap reports whether the lookup compares periods.
func (l *RuleLookup) IsOverlap() bool {
	return l != nil && (l.Op == LookupOverlaps || l.Op == LookupNotOverlaps)
}

// overlapSide resolves one end of a period: a stored date / datetime field.
func overlapSide(where, key string, e *Entity, id string, add func(string, ...any)) FieldType {
	if id == LookupID {
		add("%s: %s is $id; an overlap compares date / datetime fields", where, key)
		return ""
	}
	f := e.FieldByID(id)
	switch {
	case f == nil:
		add("%s: %s %q is not a field of entity %q (fields: %s)", where, key, id, e.ID, fieldIDs(e))
		return ""
	case f.Computed != nil:
		add("%s: %s %q is computed on the device and is not stored; compare a stored field", where, key, id)
		return ""
	case f.Type != FieldDate && f.Type != FieldDatetime:
		add("%s: %s %q is %s; an overlap compares date / datetime fields", where, key, id, f.Type)
		return ""
	}
	return f.Type
}

// checkRuleOverlap validates an overlaps / notOverlaps lookup.
func checkRuleOverlap(where string, rc *ruleCheck) {
	l, add := rc.r.Lookup, rc.add
	if rc.row == nil {
		add("%s: needs a record trigger or on.each (the row whose period is compared)", where)
	}
	e := rc.entities[l.Entity]
	if e == nil {
		add("%s: entity %q does not exist (entities: %s)", where, l.Entity, entityIDs(rc.entities))
		return
	}
	if e.IsShared() != (rc.scope != ScopePrivate) {
		add("%s: entity %q is %s but the rule works on %s data; a lookup reads only records on the same side (private with private, shared / public with shared / public)", where, e.ID, e.EffectiveScope(), rc.scope)
	}
	rc.reads[e.ID] = true
	pair := func(e *Entity, startKey, start, endKey, end string) {
		st := overlapSide(where, startKey, e, start, add)
		if end == "" {
			return
		}
		if end == start {
			add("%s: %s %q is also the start; omit it for a period of one day / instant", where, endKey, end)
			return
		}
		if et := overlapSide(where, endKey, e, end, add); st != "" && et != "" && st != et {
			add("%s: %s %q is %s but %s %q is %s; the start and end of a period have the same type", where, startKey, start, st, endKey, end, et)
		}
	}
	pair(e, "field", l.Field, "fieldEnd", l.FieldEnd)
	if rc.row != nil {
		pair(rc.row, "equals", l.Equals, "equalsEnd", l.EqualsEnd)
	}
}

// describeOverlap is the plain sentence part of an overlap lookup, e.g.
// 、「予約」の「開始」〜「終了」が「開始」〜「終了」と重なる記録がある時.
func (a *App) describeOverlap(l *RuleLookup, row *Entity) string {
	period := func(e *Entity, start, end string) string {
		name := func(id string) string {
			if e != nil {
				if f := e.FieldByID(id); f != nil {
					return f.Name
				}
			}
			return id
		}
		if end == "" {
			return "「" + name(start) + "」"
		}
		return "「" + name(start) + "」〜「" + name(end) + "」"
	}
	target := a.EntityByID(l.Entity)
	ename := l.Entity
	if target != nil {
		ename = target.Name
	}
	has := "ある"
	if l.Op == LookupNotOverlaps {
		has = "無い"
	}
	return fmt.Sprintf("、「%s」の%sが%sと重なる記録が%s時", ename, period(target, l.Field, l.FieldEnd), period(row, l.Equals, l.EqualsEnd), has)
}
