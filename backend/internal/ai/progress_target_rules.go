package ai

import (
	"encoding/json"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// FeatureProgressSettingTarget: progressBar target {"setting": "<fieldId>"}
// (dsl progress_target.go). A build without it reads only a number target and
// shows an empty bar, so it is not generated there; a generated one becomes
// the field's defaultValue (the bar such a build would have shown with the
// goal copied in).
const FeatureProgressSettingTarget = "progressBar.settingTarget"

// The goal bullet of titleGoalRules said a bar target is a number only; with
// the feature the bar reads the goal. singleRules' "target は式は書けない"
// becomes neutral (true for both builds), so restrict.go's noSingle removal
// of singleRules still matches.
const (
	goalBarLiteral = `- progressBar の target・gauge の max は数値しか書けない。上のように後から変えられる目標・予算は、その数を target / max に書き写さない(目標を変えてもバーが動かない)。`
	goalBarSetting = `- 上のように後から変えられる目標・予算は、その数を progressBar の target・gauge の max に書き写さない(目標を変えてもバーが動かない)。progressBar は target {"setting":"<fieldId>"} で目標を読める(gauge の max は数値だけ)。`
	singleBarOld   = `(progressBar・gauge の value / target / max は数値だけで式は書けない)`
	singleBarNew   = `(progressBar・gauge の value と gauge の max は数値だけ)`
)

func init() {
	// order-independent against single_rules.go / title_goal_rules.go init
	singleRules = strings.Replace(singleRules, singleBarOld, singleBarNew, 1)
	titleGoalRules = strings.Replace(titleGoalRules, goalBarLiteral, goalBarSetting, 1)
	systemPrompt = strings.Replace(systemPrompt, singleBarOld, singleBarNew, 1)
	systemPrompt = strings.Replace(systemPrompt, goalBarLiteral, goalBarSetting, 1)
	clientFeatures = append(clientFeatures, clientFeature{
		key:     FeatureProgressSettingTarget,
		rewrite: [][2]string{{goalBarSetting, goalBarLiteral}},
		note:    "- progressBar の target に式 {\"setting\":...} は書かない(この版では数値だけ)。目標が変わる時は達成率を stat の format percent で出す。\n",
		schema: func(defs map[string]any) {
			d, _ := defs["ProgressBar"].(map[string]any)
			props, _ := d["properties"].(map[string]any)
			if props != nil {
				props["target"] = map[string]any{"type": "number", "description": "Entity mode denominator (> 0), e.g. monthly budget 30000."}
			}
		},
		places: func(root map[string]any) []string { return componentPlaces(root, exprTarget) },
		strip: func(root map[string]any) int {
			n := editComponents(root, func(c map[string]any) bool {
				if !exprTarget(c) {
					return false
				}
				if v, ok := targetDefault(root, c["target"]); ok {
					c["target"] = v
					return true
				}
				return false
			})
			// no default > 0: nothing to fill the bar to
			return n + dropComponents(root, exprTarget)
		},
		violation: `progressBar target {"setting"} is not available in this app build: use a number target, or show the rate as a stat with format percent`,
	})
}

// exprTarget: a progressBar whose target is not a number.
func exprTarget(c map[string]any) bool {
	if !isType(c, dsl.CompProgressBar) {
		return false
	}
	_, obj := c["target"].(map[string]any)
	return obj
}

// targetDefault: the defaultValue (> 0) of the field a target expression
// reads. Builds without single see {"agg":"latest","entity","field"} here
// (unsingle ran first); otherwise {"setting": f} of a single entity.
func targetDefault(root map[string]any, t any) (float64, bool) {
	m, _ := t.(map[string]any)
	field, _ := m["setting"].(string)
	entity := ""
	if field == "" {
		entity, _ = m["entity"].(string)
		field, _ = m["field"].(string)
	}
	ents, _ := root["entities"].([]any)
	for _, e := range ents {
		em, _ := e.(map[string]any)
		if em == nil || (entity != "" && em["id"] != entity) || (entity == "" && em["single"] != true) {
			continue
		}
		fs, _ := em["fields"].([]any)
		for _, f := range fs {
			fm, _ := f.(map[string]any)
			if fm == nil || fm["id"] != field {
				continue
			}
			var v float64
			switch d := fm["defaultValue"].(type) {
			case float64:
				v = d
			case json.Number:
				v, _ = d.Float64()
			}
			return v, v > 0
		}
	}
	return 0, false
}
