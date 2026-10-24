package dsl

import (
	"strings"
	"testing"
)

func f64(v float64) *float64 { return &v }

// displayApp is a minimal app with one entity so display components can
// reference fields.
func displayApp(comps ...Component) App {
	return App{
		SchemaVersion: SchemaVersion,
		App:           AppMeta{Name: "g", VoiceEntry: VoiceEntryNone},
		InitialScreen: "home",
		Entities: []Entity{{ID: "expense", Name: "支出", Fields: []Field{
			{ID: "amount", Name: "金額", Type: FieldCurrency},
			{ID: "memo", Name: "メモ", Type: FieldString},
			{ID: "date", Name: "日付", Type: FieldDate},
			{ID: "receipt", Name: "レシート", Type: FieldPhoto},
		}}},
		Screens: []Screen{{ID: "home", Title: "h", Components: comps}},
	}
}

func problems(t *testing.T, app App) []string {
	t.Helper()
	err := app.Validate()
	if err == nil {
		return nil
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	return ve.Problems
}

func expectProblem(t *testing.T, app App, substr string) {
	t.Helper()
	ps := problems(t, app)
	for _, p := range ps {
		if strings.Contains(p, substr) {
			return
		}
	}
	t.Fatalf("expected a problem containing %q, got %v", substr, ps)
}

func TestDisplayComponentsValid(t *testing.T) {
	app := displayApp(
		Component{Type: CompLabel, Text: "今日", Icon: "sun.max", Style: "headline"},
		Component{Type: CompIcon, Name: "flame.fill", Size: "large", Color: "orange"},
		Component{Type: CompProgressBar, Label: "予算", Entity: "expense", Field: "amount", Target: f64(30000)},
		Component{Type: CompProgressBar, Label: "固定", Value: f64(0.4)},
		Component{Type: CompGauge, Label: "支出", Entity: "expense", Aggregate: "sum", Field: "amount", Min: f64(0), Max: f64(30000)},
		Component{Type: CompGauge, Label: "件数", Entity: "expense", Aggregate: "count", Max: f64(10)},
		Component{Type: CompBadge, Text: "今月", Color: "green"},
		Component{Type: CompLink, Text: "公式", URL: "https://jphacks.com"},
		Component{Type: CompMarkdown, Text: "**太字**"},
		Component{Type: CompAvatar, Name: "メタモン", Size: "large"},
		Component{Type: CompTable, Entity: "expense", Columns: []TableColumn{{Field: "date", Title: "日付"}, {Field: "amount"}}, Limit: 5,
			Sort: &Sort{Field: "date", Direction: "desc"}},
		Component{Type: CompTimeline, Entity: "expense", DateField: "date", TitleField: "memo", SubtitleField: "amount", Limit: 5},
		Component{Type: CompEmptyState, Icon: "tray", Title: "まだありません", Message: "追加してください"},
	)
	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestDisplayComponentsErrors(t *testing.T) {
	cases := []struct {
		name string
		c    Component
		want string
	}{
		{"label missing icon", Component{Type: CompLabel, Text: "x"}, "label needs text and icon"},
		{"icon missing name", Component{Type: CompIcon}, "icon needs name"},
		{"progress neither", Component{Type: CompProgressBar}, "needs value or entity"},
		{"progress both", Component{Type: CompProgressBar, Value: f64(0.5), Entity: "expense", Field: "amount", Target: f64(1)}, "not both"},
		{"progress out of range", Component{Type: CompProgressBar, Value: f64(1.5)}, "within 0..1"},
		{"progress no target", Component{Type: CompProgressBar, Entity: "expense", Field: "amount"}, "target > 0"},
		{"progress bad field", Component{Type: CompProgressBar, Entity: "expense", Field: "nope", Target: f64(1)}, `field "nope" not in entity`},
		{"progress bad entity", Component{Type: CompProgressBar, Entity: "ghost", Field: "amount", Target: f64(1)}, `entity "ghost" does not exist`},
		{"gauge range", Component{Type: CompGauge, Value: f64(1), Min: f64(5), Max: f64(5)}, "must be greater than min"},
		{"gauge neither", Component{Type: CompGauge}, "needs value or entity"},
		{"gauge count ok but filter bad", Component{Type: CompGauge, Entity: "expense", Aggregate: "count", Filter: &Filter{Field: "zzz", Op: "eq"}}, `filter field "zzz"`},
		{"badge empty", Component{Type: CompBadge}, "badge needs text"},
		{"link scheme", Component{Type: CompLink, Text: "x", URL: "ftp://a"}, "must be an http(s) URL"},
		{"markdown empty", Component{Type: CompMarkdown}, "markdown text is empty"},
		{"avatar bad image", Component{Type: CompAvatar, Image: "foo.png"}, "avatar image must be"},
		{"avatar field without form", Component{Type: CompAvatar, Field: "receipt"}, "requires screen.entity"},
		{"table no columns", Component{Type: CompTable, Entity: "expense"}, "at least one column"},
		{"table bad column", Component{Type: CompTable, Entity: "expense", Columns: []TableColumn{{Field: "nope"}}}, `columns[0] field "nope"`},
		{"table negative limit", Component{Type: CompTable, Entity: "expense", Columns: []TableColumn{{Field: "memo"}}, Limit: -1}, "limit must be >= 0"},
		{"timeline date type", Component{Type: CompTimeline, Entity: "expense", DateField: "memo", TitleField: "memo"}, "must be date or datetime"},
		{"timeline bad subtitle", Component{Type: CompTimeline, Entity: "expense", DateField: "date", TitleField: "memo", SubtitleField: "x"}, `subtitleField "x"`},
		{"emptyState no title", Component{Type: CompEmptyState}, "emptyState needs title"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { expectProblem(t, displayApp(tc.c), tc.want) })
	}
}

func TestAvatarFieldOnFormScreen(t *testing.T) {
	app := displayApp(Component{Type: CompAvatar, Field: "receipt", Name: "x"})
	app.Screens[0].Entity = "expense"
	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
	app.Screens[0].Components[0].Field = "memo"
	expectProblem(t, app, "must be type photo")
}

func TestSchemaRejectsFreeColor(t *testing.T) {
	raw := []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"none"},"initialScreen":"home","entities":[],
	"screens":[{"id":"home","title":"t","components":[{"type":"badge","text":"a","color":"#FF0000"}]}]}`)
	if _, err := Parse(raw); err == nil {
		t.Fatal("expected schema error for non-semantic color")
	}
	ok := []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"none"},"initialScreen":"home","entities":[],
	"screens":[{"id":"home","title":"t","components":[{"type":"badge","text":"a","color":"red"}]}]}`)
	if _, err := Parse(ok); err != nil {
		t.Fatal(err)
	}
}
