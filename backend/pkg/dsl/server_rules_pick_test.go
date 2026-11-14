package dsl

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const pickFile = "server_rules_pick.json"

// pick parses, validates and round-trips.
func TestServerRulesPickRoundTrip(t *testing.T) {
	raw := readsRaw(t, pickFile)
	a := readsApp(t, pickFile)
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
}

func TestServerRulesPickDescribe(t *testing.T) {
	a := readsApp(t, pickFile)
	want := []string{
		"毎日 08:00、「メニュー」から(条件に合う・「食べた記録」に無いものを)ランダムに 1 つ選んで、「今日のおすすめ」に記録を足す・参加者全員に通知",
		"「食べた記録」が作られた時、「メニュー」から(「食べた記録」に無いものを)古い順に 1 つ選んで、その記録を書き換える・書いた人に通知",
	}
	for i := range a.ServerRules {
		if got := a.DescribeServerRule(&a.ServerRules[i]); got != want[i] {
			t.Errorf("rule %d: %s", i, got)
		}
	}
}

// The picked and the excluding entities are reads; the results are not
// fields of the trigger snapshot.
func TestServerRulesPickReads(t *testing.T) {
	a := readsApp(t, pickFile)
	for i, want := range [][]string{{"eats", "menus"}, {"eats", "menus"}} {
		if got := a.ServerRules[i].ReadEntities(); !slices.Equal(got, want) {
			t.Errorf("rule %d reads %v", i, got)
		}
	}
	if got := a.ServerRules[0].ReferencedFields(); len(got) != 0 {
		t.Errorf("daily fields %v", got)
	}
	a.ServerRules[1].Pick.Filter = []Filter{{Field: "price", Op: "lte", Value: map[string]any{"ref": "budget"}}}
	a.Entities[1].Fields = append(a.Entities[1].Fields, Field{ID: "budget", Name: "予算", Type: FieldInteger})
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := a.ServerRules[1].ReferencedFields(); !slices.Equal(got, []string{"budget"}) {
		t.Errorf("a filter value reads the row: %v", got)
	}
}

