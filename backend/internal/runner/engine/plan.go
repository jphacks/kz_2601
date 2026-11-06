// Package engine evaluates one server rule (#340) against one trigger and
// returns what to do (#341). It does no I/O of its own: ref targets come from
// a RefReader, and the caller writes and notifies. It must not import
// internal/store (the appId-bound handle lives in the runner, not here).
package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

// Trigger kinds.
const (
	KindCreated   = dsl.TriggerCreated
	KindUpdated   = dsl.TriggerUpdated
	KindScheduled = "scheduled"
)

// Trigger is one event a rule runs on.
type Trigger struct {
	AppID string
	// Scope is the data scope of the app the event belongs to.
	Scope dsl.EntityScope
	// Kind is created / updated / scheduled.
	Kind string
	// Entity, Record: the record written (empty for scheduled).
	Entity string
	Record expreval.Record
	// Prev is the record before the update (updated only).
	Prev *expreval.Record
	// Actor is the uid whose write caused the event ("" for scheduled).
	Actor string
	// Version is the public DSL version at the time of the event (the rule
	// runs with that version).
	Version int
	// At is when the event happened ("now" / "today" in expressions).
	At time.Time
	// Location is the creator's time zone; nil = UTC.
	Location *time.Location
}

// Lister lists the records of one entity of the trigger's app (the side the
// rule works on): on.each and agg read it. The runner's reader caps how many
// records a run may read and returns an error wrapping ErrReadLimit beyond
// that (the run is stopped, never computed on part of the records).
type Lister interface {
	List(ctx context.Context, entity string) ([]expreval.Record, error)
}

// ErrReadLimit: a read cap stopped the run (errors.As a *ReadLimitError for
// which one).
var ErrReadLimit = errors.New("engine: read limit")

// ReadLimitError names the cap (a quota scope) that stopped the reads.
type ReadLimitError struct {
	Scope string
	Limit int
	Used  int
}

func (e *ReadLimitError) Error() string {
	return fmt.Sprintf("engine: read limit %s (%d > %d)", e.Scope, e.Used, e.Limit)
}

// Unwrap makes errors.Is(err, ErrReadLimit) true.
func (e *ReadLimitError) Unwrap() error { return ErrReadLimit }

// errNoLister: a rule reads records (each / agg) but the reader cannot list.
var errNoLister = errors.New("engine: the rule lists records but the reader has no List")

// RefReader reads one record a ref field of the trigger record points to.
// The runner gives a reader bound to the trigger's appId; found=false when the record does not exist.
type RefReader interface {
	Record(ctx context.Context, entity, id string) (rec expreval.Record, found bool, err error)
}

// Message is one notification to deliver to resolved recipients.
type Message struct {
	AppID  string
	RuleID string
	UIDs   []string
	Title  string
	Body   string
}

// Notifier delivers notifications (implemented by a later PR, #269).
type Notifier interface {
	Notify(ctx context.Context, m Message) error
}

// WriteKind is the kind of a planned write.
type WriteKind string

// Write kinds.
const (
	WriteCreate    WriteKind = "create"
	WriteUpdate    WriteKind = "update"
	WriteIncrement WriteKind = "increment"
)

// PlannedWrite is one record write. Values are JSON values as stored in
// Record.data (update: nil clears the field).
type PlannedWrite struct {
	Action   int // index in rule.Do
	Kind     WriteKind
	Entity   string
	RecordID string         // update / increment target ("" for create)
	Values   map[string]any // create / update
	Field    string         // increment
	By       float64        // increment
}

// PlannedNotify is one notification with its recipient still a kind (who it
// resolves to is decided by the runner).
type PlannedNotify struct {
	Action int // index in rule.Do
	To     dsl.RuleRecipient
	// RefEntity, RefID: for authorOf, the record whose author receives it
	// ("" when the ref field is empty).
	RefEntity string
	RefID     string
	// UIDs: for recipients, the people the addressed record is sent to.
	UIDs  []string
	Title string
	Body  string
}

// Plan is what one rule does for one trigger. Empty = nothing to do.
type Plan struct {
	Writes   []PlannedWrite
	Notifies []PlannedNotify
}

// Empty reports whether the plan does nothing.
func (p Plan) Empty() bool { return len(p.Writes) == 0 && len(p.Notifies) == 0 }

