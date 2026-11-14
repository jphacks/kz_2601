package ai

import "strings"

// aiOutputsRules: aiButton generate fields + outputs (dsl/ai_outputs.go)
// fills one form field per item. Appended like aiSaveToRules so prompt.go
// and catalog.go stay untouched.
var aiOutputsRules = strings.TrimSpace(`
## AI の答えを項目ごとに入力欄へ入れる(aiButton outputs)
- 「貼ったポスター/メールの文から題名・日時・場所を抜き出して予定に」「答えを採点して点数を入れて」「文から金額と日付を読んで記録に」は、フォーム画面(screen.entity)の aiButton{task: generate, shape: fields, items: [項目名…], outputs: {項目名: field id}, input: 貼る文の text field}。AI の答えが項目ごとに型を直して入力欄に入り、利用者が確認して保存する(勝手には保存しない)。
- outputs の field は string / text / integer / number / date / datetime / enum(computed・boolean・photo は不可)。キーは items の名前と同じにする。読めない値(enum の options に無い値も)は空のまま。output とは一緒に使わない。時刻つきなら datetime、点数・金額は integer / number、決まった分類は enum。
- prompt に何をどう抜き出す/採点するかを書く(例: 「{{poster}} からイベントの題名・開始日時・場所を抜き出す」「{{question}} への答え {{answer}} を 100 点満点で採点し、講評を一言」)。貼る文の field を input にする。
- 例 ポスターから予定: entity event{poster: text, title: string(required), start: datetime(required), place: string}。フォームに textField poster(複数行)・aiButton{task: generate, label: 「文から入力」, input: poster, prompt: 「{{poster}} からイベントの題名・開始日時・場所を抜き出す」, shape: fields, items: [題名, 日時, 場所], outputs: {題名: title, 日時: start, 場所: place}, area A3:F8}・textField title・datePicker start・textField place・button{label: 「カレンダーに追加」, action: {type: addToCalendar, entity: event, calendarEvent: {startField: start, titleField: title, locationField: place}}}。
- 例 採点: entity answer{question: string, answer: text, score: integer, comment: string}。aiButton{task: generate, label: 「採点する」, prompt: 「{{question}} への答え {{answer}} を 100 点満点で採点し、講評を一言」, shape: fields, items: [点数, 講評], outputs: {点数: score, 講評: comment}}。
`)

func init() { systemPrompt += "\n\n" + aiOutputsRules }
