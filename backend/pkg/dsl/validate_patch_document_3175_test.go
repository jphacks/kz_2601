package dsl

import (
	"errors"
	"slices"
	"testing"
)

func TestValidatePatchDocument_3175(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		raw       string
		wantCodes []string // nil = 通る
	}{
		{"JSON でない", `{`, []string{CodeInvalidJSON}},
		{"空", ``, []string{CodeInvalidJSON}},
		{"スキーマに合わない(必須が無い)", `{}`, []string{"schema_required"}},
		{"スキーマに合わない(型が違う)", `{"patch":"x","summary":"s"}`, []string{"schema_type"}},
		{"正しい patch", `{"patch":[{"op":"add","path":"/a","value":1}],"summary":"足した"}`, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidatePatchDocument([]byte(tc.raw))
			if tc.wantCodes == nil {
				if err != nil {
					t.Fatalf("ValidatePatchDocument() = %v, want nil", err)
				}
				return
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("ValidatePatchDocument() = %v, want *ValidationError", err)
			}
			if got := ve.Codes(); !slices.Contains(got, tc.wantCodes[0]) {
				t.Errorf("codes = %v, want to contain %v (problems %v)", got, tc.wantCodes[0], ve.Problems)
			}
		})
	}
}
