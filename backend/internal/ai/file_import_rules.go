package ai

import "strings"

// fileImportRules: "CSV を取り込みたい / PDF を読み込んで" used to be answered
// with a manual form or 「できません」. Appended like exportRules so prompt.go
// stays untouched.
var fileImportRules = strings.TrimSpace(`
## ファイルの取り込み(CSV・テキスト・PDF)
- 「CSV を取り込みたい」「Excel の名簿・一覧を入れたい」「書き出した CSV を戻したい」は **fileImport{entity, accept: [csv]}**(1 行 = 1 件で記録を足す。1 行目の項目名が field の name か id と同じ列に入る)。「PDF・テキストを読み込んで本文を残したい」「配られた資料・取扱説明書の文を入れたい」は **fileImport{entity, accept: [pdf, text], field: 本文の text field}**(本文を取り出して 1 件足す。titleField(string)にはファイル名が入る)。**手入力のフォームだけや「できません」で代替しない**。
- 置き場所: 記録の一覧の下の全幅(例 一覧 A1:F8 + fileImport A9:F10 + 追加ボタン D11:F12)。label は「名簿の CSV を取り込む」「PDF を取り込む」のように何を入れるかを書く。
- CSV の列名が field の name と違うと分かっている時だけ map: [{column: 列名, field: field id}](例「氏名」の列 → name)。分からなければ map を省き、field の name を表計算の 1 行目と同じ言葉にする。
- text / pdf の entity は 例 doc{title: string, body: text, added: date(defaultValue now)}(titleField: title)。本文の field は type text(20000 字まで)。title・body 以外の required は付けないか defaultValue を書く。要約したいと言われたら、同じ entity のフォームに aiButton{task: summarize, input: body} を足す。
- 取り込みは端末の中だけで、ファイルそのものは送らない・残さない。entity の scope が public のものと、設定の 1 件を持つ entity(single: true)には使えない(shared は可)。
- 音声・動画・画像・Word・Excel(.xlsx)のファイルそのものは読めない(Excel は「CSV で保存」してもらう)。PDF の質問に答える AI チャットはできない(本文を記録に入れて aiButton で要約する形にし、summary で正直に伝える)。
- 既存アプリへの「CSV から取り込めるようにして」は、記録の一覧がある画面に fileImport を add する(entity・list は作り直さない)。
`)

func init() { systemPrompt += "\n\n" + fileImportRules }
