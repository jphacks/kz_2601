package dsl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Edit turn: 家計簿 v1 -> 「店名フィールドを足して、毎晩20時に記録を促して」
func TestApplyPatchKakeibo(t *testing.T) {
	current, err := os.ReadFile(filepath.Join(docsDir, "examples", "kakeibo.json"))
	if err != nil {
		t.Skip(err)
	}
	var doc PatchDocument
	if err := json.Unmarshal([]byte(`{
	  "summary": "店名を記録できるようにし、毎晩20時のリマインドを追加しました",
	  "patch": [
	    {"op":"add","path":"/entities/0/fields/-","value":{"id":"shop","name":"店名","type":"string"}},
	    {"op":"add","path":"/screens/1/components/0/children/-","value":{"type":"textField","field":"shop","label":"店名"}},
	    {"op":"replace","path":"/screens/0/components/2/children/0/item/subtitle","value":"shop"},
	    {"op":"add","path":"/app/notifications","value":[{"id":"nightly","title":"家計簿","body":"今日の支出を記録しましょう","repeat":"daily","time":"20:00"}]}
	  ]}`), &doc); err != nil {
		t.Fatal(err)
	}
	// the PatchDocument itself must satisfy $defs/PatchDocument
	if err := ValidatePatchDocument(mustJSON(t, doc)); err != nil {
		t.Fatalf("patch document schema: %v", err)
	}
	next, app, err := ApplyPatch(current, doc.Patch)
	if err != nil {
		t.Fatal(err)
	}
	if app.Entities[0].FieldByID("shop") == nil {
		t.Fatal("shop field missing after patch")
	}
	if len(app.App.Notifications) != 1 || app.App.Notifications[0].Time != "20:00" {
		t.Fatalf("notification not applied: %+v", app.App.Notifications)
	}
	if _, err := Parse(next); err != nil {
		t.Fatal(err)
	}
}

func TestApplyPatchRejectsBrokenReference(t *testing.T) {
	current := mustJSON(t, Initial())
	_, _, err := ApplyPatch(current, []PatchOp{
		{Op: "replace", Path: "/initialScreen", Value: json.RawMessage(`"nope"`)},
	})
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	_, _, err = ApplyPatch(current, []PatchOp{{Op: "remove", Path: "/screens/5"}})
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
