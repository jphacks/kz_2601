package ai

import "strings"

// aiChatInputRules: aiChat inputEntity (dsl/ai_chat_input.go) lets the AI
// read the user's own records (text_chat_material.go). Appended like
// aiChatRules so prompt.go and catalog.go stay untouched; every line routes
// with 「…」は aiChat so a client without aiChat drops it (restrict.go).
var aiChatInputRules = strings.TrimSpace(`
## 自分の記録を読んで話す(aiChat inputEntity)
- 「日記を読んだうえで相談に乗ってほしい」「書いた日記をもとに AI と振り返り」「保存した問題について AI に質問したい」「取り込んだ資料(PDF)について AI に聞きたい」は aiChat に inputEntity: 記録の entity。送るたびに利用者自身の新しい記録(limit 件・既定 20・最大 20)が参考として AI に渡る。hidden の field・写真・位置は渡らない。
  - 形は aiChat の節の turn{text, role} に加えて、記録の entity(例 diary{date: date, mood: enum[よい, ふつう, つらい], body: text(required)}・problem{subject: enum, question: text, memo: text}・資料は fileImport の docs{title, body: text})。inputEntity に会話の entity(turn)を指定しない。記録を書くフォーム画面と list は別のタブに置く(aiChat の画面は A1:F12 のまま)。
  - persona に記録の読み方を書く(例「日記を読んで気持ちに寄り添う相談相手。記録に書かれた出来事に触れて 1 つずつ質問する」「保存した問題の家庭教師。答えをすぐ言わずヒントを出す」「資料に書いてあることだけで答え、無ければ無いと言う」)。
- 記録を読まない雑談・練習相手には inputEntity を付けない。ボタン 1 回で記録をまとめさせるだけなら aiButton(entity + limit)で、aiChat の inputEntity とは別。
`)

func init() { systemPrompt += "\n\n" + aiChatInputRules }
