package dsl

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

// Diff は細かい差を取らず、全体を 1 つの replace にする。
func TestDiff(t *testing.T) {
	from := mustJSON(t, Initial())

	if ops, err := Diff(from, slices.Clone(from)); err != nil || ops != nil {
		t.Fatalf("同じ中身なら差は無い: %v, %v", ops, err)
	}
	if ops, err := Diff(nil, nil); err != nil || ops != nil {
		t.Fatalf("空どうし: %v, %v", ops, err)
	}

	changed := bytes.Replace(from, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1 `), 1)
	if bytes.Equal(from, changed) {
		t.Fatal("テストの前提: 変える所が見つからない")
	}
	ops, err := Diff(from, changed)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 {
		t.Fatalf("ルートの 1 つだけにする: %d 件", len(ops))
	}
	if ops[0].Op != "replace" || ops[0].Path != "" || ops[0].From != "" {
		t.Errorf("op = %+v", ops[0])
	}
	if !bytes.Equal(ops[0].Value, changed) {
		t.Errorf("value は新しい全体: %s", ops[0].Value)
	}
}

// 片方が空でも、中身が違えば replace 1 つ。
func TestDiffWithEmptySide(t *testing.T) {
	doc := mustJSON(t, Initial())
	for _, c := range []struct {
		name     string
		from, to []byte
	}{
		{"空から", nil, doc},
		{"空へ", doc, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			ops, err := Diff(c.from, c.to)
			if err != nil {
				t.Fatal(err)
			}
			if len(ops) != 1 || ops[0].Op != "replace" || ops[0].Path != "" {
				t.Fatalf("ops = %+v", ops)
			}
			if !bytes.Equal(ops[0].Value, c.to) {
				t.Errorf("value = %s", ops[0].Value)
			}
		})
	}
}

// Diff の replace は RFC 6902 として当たり、元の文書を新しい文書にする。
// ただし値は文書の全体なので $defs/PatchValue(アプリの部品だけ)には収まらない。
// Diff は版の記録・要約のためで、Claude に渡す patch ではないため、これでよい。
func TestDiffOpsApplyButAreNotAPatchValue(t *testing.T) {
	from := mustJSON(t, Initial())
	var app App
	if err := json.Unmarshal(from, &app); err != nil {
		t.Fatal(err)
	}
	app.App.Name = "差分の実験"
	to := mustJSON(t, &app)

	ops, err := Diff(from, to)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePatchDocument(mustJSON(t, PatchDocument{Patch: ops, Summary: "全体を置き換えました"})); err == nil {
		t.Error("文書の全体は PatchValue ではない(Claude に渡す形ではない)")
	}
	next, got, err := ApplyPatch(from, ops)
	if err != nil {
		t.Fatalf("ルートの replace が当たらない: %v", err)
	}
	if got.App.Name != "差分の実験" {
		t.Errorf("name = %q", got.App.Name)
	}
	if !bytes.Equal(next, to) {
		t.Errorf("当てた結果が to と違う: %s", next)
	}
	if _, err := Parse(next); err != nil {
		t.Fatal(err)
	}
}

// PatchSchemaJSON は本体のスキーマから作った、root = PatchDocument の JSON。
func TestPatchSchemaJSON(t *testing.T) {
	raw := PatchSchemaJSON()
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("正しい JSON でない: %v", err)
	}
	if got["$ref"] != "#/$defs/PatchDocument" {
		t.Errorf("$ref = %v", got["$ref"])
	}
	if got["title"] != "AppDSLPatch" {
		t.Errorf("title = %v", got["title"])
	}
	if got["$id"] != "https://github.com/jphacks/kz_2601/docs/dsl/dsl-patch.schema.json" {
		t.Errorf("$id = %v", got["$id"])
	}
	if _, ok := got["properties"]; ok {
		t.Error("root は $ref だけにする(properties を持たない)")
	}

	var root map[string]any
	if err := json.Unmarshal(SchemaJSON(), &root); err != nil {
		t.Fatal(err)
	}
	if got["$schema"] != root["$schema"] {
		t.Errorf("$schema = %v", got["$schema"])
	}
	if !reflect.DeepEqual(got["$defs"], root["$defs"]) {
		t.Error("$defs は本体のスキーマと同じ物を使う")
	}
	for _, def := range []string{"PatchDocument", "PatchOp", "PatchValue"} {
		if _, ok := got["$defs"].(map[string]any)[def]; !ok {
			t.Errorf("$defs に %s が無い", def)
		}
	}
	if !bytes.Equal(raw, PatchSchemaJSON()) {
		t.Error("毎回同じ物を返す")
	}
}

// スキーマの op の種類が、PatchOp(RFC 6902)の 6 つと合う。
func TestPatchSchemaJSONOpsMatchPatchOp(t *testing.T) {
	var s map[string]any
	if err := json.Unmarshal(PatchSchemaJSON(), &s); err != nil {
		t.Fatal(err)
	}
	patchOp, ok := s["$defs"].(map[string]any)["PatchOp"].(map[string]any)
	if !ok {
		t.Fatal("$defs/PatchOp が無い")
	}
	var ops []string
	for _, v := range patchOp["properties"].(map[string]any)["op"].(map[string]any)["enum"].([]any) {
		ops = append(ops, v.(string))
	}
	want := []string{"add", "copy", "move", "remove", "replace", "test"}
	sorted := slices.Clone(ops)
	slices.Sort(sorted)
	if !slices.Equal(sorted, want) {
		t.Fatalf("op の種類 = %v", ops)
	}
	for _, op := range ops {
		doc := PatchDocument{Patch: []PatchOp{{Op: op, Path: "/app/name", From: "/app/name", Value: json.RawMessage(`"x"`)}}, Summary: "試し"}
		if err := ValidatePatchDocument(mustJSON(t, doc)); err != nil {
			t.Errorf("op %q がスキーマを通らない: %v", op, err)
		}
	}
	bad := PatchDocument{Patch: []PatchOp{{Op: "upsert", Path: "/app/name"}}, Summary: "試し"}
	if err := ValidatePatchDocument(mustJSON(t, bad)); err == nil {
		t.Error("知らない op は落とす")
	}
}
