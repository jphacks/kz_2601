package engine

import (
	"reflect"
	"testing"
)

func TestRefIDs_3174(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   any
		want []string
	}{
		{"文字は 1 件", "a", []string{"a"}},
		{"空の文字は無し", "", nil},
		{"配列は全部", []any{"a", "b"}, []string{"a", "b"}},
		{"配列の空と文字でない物は飛ばす", []any{"a", "", 1, nil, "b"}, []string{"a", "b"}},
		{"配列の重なりは 1 件", []any{"a", "a"}, []string{"a"}},
		{"空の配列は空", []any{}, []string{}},
		{"数は無し", 1, nil},
		{"nil は無し", nil, nil},
		{"文字の配列(any でない)は無し", []string{"a"}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := refIDs(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("refIDs(%v) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}
