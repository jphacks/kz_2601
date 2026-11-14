package ai

import "strings"

// alarmRules: "止めるまで鳴る目覚まし" was answered with a one-shot banner.
// Notification.alarm makes the same notification ring until stopped
// (iOS 26.1+ AlarmKit); older iOS / no permission / the review build post a
// normal notification at the same time, so no supportedFeatures gate is
// needed. Appended like repeatRules so prompt.go / catalog.go stay untouched.
var alarmRules = strings.TrimSpace(`
## 止めるまで鳴るアラーム(alarm)
- 「目覚まし」「アラーム」「7時に起こして」「止めるまで鳴らして」「寝坊しないように」: 通知に **alarm: true** を付ける。iPhone のアラームと同じく、ロック中も全画面で鳴り、止めるまで鳴り続ける。ユーザーが時刻を決める・変えるアラームは上の「レコードの時刻から」の形(string + timePicker の "HH:mm" の field・repeat daily・titleField にラベル・enabledField にオン/オフ)に alarm: true を足す。固定の時刻なら {id, title, body, repeat: daily / weekly / once, time, weekday?, alarm: true}。
- alarm は**起こす・止めるまで気づかせたい時だけ**。服薬・予定の N 分前・更新日などのリマインドは普通の通知(alarm を付けない)。when(条件の通知)には付けない(backend が拒否する)。
- アラームが使えない端末(古い iOS・アラームの許可が無い)では同じ時刻の普通の通知になる。summary で「止めるまで鳴る」と約束しすぎない(「アラームで知らせます」程度)。
- 既存アプリへの「アラームにして」「止めるまで鳴るようにして」: 該当する app.notifications に add /app/notifications/<i>/alarm(true)。新しい通知を足さない。
`)

// alarmChatRule: the same capability for chat answers (できる?).
var alarmChatRule = strings.TrimSpace(`
## 止めるまで鳴るアラーム
- 「目覚まし」「止めるまで鳴るアラーム」は**できる**(iPhone のアラームと同じく止めるまで鳴る。時刻は固定でも、アプリの中で毎日の時刻を決めて変える形でもよい。古い iOS では普通の通知になる)。通知だけだと思って断らない。
`)

func init() {
	systemPrompt += "\n\n" + alarmRules
	// right after the hand-written capabilities (the create prompt must keep
	// ending with the capability index); keeps chatCapabilities intact for
	// the other rules files' Replace.
	with := chatCapabilities + "\n\n" + alarmChatRule
	chatEditSystemPrompt = strings.Replace(chatEditSystemPrompt, chatCapabilities, with, 1)
	chatCreateSystemPrompt = strings.Replace(chatCreateSystemPrompt, chatCapabilities, with, 1)
}
