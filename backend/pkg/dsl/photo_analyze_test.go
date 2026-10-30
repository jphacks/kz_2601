package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func photoAnalyzeApp(analyze ...PhotoAnalyze) *App {
	app := Initial()
	app.Entities = []Entity{{ID: "expense", Name: "支出", Fields: []Field{
		{ID: "receipt", Name: "レシート", Type: FieldPhoto},
		{ID: "amount", Name: "金額", Type: FieldCurrency},
		{ID: "date", Name: "日付", Type: FieldDate},
		{ID: "category", Name: "分類", Type: FieldEnum, Options: []string{"食費", "日用品"}},
		{ID: "memo", Name: "メモ", Type: FieldText},
		{ID: "tag", Name: "タグ", Type: FieldString},
		{ID: "angle", Name: "角度", Type: FieldNumber},
		{ID: "done", Name: "済み", Type: FieldBoolean},
		{ID: "twice", Name: "2倍", Type: FieldNumber, Computed: &Expr{Kind: ExprRef, Ref: "amount"}},
	}}}
	app.Screens = append(app.Screens, Screen{ID: "form", Title: "f", Entity: "expense", Components: []Component{
		{Type: CompPhotoField, Field: "receipt", Analyze: analyze},
	}})
	return &app
}

func TestPhotoAnalyzeValid(t *testing.T) {
	app := photoAnalyzeApp(
		PhotoAnalyze{Kind: "amount", Into: "amount"},
		PhotoAnalyze{Kind: "date", Into: "date"},
		PhotoAnalyze{Kind: "classify", Into: "category"},
		PhotoAnalyze{Kind: "text", Into: "memo"},
		PhotoAnalyze{Kind: "classify", Into: "tag", Options: []string{"犬", "猫"}},
		PhotoAnalyze{Kind: "jointAngle", Into: "angle", Joint: "leftKnee"},
	)
	if err := app.Validate(); err != nil {
		t.Fatalf("valid analyze rejected: %v", err)
	}
}

func TestPhotoAnalyzeSemanticErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		a    PhotoAnalyze
		want string
	}{
		"missing into":        {PhotoAnalyze{Kind: "text", Into: "nope"}, `into "nope" not in entity`},
		"photo itself":        {PhotoAnalyze{Kind: "text", Into: "receipt"}, "another field than the photo field"},
		"computed":            {PhotoAnalyze{Kind: "amount", Into: "twice"}, "computed (read-only)"},
		"amount into text":    {PhotoAnalyze{Kind: "amount", Into: "memo"}, "must be currency|integer|number"},
		"date into boolean":   {PhotoAnalyze{Kind: "date", Into: "done"}, "must be date|datetime"},
		"angle without joint": {PhotoAnalyze{Kind: "jointAngle", Into: "angle"}, "jointAngle needs joint"},
		"joint on text":       {PhotoAnalyze{Kind: "text", Into: "memo", Joint: "knee"}, "joint is only for kind jointAngle"},
		"options on text":     {PhotoAnalyze{Kind: "text", Into: "memo", Options: []string{"a", "b"}}, "options are only for kind classify"},
		"classify string":     {PhotoAnalyze{Kind: "classify", Into: "tag"}, "needs options (2+)"},
		"classify bad option": {PhotoAnalyze{Kind: "classify", Into: "category", Options: []string{"食費", "車"}}, `option "車" is not an option`},
		"unknown kind":        {PhotoAnalyze{Kind: "barcode", Into: "memo"}, `kind "barcode" must be one of`},
	} {
		t.Run(name, func(t *testing.T) {
			err := photoAnalyzeApp(tc.a).Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

func TestPhotoAnalyzeDuplicateInto(t *testing.T) {
	err := photoAnalyzeApp(PhotoAnalyze{Kind: "text", Into: "memo"}, PhotoAnalyze{Kind: "describe", Into: "memo"}).Validate()
	if err == nil || !strings.Contains(err.Error(), "already used by another analyze entry") {
		t.Fatalf("duplicate into should fail: %v", err)
	}
}

// The schema keeps analyze on photoField only and requires kind + into.
func TestPhotoAnalyzeSchema(t *testing.T) {
	base := `{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"f",
"entities":[{"id":"e","name":"e","fields":[{"id":"p","name":"p","type":"photo"},{"id":"m","name":"m","type":"text"}]}],
"screens":[{"id":"f","title":"f","entity":"e","components":[%s]}]}`
	for name, comp := range map[string]string{
		"no into":        `{"type":"photoField","field":"p","analyze":[{"kind":"text"}]}`,
		"bad joint":      `{"type":"photoField","field":"p","analyze":[{"kind":"jointAngle","into":"m","joint":"neck"}]}`,
		"on textField":   `{"type":"textField","field":"m","analyze":[{"kind":"text","into":"m"}]}`,
		"empty analyze":  `{"type":"photoField","field":"p","analyze":[]}`,
		"unknown kind":   `{"type":"photoField","field":"p","analyze":[{"kind":"qr","into":"m"}]}`,
		"extra property": `{"type":"photoField","field":"p","analyze":[{"kind":"text","into":"m","lang":"ja"}]}`,
	} {
		if _, err := Parse([]byte(strings.Replace(base, "%s", comp, 1))); err == nil {
			t.Errorf("%s: should fail schema validation", name)
		}
	}
	ok := `{"type":"photoField","field":"p","analyze":[{"kind":"text","into":"m"}]}`
	if _, err := Parse([]byte(strings.Replace(base, "%s", ok, 1))); err != nil {
		t.Fatalf("valid analyze rejected: %v", err)
	}
}

// The bundled examples use each kind against a real form.
func TestPhotoAnalyzeExamples(t *testing.T) {
	kinds := map[string]bool{}
	for _, name := range []string{"receipt_ai.json", "form_check.json"} {
		raw, err := os.ReadFile(filepath.Join(docsDir, "examples", name))
		if err != nil {
			t.Fatal(err)
		}
		app, err := Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, s := range app.Screens {
			for _, c := range s.Components {
				for _, a := range c.Analyze {
					kinds[a.Kind] = true
				}
			}
		}
	}
	for _, k := range []string{"amount", "date", "classify", "text", "jointAngle"} {
		if !kinds[k] {
			t.Errorf("examples lack analyze kind %q", k)
		}
	}
}
