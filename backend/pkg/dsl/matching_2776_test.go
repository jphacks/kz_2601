package dsl

import (
	"reflect"
	"testing"
)

func TestMatchEntities_2776(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		app  App
		want []string
	}{
		{"実体が無い", App{}, nil},
		{"match の役を持つ実体が無い", App{Entities: []Entity{
			{ID: "post", Match: ""},
			{ID: "memo"},
		}}, nil},
		{"profile が 1 つ", App{Entities: []Entity{
			{ID: "profile", Match: MatchProfile},
		}}, []string{"profile"}},
		{"matched が 1 つ", App{Entities: []Entity{
			{ID: "contact", Match: MatchMatched},
		}}, []string{"contact"}},
		{"役の無い実体は混ざらず、並びは書いた順", App{Entities: []Entity{
			{ID: "post"},
			{ID: "contact", Match: MatchMatched},
			{ID: "memo"},
			{ID: "profile", Match: MatchProfile},
		}}, []string{"contact", "profile"}},
		{"同じ役の実体が重複", App{Entities: []Entity{
			{ID: "profile", Match: MatchProfile},
			{ID: "profile2", Match: MatchProfile},
			{ID: "contact", Match: MatchMatched},
		}}, []string{"profile", "profile2", "contact"}},
		{"役の名前が未知でも拾う", App{Entities: []Entity{
			{ID: "dating", Match: "dating"},
		}}, []string{"dating"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.app.MatchEntities(); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("MatchEntities() = %#v, want %#v", got, tc.want)
			}
		})
	}
}
