package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// A current build sends progressBar.settingTarget (iOS ClientCapabilities).
func init() { currentFeatures = append(currentFeatures, FeatureProgressSettingTarget) }

// The goal bullet tells the bar to read the goal; singleRules no longer says
// a target cannot be an expression, and still leaves the prompt for builds
// without single.
func TestProgressSettingTargetPrompt(t *testing.T) {
	if !strings.Contains(systemPrompt, goalBarSetting) || strings.Contains(systemPrompt, goalBarLiteral) || strings.Contains(systemPrompt, singleBarOld) {
		t.Fatal("systemPrompt keeps the literal-only target sentences")
	}
	off := NewClientRestriction(allComponentTypes(), nil, featuresWithout(FeatureProgressSettingTarget))
	if p := restrictPrompt(withRestriction(context.Background(), off), systemPrompt); !strings.Contains(p, goalBarLiteral) || strings.Contains(p, goalBarSetting) {
		t.Fatal("a build without the feature must get the literal-only bullet back")
	}
	noSingle := NewClientRestriction(allComponentTypes(), nil, featuresWithout(FeatureProgressSettingTarget, FeatureSingleEntity))
	if p := restrictPrompt(withRestriction(context.Background(), noSingle), systemPrompt); strings.Contains(p, "## 設定値の置き場は single の entity") {
		t.Fatal("singleRules must still leave the prompt for builds without single")
	}
}

const progressTargetDoc = `{"schemaVersion":1,"app":{"name":"歩数","voiceEntry":"floating"},"initialScreen":"home",
"entities":[
 {"id":"walk","name":"歩いた記録","fields":[{"id":"steps","name":"歩数","type":"integer","required":true},{"id":"date","name":"日付","type":"date","defaultValue":"now"}]},
 {"id":"settings","name":"設定","single":true,"fields":[{"id":"daily_goal","name":"1 日の目標","type":"integer"%s}]}],
"screens":[
 {"id":"home","title":"ホーム","toolbar":[{"label":"追加","action":{"type":"navigate","screen":"form"}},{"label":"目標","action":{"type":"navigate","screen":"goal","presentation":"sheet"}}],
  "components":[
   {"type":"progressBar","label":"今日の歩数","entity":"walk","field":"steps","filter":{"field":"date","op":"today","value":null},"target":{"setting":"daily_goal"},"tone":"positive"},
   {"type":"list","entity":"walk","item":{"title":"steps"},"onItemTap":{"type":"navigate","screen":"form"}}]},
 {"id":"form","title":"記録","entity":"walk","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"walk"}}],"components":[{"type":"numberField","field":"steps"}]},
 {"id":"goal","title":"目標","entity":"settings","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"settings"}}],"components":[{"type":"numberField","field":"daily_goal"}]}]}`

// Without the feature the setting target becomes the goal's defaultValue; a
// build without single sees agg latest first and gets the same number. With
// no default the bar goes (nothing to fill it to).
func TestProgressSettingTargetStrip(t *testing.T) {
	for name, tc := range map[string]struct {
		def, want string
		off       []string
	}{
		"default":           {`,"defaultValue":8000`, `"target":8000`, []string{FeatureProgressSettingTarget}},
		"no single":         {`,"defaultValue":8000`, `"target":8000`, []string{FeatureProgressSettingTarget, FeatureSingleEntity}},
		"no default (drop)": {``, ``, []string{FeatureProgressSettingTarget}},
	} {
		t.Run(name, func(t *testing.T) {
			doc := []byte(strings.Replace(progressTargetDoc, "%s", tc.def, 1))
			if _, err := dsl.Parse(doc); err != nil {
				t.Fatal(err)
			}
			r := NewClientRestriction(allComponentTypes(), nil, featuresWithout(tc.off...))
			out, _, changed := enforceRestriction(doc, r)
			if !changed || strings.Contains(string(out), `"target":{`) {
				t.Fatalf("enforce: changed=%v out=%s", changed, out)
			}
			if tc.want != "" && !strings.Contains(string(out), tc.want) {
				t.Fatalf("want %s in %s", tc.want, out)
			}
			if tc.want == "" && strings.Contains(string(out), `"progressBar"`) {
				t.Fatalf("bar without a default must go: %s", out)
			}
			if _, err := dsl.Parse(out); err != nil {
				t.Fatalf("enforced doc invalid: %v", err)
			}
		})
	}
}
