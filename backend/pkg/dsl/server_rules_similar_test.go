package dsl

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const similarFile = "server_rules_similar.json"

// similar parses, validates and round-trips.
func TestServerRulesSimilarRoundTrip(t *testing.T) {
	raw := readsRaw(t, similarFile)
	a := readsApp(t, similarFile)
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

func TestServerRulesSimilarDescribe(t *testing.T) {
	a := readsApp(t, similarFile)
	want := []string{
		"「プロフィール」の「趣味」・「レベル」が変わった時、「プロフィール」で「趣味」・「レベル」が一番似ている記録を探して(2 つ以上同じ時)、その記録を書き換える・「似ているプロフィール」を作った人に通知",
		"毎日 08:00、「プロフィール」の記録ごとに、「プロフィール」で「趣味」が一番似ている記録を探して(1 つ以上同じ時)、その記録を書き換える",
		"「サークル」が作られた時、「プロフィール」で「活動」が一番似ている記録を探して(1 つ以上同じ時)、「似ているプロフィール」を作った人に通知",
	}
	for i := range a.ServerRules {
		if got := a.DescribeServerRule(&a.ServerRules[i]); got != want[i] {
			t.Errorf("rule %d: %s", i, got)
		}
	}
}

// The searched entity is a read; the compared row fields are part of the
// trigger snapshot and the results are not fields.
func TestServerRulesSimilarReads(t *testing.T) {
	a := readsApp(t, similarFile)
	if got := a.ServerRules[0].ReadEntities(); !slices.Equal(got, []string{"profiles"}) {
		t.Errorf("reads %v", got)
	}
	if got := a.ServerRules[0].ReferencedFields(); !slices.Equal(got, []string{"hobbies", "level"}) {
		t.Errorf("fields %v", got)
	}
	if got := a.ServerRules[2].ReadEntities(); !slices.Equal(got, []string{"profiles"}) {
		t.Errorf("scout reads %v", got)
	}
	if got := a.ServerRules[2].ReferencedFields(); !slices.Equal(got, []string{"hobbies", "name"}) {
		t.Errorf("scout fields %v", got)
	}
}

func TestServerRulesSimilarAccepts(t *testing.T) {
	cases := []struct {
		name string
		edit func(a *App)
	}{
		{"with if", func(a *App) { a.ServerRules[0].If = expr(t, `{"op":"neq","args":[{"ref":"name"},""]}`) }},
		{"score in with", func(a *App) {
			n := a.ServerRules[0].Do[1].Notify
			n.Body = "共通点 {n} 個"
			n.With = map[string]Expr{"n": *expr(t, `{"ref":"$similarScore"}`)}
		}},
		{"increment by score", func(a *App) {
			a.Entities[0].Fields = append(a.Entities[0].Fields, Field{ID: "team", Name: "チーム", Type: FieldRef, RefEntity: "circles"})
			a.Entities[1].Fields = append(a.Entities[1].Fields, Field{ID: "points", Name: "点", Type: FieldInteger})
			a.ServerRules[0].Do = append(a.ServerRules[0].Do, RuleAction{Increment: &RuleIncrement{Ref: "team", Field: "points", By: expr(t, `{"ref":"$similarScore"}`)}})
		}},
		{"refs field", func(a *App) {
			for i := range a.Entities {
				a.Entities[i].Fields = append(a.Entities[i].Fields, Field{ID: "tags", Name: "タグ", Type: FieldRefs, RefEntity: "circles"})
			}
			a.ServerRules[2].Similar.Fields = []string{"tags"}
		}},
		{"min max", func(a *App) { a.ServerRules[0].Similar.Min = MaxSimilarMin }},
		{"with lookup", func(a *App) {
			a.ServerRules[0].Lookup = &RuleLookup{Op: LookupNotExists, Entity: "circles", Field: "name", Equals: "name"}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := readsApp(t, similarFile)
			c.edit(a)
			if err := a.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestServerRulesSimilarRejects(t *testing.T) {
	private := func(a *App) {
		a.Entities = append(a.Entities, Entity{ID: "notes", Name: "メモ", Fields: []Field{{ID: "hobbies", Name: "趣味", Type: FieldString}}})
	}
	cases := []struct {
		name string
		edit func(a *App)
		want string
	}{
		{"unknown entity", func(a *App) { a.ServerRules[0].Similar.Entity = "ghost" }, `similar: entity "ghost" does not exist`},
		{"no fields", func(a *App) { a.ServerRules[0].Similar.Fields = nil }, "fields needs 1-5 fields (got 0)"},
		{"too many fields", func(a *App) {
			a.ServerRules[0].Similar.Fields = []string{"name", "hobbies", "level", "best_name", "match_score", "best_match"}
		}, "fields needs 1-5 fields (got 6)"},
		{"twice", func(a *App) { a.ServerRules[0].Similar.Fields = []string{"hobbies", "hobbies"} }, `field "hobbies" is listed twice`},
		{"not in row", func(a *App) { a.ServerRules[2].Similar.Fields = []string{"level"} }, `field "level" is not a field of entity "circles"`},
		{"not in target", func(a *App) { a.ServerRules[0].Similar.Fields = []string{"ghost"} }, `field "ghost" is not a field of entity "profiles"`},
		{"min", func(a *App) { a.ServerRules[0].Similar.Min = 21 }, "min 21 must be 1-20"},
		{"no row", func(a *App) {
			a.ServerRules[1].On.Each = ""
			a.ServerRules[1].Do = []RuleAction{{Notify: &RuleNotify{To: RuleRecipient{Kind: RecipientOwner}, Title: "t", Body: "b"}}}
		}, "similar: needs a record trigger or on.each"},
		{"other side", func(a *App) {
			private(a)
			a.ServerRules[2].Similar.Entity = "notes"
		}, "similar reads only records on the same side"},
		{"hidden", func(a *App) { a.Entities[0].Fields[1].Hidden = true }, `field "hobbies" is hidden`},
		{"computed", func(a *App) {
			a.Entities[0].Fields[1].Computed = expr(t, `{"ref":"name"}`)
		}, "is computed on the device"},
		{"boolean", func(a *App) {
			for i := range a.Entities {
				a.Entities[i].Fields = append(a.Entities[i].Fields, Field{ID: "ok", Name: "可", Type: FieldBoolean})
			}
			a.ServerRules[2].Similar.Fields = []string{"ok"}
		}, `field "ok" is boolean`},
		{"kinds differ", func(a *App) {
			a.Entities[1].Fields[1].Type = FieldDate // circles.hobbies
		}, `field "hobbies" is date in "circles" but string in "profiles"`},
		{"result in if", func(a *App) {
			a.ServerRules[0].If = expr(t, `{"op":"gt","args":[{"ref":"$similarScore"},1]}`)
		}, `ref "$similarScore" is not a field`},
		{"result without similar", func(a *App) { a.ServerRules[1].Similar = nil }, `ref "$similar" is not a field`},
		{"result is not written", func(a *App) {
			a.ServerRules[1].Do[0].UpdateRecord.Values["$similar"] = *expr(t, `"x"`)
		}, `"$similar" is not a field of entity "profiles"`},
		{"hidden of the match", func(a *App) {
			a.Entities[0].Fields = append(a.Entities[0].Fields, Field{ID: "secret", Name: "秘密", Type: FieldString, Hidden: true})
			a.ServerRules[1].Do[0].UpdateRecord.Values["best_name"] = *expr(t, `{"ref":"$similar.secret"}`)
		}, "reads a hidden field of another record"},
		{"ref placeholder", func(a *App) { a.ServerRules[2].Do[0].Notify.Body = "{$similar}" }, "{$similar} is ref"},
		{"three read entities", func(a *App) {
			a.ServerRules[1].If = expr(t, `{"op":"gt","args":[{"agg":"count","entity":"circles"},0]}`)
			private(a)
			a.Entities[2].Scope = ScopeShared
			a.ServerRules[1].Lookup = &RuleLookup{Op: LookupExists, Entity: "notes", Field: "hobbies", Equals: "hobbies"}
		}, "reads the records of 3 entities (each + agg + lookup + similar"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := readsApp(t, similarFile)
			c.edit(a)
			err := a.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, err)
			}
		})
	}
}

// An addressed entity is never compared; the run-time table says so too.
func TestServerRulesSimilarAddressed(t *testing.T) {
	rule := `{"id": "r", "on": {"record": "created", "entity": "notes"},
		"similar": {"entity": "messages", "fields": ["body"]},
		"do": [{"notify": {"to": {"kind": "owner"}, "title": "t", "body": "b"}}]}`
	_, err := Parse(addressedWith(t, `[`+rule+`]`))
	if err == nil || !strings.Contains(err.Error(), `similar: entity "messages" is addressed`) {
		t.Fatalf("got %v", err)
	}
	a := readsApp(t, "server_rules_addressed.json")
	r := &ServerRule{ID: "r", On: RuleTrigger{Record: TriggerCreated, Entity: "notes"}, Similar: &RuleSimilar{Entity: "messages", Fields: []string{"body"}}}
	if got := a.AddressedRuleProblems(r); len(got) != 1 || !strings.Contains(got[0], "similar") {
		t.Fatalf("run-time: %v", got)
	}
}

// similar is a closed object: unknown keys, a missing key and min 0 fail the schema.
func TestServerRulesSimilarSchema(t *testing.T) {
	raw := string(readsRaw(t, similarFile))
	from := `"similar": {"entity": "profiles", "fields": ["hobbies", "level"], "min": 2}`
	if !strings.Contains(raw, from) {
		t.Fatal("testdata changed")
	}
	for name, to := range map[string]string{
		"unknown key": `"similar": {"entity": "profiles", "fields": ["hobbies"], "weights": [1]}`,
		"no fields":   `"similar": {"entity": "profiles"}`,
		"min 0":       `"similar": {"entity": "profiles", "fields": ["hobbies"], "min": 0}`,
		"6 fields":    `"similar": {"entity": "profiles", "fields": ["a", "b", "c", "d", "e", "f"]}`,
	} {
		if _, err := Parse([]byte(strings.Replace(raw, from, to, 1))); err == nil {
			t.Errorf("%s: Parse accepted it", name)
		}
	}
}

func TestSimilarItems(t *testing.T) {
	str := &Field{Type: FieldString}
	for _, c := range []struct {
		f    *Field
		v    any
		want []string
	}{
		{str, "映画、音楽, 旅行 / Ｃａｆｅ・映画", []string{"cafe", "旅行", "映画", "音楽"}},
		{str, " ", nil},
		{str, nil, nil},
		{&Field{Type: FieldEnum}, "Expert", []string{"expert"}},
		{&Field{Type: FieldInteger}, float64(3), []string{"3"}},
		{&Field{Type: FieldRef}, "AbC", []string{"AbC"}},
		{&Field{Type: FieldRefs}, []any{"a", "b", "a", 1.0, ""}, []string{"a", "b"}},
	} {
		got := SimilarItems(c.f, c.v)
		keys := sortedKeys(got)
		if len(c.want) == 0 && got != nil || !slices.Equal(keys, c.want) {
			t.Errorf("SimilarItems(%s, %#v) = %v, want %v", c.f.Type, c.v, keys, c.want)
		}
	}
	many := ""
	for i := range 30 {
		many += string(rune('a'+i%26)) + string(rune('a'+i/26)) + ","
	}
	if n := len(SimilarItems(str, many)); n != MaxSimilarItems {
		t.Errorf("items capped at %d, got %d", MaxSimilarItems, n)
	}
}
