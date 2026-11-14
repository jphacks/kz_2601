package ai

import "strings"

// drawingRules: handwriting / sketches with PencilKit. Appended like
// audioRules so prompt.go and catalog.go stay untouched.
var drawingRules = strings.TrimSpace(`
## 手書き・お絵かき(drawingField)
- 「手書きで書きたい」「お絵かき」「スケッチ」「サイン(署名)をもらう」「図・間取り・地図を描いて残す」「手書きのメモ・日記」「ぬり絵」は entity に type "photo" の field + フォーム画面(screen.entity)に **drawingField**{field, label: 「手書き」など}。**textEditor・photoField で代替しない**(写真を撮るのではなく、その場で描く)。
- 形: entity 例 sketch{title: string(required), drawing: photo, date: datetime(defaultValue now)}・titleField title。ホームに list{entity, item{title: title, subtitle: date, image: drawing}} + 追加ボタン(D11:F12)。フォームに textField(title)・drawingField(drawing・全幅 4〜6 行 例 A2:F6)・datePicker(date)+ saveRecord。
- 既存アプリへの「手書きでも書けるようにして」は、その entity に photo field を add し、そのフォームに drawingField を add する(entity・画面は作り直さない)。
- できないこと: 手書きの文字を文字(テキスト)にする・描いた線を後から 1 本ずつ直す(保存すると 1 枚の画像になる。開き直すとその上に描き足せる)。求められたら summary で正直に伝える。
`)

var drawingFieldTerm = dslTerm{"drawingField", "手書きの欄", "drawing pad"}

func init() {
	systemPrompt += "\n\n" + drawingRules
	dslTerms = append(dslTerms, drawingFieldTerm)
	termIndex[strings.ToLower(drawingFieldTerm.term)] = drawingFieldTerm
}
