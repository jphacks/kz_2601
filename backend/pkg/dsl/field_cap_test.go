package dsl

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// appWithFields: one entity with n fields; the last one is computed, so the
// cap counts computed fields too (#221).
func appWithFields(n int) App {
	fs := make([]Field, 0, n)
	for i := 0; i < n-1; i++ {
		fs = append(fs, Field{ID: fmt.Sprintf("f%d", i), Name: fmt.Sprintf("項目%d", i), Type: FieldNumber})
	}
	fs = append(fs, Field{ID: "total", Name: "合計", Type: FieldNumber,
		Computed: &Expr{Kind: ExprOp, Op: "add", Args: []Expr{{Kind: ExprRef, Ref: "f0"}, {Kind: ExprLiteral, Lit: 1.0}}}})
	app := Initial()
	app.Entities = []Entity{{ID: "checkup", Name: "健診", Fields: fs}}
	return app
}

func TestFieldCap(t *testing.T) {
	if MaxFieldsPerEntity != 40 {
		t.Fatalf("MaxFieldsPerEntity = %d, want 40 (#221)", MaxFieldsPerEntity)
	}
	for _, n := range []int{21, 40} {
		app := appWithFields(n)
		if err := app.Validate(); err != nil {
			t.Errorf("%d fields must pass: %v", n, err)
		}
	}
	app := appWithFields(41)
	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), "too many fields (41 > 40)") {
		t.Fatalf("41 fields must be rejected, got %v", err)
	}
}

// A 40-field entity must stay far inside MaxDSLBytes so the cap is reachable.
func TestFieldCapFitsDSLBytes(t *testing.T) {
	app := appWithFields(MaxFieldsPerEntity)
	raw, err := json.Marshal(app)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > MaxDSLBytes/4 {
		t.Fatalf("40-field entity is %d bytes, too close to %d", len(raw), MaxDSLBytes)
	}
}
