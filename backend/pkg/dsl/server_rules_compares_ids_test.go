package dsl

import "testing"

// comparesIDsApp: 「訪問」(店への参照つき)と「店」。
// lookup / pick の判定は「探す側の項目」だけを見るので、両方の entity を 1 つの App に置く。
func comparesIDsApp() *App {
	return &App{
		Entities: []Entity{
			{ID: "visits", Name: "訪問", Fields: []Field{
				{ID: "shop", Name: "店", Type: FieldRef, RefEntity: "shops"},
				{ID: "shops", Name: "店ども", Type: FieldRefs, RefEntity: "shops"},
				{ID: "memo", Name: "メモ", Type: FieldString},
				{ID: "day", Name: "日付", Type: FieldDate},
				{ID: "count", Name: "回数", Type: FieldInteger},
				{ID: "photo", Name: "写真", Type: FieldPhoto},
			}},
			{ID: "shops", Name: "店", Fields: []Field{
				{ID: "name", Name: "名前", Type: FieldString},
			}},
		},
	}
}

// LookupComparesIDs: 記録の id を比べる(= 値の正規化をしない)かどうか。
// $id と ref の項目だけ true。
func TestLookupComparesIDs(t *testing.T) {
	a := comparesIDsApp()
	cases := []struct {
		name   string
		lookup RuleLookup
		want   bool
	}{
		{"$id どうし", RuleLookup{Op: LookupNotExists, Entity: "visits", Field: LookupID, Equals: LookupID}, true},
		{"$id と文字の項目", RuleLookup{Op: LookupExists, Entity: "visits", Field: LookupID, Equals: "name"}, true},
		{"参照の項目", RuleLookup{Op: LookupNotExists, Entity: "visits", Field: "shop", Equals: LookupID}, true},
		{"文字の項目", RuleLookup{Op: LookupExists, Entity: "visits", Field: "memo", Equals: "name"}, false},
		{"整数の項目", RuleLookup{Op: LookupExists, Entity: "visits", Field: "count", Equals: "name"}, false},
		{"日付の項目", RuleLookup{Op: LookupExists, Entity: "visits", Field: "day", Equals: "name"}, false},
		{"比べられない型の項目", RuleLookup{Op: LookupExists, Entity: "visits", Field: "photo", Equals: "name"}, false},
		{"複数の参照の項目(1 件を比べる口ではない)", RuleLookup{Op: LookupExists, Entity: "visits", Field: "shops", Equals: LookupID}, false},
		{"無い項目", RuleLookup{Op: LookupExists, Entity: "visits", Field: "nope", Equals: "name"}, false},
		{"無い entity", RuleLookup{Op: LookupExists, Entity: "nope", Field: LookupID, Equals: LookupID}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := c.lookup
			if got := a.LookupComparesIDs(&l); got != c.want {
				t.Errorf("LookupComparesIDs(%+v) = %v, want %v", l, got, c.want)
			}
		})
	}
}

// 「探す側の項目」だけで決まる。相手(equals)が何であっても変わらない。
func TestLookupComparesIDsIgnoresEqualsSide(t *testing.T) {
	a := comparesIDsApp()
	for _, equals := range []string{LookupID, "name", "nope", ""} {
		l := RuleLookup{Op: LookupExists, Entity: "visits", Field: "shop", Equals: equals}
		if !a.LookupComparesIDs(&l) {
			t.Errorf("equals=%q で false になった", equals)
		}
		l.Field = "memo"
		if a.LookupComparesIDs(&l) {
			t.Errorf("equals=%q で true になった", equals)
		}
	}
}

// PickMatchComparesIDs: pick の exclude.match が記録の id を比べるかどうか。
func TestPickMatchComparesIDs(t *testing.T) {
	a := comparesIDsApp()
	cases := []struct {
		name    string
		exclude PickExclude
		want    bool
	}{
		{"$id どうし", PickExclude{Entity: "visits", Field: "shop", Match: &PickMatch{Field: LookupID, Equals: LookupID}}, true},
		{"$id と文字の項目", PickExclude{Entity: "visits", Field: "shop", Match: &PickMatch{Field: LookupID, Equals: "name"}}, true},
		{"参照の項目", PickExclude{Entity: "visits", Field: "shop", Match: &PickMatch{Field: "shop", Equals: LookupID}}, true},
		{"文字の項目", PickExclude{Entity: "visits", Field: "shop", Match: &PickMatch{Field: "memo", Equals: "name"}}, false},
		{"日付の項目", PickExclude{Entity: "visits", Field: "shop", Match: &PickMatch{Field: "day", Equals: "day"}}, false},
		{"複数の参照の項目", PickExclude{Entity: "visits", Field: "shop", Match: &PickMatch{Field: "shops", Equals: LookupID}}, false},
		{"無い項目", PickExclude{Entity: "visits", Field: "shop", Match: &PickMatch{Field: "nope", Equals: "name"}}, false},
		{"match が無い", PickExclude{Entity: "visits", Field: "shop"}, false},
		{"無い entity", PickExclude{Entity: "nope", Field: "shop", Match: &PickMatch{Field: LookupID, Equals: LookupID}}, false},
		{"無い entity で match も無い", PickExclude{Entity: "nope", Field: "shop"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			x := c.exclude
			if got := a.PickMatchComparesIDs(&x); got != c.want {
				t.Errorf("PickMatchComparesIDs(%+v) = %v, want %v", x, got, c.want)
			}
		})
	}
}

// lookup と pick の判定は同じ土台(項目の種類)なので、同じ項目なら同じ答えになる。
func TestComparesIDsAgreeBetweenLookupAndPick(t *testing.T) {
	a := comparesIDsApp()
	for _, field := range []string{LookupID, "shop", "shops", "memo", "day", "count", "photo", "nope"} {
		l := RuleLookup{Op: LookupExists, Entity: "visits", Field: field, Equals: LookupID}
		x := PickExclude{Entity: "visits", Field: "shop", Match: &PickMatch{Field: field, Equals: LookupID}}
		if got, want := a.PickMatchComparesIDs(&x), a.LookupComparesIDs(&l); got != want {
			t.Errorf("field %q: pick %v, lookup %v", field, got, want)
		}
	}
}
