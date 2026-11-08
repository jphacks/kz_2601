package ai

import "strings"

// adaptiveAmountRules: the prompt taught date windows relative to today
// (thisWeek / addDays(today)) and a parent's count by {"ref":"id"}, but not a
// window relative to the row's own date, so 「先週の達成率で今週の目標を調整」
// had no recipe (docs/dsl/examples/adaptive_study.json).
var adaptiveAmountRules = strings.TrimSpace(`
## 前の期間の結果で次の目標を決める(先週の達成率で今週の量を調整)
- 「先週どれだけできたかで今週の目標を調整」「達成率が低かったら量を減らして」は、週の entity(start: date 週の初め・base: integer 基準の量)と記録の entity(date・done: boolean defaultValue false)を作り、週の computed で**その行の start から見た先週**を数える: last_total = ` + adaptiveTotalExpr + `、last_done = 同じ filter に {"field":"done","op":"eq","value":true} を足す。filter の value に {"ref":"start"} を書くと行ごとの期間になる(thisWeek / addDays(today) は今日から見た期間なので、行ごとの先週に使わない)。
- 達成率 last_rate(integer・unit「%」) = ` + adaptiveRateExpr + `(先週の記録が無ければ 80 = 基準のまま)。今週の目標 target = ` + adaptiveTargetExpr + `(80% で基準どおり・基準の 0.5〜1.5 倍に収める)。週の table の列に start・last_rate・target を出す。
- 目標が決まった数(「毎週 300 分」)なら固定の値で足りるので、この計算を足さない。
`)

// The expressions exactly as the rule writes them (adaptive_amount_rules_test.go evaluates them).
const (
	adaptiveTotalExpr  = `{"agg":"count","entity":"logs","filter":[{"field":"date","op":"gte","value":{"op":"addDays","args":[{"ref":"start"},-7]}},{"field":"date","op":"lt","value":{"ref":"start"}}]}`
	adaptiveRateExpr   = `{"if":{"op":"gt","args":[{"ref":"last_total"},0]},"then":{"op":"round","args":[{"op":"mul","args":[{"op":"div","args":[{"ref":"last_done"},{"ref":"last_total"}]},100]}]},"else":80}`
	adaptiveTargetExpr = `{"op":"clamp","args":[{"op":"round","args":[{"op":"div","args":[{"op":"mul","args":[{"ref":"base"},{"ref":"last_rate"}]},80]}]},{"op":"round","args":[{"op":"mul","args":[{"ref":"base"},0.5]}]},{"op":"round","args":[{"op":"mul","args":[{"ref":"base"},1.5]}]}]}`
)

func init() { systemPrompt += "\n\n" + adaptiveAmountRules }
