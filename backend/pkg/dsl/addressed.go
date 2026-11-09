package dsl

import (
	"fmt"
	"slices"
	"strings"
)

// Addressed records (#271, docs/dsl/README.md §3.14): 1 対 1・グループ宛ての
// メッセージ. entity.addressedTo names a member / members field; a record of
// the entity is read only by its author and the participants in that field.
// appconfig stores the readers on the record when it is created (the
// recipients do not change afterwards) and hides the record from everyone
// else on every read path (list, get, events, marks).

// FieldMembers is a list of participant uids of the app's shared data (the
// recipients of a group message). Only on scope "shared" entities.
const FieldMembers FieldType = "members"

// MaxMemberValues bounds a members value (the participants of a shared app).
const MaxMemberValues = 100

// IsAddressed reports whether the entity's records are read only by their
// author and recipients.
func (e *Entity) IsAddressed() bool { return e.AddressedTo != "" }

// Recipients reads the uids of an addressed record's recipient field (a
// member string or a members list). nil when the entity is not addressed.
func (e *Entity) Recipients(data map[string]any) []string {
	if !e.IsAddressed() {
		return nil
	}
	var out []string
	add := func(v any) {
		if s, ok := v.(string); ok && s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	switch v := data[e.AddressedTo].(type) {
	case string:
		add(v)
	case []any:
		for _, x := range v {
			add(x)
		}
	case []string:
		for _, x := range v {
			add(x)
		}
	}
	return out
}

// UsesAddressed reports whether the app has an addressed entity.
func (a *App) UsesAddressed() bool {
	return slices.ContainsFunc(a.Entities, func(e Entity) bool { return e.IsAddressed() })
}

func checkAddressed(a *App, add func(string, ...any)) {
	addressed := map[string]*Entity{}
	for i := range a.Entities {
		e := &a.Entities[i]
		for j := range e.Fields {
			f := &e.Fields[j]
			if f.Type != FieldMembers {
				continue
			}
			if e.EffectiveScope() != ScopeShared {
				add(`entity %q field %q: type members (participants of the shared data) needs the entity scope "shared"`, e.ID, f.ID)
			}
			if f.DefaultValue != nil {
				add("entity %q field %q: a members field cannot have defaultValue (pick the participants in the form)", e.ID, f.ID)
			}
		}
		if !e.IsAddressed() {
			continue
		}
		addressed[e.ID] = e
		where := fmt.Sprintf("entity %q addressedTo", e.ID)
		f := e.FieldByID(e.AddressedTo)
		switch {
		case e.EffectiveScope() != ScopeShared:
			add(`%s: addressed records need scope "shared" (the recipients are participants of the shared data)`, where)
		case f == nil:
			add("%s: field %q does not exist (fields: %s)", where, e.AddressedTo, fieldIDs(e))
		case f.Type != FieldMember && f.Type != FieldMembers:
			add("%s: field %q is type %s; the recipients are a type member (one person) or members (a group) field", where, f.ID, f.Type)
		case f.Computed != nil:
			add("%s: field %q is computed; name a stored member / members field", where, f.ID)
		case f.AdminOnly():
			add("%s: field %q has editableBy; the sender chooses the recipients", where, f.ID)
		}
		switch {
		case e.Match != "":
			add("%s: a match %q entity cannot be addressed", where, e.Match)
		case e.Single:
			add("%s: a single entity cannot be addressed", where)
		case e.Edit == EditMembers:
			add(`%s: edit "members" would let people change records they cannot read; leave edit out (the author edits)`, where)
		}
		if len(e.Unique) > 0 && !slices.Contains(e.Unique, RecordAuthor) {
			add(`%s: unique on an addressed entity must include "$author" (others' records are not visible to compare with)`, where)
		}
	}
	if len(addressed) > 0 {
		checkAddressedRules(a, addressed, add)
	}
}

// checkAddressedRules applies checkAddressedRule to every server rule.
func checkAddressedRules(a *App, addressed map[string]*Entity, add func(string, ...any)) {
	for i := range a.ServerRules {
		r := &a.ServerRules[i]
		checkAddressedRule(a, r, addressed, fmt.Sprintf("serverRules[%d] (%s)", i, r.ID), add)
	}
}

// AddressedRuleProblems is checkAddressedRules for one rule: the runner
// checks again at run time (a stored DSL is decoded without Validate). rule
// may come from another version of the app; a's entities are the ones checked.
func (a *App) AddressedRuleProblems(r *ServerRule) []string {
	addressed := map[string]*Entity{}
	for i := range a.Entities {
		if e := &a.Entities[i]; e.IsAddressed() {
			addressed[e.ID] = e
		}
	}
	if len(addressed) == 0 {
		return nil
	}
	var out []string
	checkAddressedRule(a, r, addressed, r.ID, func(f string, args ...any) { out = append(out, fmt.Sprintf(f, args...)) })
	return out
}

// checkAddressedRule keeps a server rule within who reads an addressed
// record: no rule lists one (each, agg) or reads one through a ref field;
// a rule on an addressed entity notifies only its author and recipients and
// writes only its own record; no rule creates an addressed record or changes
// its recipients.
func checkAddressedRule(a *App, r *ServerRule, addressed map[string]*Entity, where string, add func(string, ...any)) {
	if e := addressed[r.On.Each]; e != nil {
		add("%s.on.each: entity %q is addressed (only its author and recipients read it); a rule cannot run on each of its records", where, e.ID)
	}
	if r.Lookup != nil && addressed[r.Lookup.Entity] != nil {
		add("%s.lookup: entity %q is addressed (only its author and recipients read it); a rule cannot look up its records", where, r.Lookup.Entity)
	}
	if r.Similar != nil && addressed[r.Similar.Entity] != nil {
		add("%s.similar: entity %q is addressed (only its author and recipients read it); a rule cannot compare its records", where, r.Similar.Entity)
	}
	if p := r.Pick; p != nil {
		if addressed[p.Entity] != nil {
			add("%s.pick: entity %q is addressed (only its author and recipients read it); a rule cannot pick its records", where, p.Entity)
		}
		if p.Exclude != nil && addressed[p.Exclude.Entity] != nil {
			add("%s.pick.exclude: entity %q is addressed (only its author and recipients read it); a rule cannot read its records", where, p.Exclude.Entity)
		}
		if p.Count != nil && addressed[p.Count.Entity] != nil {
			add("%s.pick.count: entity %q is addressed (only its author and recipients read it); a rule cannot count its records", where, p.Count.Entity)
		}
	}
	rowEntity := a.EntityByID(r.RowEntity())
	seen := map[string]bool{} // one problem per expression shape
	r.walkExprs(func(x *Expr) {
		id, how := addressedRead(x, rowEntity, addressed)
		if id != "" && !seen[how+id] {
			seen[how+id] = true
			add("%s: %s: entity %q is addressed (only its author and recipients read it); a rule cannot read it", where, how, id)
		}
	})
	row := addressed[r.On.Entity]
	if r.On.Record == "" {
		row = nil
	}
	for j := range r.Do {
		act := &r.Do[j]
		w := fmt.Sprintf("%s.do[%d]", where, j)
		switch {
		case act.Notify != nil && row != nil && act.Notify.To.Kind != RecipientAuthor && act.Notify.To.Kind != RecipientRecipients:
			add(`%s.notify.to: entity %q is addressed (only its author and recipients read it); notify kind "author" or "recipients" only`, w, row.ID)
		case act.SaveFeed != nil && addressed[act.SaveFeed.Entity] != nil:
			add("%s.saveFeed: entity %q is addressed; a rule cannot send it (the sender picks the recipients)", w, act.SaveFeed.Entity)
		case act.CreateRecord != nil && addressed[act.CreateRecord.Entity] != nil:
			add("%s.createRecord: entity %q is addressed; a rule cannot send it (the sender picks the recipients)", w, act.CreateRecord.Entity)
		case act.CreateRecord != nil && row != nil:
			add("%s.createRecord: entity %q is addressed; a rule on it cannot copy it into other records", w, row.ID)
		case act.Increment != nil && row != nil:
			add("%s.increment: entity %q is addressed; a rule on it cannot count into other records", w, row.ID)
		case act.UpdateRecord != nil && row != nil && act.UpdateRecord.Ref != "":
			add("%s.updateRecord: entity %q is addressed; a rule on it may update only the record itself (no ref)", w, row.ID)
		case act.UpdateRecord != nil:
			target := row
			if act.UpdateRecord.Ref != "" {
				target = nil
				if rowEntity != nil {
					if f := rowEntity.FieldByID(act.UpdateRecord.Ref); f != nil {
						target = addressed[f.RefEntity]
					}
				}
			}
			if target != nil {
				if _, ok := act.UpdateRecord.Values[target.AddressedTo]; ok {
					add("%s.updateRecord: %q is the recipients of entity %q; a rule cannot change them", w, target.AddressedTo, target.ID)
				}
			}
		}
	}
}

// addressedRead returns the addressed entity x reads ("" = none) and how:
// an agg over it, or {"ref":"<ref field>.<field>"} from the row. The row's
// own fields are not reads of another record.
func addressedRead(x *Expr, row *Entity, addressed map[string]*Entity) (id, how string) {
	switch {
	case x.Kind == ExprAgg && x.Agg != nil && addressed[x.Agg.Entity] != nil:
		return x.Agg.Entity, "agg"
	case x.Kind == ExprRef && row != nil && !strings.HasPrefix(x.Ref, PrevPrefix):
		field, _, ok := strings.Cut(x.Ref, ".")
		if !ok {
			return "", ""
		}
		if f := row.FieldByID(field); f != nil && addressed[f.RefEntity] != nil {
			return f.RefEntity, fmt.Sprintf("ref %q", x.Ref)
		}
	}
	return "", ""
}
