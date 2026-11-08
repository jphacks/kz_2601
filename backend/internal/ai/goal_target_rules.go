package ai

import "strings"

// goalTargetRules: a progressBar target (gauge max) is a number literal, so a
// bar next to a goal / budget entity ignores the goal the user edits
// (nav_study: target 120 beside goal.target_minutes; dsl goal_target.go).
// Appended like groupByRules so prompt.go stays untouched; builds without
// single get the agg latest snippet (restrict.go noSingle).
var goalTargetRules = strings.TrimSpace(`
## 変えられる目標と進み具合のバー
- progressBar の target・gauge の max は数値しか書けない。目標・予算を entity(single の設定など)に持たせて後から変えられるようにした時は、その数を target / max に書き写さない(目標を変えてもバーが動かない)。達成率は stat の format percent で目標を読む: ` + goalRateSetting + `。progressBar{entity,field,target} は目標が依頼文で決まっていて変えない時だけ。
`)

const (
	goalRateSetting = `{"type":"stat","label":"目標の達成率","format":"percent","value":{"op":"div","args":[{"agg":"sum","entity":"study","field":"minutes"},{"setting":"target_minutes"}]}}`
	goalRateLatest  = `{"type":"stat","label":"目標の達成率","format":"percent","value":{"op":"div","args":[{"agg":"sum","entity":"study","field":"minutes"},{"agg":"latest","entity":"goal","field":"target_minutes","by":"date"}]}}`
)

func init() { systemPrompt += "\n\n" + goalTargetRules }
