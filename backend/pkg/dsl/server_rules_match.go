package dsl

import (
	"slices"
	"strings"
)

// Matching a record write against serverRules (#341). appconfig queues an
// outbox event only for the rules these select; the runner evaluates `if`.

// MatchRecordRules returns the rules triggered when a record of entity is
// created / updated (kind = TriggerCreated / TriggerUpdated). changed lists
// the fields an update changed: an update that changed nothing triggers
// nothing, and a rule with on.fields needs one of them in changed.
func (a *App) MatchRecordRules(entity, kind string, changed []string) []*ServerRule {
	if (kind != TriggerCreated && kind != TriggerUpdated) || (kind == TriggerUpdated && len(changed) == 0) {
		return nil
	}
	var out []*ServerRule
	for i := range a.ServerRules {
		r := &a.ServerRules[i]
		if r.On.Record != kind || r.On.Entity != entity || !r.IsEnabled() {
			continue
		}
		if kind == TriggerUpdated && len(r.On.Fields) > 0 && !slices.ContainsFunc(r.On.Fields, func(f string) bool { return slices.Contains(changed, f) }) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// HasRecordRules reports whether any rule is triggered by kind writes of
// entity, whatever fields change.
func (a *App) HasRecordRules(entity, kind string) bool {
	return kind != "" && slices.ContainsFunc(a.ServerRules, func(r ServerRule) bool { return r.On.Record == kind && r.On.Entity == entity })
}

// ReferencedFields returns the trigger record's field ids the rule reads
// (condition, values, agg filter values, notification placeholders, ref
// fields it follows),
// sorted and unique. "id" and {$author} are not fields and are left out.
func (r *ServerRule) ReferencedFields() []string {
	seen := map[string]bool{}
	addField := func(id string) {
		if id != "" && !isSimilarResult(id) && !isPickResult(id) {
			seen[id] = true
		}
	}
	r.walkExprs(func(e *Expr) {
		if e.Kind == ExprRef && e.Ref != "id" {
			ref := strings.TrimPrefix(e.Ref, PrevPrefix)
			ref, _, _ = strings.Cut(ref, ".") // lookup "<ref field>.<field>" reads the ref field
			addField(ref)
		}
	})
	if r.Lookup != nil && r.Lookup.Equals != LookupID {
		addField(r.Lookup.Equals) // the row's value the lookup compares
	}
	if r.Lookup != nil {
		addField(r.Lookup.EqualsEnd) // the end of the row's period (overlap)
	}
	if r.Similar != nil {
		for _, f := range r.Similar.Fields {
			addField(f) // the row's values similar compares
		}
	}
	if p := r.Pick; p != nil {
		if p.Exclude != nil && p.Exclude.Match != nil && p.Exclude.Match.Equals != LookupID {
			addField(p.Exclude.Match.Equals) // the row's value exclude.match compares
		}
		if p.Count != nil && p.Count.Entity == r.RowEntity() {
			addField(p.Count.Field) // the row's current assignment is not counted
		}
	}
	for _, act := range r.Do {
		switch {
		case act.Notify != nil:
			addField(act.Notify.To.Field) // authorOf: the ref field; recipients: addressedTo
			for _, s := range []string{act.Notify.Title, act.Notify.Body} {
				for _, m := range notifyPlaceholderRe.FindAllStringSubmatch(s, -1) {
					if _, named := act.Notify.With[m[1]]; m[1] != notifyAuthorPlaceholder && !named {
						addField(m[1])
					}
				}
			}
		case act.UpdateRecord != nil:
			addField(act.UpdateRecord.Ref)
		case act.Increment != nil:
			addField(act.Increment.Ref)
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
