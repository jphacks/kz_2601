package ai

import "strings"

// Server-rules reads (#340 reads): agg in if / values / notify.with, on.each
// (a time trigger that runs once per record) and notify.with ({name}). The
// base prompt (server_rules_pass.go) says agg is not available and a time
// trigger has no row; init() swaps those lines and appends the vocabulary.
// server_rules_pass.go's init runs first (files initialize in name order).

// serverRulesReadsLines: base line -> replacement.
var serverRulesReadsLines = [][2]string{
	{
		`- if(省略可): 真偽の Expr(eq / neq / gt / and ...)。行はきっかけのレコード({"ref":"<field>"}・{"ref":"<ref field>.<field>"}・updated では {"ref":"$prev.<field>"} で前の値)。agg・streak・doneInDays・computed field・setting は使えない。時刻のきっかけでは ref を書かない。`,
		`- if(省略可): 真偽の Expr(eq / neq / gt / and ...)。行はきっかけのレコード({"ref":"<field>"}・{"ref":"<ref field>.<field>"}・updated では {"ref":"$prev.<field>"} で前の値)か on.each のレコード。agg は使える(下の「集計・まとめ・期限」)。streak・doneInDays・computed field・setting は使えない。each の無い時刻のきっかけでは ref を書かない。`,
	},
	{
		`updateRecord の ref 省略 = きっかけのレコード自身。どちらもレコードのきっかけだけ。`,
		`updateRecord の ref 省略 = きっかけのレコード自身。どちらもレコードのきっかけか on.each だけ(each では行のレコード)。`,
	},
	{
		`と {$author}(書いた人の表示名・shared / public のレコードのきっかけ)だけ。押すとこのアプリが開く。`,
		`と {$author}(書いた人の表示名・shared / public のレコードのきっかけ)と {<with の名前>}(notify.with の数・日付)だけ。押すとこのアプリが開く。`,
	},
}

var serverRulesReadsSection = strings.TrimSpace(`
## 集計・まとめ・期限(サーバーで記録を読む)
- agg(count / sum / avg / min / max・filter に thisWeek / thisMonth / today / thisYear や比較)は if・values・notify.with で使える。読めるのは同じアプリの**ルールと同じ側**(private どうし・shared / public どうし)の entity だけ。1 つのルールが読む entity は each と agg を合わせて 2 つまで。computed field は agg で読まない(元の field を読む)。
- 条件つきの集計: 「今月の支出が 5 万円を超えたら知らせて」= on created expenses・if {"op":"gt","args":[{"agg":"sum","entity":"expenses","field":"amount","filter":{"field":"date","op":"thisMonth","value":null}},50000]}・notify の body に {total}・with {"total": 同じ agg}。
- 定期のまとめ: 「毎週日曜の夜に今週の合計を記録して知らせて」= on {every:"weekly",weekday:"sun",time:"20:00"}・createRecord(まとめの entity に values で agg の合計・件数・{"op":"today","args":[]})・notify(body「今週は {count} 件・{total} 円でした」・with に count / total)。
- 期限のリマインド: 「締め切りの 3 日前に知らせて」= on {every:"daily",time:"08:00",each:"tasks"}・if {"op":"and","args":[{"op":"neq","args":[{"ref":"done"},true]},{"op":"eq","args":[{"op":"daysUntil","args":[{"ref":"due"}]},3]}]}・notify(body「{title} の締め切りまであと 3 日です」)。each の行はそのレコードなので {<field>}・to.kind author(shared)が使える。
- 順位: 「毎晩、歩数の順位をつけて」(shared)= on {every:"daily",time:"21:00",each:"players"}・updateRecord(ref 省略)values {"rank":{"op":"add","args":[{"agg":"count","entity":"players","filter":{"field":"score","op":"gt","value":{"ref":"score"}}},1]}}。
- with: {名前: 数か日付の Expr}(3 つまで・小文字の id・行の field と同じ名前は不可・文字は不可)。title / body で {名前} を必ず使う。生成では with は [{"name":"total","value":<Expr>}] の配列で書く。
- 読む件数には上限がある(1 回の実行・アプリの 1 日)。超えたらその回は動かない。毎日・毎週・毎月より細かくはできない。
- 状態の変化(「完了になったら作った人に知らせて」)は on {record:"updated",entity,fields:["status"]}・if {"op":"eq","args":[{"ref":"status"},"done"]} で書く(each や agg は要らない)。
- まだできない: おすすめ・相性・似ている人の推薦、複数人の割り当て・日程の自動調整、距離での自動グループ分け、外部のデータの取得。頼まれたら declined で断り、近い代わり(一覧・集計・順位・リマインド)を添える。
`)

func init() {
	for _, l := range serverRulesReadsLines {
		serverRulesPrompt = strings.Replace(serverRulesPrompt, l[0], l[1], 1)
	}
	serverRulesPrompt += "\n\n" + serverRulesReadsSection
}
