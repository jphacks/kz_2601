package dsl

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Cross-entity lookup of a server rule: the rule runs only when a record
// of another entity (exists) / no record (notExists) has the same value as
// the row, e.g. a registration whose student id is not on the imported
// roster, or a shop with no visit yet. Closed and bounded:
//
//   - one lookup per rule, equality of one field only (no ranges, no text
//     search); the row is the trigger record or the on.each record;
//   - the entity is of the same app and on the rule's side (like agg), never
//     addressed (addressed.go); it counts toward MaxRuleReadEntities;
//   - the runner lists the entity once per run through the same capped,
//     quota-counted reader as on.each / agg and matches in memory, so a
//     lookup reads no record the rule could not already read with agg.
//
// It is checked after `if` (both must hold).

// RuleLookup is a rule's cross-entity lookup.
type RuleLookup struct {
	Op     string `json:"op"`     // exists|notExists|overlaps|notOverlaps
	Entity string `json:"entity"` // the entity searched
	Field  string `json:"field"`  // its field compared ("$id" = the record id)
	Equals string `json:"equals"` // the row's field it must equal ("$id" = the row's id)
	// FieldEnd / EqualsEnd: the end of the period of an overlap op
	// (server_rules_overlap.go); "" = the period is its start only.
	FieldEnd  string `json:"fieldEnd,omitempty"`
	EqualsEnd string `json:"equalsEnd,omitempty"`
}

// Lookup ops and the record id name.
const (
	LookupExists    = "exists"
	LookupNotExists = "notExists"
	LookupID        = "$id"
)

// lookupKind groups the comparable values: text and numbers compare as the
// same normalized text, dates as dates, ids (ref / $id) as ids.
type lookupKind int

const (
	lookupNone lookupKind = iota
	lookupScalar
	lookupDate
	lookupRecordID
)

// lookupSide resolves one side of a lookup: the kind and, for an id, the
// entity the id belongs to.
func lookupSide(e *Entity, id string) (*Field, lookupKind, string) {
	if id == LookupID {
		return nil, lookupRecordID, e.ID
	}
	f := e.FieldByID(id)
	if f == nil {
		return nil, lookupNone, ""
	}
	switch f.Type {
	case FieldString, FieldText, FieldEnum, FieldInteger, FieldNumber:
		return f, lookupScalar, ""
	case FieldDate:
		return f, lookupDate, ""
	case FieldRef:
		return f, lookupRecordID, f.RefEntity
	}
	return f, lookupNone, ""
}

// checkRuleLookup validates rc.r.Lookup.
func checkRuleLookup(where string, rc *ruleCheck) {
	l := rc.r.Lookup
	if l == nil {
		return
	}
	where += ".lookup"
	add := rc.add
	if l.IsOverlap() {
		checkRuleOverlap(where, rc)
		return
	}
	if l.Op != LookupExists && l.Op != LookupNotExists {
		add("%s: op %q must be exists, notExists, overlaps or notOverlaps", where, l.Op)
	}
	if l.FieldEnd != "" || l.EqualsEnd != "" {
		add("%s: fieldEnd / equalsEnd are only for op overlaps / notOverlaps", where)
	}
	if rc.row == nil {
		add("%s: needs a record trigger or on.each (the row whose value is looked up)", where)
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
	tf, tk, tref := lookupSide(e, l.Field)
	switch {
	case l.Field != LookupID && tf == nil:
		add("%s: field %q is not a field of entity %q (fields: %s, or $id)", where, l.Field, e.ID, fieldIDs(e))
	case tf != nil && tf.Computed != nil:
		add("%s: field %q is computed on the device and is not stored; compare a stored field", where, tf.ID)
	case tk == lookupNone:
		add("%s: field %q is %s; a lookup compares a string / text / enum / integer / number / date / ref field or $id", where, tf.ID, tf.Type)
	}
	if rc.row == nil {
		return
	}
	rf, rk, rref := lookupSide(rc.row, l.Equals)
	switch {
	case l.Equals != LookupID && rf == nil:
		add("%s: equals %q is not a field of entity %q (fields: %s, or $id)", where, l.Equals, rc.row.ID, fieldIDs(rc.row))
		return
	case rf != nil && rf.Computed != nil:
		add("%s: equals %q is computed on the device and is not stored; compare a stored field", where, rf.ID)
		return
	case rk == lookupNone:
		add("%s: equals %q is %s; a lookup compares a string / text / enum / integer / number / date / ref field or $id", where, rf.ID, rf.Type)
		return
	}
	if tk == lookupNone {
		return
	}
	switch {
	case tk != rk:
		add("%s: field %q and equals %q are not comparable (text / numbers, dates and ids each compare only with their own kind)", where, l.Field, l.Equals)
	case tk == lookupRecordID && tref != rref:
		add("%s: field %q holds ids of entity %q but equals %q holds ids of entity %q", where, l.Field, tref, l.Equals, rref)
	}
}

// LookupValueKey is the value a lookup compares, "" when it never matches
// (empty / not a value). Text and numbers are compared as text with width,
// case and surrounding spaces ignored (an imported roster often differs in
// those); ids (id true) are compared exactly.
func LookupValueKey(v any, id bool) string {
	switch x := v.(type) {
	case string:
		if id {
			return x
		}
		return strings.ToLower(strings.TrimSpace(norm.NFKC.String(x)))
	case float64:
		return lookupNumberKey(x, id)
	case float32:
		return lookupNumberKey(float64(x), id)
	case int:
		return lookupNumberKey(float64(x), id)
	case int64:
		return lookupNumberKey(float64(x), id)
	}
	return ""
}

func lookupNumberKey(f float64, id bool) string {
	if id || math.IsNaN(f) || math.IsInf(f, 0) {
		return ""
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// LookupComparesIDs reports whether the lookup compares record ids (exact)
// rather than values.
func (a *App) LookupComparesIDs(l *RuleLookup) bool {
	e := a.EntityByID(l.Entity)
	if e == nil {
		return false
	}
	_, k, _ := lookupSide(e, l.Field)
	return k == lookupRecordID
}

// describeLookup is the plain sentence part of a lookup, e.g.
// 「名簿」の「学籍番号」が「学籍番号」と同じ記録が無い時.
func (a *App) describeLookup(r *ServerRule, row *Entity) string {
	l := r.Lookup
	if l == nil {
		return ""
	}
	if l.IsOverlap() {
		return a.describeOverlap(l, row)
	}
	name := func(e *Entity, id, self string) string {
		if id == LookupID {
			return self
		}
		if e != nil {
			if f := e.FieldByID(id); f != nil {
				return f.Name
			}
		}
		return id
	}
	target := a.EntityByID(l.Entity)
	ename := l.Entity
	if target != nil {
		ename = target.Name
	}
	has := "ある"
	if l.Op == LookupNotExists {
		has = "無い"
	}
	return fmt.Sprintf("、「%s」の「%s」が「%s」と同じ記録が%s時", ename, name(target, l.Field, "ID"), name(row, l.Equals, "この記録"), has)
}
