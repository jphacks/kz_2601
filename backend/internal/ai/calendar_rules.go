package ai

import "strings"

// calendarRules: "カレンダーに入れたい" used to be answered with "できません"
// or an in-app calendar only. addToCalendar opens the system "new event"
// sheet filled from the record. Appended like randomRules so prompt.go stays
// untouched; the keys themselves come from the schema catalog.
var calendarRules = strings.TrimSpace(`
## 端末のカレンダーに入れる(addToCalendar)
- 「iPhone のカレンダーに入れたい」「Google カレンダーにも登録」「予定に追加」「締切をカレンダーに」: 予定の entity の記録フォーム(screen.entity)に **button{label: 「カレンダーに追加」, action: {type: addToCalendar, entity, calendarEvent:{startField, …}}}** を保存ボタンの近く(半幅)に置く。押すと OS の「新規イベント」画面が中身を埋めて開き、ユーザーが「追加」を押す。カレンダーの許可は要らない。**「できません」やアプリ内の calendar 部品だけで代替しない**(アプリ内の calendar 部品は月の表示で、端末のカレンダーには入らない。両方求められたら両方)。
- 項目の対応: startField は日付(date = 終日の予定)か日時(datetime = その時刻から)の field が必須。終わりの field があれば endField、無ければ durationMinutes(省略で 60 分)。titleField(省略で entity の titleField)・locationField(場所の名前の string)・notesField(メモ)は entity にある時だけ付ける。時刻つきの予定にしたい時は日付 field ではなく datetime の field を作る。
- 「前日に知らせて」「30 分前に通知」はカレンダー側の通知 alertMinutes(1440 / 30)。アプリのローカル通知(app.notifications)と二重にしない(どちらか片方。端末のカレンダーに入れる依頼なら alertMinutes)。
- 一覧から入れたい時は、一覧の行を開く(navigate)フォームに上のボタンを置く(onItemTap を addToCalendar にすると編集が開けなくなる)。
- 既存アプリへの「カレンダーにも入れられるようにして」は、その entity のフォーム画面に上のボタンを add する(entity は作り直さない)。日付の field が無ければ datetime の field を add してから。
- アラーム(止めるまで鳴り続ける目覚まし)・他のアプリの利用時間の取得・スクリーンタイムの制限は作れない。目覚ましは時刻指定のローカル通知、制限は自分で記録する形にして、summary で正直に伝える。
`)

func init() { systemPrompt += "\n\n" + calendarRules }
