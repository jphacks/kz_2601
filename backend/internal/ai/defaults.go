package ai

import (
	"encoding/json"
	"regexp"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// The prompt tells Claude not to write values that are implied (fewer output
// tokens = faster generation). fillDefaults puts them back before dsl.Parse so
// pkg/dsl stays the strict gate and iOS always receives explicit values:
//
//	schemaVersion            -> 1
//	app.voiceEntry           -> "floating"
//	date/datetime field      -> defaultValue "now"
//	navigate.presentation    -> "push" from list/calendar/map item taps,
//	                            "sheet" to a form screen (screen.entity), else "push"
//	filter.value             -> null
//	sort.direction           -> "desc"
//	list.swipeToDelete       -> true
//	stat/chart.aggregate     -> "sum" (not when stat.value is set)
//	agg.filter[] value       -> null (each filter of a list too)
//
// abbreviateExample is the inverse used on the few-shot examples, and
// defaults_test.go asserts fillDefaults(abbreviateExample(x)) == x.

// fillDefaults completes a full AppDSL document. Invalid JSON is returned as is.
func fillDefaults(raw []byte) []byte {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return raw
	}
	if _, ok := doc["schemaVersion"]; !ok {
		doc["schemaVersion"] = dsl.SchemaVersion
	}
	if app, ok := doc["app"].(map[string]any); ok {
		if _, ok := app["voiceEntry"]; !ok {
			app["voiceEntry"] = string(dsl.VoiceEntryFloating)
		}
	}
	fillNode(doc, formScreens(doc))
	out, err := json.Marshal(doc)
	if err != nil {
		return raw
	}
	return out
}

// fillPatchDefaults completes DSL sub-documents carried by patch values
// (a new component, action or field) against the current document.
func fillPatchDefaults(ops []dsl.PatchOp, current []byte) []dsl.PatchOp {
	var doc map[string]any
	_ = json.Unmarshal(current, &doc)
	forms := formScreens(doc)
	out := make([]dsl.PatchOp, len(ops))
	for i, op := range ops {
		out[i] = op
		if len(op.Value) == 0 {
			continue
		}
		var v any
		if err := json.Unmarshal(op.Value, &v); err != nil {
			continue
		}
		fillNode(v, forms)
		if b, err := json.Marshal(v); err == nil {
			out[i].Value = b
		}
	}
	return out
}

// formScreens maps screen id -> true when the screen is a form (has entity).
func formScreens(doc map[string]any) map[string]bool {
	forms := map[string]bool{}
	screens, _ := doc["screens"].([]any)
	for _, s := range screens {
		sm, _ := s.(map[string]any)
		if id, _ := sm["id"].(string); id != "" {
			if e, _ := sm["entity"].(string); e != "" {
				forms[id] = true
			}
		}
	}
	return forms
}

var (
	actionTypes = map[string]bool{string(dsl.ActionNavigate): true}
	dateTypes   = map[string]bool{string(dsl.FieldDate): true, string(dsl.FieldDatetime): true}
)

// fillNode walks any DSL fragment and applies the defaults by shape, so the
// same code serves the full document and patch values.
func fillNode(v any, forms map[string]bool) {
	switch t := v.(type) {
	case []any:
		for _, e := range t {
			fillNode(e, forms)
		}
	case map[string]any:
		typ, _ := t["type"].(string)
		switch {
		case actionTypes[typ]:
			fillAction(t, forms, false)
		case dateTypes[typ] && t["id"] != nil && t["name"] != nil: // entity field
			if _, ok := t["defaultValue"]; !ok && t["computed"] == nil {
				t["defaultValue"] = "now"
			}
		case typ != "":
			fillComponent(t, typ)
		}
		fillFilters(t["filter"])
		if s, ok := t["sort"].(map[string]any); ok {
			if _, ok := s["direction"]; !ok {
				s["direction"] = "desc"
			}
		}
		for _, k := range []string{"onItemTap", "onDateTap"} {
			if a, ok := t[k].(map[string]any); ok {
				fillAction(a, forms, true)
			}
		}
		for k, sub := range t {
			switch k {
			case "filter", "sort", "onItemTap", "onDateTap":
				continue
			}
			fillNode(sub, forms)
		}
	}
}

func fillComponent(c map[string]any, typ string) {
	switch dsl.ComponentType(typ) {
	case dsl.CompList:
		if _, ok := c["swipeToDelete"]; !ok {
			c["swipeToDelete"] = true
		}
	case dsl.CompStat, dsl.CompChart:
		if _, ok := c["value"]; ok && typ == string(dsl.CompStat) {
			return // stat.value (Expr) replaces aggregate
		}
		if _, ok := c["aggregate"]; !ok {
			c["aggregate"] = "sum"
		}
	}
}

// fillFilters sets value null on a filter object or on each filter of an
// agg's filter list, then completes Expr nodes inside the values.
func fillFilters(v any) {
	switch t := v.(type) {
	case map[string]any:
		if _, ok := t["value"]; !ok {
			t["value"] = nil
		}
		fillNode(t["value"], nil)
	case []any:
		for _, f := range t {
			fillFilters(f)
		}
	}
}

func fillAction(a map[string]any, forms map[string]bool, fromItem bool) {
	if a["type"] != string(dsl.ActionNavigate) {
		return
	}
	if _, ok := a["presentation"]; ok {
		return
	}
	screen, _ := a["screen"].(string)
	switch {
	case fromItem:
		a["presentation"] = "push"
	case forms[screen]:
		a["presentation"] = "sheet"
	default:
		a["presentation"] = "push"
	}
}

// abbreviateExample renders a canonical example the way the prompt asks
// Claude to write: compact (no whitespace) and without the implied values
// above. Text-level removal keeps the source key order; the round-trip test
// proves it is lossless for the embedded examples.
func abbreviateExample(src string) string {
	s := compactJSON(src)
	for _, re := range abbreviateRes {
		s = re.ReplaceAllString(s, "")
	}
	return s
}

var abbreviateRes = []*regexp.Regexp{
	regexp.MustCompile(`"schemaVersion":1,`),
	regexp.MustCompile(`,"voiceEntry":"floating"`),
	regexp.MustCompile(`,"presentation":"(sheet|push)"`),
	regexp.MustCompile(`,"value":null`),
	regexp.MustCompile(`,"direction":"desc"`),
	regexp.MustCompile(`,"swipeToDelete":true`),
	regexp.MustCompile(`,"defaultValue":"now"`),
}

// compactJSON strips insignificant whitespace (invalid input is returned as is).
func compactJSON(src string) string {
	var v json.RawMessage
	if err := json.Unmarshal([]byte(src), &v); err != nil {
		return src
	}
	out, err := compactRaw(v)
	if err != nil {
		return src
	}
	return string(out)
}
