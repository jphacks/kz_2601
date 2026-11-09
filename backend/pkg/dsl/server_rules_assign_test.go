package dsl

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const assignFile = "server_rules_assign.json"

// pick with count parses, validates, round-trips and is described.
func TestServerRulesAssignRoundTrip(t *testing.T) {
	raw := readsRaw(t, assignFile)
	a := readsApp(t, assignFile)
	out, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	var want, got any
	_ = json.Unmarshal(raw, &want)
	_ = json.Unmarshal(out, &got)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("round-trip mismatch\n got: %s", out)
	}
	descs := []string{
		"「シフト」が作られた時(条件に合えば)、「スタッフ」から(「休み」に同じ「日付」で無いものを)「シフト」が一番少ないものを 1 つ選んで、その記録を書き換える・「選んだスタッフ」を作った人に通知",
		"毎日 07:00、「シフト」の記録ごとに(条件に合えば)、「スタッフ」から「シフト」が一番少ないものを 1 つ選んで、その記録を書き換える",
	}
	for i := range a.ServerRules {
		if got := a.DescribeServerRule(&a.ServerRules[i]); got != descs[i] {
			t.Errorf("rule %d: %s", i, got)
		}
	}
}

// count, exclude and the candidates are reads; match and the row's own
// assignment are fields of the trigger snapshot.
func TestServerRulesAssignReads(t *testing.T) {
	a := readsApp(t, assignFile)
	if got := a.ServerRules[0].ReadEntities(); !slices.Equal(got, []string{"offs", "shifts", "staff"}) {
		t.Errorf("reads %v", got)
	}
	if got := a.ServerRules[0].ReferencedFields(); !slices.Equal(got, []string{"day", "staff"}) {
		t.Errorf("fields %v", got)
	}
}

