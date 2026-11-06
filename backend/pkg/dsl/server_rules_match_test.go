package dsl

import (
	"reflect"
	"testing"
)

func ruleIDs(rs []*ServerRule) []string {
	out := []string{}
	for _, r := range rs {
		out = append(out, r.ID)
	}
	return out
}

// #341: a write matches only the record rules of its entity and kind; an
// update rule with fields needs one of them changed.
func TestMatchRecordRules(t *testing.T) {
	a := serverRulesApp(t)
	for _, tc := range []struct {
		name, entity, kind string
		changed            []string
		want               []string
	}{
		{"created rsvp", "rsvps", TriggerCreated, nil, []string{"notify_rsvp"}},
		{"created message", "messages", TriggerCreated, nil, []string{"new_message"}},
		{"updated rsvp has no rule", "rsvps", TriggerUpdated, []string{"answer"}, []string{}},
		{"created event has no rule", "events", TriggerCreated, nil, []string{}},
		{"status changed", "events", TriggerUpdated, []string{"date", "status"}, []string{"status_changed"}},
		{"other field changed", "events", TriggerUpdated, []string{"title"}, []string{}},
		{"nothing changed", "events", TriggerUpdated, nil, []string{}},
		{"time rules never match", "", "", nil, []string{}},
	} {
		if got := ruleIDs(a.MatchRecordRules(tc.entity, tc.kind, tc.changed)); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
	a.ServerRules[2].On.Fields = nil
	if got := ruleIDs(a.MatchRecordRules("events", TriggerUpdated, []string{"title"})); !reflect.DeepEqual(got, []string{"status_changed"}) {
		t.Errorf("updated without fields: got %v", got)
	}
	if !a.HasRecordRules("events", TriggerUpdated) || a.HasRecordRules("events", TriggerCreated) {
		t.Error("HasRecordRules")
	}
}

func TestReferencedFields(t *testing.T) {
	a := serverRulesApp(t)
	want := map[string][]string{
		"notify_rsvp":    {"answer", "event"}, // if + authorOf + increment ref ({$author} is not a field)
		"new_message":    {"body"},            // placeholder
		"status_changed": {"status", "title"}, // $prev.status + placeholders + createRecord values
		"morning":        {},
	}
	for i := range a.ServerRules {
		r := &a.ServerRules[i]
		if got := r.ReferencedFields(); !reflect.DeepEqual(got, want[r.ID]) {
			t.Errorf("%s: got %v, want %v", r.ID, got, want[r.ID])
		}
	}
	r := ServerRule{
		If: expr(t, `{"op":"and","args":[{"op":"gt","args":[{"ref":"event.yes_count"},1]},{"op":"neq","args":[{"ref":"id"},""]}]}`),
		Do: []RuleAction{{UpdateRecord: &RuleUpdateRecord{Ref: "event", Values: map[string]Expr{"status": *expr(t, `{"if":{"ref":"a"},"then":{"ref":"b"},"else":"x"}`)}}}},
	}
	if got := r.ReferencedFields(); !reflect.DeepEqual(got, []string{"a", "b", "event"}) {
		t.Errorf("lookup / id / if-then-else: got %v", got)
	}
}
