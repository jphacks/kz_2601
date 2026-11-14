package ai

import "strings"

// aiOutputsWhenRule: when not to use outputs, placed right after the outputs
// section (runs after ai_outputs_rules.go: files initialize in name order).
// The line routes with 「…」は aiButton so a client without aiButton drops it
// (restrict.go).
var aiOutputsWhenRule = `- 入力欄に入れる話が無い(表示するだけ)なら outputs を付けない。「AI が出した持ち物・アイデアを 1 つずつリストに追加」は aiButton の saveTo(shape: list)で、outputs(1 件のフォームの欄を埋める)とは別。既存の aiButton に「読み取った値をフォームの欄に入れて」と言われたら shape: fields・items・outputs にする patch。`

func init() {
	systemPrompt = strings.Replace(systemPrompt, aiOutputsRules, aiOutputsRules+"\n"+aiOutputsWhenRule, 1)
}