func TestServerRulesAssignAccepts(t *testing.T) {
	cases := []struct {
		name string
		edit func(a *App)
	}{
		{"most", func(a *App) { a.ServerRules[1].Pick.Order = PickMost }},
		{"refs count", func(a *App) {
			a.Entities[0].Fields = append(a.Entities[0].Fields, Field{ID: "x", Name: "x", Type: FieldString})
			a.Entities[2].Fields = append(a.Entities[2].Fields, Field{ID: "many", Name: "複数", Type: FieldRefs, RefEntity: "staff"})
			a.ServerRules[1].Pick.Count = &PickCount{Entity: "offs", Field: "many"}
		}},
		{"match by id", func(a *App) {
			a.Entities[2].Fields = append(a.Entities[2].Fields, Field{ID: "shift", Name: "シフト", Type: FieldRef, RefEntity: "shifts"})
			a.ServerRules[0].Pick.Exclude.Match = &PickMatch{Field: "shift", Equals: LookupID}
		}},
		{"create log", func(a *App) {
			a.ServerRules[1].Do = []RuleAction{{CreateRecord: &RuleCreateRecord{Entity: "offs", Values: map[string]Expr{
				"staff": *expr(t, `{"ref":"$pick"}`), "day": *expr(t, `{"ref":"day"}`)}}}}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := readsApp(t, assignFile)
			c.edit(a)
			if err := a.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestServerRulesAssignRejects(t *testing.T) {
	cases := []struct {
		name string
		edit func(a *App)
		want string
	}{
		{"fewest without count", func(a *App) { a.ServerRules[1].Pick.Count = nil }, "order fewest needs count"},
		{"count with random", func(a *App) { a.ServerRules[1].Pick.Order = PickRandom }, "count is only for order fewest / most"},
		{"count entity", func(a *App) { a.ServerRules[1].Pick.Count.Entity = "ghost" }, `count: entity "ghost" does not exist`},
		{"count field", func(a *App) { a.ServerRules[1].Pick.Count.Field = "nope" }, `field "nope" is not a field of entity "shifts"`},
		{"count not ref", func(a *App) { a.ServerRules[1].Pick.Count.Field = "day" }, `field "day" is date; count needs a ref / refs field`},
		{"count other entity", func(a *App) {
			a.ServerRules[1].Pick.Count = &PickCount{Entity: "offs", Field: "staff"}
			a.Entities[2].Fields[0].RefEntity = "shifts"
		}, `points to entity "shifts", not to the picked entity "staff"`},
		{"count hidden", func(a *App) { a.Entities[1].Fields[1].Hidden = true }, `field "staff" is hidden`},
		{"count other side", func(a *App) {
			a.Entities = append(a.Entities, Entity{ID: "mine", Name: "自分", Fields: []Field{{ID: "s", Name: "s", Type: FieldRef, RefEntity: "staff"}}})
			a.ServerRules[1].Pick.Count = &PickCount{Entity: "mine", Field: "s"}
		}, "pick reads only records on the same side"},
		{"match field", func(a *App) { a.ServerRules[0].Pick.Exclude.Match.Field = "nope" }, `field "nope" is not a field of entity "offs"`},
		{"match equals", func(a *App) { a.ServerRules[0].Pick.Exclude.Match.Equals = "nope" }, `equals "nope" is not a field of entity "shifts"`},
		{"match kinds", func(a *App) { a.ServerRules[0].Pick.Exclude.Match.Equals = "staff" }, "are not comparable"},
		{"match hidden", func(a *App) { a.Entities[2].Fields[1].Hidden = true }, `field "day" is hidden`},
		{"match without row", func(a *App) {
			r := &a.ServerRules[1]
			r.On.Each = ""
			r.If = nil
			r.Pick.Exclude = &PickExclude{Entity: "offs", Field: "staff", Match: &PickMatch{Field: "day", Equals: "day"}}
			r.Do = []RuleAction{{Notify: &RuleNotify{To: RuleRecipient{Kind: RecipientMembers}, Title: "t", Body: "{$pick.name}"}}}
		}, "match: needs a record trigger or on.each"},
		{"four read entities", func(a *App) {
			a.Entities = append(a.Entities, Entity{ID: "logs", Name: "ログ", Scope: ScopeShared, Fields: []Field{{ID: "n", Name: "n", Type: FieldInteger}}})
			a.ServerRules[0].If = expr(t, `{"op":"gt","args":[{"agg":"count","entity":"logs"},0]}`)
		}, "reads the records of 4 entities (each + agg + lookup + similar + pick: logs, offs, shifts, staff); at most 3"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := readsApp(t, assignFile)
			c.edit(a)
			err := a.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, err)
			}
		})
	}
}

// Without count a pick still reads at most 2 entities.
func TestServerRulesAssignCapOnlyWithCount(t *testing.T) {
	a := readsApp(t, assignFile)
	r := &a.ServerRules[0]
	r.Pick.Order, r.Pick.Count = PickRandom, nil
	r.If = expr(t, `{"op":"gt","args":[{"agg":"count","entity":"shifts"},0]}`)
	if err := a.Validate(); err == nil || !strings.Contains(err.Error(), "at most 2") {
		t.Fatalf("got %v", err)
	}
}

// An addressed entity is never counted (validation and the run-time table).
func TestServerRulesAssignAddressed(t *testing.T) {
	a := readsApp(t, "server_rules_addressed.json")
	r := &ServerRule{ID: "r", On: RuleTrigger{Record: TriggerCreated, Entity: "notes"},
		Pick: &RulePick{Entity: "notes", Order: PickFewest, Count: &PickCount{Entity: "messages", Field: "x"}}}
	if got := a.AddressedRuleProblems(r); len(got) != 1 || !strings.Contains(got[0], "pick.count") {
		t.Fatalf("run-time: %v", got)
	}
}

// count and match are closed objects in the schema.
func TestServerRulesAssignSchema(t *testing.T) {
	raw := string(readsRaw(t, assignFile))
	from := `"count": {"entity": "shifts", "field": "staff"}, "exclude"`
	if !strings.Contains(raw, from) {
		t.Fatal("testdata changed")
	}
	for name, to := range map[string]string{
		"count key":     `"count": {"entity": "shifts", "field": "staff", "max": 2}, "exclude"`,
		"count missing": `"count": {"entity": "shifts"}, "exclude"`,
	} {
		if _, err := Parse([]byte(strings.Replace(raw, from, to, 1))); err == nil {
			t.Errorf("%s: Parse accepted it", name)
		}
	}
	if _, err := Parse([]byte(strings.Replace(raw, `"match": {"field": "day", "equals": "day"}`, `"match": {"field": "day"}`, 1))); err == nil {
		t.Error("match without equals: Parse accepted it")
	}
}
