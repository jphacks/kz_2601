package ai

import "strings"

// The creator's on / off switch of a rule (#347): "止めて" turns the rule
// off (enabled: false, kept in the DSL) instead of removing it; "消して"
// still removes it. init() swaps the base prompt's "やめて = 外す" line and
// appends the vocabulary (server_rules_pass.go's init runs first).

var serverRulesToggleLine = [2]string{
	`(足す・直す・消すを反映。「その通知をやめて」なら外す)`,
	`(足す・直す・消す・オン / オフを反映。「消して」「もう要らない」なら外す。「止めて」「やめて」「オフに」は下の「オン / オフ」)`,
}

var serverRulesToggleSection = strings.TrimSpace(`
## オン / オフ(enabled)
- 「朝の通知を止めて」「この自動の動きをいったんオフに」「しばらくやめて」= そのルールに "enabled": false を足す(ルールは消さない・id・on・do は変えない)。
- 「また動かして」「オンに戻して」= そのルールの enabled を外す(省略 = オン)。
- 「消して」「もう要らない」「削除して」だけはルールごと外す。どれか分からなければオフにする(消さない)。
- 頼まれていないルールの enabled は今のまま出す(オフのルールを勝手にオンに戻さない)。
`)

func init() {
	serverRulesPrompt = strings.Replace(serverRulesPrompt, serverRulesToggleLine[0], serverRulesToggleLine[1], 1)
	serverRulesPrompt += "\n\n" + serverRulesToggleSection
}
