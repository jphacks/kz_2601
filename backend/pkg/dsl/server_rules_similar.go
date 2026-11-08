package dsl

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Similar records of a server rule: the rule runs only when a record of an
// entity shares at least min values with the row over the listed fields
// (common hobbies, answers, tags), and its actions can use the most similar
// one ("$similar", a ref) and how many values they share ("$similarScore").
// E.g. write the best match into a profile, or tell its author. Closed and
// bounded like lookup:
//
//   - one similar per rule, 1..MaxSimilarFields fields that exist with the
//     same id in the row's entity and the searched one; a value counts once
//     per field (a string / text is split into items at 、 , / ・ and line
//     breaks; a refs field is its ids); the score is the shared items;
//   - the entity is of the same app and on the rule's side, never addressed;
//     it counts toward MaxRuleReadEntities and is listed once per run through
//     the same capped, quota-counted reader as agg;
//   - the row itself and, on shared / public data, records by the row's
//     author are never matched; ties go to the first listed (newest) record;
//   - hidden and computed fields are not compared.
//
// It is checked after `if` and lookup. "$similar" / "$similarScore" are read
// only by the actions (values, by, notify.with, {$similarScore}, notify
// authorOf "$similar"); `if` runs before the search.

// RuleSimilar is a rule's search for the most similar record.
type RuleSimilar struct {
	Entity string   `json:"entity"`
	Fields []string `json:"fields"`
	Min    int      `json:"min,omitempty"` // shared items needed; 0 = 1
}

// Similar limits and result names.
const (
	MaxSimilarFields = 5
	MaxSimilarMin    = 20
	// MaxSimilarItems: items of one field value that are compared.
	MaxSimilarItems = 20
	SimilarRef      = "$similar"
	SimilarScore    = "$similarScore"
)

// similarSplitRe separates the items of a string / text value.
var similarSplitRe = regexp.MustCompile(`[、,，/／・\n]+`)

// similarKind groups the comparable field types.
type similarKind int

const (
	similarNone similarKind = iota
	similarText
	similarDate
	similarIDs
)

func similarFieldKind(f *Field) (similarKind, string) {
	switch f.Type {
	case FieldString, FieldText, FieldEnum, FieldInteger, FieldNumber:
		return similarText, ""
	case FieldDate:
		return similarDate, ""
	case FieldRef, FieldRefs:
		return similarIDs, f.RefEntity
	}
	return similarNone, ""
}

// MinShared is the shared items a match needs.
func (s *RuleSimilar) MinShared() int { return max(s.Min, 1) }

// SimilarRow is row with the two result fields the actions read.
func SimilarRow(row *Entity, s *RuleSimilar) *Entity {
	if row == nil || s == nil {
		return row
	}
	out := *row
	out.Fields = append(slices.Clone(row.Fields),
		Field{ID: SimilarRef, Name: "似ている記録", Type: FieldRef, RefEntity: s.Entity},
		Field{ID: SimilarScore, Name: "共通の数", Type: FieldInteger})
	return &out
}

// isSimilarResult reports whether id is a result field (or a lookup through
// "$similar").
func isSimilarResult(id string) bool {
	return id == SimilarRef || id == SimilarScore || strings.HasPrefix(id, SimilarRef+".")
}

