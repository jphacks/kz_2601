package dsl

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func readsRaw(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func readsApp(t *testing.T, name string) *App {
	t.Helper()
	a, err := Parse(readsRaw(t, name))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// The read vocabulary (agg / on.each / notify.with) parses, validates and
// round-trips without losing a key.
func TestServerRulesReadsRoundTrip(t *testing.T) {
	for _, name := range []string{"server_rules_reads.json", "server_rules_ranking.json"} {
		t.Run(name, func(t *testing.T) {
			raw := readsRaw(t, name)
			a := readsApp(t, name)
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
		})
	}
}

func TestServerRulesReadsDescribe(t *testing.T) {
	a := readsApp(t, "server_rules_reads.json")
	want := []string{
		"「支出」が作られた時(条件に合えば)、「支出」を集計してアプリの作成者に通知",
		"毎週日曜 20:00、「支出」を集計して「週のまとめ」に記録を足す・アプリの作成者に通知",
		"毎日 08:00、「やること」の記録ごとに(条件に合えば)、アプリの作成者に通知",
	}
	for i := range a.ServerRules {
		if got := a.DescribeServerRule(&a.ServerRules[i]); got != want[i] {
			t.Errorf("rule %d: %s", i, got)
		}
	}
	r := readsApp(t, "server_rules_ranking.json")
	if got := r.DescribeServerRule(&r.ServerRules[0]); got != "毎日 21:00、「参加者」の記録ごとに、「参加者」を集計してその記録を書き換える・書いた人に通知" {
		t.Errorf("ranking: %s", got)
	}
}

func TestServerRulesReadEntitiesAndFields(t *testing.T) {
	a := readsApp(t, "server_rules_reads.json")
	if got := a.ServerRules[1].ReadEntities(); !slices.Equal(got, []string{"expenses"}) {
		t.Errorf("weekly reads %v", got)
	}
	if got := a.ServerRules[2].ReadEntities(); !slices.Equal(got, []string{"tasks"}) {
		t.Errorf("each reads %v", got)
	}
	if got := a.ServerRules[2].AggEntities(); len(got) != 0 {
		t.Errorf("each aggs %v", got)
	}
	// a ref inside an agg filter value reads the trigger record (outbox snapshot)
	r := readsApp(t, "server_rules_ranking.json")
	if got := r.ServerRules[0].ReferencedFields(); !slices.Equal(got, []string{"name", "score"}) {
		t.Errorf("ranking fields %v", got)
	}
	// with names are not fields of the record
	if got := a.ServerRules[0].ReferencedFields(); len(got) != 0 {
		t.Errorf("over_budget fields %v", got)
	}
}

func TestServerRulesReadsAccepts(t *testing.T) {
	cases := []struct {
		name string
		edit func(a *App)
	}{
		{"agg in a value of a record trigger", func(a *App) {
			a.ServerRules[0].Do = append(a.ServerRules[0].Do, RuleAction{UpdateRecord: &RuleUpdateRecord{Values: map[string]Expr{
				"memo": *expr(t, `{"op":"concat","args":["今月 ",{"op":"toString","args":[{"agg":"count","entity":"expenses"}]}]}`)}}})
		}},
		{"each with a date with", func(a *App) {
			a.ServerRules[2].Do[0].Notify.With = map[string]Expr{"first": *expr(t, `{"agg":"min","entity":"tasks","field":"due"}`)}
			a.ServerRules[2].Do[0].Notify.Body = "{title}: 最初の締め切りは {first}"
		}},
		{"each updates its record", func(a *App) {
			a.ServerRules[2].Do = append(a.ServerRules[2].Do, RuleAction{UpdateRecord: &RuleUpdateRecord{Values: map[string]Expr{"done": *expr(t, `false`)}}})
		}},
		{"monthly each", func(a *App) {
			a.ServerRules[2].On = RuleTrigger{Every: "monthly", Day: 1, Time: "09:00", Each: "tasks"}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := readsApp(t, "server_rules_reads.json")
			c.edit(a)
			if err := a.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestServerRulesReadsRejects(t *testing.T) {
	cases := []struct {
		name string
		edit func(a *App)
		want string
	}{
		{"each on a record trigger", func(a *App) { a.ServerRules[0].On.Each = "tasks" }, "each are only for time triggers"},
		{"each unknown", func(a *App) { a.ServerRules[2].On.Each = "ghost" }, `each "ghost" does not exist`},
		{"each other side", func(a *App) {
			a.Entities = append(a.Entities, Entity{ID: "board", Name: "掲示", Scope: ScopeShared, Fields: []Field{{ID: "n", Name: "数", Type: FieldInteger}}})
			a.ServerRules = []ServerRule{a.ServerRules[2]}
		}, "each needs an entity on the same side"},
		{"agg other side", func(a *App) {
			a.Entities = append(a.Entities, Entity{ID: "board", Name: "掲示", Scope: ScopeShared, Fields: []Field{{ID: "n", Name: "数", Type: FieldInteger}}})
			a.ServerRules[0].If = expr(t, `{"op":"gt","args":[{"agg":"count","entity":"board"},3]}`)
		}, "agg reads only records on the same side"},
		{"three read entities", func(a *App) {
			a.ServerRules[2].If = expr(t, `{"op":"gt","args":[{"op":"add","args":[{"agg":"count","entity":"expenses"},{"agg":"count","entity":"summaries"}]},0]}`)
		}, "reads the records of 3 entities"},
		{"agg over a computed field", func(a *App) {
			a.Entities[0].Fields = append(a.Entities[0].Fields, Field{ID: "tax", Name: "税", Type: FieldNumber, Computed: expr(t, `{"op":"mul","args":[{"ref":"amount"},0.1]}`)})
			a.ServerRules[0].If = expr(t, `{"op":"gt","args":[{"agg":"sum","entity":"expenses","field":"tax"},10]}`)
		}, "calculated on the device"},
		{"with text", func(a *App) {
			a.ServerRules[1].Do[1].Notify.With["count"] = *expr(t, `{"agg":"latest","entity":"expenses","field":"memo"}`)
		}, "must be a number or a date"},
		{"with unused", func(a *App) {
			a.ServerRules[1].Do[1].Notify.Body = "今週は {total} 円でした"
		}, "{count} is not used"},
		{"with too many", func(a *App) {
			n := a.ServerRules[1].Do[1].Notify
			n.With["a"] = *expr(t, `1`)
			n.With["b"] = *expr(t, `2`)
			n.Body = "{count}{total}{a}{b}"
		}, "too many values (4 > 3)"},
		{"with shadows a field", func(a *App) {
			n := a.ServerRules[2].Do[0].Notify
			n.With = map[string]Expr{"title": *expr(t, `1`)}
		}, "is a field of entity"},
		{"with bad name", func(a *App) {
			n := a.ServerRules[1].Do[1].Notify
			n.With["Bad"] = *expr(t, `1`)
			n.Body += "{Bad}"
		}, "is not a valid name"},
		{"placeholder without each or with", func(a *App) { a.ServerRules[1].Do[1].Notify.Body = "{memo}" }, "time trigger has no record"},
		{"update without each", func(a *App) {
			a.ServerRules[1].Do = append(a.ServerRules[1].Do, RuleAction{UpdateRecord: &RuleUpdateRecord{Values: map[string]Expr{"total": *expr(t, `1`)}}})
		}, "needs a record trigger or on.each"},
		{"each chain", func(a *App) {
			a.ServerRules = append(a.ServerRules, ServerRule{ID: "on_task", On: RuleTrigger{Record: TriggerUpdated, Entity: "tasks", Fields: []string{"done"}},
				Do: []RuleAction{{Notify: &RuleNotify{To: RuleRecipient{Kind: RecipientOwner}, Title: "済み", Body: "{title} が済みました"}}}})
			a.ServerRules[2].Do = append(a.ServerRules[2].Do, RuleAction{UpdateRecord: &RuleUpdateRecord{Values: map[string]Expr{"done": *expr(t, `false`)}}})
		}, "no chains"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := readsApp(t, "server_rules_reads.json")
			c.edit(a)
			err := a.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, err)
			}
		})
	}
}

// with is not a JSON open door: more than 3 names fail the closed schema.
func TestServerRulesReadsSchema(t *testing.T) {
	raw := string(readsRaw(t, "server_rules_reads.json"))
	from := `"with": {"total":`
	if !strings.Contains(raw, from) {
		t.Fatal("testdata changed")
	}
	bad := strings.Replace(raw, from, `"with": {"a": 1, "b": 2, "c": 3, "total":`, 1)
	if _, err := Parse([]byte(bad)); err == nil {
		t.Fatal("Parse accepted 4 with values")
	}
	if _, err := Parse([]byte(strings.Replace(raw, `"each": "tasks"`, `"each": "tasks", "eachWhere": 1`, 1))); err == nil {
		t.Fatal("Parse accepted an unknown trigger key")
	}
}
