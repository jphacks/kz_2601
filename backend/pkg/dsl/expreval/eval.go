package expreval

import (
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Record is one stored record (appconfig Record). Data holds JSON values
// (float64 | string | bool | nil | []any | map[string]any).
type Record struct {
	ID         string          `json:"id"`
	Data       map[string]any  `json:"data"`
	CreatedAt  string          `json:"createdAt,omitempty"`  // "" = none
	Author     string          `json:"author,omitempty"`     // uid, "" = none
	AuthorName string          `json:"authorName,omitempty"` // "" = none
	Marks      map[string]Mark `json:"marks,omitempty"`
}

// Mark is the per-record state of one Entity mark (#255).
type Mark struct {
	Count int  `json:"count"`
	Mine  bool `json:"mine"`
}

// Source gives an entity's records (agg, lookup and refs read it).
type Source interface {
	Records(entity string) []Record
}

// SourceFunc adapts a function to Source.
type SourceFunc func(entity string) []Record

// Records implements Source.
func (f SourceFunc) Records(entity string) []Record { return f(entity) }

// Env is everything an evaluation may read besides the expression itself.
type Env struct {
	Entities []dsl.Entity
	Source   Source         // nil = no records
	Now      time.Time      // "now" / "today" and period filters
	Location *time.Location // the device time zone; nil = UTC
}

// Row is the row context that {"ref": ...} reads.
type Row struct {
	Entity *dsl.Entity // nil when the entity is unknown
	Record Record
	// Draft marks a form draft (not saved): its computed values are not memoized.
	Draft bool
}

// MaxDepth bounds nesting plus computed chains (iOS ExprEvaluator.maxDepth).
const MaxDepth = 48

// maxHistory is DailyCheck.maxHistory.
const maxHistory = 400

// Evaluate evaluates e once. It is a pure function of its inputs.
func Evaluate(e dsl.Expr, env Env, row *Row) Value {
	return New(env).Eval(e, row)
}

// Evaluator memoizes computed fields and aggregates for one render (like iOS:
// one instance per screen render). Not safe for concurrent use.
type Evaluator struct {
	entities   map[string]*dsl.Entity
	source     Source
	now        time.Time
	loc        *time.Location
	computed   map[string]Value
	aggs       map[string]Value
	views      map[string][]Record
	inProgress map[string]bool
	depth      int
	singles    []*dsl.Entity // single entities in entity order (#267)
}

// New builds an Evaluator over env.
func New(env Env) *Evaluator {
	ev := &Evaluator{
		entities:   map[string]*dsl.Entity{},
		source:     env.Source,
		now:        env.Now,
		loc:        env.Location,
		computed:   map[string]Value{},
		aggs:       map[string]Value{},
		views:      map[string][]Record{},
		inProgress: map[string]bool{},
	}
	if ev.loc == nil {
		ev.loc = time.UTC
	}
	for i := range env.Entities {
		if _, dup := ev.entities[env.Entities[i].ID]; !dup {
			ev.entities[env.Entities[i].ID] = &env.Entities[i]
			if env.Entities[i].Single {
				ev.singles = append(ev.singles, &env.Entities[i])
			}
		}
	}
	return ev
}

func (ev *Evaluator) records(entity string) []Record {
	if ev.source == nil {
		return nil
	}
	return ev.source.Records(entity)
}

// Eval evaluates e in row (nil = no row, e.g. stat.value).
func (ev *Evaluator) Eval(e dsl.Expr, row *Row) Value {
	if ev.depth >= MaxDepth {
		return Value{}
	}
	ev.depth++
	defer func() { ev.depth-- }()
	switch e.Kind {
	case dsl.ExprLiteral:
		return literal(normalizeJSON(e.Lit))
	case dsl.ExprRef:
		if row == nil {
			return Value{}
		}
		return ev.FieldValue(e.Ref, row)
	case dsl.ExprAgg:
		if e.Agg == nil {
			return Value{}
		}
		return ev.aggregate(e.Agg, row)
	case dsl.ExprSetting:
		return ev.setting(e.Setting)
	case dsl.ExprOp:
		return ev.apply(e.Op, e.Args, row)
	case dsl.ExprIf:
		if e.If == nil || e.Then == nil || e.Else == nil {
			return Value{}
		}
		if ev.Eval(*e.If, row).truthy() {
			return ev.Eval(*e.Then, row)
		}
		return ev.Eval(*e.Else, row)
	}
	return Value{}
}

// literal is ExprEvaluator.value(from:field: nil).
func literal(v any) Value {
	switch x := v.(type) {
	case float64:
		return NumberValue(x)
	case bool:
		return BoolValue(x)
	case string:
		return StringValue(x)
	}
	return Value{}
}

// stored is ExprEvaluator.value(from:field:): date/datetime field strings that
// parse become dates; null, arrays and objects are nil.
func (ev *Evaluator) stored(v any, f *dsl.Field) Value {
	if s, ok := v.(string); ok && f != nil && isDate(f.Type) {
		if t, ok := parseDate(s, ev.loc); ok {
			return DateValue(t)
		}
	}
	return literal(v)
}

func isDate(t dsl.FieldType) bool { return t == dsl.FieldDate || t == dsl.FieldDatetime }

// field is Entity.field(_:): a declared field or the pseudo field of a reserved value.
func field(e *dsl.Entity, id string) *dsl.Field {
	if e == nil {
		return nil
	}
	for i := range e.Fields {
		if e.Fields[i].ID == id {
			return &e.Fields[i]
		}
	}
	if mid, ok := strings.CutPrefix(id, "$marks."); ok {
		if !e.IsShared() {
			return nil
		}
		for _, m := range e.Marks {
			if m.ID == mid {
				return &dsl.Field{ID: id, Name: m.Label, Type: dsl.FieldInteger}
			}
		}
		return nil
	}
	switch {
	case id == "$author" && e.IsShared():
		return &dsl.Field{ID: id, Name: "投稿者", Type: dsl.FieldString}
	case id == "$createdAt":
		return &dsl.Field{ID: id, Name: "投稿日時", Type: dsl.FieldDatetime}
	}
	return nil
}

// get is Record.subscript: reserved values, then data.
func get(r Record, key string) any {
	if mid, ok := strings.CutPrefix(key, "$marks."); ok {
		return float64(r.Marks[mid].Count)
	}
	switch key {
	case "$author":
		if r.AuthorName == "" {
			return nil
		}
		return r.AuthorName
	case "$createdAt":
		if r.CreatedAt == "" {
			return nil
		}
		return r.CreatedAt
	}
	return normalizeJSON(r.Data[key])
}

// FieldValue is a row's field value: "id" = record id, "<ref>.<field>" = lookup
// (one level), computed fields are evaluated (memoized), others are stored values.
func (ev *Evaluator) FieldValue(id string, row *Row) Value {
	if id == "id" {
		return StringValue(row.Record.ID)
	}
	if i := strings.IndexByte(id, '.'); i >= 0 {
		return ev.lookup(id[:i], id[i+1:], row)
	}
	f := field(row.Entity, id)
	if f == nil || f.Computed == nil {
		return ev.stored(get(row.Record, id), f)
	}
	eid := ""
	if row.Entity != nil {
		eid = row.Entity.ID
	}
	key := eid + "/" + row.Record.ID + "/" + id
	if !row.Draft {
		if hit, ok := ev.computed[key]; ok {
			return hit
		}
	}
	if ev.inProgress[key] {
		return Value{} // cycle
	}
	ev.inProgress[key] = true
	v := ev.Eval(*f.Computed, row)
	delete(ev.inProgress, key)
	if !row.Draft {
		ev.computed[key] = v
	}
	return v
}

func (ev *Evaluator) lookup(refField, target string, row *Row) Value {
	if strings.Contains(target, ".") {
		return Value{}
	}
	f := field(row.Entity, refField)
	if f == nil || f.Type != dsl.FieldRef || f.RefEntity == "" {
		return Value{}
	}
	id, ok := get(row.Record, refField).(string)
	if !ok || id == "" {
		return Value{}
	}
	for _, r := range ev.records(f.RefEntity) {
		if r.ID == id {
			return ev.FieldValue(target, &Row{Entity: ev.entities[f.RefEntity], Record: r})
		}
	}
	return Value{}
}

// viewRecords merges computed values into data (ExprEvaluator.viewRecords).
// A computed value that is nil drops the key.
func (ev *Evaluator) viewRecords(entityID string) []Record {
	if hit, ok := ev.views[entityID]; ok {
		return hit
	}
	raw := ev.records(entityID)
	e := ev.entities[entityID]
	var computed []*dsl.Field
	if e != nil {
		for i := range e.Fields {
			if e.Fields[i].Computed != nil {
				computed = append(computed, &e.Fields[i])
			}
		}
	}
	if len(computed) == 0 {
		return raw
	}
	out := make([]Record, len(raw))
	for i, r := range raw {
		row := &Row{Entity: e, Record: r}
		data := make(map[string]any, len(r.Data)+len(computed))
		for k, v := range r.Data {
			data[k] = v
		}
		for _, f := range computed {
			v := ev.FieldValue(f.ID, row)
			if v.IsNil() {
				delete(data, f.ID)
			} else {
				data[f.ID] = v.recordJSON(f.Type == dsl.FieldDatetime, ev.loc)
			}
		}
		r.Data = data
		out[i] = r
	}
	ev.views[entityID] = out
	return out
}

// ---- agg ---------------------------------------------------------------------

// filter is a Filter whose value Expr has been evaluated to a JSON value.
type filter struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

// resolve evaluates each filter.value Expr in the outer row. If one evaluates
// to nil it returns false: the filters match no record.
func (ev *Evaluator) resolve(fs []dsl.Filter, row *Row) ([]filter, bool) {
	out := make([]filter, 0, len(fs))
	for _, f := range fs {
		v := normalizeJSON(f.Value)
		if m, ok := v.(map[string]any); ok {
			if len(m) == 1 && m["me"] == true {
				out = append(out, filter{f.Field, f.Op, v}) // {"me": true}: resolved by the caller, not an Expr
				continue
			}
			var e dsl.Expr
			raw, err := json.Marshal(m)
			if err == nil {
				err = json.Unmarshal(raw, &e)
			}
			if err != nil {
				continue // iOS drops a filter it cannot decode
			}
			val := ev.Eval(e, row)
			if val.IsNil() {
				return nil, false
			}
			v = val.recordJSON(false, ev.loc)
		}
		out = append(out, filter{f.Field, f.Op, v})
	}
	return out, true
}

func (ev *Evaluator) aggregate(a *dsl.AggExpr, row *Row) Value {
	resolved, ok := ev.resolve(a.Filter, row)
	keyBytes, _ := json.Marshal(struct {
		A, E, F, B string
		Fs         []filter
		None       bool
	}{a.Agg, a.Entity, a.Field, a.By, resolved, !ok})
	key := string(keyBytes)
	if hit, ok := ev.aggs[key]; ok {
		return hit
	}
	e := ev.entities[a.Entity]
	usesComputed := false
	names := []string{a.Field, a.By}
	for _, f := range a.Filter {
		names = append(names, f.Field)
	}
	for _, n := range names {
		if n == "" {
			continue
		}
		if f := field(e, n); f != nil && f.Computed != nil {
			usesComputed = true
		}
	}
	base := ev.records(a.Entity)
	if usesComputed {
		base = ev.viewRecords(a.Entity)
	}
	rows := base
	if !ok {
		rows = nil
	} else if len(resolved) > 0 {
		rows = nil
		for _, r := range base {
			ok := true
			for _, f := range resolved {
				if !ev.matches(f, r, field(e, f.Field)) {
					ok = false
					break
				}
			}
			if ok {
				rows = append(rows, r)
			}
		}
	}
	var nums []float64
	if a.Field != "" {
		for _, r := range rows {
			switch x := get(r, a.Field).(type) {
			case float64:
				nums = append(nums, x)
			case string:
				if n, ok := parseNumber(x); ok {
					nums = append(nums, n)
				}
			}
		}
	}
	var v Value
	switch a.Agg {
	case "count":
		v = NumberValue(float64(len(rows)))
	case "sum":
		v = NumberValue(sum(nums))
	case "avg":
		if len(nums) > 0 {
			v = NumberValue(sum(nums) / float64(len(nums)))
		}
	case "min", "max":
		if len(nums) > 0 {
			m := nums[0]
			for _, n := range nums[1:] {
				if (a.Agg == "min" && n < m) || (a.Agg == "max" && n > m) {
					m = n
				}
			}
			v = NumberValue(m)
		}
	case "latest", "earliest":
		if a.Field != "" {
			if r, ok := ev.pick(a.Agg, a.By, e, rows); ok {
				v = ev.stored(get(r, a.Field), field(e, a.Field))
			}
		}
	}
	ev.aggs[key] = v
	return v
}

func sum(ns []float64) float64 {
	s := 0.0
	for _, n := range ns {
		s += n
	}
	return s
}

// pick is Aggregator.pick: order by `by` (default: the entity's first
// date/datetime field, else creation order); same date → later createdAt is
// newer; records whose `by` is not a date are skipped.
func (ev *Evaluator) pick(op, by string, e *dsl.Entity, rows []Record) (Record, bool) {
	byField := by
	if byField == "" && e != nil {
		for _, f := range e.Fields {
			if isDate(f.Type) {
				byField = f.ID
				break
			}
		}
	}
	type keyed struct {
		date    time.Time
		hasDate bool
		created string
		index   int
		r       Record
	}
	var ks []keyed
	for i, r := range rows {
		k := keyed{created: r.CreatedAt, index: i, r: r}
		if byField != "" {
			t, ok := parseDateJSON(get(r, byField), ev.loc)
			if !ok {
				continue
			}
			k.date, k.hasDate = t, true
		}
		ks = append(ks, k)
	}
	if len(ks) == 0 {
		return Record{}, false
	}
	sort.SliceStable(ks, func(i, j int) bool {
		a, b := ks[i], ks[j]
		if a.hasDate && b.hasDate && !a.date.Equal(b.date) {
			return a.date.Before(b.date)
		}
		if a.created != b.created {
			return a.created < b.created
		}
		return a.index < b.index
	})
	if op == "earliest" {
		return ks[0].r, true
	}
	return ks[len(ks)-1].r, true
}

// matches is ExprEvaluator.matches + Aggregator.matches.
func (ev *Evaluator) matches(f filter, r Record, fd *dsl.Field) bool {
	if f.Op == "contains" && fd != nil && fd.Type == dsl.FieldRefs {
		ids := ev.liveRefIDs(get(r, f.Field), fd)
		want, _ := f.Value.(string)
		return len(ids) == 0 || contains(ids, want)
	}
	if mid, ok := strings.CutPrefix(f.Field, "$marks."); ok {
		if f.Value == nil {
			return false
		}
		mine := r.Marks[mid].Mine
		if f.Op == "neq" {
			return !mine
		}
		return mine
	}
	if f.Field == "$author" {
		uid, ok := f.Value.(string)
		if !ok {
			return false
		}
		same := r.Author == uid
		if f.Op == "neq" {
			return !same
		}
		return same
	}
	v := get(r, f.Field)
	switch f.Op {
	case "thisMonth", "today", "thisYear", "thisWeek":
		t, ok := parseDateJSON(v, ev.loc)
		if !ok {
			return false
		}
		switch f.Op {
		case "thisMonth":
			return sameMonth(t, ev.now, ev.loc)
		case "today":
			return sameDay(t, ev.now, ev.loc)
		case "thisYear":
			return sameYear(t, ev.now, ev.loc)
		}
		start, end := week(ev.now, ev.loc)
		return !t.Before(start) && t.Before(end)
	case "contains":
		ids := refIDs(v)
		want, _ := f.Value.(string)
		return len(ids) == 0 || contains(ids, want)
	case "eq", "neq":
		same := jsonEqual(v, f.Value) || jsonDisplay(v) == jsonDisplay(f.Value)
		return same == (f.Op == "eq")
	case "gt", "gte", "lt", "lte":
		c, ok := ev.filterOrder(v, f.Value)
		if !ok {
			return false
		}
		switch f.Op {
		case "gt":
			return c > 0
		case "gte":
			return c >= 0
		case "lt":
			return c < 0
		}
		return c <= 0
	}
	return true
}

// filterOrder is Aggregator.filterOrder: numbers (numeric strings, commas
// allowed) as numbers, then dates (date strings) as dates; nothing else.
func (ev *Evaluator) filterOrder(a, b any) (int, bool) {
	num := func(v any) (float64, bool) {
		if f, ok := jsonDouble(v); ok {
			return f, true
		}
		if s, ok := v.(string); ok {
			return parseNumber(s)
		}
		return 0, false
	}
	if x, ok := num(a); ok {
		if y, ok := num(b); ok {
			return cmpFloat(x, y), true
		}
	}
	if x, ok := parseDateJSON(a, ev.loc); ok {
		if y, ok := parseDateJSON(b, ev.loc); ok {
			return x.Compare(y), true
		}
	}
	return 0, false
}

// refIDs is Aggregator.refIds: an array of ids, or one string as one id.
func refIDs(v any) []string {
	switch x := v.(type) {
	case []any:
		var out []string
		for _, e := range x {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if x == "" {
			return nil
		}
		return []string{x}
	}
	return nil
}

// liveRefIDs drops ids whose record no longer exists.
func (ev *Evaluator) liveRefIDs(v any, f *dsl.Field) []string {
	ids := refIDs(v)
	if len(ids) == 0 || f.RefEntity == "" {
		return ids
	}
	live := map[string]bool{}
	for _, r := range ev.records(f.RefEntity) {
		live[r.ID] = true
	}
	var out []string
	for _, id := range ids {
		if live[id] {
			out = append(out, id)
		}
	}
	return out
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// ---- daily check (#172) --------------------------------------------------------

// dailyDates is DailyCheck.dates: "<field>__dates" plus the field value, as
// sorted unique "YYYY-MM-DD".
func (ev *Evaluator) dailyDates(r Record, fieldID string) map[string]bool {
	set := map[string]bool{}
	if xs, ok := get(r, fieldID+"__dates").([]any); ok {
		for _, x := range xs {
			if d, ok := ev.dayOf(x); ok {
				set[d] = true
			}
		}
	}
	if d, ok := ev.dayOf(get(r, fieldID)); ok {
		set[d] = true
	}
	return set
}

func (ev *Evaluator) dayOf(v any) (string, bool) {
	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}
	t, ok := parseDate(s, ev.loc)
	if !ok {
		return "", false
	}
	if strings.Contains(s, "T") {
		return dayKey(t, ev.loc), true
	}
	rs := []rune(s)
	if len(rs) > 10 {
		rs = rs[:10]
	}
	return string(rs), true
}

func (ev *Evaluator) streak(days map[string]bool) int {
	d := startOfDay(ev.now, ev.loc)
	if !days[dayKey(d, ev.loc)] {
		d = addDays(d, -1, ev.loc)
	}
	n := 0
	for days[dayKey(d, ev.loc)] && n <= maxHistory {
		n++
		d = addDays(d, -1, ev.loc)
	}
	return n
}

func (ev *Evaluator) doneInDays(days map[string]bool, n int) int {
	if n < 1 {
		return 0
	}
	start := startOfDay(ev.now, ev.loc)
	c := 0
	for i := 0; i < n && i < maxHistory; i++ {
		if days[dayKey(addDays(start, -i, ev.loc), ev.loc)] {
			c++
		}
	}
	return c
}

// roundAway is Swift's .toNearestOrAwayFromZero.
func roundAway(x float64) float64 { return math.Round(x) }
