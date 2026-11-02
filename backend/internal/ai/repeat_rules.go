package ai

import "strings"

// repeatRules (#266): a subscription's "day before the renewal" notification
// was repeat once (silent after the first renewal) and days_left went
// negative. Record-based notifications now repeat monthly / yearly and
// nextOccurrence gives the next renewal / birthday. Appended like
// retentionRules so prompt.go / chat_first.go stay untouched.
var repeatRules = strings.TrimSpace(`
## 毎月・毎年くり返す日(サブスクの更新日・誕生日・記念日・#266)
- 「サブスクの更新日の前日に通知」「毎月の支払日・引き落とし日を知らせて」「誕生日・記念日に通知」: entity に日付 field(date + datePicker・例 renew_on / birthday)を置き、app.notifications に {id, title, body, **repeat: "monthly"(毎月)/ "yearly"(毎年)**, entity, field: <日付 field>, offsetMinutes(当日 0・前日 -1440・3 日前 -4320・1 週間前 -10080), titleField: <名前の field>, time?: "09:00"} を 1 件書く。**repeat once にしない**(更新日を過ぎると翌月から鳴らない)。scheduleNotification ボタンを並べて代用しない。
- monthly / yearly は**レコードの date / datetime field 専用**(固定 time だけの通知・scheduleNotification・"HH:mm" の string field には使えない。backend が拒否する)。毎月は日、毎年は月日でくり返す。日が無い月は月末に寄せる(31 日 → 2/28・4/30、2/29 生まれ → 平年 2/28)。
- 次の日・残り日数は **nextOccurrence**: computed next(date) = {"op":"nextOccurrence","args":[{"ref":"renew_on"},"monthly"]}、days_left(integer・unit 日) = {"op":"daysUntil","args":[{"op":"nextOccurrence","args":[{"ref":"renew_on"},"monthly"]}]}。daysUntil(ref renew_on) だけだと更新日を過ぎてマイナスになる。誕生日・記念日は "yearly"(次の誕生日までの日数)。第 2 引数は文字列 "monthly" / "yearly" だけ。
- 一覧は days_left の昇順(近い順)に並べ、item に days_left を出す。月額の合計は stat sum。登録する日付は「次の更新日」でも「契約した日・生まれた日」でもよい(どちらでも同じ日にくり返す)。
- 既存アプリへの「毎月通知して」「毎年にして」: 該当する app.notifications の repeat を replace で monthly / yearly にする(entity・field は変えない)。days_left が daysUntil(ref 日付) なら nextOccurrence を挟む replace をする。
`)

// repeatChatRule: the same capability for chat answers (できる?).
var repeatChatRule = strings.TrimSpace(`
## 毎月・毎年の通知(#266)
- 「サブスクの更新日の前日に毎月通知」「毎年の誕生日・記念日に通知」「次の更新日までの日数」は**できる**(記録の日付から毎月・毎年くり返す通知と、次の日付の計算。日が無い月は月末に寄せる)。通知が「1 回・毎日・毎週」だけだと思って断らない。
`)

func init() {
	systemPrompt += "\n\n" + repeatRules
	// right after the hand-written capabilities (the create prompt must keep
	// ending with the capability index). Inserting after chatCapabilities
	// keeps its text intact, so retention_rules.go's Replace still matches.
	with := chatCapabilities + "\n\n" + repeatChatRule
	chatEditSystemPrompt = strings.Replace(chatEditSystemPrompt, chatCapabilities, with, 1)
	chatCreateSystemPrompt = strings.Replace(chatCreateSystemPrompt, chatCapabilities, with, 1)
	t := dslTerm{"nextOccurrence", "次の日付", "next date"}
	dslTerms = append(dslTerms, t)
	termIndex[strings.ToLower(t.term)] = t
}
