package dsl

import (
	"fmt"
	"slices"
	"strings"
)

// Pick of a server rule: the rule runs only when an entity has a record to
// recommend, and its actions use that one record ("$pick", a ref, and
// "$pick.<field>"). E.g. today's random menu, the oldest dish not eaten yet,
// the newest unread post. Closed and bounded like similar:
//
//   - one pick per rule (not with similar); order random / oldest / newest
//     (created time); random is fixed per run (seeded by the run id, so a
//     retry picks the same record and sends nothing twice);
//   - filter: the agg filter vocabulary (at most MaxPickFilters, no hidden
//     or computed field); exclude: skip the records some record of another
//     entity points to with a ref / refs field (eaten, already shown);
//   - the entities are of the same app and on the rule's side, never
//     addressed; they count toward MaxRuleReadEntities and are listed once
//     per run through the same capped, quota-counted reader as agg;
//   - the row itself is never picked; hidden fields are not read
//     ("$pick.<hidden>", {$pick.<hidden>}).
//
// It is checked after `if` and lookup. Unlike similar it needs no row: a
// time trigger without each can pick (and notify owner / members, or
// createRecord). "$pick…" is read only by the actions.

// RulePick is a rule's choice of one record.
type RulePick struct {
	Entity  string       `json:"entity"`
	Order   string       `json:"order"` // random|oldest|newest
	Filter  []Filter     `json:"filter,omitempty"`
	Exclude *PickExclude `json:"exclude,omitempty"`
	// Count: order fewest / most compare how many records of another
	// entity point to each candidate (server_rules_assign.go).
	Count *PickCount `json:"count,omitempty"`
}

// PickExclude skips the records of the picked entity that a record of
// Entity points to with Field (a ref / refs to the picked entity).
type PickExclude struct {
	Entity string `json:"entity"`
	Field  string `json:"field"`
	// Match: only the records whose field equals the row's (server_rules_assign.go).
	Match *PickMatch `json:"match,omitempty"`
}

// Pick orders, limits and result names.
const (
	PickRandom     = "random"
	PickOldest     = "oldest"
	PickNewest     = "newest"
	MaxPickFilters = 3
	PickRef        = "$pick"
	// pickRowID is the entity id of the actions' row of a time trigger
	// without each (only the pick results).
	pickRowID = "$pick"
)

// PickOrders are the orders of rule.pick.
var PickOrders = []string{PickRandom, PickOldest, PickNewest, PickFewest, PickMost}

// PickRow is row with the pick results the actions read: "$pick" (ref) and
// "$pick.<field>" for each field of the picked entity but ref / refs (so
// {$pick.<field>} is checked like a row field). A time trigger without each (row nil) gets
// a row of the results only.
func PickRow(row, target *Entity, p *RulePick) *Entity {
	if p == nil || target == nil {
		return row
	}
	var out Entity
	if row != nil {
		out = *row
		out.Fields = slices.Clone(row.Fields)
	} else {
		out = Entity{ID: pickRowID, Name: "選んだ記録"}
	}
	out.Fields = append(out.Fields, Field{ID: PickRef, Name: target.Name, Type: FieldRef, RefEntity: target.ID})
	for _, f := range target.Fields {
		if f.Type == FieldRef || f.Type == FieldRefs {
			continue // not inserted, and never a recipient (authorOf is "$pick" only)
		}
		f.ID = PickRef + "." + f.ID
		out.Fields = append(out.Fields, f)
	}
	return &out
}

// IsPickRow reports whether e is the results-only row of PickRow.
func IsPickRow(e *Entity) bool { return e != nil && e.ID == pickRowID }

// isPickResult reports whether id is "$pick" or "$pick.<field>".
func isPickResult(id string) bool {
	return id == PickRef || strings.HasPrefix(id, PickRef+".")
}

