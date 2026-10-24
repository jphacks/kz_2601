package ai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// The abbreviated few-shot shown to Claude must be exactly what fillDefaults
// turns back into the canonical example, otherwise the prompt teaches a form
// the server cannot complete.
func TestAbbreviateRoundTrip(t *testing.T) {
	for name, src := range map[string]string{"kakeibo": fewShotKakeibo, "expense_en": fewShotExpenseEN} {
		short := abbreviateExample(src)
		for _, gone := range []string{`"schemaVersion"`, `"voiceEntry"`, `"presentation"`, `"value":null`, `"direction"`, `"swipeToDelete"`, `"defaultValue":"now"`, "\n", "  "} {
			if strings.Contains(short, gone) {
				t.Errorf("%s: abbreviated example still contains %q", name, gone)
			}
		}
		if !strings.HasPrefix(short, `{"app":{"name":`) {
			t.Errorf("%s: abbreviated example must start with the app block: %.40s", name, short)
		}
		got := fillDefaults([]byte(short))
		if string(got) != string(canon([]byte(src))) {
			t.Errorf("%s: fillDefaults(abbreviate(x)) != x\n got %s\nwant %s", name, got, canon([]byte(src)))
		}
		if _, err := dsl.Parse(got); err != nil {
			t.Errorf("%s: filled example does not validate: %v", name, err)
		}
		if len(short) >= len(src)*3/4 {
			t.Errorf("%s: abbreviation saved too little: %d -> %d bytes", name, len(src), len(short))
		}
	}
}

func TestFillDefaults_RulesAndIdempotence(t *testing.T) {
	in := `{"app":{"name":"x"},"initialScreen":"home",
	 "entities":[{"id":"e","name":"E","fields":[{"id":"d","name":"D","type":"date"},{"id":"t","name":"T","type":"datetime","defaultValue":"2026-01-01"}]}],
	 "screens":[
	  {"id":"home","title":"H","toolbar":[{"label":"a","action":{"type":"navigate","screen":"form"}}],"components":[
	    {"type":"button","label":"b","action":{"type":"navigate","screen":"stats"}},
	    {"type":"button","label":"c","action":{"type":"navigate","screen":"form","presentation":"fullScreen"}},
	    {"type":"stat","label":"s","entity":"e","filter":{"field":"d","op":"today"}},
	    {"type":"chart","entity":"e","kind":"bar","groupBy":"d","aggregate":"count"},
	    {"type":"section","children":[{"type":"list","entity":"e","item":{"title":"d"},"sort":{"field":"d"},"onItemTap":{"type":"navigate","screen":"form"}}]},
	    {"type":"list","entity":"e","item":{"title":"d"},"swipeToDelete":false}]},
	  {"id":"form","title":"F","entity":"e","components":[]},
	  {"id":"stats","title":"S","components":[]}]}`
	out := fillDefaults([]byte(in))
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{
		`"schemaVersion":1`, `"voiceEntry":"floating"`,
		`"id":"d","name":"D","type":"date"`, `"defaultValue":"now"`, `"defaultValue":"2026-01-01"`,
		`"presentation":"sheet","screen":"form","type":"navigate"`,  // toolbar -> form
		`"presentation":"push","screen":"stats","type":"navigate"`,  // button -> non-form
		`"presentation":"fullScreen"`,                                // explicit value kept
		`"filter":{"field":"d","op":"today","value":null}`,
		`"aggregate":"sum"`, `"aggregate":"count"`,
		`"sort":{"direction":"desc","field":"d"}`,
		`"swipeToDelete":true`, `"swipeToDelete":false`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in\n%s", want, s)
		}
	}
	if strings.Count(s, `"presentation":"push"`) != 2 { // stats button + onItemTap
		t.Errorf("want 2 push presentations: %s", s)
	}
	if again := fillDefaults(out); string(again) != s {
		t.Errorf("fillDefaults is not idempotent")
	}
	if bad := fillDefaults([]byte("not json")); string(bad) != "not json" {
		t.Errorf("invalid input must pass through")
	}
}

func TestFillPatchDefaults(t *testing.T) {
	ops := []dsl.PatchOp{
		{Op: "add", Path: "/screens/0/components/-", Value: json.RawMessage(`{"type":"button","label":"b","action":{"type":"navigate","screen":"expense_form"}}`)},
		{Op: "add", Path: "/entities/0/fields/-", Value: json.RawMessage(`{"id":"when","name":"When","type":"date"}`)},
		{Op: "replace", Path: "/screens/0/components/2/children/0/filter", Value: json.RawMessage(`{"field":"date","op":"thisMonth"}`)},
		{Op: "remove", Path: "/screens/2"},
	}
	got := fillPatchDefaults(ops, []byte(fewShotKakeibo))
	if !strings.Contains(string(got[0].Value), `"presentation":"sheet"`) {
		t.Errorf("new button to a form should get sheet: %s", got[0].Value)
	}
	if !strings.Contains(string(got[1].Value), `"defaultValue":"now"`) {
		t.Errorf("new date field should default to now: %s", got[1].Value)
	}
	if string(got[2].Value) != `{"field":"date","op":"thisMonth"}` {
		// a bare filter value has no surrounding key, so it is left alone (dsl rejects it with a clear problem)
		t.Errorf("bare filter unexpectedly rewritten: %s", got[2].Value)
	}
	if len(got[3].Value) != 0 || len(ops[0].Value) == len(got[0].Value) {
		t.Errorf("remove op must stay untouched and input ops must not be mutated")
	}
	next, _, err := dsl.ApplyPatch([]byte(fewShotKakeibo), got[:2])
	if err != nil {
		t.Fatalf("filled patch must apply: %v", err)
	}
	if !strings.Contains(string(next), `"when"`) {
		t.Fatal("patched document lost the new field")
	}
}

func TestDraftSummary(t *testing.T) {
	ja := &dsl.AppMeta{Name: "家計簿", Tabs: []dsl.Tab{{Title: "ホーム"}, {Title: "集計"}}}
	if got := draftSummary(ja, LocaleJA); got != "家計簿アプリを作っています。ホームと集計の2タブです。" {
		t.Errorf("ja: %q", got)
	}
	en := &dsl.AppMeta{Name: "Expense Tracker"}
	if got := draftSummary(en, LocaleEN); got != "Making an Expense Tracker app." {
		t.Errorf("en: %q", got)
	}
}