// PlanRule evaluates rule for ev (named PlanRule because the result type is
// Plan): the trigger must match the rule, `if` must be
// true, then each action's values / by / text are computed. An action whose
// target ref is empty (or whose by is not a number) is skipped. A rule with
// agg needs refs to implement Lister. A rule with on.each runs on one
// record here (ev.Record); PlanEach runs it on every record.
func PlanRule(ctx context.Context, app *dsl.App, rule *dsl.ServerRule, ev Trigger, refs RefReader) (Plan, error) {
	return planOne(ctx, app, rule, ev, refs, map[string][]expreval.Record{})
}

// RowPlan is the plan of an on.each rule for one record.
type RowPlan struct {
	// Trigger is ev with Entity / Record set to the record (the row the
	// notifications resolve author / authorOf from).
	Trigger Trigger
	Plan    Plan
}

// PlanEach evaluates an on.each rule for every record of its entity (listed
// through refs, which must implement Lister) and returns the non-empty plans
// in the order the records were listed.
func PlanEach(ctx context.Context, app *dsl.App, rule *dsl.ServerRule, ev Trigger, refs RefReader) ([]RowPlan, error) {
	if app == nil || rule == nil {
		return nil, errors.New("engine: app and rule are required")
	}
	if rule.On.Each == "" || ev.Kind != KindScheduled {
		return nil, fmt.Errorf("engine: rule %q is not a time-triggered each rule (trigger %q)", rule.ID, ev.Kind)
	}
	l, ok := refs.(Lister)
	if !ok {
		return nil, errNoLister
	}
	lists := map[string][]expreval.Record{}
	recs, err := l.List(ctx, rule.On.Each)
	if err != nil {
		return nil, err
	}
	lists[rule.On.Each] = recs
	var out []RowPlan
	for _, rec := range recs {
		if rec.ID == "" {
			continue
		}
		rt := ev
		rt.Entity, rt.Record = rule.On.Each, rec
		p, err := planOne(ctx, app, rule, rt, refs, lists)
		if err != nil {
			return nil, err
		}
		if !p.Empty() {
			out = append(out, RowPlan{Trigger: rt, Plan: p})
		}
	}
	return out, nil
}

// planOne is PlanRule with the lists already read in this run (shared by
// the rows of PlanEach).
func planOne(ctx context.Context, app *dsl.App, rule *dsl.ServerRule, ev Trigger, refs RefReader, lists map[string][]expreval.Record) (Plan, error) {
	if app == nil || rule == nil {
		return Plan{}, errors.New("engine: app and rule are required")
	}
	row, fire, err := matchTrigger(app, rule, ev)
	if err != nil || !fire {
		return Plan{}, err
	}
	loc := ev.Location
	if loc == nil {
		loc = time.UTC
	}
	src := &refSource{ctx: ctx, row: row, refs: refs, cache: map[string][]expreval.Record{}, lists: lists, listed: map[string]bool{}}
	for _, id := range rule.AggEntities() {
		src.listed[id] = true
	}
	e := expreval.New(expreval.Env{Entities: app.Entities, Source: src, Now: ev.At, Location: loc})
	p := planner{app: app, ev: ev, row: row, e: e, loc: loc, each: rule.On.Each != ""}
	if rule.If != nil {
		v := e.Eval(*rule.If, row)
		if src.err != nil {
			return Plan{}, src.err
		}
		if v.Kind != expreval.Bool || !v.Bool {
			return Plan{}, nil
		}
	}
	var out Plan
	for i := range rule.Do {
		if err := p.action(i, &rule.Do[i], &out); err != nil {
			return Plan{}, err
		}
		if src.err != nil {
			return Plan{}, src.err
		}
	}
	return out, nil
}

