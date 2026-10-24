package ai

import (
	"encoding/json"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// fixEnglishUnits is a deterministic post-process for locale en creates
// (#142). iOS renders currency as "¥1,200" whatever the unit and appends the
// unit after a currency numberField, so an English "$" unit showed up as
// "¥ 15 $"; and a stat with aggregate count gets the unit "items", which reads
// wrong for books or workouts ("3 items"). It
//   - drops unit from currency fields and from numberFields bound to them;
//   - turns a count stat whose label does not say "item" into
//     stat.value {"agg":"count",...} (a bare number; the label names the unit).
//
// Returns raw unchanged for other locales, when nothing applies, or when the
// result does not validate. No API calls.
func fixEnglishUnits(raw []byte, loc Locale) []byte {
	if loc != LocaleEN {
		return raw
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return raw
	}
	changed := false
	currency := map[string]map[string]bool{} // entity -> currency field ids
	for _, e := range anyList(doc["entities"]) {
		eo, _ := e.(map[string]any)
		id, _ := eo["id"].(string)
		currency[id] = map[string]bool{}
		for _, f := range anyList(eo["fields"]) {
			fo, _ := f.(map[string]any)
			if fo["type"] != "currency" {
				continue
			}
			fid, _ := fo["id"].(string)
			currency[id][fid] = true
			if _, ok := fo["unit"]; ok {
				delete(fo, "unit")
				changed = true
			}
		}
	}
	for _, s := range anyList(doc["screens"]) {
		so, _ := s.(map[string]any)
		ent, _ := so["entity"].(string)
		for _, c := range anyList(so["components"]) {
			co, _ := c.(map[string]any)
			comps := append([]any{co}, anyList(co["children"])...)
			for _, x := range comps {
				xo, _ := x.(map[string]any)
				switch xo["type"] {
				case "numberField":
					if fid, _ := xo["field"].(string); currency[ent][fid] {
						if _, ok := xo["unit"]; ok {
							delete(xo, "unit")
							changed = true
						}
					}
				case "stat":
					if countStatToValue(xo) {
						changed = true
					}
				}
			}
		}
	}
	if !changed {
		return raw
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return raw
	}
	if _, err := dsl.Parse(out); err != nil {
		return raw
	}
	return out
}

// countStatToValue rewrites {aggregate:count, entity, filter?} as
// {value:{agg:count, entity, filter?}} unless the label is about items.
func countStatToValue(c map[string]any) bool {
	if c["aggregate"] != "count" || c["value"] != nil {
		return false
	}
	label, _ := c["label"].(string)
	if strings.Contains(strings.ToLower(label), "item") {
		return false
	}
	ent, _ := c["entity"].(string)
	if ent == "" {
		return false
	}
	v := map[string]any{"agg": "count", "entity": ent}
	if f, ok := c["filter"]; ok && f != nil {
		v["filter"] = f
	}
	for _, k := range []string{"aggregate", "entity", "field", "filter"} {
		delete(c, k)
	}
	c["value"] = v
	return true
}

func anyList(v any) []any { a, _ := v.([]any); return a }
