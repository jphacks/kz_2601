package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// countWhereApp builds days/answers with one computed on `on` set to expr.
func countWhereApp(t *testing.T, on, expr string) *dsl.App {
	t.Helper()
	days := `{"id":"date","name":"日付","type":"date"}`
	answers := `{"id":"day","name":"候補日","type":"ref","refEntity":"days"},
		{"id":"status","name":"参加","type":"enum","options":["参加できる","未定"]}`
	c := `{"id":"n","name":"人数","type":"integer","computed":` + expr + `}`
	if on == "days" {
		days += "," + c
	} else {
		answers += "," + c
	}
	a := &dsl.App{}
	raw := `[{"id":"days","name":"候補日","fields":[` + days + `]},
		{"id":"answers","name":"回答","fields":[` + answers + `]}]`
	if err := json.Unmarshal([]byte(raw), &a.Entities); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestCheckCountWhereExample(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "dsl", "examples", "schedule_poll.json"))
	if err != nil {
		t.Fatal(err)
	}
	a, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := countWhereFields(a), []string{"days.ok_count", "days.maybe_count"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("countWhereFields = %v, want %v", got, want)
	}
	if miss := checkCountWhere(map[string]any{"countWhere": true}, a); miss != nil {
		t.Fatalf("schedule_poll: %v", miss)
	}
	if miss := checkCountWhere(map[string]any{"countWhere": false}, a); miss == nil {
		t.Fatal("countWhere=false should miss on schedule_poll")
	}
	if miss := checkCountWhere(map[string]any{}, a); miss != nil {
		t.Fatalf("no expect key: %v", miss)
	}
}

func TestCheckCountWhere(t *testing.T) {
	const rowRef = `{"field":"day","op":"eq","value":{"ref":"id"}}`
	const byValue = `{"field":"status","op":"eq","value":"参加できる"}`
	for _, tc := range []struct {
		name, on, expr string
		want           bool
	}{
		{"row ref and value", "days", `{"agg":"count","entity":"answers","filter":[` + rowRef + `,` + byValue + `]}`, true},
		{"parent only", "days", `{"agg":"count","entity":"answers","filter":[` + rowRef + `]}`, false},
		{"single non-array filter", "days", `{"agg":"count","entity":"answers","filter":` + rowRef + `}`, false},
		{"value only", "days", `{"agg":"count","entity":"answers","filter":[` + byValue + `]}`, false},
		{"not count", "days", `{"agg":"latest","entity":"answers","field":"status","filter":[` + rowRef + `,` + byValue + `]}`, false},
		{"nested in op", "days", `{"op":"add","args":[{"agg":"count","entity":"answers","filter":[` + rowRef + `,` + byValue + `]},1]}`, true},
		{"on the other entity", "answers", `{"agg":"count","entity":"days","filter":[{"field":"id","op":"eq","value":{"ref":"day"}},{"field":"date","op":"today","value":null}]}`, false},
		// The row's id must be matched by a ref to this entity, not any field.
		{"row id on a non-ref field", "days", `{"agg":"count","entity":"answers","filter":[{"field":"status","op":"eq","value":{"ref":"id"}},` + byValue + `]}`, false},
		{"self count by id", "days", `{"agg":"count","entity":"days","filter":[{"field":"id","op":"eq","value":{"ref":"id"}},{"field":"date","op":"eq","value":"2026-11-08"}]}`, false},
		{"ref to another entity", "answers", `{"agg":"count","entity":"answers","filter":[` + rowRef + `,` + byValue + `]}`, false},
		// A date window or a value on the parent field is not "children with a given value".
		{"row ref and date window", "days", `{"agg":"count","entity":"answers","filter":[` + rowRef + `,{"field":"status","op":"thisWeek","value":""}]}`, false},
		{"value on the row-ref field only", "days", `{"agg":"count","entity":"answers","filter":[` + rowRef + `,{"field":"day","op":"neq","value":""}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := countWhereApp(t, tc.on, tc.expr)
			if got := len(countWhereFields(a)) > 0; got != tc.want {
				t.Fatalf("countWhereFields = %v, want found=%v", countWhereFields(a), tc.want)
			}
			if miss := checkCountWhere(map[string]any{"countWhere": tc.want}, a); miss != nil {
				t.Fatalf("countWhere=%v: %v", tc.want, miss)
			}
		})
	}

	// Entities in either order: the parent may come after its children.
	a := countWhereApp(t, "days", `{"agg":"count","entity":"answers","filter":[`+rowRef+`,`+byValue+`]}`)
	a.Entities[0], a.Entities[1] = a.Entities[1], a.Entities[0]
	if got := countWhereFields(a); !reflect.DeepEqual(got, []string{"days.n"}) {
		t.Fatalf("countWhereFields = %v", got)
	}
}
