package ai

import (
	"regexp"
	"strings"
)

// Server-rule lookup (rule.lookup: exists / notExists in another entity,
// dsl/server_rules_lookup.go). This init runs before server_rules_pass.go /
// server_rules_vocab.go (files initialize in name order), so it edits the
// reads section, which server_rules_vocab.go then appends to the prompt.

// serverRulesLookupLines: reads section line -> replacement.
var serverRulesLookupLines = [][2]string{
	{`1 つのルールが読む entity は each と agg を合わせて 2 つまで。`, `1 つのルールが読む entity は each・agg・lookup を合わせて 2 つまで。`},
	{`- まだできない: おすすめ・相性・似ている人の推薦、`, `- まだできない: 好み・相性・似ている人での推薦(「まだ行っていない店」「名簿に無い人」は lookup で書ける)、`},
}

var serverRulesLookupSection = strings.TrimSpace(`
## 別の記録との突き合わせ(lookup)
- rule.lookup {"op":"exists|notExists","entity":"<探す entity>","field":"<その field か $id>","equals":"<行の field か $id>"}: 探す entity に field の値が行の equals の値と同じレコードがある / 無い時だけ動く(if と両方そろった時)。行はきっかけのレコードか on.each のレコード(each の無い時刻のきっかけでは使えない)。1 ルールに 1 つ。
- 等しいかだけを見る。文字と数は全角半角・大文字小文字・前後の空白を区別しない。日付は日付どうし、ref と $id は同じ entity の ID どうし。行の値が空なら動かない。探す entity が行と同じなら行のレコード自身は数えない(重複の検出)。
- 探す entity は同じアプリ・ルールと同じ側だけ(宛先付きは不可)。読む件数の上限は each・agg と同じ。
- 名簿との照合: 「名簿に無い学籍番号の履修登録に印をつけて」= on {every:"daily",time:"07:00",each:"registrations"}・lookup {"op":"notExists","entity":"roster","field":"student_id","equals":"student_id"}・updateRecord(ref 省略)values [{"field":"mismatch","value":true}]。画面は mismatch が true の list。登録の時に知らせるなら on {record:"created",entity:"registrations"} に同じ lookup・notify。
- まだ行っていない店: 「行っていない店を毎週教えて」= on {every:"weekly",weekday:"sun",time:"10:00",each:"shops"}・lookup {"op":"notExists","entity":"visits","field":"shop","equals":"$id"}・notify(body「{name} にまだ行っていません」)。
`)

func init() {
	for _, l := range serverRulesLookupLines {
		serverRulesReadsSection = strings.Replace(serverRulesReadsSection, l[0], l[1], 1)
	}
	serverRulesReadsSection += "\n\n" + serverRulesLookupSection
	serverRulesAskRe = regexp.MustCompile(serverRulesAskRe.String() + `|突合|照合|名簿|載って(い)?ない|まだ行って(い)?ない`)
}
