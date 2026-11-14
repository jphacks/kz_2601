package dsl

import (
	"encoding/json"
	"strings"
	"testing"
)

// mealChatApp: turn{text, role} + meal log, an aiChat over turn with saveTo.
func mealChatApp(saveTo *AISaveTo) App {
	return App{
		SchemaVersion: SchemaVersion,
		App:           AppMeta{Name: "食事メモ", VoiceEntry: VoiceEntryNone},
		InitialScreen: "talk",
		Entities: []Entity{
			{ID: "turn", Name: "会話", Fields: []Field{
				{ID: "text", Name: "本文", Type: FieldText, Required: true},
				{ID: "role", Name: "役", Type: FieldEnum, Options: []string{"user", "assistant"}},
			}},
			{ID: "meal", Name: "食事", TitleField: "dish", Fields: []Field{
				{ID: "dish", Name: "料理", Type: FieldString, Required: true},
				{ID: "date", Name: "日付", Type: FieldDate, Required: true},
				{ID: "kind", Name: "区分", Type: FieldEnum, Options: []string{"朝食", "昼食", "夕食"}},
				{ID: "kcal", Name: "カロリー", Type: FieldNumber, Unit: "kcal"},
				{ID: "memo", Name: "メモ", Type: FieldText},
				{ID: "done", Name: "済", Type: FieldBoolean},
				{ID: "n", Name: "数", Type: FieldInteger},
				{ID: "secret", Name: "答え", Type: FieldString, Hidden: true},
				{ID: "calc", Name: "計算", Type: FieldString, Computed: &Expr{Kind: ExprLiteral, Lit: "x"}},
			}},
			{ID: "wall", Name: "掲示", Scope: ScopePublic, Fields: []Field{{ID: "t", Name: "t", Type: FieldString}}},
			{ID: "prefs", Name: "設定", Single: true, Fields: []Field{{ID: "t", Name: "t", Type: FieldString}}},
			{ID: "board", Name: "共有", Scope: ScopeShared, Fields: []Field{
				{ID: "t", Name: "t", Type: FieldString},
				{ID: "pin", Name: "固定", Type: FieldString, EditableBy: RoleAdmins},
			}},
			{ID: "strict", Name: "必須", Fields: []Field{
				{ID: "t", Name: "t", Type: FieldString},
				{ID: "who", Name: "誰", Type: FieldString, Required: true},
			}},
		},
		Screens: []Screen{
			{ID: "talk", Title: "話す", Components: []Component{
				{Type: CompAiChat, Entity: "turn", TextField: "text", RoleField: "role", Persona: "食事を聞いて記録を手伝う相棒", SaveTo: saveTo},
			}},
		},
	}
}

func TestAIChatSaveToValidPasses(t *testing.T) {
	for _, s := range []*AISaveTo{
		{Entity: "meal", Fields: []string{"dish", "date", "kind", "kcal", "memo"}},
		{Entity: "board", Fields: []string{"t"}},
	} {
		app := mealChatApp(s)
		if err := app.Validate(); err != nil {
			t.Fatalf("%v: %v", s, err)
		}
		raw, err := json.Marshal(app)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), `"field":`) {
			t.Fatalf("aiChat saveTo must not marshal an empty field: %s", raw)
		}
		if _, err := Parse(raw); err != nil {
			t.Fatalf("schema rejects aiChat saveTo: %v", err)
		}
	}
}

