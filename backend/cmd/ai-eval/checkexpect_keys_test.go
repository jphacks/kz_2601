package main

import "testing"

// TestCheckExpect014Keys: every key of checkExpect, once met and once missed.
func TestCheckExpect014Keys(t *testing.T) {
	type side struct {
		m       Metrics
		summary string
	}
	cases := []struct {
		key  string
		v    any
		pass side
		fail side
	}{
		{"shared", true, side{m: Metrics{Scopes: []string{"a:shared"}}}, side{m: Metrics{Scopes: []string{"a:private"}}}},
		{"shared", false, side{m: Metrics{Scopes: []string{"a:private"}}}, side{m: Metrics{Scopes: []string{"a:public"}}}},
		{"substitution", true, side{summary: "地図は代替のリストにしました"}, side{summary: "作りました"}},
		{"substitution", true, side{summary: "Push was replaced by a list"}, side{summary: "done"}},
		{"substitution", true, side{summary: "That isn't possible here"}, side{summary: "done"}},
		{"substitution", false, side{summary: "作りました"}, side{summary: "それはできません"}},
		{"health", "steps", side{m: Metrics{Health: []string{"steps:day"}}}, side{m: Metrics{Health: []string{"sleep:day"}}}},
		{"healthField", true, side{m: Metrics{HealthField: true}}, side{}},
		{"grid", true, side{m: Metrics{Grid: []string{"home"}}}, side{}},
		{"grid", false, side{}, side{m: Metrics{Grid: []string{"home"}}}},
		{"oneAdd", true, side{}, side{m: Metrics{DupAdd: []string{"home"}}}},
		{"prefillRef", true, side{m: Metrics{PrefillRefs: []string{"x"}}}, side{}},
		{"calendar", true, side{m: Metrics{HasCalendar: true}}, side{}},
		{"photo", true, side{m: Metrics{HasPhoto: true}}, side{}},
		{"chart", true, side{m: Metrics{HasChart: true}}, side{}},
		{"notification", "daily", side{m: Metrics{Notifications: []string{"x@daily"}}}, side{m: Metrics{Notifications: []string{"x@weekly"}}}},
		{"notification", "any", side{m: Metrics{Notifications: []string{"x@weekly"}}}, side{}},
		{"calc", true, side{m: Metrics{Settlement: true}}, side{}},
		{"calc", true, side{m: Metrics{ProgressTo: true}}, side{}},
		{"calc", false, side{}, side{m: Metrics{StatValues: 1}}},
		{"computed", true, side{m: Metrics{Computed: []string{"e.f"}}}, side{}},
		{"statValue", true, side{m: Metrics{StatValues: 2}}, side{}},
		{"settlement", true, side{m: Metrics{Settlement: true}}, side{}},
		{"ref", true, side{m: Metrics{RefFields: 1}}, side{}},
		{"noShared", true, side{m: Metrics{Scopes: []string{"a:private"}}}, side{m: Metrics{Scopes: []string{"a:shared", "b:public"}}}},
		{"noShared", false, side{m: Metrics{Scopes: []string{"a:shared"}}}, side{m: Metrics{Scopes: []string{"a:shared"}}}},
		{"editMembers", true, side{m: Metrics{EditMembers: []string{"a"}}}, side{}},
		{"authorShown", true, side{m: Metrics{AuthorShown: []string{"s:list"}}}, side{}},
		{"nameInput", false, side{}, side{m: Metrics{NameInputs: []string{"a.name"}}}},
		{"meFilter", true, side{m: Metrics{MeFilter: []string{"s:list"}}}, side{}},
		{"scope", "family", side{m: Metrics{Scopes: []string{"a:family"}}}, side{m: Metrics{Scopes: []string{"a:public"}}}},
		{"scopeNot", "public", side{m: Metrics{Scopes: []string{"a:family"}}}, side{m: Metrics{Scopes: []string{"a:public", "b:public"}}}},
		{"summaryHas", []any{"買い物", "共有"}, side{summary: "買い物を共有します"}, side{summary: "買い物です"}},
		{"summaryNot", []any{"代替"}, side{summary: "通知します"}, side{summary: "代替します"}},
		{"unknownKey", true, side{}, side{}},
	}
	for _, tc := range cases {
		exp := map[string]any{tc.key: tc.v}
		if miss := checkExpect(exp, &tc.pass.m, tc.pass.summary); len(miss) != 0 {
			t.Errorf("%s=%v pass side: miss = %v", tc.key, tc.v, miss)
		}
		miss := checkExpect(exp, &tc.fail.m, tc.fail.summary)
		// noShared=false never misses; an unknown key is ignored.
		wantMiss := (tc.key != "noShared" || tc.v != false) && tc.key != "unknownKey"
		if (len(miss) == 1) != wantMiss {
			t.Errorf("%s=%v fail side: miss = %v, want one miss: %v", tc.key, tc.v, miss, wantMiss)
		}
	}
}

// TestCheckExpect014SortedMisses: several misses come back sorted.
func TestCheckExpect014SortedMisses(t *testing.T) {
	exp := map[string]any{"photo": true, "chart": true, "calendar": true}
	miss := checkExpect(exp, &Metrics{}, "")
	if len(miss) != 3 || miss[0] > miss[1] || miss[1] > miss[2] {
		t.Fatalf("miss = %v, want 3 sorted", miss)
	}
}
