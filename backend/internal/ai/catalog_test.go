package ai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// schemaTypes reads every component type (Container enum + Leaf consts) and
// every action type from the canonical schema, the same way the iOS
// ComponentRegistryTests does, so both sides enforce "schema is the only truth".
func schemaTypes(t *testing.T) (components, actions []string, defs map[string]any) {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(dsl.SchemaJSON(), &root); err != nil {
		t.Fatal(err)
	}
	defs = root["$defs"].(map[string]any)
	components = append(components, enumValues(defs["Container"].(map[string]any), "type")...)
	for _, name := range refNames(defs["Leaf"]) {
		ct := constType(defs[name].(map[string]any))
		if ct == "" {
			t.Fatalf("$defs/%s has no type const", name)
		}
		components = append(components, ct)
	}
	actions = enumValues(defs["Action"].(map[string]any), "type")
	if len(components) < 18 || len(actions) < 9 {
		t.Fatalf("schema scan incomplete: %d components, %d actions", len(components), len(actions))
	}
	return components, actions, defs
}

// Every schema type must appear as its own catalog line, and the catalog must
// be what the system prompt embeds (no hand-written copy can drift again).
func TestCatalogCoversEverySchemaType(t *testing.T) {
	components, actions, _ := schemaTypes(t)
	for _, c := range components {
		if !strings.Contains(catalog, "\n- "+c+":") {
			t.Errorf("component %q missing from catalog", c)
		}
	}
	for _, a := range actions {
		if !strings.Contains(catalog, "\n- "+a+"{") && !strings.Contains(catalog, "\n- "+a+" ") {
			t.Errorf("action %q missing from catalog", a)
		}
	}
	if !strings.Contains(systemPrompt, catalog) {
		t.Fatal("system prompt does not embed the generated catalog")
	}
	// Go's closed enums must match the schema too (adding a Leaf to the schema
	// without a ComponentType constant would silently pass the catalog test).
	goTypes := map[string]bool{}
	for _, ct := range []dsl.ComponentType{dsl.CompVStack, dsl.CompHStack, dsl.CompSection, dsl.CompText, dsl.CompButton,
		dsl.CompTextField, dsl.CompNumberField, dsl.CompDatePicker, dsl.CompPicker, dsl.CompToggle, dsl.CompPhotoField,
		dsl.CompImage, dsl.CompList, dsl.CompStat, dsl.CompChart, dsl.CompVoiceInput, dsl.CompSpacer, dsl.CompDivider,
		dsl.CompCard, dsl.CompDisclosure, dsl.CompCarousel, dsl.CompGrid,
		dsl.CompSearchBar, dsl.CompCalendar, dsl.CompShareButton, dsl.CompOpenURLButton,
		dsl.CompMapView, dsl.CompLocationButton, dsl.CompQrScanner, dsl.CompTimerDisplay,
		dsl.CompSlider, dsl.CompStepper, dsl.CompTextEditor, dsl.CompSecureField, dsl.CompTimePicker, dsl.CompColorPicker, dsl.CompRating,
		// display
		dsl.CompLabel, dsl.CompIcon, dsl.CompProgressBar, dsl.CompGauge, dsl.CompBadge, dsl.CompLink, dsl.CompMarkdown,
		dsl.CompAvatar, dsl.CompTable, dsl.CompTimeline, dsl.CompEmptyState,
	} {
		goTypes[string(ct)] = true
	}
	for _, c := range components {
		if !goTypes[c] {
			t.Errorf("schema component %q has no dsl.ComponentType constant", c)
		}
	}
	if len(goTypes) != len(components) {
		t.Errorf("dsl has %d component types, schema has %d", len(goTypes), len(components))
	}
}

// Every type needs a description (and components an x-usage) in the schema;
// an empty one would leave Claude guessing when to pick the component.
func TestEverySchemaTypeHasDescription(t *testing.T) {
	_, actions, defs := schemaTypes(t)
	container := defs["Container"].(map[string]any)
	for _, c := range enumValues(container, "type") {
		if enumDesc(container, c) == "" {
			t.Errorf("Container type %q has no x-enumDescriptions entry", c)
		}
	}
	if s, _ := container[keyUsage].(string); s == "" {
		t.Error("Container has no x-usage")
	}
	for _, name := range refNames(defs["Leaf"]) {
		def := defs[name].(map[string]any)
		if s, _ := def["description"].(string); strings.TrimSpace(s) == "" {
			t.Errorf("$defs/%s has no description", name)
		}
		if s, _ := def[keyUsage].(string); strings.TrimSpace(s) == "" {
			t.Errorf("$defs/%s has no x-usage", name)
		}
	}
	action := defs["Action"].(map[string]any)
	for _, a := range actions {
		if enumDesc(action, a) == "" {
			t.Errorf("Action type %q has no x-enumDescriptions entry", a)
		}
	}
}

func TestCatalogRendering(t *testing.T) {
	for _, want := range []string{
		"- section: children / spacing(none|small|medium|large), title",
		"- vstack: children / spacing(none|small|medium|large) —", // title is section-only (x-for)
		"- list: entity, item{title} / emptyText, filter{field,op,value}, onItemTap, searchField, sort{field,direction}, swipeToDelete",
		"- stat: label, entity, aggregate(count|sum|avg|min|max) / field, filter{field,op,value}, format(plain|currency|percent)",
		"- filter{field, op(eq|neq|gt|gte|lt|lte|thisMonth|today), value}",
		"- navigate{screen / presentation(push|sheet|fullScreen)}",
		"- saveRecord{entity / screen}",
		"- back{(追加キー無し)}",
		"- scheduleNotification{notification{id,title,body,repeat}}",
		"- voiceInput: / prompt, size(compact|large)",
		"- divider: (追加キー無し) —",
		"- notification{id, title, body, repeat(once|daily|weekly) / time, weekday(mon|tue|wed|thu|fri|sat|sun)}",
		"どの action にも付けられる任意キー: confirmMessage",
	} {
		if !strings.Contains(catalog, want) {
			t.Errorf("catalog lacks %q\n%s", want, catalog)
		}
	}
	// x-* metadata never reaches the generation schema (prompt tokens).
	for _, raw := range []json.RawMessage{GenerationSchema(), PatchGenerationSchema()} {
		if strings.Contains(string(raw), `"x-`) {
			t.Error("generation schema still contains x-* metadata")
		}
	}
}
