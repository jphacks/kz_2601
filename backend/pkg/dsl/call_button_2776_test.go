package dsl

import "testing"

func TestHasCallButton_2776(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		app  App
		want bool
	}{
		{"画面が無い", App{}, false},
		{"通話のボタンが無い", App{Screens: []Screen{{ID: "home", Components: []Component{
			{Type: CompList, Entity: "post"},
			{Type: CompTable, Entity: "post"},
		}}}}, false},
		{"通話のボタンが 1 つ", App{Screens: []Screen{{ID: "home", Components: []Component{
			{Type: CompCallButton},
		}}}}, true},
		{"2 つ目の画面で見つかる", App{Screens: []Screen{
			{ID: "home", Components: []Component{{Type: CompList, Entity: "post"}}},
			{ID: "talk", Components: []Component{{Type: CompCallButton, Room: "team-a"}}},
		}}, true},
		{"セクションの奥にある", App{Screens: []Screen{{ID: "home", Components: []Component{
			{Type: CompSection, Children: []Component{{Type: CompCallButton}}},
		}}}}, true},
		{"タブの奥の画面のセクションにある", App{
			App: AppMeta{Tabs: []Tab{{Screen: "home"}, {Screen: "talk"}}},
			Screens: []Screen{
				{ID: "home", Components: []Component{{Type: CompList, Entity: "post"}}},
				{ID: "talk", Components: []Component{{Type: CompVStack, Children: []Component{
					{Type: CompCallButton, Room: "lobby"},
				}}}},
			},
		}, true},
		{"同じ部屋の通話のボタンが重複", App{Screens: []Screen{{ID: "home", Components: []Component{
			{Type: CompCallButton, Room: "lobby"},
			{Type: CompCallButton, Room: "lobby"},
		}}}}, true},
		{"どの画面からも辿れなくても見つかる", App{
			InitialScreen: "home",
			Screens: []Screen{
				{ID: "home", Components: []Component{{Type: CompList, Entity: "post"}}},
				{ID: "orphan", Components: []Component{{Type: CompCallButton}}},
			},
		}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.app.HasCallButton(); got != tc.want {
				t.Errorf("HasCallButton() = %v, want %v", got, tc.want)
			}
		})
	}
}
