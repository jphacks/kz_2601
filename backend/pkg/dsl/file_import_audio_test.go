package dsl

import (
	"strings"
	"testing"
)

func TestFileImportAudioValid(t *testing.T) {
	app := importApp()
	app.Screens[0].Components = []Component{
		{Type: CompFileImport, Entity: "doc", Accept: []string{"audio"}, Field: "body", Label: "録音ファイルを文字にする"},
		{Type: CompFileImport, Entity: "doc", Accept: []string{"pdf", "text", "audio"}, Field: "body"},
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("valid fileImport audio rejected: %v", err)
	}
}

func TestFileImportAudioErrors(t *testing.T) {
	app := importApp()
	app.Entities[1].Fields = append(app.Entities[1].Fields, Field{ID: "memo", Name: "メモ", Type: FieldString})
	app.Screens[0].Components = []Component{
		{Type: CompFileImport, Entity: "doc", Accept: []string{"audio"}},
		{Type: CompFileImport, Entity: "doc", Accept: []string{"audio"}, Field: "memo"},
		{Type: CompFileImport, Entity: "doc", Accept: []string{"audio", "audio"}, Field: "body"},
		{Type: CompFileImport, Entity: "post", Accept: []string{"audio"}, Field: "text"},
	}
	err := app.Validate()
	if err == nil {
		t.Fatal("want errors")
	}
	for _, want := range []string{
		`with accept text / pdf / audio needs field`,
		`accept audio puts a transcript of up to 30 minutes into field "memo"; make it type text`,
		`accept "audio" is listed twice`,
		`cannot add to public entity "post"`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}
}
