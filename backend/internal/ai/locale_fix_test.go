package ai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Shape of the #142 eval outputs: "$" units on currency, count stats on books.
const enUnitsDSL = `{"schemaVersion":1,"app":{"name":"Reading Log","icon":"book","theme":{"primaryColor":"#1565C0"},"voiceEntry":"floating"},"initialScreen":"home",
"entities":[{"id":"book","name":"Book","fields":[{"id":"title","name":"Title","type":"string"},{"id":"price","name":"Price","type":"currency","unit":"$"},{"id":"date","name":"Date","type":"date","defaultValue":"now"}]}],
"screens":[{"id":"home","title":"Books","toolbar":[{"label":"Add","icon":"plus","action":{"type":"navigate","screen":"form","presentation":"sheet"}}],"components":[
{"type":"stat","id":"read","area":"A1:C2","label":"Books read this year","entity":"book","aggregate":"count","filter":{"field":"date","op":"thisYear","value":null}},
{"type":"stat","id":"items","area":"D1:F2","label":"Items","entity":"book","aggregate":"count"},
{"type":"list","id":"l","area":"A3:F10","entity":"book","item":{"title":"title","trailing":"price"},"onItemTap":{"type":"navigate","screen":"form","presentation":"push"}}]},
{"id":"form","title":"Book","entity":"book","toolbar":[{"label":"Save","icon":"checkmark","action":{"type":"saveRecord","entity":"book"}}],"components":[
{"type":"section","id":"s","area":"A1:F5","children":[{"type":"textField","field":"title","label":"Title"},{"type":"numberField","field":"price","label":"Price","unit":"$"},{"type":"datePicker","field":"date","label":"Date"}]}]}]}`

func TestFixEnglishUnits(t *testing.T) {
	if _, err := dsl.Parse([]byte(enUnitsDSL)); err != nil {
		t.Fatalf("fixture must validate: %v", err)
	}
	if got := fixEnglishUnits([]byte(enUnitsDSL), LocaleJA); string(got) != enUnitsDSL {
		t.Fatal("ja must be untouched")
	}
	out := fixEnglishUnits([]byte(enUnitsDSL), LocaleEN)
	app, err := dsl.Parse(out)
	if err != nil {
		t.Fatalf("fixed DSL must validate: %v", err)
	}
	if strings.Contains(string(out), `"$"`) {
		t.Errorf("currency units must be dropped: %s", out)
	}
	stats := map[string]*dsl.Component{}
	for i := range app.Screens[0].Components {
		c := &app.Screens[0].Components[i]
		stats[c.ID] = c
	}
	read := stats["read"]
	if read.Aggregate != "" || read.Value == nil || read.Value.Agg == nil || read.Value.Agg.Agg != "count" || len(read.Value.Agg.Filter) != 1 {
		b, _ := json.Marshal(read)
		t.Errorf("count stat on books must become stat.value count with the filter: %s", b)
	}
	if stats["items"].Aggregate != "count" {
		t.Error(`a count stat labelled "Items" keeps aggregate count (iOS unit "items" fits)`)
	}
	// already clean: unchanged bytes
	if again := fixEnglishUnits(out, LocaleEN); string(again) != string(out) {
		t.Error("second pass must be a no-op")
	}
}

func TestPluralEN_Uncountable(t *testing.T) {
	for in, want := range map[string]string{"spending": "spending", "fan spending": "fan spending", "reading": "reading", "daily check": "daily checks", "study session": "study sessions"} {
		if got := pluralEN(in); got != want {
			t.Errorf("pluralEN(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSystemPromptHasEnglishRules(t *testing.T) {
	for _, s := range []string{
		"## locale en のとき",
		`"last 7 days" / "past week"`,
		"済みの件数",
		`"$" を書くと "¥ 15 $" になる`,
		`単位 "items" を付ける`,
		"boolean field を置かない",
	} {
		if !strings.Contains(systemPrompt, s) {
			t.Errorf("system prompt lost the en rule %q", s)
		}
	}
	if strings.Contains(fewShotExpenseEN, `"$"`) {
		t.Error(`en few-shot must not carry unit "$" (iOS renders currency as ¥)`)
	}
}
