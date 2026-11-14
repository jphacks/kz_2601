package expreval

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// casesDir holds the shared cases. iOS writes the expected values
// (EXPR_CONFORMANCE_UPDATE=1 swift test --filter ExprConformanceTests).
const casesDir = "../../../../docs/dsl/conformance/expr"

type caseRow struct {
	Entity string `json:"entity"`
	Record Record `json:"record"`
}

type expected struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
	Text  string `json:"text"`
}

type conformanceCase struct {
	Name     string              `json:"name"`
	Expr     dsl.Expr            `json:"expr"`
	Now      string              `json:"now"`
	TimeZone string              `json:"timeZone"`
	Row      *caseRow            `json:"row"`
	Entities []dsl.Entity        `json:"entities"`
	Records  map[string][]Record `json:"records"`
	Expected *expected           `json:"expected"`
}

type caseFile struct {
	Defaults struct {
		Now      string              `json:"now"`
		TimeZone string              `json:"timeZone"`
		Entities []dsl.Entity        `json:"entities"`
		Records  map[string][]Record `json:"records"`
	} `json:"defaults"`
	Cases []conformanceCase `json:"cases"`
}

func TestConformance(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(casesDir, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no case files in %s (%v)", casesDir, err)
	}
	sort.Strings(files)
	total := 0
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var f caseFile
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		for _, c := range f.Cases {
			total++
			t.Run(filepath.Base(path)+"/"+c.Name, func(t *testing.T) {
				runCase(t, c, f)
			})
		}
	}
	t.Logf("%d cases in %d files", total, len(files))
}

func runCase(t *testing.T, c conformanceCase, f caseFile) {
	t.Helper()
	tzName, nowStr := pick(c.TimeZone, f.Defaults.TimeZone), pick(c.Now, f.Defaults.Now)
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		t.Fatal(err)
	}
	now, err := time.Parse(time.RFC3339, nowStr)
	if err != nil {
		t.Fatal(err)
	}
	entities := c.Entities
	if entities == nil {
		entities = f.Defaults.Entities
	}
	records := c.Records
	if records == nil {
		records = f.Defaults.Records
	}
	env := Env{Entities: entities, Now: now, Location: loc, Source: SourceFunc(func(e string) []Record { return records[e] })}
	ev := New(env)
	var row *Row
	if c.Row != nil {
		row = &Row{Entity: ev.entities[c.Row.Entity], Record: c.Row.Record}
	}
	got := ev.Eval(c.Expr, row)
	want := c.Expected
	if want == nil {
		t.Fatal("expected is missing (generate it from iOS)")
	}
	if got.Kind.String() != want.Type {
		t.Fatalf("type: got %s %+v, want %s %v", got.Kind, got, want.Type, want.Value)
	}
	if got.IsNil() {
		return
	}
	if text := got.Text(loc); text != want.Text {
		t.Errorf("text: got %q, want %q", text, want.Text)
	}
	switch got.Kind {
	case Number:
		if !sameNumber(got.Num, want.Value) {
			t.Errorf("number: got %v, want %v", got.Num, want.Value)
		}
	case String:
		if got.Str != want.Value {
			t.Errorf("string: got %q, want %q", got.Str, want.Value)
		}
	case Bool:
		if got.Bool != want.Value {
			t.Errorf("bool: got %v, want %v", got.Bool, want.Value)
		}
	case Date:
		s, _ := want.Value.(string)
		w, err := time.Parse(time.RFC3339, s)
		// expected dates have whole seconds (fractional seconds are dropped)
		if err != nil || !got.Time.Truncate(time.Second).Equal(w) {
			t.Errorf("date: got %s, want %s", got.Time.In(loc).Format(time.RFC3339), s)
		}
	}
}

func pick(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func sameNumber(got float64, want any) bool {
	switch w := want.(type) {
	case float64:
		return got == w
	case string:
		switch w {
		case "inf":
			return math.IsInf(got, 1)
		case "-inf":
			return math.IsInf(got, -1)
		case "nan":
			return math.IsNaN(got)
		}
	}
	return false
}

// TestConformanceCoverage: every op of dsl.Ops, every agg kind and every filter op
// appears in at least one shared case.
func TestConformanceCoverage(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(casesDir, "*.json"))
	seen := map[string]bool{}
	var walk func(e dsl.Expr)
	walk = func(e dsl.Expr) {
		switch e.Kind {
		case dsl.ExprOp:
			seen["op:"+e.Op] = true
			for _, a := range e.Args {
				walk(a)
			}
		case dsl.ExprAgg:
			seen["agg:"+e.Agg.Agg] = true
			for _, f := range e.Agg.Filter {
				seen["filter:"+f.Op] = true
			}
		case dsl.ExprIf:
			seen["if"] = true
			walk(*e.If)
			walk(*e.Then)
			walk(*e.Else)
		case dsl.ExprRef:
			seen["ref"] = true
		case dsl.ExprSetting:
			seen["setting"] = true
		}
	}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var f caseFile
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatal(err)
		}
		for _, c := range f.Cases {
			walk(c.Expr)
		}
	}
	want := []string{"if", "ref", "setting"}
	for op := range dsl.Ops {
		want = append(want, "op:"+op)
	}
	for _, a := range []string{"count", "sum", "avg", "min", "max", "latest", "earliest"} {
		want = append(want, "agg:"+a)
	}
	for _, f := range dsl.FilterOps {
		want = append(want, "filter:"+f)
	}
	for _, w := range want {
		if !seen[w] {
			t.Errorf("no shared case covers %s", w)
		}
	}
}
