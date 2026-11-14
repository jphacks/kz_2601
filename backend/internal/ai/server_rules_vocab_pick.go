package ai

import (
	"regexp"
	"strings"
)

// Server-rule pick (rule.pick: one record to recommend, random / oldest /
// newest, dsl/server_rules_pick.go). This init runs after
// server_rules_vocab.go (files initialize in name order): it swaps lines of
// the reads section (as server_rules_similar_vocab.go left it) both in the
// section and in the prompt it was appended to, then appends its own.

// serverRulesPickLines: prompt line -> replacement.
var serverRulesPickLines = [][2]string{
	{`1 つのルールが読む entity は each・agg・lookup・similar を合わせて 2 つまで。`, `1 つのルールが読む entity は each・agg・lookup・similar・pick を合わせて 2 つまで。`},
	{`共通の項目の数で「似ている人・合うサークル」を探すのは similar で書ける)、`, `共通の項目の数で「似ている人・合うサークル」を探すのは similar、ランダム・まだ食べていない・一番古いものから 1 つ選ぶおすすめは pick で書ける)、`},
}

var serverRulesPickSection = strings.TrimSpace(`
## おすすめを 1 つ選ぶ(pick)
- rule.pick {"entity":"<選ぶ entity>","order":"random|oldest|newest","filter":[<agg と同じ絞り込み>],"exclude":{"entity":"<除く記録の entity>","field":"<選ぶ entity への ref / refs>"}}: entity のレコードから、filter(3 つまで・省略可)に合い、exclude の entity のレコードが field で指していない(食べた・見せた・読んだ)ものを 1 つ選び、選べた時だけ動く(if・lookup と全部そろった時)。1 ルールに 1 つ・similar と同じルールには書かない。
- order: random(ランダム。同じ実行のやり直しでは同じ記録)・oldest(一番古く作った記録)・newest(一番新しい記録)。
- do でだけ使える: {"ref":"$pick"}(選んだ記録の ID。ref の field に書ける)・{"ref":"$pick.<field>"}(その記録の値)・通知の {$pick.<field>}(文字・数・日付の field)・to {"kind":"authorOf","field":"$pick"}(その記録を書いた人)。if では使えない。$pick の記録は書き換えない(ログの entity に createRecord で残す)。
- each の無い時刻のきっかけでも使える(宛先は owner・members、createRecord)。行のレコード自身は選ばない。entity と exclude の entity は同じアプリ・ルールと同じ側だけ(宛先付きは不可)。hidden・computed の field では絞らない。
- 毎朝のおすすめ: 「まだ食べていないメニューを毎朝 1 つおすすめして」(shared の menus{name}・eats{menu:ref→menus}・picks{menu:ref→menus, name:string, day:date})= on {every:"daily",time:"08:00"}・pick {"entity":"menus","order":"random","exclude":{"entity":"eats","field":"menu"}}・createRecord picks values [{"field":"menu","value":{"ref":"$pick"}},{"field":"name","value":{"ref":"$pick.name"}},{"field":"day","value":{"op":"today","args":[]}}]・notify to {"kind":"members"}(body「今日は {$pick.name} はどうですか」)。画面は picks の list。
- 献立の提案: 「毎晩、明日の献立を主菜からランダムに決めて」(private の recipes{name, kind:enum})= on {every:"daily",time:"20:00"}・pick {"entity":"recipes","order":"random","filter":[{"field":"kind","op":"eq","value":"main"}]}・createRecord plans・notify owner(body「明日は {$pick.name}」)。
- 順番に出す: 「登録した豆知識を古い順に毎日 1 つ出して」= pick {"entity":"tips","order":"oldest","exclude":{"entity":"shown","field":"tip"}} と createRecord shown {tip:$pick}(出したものを記録して次は除く)。
`)

func init() {
	for _, l := range serverRulesPickLines {
		// the lines are in the reads section, already appended to the prompt
		serverRulesReadsSection = strings.Replace(serverRulesReadsSection, l[0], l[1], 1)
		serverRulesPrompt = strings.Replace(serverRulesPrompt, l[0], l[1], 1)
	}
	serverRulesReadsSection += "\n\n" + serverRulesPickSection // the prompt still ends with the section
	serverRulesPrompt += "\n\n" + serverRulesPickSection
	serverRulesAskRe = regexp.MustCompile(serverRulesAskRe.String() + `|ランダム|おまかせ|日替わり|今日の(おすすめ|一品|献立)|献立を(決め|考え|提案)|まだ(食べて|読んで|見て|出して)(い)?ない|(?i:random)`)
}