func TestServerRulesPickAccepts(t *testing.T) {
	cases := []struct {
		name string
		edit func(a *App)
	}{
		{"newest without filter", func(a *App) {
			a.ServerRules[0].Pick.Order, a.ServerRules[0].Pick.Filter, a.ServerRules[0].Pick.Exclude = PickNewest, nil, nil
		}},
		{"period filter", func(a *App) {
			a.Entities[0].Fields = append(a.Entities[0].Fields, Field{ID: "added", Name: "追加日", Type: FieldDate})
			a.ServerRules[0].Pick.Filter = append(a.ServerRules[0].Pick.Filter, Filter{Field: "added", Op: "thisMonth"})
		}},
		{"refs exclude", func(a *App) {
			a.Entities[2].Fields = append(a.Entities[2].Fields, Field{ID: "items", Name: "品", Type: FieldRefs, RefEntity: "menus"})
			a.ServerRules[0].Pick.Exclude = &PickExclude{Entity: "picks", Field: "items"}
		}},
		{"exclude own log", func(a *App) { a.ServerRules[0].Pick.Exclude = &PickExclude{Entity: "picks", Field: "menu"} }},
		{"authorOf the pick", func(a *App) {
			a.ServerRules[0].Do[1].Notify.To = RuleRecipient{Kind: RecipientAuthorOf, Field: PickRef}
		}},
		{"owner", func(a *App) { a.ServerRules[0].Do[1].Notify.To = RuleRecipient{Kind: RecipientOwner} }},
		{"number in with", func(a *App) {
			n := a.ServerRules[1].Do[1].Notify
			n.Body = "{yen} 円"
			n.With = map[string]Expr{"yen": *expr(t, `{"ref":"$pick.price"}`)}
		}},
		{"with lookup", func(a *App) {
			a.ServerRules[1].Lookup = &RuleLookup{Op: LookupExists, Entity: "menus", Field: LookupID, Equals: "menu"}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := readsApp(t, pickFile)
			c.edit(a)
			if err := a.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestServerRulesPickRejects(t *testing.T) {
	private := func(a *App) {
		a.Entities = append(a.Entities, Entity{ID: "notes", Name: "メモ", Fields: []Field{{ID: "menu", Name: "メニュー", Type: FieldRef, RefEntity: "menus"}}})
	}
	cases := []struct {
		name string
		edit func(a *App)
		want string
	}{
		{"unknown entity", func(a *App) { a.ServerRules[0].Pick.Entity = "ghost" }, `pick: entity "ghost" does not exist`},
		{"order", func(a *App) { a.ServerRules[0].Pick.Order = "best" }, `order "best" must be one of random|oldest|newest`},
		{"other side", func(a *App) {
			private(a)
			a.ServerRules[0].Pick.Exclude = &PickExclude{Entity: "notes", Field: "menu"}
		}, "pick reads only records on the same side"},
		{"single", func(a *App) { a.Entities[0].Single = true }, "is single"},
		{"too many filters", func(a *App) {
			f := a.ServerRules[0].Pick.Filter[0]
			a.ServerRules[0].Pick.Filter = []Filter{f, f, f, f}
		}, "filter has 4 conditions; at most 3"},
		{"hidden filter", func(a *App) { a.Entities[0].Fields[1].Hidden = true }, `field "category" is hidden`},
		{"computed filter", func(a *App) {
			a.Entities[0].Fields = append(a.Entities[0].Fields, Field{ID: "tax", Name: "税込", Type: FieldInteger, Computed: expr(t, `{"ref":"price"}`)})
			a.ServerRules[0].Pick.Filter = []Filter{{Field: "tax", Op: "gt", Value: 100.0}}
		}, "computed field"},
		{"my posts filter", func(a *App) {
			a.ServerRules[0].Pick.Filter = []Filter{{Field: RecordAuthor, Op: "eq", Value: map[string]any{"me": true}}}
		}, "not inside agg"},
		{"row ref without row", func(a *App) {
			a.ServerRules[0].Pick.Filter = []Filter{{Field: "price", Op: "lte", Value: map[string]any{"ref": "price"}}}
		}, `{"ref":"price"} is not allowed here`},
		{"exclude field", func(a *App) { a.ServerRules[0].Pick.Exclude.Field = "next2" }, `field "next2" is not a field of entity "eats"`},
		{"exclude not ref", func(a *App) {
			a.ServerRules[0].Pick.Exclude = &PickExclude{Entity: "picks", Field: "name"}
		}, `field "name" is string; exclude needs a ref / refs field`},
		{"exclude other entity", func(a *App) {
			a.Entities[1].Fields[0].RefEntity = "picks"
		}, `points to entity "picks", not to the picked entity "menus"`},
		{"exclude hidden", func(a *App) { a.Entities[1].Fields[0].Hidden = true }, `field "menu" is hidden`},
		{"with similar", func(a *App) {
			a.ServerRules[1].Similar = &RuleSimilar{Entity: "menus", Fields: []string{"menu"}}
		}, "similar or pick, not both"},
		{"result in if", func(a *App) {
			a.ServerRules[1].If = expr(t, `{"op":"neq","args":[{"ref":"$pick"},""]}`)
		}, `ref "$pick" is not a field`},
		{"result without pick", func(a *App) { a.ServerRules[1].Pick = nil }, `ref "$pick" is not a field`},
		{"result is not written", func(a *App) {
			a.ServerRules[1].Do[0].UpdateRecord.Values["$pick"] = *expr(t, `"x"`)
		}, `"$pick" is not a field of entity "eats"`},
		{"update the pick", func(a *App) { a.ServerRules[1].Do[0].UpdateRecord.Ref = PickRef }, `ref "$pick" is not a field of entity "eats"`},
		{"hidden of the pick", func(a *App) {
			a.Entities[0].Fields = append(a.Entities[0].Fields, Field{ID: "secret", Name: "秘密", Type: FieldString, Hidden: true})
			a.ServerRules[0].Do[0].CreateRecord.Values["name"] = *expr(t, `{"ref":"$pick.secret"}`)
		}, "reads a hidden field of another record"},
		{"hidden placeholder", func(a *App) {
			a.Entities[0].Fields = append(a.Entities[0].Fields, Field{ID: "secret", Name: "秘密", Type: FieldString, Hidden: true})
			a.ServerRules[0].Do[1].Notify.Body = "{$pick.secret}"
		}, "{$pick.secret} is hidden"},
		{"ref placeholder", func(a *App) { a.ServerRules[0].Do[1].Notify.Body = "{$pick}" }, "{$pick} is ref"},
		{"no row placeholder", func(a *App) { a.ServerRules[0].Do[1].Notify.Body = "{name}" }, "a time trigger has no record"},
		{"no row author placeholder", func(a *App) { a.ServerRules[0].Do[1].Notify.Body = "{$author}" }, "a time trigger has no record"},
		{"no row author", func(a *App) {
			a.ServerRules[0].Do[1].Notify.To = RuleRecipient{Kind: RecipientAuthor}
		}, "kind author needs a record trigger or on.each"},
		{"no row update", func(a *App) {
			a.ServerRules[0].Do[0] = RuleAction{UpdateRecord: &RuleUpdateRecord{Values: map[string]Expr{"name": *expr(t, `"x"`)}}}
		}, "updateRecord needs a record trigger"},
		{"private authorOf", func(a *App) {
			for i := range a.Entities {
				a.Entities[i].Scope = ""
			}
			a.ServerRules[0].Do[1].Notify.To = RuleRecipient{Kind: RecipientAuthorOf, Field: PickRef}
		}, `kind "authorOf" is not allowed on private data`},
		{"three read entities", func(a *App) {
			a.ServerRules[1].If = expr(t, `{"op":"gt","args":[{"agg":"count","entity":"picks"},0]}`)
		}, "reads the records of 3 entities (each + agg + lookup + similar + pick"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := readsApp(t, pickFile)
			c.edit(a)
			err := a.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, err)
			}
		})
	}
}

// An addressed entity is never picked nor read by exclude; the run-time
// table says so too.
func TestServerRulesPickAddressed(t *testing.T) {
	rule := `{"id": "r", "on": {"record": "created", "entity": "notes"},
		"pick": {"entity": "messages", "order": "random"},
		"do": [{"notify": {"to": {"kind": "owner"}, "title": "t", "body": "b"}}]}`
	_, err := Parse(addressedWith(t, `[`+rule+`]`))
	if err == nil || !strings.Contains(err.Error(), `pick: entity "messages" is addressed`) {
		t.Fatalf("got %v", err)
	}
	a := readsApp(t, "server_rules_addressed.json")
	r := &ServerRule{ID: "r", On: RuleTrigger{Record: TriggerCreated, Entity: "notes"}, Pick: &RulePick{Entity: "notes", Order: PickRandom, Exclude: &PickExclude{Entity: "messages", Field: "x"}}}
	if got := a.AddressedRuleProblems(r); len(got) != 1 || !strings.Contains(got[0], "pick.exclude") {
		t.Fatalf("run-time: %v", got)
	}
}

// pick is a closed object: unknown keys, a missing order and an unknown
// order fail the schema.
func TestServerRulesPickSchema(t *testing.T) {
	raw := string(readsRaw(t, pickFile))
	from := `"pick": {"entity": "menus", "order": "oldest", "exclude": {"entity": "eats", "field": "menu"}}`
	if !strings.Contains(raw, from) {
		t.Fatal("testdata changed")
	}
	for name, to := range map[string]string{
		"unknown key":     `"pick": {"entity": "menus", "order": "oldest", "count": 3}`,
		"no order":        `"pick": {"entity": "menus"}`,
		"unknown order":   `"pick": {"entity": "menus", "order": "best"}`,
		"exclude key":     `"pick": {"entity": "menus", "order": "oldest", "exclude": {"entity": "eats", "field": "menu", "by": "me"}}`,
		"exclude missing": `"pick": {"entity": "menus", "order": "oldest", "exclude": {"entity": "eats"}}`,
	} {
		if _, err := Parse([]byte(strings.Replace(raw, from, to, 1))); err == nil {
			t.Errorf("%s: Parse accepted it", name)
		}
	}
}