// matchTrigger checks that ev is the rule's trigger and returns the row of
// the rule's expressions (nil for a time trigger). fire=false when an
// updated trigger's fields did not change.
func matchTrigger(app *dsl.App, rule *dsl.ServerRule, ev Trigger) (*expreval.Row, bool, error) {
	if rule.IsScheduled() {
		if ev.Kind != KindScheduled {
			return nil, false, fmt.Errorf("engine: rule %q is time-triggered but the trigger is %q", rule.ID, ev.Kind)
		}
		if rule.On.Each == "" {
			return nil, true, nil
		}
		ent := app.EntityByID(rule.On.Each)
		if ent == nil || ev.Record.ID == "" || ev.Entity != ent.ID {
			return nil, false, fmt.Errorf("engine: rule %q runs on each %q record; use PlanEach", rule.ID, rule.On.Each)
		}
		return &expreval.Row{Entity: ent, Record: ev.Record}, true, nil
	}
	if ev.Kind != rule.On.Record || ev.Entity != rule.On.Entity {
		return nil, false, fmt.Errorf("engine: rule %q is %s on %q but the trigger is %s on %q", rule.ID, rule.On.Record, rule.On.Entity, ev.Kind, ev.Entity)
	}
	ent := app.EntityByID(ev.Entity)
	if ent == nil {
		return nil, false, fmt.Errorf("engine: entity %q is not in the app", ev.Entity)
	}
	row := &expreval.Row{Entity: ent, Record: ev.Record}
	if ev.Kind == KindUpdated {
		if ev.Prev == nil {
			return nil, false, fmt.Errorf("engine: rule %q: updated trigger without the previous record", rule.ID)
		}
		row.Prev = ev.Prev
		if len(rule.On.Fields) > 0 && !slices.ContainsFunc(rule.On.Fields, func(f string) bool {
			return !sameJSON(ev.Prev.Data[f], ev.Record.Data[f])
		}) {
			return row, false, nil
		}
	}
	return row, true, nil
}

func sameJSON(a, b any) bool {
	x, err1 := json.Marshal(a)
	y, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && bytes.Equal(x, y)
}

type planner struct {
	app *dsl.App
	ev  Trigger
	row *expreval.Row
	e   *expreval.Evaluator
	loc *time.Location
	// each: the row was just listed (on.each), so an update of the row that
	// changes nothing is not written (a daily recount writes only changes).
	each bool
}

func (p *planner) action(i int, act *dsl.RuleAction, out *Plan) error {
	switch {
	case act.Notify != nil:
		n := PlannedNotify{Action: i, To: act.Notify.To,
			Title: p.render(act.Notify.Title, dsl.MaxNotifyTitleRunes, act.Notify.With),
			Body:  p.render(act.Notify.Body, dsl.MaxNotifyBodyRunes, act.Notify.With)}
		if n.To.Kind == dsl.RecipientAuthorOf && p.row != nil {
			if f := p.row.Entity.FieldByID(n.To.Field); f != nil {
				n.RefEntity, n.RefID = f.RefEntity, refID(p.row.Record, f.ID)
			}
		}
		if n.To.Kind == dsl.RecipientRecipients && p.row != nil {
			n.UIDs = p.row.Entity.Recipients(p.row.Record.Data)
		}
		out.Notifies = append(out.Notifies, n)
	case act.CreateRecord != nil:
		target := p.app.EntityByID(act.CreateRecord.Entity)
		if target == nil {
			return fmt.Errorf("engine: createRecord entity %q is not in the app", act.CreateRecord.Entity)
		}
		vals := p.values(target, act.CreateRecord.Values)
		for k, v := range vals {
			if v == nil {
				delete(vals, k) // a new record simply lacks the field
			}
		}
		out.Writes = append(out.Writes, PlannedWrite{Action: i, Kind: WriteCreate, Entity: target.ID, Values: vals})
	case act.UpdateRecord != nil:
		target, id, ok := p.target(act.UpdateRecord.Ref)
		if !ok {
			return nil
		}
		vals := p.values(target, act.UpdateRecord.Values)
		if p.each && act.UpdateRecord.Ref == "" && p.unchanged(vals) {
			return nil
		}
		out.Writes = append(out.Writes, PlannedWrite{Action: i, Kind: WriteUpdate, Entity: target.ID, RecordID: id, Values: vals})
	case act.Increment != nil:
		if act.Increment.Ref == "" {
			return nil
		}
		target, id, ok := p.target(act.Increment.Ref)
		if !ok {
			return nil
		}
		by := 1.0
		if act.Increment.By != nil {
			v := p.e.Eval(*act.Increment.By, p.row)
			if v.Kind != expreval.Number || math.IsNaN(v.Num) || math.IsInf(v.Num, 0) {
				return nil
			}
			by = v.Num
		}
		out.Writes = append(out.Writes, PlannedWrite{Action: i, Kind: WriteIncrement, Entity: target.ID, RecordID: id,
			Field: act.Increment.Field, By: by})
	}
	return nil
}

