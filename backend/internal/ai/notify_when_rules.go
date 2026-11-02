package ai

import "strings"

// notifyWhenRules (#265): "在庫が少なくなったら教えて" is app.notifications[].when,
// a boolean screen-scalar Expr (agg, no ref) that iOS evaluates after a record
// save / delete / sync and posts once when it turns true. prompt.go still says
// condition notifications are impossible; init() swaps that line (and
// restrict.go swaps it back for builds without FeatureNotificationWhen).
var notifyWhenRules = strings.TrimSpace(`
## 条件で知らせる通知は app.notifications の when(#265)
- 「在庫が少なくなったら教えて」「5個を下回ったら知らせて」「予算を超えたら通知」「目標に届いたら教えて」: app.notifications に {id, title, body, **repeat: "once", when: 条件の式**} を 1 件書く。when は stat.value と同じ画面の式(**ref は書けない・記録は agg で集計**)で、真偽になる比較(lt / lte / gt / gte / eq・and / or)。when を書いたら time・weekday・entity・field・offsetMinutes・titleField・enabledField は書かない(時刻やレコードの日付の通知とは組み合わせない。backend が拒否する)。
- 在庫(閾値は言われた数・無ければ 5): 品目ごとの在庫が computed field(例 items.stock = 入荷の sum − 出荷の sum)なら「下回った品目が 1 つ以上」= {"op":"gt","args":[{"agg":"count","entity":"items","filter":{"field":"stock","op":"lt","value":5}},0]}。1 品目だけ・全体の数なら {"op":"lt","args":[{"agg":"sum","entity":"moves","field":"qty"},5]}(入荷を正・出荷を負で記録)。
- 予算オーバー: {"op":"gt","args":[{"agg":"sum","entity":"expenses","field":"amount","filter":{"field":"date","op":"thisMonth","value":null}},50000]}(予算を記録で持つなら 50000 の代わりに {"agg":"latest","entity":"settings","field":"budget"})。
- 目標達成: {"op":"gte","args":[{"agg":"sum","entity":"<記録>","field":"<量>"},<目標>]}(今月・今週なら agg に filter thisMonth / thisWeek)。
- 鳴るのは条件が「満たしていない → 満たした」に変わった時に 1 回だけ(満たしたまま記録しても鳴らない。満たさなくなってから、また満たすと鳴る)。title / body に条件を書く(「在庫が 5 個を下回りました」「今月の予算を超えました」)。
- 確かめるのは**この端末で記録を保存・削除・同期した直後**。ほかの人の操作だけで条件を満たした時にすぐ届けるのは今はできない(この端末が開いて同期した時に届く)。共有(shared / public)の entity に使ったら、app.description(編集は summary)に「ほかの人の記録で条件を満たした時は、アプリを開いた時に知らせます。」と添える。
- 不足の一覧・残りの stat などの画面は通知と一緒に作ってよい。「条件で知らせる通知は今はできない」と申告しない・代替しない。
- 編集ターン: 「少なくなったら教えて」は add /app/notifications/-(notifications が無ければ add /app/notifications に配列)、閾値の変更は replace /app/notifications/<i>/when、「その通知をやめて」は remove /app/notifications/<i>。
`)

// notifyWhenChatRule: the same capability for chat answers (できる?).
var notifyWhenChatRule = strings.TrimSpace(`
## 条件で知らせる通知(#265)
- 「在庫が N 個を下回ったら」「予算を超えたら」「目標に届いたら」知らせるのは**できる**(この端末で記録を保存・削除・同期した時に条件を確かめ、満たした時に 1 回だけすぐ知らせる)。「時刻かレコードの日付の通知だけ」と思って断らない。ほかの人の操作だけで満たした時にすぐ届けるのは今はできない(アプリを開いて同期した時に届く)。
`)

// noNotifyWhenChatRule replaces notifyWhenChatRule for builds that cannot
// evaluate when (sends allowedComponents without FeatureNotificationWhen).
const noNotifyWhenChatRule = "## 条件で知らせる通知(#265)\n- このアプリの版では、条件(在庫が減ったら・予算を超えたら等)で知らせる通知はできない。頼まれたら「この版では条件で知らせる通知はできません。最新版に更新すると使えます」と答え、不足の一覧などで近い形を添える。"

// condNotifyCannotLine is prompt.go's "できないことの前提" line; condNotifyCanLine replaces it.
const (
	condNotifyCannotLine = "  - **条件で知らせる通知**(在庫が N 個を下回ったら・残りが減ったら)はできない(通知は決まった時刻かレコードの時刻だけ)。不足の一覧などで代替し「○○の通知は今はできないため、△△で代替しました。」と申告する。"
	condNotifyCanLine    = "  - **条件で知らせる通知**(在庫が N 個を下回ったら・予算を超えたら・目標に届いたら)は**できる**: 下の「条件で知らせる通知は app.notifications の when」の形で書く。できないのは、ほかの人の操作だけで条件を満たした時にすぐ届けることだけ(この端末が同期した時に届く)。"
)

func init() {
	systemPrompt = strings.Replace(systemPrompt, condNotifyCannotLine, condNotifyCanLine, 1)
	systemPrompt += "\n\n" + notifyWhenRules
	// right after the hand-written capabilities (the create prompt must keep
	// ending with the capability index); keeps chatCapabilities intact for
	// the other rules files' Replace.
	with := chatCapabilities + "\n\n" + notifyWhenChatRule
	chatEditSystemPrompt = strings.Replace(chatEditSystemPrompt, chatCapabilities, with, 1)
	chatCreateSystemPrompt = strings.Replace(chatCreateSystemPrompt, chatCapabilities, with, 1)
	// no dslTerm for "when": it is an ordinary English word in chat answers
}
