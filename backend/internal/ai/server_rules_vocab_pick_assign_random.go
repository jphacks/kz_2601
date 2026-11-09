package ai

import "strings"

// When a duty is picked at random rather than by count (runs after
// server_rules_vocab_pick_assign_order.go: files initialize in name order).
// The assign section mentions 当番 / 担当, so without this line a
// 「当番をランダムに」 request drifts to fewest.

// serverRulesAssignRandomLine: the assign section's line -> line + bullet.
var serverRulesAssignRandomLine = [2]string{
	`同じ数なら先に作った候補(順に割り当てると交代になる)。`,
	`同じ数なら先に作った候補(順に割り当てると交代になる)。
- 「当番をランダムに 1 人選んで」「くじで決めて」は当番・担当でも order: random(fewest にしない)。fewest は「回数が一番少ない人・偏らないように」、most は「一番多い・票が一番多いもの」と言われた時だけ(投票なら count は投票の記録の ref)。`,
}

func init() {
	l := serverRulesAssignRandomLine
	serverRulesAssignSection = strings.Replace(serverRulesAssignSection, l[0], l[1], 1)
	serverRulesReadsSection = strings.Replace(serverRulesReadsSection, l[0], l[1], 1)
	serverRulesPrompt = strings.Replace(serverRulesPrompt, l[0], l[1], 1)
}
