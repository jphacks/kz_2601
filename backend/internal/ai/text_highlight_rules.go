package ai

import "strings"

// textHighlightRules: text {{fieldId}} on form / detail screens and
// text.highlight (dsl/text_highlight.go). Appended like speechRules so
// prompt.go and catalog.go stay untouched.
var textHighlightRules = strings.TrimSpace(`
## 文の大事な語に印をつける(text.highlight)
- 「大事な部分を目立たせて」「重要な語をハイライト」「キーワードに印」「エラーの重要箇所を強調」は text の highlight。印は表示だけで、文そのものは変えない。
- 記録の文に印をつける: フォーム・詳細画面(screen.entity)に text{text: "{{message}}", highlight: {field: "keywords"}}。text の {{fieldId}} は screen.entity の画面だけで開いているレコードの値になる(他の画面では使えない)。keywords は string / text の field で、語を「,」か「、」で区切って入れる。
- AI に語を選ばせる: 同じフォームに aiButton{task: "extract", input: "message", output: "keywords", prompt: 「{{message}} から大事な語を 5 個まで、「,」区切りで」}。押すと抜き出した語に印がつく。
- 決まった語に印: highlight{words: ["期限", "金額"]}(20 個まで・どの画面でも)。
- style は marker(既定・蛍光ペン)か bold。大文字小文字・全角半角は区別せず、日本語は語の一部でも合う。
- 例 エラー文から検索の言葉と大事な部分を出す: entity error{message: text(required), keywords: string, query: string}。フォームに textField{message, multiline}・aiButton extract(output keywords)・aiButton suggest(input message, output query, prompt 「{{message}} を調べる検索の言葉を 1 行で」)・text「{{message}}」+ highlight{field: keywords}・saveRecord。一覧は list{item{title: query, subtitle: message}}、onItemTap でそのフォームを開く。
`)

func init() {
	systemPrompt += "\n\n" + textHighlightRules
}