func TestAIChatSaveToSemanticErrors(t *testing.T) {
	cases := map[string]*AISaveTo{
		`aiChat saveTo entity "nope" does not exist`:                            {Entity: "nope", Fields: []string{"t"}},
		`aiChat saveTo entity must differ from the conversation entity`:         {Entity: "turn", Fields: []string{"text"}},
		`aiChat saveTo cannot add to public entity "wall"`:                      {Entity: "wall", Fields: []string{"t"}},
		`aiChat saveTo cannot add to single entity "prefs"`:                     {Entity: "prefs", Fields: []string{"t"}},
		`aiChat saveTo needs 1..8 fields`:                                       {Entity: "meal"},
		`aiChat saveTo field "dish" is listed twice`:                            {Entity: "meal", Fields: []string{"dish", "date", "dish"}},
		`aiChat saveTo field "zz" not in entity "meal"`:                         {Entity: "meal", Fields: []string{"dish", "date", "zz"}},
		`aiChat saveTo field "calc" is computed`:                                {Entity: "meal", Fields: []string{"dish", "date", "calc"}},
		`aiChat saveTo field "done" must be string, text, number, date or enum`: {Entity: "meal", Fields: []string{"dish", "date", "done"}},
		`aiChat saveTo field "n" must be string, text, number, date or enum`:    {Entity: "meal", Fields: []string{"dish", "date", "n"}},
		`aiChat saveTo field "pin" is editableBy admins`:                        {Entity: "board", Fields: []string{"pin"}},
		`aiChat saveTo field "secret" is hidden`:                                {Entity: "meal", Fields: []string{"dish", "date", "secret"}},
		`required field "who" of entity "strict" would stay empty`:              {Entity: "strict", Fields: []string{"t"}},
		`aiChat saveTo takes fields (a list of field ids), not field`:           {Entity: "meal", Field: "dish", Fields: []string{"dish", "date"}},
	}
	for want, s := range cases {
		app := mealChatApp(s)
		err := app.Validate()
		ve, ok := err.(*ValidationError)
		if !ok {
			t.Errorf("%s: expected ValidationError, got %v", want, err)
			continue
		}
		if joined := strings.Join(ve.Problems, "\n"); !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}
	// names, units and enum options over what /v1/text reads
	long := func(n int) string { return strings.Repeat("あ", n) }
	bounds := map[string]func(a *App){
		`aiChat saveTo entity "meal" name must be at most 40 characters`: func(a *App) { a.Entities[1].Name = long(41) },
		`aiChat saveTo field "dish" name must be 1..40 characters`:       func(a *App) { a.Entities[1].Fields[0].Name = long(41) },
		`aiChat saveTo field "kcal" unit must be at most 20 characters`:  func(a *App) { a.Entities[1].Fields[3].Unit = long(21) },
		`aiChat saveTo field "kind" must have at most 50 options`: func(a *App) {
			opts := make([]string, 51)
			for i := range opts {
				opts[i] = "o" + strings.Repeat("x", i)
			}
			a.Entities[1].Fields[2].Options = opts
		},
		`aiChat saveTo field "kind" options must be 1..50 characters`: func(a *App) { a.Entities[1].Fields[2].Options = []string{"朝食", long(51)} },
	}
	for want, mutate := range bounds {
		app := mealChatApp(&AISaveTo{Entity: "meal", Fields: []string{"dish", "date", "kind", "kcal"}})
		mutate(&app)
		ve, ok := app.Validate().(*ValidationError)
		if !ok {
			t.Errorf("%s: expected ValidationError", want)
			continue
		}
		if joined := strings.Join(ve.Problems, "\n"); !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}
	atBounds := mealChatApp(&AISaveTo{Entity: "meal", Fields: []string{"dish", "date", "kind", "kcal"}})
	atBounds.Entities[1].Name, atBounds.Entities[1].Fields[0].Name, atBounds.Entities[1].Fields[3].Unit = long(40), long(40), long(20)
	if err := atBounds.Validate(); err != nil {
		t.Fatalf("at the bounds: %v", err)
	}
	// a required date fills itself (today), so it may stay out of fields
	ok := mealChatApp(&AISaveTo{Entity: "meal", Fields: []string{"dish"}})
	if err := ok.Validate(); err != nil {
		t.Fatalf("required date outside fields: %v", err)
	}
}

// The schema keeps the two shapes apart: aiChat takes {entity, fields},
// aiButton {entity, field}.
func TestAIChatSaveToSchemaShapes(t *testing.T) {
	chat := `{"type":"aiChat","entity":"turn","textField":"text","roleField":"role","persona":"p","saveTo":%s}`
	button := `{"type":"aiButton","task":"generate","prompt":"p","shape":"list","saveTo":%s}`
	for _, tc := range []struct {
		comp, saveTo string
		ok           bool
	}{
		{chat, `{"entity":"meal","fields":["dish"]}`, true},
		{chat, `{"entity":"meal","field":"dish"}`, false},
		{chat, `{"entity":"meal","fields":[]}`, false},
		{chat, `{"entity":"meal","fields":["a","b","c","d","e","f","g","h","i"]}`, false},
		{button, `{"entity":"meal","field":"dish"}`, true},
		{button, `{"entity":"meal","fields":["dish"]}`, false},
	} {
		comp := strings.Replace(tc.comp, "%s", tc.saveTo, 1)
		problems := schemaProblemsOfComponent(t, comp)
		if (len(problems) == 0) != tc.ok {
			t.Errorf("%s: ok=%v problems=%v", comp, tc.ok, problems)
		}
	}
}

// schemaProblemsOfComponent validates a one-component app against the
// schema only.
func schemaProblemsOfComponent(t *testing.T, comp string) []string {
	t.Helper()
	app := mealChatApp(nil)
	raw, err := json.Marshal(app)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var c any
	if err := json.Unmarshal([]byte(comp), &c); err != nil {
		t.Fatal(err)
	}
	doc["screens"].([]any)[0].(map[string]any)["components"] = []any{c}
	raw, _ = json.Marshal(doc)
	if err := ValidateSchema(raw); err != nil {
		return err.(*ValidationError).Problems
	}
	return nil
}
