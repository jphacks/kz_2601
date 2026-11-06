package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func importApp() App {
	app := Initial()
	app.Entities = []Entity{
		{ID: "member", Name: "部員", TitleField: "name", Fields: []Field{
			{ID: "name", Name: "名前", Type: FieldString, Required: true},
			{ID: "grade", Name: "学年", Type: FieldInteger},
			{ID: "photo", Name: "写真", Type: FieldPhoto},
			{ID: "label", Name: "表示", Type: FieldString, Computed: &Expr{Kind: ExprLiteral, Lit: "x"}},
		}},
		{ID: "doc", Name: "資料", TitleField: "title", Fields: []Field{
			{ID: "title", Name: "題名", Type: FieldString, Required: true},
			{ID: "body", Name: "本文", Type: FieldText},
			{ID: "pages", Name: "ページ", Type: FieldInteger},
			{ID: "done", Name: "読んだ", Type: FieldBoolean, Required: true},
			{ID: "added", Name: "追加日", Type: FieldDate, Required: true},
		}},
		{ID: "post", Name: "投稿", Scope: ScopePublic, Fields: []Field{{ID: "text", Name: "本文", Type: FieldText}}},
		{ID: "setting", Name: "設定", Single: true, Fields: []Field{{ID: "goal", Name: "目標", Type: FieldInteger}}},
	}
	return app
}

func TestFileImportValid(t *testing.T) {
	app := importApp()
	app.Screens[0].Components = []Component{
		{Type: CompFileImport, Entity: "member", Accept: []string{"csv"}},
		{Type: CompFileImport, Entity: "member", Accept: []string{"csv"}, Label: "名簿",
			Map: []ImportColumn{{Column: "氏名", Field: "name"}, {Column: "学年(数字)", Field: "grade"}}},
		{Type: CompFileImport, Entity: "doc", Accept: []string{"pdf", "text"}, Field: "body"},
		{Type: CompFileImport, Entity: "doc", Accept: []string{"csv", "pdf"}, Field: "body",
			Map: []ImportColumn{{Column: "本文", Field: "body"}}},
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("valid fileImport rejected: %v", err)
	}
}

func TestFileImportSemanticErrors(t *testing.T) {
	app := importApp()
	f := func(mod func(c *Component)) Component {
		c := Component{Type: CompFileImport, Entity: "member", Accept: []string{"csv"}}
		mod(&c)
		return c
	}
	app.Screens[0].Components = []Component{
		f(func(c *Component) { c.Entity = "nope" }),
		f(func(c *Component) { c.Entity = "post" }),
		f(func(c *Component) { c.Entity = "setting" }),
		f(func(c *Component) { c.Accept = nil }),
		f(func(c *Component) { c.Accept = []string{"xlsx"} }),
		f(func(c *Component) { c.Accept = []string{"csv", "csv"} }),
		f(func(c *Component) { c.Field = "name" }),
		f(func(c *Component) { c.Entity, c.Accept = "doc", []string{"pdf"} }),
		f(func(c *Component) { c.Entity, c.Accept, c.Field = "doc", []string{"text"}, "zzz" }),
		f(func(c *Component) { c.Entity, c.Accept, c.Field = "doc", []string{"text"}, "pages" }),
		f(func(c *Component) { c.Accept, c.Field = []string{"text"}, "label" }),
		f(func(c *Component) { c.Accept, c.Field = []string{"text"}, "grade" }),
		f(func(c *Component) {
			c.Entity, c.Accept, c.Field = "doc", []string{"pdf"}, "body"
			c.Map = []ImportColumn{{Column: "a", Field: "body"}}
		}),
		f(func(c *Component) { c.Map = []ImportColumn{{Column: "", Field: "photo"}} }),
		f(func(c *Component) { c.Map = []ImportColumn{{Column: "a", Field: "label"}, {Column: "a", Field: "xx"}} }),
		f(func(c *Component) { c.Map = []ImportColumn{{Column: "a", Field: "name"}, {Column: "b", Field: "name"}} }),
	}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		`fileImport entity "nope" does not exist`,
		`cannot add to public entity "post"`,
		`cannot add to single entity "setting"`,
		`fileImport needs accept`,
		`accept "xlsx" must be csv, text or pdf`,
		`accept "csv" is listed twice`,
		`fileImport field "name" is only for accept text / pdf`,
		`with accept text / pdf needs field`,
		`fileImport field "zzz" not in entity "doc"`,
		`field "pages" must be text`,
		`field "label" is computed`,
		`field "grade" must be text`,
		`map is only for accept csv`,
		`column is empty`,
		`field "photo" is photo; a CSV cell cannot hold it`,
		`column "a" is listed twice`,
		`field "label" is computed (read-only)`,
		`field "xx" not in entity "member"`,
		`field "name" is mapped twice`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

// text / pdf fill only the body field and the title (the file name); other
// required fields need a default (boolean / enum / date fill themselves).
func TestFileImportBodyRequiredFields(t *testing.T) {
	app := importApp()
	doc := app.EntityByID("doc")
	doc.Fields = append(doc.Fields, Field{ID: "author", Name: "作成者", Type: FieldString, Required: true})
	app.Screens[0].Components = []Component{{Type: CompFileImport, Entity: "doc", Accept: []string{"pdf"}, Field: "body"}}
	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), `required field "author" of entity "doc" would stay empty`) {
		t.Fatalf("want the required author problem, got %v", err)
	}
	if strings.Contains(err.Error(), `"title" of entity`) || strings.Contains(err.Error(), `"done" of entity`) {
		t.Fatalf("title / boolean must not be reported: %v", err)
	}
	doc.Fields[len(doc.Fields)-1].DefaultValue = "自分"
	if err := app.Validate(); err != nil {
		t.Fatalf("defaultValue should satisfy required: %v", err)
	}
}

func TestFileImportExample(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "file_import.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := Parse(raw)
	if err != nil {
		t.Fatalf("file_import.json: %v", err)
	}
	var csv, body bool
	for _, s := range app.Screens {
		for _, c := range s.Components {
			if c.Type != CompFileImport {
				continue
			}
			csv = csv || (c.Entity == "member" && len(c.Map) == 1)
			body = body || (c.Entity == "doc" && c.Field == "body")
		}
	}
	if !csv || !body {
		t.Fatalf("example lacks the csv (%v) or the pdf / text (%v) fileImport", csv, body)
	}
}
