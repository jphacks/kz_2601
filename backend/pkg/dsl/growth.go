package dsl

import "math"

// growth: a picture that grows with one number (tree bears fruit / flower
// blooms / plant grows). The number comes from exactly one of metric (device
// steps, like healthStat), entity (aggregate of records) or value (Expr, like
// stat.value). One fruit per step; target = fully grown.

// GrowthKinds is the closed set of growth.kind ("" = tree).
var GrowthKinds = map[string]bool{"": true, "tree": true, "flower": true, "plant": true}

// MaxGrowthItems caps target / step (fruits, flowers or leaves on screen).
const MaxGrowthItems = 30

func checkGrowth(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	if c.Label == "" {
		add("%s: growth needs label", where)
	}
	if !GrowthKinds[c.Kind] {
		add("%s: growth kind %q must be tree, flower or plant", where, c.Kind)
	}
	sources := 0
	for _, set := range []bool{c.Metric != "", c.Entity != "", c.Value != nil} {
		if set {
			sources++
		}
	}
	switch {
	case sources == 0:
		add(`%s: growth needs one number source: metric "steps", entity (+aggregate/field) or value (an expression)`, where)
	case sources > 1:
		add("%s: growth takes only one of metric, entity or value", where)
	}
	if c.Live && c.Metric != "steps" {
		add("%s: growth live is only for metric steps", where)
	}
	switch {
	case c.Metric != "":
		if c.Metric != "steps" {
			add("%s: growth metric must be steps", where)
		}
		if c.Aggregate != "" || c.Field != "" || c.Filter != nil {
			add("%s: growth with metric reads device steps; remove aggregate/field/filter", where)
		}
	case c.Entity != "":
		if c.Aggregate != "" && c.Aggregate != "count" && c.Aggregate != "sum" {
			add("%s: growth aggregate must be count or sum", where)
		}
		if e, ok := checkAggregateRef(where, c, entities, add); ok && c.Aggregate != "count" {
			if f := e.FieldByID(c.Field); f != nil && !numericField(f.Type) {
				add("%s: growth field %q must be an integer, number or currency field", where, c.Field)
			}
		}
	case c.Value != nil:
		if c.Aggregate != "" || c.Field != "" || c.Filter != nil {
			add("%s: growth with value must not set aggregate/field/filter (put them inside an {\"agg\":...} in value)", where)
		}
		ch := &exprChecker{entities: entities, add: add}
		ch.check(where+".value", c.Value, nil, 1)
	}
	target := checkGrowthTarget(where, c.Target, entities, add)
	if c.Step != nil {
		if *c.Step <= 0 || math.IsNaN(*c.Step) {
			add("%s: growth step must be > 0", where)
		} else if target > 0 && target / *c.Step > MaxGrowthItems {
			add("%s: growth target / step must be <= %d (one fruit per step); use a larger step", where, MaxGrowthItems)
		}
	}
}

// checkGrowthTarget returns the literal target (0 when it is a setting or invalid).
func checkGrowthTarget(where string, t *Expr, entities map[string]*Entity, add func(string, ...any)) float64 {
	if t == nil {
		add("%s: growth needs target > 0 (the value at which it is fully grown)", where)
		return 0
	}
	if v, ok := t.Number(); ok {
		if v <= 0 {
			add("%s: growth target must be > 0", where)
			return 0
		}
		return v
	}
	if t.Kind != ExprSetting {
		add(`%s: growth target must be a number or {"setting":"<fieldId>"}`, where)
		return 0
	}
	_, f, n := SettingField(entities, t.Setting)
	switch {
	case n == 0:
		add(`%s: growth target {"setting":%q} is not a field of a single entity`, where, t.Setting)
	case n > 1:
		add(`%s: growth target {"setting":%q} is ambiguous: more than one single entity has field %q`, where, t.Setting, t.Setting)
	case !numericField(f.Type):
		add(`%s: growth target {"setting":%q} must be an integer, number or currency field`, where, t.Setting)
	}
	return 0
}
