package dsl

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const overlapFile = "server_rules_overlap.json"

func TestServerRulesOverlapRoundTrip(t *testing.T) {
	raw := readsRaw(t, overlapFile)
	a := readsApp(t, overlapFile)
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

func TestServerRulesOverlapDescribe(t *testing.T) {
	a := readsApp(t, overlapFile)
	want := []string{
		"「予約」が作られた時、「予約」の「開始」〜「終了」が「開始」〜「終了」と重なる記録がある時、アプリの作成者に通知",
		"毎日 06:00、「予約」の記録ごとに(条件に合えば)、「予約」の「開始」〜「終了」が「開始」〜「終了」と重なる記録がある時、その記録を書き換える",
		"毎日 07:00、「作業」の記録ごとに、「作業できない日」の「日付」が「開始日」〜「締切」と重なる記録がある時、アプリの作成者に通知",
	}
	for i := range a.ServerRules {
		if got := a.DescribeServerRule(&a.ServerRules[i]); got != want[i] {
			t.Errorf("rule %d: %s", i, got)
		}
	}
}

// The overlap entity is a read and both ends of the row's period are part of
// the trigger snapshot.
func TestServerRulesOverlapReads(t *testing.T) {
	a := readsApp(t, overlapFile)
	if got := a.ServerRules[0].ReadEntities(); !slices.Equal(got, []string{"bookings"}) {
		t.Errorf("reads %v", got)
	}
	if got := a.ServerRules[0].ReferencedFields(); !slices.Equal(got, []string{"end", "start", "title"}) {
		t.Errorf("fields %v", got)
	}
	if got := a.ServerRules[2].ReadEntities(); !slices.Equal(got, []string{"days_off", "tasks"}) {
		t.Errorf("each + overlap reads %v", got)
	}
}

func TestServerRulesOverlapAccepts(t *testing.T) {
	cases := []struct {
		name string
		edit func(a *App)
	}{
		{"notOverlaps", func(a *App) { a.ServerRules[0].Lookup.Op = LookupNotOverlaps }},
		{"no ends", func(a *App) {
			a.ServerRules[0].Lookup.FieldEnd, a.ServerRules[0].Lookup.EqualsEnd = "", ""
		}},
		{"date with datetime", func(a *App) {
			a.ServerRules[0].Lookup = &RuleLookup{Op: LookupOverlaps, Entity: "days_off", Field: "day", Equals: "start", EqualsEnd: "end"}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := readsApp(t, overlapFile)
			c.edit(a)
			if err := a.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestServerRulesOverlapRejects(t *testing.T) {
	cases := []struct {
		name string
		edit func(l *RuleLookup)
		want string
	}{
		{"text field", func(l *RuleLookup) { l.Field = "title" }, `field "title" is string; an overlap compares date / datetime fields`},
		{"id", func(l *RuleLookup) { l.Equals = LookupID }, "equals is $id"},
		{"unknown end", func(l *RuleLookup) { l.FieldEnd = "ghost" }, `fieldEnd "ghost" is not a field of entity "bookings"`},
		{"end is start", func(l *RuleLookup) { l.EqualsEnd = "start" }, `equalsEnd "start" is also the start`},
		{"mixed types", func(l *RuleLookup) {
			l.Entity, l.Field, l.FieldEnd = "tasks", "from", "due"
			l.EqualsEnd = "clash"
		}, `equalsEnd "clash" is boolean`},
		{"ends on equality", func(l *RuleLookup) {
			l.Op = LookupExists
		}, "fieldEnd / equalsEnd are only for op overlaps / notOverlaps"},
		{"unknown entity", func(l *RuleLookup) { l.Entity = "ghost" }, `entity "ghost" does not exist`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := readsApp(t, overlapFile)
			c.edit(a.ServerRules[0].Lookup)
			err := a.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, err)
			}
		})
	}
	a := readsApp(t, overlapFile)
	a.Entities[0].Fields[2].Type = FieldDate // end: date, start: datetime
	if err := a.Validate(); err == nil || !strings.Contains(err.Error(), "the start and end of a period have the same type") {
		t.Fatalf("mixed start / end: %v", err)
	}
	a = readsApp(t, overlapFile)
	a.ServerRules[2].On.Each = ""
	if err := a.Validate(); err == nil || !strings.Contains(err.Error(), "needs a record trigger or on.each (the row whose period") {
		t.Fatalf("no row: %v", err)
	}
}
