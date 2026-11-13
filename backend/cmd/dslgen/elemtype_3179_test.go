package main

import "testing"

func TestElemType_3179(t *testing.T) {
	t.Parallel()
	list := func(e *shape) *shape { return &shape{kind: kList, elem: e} }
	tests := []struct {
		name string
		in   *shape
		want string
	}{
		{"文字", &shape{kind: kString}, "String"},
		{"整数", &shape{kind: kInt}, "Int"},
		{"数", &shape{kind: kNumber}, "Double"},
		{"真偽", &shape{kind: kBool}, "Boolean"},
		{"enum", &shape{kind: kEnum, enum: &enumOcc{name: "Mode"}}, "Mode"},
		{"class", &shape{kind: kClass, class: "Meta"}, "Meta"},
		{"sealed", &shape{kind: kSealed, class: "Expr"}, "Expr"},
		{"component", &shape{kind: kComponent}, "Component"},
		{"componentType", &shape{kind: kComponentType}, "ComponentType"},
		{"JsonElement", &shape{kind: kJSON}, "JsonElement"},
		{"JsonObject", &shape{kind: kJSONObject}, "JsonObject"},
		{"入れ子の配列", list(&shape{kind: kString}), "List<String>"},
		{"入れ子の入れ子", list(list(&shape{kind: kInt})), "List<List<Int>>"},
		{"配列の class", list(&shape{kind: kClass, class: "Row"}), "List<Row>"},
	}
	g := &generator{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := g.elemType(tc.in); got != tc.want {
				t.Errorf("elemType = %q, want %q", got, tc.want)
			}
		})
	}
}

// スキーマの小さな断片から、配列の中身の Kotlin の型が決まる。
func TestElemTypeFromSchema_3179(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		items string // 配列の定義(JSON)
		want  string
	}{
		{"参照(class)", `{"type":"array","items":{"$ref":"#/$defs/Meta"}}`, "Meta"},
		{"参照(component)", `{"type":"array","items":{"$ref":"#/$defs/Slider"}}`, "Component"},
		{"参照(sealed)", `{"type":"array","items":{"$ref":"#/$defs/Expr"}}`, "Expr"},
		{"文字", `{"type":"array","items":{"type":"string"}}`, "String"},
		{"数", `{"type":"array","items":{"type":"number"}}`, "Double"},
		{"入れ子の配列", `{"type":"array","items":{"type":"array","items":{"type":"integer"}}}`, "List<Int>"},
		{"3 重の配列", `{"type":"array","items":{"type":"array","items":{"type":"array","items":{"type":"boolean"}}}}`, "List<List<Boolean>>"},
		{"型の無い配列", `{"type":"array"}`, "JsonElement"},
		{"型の無い中身", `{"type":"array","items":{}}`, "JsonElement"},
		{"properties の無い object", `{"type":"array","items":{"type":"object"}}`, "JsonObject"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := defsGenerator_3179(t, refShapeDefs_3179)
			p, err := parseOrdered([]byte(tc.items))
			if err != nil {
				t.Fatal(err)
			}
			s, err := g.shapeOf("Owner", "key", p)
			if err != nil {
				t.Fatal(err)
			}
			if s.kind != kList {
				t.Fatalf("kind = %d, want list", s.kind)
			}
			if got := g.elemType(s.elem); got != tc.want {
				t.Errorf("elemType = %q, want %q", got, tc.want)
			}
		})
	}
}
