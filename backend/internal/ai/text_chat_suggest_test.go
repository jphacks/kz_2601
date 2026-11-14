package ai

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// mealSaveTo is what iOS sends for saveTo {meal, [dish, date, kind, kcal, memo]}.
func mealSaveTo() *ChatSaveTo {
	return &ChatSaveTo{Entity: "meal", Name: "食事", Today: "2026-11-08", Fields: []ChatSaveField{
		{ID: "dish", Name: "料理", Type: "string", Required: true},
		{ID: "date", Name: "日付", Type: "date"},
		{ID: "kind", Name: "区分", Type: "enum", Options: []string{"朝食", "昼食", "夕食"}},
		{ID: "kcal", Name: "カロリー", Type: "number", Unit: "kcal"},
		{ID: "memo", Name: "メモ", Type: "text"},
	}}
}

func chatSaveBody(t *testing.T, s *ChatSaveTo) string {
	t.Helper()
	b, _ := json.Marshal(TextRequest{Task: "chat", Instruction: "食事の記録の相棒",
		Messages: []TextMessage{{Role: "user", Text: "今日の夜カレー食べた"}}, SaveTo: s})
	return string(b)
}

// A chat with saveTo adds the record frame and suggest to the schema; the
// suggestion comes back checked against the fields.
func TestTextChatSuggest(t *testing.T) {
	out := `{"text":"カレーですね。記録しますか?","suggest":{"dish":" カレー ","date":"2026-11-08","kind":"夕食","kcal":"1,200","memo":null,"extra":"x"}}`
	svc, fc := textSvc(out)
	rec := postText(t, NewHandler(svc), "u1", chatSaveBody(t, mealSaveTo()))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	res := textResult(t, rec.Body.Bytes())
	want := map[string]any{"dish": "カレー", "date": "2026-11-08", "kind": "夕食", "kcal": float64(1200)}
	if res.Text != "カレーですね。記録しますか?" || len(res.Suggest) != len(want) {
		t.Fatalf("result %+v", res)
	}
	for k, v := range want {
		if res.Suggest[k] != v {
			t.Errorf("suggest[%s] = %#v, want %#v", k, res.Suggest[k], v)
		}
	}
	c := fc.calls[0]
	if !strings.Contains(c.System, "<record>") || !strings.Contains(c.System, "today: 2026-11-08") ||
		!strings.Contains(c.System, "- kind (区分): enum options=朝食 / 昼食 / 夕食") || !strings.Contains(c.System, "dish (料理): string required") {
		t.Fatalf("system %q", c.System)
	}
	var sch struct {
		Required   []string `json:"required"`
		Properties struct {
			Suggest struct {
				Properties map[string]map[string]any `json:"properties"`
			} `json:"suggest"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(c.Schema, &sch); err != nil {
		t.Fatal(err)
	}
	p := sch.Properties.Suggest.Properties
	if len(sch.Required) != 1 || sch.Required[0] != "text" || p["kcal"]["type"] != "number" || len(p["kind"]["enum"].([]any)) != 3 || p["date"]["type"] != "string" {
		t.Fatalf("schema %s", c.Schema)
	}
	// without saveTo the chat schema and frame stay as they were
	svc2, fc2 := textSvc(reply("こんばんは"))
	rec = postText(t, NewHandler(svc2), "u1", chatSaveBody(t, nil))
	if rec.Code != http.StatusOK || strings.Contains(fc2.calls[0].System, "<record>") || strings.Contains(string(fc2.calls[0].Schema), "suggest") {
		t.Fatalf("no saveTo: %d %q", rec.Code, fc2.calls[0].System)
	}
	if textResult(t, rec.Body.Bytes()).Suggest != nil {
		t.Fatal("no saveTo, no suggest")
	}
}

// Values that do not fit are dropped; a missing required field drops the
// whole suggestion.
func TestParseChatSuggest(t *testing.T) {
	s := mealSaveTo()
	cases := []struct {
		name, raw string
		want      map[string]any
	}{
		{"none", `{"text":"x"}`, nil},
		{"empty", `{"text":"x","suggest":{}}`, nil},
		{"enum not an option", `{"suggest":{"dish":"カレー","kind":"ディナー"}}`, map[string]any{"dish": "カレー"}},
		{"bad date", `{"suggest":{"dish":"カレー","date":"11/8"}}`, map[string]any{"dish": "カレー"}},
		{"date far from today", `{"suggest":{"dish":"カレー","date":"2024-11-08"}}`, map[string]any{"dish": "カレー"}},
		{"date near today", `{"suggest":{"dish":"カレー","date":"2026-11-07"}}`, map[string]any{"dish": "カレー", "date": "2026-11-07"}},
		{"number too large", `{"suggest":{"dish":"カレー","kcal":1e12}}`, map[string]any{"dish": "カレー"}},
		{"number not a number", `{"suggest":{"dish":"カレー","kcal":"たくさん"}}`, map[string]any{"dish": "カレー"}},
		{"negative number", `{"suggest":{"dish":"カレー","kcal":-5.5}}`, map[string]any{"dish": "カレー", "kcal": -5.5}},
		{"wrong type", `{"suggest":{"dish":12}}`, nil},
		{"required missing", `{"suggest":{"kind":"夕食"}}`, nil},
		{"required blank", `{"suggest":{"dish":"  ","kind":"夕食"}}`, nil},
		{"not JSON", `nope`, nil},
	}
	for _, c := range cases {
		got := parseChatSuggest([]byte(c.raw), s)
		if len(got) != len(c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
			continue
		}
		for k, v := range c.want {
			if got[k] != v {
				t.Errorf("%s: [%s] %#v, want %#v", c.name, k, got[k], v)
			}
		}
	}
	long := parseChatSuggest([]byte(`{"suggest":{"dish":"`+strings.Repeat("あ", 300)+`","memo":"`+strings.Repeat("い", 3000)+`"}}`), s)
	if len([]rune(long["dish"].(string))) != maxChatSuggestString || len([]rune(long["memo"].(string))) != maxChatSuggestText {
		t.Fatalf("caps: %d %d", len([]rune(long["dish"].(string))), len([]rune(long["memo"].(string))))
	}
	if parseChatSuggest([]byte(`{"suggest":{"dish":"x"}}`), nil) != nil {
		t.Fatal("no saveTo, no suggest")
	}
}

func TestNormalizeChatSaveTo(t *testing.T) {
	cases := map[string]func(s *ChatSaveTo){
		"entity id":       func(s *ChatSaveTo) { s.Entity = "Meal!" },
		"today":           func(s *ChatSaveTo) { s.Today = "2026/11/08" },
		"no fields":       func(s *ChatSaveTo) { s.Fields = nil },
		"too many fields": func(s *ChatSaveTo) { s.Fields = append(s.Fields, s.Fields...) },
		"twice":           func(s *ChatSaveTo) { s.Fields[1].ID = "dish" },
		"type":            func(s *ChatSaveTo) { s.Fields[0].Type = "boolean" },
		"enum options":    func(s *ChatSaveTo) { s.Fields[2].Options = nil },
		"options on text": func(s *ChatSaveTo) { s.Fields[4].Options = []string{"a"} },
	}
	for name, mutate := range cases {
		s := mealSaveTo()
		mutate(s)
		req := TextRequest{Task: "chat", Instruction: "p", Messages: []TextMessage{{Role: "user", Text: "x"}}, SaveTo: s}
		if err := normalizeTextRequest(&req); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	ok := TextRequest{Task: "chat", Instruction: "p", Messages: []TextMessage{{Role: "user", Text: "x"}}, SaveTo: mealSaveTo()}
	if err := normalizeTextRequest(&ok); err != nil {
		t.Fatal(err)
	}
	// names, units and options over the bounds drop saveTo: the chat goes on without suggest
	drops := map[string]func(s *ChatSaveTo){
		"long entity name": func(s *ChatSaveTo) { s.Name = strings.Repeat("あ", 41) },
		"no field name":    func(s *ChatSaveTo) { s.Fields[0].Name = " " },
		"long field name":  func(s *ChatSaveTo) { s.Fields[0].Name = strings.Repeat("あ", 41) },
		"long unit":        func(s *ChatSaveTo) { s.Fields[3].Unit = strings.Repeat("u", 21) },
		"many options":     func(s *ChatSaveTo) { s.Fields[2].Options = manyOptions(51) },
		"long option":      func(s *ChatSaveTo) { s.Fields[2].Options = []string{"朝食", strings.Repeat("あ", 51)} },
		"blank option":     func(s *ChatSaveTo) { s.Fields[2].Options = []string{"朝食", " "} },
	}
	for name, mutate := range drops {
		s := mealSaveTo()
		mutate(s)
		req := TextRequest{Task: "chat", Instruction: "p", Messages: []TextMessage{{Role: "user", Text: "x"}}, SaveTo: s}
		if err := normalizeTextRequest(&req); err != nil {
			t.Errorf("%s: %v (want saveTo dropped, not 400)", name, err)
		} else if req.SaveTo != nil {
			t.Errorf("%s: saveTo kept", name)
		}
	}
	edge := mealSaveTo()
	edge.Name, edge.Fields[0].Name, edge.Fields[3].Unit = strings.Repeat("あ", 40), strings.Repeat("あ", 40), strings.Repeat("u", 20)
	edge.Fields[2].Options = manyOptions(50)
	atBounds := TextRequest{Task: "chat", Instruction: "p", Messages: []TextMessage{{Role: "user", Text: "x"}}, SaveTo: edge}
	if err := normalizeTextRequest(&atBounds); err != nil || atBounds.SaveTo == nil {
		t.Fatalf("at the bounds: err=%v saveTo=%v", err, atBounds.SaveTo)
	}
	gen := TextRequest{Instruction: "励ましの一言", SaveTo: mealSaveTo()}
	if err := normalizeTextRequest(&gen); err == nil || !strings.Contains(err.Error(), "saveTo is only for task chat") {
		t.Fatalf("generate with saveTo: %v", err)
	}
}

// The creator pays for a chat with saveTo only when the fields are the
// component's own, as the device builds them from the DSL; an older device
// sends no saveTo and still matches.
func TestRequestInAppChatSaveTo(t *testing.T) {
	app := dsl.App{
		Entities: []dsl.Entity{
			{ID: "turn", Name: "会話", Fields: []dsl.Field{{ID: "text", Name: "本文", Type: dsl.FieldText}}},
			{ID: "meal", Name: " 食事 ", Fields: []dsl.Field{
				{ID: "dish", Name: "料理", Type: dsl.FieldString, Required: true},
				{ID: "date", Name: "日付", Type: dsl.FieldDate, Required: true, DefaultValue: "now"},
				{ID: "kind", Name: "区分", Type: dsl.FieldEnum, Options: []string{"朝食", "昼食", "夕食"}},
				{ID: "kcal", Name: "カロリー", Type: dsl.FieldNumber, Unit: "kcal"},
				{ID: "memo", Name: "メモ", Type: dsl.FieldText},
			}},
		},
		Screens: []dsl.Screen{{ID: "s", Components: []dsl.Component{
			{Type: dsl.CompAiChat, Entity: "turn", Persona: "p", SaveTo: &dsl.AISaveTo{Entity: "meal", Fields: []string{"dish", "date", "kind", "kcal", "memo"}}},
		}}},
	}
	req := func(s *ChatSaveTo) TextRequest {
		r := TextRequest{Task: "chat", Instruction: "p", Messages: []TextMessage{{Role: "user", Text: "x"}}, SaveTo: s}
		if err := normalizeTextRequest(&r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	if !requestInApp(&app, req(mealSaveTo())) {
		t.Fatal("the component's own saveTo must match")
	}
	if !requestInApp(&app, req(nil)) {
		t.Fatal("a chat without saveTo (older device) must match")
	}
	other := mealSaveTo()
	other.Fields[2].Options = []string{"朝食", "昼食", "夕食", "夜食"}
	if requestInApp(&app, req(other)) {
		t.Fatal("other options must not match")
	}
	fewer := mealSaveTo()
	fewer.Fields = fewer.Fields[:1]
	if requestInApp(&app, req(fewer)) {
		t.Fatal("other fields must not match")
	}
	app.Screens[0].Components[0].SaveTo = nil
	if requestInApp(&app, req(mealSaveTo())) {
		t.Fatal("a saveTo the component does not have must not match")
	}
}

// The flattened generation schema accepts both saveTo shapes (aiButton
// {entity, field}, aiChat {entity, fields}).
func TestGenerationSchemaSaveToShapes(t *testing.T) {
	var root map[string]any
	if err := json.Unmarshal(GenerationSchema(), &root); err != nil {
		t.Fatal(err)
	}
	leaf := root["$defs"].(map[string]any)["Leaf"].(map[string]any)["properties"].(map[string]any)
	alts, ok := leaf["saveTo"].(map[string]any)["anyOf"].([]any)
	if !ok || len(alts) != 2 {
		t.Fatalf("leaf saveTo: %v", leaf["saveTo"])
	}
	defs := root["$defs"].(map[string]any)
	if defs["AISaveTo"] == nil || defs["AIChatSaveTo"] == nil {
		t.Fatal("saveTo defs pruned")
	}
	t.Logf("generation schema: %d bytes", len(GenerationSchema()))
}

func manyOptions(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "o" + strconv.Itoa(i)
	}
	return out
}
