package dsl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Expr is a calculation tree (docs/dsl/expressions.md):
//
//	number | string | boolean
//	{"ref": "<fieldId>"}                         row field ("id" = record id)
//	{"ref": "<refField>.<fieldId>"}              field of the record a ref field points to (1 level)
//	{"agg": "...", "entity": "...", "field"?, "by"?, "filter"?: Filter | [Filter]}
//	{"op": "<name>", "args": [Expr, ...]}
//	{"if": Expr, "then": Expr, "else": Expr}
//
// iOS evaluates it; the backend only validates (names, arity, references,
// row context, cycles, depth, coarse types).
type Expr struct {
	Kind ExprKind
	Lit  any // float64 | string | bool (Kind == ExprLiteral)
	Ref  string
	Agg  *AggExpr
	Op   string
	Args []Expr
	If   *Expr
	Then *Expr
	Else *Expr
}

// ExprKind discriminates Expr.
type ExprKind int

const (
	ExprLiteral ExprKind = iota
	ExprRef
	ExprAgg
	ExprOp
	ExprIf
)

// AggExpr aggregates an entity's records. Filter is ANDed.
type AggExpr struct {
	Agg    string   `json:"agg"` // count|sum|avg|min|max|latest|earliest
	Entity string   `json:"entity"`
	Field  string   `json:"field,omitempty"`
	By     string   `json:"by,omitempty"` // latest/earliest: date field to order by
	Filter []Filter `json:"-"`
	// filterList remembers whether filter was written as an array so the
	// document round-trips byte-for-byte in shape.
	filterList bool
}

// MaxExprDepth bounds nesting (a literal alone is depth 1).
const MaxExprDepth = 8

// NumExpr is a number literal (progressBar / gauge `value`).
func NumExpr(v float64) *Expr { return &Expr{Kind: ExprLiteral, Lit: v} }

// Number returns the literal number, if the expression is one.
func (e *Expr) Number() (float64, bool) {
	if e == nil || e.Kind != ExprLiteral {
		return 0, false
	}
	f, ok := e.Lit.(float64)
	return f, ok
}

// UnmarshalJSON accepts the five shapes above and rejects anything else
// (null, arrays, objects with unknown or mixed keys).
func (e *Expr) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return errors.New("expr: empty")
	}
	switch b[0] {
	case '{':
	case '[':
		return errors.New("expr: an array is not an expression (use {\"op\":...,\"args\":[...]})")
	case 'n':
		return errors.New("expr: null is not an expression")
	default:
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*e = Expr{Kind: ExprLiteral, Lit: v}
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	allow := func(keys ...string) error {
		for k := range m {
			ok := false
			for _, a := range keys {
				if k == a {
					ok = true
				}
			}
			if !ok {
				return fmt.Errorf("expr: unknown key %q (allowed: %s)", k, strings.Join(keys, ", "))
			}
		}
		return nil
	}
	switch {
	case m["ref"] != nil:
		if err := allow("ref"); err != nil {
			return err
		}
		*e = Expr{Kind: ExprRef}
		return json.Unmarshal(m["ref"], &e.Ref)
	case m["agg"] != nil:
		if err := allow("agg", "entity", "field", "by", "filter"); err != nil {
			return err
		}
		a := &AggExpr{}
		if err := json.Unmarshal(m["agg"], &a.Agg); err != nil {
			return err
		}
		if raw, ok := m["entity"]; ok {
			if err := json.Unmarshal(raw, &a.Entity); err != nil {
				return err
			}
		}
		if raw, ok := m["field"]; ok {
			if err := json.Unmarshal(raw, &a.Field); err != nil {
				return err
			}
		}
		if raw, ok := m["by"]; ok {
			if err := json.Unmarshal(raw, &a.By); err != nil {
				return err
			}
		}
		if raw, ok := m["filter"]; ok {
			raw = bytes.TrimSpace(raw)
			if len(raw) > 0 && raw[0] == '[' {
				a.filterList = true
				if err := strictUnmarshal(raw, &a.Filter); err != nil {
					return err
				}
			} else {
				var f Filter
				if err := strictUnmarshal(raw, &f); err != nil {
					return err
				}
				a.Filter = []Filter{f}
			}
		}
		*e = Expr{Kind: ExprAgg, Agg: a}
		return nil
	case m["op"] != nil:
		if err := allow("op", "args"); err != nil {
			return err
		}
		*e = Expr{Kind: ExprOp}
		if err := json.Unmarshal(m["op"], &e.Op); err != nil {
			return err
		}
		if raw, ok := m["args"]; ok {
			if err := json.Unmarshal(raw, &e.Args); err != nil {
				return err
			}
		}
		if e.Args == nil {
			e.Args = []Expr{}
		}
		return nil
	case m["if"] != nil:
		if err := allow("if", "then", "else"); err != nil {
			return err
		}
		*e = Expr{Kind: ExprIf, If: &Expr{}, Then: &Expr{}, Else: &Expr{}}
		if err := json.Unmarshal(m["if"], e.If); err != nil {
			return err
		}
		if m["then"] == nil || m["else"] == nil {
			return errors.New("expr: if needs then and else")
		}
		if err := json.Unmarshal(m["then"], e.Then); err != nil {
			return err
		}
		return json.Unmarshal(m["else"], e.Else)
	default:
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return fmt.Errorf("expr: object with keys %v is not an expression (one of ref | agg | op | if)", keys)
	}
}

