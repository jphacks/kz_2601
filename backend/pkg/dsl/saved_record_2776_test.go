package dsl

import (
	"reflect"
	"testing"
)

func TestOpenedEntities_2776(t *testing.T) {
	t.Parallel()
	taskForm := Screen{ID: "form", Entity: "task", Components: []Component{{Type: CompTextField, Field: "title"}}}
	tests := []struct {
		name string
		app  App
		want map[string]bool
	}{
		{"画面が無い", App{}, map[string]bool{}},
		{"記録を開く部品が無い", App{
			InitialScreen: "home",
			Screens: []Screen{
				{ID: "home", Components: []Component{{Type: CompTable, Entity: "task"}}},
				taskForm,
			},
		}, map[string]bool{}},
		{"onItemTap の無い一覧は開かない", App{
			InitialScreen: "home",
			Screens: []Screen{
				{ID: "home", Components: []Component{{Type: CompList, Entity: "task"}}},
				taskForm,
			},
		}, map[string]bool{}},
		{"一覧の onItemTap で開く", App{
			InitialScreen: "home",
			Screens: []Screen{
				{ID: "home", Components: []Component{
					{Type: CompList, Entity: "task", OnItemTap: &Action{Type: ActionNavigate, Screen: "form"}},
				}},
				taskForm,
			},
		}, map[string]bool{"task": true}},
		{"prefillRef 付きは新しい記録なので開いていない", App{
			InitialScreen: "home",
			Screens: []Screen{
				{ID: "home", Components: []Component{
					{Type: CompList, Entity: "task", OnItemTap: &Action{Type: ActionNavigate, Screen: "form", PrefillRef: "from"}},
				}},
				taskForm,
			},
		}, map[string]bool{}},
		{"行き先が別の実体の入力画面なら開いていない", App{
			InitialScreen: "home",
			Screens: []Screen{
				{ID: "home", Components: []Component{
					{Type: CompList, Entity: "task", OnItemTap: &Action{Type: ActionNavigate, Screen: "noteForm"}},
				}},
				{ID: "noteForm", Entity: "note", Components: []Component{{Type: CompTextField, Field: "body"}}},
			},
		}, map[string]bool{}},
		{"暦の onDateTap で開く", App{
			InitialScreen: "home",
			Screens: []Screen{
				{ID: "home", Components: []Component{
					{Type: CompCalendar, Entity: "task", OnDateTap: &Action{Type: ActionNavigate, Screen: "form"}},
				}},
				taskForm,
			},
		}, map[string]bool{"task": true}},
		{"暦に onItemTap だけ書いても開かない", App{
			InitialScreen: "home",
			Screens: []Screen{
				{ID: "home", Components: []Component{
					{Type: CompCalendar, Entity: "task", OnItemTap: &Action{Type: ActionNavigate, Screen: "form"}},
				}},
				taskForm,
			},
		}, map[string]bool{}},
		{"セクションの奥の一覧でも開く", App{
			InitialScreen: "home",
			Screens: []Screen{
				{ID: "home", Components: []Component{{Type: CompSection, Children: []Component{
					{Type: CompList, Entity: "task", OnItemTap: &Action{Type: ActionNavigate, Screen: "form"}},
				}}}},
				taskForm,
			},
		}, map[string]bool{"task": true}},
		{"タブの奥の画面のセクションでも開く", App{
			App:           AppMeta{Tabs: []Tab{{Screen: "home"}, {Screen: "tasks"}}},
			InitialScreen: "home",
			Screens: []Screen{
				{ID: "home", Components: []Component{{Type: CompTable, Entity: "task"}}},
				{ID: "tasks", Components: []Component{{Type: CompVStack, Children: []Component{
					{Type: CompList, Entity: "task", OnItemTap: &Action{Type: ActionNavigate, Screen: "form"}},
				}}}},
				taskForm,
			},
		}, map[string]bool{"task": true}},
		{"辿れない画面の一覧は数えない", App{
			InitialScreen: "home",
			Screens: []Screen{
				{ID: "home", Components: []Component{{Type: CompTable, Entity: "task"}}},
				{ID: "orphan", Components: []Component{
					{Type: CompList, Entity: "task", OnItemTap: &Action{Type: ActionNavigate, Screen: "form"}},
				}},
				taskForm,
			},
		}, map[string]bool{}},
		{"同じ実体を開く一覧が重複しても 1 つ", App{
			InitialScreen: "home",
			Screens: []Screen{
				{ID: "home", Components: []Component{
					{Type: CompList, Entity: "task", OnItemTap: &Action{Type: ActionNavigate, Screen: "form"}},
					{Type: CompGridTable, Entity: "task", OnItemTap: &Action{Type: ActionNavigate, Screen: "form"}},
				}},
				taskForm,
			},
		}, map[string]bool{"task": true}},
		{"実体ごとに分かれて入る", App{
			InitialScreen: "home",
			Screens: []Screen{
				{ID: "home", Components: []Component{
					{Type: CompList, Entity: "task", OnItemTap: &Action{Type: ActionNavigate, Screen: "form"}},
					{Type: CompList, Entity: "note", OnItemTap: &Action{Type: ActionNavigate, Screen: "noteForm"}},
				}},
				taskForm,
				{ID: "noteForm", Entity: "note", Components: []Component{{Type: CompTextField, Field: "body"}}},
			},
		}, map[string]bool{"task": true, "note": true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.app.OpenedEntities(); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("OpenedEntities() = %#v, want %#v", got, tc.want)
			}
		})
	}
}
