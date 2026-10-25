package ai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// kakeiboWith edits the kakeibo few-shot's home screen.
func kakeiboWith(t *testing.T, edit func(home map[string]any)) ([]byte, *dsl.App) {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(fewShotKakeibo), &doc); err != nil {
		t.Fatal(err)
	}
	edit(doc["screens"].([]any)[0].(map[string]any))
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	app, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return raw, app
}

func homeOf(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc["screens"].([]any)[0].(map[string]any)
}

func componentAt(home map[string]any, typ string) []string {
	var areas []string
	for _, c := range home["components"].([]any) {
		if cm := c.(map[string]any); cm["type"] == typ {
			s, _ := cm["area"].(string)
			areas = append(areas, s)
		}
	}
	return areas
}

func TestFixLayoutRulesMovesButtonOffMicSeat(t *testing.T) {
	raw, app := kakeiboWith(t, func(home map[string]any) {
		for _, c := range home["components"].([]any) {
			if cm := c.(map[string]any); cm["type"] == "button" {
				cm["area"] = "A11:C12" // #169 a06
			}
		}
	})
	home := homeOf(t, fixLayoutRules(raw, app))
	if got := componentAt(home, "button"); len(got) != 1 || got[0] != "D11:F12" {
		t.Fatalf("button areas = %v, want [D11:F12]", got)
	}
}

func TestFixLayoutRulesSeatTakenGoesToToolbar(t *testing.T) {
	raw, app := kakeiboWith(t, func(home map[string]any) {
		comps := home["components"].([]any)
		comps = append(comps, map[string]any{
			"type": "button", "area": "A11:C12", "label": "グラフ",
			"action": map[string]any{"type": "navigate", "screen": "stats"},
		})
		home["components"] = comps
	})
	home := homeOf(t, fixLayoutRules(raw, app))
	if got := componentAt(home, "button"); len(got) != 1 || got[0] != "D11:F12" {
		t.Fatalf("button areas = %v, want only the right one", got)
	}
	tb := home["toolbar"].([]any)
	if len(tb) != 2 || tb[1].(map[string]any)["label"] != "グラフ" {
		t.Fatalf("toolbar = %v, want the seat button appended", tb)
	}
	if fixed := fixLayoutRules(fixLayoutRules(raw, app), app); len(homeOf(t, fixed)["toolbar"].([]any)) != 2 {
		t.Fatal("second pass must not duplicate the toolbar button")
	}
}

func TestFixLayoutRulesSwapsDateTitle(t *testing.T) {
	raw, app := kakeiboWith(t, func(home map[string]any) {
		for _, c := range home["components"].([]any) {
			children, _ := c.(map[string]any)["children"].([]any)
			for _, ch := range children {
				if chm := ch.(map[string]any); chm["type"] == "list" {
					chm["item"] = map[string]any{"title": "date", "subtitle": "memo", "trailing": "amount"} // #169 e03
				}
			}
		}
	})
	fixed := fixLayoutRules(raw, app)
	if string(fixed) == string(raw) {
		t.Fatal("date title left as is")
	}
	fixedApp, err := dsl.Parse(fixed)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range fixedApp.Screens[0].Components {
		for _, ch := range c.Children {
			if ch.Type == dsl.CompList && (ch.Item.Title != "memo" || ch.Item.Subtitle != "date") {
				t.Fatalf("item = %+v, want title memo / subtitle date", ch.Item)
			}
		}
	}
}

func TestFixLayoutRulesKeepsCleanApp(t *testing.T) {
	app, err := dsl.Parse([]byte(fewShotKakeibo))
	if err != nil {
		t.Fatal(err)
	}
	if fixed := fixLayoutRules([]byte(fewShotKakeibo), app); string(fixed) != fewShotKakeibo {
		t.Fatal("a clean app must come back byte-identical")
	}
}

func TestFixButtonOverlapAvoidsRegeneration(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(fewShotKakeibo), &doc); err != nil {
		t.Fatal(err)
	}
	home := doc["screens"].([]any)[0].(map[string]any)
	for _, c := range home["components"].([]any) {
		if cm := c.(map[string]any); cm["type"] == "section" {
			cm["area"] = "A3:F12" // #169 warikan: list section under the D11:F12 button
		}
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dsl.Parse(raw); err == nil {
		t.Fatal("fixture should fail the overlap check")
	}
	fixed := fixButtonOverlap(raw)
	if _, err := dsl.Parse(fixed); err != nil {
		t.Fatalf("still invalid: %v", err)
	}
	if got := componentAt(homeOf(t, fixed), "section"); len(got) != 1 || got[0] != "A3:F10" {
		t.Fatalf("section areas = %v, want [A3:F10]", got)
	}
	if string(fixButtonOverlap([]byte(fewShotKakeibo))) != fewShotKakeibo {
		t.Fatal("a clean app must come back byte-identical")
	}
}

func TestSystemPromptHasAudit3Rules(t *testing.T) {
	for _, s := range []string{
		"その enum の内訳 chart",
		"実施記録・log の entity を別に足さない",
		`{"op":"streak","args":[{"ref":"<toggle field>"}]}`, // #172 replaced "streak → today's count"
	} {
		if !strings.Contains(systemPrompt, s) {
			t.Errorf("system prompt lost the #169 rule %q", s)
		}
	}
}
