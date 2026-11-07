package dsl

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const lookupFile = "server_rules_lookup.json"

// The lookup parses, validates and round-trips.
func TestServerRulesLookupRoundTrip(t *testing.T) {
	raw := readsRaw(t, lookupFile)
	a := readsApp(t, lookupFile)
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

func TestServerRulesLookupDescribe(t *testing.T) {
	a := readsApp(t, lookupFile)
	want := []string{
		"毎日 07:00、「履修登録」の記録ごとに、「名簿」の「学籍番号」が「学籍番号」と同じ記録が無い時、その記録を書き換える",
		"「履修登録」が作られた時、「名簿」の「学籍番号」が「学籍番号」と同じ記録が無い時、アプリの作成者に通知",
		"毎週日曜 10:00、「店」の記録ごとに、「訪問」の「店」が「この記録」と同じ記録が無い時、アプリの作成者に通知",
	}
	for i := range a.ServerRules {
		if got := a.DescribeServerRule(&a.ServerRules[i]); got != want[i] {
			t.Errorf("rule %d: %s", i, got)
		}
	}
}

// The lookup entity is a read (counted with each / agg) and the row field it
// compares is part of the trigger snapshot.
func TestServerRulesLookupReads(t *testing.T) {
	a := readsApp(t, lookupFile)
	if got := a.ServerRules[0].ReadEntities(); !slices.Equal(got, []string{"registrations", "roster"}) {
		t.Errorf("each + lookup reads %v", got)
	}
	if got := a.ServerRules[1].ReadEntities(); !slices.Equal(got, []string{"roster"}) {
		t.Errorf("record lookup reads %v", got)
	}
	if got := a.ServerRules[1].ReferencedFields(); !slices.Equal(got, []string{"student_id"}) {
		t.Errorf("record lookup fields %v", got)
	}
	if got := a.ServerRules[2].ReferencedFields(); !slices.Equal(got, []string{"name"}) {
		t.Errorf("$id is not a field: %v", got)
	}
}

func TestServerRulesLookupAccepts(t *testing.T) {
	cases := []struct {
		name string
		edit func(a *App)
	}{
		{"exists", func(a *App) { a.ServerRules[1].Lookup.Op = LookupExists }},
		{"lookup with if", func(a *App) {
			a.ServerRules[0].If = expr(t, `{"op":"neq","args":[{"ref":"mismatch"},true]}`)
		}},
		{"same entity (duplicates)", func(a *App) {
			a.ServerRules[1].Lookup = &RuleLookup{Op: LookupExists, Entity: "registrations", Field: "student_id", Equals: "student_id"}
		}},
		{"text with an integer", func(a *App) {
			a.Entities[0].Fields[0].Type = FieldInteger
		}},
		{"ref with ref", func(a *App) {
			a.Entities[2].Fields = append(a.Entities[2].Fields, Field{ID: "best", Name: "似た店", Type: FieldRef, RefEntity: "shops"})
			a.ServerRules[2].Lookup.Equals = "best"
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := readsApp(t, lookupFile)
			c.edit(a)
			if err := a.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestServerRulesLookupRejects(t *testing.T) {
	shared := func(a *App) {
		a.Entities = append(a.Entities, Entity{ID: "board", Name: "掲示", Scope: ScopeShared, Fields: []Field{{ID: "code", Name: "番号", Type: FieldString}}})
	}
	cases := []struct {
		name string
		edit func(a *App)
		want string
	}{
		{"bad op", func(a *App) { a.ServerRules[1].Lookup.Op = "in" }, `op "in" must be exists or notExists`},
		{"unknown entity", func(a *App) { a.ServerRules[1].Lookup.Entity = "ghost" }, `entity "ghost" does not exist`},
		{"unknown field", func(a *App) { a.ServerRules[1].Lookup.Field = "ghost" }, `field "ghost" is not a field of entity "roster"`},
		{"unknown equals", func(a *App) { a.ServerRules[1].Lookup.Equals = "ghost" }, `equals "ghost" is not a field of entity "registrations"`},
		{"no row", func(a *App) {
			a.ServerRules[0].On.Each = ""
			a.ServerRules[0].Do = []RuleAction{{Notify: &RuleNotify{To: RuleRecipient{Kind: RecipientOwner}, Title: "t", Body: "b"}}}
		}, "lookup: needs a record trigger or on.each"},
		{"other side", func(a *App) {
			shared(a)
			a.ServerRules[1].Lookup = &RuleLookup{Op: LookupExists, Entity: "board", Field: "code", Equals: "student_id"}
		}, "a lookup reads only records on the same side"},
		{"boolean field", func(a *App) { a.ServerRules[1].Lookup.Equals = "mismatch" }, "equals \"mismatch\" is boolean"},
		{"date with text", func(a *App) {
			a.ServerRules[2].Lookup = &RuleLookup{Op: LookupExists, Entity: "visits", Field: "date", Equals: "name"}
		}, "are not comparable"},
		{"id of another entity", func(a *App) {
			a.ServerRules[2].Lookup = &RuleLookup{Op: LookupExists, Entity: "roster", Field: "$id", Equals: "$id"}
		}, `holds ids of entity "roster" but equals "$id" holds ids of entity "shops"`},
		{"id with text", func(a *App) { a.ServerRules[2].Lookup.Equals = "name" }, "are not comparable"},
		{"computed", func(a *App) {
			a.Entities[0].Fields = append(a.Entities[0].Fields, Field{ID: "upper", Name: "大", Type: FieldString, Computed: expr(t, `{"ref":"student_id"}`)})
			a.ServerRules[1].Lookup.Field = "upper"
		}, "is computed on the device"},
		{"three read entities", func(a *App) {
			a.ServerRules[0].If = expr(t, `{"op":"gt","args":[{"agg":"count","entity":"visits"},0]}`)
		}, "reads the records of 3 entities (each + agg + lookup"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := readsApp(t, lookupFile)
			c.edit(a)
			err := a.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, err)
			}
		})
	}
}

// An addressed entity is never looked up (its records are read only by
// their author and recipients); the run-time table says so too.
func TestServerRulesLookupAddressed(t *testing.T) {
	rule := `{"id": "r", "on": {"record": "created", "entity": "notes"},
		"lookup": {"op": "exists", "entity": "messages", "field": "body", "equals": "text"},
		"do": [{"notify": {"to": {"kind": "owner"}, "title": "t", "body": "b"}}]}`
	_, err := Parse(addressedWith(t, `[`+rule+`]`))
	if err == nil || !strings.Contains(err.Error(), `lookup: entity "messages" is addressed`) {
		t.Fatalf("got %v", err)
	}
	a := readsApp(t, "server_rules_addressed.json")
	r := &ServerRule{ID: "r", On: RuleTrigger{Record: TriggerCreated, Entity: "notes"}, Lookup: &RuleLookup{Op: LookupExists, Entity: "messages", Field: "body", Equals: "text"}}
	if got := a.AddressedRuleProblems(r); len(got) != 1 || !strings.Contains(got[0], "lookup") {
		t.Fatalf("run-time: %v", got)
	}
}

// The lookup is a closed object: unknown keys and a missing key fail the schema.
func TestServerRulesLookupSchema(t *testing.T) {
	raw := string(readsRaw(t, lookupFile))
	from := `"lookup": {"op": "notExists", "entity": "roster",`
	if !strings.Contains(raw, from) {
		t.Fatal("testdata changed")
	}
	for name, to := range map[string]string{
		"unknown key": `"lookup": {"op": "notExists", "where": "x", "entity": "roster",`,
		"unknown op":  `"lookup": {"op": "contains", "entity": "roster",`,
	} {
		if _, err := Parse([]byte(strings.Replace(raw, from, to, 1))); err == nil {
			t.Errorf("%s: Parse accepted it", name)
		}
	}
	if _, err := Parse([]byte(strings.Replace(raw, `, "equals": "student_id"}`, `}`, 1))); err == nil {
		t.Error("Parse accepted a lookup without equals")
	}
}

func TestLookupValueKey(t *testing.T) {
	for _, c := range []struct {
		v    any
		id   bool
		want string
	}{
		{" Ａ１２３ ", false, "a123"},
		{"a123", false, "a123"},
		{float64(123), false, "123"},
		{int64(123), false, "123"},
		{"123", false, "123"},
		{"", false, ""},
		{"  ", false, ""},
		{nil, false, ""},
		{true, false, ""},
		{"AbC", true, "AbC"},
		{float64(1), true, ""},
	} {
		if got := LookupValueKey(c.v, c.id); got != c.want {
			t.Errorf("LookupValueKey(%#v, %v) = %q, want %q", c.v, c.id, got, c.want)
		}
	}
}
