package ai

import (
	"regexp"
	"strings"
)

// Server-rule assignment (rule.pick with order fewest / most and count,
// dsl/server_rules_assign.go). This init runs after
// server_rules_vocab_pick.go (files initialize in name order): it swaps
// lines of the reads section as the pick left them, both in the section and
// in the prompt it was appended to, then appends its own.

// serverRulesAssignLines: prompt line -> replacement.
var serverRulesAssignLines = [][2]string{
	{`1 つのルールが読む entity は each・agg・lookup・similar・pick を合わせて 2 つまで。`, `1 つのルールが読む entity は each・agg・lookup・similar・pick を合わせて 2 つまで。pick に count があるルールは 3 つまで。`},
	{`複数人の割り当て・日程の自動調整、`, `全体を最適にする割り当て・日程の最適化(担当が一番少ない人・空いている日へ 1 件ずつ割り当てる、一番多く集まれる日を選ぶのは pick の fewest / most で書ける)、`},
}

var serverRulesAssignSection = strings.TrimSpace(`
## 割り当て・日程(pick の fewest / most)
- rule.pick {"entity":"<候補の entity>","order":"fewest|most","count":{"entity":"<数える記録の entity>","field":"<候補への ref / refs>"},"filter":[...],"exclude":{"entity":"<除く記録>","field":"<候補への ref>","match":{"field":"<除く記録の field>","equals":"<行の field>"}}}: 候補ごとに count の記録が指している数を数え、fewest は一番少ない(担当が少ない人・空いている日)、most は一番多い(一番多く集まれる日。1 つも無ければ動かない)候補を 1 つ選ぶ。同じ数なら先に作った候補(順に割り当てると交代になる)。
- 選んだ候補は {"ref":"$pick"} で行の ref field に書く(updateRecord・ref 省略)。行のレコード自身は数えない。each では同じ実行で先に割り当てた行も数えるので、まとめて割り当てても 1 人に偏らない。
- exclude.match(省略可): exclude の記録のうち、field の値が行の equals と同じものだけで除く(その日に休みの人)。レコードのきっかけか on.each だけ。比べ方は lookup と同じ。
- まだ割り当てていない行だけ: if {"op":"eq","args":[{"op":"coalesce","args":[{"ref":"<ref field>"},"-"]},"-"]}。
- count の entity も同じアプリ・ルールと同じ側だけ(宛先付き・hidden・computed は不可)。全体を見て最適な組み合わせを解く・時間帯の重なりを計算するのはできない(1 件ずつ、一番少ない候補へ)。
- シフトの割り当て: 「シフトを作ったら、入っている回数が一番少なくて休みでない人を自動で割り当てて」(shared の staff{name}・shifts{day:date, staff:ref→staff}・offs{staff:ref→staff, day:date})= on {record:"created",entity:"shifts"}・if(staff が空)・pick {"entity":"staff","order":"fewest","count":{"entity":"shifts","field":"staff"},"exclude":{"entity":"offs","field":"staff","match":{"field":"day","equals":"day"}}}・updateRecord values [{"field":"staff","value":{"ref":"$pick"}}]・notify to {"kind":"members"}(body「{day} の担当は {$pick.name} さんです」)。毎朝まとめてなら on {every:"daily",time:"07:00",each:"shifts"} に同じ if・pick。
- 作業日の割り付け: 「タスクを足したら、作業できる日のうち一番空いている日に入れて」(private の days{date:date, off:boolean}・tasks{title, day:ref→days})= on {record:"created",entity:"tasks"}・pick {"entity":"days","order":"fewest","filter":[{"field":"off","op":"neq","value":true},{"field":"date","op":"gte","value":{"op":"today","args":[]}}],"count":{"entity":"tasks","field":"day"}}・updateRecord values [{"field":"day","value":{"ref":"$pick"}}]。
- 日程調整: 「みんなが一番集まれる日を決めて」(shared の dates{label}・answers{date:ref→dates}= 参加できる日の回答)= on {every:"daily",time:"21:00"}・pick {"entity":"dates","order":"most","count":{"entity":"answers","field":"date"}}・notify to {"kind":"members"}(body「一番集まれるのは {$pick.label} です」)。
`)

func init() {
	for _, l := range serverRulesAssignLines {
		serverRulesReadsSection = strings.Replace(serverRulesReadsSection, l[0], l[1], 1)
		serverRulesPrompt = strings.Replace(serverRulesPrompt, l[0], l[1], 1)
	}
	serverRulesReadsSection += "\n\n" + serverRulesAssignSection // the prompt still ends with the section
	serverRulesPrompt += "\n\n" + serverRulesAssignSection
	serverRulesAskRe = regexp.MustCompile(serverRulesAskRe.String() + `|割り?当て|割り?振|シフト|当番|担当を(決め|振|回)|交代で|順番に(担当|回)|空いている日|集まれる日|日程(を)?(調整|決め)|(?i:assign|rotation|roster)`)
}
