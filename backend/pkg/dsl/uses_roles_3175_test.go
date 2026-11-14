package dsl

import "testing"

func TestUsesRoles_3175(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		app  App
		want bool
	}{
		{"何も無い", App{Entities: []Entity{{ID: "a", Fields: []Field{{ID: "f"}}}}}, false},
		{"自分だけの削除は対象外", App{Entities: []Entity{{ID: "a", DeleteBy: DeleteByAuthor}}}, false},
		{"管理者だけが直せる項目", App{Entities: []Entity{{ID: "a", Fields: []Field{{ID: "f", EditableBy: RoleAdmins}}}}}, true},
		{"deleteBy が admins", App{Entities: []Entity{{ID: "a", DeleteBy: DeleteByAdmins}}}, true},
		{"deleteBy が authorOrAdmins", App{Entities: []Entity{{ID: "a", DeleteBy: DeleteByAuthorOrAdmins}}}, true},
		{"2 つ目の実体で見つかる", App{Entities: []Entity{{ID: "a"}, {ID: "b", DeleteBy: DeleteByAdmins}}}, true},
		{"画面の部品の visibleTo", App{Screens: []Screen{{ID: "s", Components: []Component{{Type: "button", VisibleTo: RoleAdmins}}}}}, true},
		{"入れ子の部品の visibleTo", App{Screens: []Screen{{ID: "s", Components: []Component{{Type: "vstack", Children: []Component{{Type: "button", VisibleTo: RoleAdmins}}}}}}}, true},
		{"visibleTo の無い部品", App{Screens: []Screen{{ID: "s", Components: []Component{{Type: "button"}}}}}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.app.UsesRoles(); got != tc.want {
				t.Errorf("UsesRoles() = %v, want %v", got, tc.want)
			}
		})
	}
}
