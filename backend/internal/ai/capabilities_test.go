package ai

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// schemaNames returns every component type, op and action type in the schema.
func schemaNames(t *testing.T) (types, ops, actions []string) {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(dsl.SchemaJSON(), &root); err != nil {
		t.Fatal(err)
	}
	defs := root["$defs"].(map[string]any)
	types = enumValues(defs["Container"].(map[string]any), "type")
	for _, n := range refNames(defs["Leaf"]) {
		types = append(types, constType(defs[n].(map[string]any)))
	}
	ops = enumValues(defs["ExprOp"].(map[string]any), "op")
	actions = enumValues(defs["Action"].(map[string]any), "type")
	if len(types) < 40 || len(ops) < 30 || len(actions) < 5 {
		t.Fatalf("schema walk too small: %d types, %d ops, %d actions", len(types), len(ops), len(actions))
	}
	return types, ops, actions
}

// #230: both chat prompts must know every part, op and action the generator
// can build, so "できる?" answers never lag behind the schema.
func TestChatPrompts_CoverSchema(t *testing.T) {
	types, ops, actions := schemaNames(t)
	for name, p := range map[string]string{"edit": chatEditSystemPrompt, "create": chatCreateSystemPrompt} {
		for _, group := range [][]string{types, ops, actions} {
			for _, n := range group {
				if !strings.Contains(p, n) {
					t.Errorf("%s chat prompt lacks %q", name, n)
				}
			}
		}
	}
	for _, w := range []string{"video", "location", "health"} {
		if !strings.Contains(chatCreateSystemPrompt, w) {
			t.Errorf("create chat prompt lacks field capability %q", w)
		}
	}
}

// The index is a summary: one short line per entry, much smaller than the
// catalog, and placed after the static rules (the system prompt is static
// as a whole, so it stays cacheable).
func TestCapabilityIndex_IsSummary(t *testing.T) {
	if n, c := utf8.RuneCountInString(chatCapabilityIndex), utf8.RuneCountInString(catalog); n*3 > c {
		t.Errorf("index %d runes is not a summary of the %d-rune catalog", n, c)
	}
	for _, l := range strings.Split(chatCapabilityIndex, "\n") {
		if utf8.RuneCountInString(l) > 900 {
			t.Errorf("index line too long: %.60s…", l)
		}
	}
	if !strings.HasSuffix(chatCreateSystemPrompt, chatCapabilityIndex) {
		t.Error("create prompt must end with the capability index")
	}
	if strings.Contains(chatEditSystemPrompt, chatCapabilityIndex) {
		t.Error("edit prompt already carries the full catalog; do not add the index too")
	}
	for _, w := range []string{"連続日数", "直近", "動画", "歩数", "検索"} {
		if !strings.Contains(chatCapabilityIndex, w) {
			t.Errorf("index lost %q (descriptions cut too short?)", w)
		}
	}
}

// capabilityWords maps words users ask about to the schema name that makes
// them buildable. A limit must not mention them (#230: 「連続日数」 was listed
// as impossible after #192 shipped streak).
var capabilityWords = map[string]string{
	"連続日数": "streak", "直近": "doneInDays", "動画": "videoField", "歩数": "healthStat",
	"体重": "healthStat", "ヘルスケア": "healthStat", "場所": "locationPicker", "検索": "searchBar",
	"道のり": "locationTracker", "地図": "mapView", "QR": "qrScanner", "グラフ": "chart",
	"カレンダー": "calendar", "割り勘": "settlement", "タイマー表示": "timerDisplay",
	"ナビ": "routeGuide", "道案内": "routeGuide", "経路案内": "routeGuide",
}

func TestChatLimits_NoConflictWithSchema(t *testing.T) {
	types, ops, actions := schemaNames(t)
	all := map[string]bool{}
	for _, group := range [][]string{types, ops, actions} {
		for _, n := range group {
			all[n] = true
		}
	}
	for w, n := range capabilityWords {
		if !all[n] {
			t.Errorf("capabilityWords %q -> %q is not in the schema", w, n)
		}
	}
	for _, l := range chatLimits {
		for w, n := range capabilityWords {
			if strings.Contains(l.What, w) {
				t.Errorf("limit %q says %q is impossible, but the schema has %s", l.What, w, n)
			}
		}
		for n := range all {
			if len(n) > 3 && strings.Contains(strings.ToLower(l.What), strings.ToLower(n)) {
				t.Errorf("limit %q names schema entry %q", l.What, n)
			}
		}
	}
	// Every limit reaches both prompts.
	for name, p := range map[string]string{"edit": chatEditSystemPrompt, "create": chatCreateSystemPrompt} {
		for _, l := range chatLimits {
			if !strings.Contains(p, l.What) {
				t.Errorf("%s prompt lacks limit %q", name, l.What)
			}
		}
		if strings.Contains(p, "連続日数(代わり") {
			t.Errorf("%s prompt still says streaks are impossible", name)
		}
	}
}

func TestShortDesc(t *testing.T) {
	for in, want := range map[string]string{
		"区切り線。":                 "区切り線",
		"A single symbol. Use it.": "A single symbol。Use it",
		"場所を選ぶ入力欄。タップで開く。":      "場所を選ぶ入力欄。タップで開く",
	} {
		if got := shortDesc(in); got != want {
			t.Errorf("shortDesc(%q) = %q, want %q", in, got, want)
		}
	}
	long := strings.Repeat("あ", 100) + "。"
	if got := shortDesc(long); utf8.RuneCountInString(got) != capDescRunes+1 {
		t.Errorf("long first sentence not cut: %d runes", utf8.RuneCountInString(got))
	}
	if got := opDesc("streak", `streak({"ref":x}) = 連続日数`); got != "連続日数" {
		t.Errorf("opDesc = %q", got)
	}
}