// unchanged reports whether writing vals to the row changes nothing.
func (p *planner) unchanged(vals map[string]any) bool {
	for k, v := range vals {
		if !sameJSON(normalize(v), normalize(p.row.Record.Data[k])) {
			return false
		}
	}
	return true
}

// normalize makes stored numbers (int64 from Firestore) compare equal to
// computed ones (float64).
func normalize(v any) any {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case float32:
		return float64(n)
	}
	return v
}

// target is the trigger record (ref "") or the record its ref field points
// to. ok=false: no record trigger, or the ref field is empty / unknown.
func (p *planner) target(ref string) (*dsl.Entity, string, bool) {
	if p.row == nil {
		return nil, "", false
	}
	if ref == "" {
		return p.row.Entity, p.row.Record.ID, p.row.Record.ID != ""
	}
	f := p.row.Entity.FieldByID(ref)
	if f == nil || f.Type != dsl.FieldRef {
		return nil, "", false
	}
	t := p.app.EntityByID(f.RefEntity)
	id := refID(p.row.Record, ref)
	return t, id, t != nil && id != ""
}

func (p *planner) values(target *dsl.Entity, exprs map[string]dsl.Expr) map[string]any {
	out := make(map[string]any, len(exprs))
	for k, x := range exprs {
		f := target.FieldByID(k)
		if f == nil {
			continue
		}
		if f.Type == dsl.FieldRefs {
			out[k] = p.refsValue(x)
			continue
		}
		out[k] = p.e.Eval(x, p.row).RecordJSON(f.Type == dsl.FieldDatetime, p.loc)
	}
	return out
}

// refsValue copies a refs field read by {"ref": "<refs field>"} (a list is
// not an Expr value).
func (p *planner) refsValue(x dsl.Expr) any {
	if x.Kind != dsl.ExprRef || p.row == nil {
		return nil
	}
	f := p.row.Entity.FieldByID(x.Ref)
	if f == nil || f.Type != dsl.FieldRefs {
		return nil
	}
	xs, ok := p.row.Record.Data[x.Ref].([]any)
	if !ok {
		return nil
	}
	ids := make([]any, 0, len(xs))
	for _, v := range xs {
		if s, ok := v.(string); ok && s != "" {
			ids = append(ids, s)
		}
	}
	return ids
}

// list reads all records of entity once per run.
func (s *refSource) list(entity string) []expreval.Record {
	if hit, ok := s.lists[entity]; ok {
		return hit
	}
	if s.err != nil {
		return nil
	}
	l, ok := s.refs.(Lister)
	if !ok {
		s.err = errNoLister
		return nil
	}
	recs, err := l.List(s.ctx, entity)
	if err != nil {
		s.err = err
		return nil
	}
	s.lists[entity] = recs
	return recs
}

func refID(r expreval.Record, field string) string {
	s, _ := r.Data[field].(string)
	return s
}

// refSource is the expreval.Source of a rule: the records the trigger
// record's ref fields point to, read on demand.
type refSource struct {
	ctx   context.Context // expreval.Source has no ctx; lives for one PlanRule call
	row   *expreval.Row
	refs  RefReader
	cache map[string][]expreval.Record
	err   error
	// listed: entities the rule's aggs read (all their records, through
	// Lister); lists: what was listed in this run (shared by PlanEach rows).
	listed map[string]bool
	lists  map[string][]expreval.Record
}

func (s *refSource) Records(entity string) []expreval.Record {
	if s.listed[entity] {
		return s.list(entity)
	}
	if hit, ok := s.cache[entity]; ok {
		return hit
	}
	var out []expreval.Record
	if s.row != nil && s.row.Entity != nil && s.refs != nil && s.err == nil {
		seen := map[string]bool{}
		for _, f := range s.row.Entity.Fields {
			if f.Type != dsl.FieldRef || f.RefEntity != entity {
				continue
			}
			id := refID(s.row.Record, f.ID)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			r, found, err := s.refs.Record(s.ctx, entity, id)
			if err != nil {
				s.err = fmt.Errorf("engine: read %s/%s: %w", entity, id, err)
				return nil
			}
			if found {
				r.ID = id
				out = append(out, r)
			}
		}
	}
	s.cache[entity] = out
	return out
}