// checkRuleSimilar validates rc.r.Similar and sets rc.doRow.
func checkRuleSimilar(where string, rc *ruleCheck) {
	s := rc.r.Similar
	if s == nil {
		return
	}
	where += ".similar"
	add := rc.add
	if rc.row == nil {
		add("%s: needs a record trigger or on.each (the row compared with the other records)", where)
		return
	}
	if s.Min < 0 || s.Min > MaxSimilarMin {
		add("%s: min %d must be 1-%d (omit for 1)", where, s.Min, MaxSimilarMin)
	}
	e := rc.entities[s.Entity]
	if e == nil {
		add("%s: entity %q does not exist (entities: %s)", where, s.Entity, entityIDs(rc.entities))
		return
	}
	if e.IsShared() != (rc.scope != ScopePrivate) {
		add("%s: entity %q is %s but the rule works on %s data; similar reads only records on the same side (private with private, shared / public with shared / public)", where, e.ID, e.EffectiveScope(), rc.scope)
	}
	rc.reads[e.ID] = true
	if n := len(s.Fields); n == 0 || n > MaxSimilarFields {
		add("%s: fields needs 1-%d fields (got %d)", where, MaxSimilarFields, n)
	}
	seen := map[string]bool{}
	for _, id := range s.Fields {
		if seen[id] {
			add("%s: field %q is listed twice", where, id)
			continue
		}
		seen[id] = true
		rf, tf := rc.row.FieldByID(id), e.FieldByID(id)
		switch {
		case rf == nil:
			add("%s: field %q is not a field of entity %q (fields: %s); compare fields with the same id in both entities", where, id, rc.row.ID, fieldIDs(rc.row))
			continue
		case tf == nil:
			add("%s: field %q is not a field of entity %q (fields: %s); compare fields with the same id in both entities", where, id, e.ID, fieldIDs(e))
			continue
		case rf.Computed != nil || tf.Computed != nil:
			add("%s: field %q is computed on the device and is not stored; compare a stored field", where, id)
			continue
		case rf.Hidden || tf.Hidden:
			add("%s: field %q is hidden from answerers and cannot be compared", where, id)
			continue
		}
		rk, rref := similarFieldKind(rf)
		tk, tref := similarFieldKind(tf)
		switch {
		case rk == similarNone || tk == similarNone:
			bad := rf.Type
			if rk != similarNone {
				bad = tf.Type
			}
			add("%s: field %q is %s; similar compares string / text / enum / integer / number / date / ref / refs fields", where, id, bad)
		case rk != tk || rref != tref:
			add("%s: field %q is %s in %q but %s in %q; they are not comparable", where, id, rf.Type, rc.row.ID, tf.Type, e.ID)
		}
	}
	rc.doRow = SimilarRow(rc.row, s)
	reported := map[string]bool{}
	rc.r.walkExprs(func(x *Expr) {
		fid, ok := strings.CutPrefix(x.Ref, SimilarRef+".")
		if x.Kind != ExprRef || !ok || reported[fid] {
			return
		}
		if f := e.FieldByID(fid); f != nil && f.Hidden {
			reported[fid] = true
			add("%s: {\"ref\":%q} reads a hidden field of another record; hidden values are not copied by a rule", where, x.Ref)
		}
	})
}

// SimilarItems is the set of compared items of one stored value of field f
// (nil when empty): ids for ref / refs, else LookupValueKey of the value or
// of each item of a string / text.
func SimilarItems(f *Field, v any) map[string]bool {
	if f == nil {
		return nil
	}
	out := map[string]bool{}
	put := func(k string) {
		if k != "" && len(out) < MaxSimilarItems {
			out[k] = true
		}
	}
	switch f.Type {
	case FieldRef:
		put(LookupValueKey(v, true))
	case FieldRefs:
		xs, _ := v.([]any)
		for _, x := range xs {
			put(LookupValueKey(x, true))
		}
	case FieldString, FieldText:
		s, _ := v.(string)
		for _, item := range similarSplitRe.Split(s, -1) {
			put(LookupValueKey(item, false))
		}
	default:
		put(LookupValueKey(v, false))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// describeSimilar is the plain sentence part of similar, e.g.
// 、「プロフィール」で「趣味」・「レベル」が一番似ている記録を探して(2 つ以上同じ時).
func (a *App) describeSimilar(r *ServerRule, row *Entity) string {
	s := r.Similar
	if s == nil {
		return ""
	}
	target := a.EntityByID(s.Entity)
	ename := s.Entity
	if target != nil {
		ename = target.Name
	}
	names := make([]string, len(s.Fields))
	for i, id := range s.Fields {
		names[i] = "「" + id + "」"
		if row != nil {
			if f := row.FieldByID(id); f != nil {
				names[i] = "「" + f.Name + "」"
			}
		}
	}
	return fmt.Sprintf("、「%s」で%sが一番似ている記録を探して(%d つ以上同じ時)", ename, strings.Join(names, "・"), s.MinShared())
}
