package runner

import (
	"errors"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestSameValueTable2588(t *testing.T) {
	dt := &dsl.Field{ID: "at", Type: dsl.FieldDatetime}
	str := &dsl.Field{ID: "s", Type: dsl.FieldString}
	cases := []struct {
		name     string
		f        *dsl.Field
		old, v   any
		wantSame bool
	}{
		{name: "int64 and float64 same", f: nil, old: int64(3), v: float64(3), wantSame: true},
		{name: "float64 and int64 same", f: nil, old: float64(1200), v: int64(1200), wantSame: true},
		{name: "int and int64 same", f: nil, old: 5, v: int64(5), wantSame: true},
		{name: "int64 and float64 differ by fraction", f: nil, old: int64(3), v: 3.5},
		{name: "number and numeric string", f: nil, old: int64(3), v: "3"},
		{name: "number and nil", f: nil, old: float64(0), v: nil},
		{name: "datetime same instant other zone", f: dt, old: "2026-01-02T09:00:00+09:00", v: "2026-01-02T00:00:00Z", wantSame: true},
		{name: "datetime other instant", f: dt, old: "2026-01-02T09:00:00+09:00", v: "2026-01-02T09:00:00Z"},
		{name: "datetime unparsable falls back to text", f: dt, old: "yesterday", v: "yesterday", wantSame: true},
		{name: "datetime one side unparsable", f: dt, old: "2026-01-02T00:00:00Z", v: "2026-01-02"},
		{name: "same instant on a string field is text", f: str, old: "2026-01-02T09:00:00+09:00", v: "2026-01-02T00:00:00Z"},
		{name: "same text", f: str, old: "りんご", v: "りんご", wantSame: true},
		{name: "other text", f: str, old: "りんご", v: "みかん"},
		{name: "text and number", f: str, old: "3", v: int64(3)},
		{name: "same bool", f: nil, old: true, v: true, wantSame: true},
		{name: "other bool", f: nil, old: true, v: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameValue(tc.f, tc.old, tc.v); got != tc.wantSame {
				t.Fatalf("sameValue(%v, %v) = %v, want %v", tc.old, tc.v, got, tc.wantSame)
			}
		})
	}
}

func TestImportNumberTable2588(t *testing.T) {
	cases := []struct {
		name   string
		v      any
		want   float64
		wantOK bool
	}{
		{name: "int64", v: int64(1200), want: 1200, wantOK: true},
		{name: "int", v: 300, want: 300, wantOK: true},
		{name: "float64", v: 3.5, want: 3.5, wantOK: true},
		{name: "negative", v: int64(-7), want: -7, wantOK: true},
		// text is read by importValue, not here
		{name: "comma text", v: "1,200"},
		{name: "yen text", v: "¥300"},
		{name: "decimal text", v: "3.5"},
		{name: "empty text", v: ""},
		{name: "nil", v: nil},
		{name: "bool", v: true},
		{name: "float32", v: float32(1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := importNumber(tc.v)
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("importNumber(%#v) = %v, %v; want %v, %v", tc.v, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestImportableTable2588(t *testing.T) {
	want := map[dsl.FieldType]bool{
		dsl.FieldString: true, dsl.FieldText: true, dsl.FieldInteger: true, dsl.FieldNumber: true,
		dsl.FieldCurrency: true, dsl.FieldBoolean: true, dsl.FieldDate: true, dsl.FieldDatetime: true,
		dsl.FieldEnum:  true,
		dsl.FieldPhoto: false, dsl.FieldVideo: false, dsl.FieldLocation: false, dsl.FieldRef: false,
		dsl.FieldRefs: false, "": false, "unknown": false,
	}
	for ft, ok := range want {
		if got := importable(ft); got != ok {
			t.Errorf("importable(%q) = %v, want %v", ft, got, ok)
		}
	}
}

// A computed field is never an import target, by explicit field map or by header.
func TestCheckImportComputedField2588(t *testing.T) {
	app := &dsl.App{Entities: []dsl.Entity{{ID: "item", Fields: []dsl.Field{
		{ID: "name", Name: "名前", Type: dsl.FieldString},
		{ID: "total", Name: "合計", Type: dsl.FieldNumber, Computed: &dsl.Expr{}},
	}}}}
	spec := SheetImport{SpreadsheetID: "sheet", Range: "A1:C9", IDColumn: "id", Entity: "item"}

	_, lookup, err := checkImport(app, spec)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"total", "合計"} {
		if f := lookup(h); f != nil {
			t.Errorf("header %q maps to computed field %q", h, f.ID)
		}
	}
	if f := lookup("名前"); f == nil || f.ID != "name" {
		t.Errorf("header 名前 = %v", f)
	}

	spec.Fields = map[string]string{"合計": "total"}
	if _, _, err := checkImport(app, spec); !errors.Is(err, ErrImportSpec) {
		t.Fatalf("explicit computed field: err = %v, want ErrImportSpec", err)
	}
}

func TestCellTable2588(t *testing.T) {
	row := []string{" a ", "", "\tb\n", "c"}
	cases := []struct {
		i    int
		want string
	}{
		{0, "a"},
		{1, ""},
		{2, "b"},
		{3, "c"},
		{4, ""},   // just past the end
		{100, ""}, // far out of range
	}
	for _, tc := range cases {
		if got := cell(row, tc.i); got != tc.want {
			t.Errorf("cell(row, %d) = %q, want %q", tc.i, got, tc.want)
		}
	}
	if got := cell(nil, 0); got != "" {
		t.Errorf("cell(nil, 0) = %q", got)
	}
}
