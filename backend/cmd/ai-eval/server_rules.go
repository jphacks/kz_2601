package main

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/runner/engine"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

// Server rules (#343) expect keys, checked by checkServerRules (run with
// -server-rules so the pass is on):
//   - serverRules (bool): app.serverRules is non-empty (validated by dsl.Parse).
//   - ruleTrigger (string): some rule's trigger is created|updated|daily|weekly|monthly.
//   - ruleAction (string): some rule has a notify|createRecord|updateRecord|increment action.
//   - ruleTo (string): some notify goes to owner|author|authorOf|members|recipients.
//   - ruleSim (bool): every rule runs through engine.PlanRule (on.each:
//     PlanEach) on made-up records (the 模擬の実行) without an error.
//   - ruleEach (bool): some rule has on.each (#340 reads).
//   - ruleAgg (bool): some rule reads records with agg.
//   - ruleWith (bool): some notify has with.
//   - ruleLookup (string): some rule has a lookup with op exists|notExists|overlaps|notOverlaps.
//   - ruleSimilar (bool): some rule searches the most similar record (similar).
//   - rulePick (string): some rule picks one record with order random|oldest|newest (pick).
//
// Cases: internal/ai/eval/server_rules_cases.json (-cases).
func checkServerRules(exp map[string]any, a *dsl.App) []string {
	var miss []string
	if want, ok := exp["serverRules"].(bool); ok && (len(a.ServerRules) > 0) != want {
		miss = append(miss, fmt.Sprintf("serverRules=%d", len(a.ServerRules)))
	}
	var triggers, actions, tos, lookups, picks []string
	has := map[string]bool{}
	for i := range a.ServerRules {
		r := &a.ServerRules[i]
		triggers = append(triggers, r.On.Record+r.On.Every)
		has["ruleEach"] = has["ruleEach"] || r.On.Each != ""
		has["ruleAgg"] = has["ruleAgg"] || len(r.AggEntities()) > 0
		has["ruleSimilar"] = has["ruleSimilar"] || r.Similar != nil
		if r.Lookup != nil {
			lookups = append(lookups, r.Lookup.Op)
		}
		if r.Pick != nil {
			picks = append(picks, r.Pick.Order)
		}
		for _, act := range r.Do {
			switch {
			case act.Notify != nil:
				actions = append(actions, "notify")
				tos = append(tos, act.Notify.To.Kind)
				has["ruleWith"] = has["ruleWith"] || len(act.Notify.With) > 0
			case act.CreateRecord != nil:
				actions = append(actions, "createRecord")
			case act.UpdateRecord != nil:
				actions = append(actions, "updateRecord")
			case act.Increment != nil:
				actions = append(actions, "increment")
			}
		}
	}
	for key, got := range map[string][]string{"ruleTrigger": triggers, "ruleAction": actions, "ruleTo": tos, "ruleLookup": lookups, "rulePick": picks} {
		if want, ok := exp[key].(string); ok && !slices.Contains(got, want) {
			miss = append(miss, fmt.Sprintf("%s=%v", key, got))
		}
	}
	for _, key := range []string{"ruleEach", "ruleAgg", "ruleWith", "ruleSimilar"} {
		if want, ok := exp[key].(bool); ok && want != has[key] {
			miss = append(miss, fmt.Sprintf("%s=%v", key, has[key]))
		}
	}
	if want, ok := exp["ruleSim"].(bool); ok && want {
		if len(a.ServerRules) == 0 {
			miss = append(miss, "ruleSim: no rules")
		}
		for i := range a.ServerRules {
			if err := simulateRule(a, &a.ServerRules[i]); err != nil {
				miss = append(miss, "ruleSim: "+err.Error())
			}
		}
	}
	return miss
}

// simulateRule runs one rule on made-up records (every ref resolves to a
// made-up record of its entity).
func simulateRule(a *dsl.App, r *dsl.ServerRule) error {
	at := time.Date(2026, 11, 7, 7, 0, 0, 0, time.UTC)
	ev := engine.Trigger{AppID: "sim", Kind: engine.KindScheduled, At: at, Actor: "u2"}
	if !r.IsScheduled() {
		e := a.EntityByID(r.On.Entity)
		if e == nil {
			return fmt.Errorf("rule %q: entity %q not found", r.ID, r.On.Entity)
		}
		ev.Kind, ev.Entity, ev.Scope = r.On.Record, e.ID, e.EffectiveScope()
		ev.Record = sampleRecord(e, "r1", false)
		if r.On.Record == dsl.TriggerUpdated {
			prev := sampleRecord(e, "r1", true)
			ev.Prev = &prev
		}
	}
	if r.On.Each != "" {
		ev.Scope = schedScope(a)
		_, err := engine.PlanEach(context.Background(), a, r, ev, sampleRefs{a})
		return err
	}
	_, err := engine.PlanRule(context.Background(), a, r, ev, sampleRefs{a})
	return err
}

// schedScope is the scope a time-triggered rule runs on (any shared entity
// makes the app's schedule shared).
func schedScope(a *dsl.App) dsl.EntityScope {
	s := dsl.ScopePrivate
	for i := range a.Entities {
		switch a.Entities[i].EffectiveScope() {
		case dsl.ScopePublic:
			return dsl.ScopePublic
		case dsl.ScopeShared:
			s = dsl.ScopeShared
		}
	}
	return s
}

type sampleRefs struct{ a *dsl.App }

func (s sampleRefs) Record(_ context.Context, entity, id string) (expreval.Record, bool, error) {
	e := s.a.EntityByID(entity)
	if e == nil {
		return expreval.Record{}, false, nil
	}
	return sampleRecord(e, id, false), true, nil
}

// List gives two made-up records of the entity (on.each / agg).
func (s sampleRefs) List(_ context.Context, entity string) ([]expreval.Record, error) {
	e := s.a.EntityByID(entity)
	if e == nil {
		return nil, nil
	}
	return []expreval.Record{sampleRecord(e, "l1", false), sampleRecord(e, "l2", true)}, nil
}

// sampleRecord fills every field with a value of its type (prev: a different one).
func sampleRecord(e *dsl.Entity, id string, prev bool) expreval.Record {
	data := map[string]any{}
	for _, f := range e.Fields {
		if f.Computed != nil {
			continue
		}
		switch f.Type {
		case dsl.FieldString, dsl.FieldText:
			data[f.ID] = map[bool]string{false: "テスト", true: "前"}[prev]
		case dsl.FieldInteger, dsl.FieldNumber, dsl.FieldCurrency:
			data[f.ID] = map[bool]float64{false: 3, true: 1}[prev]
		case dsl.FieldBoolean:
			data[f.ID] = !prev
		case dsl.FieldDate:
			data[f.ID] = "2026-11-07"
		case dsl.FieldDatetime:
			data[f.ID] = "2026-11-07T07:00:00Z"
		case dsl.FieldEnum:
			if n := len(f.Options); n > 0 {
				data[f.ID] = f.Options[map[bool]int{false: 0, true: n - 1}[prev]]
			}
		case dsl.FieldRef:
			data[f.ID] = "ref1"
		case dsl.FieldRefs:
			data[f.ID] = []any{"ref1"}
		}
	}
	return expreval.Record{ID: id, Author: "u2", AuthorName: "花子", Data: data}
}
