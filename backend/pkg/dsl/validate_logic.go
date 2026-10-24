package dsl

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
	e := entities[c.Entity]
	if e == nil {
		add("%s: stat entity %q does not exist", where, c.Entity)
		return
	}
	if c.Aggregate != "count" && e.FieldByID(c.Field) == nil {
		add("%s: stat field %q not in entity %q (required unless aggregate=count)", where, c.Field, e.ID)
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
