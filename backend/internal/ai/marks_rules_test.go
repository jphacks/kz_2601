package ai

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #255: likes / saves are entity.marks, not child entities.
func TestPromptTeachesMarks(t *testing.T) {
	for _, want := range []string{
		`"mark":"like"`,
		"別 entity、親を指す ref、件数の computed(agg count)",
		`{"field":"$marks.save","op":"eq","value":{"me":true}}`,
		`"$marks.<id>"`,
		"### Entity の marks",
		"bookmark=保存・あとで見る",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
}

func TestMarksCatalogFromSchema(t *testing.T) {
	out, err := buildMarksCatalog(dsl.SchemaJSON())
	if err != nil {
		t.Fatal(err)
	}
	for _, icon := range dsl.MarkIcons {
		if !strings.Contains(out, icon+"=") {
			t.Errorf("catalog lacks icon %q: %s", icon, out)
		}
	}
	if !strings.Contains(out, "mark{id, icon(heart|star") {
		t.Errorf("catalog lacks mark keys: %s", out)
	}
	if _, err := buildMarksCatalog([]byte(`{"$defs":{}}`)); err == nil {
		t.Fatal("schema without marks must fail")
	}
}

// The chat answers "can it do likes?" from the capability index, which lists
// the record values (incl. $marks.<id>) from the same schema.
func TestCapabilityIndexKnowsMarks(t *testing.T) {
	if !strings.Contains(chatCapabilityIndex, "$marks.<id>") {
		t.Fatalf("capability index lacks $marks: %s", chatCapabilityIndex)
	}
}
