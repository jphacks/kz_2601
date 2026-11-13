package ai

import (
	"reflect"
	"testing"
)

func TestSquash_2597(t *testing.T) {
	tests := []struct {
		name string
		in   []ChatMessage
		want []ChatMessage
	}{
		{"empty", nil, []ChatMessage{}},
		{"single", []ChatMessage{{"user", "a"}}, []ChatMessage{{"user", "a"}}},
		{"same role joined", []ChatMessage{{"user", "a"}, {"user", "b"}, {"assistant", "c"}},
			[]ChatMessage{{"user", "a\nb"}, {"assistant", "c"}}},
		{"empty text still joined", []ChatMessage{{"user", "a"}, {"user", ""}},
			[]ChatMessage{{"user", "a\n"}}},
		{"alternating kept", []ChatMessage{{"user", "a"}, {"assistant", "b"}, {"user", "c"}},
			[]ChatMessage{{"user", "a"}, {"assistant", "b"}, {"user", "c"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := squash(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestCompactJSON_2597(t *testing.T) {
	if got := compactJSON("{ \"a\" : [ 1, 2 ] }"); got != `{"a":[1,2]}` {
		t.Fatalf("got %q", got)
	}
	for _, bad := range []string{"{broken", "", "not json"} {
		if got := compactJSON(bad); got != bad {
			t.Fatalf("invalid input must pass through: %q -> %q", bad, got)
		}
	}
}

func TestToToolbar_2597(t *testing.T) {
	btn := map[string]any{"label": "追加", "action": map[string]any{"type": "add"}, "icon": "plus"}

	sm := map[string]any{}
	toToolbar(sm, btn)
	tb, _ := sm["toolbar"].([]any)
	if len(tb) != 1 {
		t.Fatalf("toolbar not created: %v", sm)
	}
	item := tb[0].(map[string]any)
	if item["label"] != "追加" || item["icon"] != "plus" {
		t.Fatalf("item %v", item)
	}

	toToolbar(sm, btn) // same action: no duplicate
	if tb, _ = sm["toolbar"].([]any); len(tb) != 1 {
		t.Fatalf("duplicate added: %v", tb)
	}

	noIcon := map[string]any{"label": "x", "action": map[string]any{"type": "other"}}
	toToolbar(sm, noIcon)
	tb, _ = sm["toolbar"].([]any)
	if len(tb) != 2 {
		t.Fatalf("want 2 got %v", tb)
	}
	if _, has := tb[1].(map[string]any)["icon"]; has {
		t.Fatalf("icon must be omitted")
	}
}
