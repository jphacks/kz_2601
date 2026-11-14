package ai

import (
	"regexp"
	"strings"
)

// Server-rule similar (rule.similar: the record sharing the most values with
// the row, dsl/server_rules_similar.go). This init runs after
// server_rules_lookup_vocab.go and before server_rules_vocab.go (files
// initialize in name order), so it edits the reads section as the lookup
// left it, which server_rules_vocab.go then appends to the prompt.

// serverRulesSimilarLines: reads section line -> replacement.
var serverRulesSimilarLines = [][2]string{
	{`1 つのルールが読む entity は each・agg・lookup を合わせて 2 つまで。`, `1 つのルールが読む entity は each・agg・lookup・similar を合わせて 2 つまで。`},
	{`- まだできない: 好み・相性・似ている人での推薦(「まだ行っていない店」「名簿に無い人」は lookup で書ける)、`, `- まだできない: 好みを学ぶ推薦・文章の意味の近さ(「まだ行っていない店」「名簿に無い人」は lookup、共通の項目の数で「似ている人・合うサークル」を探すのは similar で書ける)、`},
}

var serverRulesSimilarSection = strings.TrimSpace(`
## 一番似ている記録(similar)
- rule.similar {"entity":"<探す entity>","fields":["<field>",...],"min":<数>}: 行(きっかけか on.each のレコード)と fields の値が同じ数(共通点)が一番多いレコードを探し、min(省略 = 1)以上の時だけ動く(if・lookup と全部そろった時)。1 ルールに 1 つ。
- fields は 1〜5 個で、行の entity と探す entity の**両方に同じ id である** field(string / text / enum / integer / number / date / ref / refs)。string / text は 、 , / ・ 改行 で区切った項目ごと(「映画、音楽」は 2 つ)、refs は選んだ ID ごとに数える。hidden・computed は比べない。
- do でだけ使える: {"ref":"$similar"}(見つかった記録の ID。ref の field に書ける)・{"ref":"$similar.<field>"}(その記録の値)・{"ref":"$similarScore"}(共通点の数)・通知の {$similarScore}・to {"kind":"authorOf","field":"$similar"}(見つかった記録を書いた人)。if では使えない。$similar の field には書かない(行の ref field に書く)。
- 行のレコード自身と、shared / public では同じ人が書いた記録は選ばない。同じ数なら新しい記録。探す entity は同じアプリ・ルールと同じ側だけ(宛先付きは不可)。読む件数の上限は each・agg と同じ。
- 似ている人の推薦: 「趣味とレベルが近い人をおすすめして」(shared の profiles{hobbies:string, level:enum, best_match:ref→profiles, match_score:integer})= on {record:"updated",entity:"profiles",fields:["hobbies","level"]}・similar {"entity":"profiles","fields":["hobbies","level"],"min":2}・updateRecord(ref 省略)values [{"field":"best_match","value":{"ref":"$similar"}},{"field":"match_score","value":{"ref":"$similarScore"}}]。毎日まとめて付け直すなら on {every:"daily",time:"08:00",each:"profiles"} に同じ similar。画面はプロフィールに best_match を出す。
- 合う人へのお誘い: 「サークルを作ったら活動が合う学生に知らせて」= on {record:"created",entity:"circles"}・similar {"entity":"students","fields":["tags"]}(circles と students の両方に tags)・notify to {"kind":"authorOf","field":"$similar"}(body「{name} があなたと {$similarScore} 個合います」)。
`)

func init() {
	for _, l := range serverRulesSimilarLines {
		serverRulesReadsSection = strings.Replace(serverRulesReadsSection, l[0], l[1], 1)
	}
	serverRulesReadsSection += "\n\n" + serverRulesSimilarSection
	serverRulesAskRe = regexp.MustCompile(serverRulesAskRe.String() + `|似て(い)?る|近い人|相性|共通(点|の)|おすすめ|オススメ|推薦|スカウト|(?i:similar|recommend)`)
}
