package dsl

import "strings"

// checkStat: exactly one of aggregate (entity/field/filter) or value (Expr,
// screen scalar = no row context).
func checkStat(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	switch {
	case c.Value != nil && c.Aggregate != "":
		add("%s: stat takes either aggregate or value, not both (remove aggregate/entity/field/filter when using value)", where)
		return
	case c.Value == nil && c.Aggregate == "":
		add("%s: stat needs aggregate (with entity) or value (an expression)", where)
		return
	case c.Value != nil:
		if c.Entity != "" || c.Field != "" || c.Filter != nil {
			add("%s: stat with value must not set entity/field/filter (put them inside an {\"agg\":...} in value)", where)
		}
		ch := &exprChecker{entities: entities, add: add}
		ch.check(where+".value", c.Value, nil, 1)
		return
	}
	if c.Entity == "" {
		add("%s: stat needs entity (with aggregate) or value (an expression, e.g. {\"op\":\"div\",\"args\":[{\"agg\":\"sum\",...},100000]})", where)
		return
	}
	e := entities[c.Entity]
	if e == nil {
		add("%s: stat entity %q does not exist", where, c.Entity)
		return
	}
	if c.Aggregate != "count" && e.FieldByID(c.Field) == nil {
		add("%s: stat field %q not in entity %q (required unless aggregate=count)", where, c.Field, e.ID)
	}
	if IsPickAgg(c.Aggregate) {
		checkPickBy(where, c.Aggregate, c.By, e, add)
	} else if c.By != "" {
		add("%s: by is only for aggregate latest/earliest (remove it)", where)
	}
	if c.Filter != nil && e.FieldByID(c.Filter.Field) == nil {
		add("%s: filter field %q not in entity %q", where, c.Filter.Field, e.ID)
	}
}

// checkSettlement: entity + name field + numeric balance field.
func checkSettlement(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	e := entities[c.Entity]
	if e == nil {
		add("%s: settlement entity %q does not exist", where, c.Entity)
		return
	}
	if e.FieldByID(c.NameField) == nil {
		add("%s: settlement nameField %q not in entity %q (fields: %s)", where, c.NameField, e.ID, fieldIDs(e))
	}
	if f := e.FieldByID(c.BalanceField); f == nil {
		add("%s: settlement balanceField %q not in entity %q (fields: %s)", where, c.BalanceField, e.ID, fieldIDs(e))
	} else if fieldExprType(f.Type) != tNum {
		add("%s: settlement balanceField %q must be number/integer/currency (got %s); make it a computed balance = paid - share", where, c.BalanceField, f.Type)
	}
}

// checkComponentFilterValue validates an Expr written as a list/stat/chart/...
// filter value. Screen components have no row context, so `ref` is invalid.
func checkComponentFilterValue(where string, f *Filter, entities map[string]*Entity, add func(string, ...any)) {
	ve, err := exprFromValue(f.Value)
	if err != nil {
		add("%s: filter value: %v", where, err)
		return
	}
	if ve == nil || ve.Kind == ExprLiteral {
		return
	}
	ch := &exprChecker{entities: entities, add: add}
	ch.check(where+".filter.value", ve, nil, 2)
}

// checkPickBy: latest/earliest order records by `by` (a date/datetime field).
// Omitted = the entity's first date/datetime field, or creation order when it
// has none (iOS). Ties (same day) fall back to creation order.
func checkPickBy(where, agg, by string, e *Entity, add func(string, ...any)) {
	if by == "" {
		return
	}
	f := e.FieldByID(by)
	switch {
	case f == nil:
		add("%s: %s by %q not in entity %q (fields: %s)", where, agg, by, e.ID, fieldIDs(e))
	case f.Type != FieldDate && f.Type != FieldDatetime:
		add("%s: %s by %q must be a date or datetime field (got %s); it orders the records, and field is the value returned", where, agg, by, f.Type)
	}
}

// checkLookup validates {"ref":"<refField>.<fieldId>"}: refField is a type ref
// field of the row's entity, fieldId a field (or "id") of its refEntity.
// Only one level: "a.b.c" is rejected.
func (c *exprChecker) checkLookup(where, ref string, row *Entity) exprType {
	parts := strings.Split(ref, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		c.add("%s: ref %q: only one level of \"<refField>.<fieldId>\" is supported (make a computed field on the referenced entity for deeper values)", where, ref)
		return tAny
	}
	rf := row.FieldByID(parts[0])
	switch {
	case rf == nil:
		c.add("%s: ref %q: %q is not a field of entity %q (fields: %s)", where, ref, parts[0], row.ID, fieldIDs(row))
		return tAny
	case rf.Type != FieldRef:
		c.add("%s: ref %q: %q is %s, not a ref field; \"<field>.<field>\" reads the record a type ref field points to", where, ref, parts[0], rf.Type)
		return tAny
	}
	target := c.entities[rf.RefEntity]
	if target == nil {
		return tAny // refEntity problems are reported by checkComputedFields
	}
	if parts[1] == "id" {
		return tStr
	}
	f := target.FieldByID(parts[1])
	if f == nil {
		c.add("%s: ref %q: %q is not a field of entity %q (fields: %s)", where, ref, parts[1], target.ID, fieldIDs(target))
		return tAny
	}
	c.dep(target, f.ID)
	return fieldExprType(f.Type)
}
