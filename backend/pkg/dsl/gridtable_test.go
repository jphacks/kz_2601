package dsl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func gridApp() App {
	app := Initial()
	app.Entities = []Entity{{ID: "lecture", Name: "授業", Fields: []Field{
		{ID: "subject", Name: "科目", Type: FieldString},
		{ID: "weekday", Name: "曜日", Type: FieldEnum, Options: []string{"月", "火", "水", "木", "金"}},
		{ID: "period", Name: "時限", Type: FieldInteger},
		{ID: "slot", Name: "時間帯", Type: FieldEnum, Options: []string{"朝", "昼", "夜"}},
		{ID: "when", Name: "日付", Type: FieldDate},
	}}}
	app.Screens = append(app.Screens, Screen{ID: "form", Title: "f", Entity: "lecture"})
	return app
}

func TestGridTableValid(t *testing.T) {
	app := gridApp()
	app.Screens[0].Components = []Component{
		{Type: CompGridTable, Entity: "lecture", ColumnField: "weekday", RowField: "period", TitleField: "subject",
			RowRange: &GridRange{From: 1, To: 6}, OnItemTap: &Action{Type: ActionNavigate, Screen: "form"}},
		{Type: CompGridTable, Entity: "lecture", ColumnField: "weekday", RowField: "slot", TitleField: "subject",
			ColumnValues: []string{"月", "水", "金"}, RowValues: []string{"夜", "朝"}},
		{Type: CompGridTable, Entity: "lecture", ColumnField: "subject", RowField: "period", TitleField: "weekday",
			ColumnValues: []string{"A", "B"}},
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("valid gridTable rejected: %v", err)
	}
}

func TestGridTableSemanticErrors(t *testing.T) {
	app := gridApp()
	g := func(mod func(c *Component)) Component {
		c := Component{Type: CompGridTable, Entity: "lecture", ColumnField: "weekday", RowField: "period", TitleField: "subject"}
		mod(&c)
		return c
	}
	app.Screens[0].Components = []Component{
		g(func(c *Component) { c.Entity = "nope" }),
		g(func(c *Component) { c.ColumnField = "missing" }),
		g(func(c *Component) { c.ColumnField = "when" }),                                       // date column
		g(func(c *Component) { c.RowField = "when" }),                                          // date row
		g(func(c *Component) { c.TitleField = "" }),                                            // no title
		g(func(c *Component) { c.SubtitleField = "zzz" }),                                      // unknown subtitle
		g(func(c *Component) { c.ColumnValues = []string{"月", "土"} }),                          // not an option
		g(func(c *Component) { c.ColumnValues = []string{"月", "月"} }),                          // duplicate
		g(func(c *Component) { c.RowRange = &GridRange{From: 5, To: 1} }),                      // inverted
		g(func(c *Component) { c.RowRange = &GridRange{From: 1, To: 30} }),                     // too many rows
		g(func(c *Component) { c.RowValues = []string{"1"} }),                                  // rowValues on integer
		g(func(c *Component) { c.RowField = "slot"; c.RowRange = &GridRange{From: 1, To: 2} }), // range on enum
		g(func(c *Component) { c.ColumnField = "subject" }),                                    // string without values
		g(func(c *Component) { c.RowField = "weekday" }),                                       // same field
	}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		`gridTable entity "nope" does not exist`,
		`gridTable columnField "missing" not in entity`,
		`gridTable columnField "when" must be enum or string`,
		`gridTable rowField "when" must be integer, enum or string`,
		`gridTable needs titleField`,
		`gridTable subtitleField "zzz" not in entity`,
		`gridTable columnValues "土" is not an option of "weekday"`,
		`gridTable columnValues has duplicate "月"`,
		`gridTable rowRange to (1) must be >= from (5)`,
		`gridTable rowRange has 30 rows (max 24)`,
		`gridTable rowValues is for enum/string rowField`,
		`gridTable rowRange is only for integer rowField`,
		`gridTable columnField "subject" is string; list the columns in columnValues`,
		`gridTable columnField and rowField must differ`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

// The bundled timetable example parses with the gridTable fields intact.
func TestGridTableExampleRoundTrip(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "timetable.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := Parse(raw)
	if err != nil {
		t.Fatalf("timetable.json: %v", err)
	}
	var grid *Component
	for i := range app.Screens[0].Components {
		if app.Screens[0].Components[i].Type == CompGridTable {
			grid = &app.Screens[0].Components[i]
		}
	}
	if grid == nil || grid.ColumnField != "weekday" || grid.RowField != "period" || grid.RowRange == nil || grid.RowRange.To != 5 {
		b, _ := json.Marshal(grid)
		t.Fatalf("gridTable not decoded: %s", b)
	}
	if missing := app.UnlistedEntities(); len(missing) != 0 {
		t.Errorf("gridTable should count as a list of its entity, unlisted=%v", missing)
	}
}
