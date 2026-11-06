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
		if r.On.Record != kind || r.On.Entity != entity {
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
// (condition, values, notification placeholders, ref fields it follows),
// sorted and unique. "id" and {$author} are not fields and are left out.
func (r *ServerRule) ReferencedFields() []string {
	seen := map[string]bool{}
	addField := func(id string) {
		if id != "" {
			seen[id] = true
		}
	}
	var walk func(e *Expr)
	walk = func(e *Expr) {
		if e == nil {
			return
		}
		if e.Kind == ExprRef && e.Ref != "id" {
			ref := strings.TrimPrefix(e.Ref, PrevPrefix)
			ref, _, _ = strings.Cut(ref, ".") // lookup "<ref field>.<field>" reads the ref field
			addField(ref)
		}
		for i := range e.Args {
			walk(&e.Args[i])
		}
		walk(e.If)
		walk(e.Then)
		walk(e.Else)
	}
	walkValues := func(values map[string]Expr) {
		for k := range values {
			v := values[k]
			walk(&v)
		}
	}
	walk(r.If)
	for _, act := range r.Do {
		switch {
		case act.Notify != nil:
			if act.Notify.To.Kind == RecipientAuthorOf {
				addField(act.Notify.To.Field)
			}
			for _, s := range []string{act.Notify.Title, act.Notify.Body} {
				for _, m := range notifyPlaceholderRe.FindAllStringSubmatch(s, -1) {
					if m[1] != notifyAuthorPlaceholder {
						addField(m[1])
					}
				}
			}
		case act.CreateRecord != nil:
			walkValues(act.CreateRecord.Values)
		case act.UpdateRecord != nil:
			addField(act.UpdateRecord.Ref)
			walkValues(act.UpdateRecord.Values)
		case act.Increment != nil:
			addField(act.Increment.Ref)
			walk(act.Increment.By)
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
