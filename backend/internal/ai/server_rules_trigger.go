package ai

import (
	"regexp"
	"strings"
	"sync"
)

// The server-rules pass is a second model call, so its trigger asks for a
// server signal, not a bare time or notify word: 「毎日」「通知して」「自動で」
// alone are on-device notifications or screen work (about a quarter of the
// eval transcripts matched them). The pass runs when the request names
// another person to reach, someone else's action, a threshold, a server-side
// write, or one of the vocab words.

// serverRulesAskBase is the base alternation of serverRulesAskRe
// (server_rules_pass.go). Keep it in sync: TestServerRulesTriggerTightens
// fails when it no longer matches.
const serverRulesAskBase = `通知|知らせ|教えて|届[くけい]|促|リマインド|毎朝|毎晩|毎日|毎週|毎月|自動|数え|カウント|送っ|送信|送る|(?i:notif|remind|alert|let me know|every (day|morning|night|evening|week|month)|daily|weekly|monthly|automatic|count|\bsend)`

// serverRulesAskCore replaces serverRulesAskBase.
const serverRulesAskCore = `` +
	// a person to reach + a notify verb
	`((みんな|全員|各自|作成者|その人|私|家族|友達|仲間|チーム|パートナー|先生|部員|スタッフ|相手|メンバー|参加者)(全員|みんな)?|(作った|登録した|書いた|投稿した|選んだ)(人|私))に[^。]{0,12}(通知|知らせ|届|促|リマインド|教え)|宛先|` +
	// someone else's action
	`誰かが|(メッセージ|投稿|書き込み|コメント|返信|いいね|回答|申し込み|申込|出欠)が(来|届|あ|つ|付|入)|状態が変わ|` +
	// a threshold to watch
	`(超え|下回っ|上回っ)たら[^。]{0,4}(通知|知らせ|教え)|` +
	// server-side writes, sends and removals
	`促|送って|送信|(の|回)数を数え|カウントして|自動で(記録|割|付け|順位)|通知(は|を)(やめ|止め|消|外|オフ)|` +
	// a scheduled summary written to a record
	`毎(朝|晩|日|週|月)[^。]*(合計|件数|平均|順位|集計)[^。]*(記録|残|つけ)|` +
	`(?i:\b(notify|remind|alert|tell|ping|send|message)( to)? (the owner|everyone|everybody|all (the )?members|the members|members|the team|others)|someone (posts|adds|writes|replies|joins)|new (message|post|comment|reply))`

// serverRulesAskTighten: alternation -> tighter form. The vocab files add
// their branches in their own init; a pair whose branch is not there is a
// no-op.
var serverRulesAskTighten = [][2]string{
	{serverRulesAskBase, serverRulesAskCore},
	{`|シフト|当番|`, `|`},                    // pick assign: shift tables, duty lots
	{`|日程(を)?(調整|決め)`, ``},               // pick assign: an RSVP poll
	{`|ランダム|`, `|毎(朝|晩|日|週)[^。]*ランダムに|`}, // pick: a random button
	{`|献立を(決め|考え|提案)|`, `|献立を決め|`},       // pick: AI menu ideas
	{`|気圧|`, `|気圧を[^、。]{0,4}残|`},         // saveFeed: a pressure feed or sensor
	{`|名簿|`, `|名簿に(無|な)い|`},              // lookup: a roster import
}

func tightenServerRulesAsk(re string) string {
	for _, p := range serverRulesAskTighten {
		re = strings.Replace(re, p[0], p[1], 1)
	}
	return re
}

// serverRulesAsk is serverRulesAskRe tightened. It is built on first use,
// after every init has run, so it does not depend on the order the vocab
// files append in.
var serverRulesAsk = sync.OnceValue(func() *regexp.Regexp {
	return regexp.MustCompile(tightenServerRulesAsk(serverRulesAskRe.String()))
})
