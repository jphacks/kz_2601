package ai

import (
	"encoding/json"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// fixStatUnitLabels drops a unit the stat value already shows from the end of
// its label (#177: sum over reps (unit 回) labelled 「今日の合計回数(回)」 read
// 「3 回」 twice). iOS appends field.unit to every aggregate except count
// (count gets 「件」 / a bare number via stat.value, so its label may carry the
// unit and is left alone). Stats in value mode have no field and are skipped.
// Returns raw unchanged when nothing applies or the result does not validate.
func fixStatUnitLabels(raw []byte) []byte {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return raw
	}
	units := map[string]map[string]string{} // entity -> field -> unit
	for _, e := range anyList(doc["entities"]) {
		eo, _ := e.(map[string]any)
		id, _ := eo["id"].(string)
		units[id] = map[string]string{}
		for _, f := range anyList(eo["fields"]) {
			fo, _ := f.(map[string]any)
			fid, _ := fo["id"].(string)
			if u, _ := fo["unit"].(string); strings.TrimSpace(u) != "" {
				units[id][fid] = strings.TrimSpace(u)
			}
		}
	}
	changed := false
	var walk func(cs []any)
	walk = func(cs []any) {
		for _, c := range cs {
			co, _ := c.(map[string]any)
			if co == nil {
				continue
			}
			if co["type"] == "stat" {
				agg, _ := co["aggregate"].(string)
				ent, _ := co["entity"].(string)
				fid, _ := co["field"].(string)
				label, _ := co["label"].(string)
				if u := units[ent][fid]; agg != "" && agg != "count" && u != "" {
					if trimmed, ok := trimUnitSuffix(label, u); ok {
						co["label"] = trimmed
						changed = true
					}
				}
			}
			walk(anyList(co["children"]))
		}
	}
	for _, s := range anyList(doc["screens"]) {
		so, _ := s.(map[string]any)
		walk(anyList(so["components"]))
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

// trimUnitSuffix removes a trailing "(u)" / "（u）" (case-insensitive, spaces
// before it trimmed) and reports whether it did. The rest must not be empty.
func trimUnitSuffix(label, unit string) (string, bool) {
	l := strings.TrimSpace(label)
	for _, p := range [][2]string{{"(", ")"}, {"（", "）"}} {
		suffix := p[0] + unit + p[1]
		if len(l) < len(suffix) || !strings.EqualFold(l[len(l)-len(suffix):], suffix) {
			continue
		}
		if rest := strings.TrimSpace(l[:len(l)-len(suffix)]); rest != "" {
			return rest, true
		}
	}
	return label, false
}