func strictUnmarshal(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// MarshalJSON writes the canonical shape.
func (e Expr) MarshalJSON() ([]byte, error) {
	switch e.Kind {
	case ExprLiteral:
		return json.Marshal(e.Lit)
	case ExprRef:
		return json.Marshal(map[string]string{"ref": e.Ref})
	case ExprAgg:
		if e.Agg == nil {
			return nil, errors.New("expr: agg is nil")
		}
		return json.Marshal(e.Agg)
	case ExprOp:
		args := e.Args
		if args == nil {
			args = []Expr{}
		}
		return json.Marshal(struct {
			Op   string `json:"op"`
			Args []Expr `json:"args"`
		}{e.Op, args})
	case ExprIf:
		return json.Marshal(struct {
			If   *Expr `json:"if"`
			Then *Expr `json:"then"`
			Else *Expr `json:"else"`
		}{e.If, e.Then, e.Else})
	}
	return nil, fmt.Errorf("expr: unknown kind %d", e.Kind)
}

// MarshalJSON keeps a single filter as an object unless it was a list.
func (a AggExpr) MarshalJSON() ([]byte, error) {
	type plain AggExpr
	out := struct {
		plain
		Filter any `json:"filter,omitempty"`
	}{plain: plain(a)}
	switch {
	case len(a.Filter) == 1 && !a.filterList:
		out.Filter = a.Filter[0]
	case len(a.Filter) > 0 || a.filterList:
		out.Filter = a.Filter
	}
	return json.Marshal(out)
}

// exprFromValue converts a decoded Filter.value (any) into an Expr.
// nil (JSON null) returns nil.
func exprFromValue(v any) (*Expr, error) {
	if v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var e Expr
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// ---- validation -----------------------------------------------------------

// exprType is the coarse static type used to catch obvious mismatches.
type exprType int

const (
	tAny exprType = iota
	tNum
	tStr
	tBool
	tDate
	tList // type refs field: a list of record ids (only len reads it, #231)
)

func (t exprType) String() string {
	return [...]string{"any", "number", "string", "boolean", "date", "list of ids (type refs)"}[t]
}

func fieldExprType(ft FieldType) exprType {
	switch ft {
	case FieldInteger, FieldNumber, FieldCurrency:
		return tNum
	case FieldBoolean:
		return tBool
	case FieldDate, FieldDatetime:
		return tDate
	case FieldRefs:
		return tList
	default:
		return tStr
	}
}

// ComputedFieldTypes are the Field.type values a computed field may declare.
var ComputedFieldTypes = []FieldType{FieldNumber, FieldInteger, FieldCurrency, FieldString, FieldBoolean, FieldDate}

// AggregateFuncs is the closed set of agg / aggregate names.
var AggregateFuncs = []string{"count", "sum", "avg", "min", "max", "latest", "earliest"}

// IsPickAgg reports whether agg returns one record's value (latest/earliest)
// instead of folding numbers.
func IsPickAgg(agg string) bool { return agg == "latest" || agg == "earliest" }

// FilterOps is the closed set of Filter.op values.
// contains: a refs field holds the value (an empty refs = everyone, #231).
var FilterOps = []string{"eq", "neq", "gt", "gte", "lt", "lte", "thisMonth", "today", "thisWeek", "thisYear", "contains"}

// opSpec is arity + argument/result types of one op. maxArgs -1 = variadic.
type opSpec struct {
	minArgs, maxArgs int
	arg              exprType // expected type of every argument (tAny = no check)
	result           exprType // tAny = depends on the args (coalesce)
}

// Ops is the closed operator table (keep in sync with $defs/ExprOp.op enum
// and docs/dsl/expressions.md). "date" args also accept strings.
var Ops = map[string]opSpec{
	"add": {2, -1, tNum, tNum}, "sub": {2, 2, tNum, tNum}, "mul": {2, -1, tNum, tNum}, "div": {2, 2, tNum, tNum},
	"mod": {2, 2, tNum, tNum}, "neg": {1, 1, tNum, tNum}, "abs": {1, 1, tNum, tNum}, "sqrt": {1, 1, tNum, tNum},
	"pow": {2, 2, tNum, tNum}, "round": {1, 2, tNum, tNum}, "floor": {1, 2, tNum, tNum}, "ceil": {1, 2, tNum, tNum}, // 2nd arg = decimal places
	"min": {2, -1, tNum, tNum}, "max": {2, -1, tNum, tNum}, "clamp": {3, 3, tNum, tNum},
	"eq": {2, 2, tAny, tBool}, "neq": {2, 2, tAny, tBool},
	"gt": {2, 2, tAny, tBool}, "gte": {2, 2, tAny, tBool}, "lt": {2, 2, tAny, tBool}, "lte": {2, 2, tAny, tBool},
	"and": {2, 2, tBool, tBool}, "or": {2, 2, tBool, tBool}, "not": {1, 1, tBool, tBool},
	"concat": {2, 2, tAny, tStr}, "len": {1, 1, tStr, tNum},
	"now": {0, 0, tAny, tDate}, "today": {0, 0, tAny, tDate},
	"daysBetween": {2, 2, tDate, tNum}, "daysUntil": {1, 1, tDate, tNum}, "addDays": {2, 2, tAny, tDate}, "hoursBetween": {2, 2, tAny, tNum},
	"year": {1, 1, tDate, tNum}, "month": {1, 1, tDate, tNum}, "day": {1, 1, tDate, tNum}, "weekday": {1, 1, tDate, tNum},
	"toNumber": {1, 1, tAny, tNum}, "toString": {1, 1, tAny, tStr}, "coalesce": {2, 2, tAny, tAny},
	// daily check history (#172): arg 0 is {"ref": <resets daily toggle field>}
	"streak": {1, 1, tDate, tNum}, "doneInDays": {2, 2, tAny, tNum},
}

// fieldKey identifies a field across entities ("members.balance").
type fieldKey struct{ entity, field string }

func (k fieldKey) String() string { return k.entity + "." + k.field }

// exprChecker validates one expression tree. row is the record context
// (nil = screen scalar such as stat.value, where `ref` is invalid).
type exprChecker struct {
	entities map[string]*Entity
	add      func(string, ...any)
	deps     map[fieldKey]bool // computed fields this expression reads (cycle check)
	// daily: entity id -> field ids used as list.item.toggle with resets
	// daily (streak / doneInDays read their history). nil = not checked.
	daily   map[string]map[string]bool
	tooDeep bool
}

func (c *exprChecker) dep(e *Entity, fid string) {
	if c.deps == nil {
		return
	}
	if f := e.FieldByID(fid); f != nil && f.Computed != nil {
		c.deps[fieldKey{e.ID, fid}] = true
	}
}

// accepts reports whether a value of type got can be used where want is expected.
func accepts(want, got exprType) bool {
	if want == tAny || got == tAny || want == got {
		return true
	}
	return want == tDate && got == tStr // "YYYY-MM-DD" literals
}

func (c *exprChecker) check(where string, e *Expr, row *Entity, depth int) exprType {
	if e == nil {
		c.add("%s: expression is missing", where)
		return tAny
	}
	if depth > MaxExprDepth {
		if !c.tooDeep {
			c.add("%s: expression is nested deeper than %d levels; simplify it (use a computed field for the inner part)", where, MaxExprDepth)
			c.tooDeep = true
		}
		return tAny
	}
	switch e.Kind {
	case ExprLiteral:
		switch e.Lit.(type) {
		case float64:
			return tNum
		case string:
			return tStr
		case bool:
			return tBool
		}
		return tAny
	case ExprRef:
		if row == nil {
			c.add("%s: {\"ref\":%q} is not allowed here (no current record; stat.value is a screen value). Use {\"agg\":...} over an entity instead", where, e.Ref)
			return tAny
		}
		if e.Ref == "id" {
			return tStr
		}
		if strings.Contains(e.Ref, ".") {
			return c.checkLookup(where, e.Ref, row)
		}
		f := row.FieldByID(e.Ref)
		if f == nil {
			c.add("%s: ref %q is not a field of entity %q (fields: %s, or \"id\")", where, e.Ref, row.ID, fieldIDs(row))
			return tAny
		}
		c.dep(row, f.ID)
		return fieldExprType(f.Type)
	case ExprAgg:
		return c.checkAgg(where, e.Agg, row, depth)
	case ExprOp:
		spec, ok := Ops[e.Op]
		if !ok {
			c.add("%s: unknown op %q (allowed: %s)", where, e.Op, strings.Join(sortedOps(), " "))
			return tAny
		}
		n := len(e.Args)
		switch {
		case spec.maxArgs < 0 && n < spec.minArgs:
			c.add("%s: op %s needs at least %d args (got %d)", where, e.Op, spec.minArgs, n)
		case spec.maxArgs >= 0 && (n < spec.minArgs || n > spec.maxArgs):
			if spec.minArgs == spec.maxArgs {
				c.add("%s: op %s takes exactly %d args (got %d)", where, e.Op, spec.minArgs, n)
			} else {
				c.add("%s: op %s takes %d-%d args (got %d)", where, e.Op, spec.minArgs, spec.maxArgs, n)
			}
		}
		types := make([]exprType, n)
		for i := range e.Args {
			aw := fmt.Sprintf("%s.%s.args[%d]", where, e.Op, i)
			types[i] = c.check(aw, &e.Args[i], row, depth+1)
			want := spec.arg
			if e.Op == "addDays" {
				want = tDate
				if i == 1 {
					want = tNum
				}
			}
			if types[i] == tList {
				if e.Op != "len" {
					c.add("%s: a type refs field is a list of ids; only op len (the number of chosen records) reads it. Filter another entity with op contains instead", aw)
				}
				continue
			}
			if !accepts(want, types[i]) {
				c.add("%s: %s expects a %s but got a %s%s", aw, e.Op, want, types[i], argHint(e.Op, want, types[i]))
			}
			if e.Op == "hoursBetween" {
				c.checkHoursBetweenArg(aw, types[i])
			}
		}
		switch e.Op {
		case "streak", "doneInDays":
			c.checkDailyHistory(where, e, row)
		case "gt", "gte", "lt", "lte":
			if n == 2 && types[0] != tAny && types[1] != tAny && !accepts(types[0], types[1]) && !accepts(types[1], types[0]) {
				c.add("%s: %s compares a %s with a %s; compare values of the same type", where, e.Op, types[0], types[1])
			}
		case "coalesce":
			for _, t := range types {
				if t != tAny {
					return t
				}
			}
		}
		return spec.result
	case ExprIf:
		if t := c.check(where+".if", e.If, row, depth+1); !accepts(tBool, t) {
			c.add("%s.if: condition must be a boolean (got a %s); use eq/gt/... to compare", where, t)
		}
		t1 := c.check(where+".then", e.Then, row, depth+1)
		t2 := c.check(where+".else", e.Else, row, depth+1)
		if t1 == t2 {
			return t1
		}
		return tAny
	}
	c.add("%s: invalid expression", where)
	return tAny
}

func (c *exprChecker) checkAgg(where string, a *AggExpr, row *Entity, depth int) exprType {
	if a == nil {
		c.add("%s: agg is empty", where)
		return tAny
	}
	where += ".agg"
	known := false
	for _, f := range AggregateFuncs {
		if a.Agg == f {
			known = true
		}
	}
	if !known {
		c.add("%s: agg %q must be one of %s", where, a.Agg, strings.Join(AggregateFuncs, "|"))
	}
	e := c.entities[a.Entity]
	if e == nil {
		c.add("%s: entity %q does not exist (entities: %s)", where, a.Entity, entityIDs(c.entities))
		return tNum
	}
	result := tNum
	if IsPickAgg(a.Agg) {
		checkPickBy(where, a.Agg, a.By, e, c.add)
		if fld := e.FieldByID(a.By); fld != nil {
			c.dep(e, fld.ID)
		}
	} else if a.By != "" {
		c.add("%s: by is only for agg latest/earliest (remove it)", where)
	}
	if a.Agg != "count" {
		f := e.FieldByID(a.Field)
		if f == nil {
			c.add("%s: field %q not in entity %q (required unless agg=count; fields: %s)", where, a.Field, e.ID, fieldIDs(e))
		} else if IsPickAgg(a.Agg) {
			c.dep(e, f.ID)
			result = fieldExprType(f.Type)
		} else {
			c.dep(e, f.ID)
			ft := fieldExprType(f.Type)
			switch {
			case (a.Agg == "sum" || a.Agg == "avg") && ft != tNum:
				c.add("%s: %s needs a numeric field but %q is %s", where, a.Agg, f.ID, f.Type)
			case (a.Agg == "min" || a.Agg == "max") && ft != tNum && ft != tDate:
				c.add("%s: %s needs a numeric or date field but %q is %s", where, a.Agg, f.ID, f.Type)
			case ft == tDate:
				result = tDate
			}
		}
	}
	for i := range a.Filter {
		fw := fmt.Sprintf("%s.filter[%d]", where, i)
		c.checkFilter(fw, &a.Filter[i], e, row, depth)
	}
	return result
}

// checkFilter validates field/op/value of a filter over entity e. The value
// may be an Expr evaluated in the outer row context (row).
func (c *exprChecker) checkFilter(where string, f *Filter, e *Entity, row *Entity, depth int) {
	if strings.HasPrefix(f.Field, RecordMarkPrefix) {
		c.add(`%s: %q (marks, #255) works only as the filter of a list/table/timeline/stat/chart component, not inside agg; marks are not records, so show the count with %q (table column / sort) instead of an agg`, where, f.Field, f.Field)
		return
	}
	if f.Field == RecordAuthor || isMeValue(f.Value) {
		c.add(`%s: %q / {"me":true} (only my posts) works only as the filter of a list/table/timeline/stat/chart component, not inside agg; put it on the component (e.g. stat aggregate count with filter {"field":"$author","op":"eq","value":{"me":true}})`, where, RecordAuthor)
		return
	}
	if fld := e.FieldByID(f.Field); fld == nil {
		c.add("%s: field %q not in entity %q (fields: %s)", where, f.Field, e.ID, fieldIDs(e))
	} else {
		c.dep(e, fld.ID)
		switch {
		case f.Op == "contains" && fld.Type != FieldRefs:
			c.add("%s: filter op contains is only for a type refs field (%q is %s); use eq", where, fld.ID, fld.Type)
		case f.Op != "contains" && fld.Type == FieldRefs:
			c.add("%s: %q is type refs (a list of ids); use op contains", where, fld.ID)
		}
	}
	known := false
	for _, op := range FilterOps {
		if f.Op == op {
			known = true
		}
	}
	if !known {
		c.add("%s: filter op %q must be one of %s", where, f.Op, strings.Join(FilterOps, "|"))
	}
	if IsPeriodFilterOp(f.Op) {
		checkPeriodFilter(where, f, e, c.add)
		return
	}
	ve, err := exprFromValue(f.Value)
	if err != nil {
		c.add("%s: filter value: %v", where, err)
		return
	}
	if ve != nil && ve.Kind != ExprLiteral {
		c.check(where+".value", ve, row, depth+1)
	}
}

// argHint: subtracting times / dates is hoursBetween / daysBetween, never
// toNumber ("09:30" is not 9.5 and a date is not a number). #229: the generic
// toNumber hint led retries to {"op":"sub","args":[toNumber(end),toNumber(start)]}.
func argHint(op string, want, got exprType) string {
	if want == tNum && (got == tStr || got == tDate) && (op == "sub" || op == "add") {
		return ` (to get the hours between two "HH:mm" times or datetimes use {"op":"hoursBetween","args":[{"ref":"start"},{"ref":"end"}]}; days between dates use daysBetween; do not toNumber a time)`
	}
	return convertHint(want)
}

func convertHint(want exprType) string {
	switch want {
	case tNum:
		return " (wrap it in {\"op\":\"toNumber\",...})"
	case tStr:
		return " (wrap it in {\"op\":\"toString\",...})"
	}
	return ""
}

func sortedOps() []string {
	out := make([]string, 0, len(Ops))
	for k := range Ops {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func fieldIDs(e *Entity) string {
	ids := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		ids = append(ids, f.ID)
	}
	return strings.Join(ids, ", ")
}

func entityIDs(m map[string]*Entity) string {
	ids := make([]string, 0, len(m))
	for k := range m {
		ids = append(ids, k)
	}
	sort.Strings(ids)
	return strings.Join(ids, ", ")
}

// checkComputedFields validates every Field.computed (row context = its own
// entity), ref fields, and rejects cycles among computed fields.
func checkComputedFields(entities []Entity, byID map[string]*Entity, daily map[string]map[string]bool, add func(string, ...any)) {
	graph := map[fieldKey][]fieldKey{}
	for i := range entities {
		e := &entities[i]
		for j := range e.Fields {
			f := &e.Fields[j]
			where := fmt.Sprintf("entity %q field %q", e.ID, f.ID)
			if f.Type == FieldRef || f.Type == FieldRefs {
				switch {
				case f.RefEntity == "":
					add("%s: type %s needs refEntity (the entity id it points to)", where, f.Type)
				case byID[f.RefEntity] == nil:
					add("%s: refEntity %q does not exist (entities: %s)", where, f.RefEntity, entityIDs(byID))
				}
			} else if f.RefEntity != "" {
				add("%s: refEntity is only allowed on type ref or refs", where)
			}
			checkHealthField(where, f, add)
			if f.Computed == nil {
				continue
			}
			okType := false
			for _, t := range ComputedFieldTypes {
				if f.Type == t {
					okType = true
				}
			}
			if !okType {
				add("%s: computed field type must be one of number|integer|currency|string|boolean|date (got %s)", where, f.Type)
			}
			if f.Required {
				add("%s: computed fields are read-only; remove required", where)
			}
			if f.DefaultValue != nil {
				add("%s: computed fields are read-only; remove defaultValue", where)
			}
			c := &exprChecker{entities: byID, add: add, deps: map[fieldKey]bool{}, daily: daily}
			got := c.check(where+".computed", f.Computed, e, 1)
			if okType && !accepts(fieldExprType(f.Type), got) {
				add("%s: computed expression yields a %s but the field type is %s; change the type or convert%s", where, got, f.Type, convertHint(fieldExprType(f.Type)))
			}
			checkPayFromHours(where, f, add)
			key := fieldKey{e.ID, f.ID}
			for d := range c.deps {
				graph[key] = append(graph[key], d)
			}
			sort.Slice(graph[key], func(a, b int) bool { return graph[key][a].String() < graph[key][b].String() })
		}
	}
	reportCycles(graph, add)
}

// reportCycles runs a DFS over computed-field dependencies; each cycle is
// reported once with its path.
func reportCycles(graph map[fieldKey][]fieldKey, add func(string, ...any)) {
	const (
		white = iota
		grey
		black
	)
	color := map[fieldKey]int{}
	var stack []fieldKey
	var visit func(k fieldKey)
	visit = func(k fieldKey) {
		color[k] = grey
		stack = append(stack, k)
		for _, d := range graph[k] {
			switch color[d] {
			case white:
				visit(d)
			case grey:
				var path []string
				start := 0
				for i, s := range stack {
					if s == d {
						start = i
					}
				}
				for _, s := range stack[start:] {
					path = append(path, s.String())
				}
				path = append(path, d.String())
				add("computed field cycle: %s; computed fields must not depend on themselves (remove one of these references)", strings.Join(path, " -> "))
			}
		}
		stack = stack[:len(stack)-1]
		color[k] = black
	}
	keys := make([]fieldKey, 0, len(graph))
	for k := range graph {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool { return keys[a].String() < keys[b].String() })
	for _, k := range keys {
		if color[k] == white {
			visit(k)
		}
	}
}
