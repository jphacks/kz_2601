package dsl

import (
	"fmt"
	"strings"
)

// DisabledNoteEN ends the English description of a rule that is off.
const DisabledNoteEN = " (off)"

var weekdayEN = map[string]string{"mon": "Monday", "tue": "Tuesday", "wed": "Wednesday", "thu": "Thursday", "fri": "Friday", "sat": "Saturday", "sun": "Sunday"}

// DescribeServerRuleIn is DescribeServerRule in lang: "en" gives English,
// anything else the Japanese sentence.
func (a *App) DescribeServerRuleIn(r *ServerRule, lang string) string {
	if lang == "en" {
		return a.describeServerRuleEN(r)
	}
	return a.DescribeServerRule(r)
}

// describeServerRuleEN mirrors DescribeServerRule, e.g.
// When "出欠" is created (if the conditions match): notify whoever created the "予定".
func (a *App) describeServerRuleEN(r *ServerRule) string {
	q := func(s string) string { return `"` + s + `"` }
	ename := func(id string) string {
		if e := a.EntityByID(id); e != nil {
			return e.Name
		}
		return id
	}
	row := a.EntityByID(r.RowEntity())
	fname := func(e *Entity, id string) string {
		if e != nil {
			if f := e.FieldByID(id); f != nil {
				return f.Name
			}
		}
		return id
	}
	refName := func(ref string) string {
		if ref == SimilarRef && r.Similar != nil {
			return "the similar " + q(ename(r.Similar.Entity))
		}
		if ref == PickRef && r.Pick != nil {
			return "the picked " + q(ename(r.Pick.Entity))
		}
		if row != nil {
			if f := row.FieldByID(ref); f != nil {
				return "the " + q(ename(f.RefEntity))
			}
		}
		return "the " + q(ref)
	}
	var when string
	t := r.On
	switch {
	case t.Every == "daily":
		when = "Every day at " + t.Time
	case t.Every == "weekly":
		when = fmt.Sprintf("Every %s at %s", weekdayEN[t.Weekday], t.Time)
	case t.Every == "monthly":
		when = fmt.Sprintf("On day %d of every month at %s", t.Day, t.Time)
	case t.Record == TriggerCreated:
		when = fmt.Sprintf("When %s is created", q(ename(t.Entity)))
	case len(t.Fields) > 0:
		names := make([]string, len(t.Fields))
		for i, f := range t.Fields {
			names[i] = q(fname(row, f))
		}
		when = fmt.Sprintf("When %s of %s changes", strings.Join(names, " / "), q(ename(t.Entity)))
	default:
		when = fmt.Sprintf("When %s is updated", q(ename(t.Entity)))
	}
	if t.Each != "" {
		when += fmt.Sprintf(", for each %s record", q(ename(t.Each)))
	}
	if r.If != nil {
		when += " (if the conditions match)"
	}
	when += a.describeLookupEN(r, row) + a.describeSimilarEN(r, row) + a.describePickEN(r)
	acts := make([]string, 0, len(r.Do))
	for _, act := range r.Do {
		switch {
		case act.Notify != nil:
			var who string
			switch act.Notify.To.Kind {
			case RecipientOwner:
				who = "the app's creator"
			case RecipientAuthor:
				who = "the writer"
			case RecipientAuthorOf:
				who = "whoever created " + refName(act.Notify.To.Field)
			case RecipientMembers:
				who = "all members"
			case RecipientRecipients:
				who = "the recipients"
			}
			acts = append(acts, "notify "+who)
		case act.CreateRecord != nil:
			acts = append(acts, "add a record to "+q(ename(act.CreateRecord.Entity)))
		case act.UpdateRecord != nil && act.UpdateRecord.Ref == "":
			acts = append(acts, "update that record")
		case act.UpdateRecord != nil:
			acts = append(acts, "update "+refName(act.UpdateRecord.Ref)+" record")
		case act.Increment != nil:
			var target *Entity
			if row != nil {
				if f := row.FieldByID(act.Increment.Ref); f != nil {
					target = a.EntityByID(f.RefEntity)
				}
			}
			acts = append(acts, fmt.Sprintf("count %s of %s", q(fname(target, act.Increment.Field)), refName(act.Increment.Ref)))
		}
	}
	note := ""
	if !r.IsEnabled() {
		note = DisabledNoteEN
	}
	return when + ": " + a.describeReadsEN(r) + strings.Join(acts, ", ") + note
}

func (a *App) describeReadsEN(r *ServerRule) string {
	ids := r.AggEntities()
	if len(ids) == 0 {
		return ""
	}
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = id
		if e := a.EntityByID(id); e != nil {
			names[i] = e.Name
		}
		names[i] = `"` + names[i] + `"`
	}
	return "total " + strings.Join(names, " / ") + ", then "
}

func (a *App) describeLookupEN(r *ServerRule, row *Entity) string {
	l := r.Lookup
	if l == nil {
		return ""
	}
	name := func(e *Entity, id, self string) string {
		if id == LookupID {
			return self
		}
		if e != nil {
			if f := e.FieldByID(id); f != nil {
				return `"` + f.Name + `"`
			}
		}
		return `"` + id + `"`
	}
	target := a.EntityByID(l.Entity)
	ename := l.Entity
	if target != nil {
		ename = target.Name
	}
	has := "a"
	if l.Op == LookupNotExists {
		has = "no"
	}
	return fmt.Sprintf(", if %s \"%s\" record has %s equal to %s", has, ename, name(target, l.Field, "an ID"), name(row, l.Equals, "this record"))
}

func (a *App) describeSimilarEN(r *ServerRule, row *Entity) string {
	s := r.Similar
	if s == nil {
		return ""
	}
	ename := s.Entity
	if target := a.EntityByID(s.Entity); target != nil {
		ename = target.Name
	}
	names := make([]string, len(s.Fields))
	for i, id := range s.Fields {
		names[i] = `"` + id + `"`
		if row != nil {
			if f := row.FieldByID(id); f != nil {
				names[i] = `"` + f.Name + `"`
			}
		}
	}
	return fmt.Sprintf(", find the most similar \"%s\" record by %s (%d or more in common)", ename, strings.Join(names, " / "), s.MinShared())
}

func (a *App) describePickEN(r *ServerRule) string {
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
	how := map[string]string{PickRandom: "at random", PickOldest: "oldest first", PickNewest: "newest first"}[p.Order]
	var conds []string
	if len(p.Filter) > 0 {
		conds = append(conds, "matching the conditions")
	}
	if p.Exclude != nil {
		conds = append(conds, "not in \""+name(p.Exclude.Entity)+"\"")
	}
	cond := ""
	if len(conds) > 0 {
		cond = " (" + strings.Join(conds, ", ") + ")"
	}
	return fmt.Sprintf(", pick 1 from \"%s\" %s%s", name(p.Entity), how, cond)
}