// checkRulePick validates rc.r.Pick and sets rc.doRow.
func checkRulePick(where string, rc *ruleCheck) {
	p := rc.r.Pick
	if p == nil {
		return
	}
	where += ".pick"
	add := rc.add
	if rc.r.Similar != nil {
		add("%s: a rule has similar or pick, not both", where)
		return
	}
	if !slices.Contains(PickOrders, p.Order) {
		add("%s: order %q must be one of %s", where, p.Order, strings.Join(PickOrders, "|"))
	}
	e := rc.entities[p.Entity]
	if e == nil {
		add("%s: entity %q does not exist (entities: %s)", where, p.Entity, entityIDs(rc.entities))
		return
	}
	if e.IsShared() != (rc.scope != ScopePrivate) {
		add("%s: entity %q is %s but the rule works on %s data; pick reads only records on the same side (private with private, shared / public with shared / public)", where, e.ID, e.EffectiveScope(), rc.scope)
	}
	if e.Single {
		add("%s: entity %q is single (always one record); read it with a ref instead of pick", where, e.ID)
	}
	rc.reads[e.ID] = true
	if len(p.Filter) > MaxPickFilters {
		add("%s: filter has %d conditions; at most %d", where, len(p.Filter), MaxPickFilters)
	}
	for i := range p.Filter {
		fw := fmt.Sprintf("%s.filter[%d]", where, i)
		if f := e.FieldByID(p.Filter[i].Field); f != nil && f.Hidden {
			add("%s: field %q is hidden from answerers; a pick cannot choose by it", fw, f.ID)
			continue
		}
		ch := rc.expr()
		ch.checkFilter(fw, &p.Filter[i], e, rc.row, 1)
		checkNoComputed(fw, ch, add)
	}
	if x := p.Exclude; x != nil {
		checkPickExclude(where+".exclude", rc, e, x)
	}
	checkPickCount(where, rc, e, p)
	rc.doRow = PickRow(rc.row, e, p)
	reported := map[string]bool{}
	rc.r.walkExprs(func(x *Expr) {
		fid, ok := strings.CutPrefix(x.Ref, PickRef+".")
		if x.Kind != ExprRef || !ok || reported[fid] {
			return
		}
		if f := e.FieldByID(fid); f != nil && f.Hidden {
			reported[fid] = true
			add("%s: {\"ref\":%q} reads a hidden field of another record; hidden values are not copied by a rule", where, x.Ref)
		}
	})
}

func checkPickExclude(where string, rc *ruleCheck, picked *Entity, x *PickExclude) {
	add := rc.add
	e := rc.entities[x.Entity]
	if e == nil {
		add("%s: entity %q does not exist (entities: %s)", where, x.Entity, entityIDs(rc.entities))
		return
	}
	if e.IsShared() != (rc.scope != ScopePrivate) {
		add("%s: entity %q is %s but the rule works on %s data; pick reads only records on the same side (private with private, shared / public with shared / public)", where, e.ID, e.EffectiveScope(), rc.scope)
	}
	rc.reads[e.ID] = true
	f := e.FieldByID(x.Field)
	switch {
	case f == nil:
		add("%s: field %q is not a field of entity %q (fields: %s)", where, x.Field, e.ID, fieldIDs(e))
	case f.Computed != nil:
		add("%s: field %q is computed on the device and is not stored; use a stored ref field", where, f.ID)
	case f.Hidden:
		add("%s: field %q is hidden from answerers and cannot be read by a pick", where, f.ID)
	case f.Type != FieldRef && f.Type != FieldRefs:
		add("%s: field %q is %s; exclude needs a ref / refs field to entity %q", where, f.ID, f.Type, picked.ID)
	case f.RefEntity != picked.ID:
		add("%s: field %q points to entity %q, not to the picked entity %q", where, f.ID, f.RefEntity, picked.ID)
	}
	if x.Match != nil {
		checkPickMatch(where+".match", rc, e, x.Match)
	}
}

// describePick is the plain sentence part of pick, e.g.
// 、「メニュー」から(条件に合う・「食べた記録」に無いものを)ランダムに 1 つ選んで.
func (a *App) describePick(r *ServerRule) string {
	p := r.Pick
	if p == nil {
		return ""
	}
	name := func(id string) string {
		if e := a.EntityByID(id); e != nil {
			return e.Name
		}
		return id
	}
	how := map[string]string{PickRandom: "ランダムに", PickOldest: "古い順に", PickNewest: "新しい順に"}[p.Order]
	if p.Count != nil {
		how = describePickCount(p, name)
	}
	var conds []string
	if len(p.Filter) > 0 {
		conds = append(conds, "条件に合う")
	}
	if p.Exclude != nil {
		conds = append(conds, "「"+name(p.Exclude.Entity)+"」に"+a.pickMatchNote(p.Exclude)+"無い")
	}
	cond := ""
	if len(conds) > 0 {
		cond = "(" + strings.Join(conds, "・") + "ものを)"
	}
	return fmt.Sprintf("、「%s」から%s%s 1 つ選んで", name(p.Entity), cond, how)
}
