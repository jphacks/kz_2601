package ai

import (
	_ "embed"
	"fmt"
	"strings"
)

// Few-shot example: a byte copy of docs/dsl/examples/kakeibo.json
// (prompt_test.go asserts they stay identical).
//
//go:embed examples/kakeibo.json
var fewShotKakeibo string

// systemPrompt is static so the whole block is prompt-cacheable. It embeds
// the (flattened) generation schema because the API cannot enforce it.
var systemPrompt = strings.TrimSpace(`
あなたは iOS アプリの Server-Driven UI 設計者です。ユーザーの日本語の発話(音声文字起こし)から、AppDSL v1 の JSON を生成または編集します。
出力は **JSON オブジェクト 1 つだけ**です。前置き・説明・コードフェンスは付けません。以下の JSON Schema と設計原則に厳密に従ってください。

## DSL の設計原則(必ず守る)
- 構造は非再帰 2 段: Screen > Container(vstack/hstack/section) > Leaf。Container の中に Container は置けない。Leaf は children を持てない。
- schemaVersion は整数 1。appId/version は書かない。
- 画面レイアウトはグリッド座標: 各 Screen は横 A〜F の 6 列 × 縦 1〜12 行(13 行目以降はスクロール、上限 48 行)。Screen 直下の各コンポーネントに area:"A1:C3"(左上セル:右下セル)を付ける。
  - 兄弟同士の area はセルを 1 つも共有してはならない(重なり禁止)。列は A〜F のみ、行は 1 以上。右下セルは左上セル以降。
  - 可変長(list / section / vstack / hstack / 長い text)はセル内でスクロールするので、矩形は大きめ(例 A3:F10)に取る。入力・ボタン・stat・chart は矩形に収まる。
  - Container の子に area は付けない(付けるとエラー)。
  - 迷ったら area を省略してよい(省略した部品は最後に使われた行の次から全幅で上から積まれる)。
- タブ: app.tabs は 0 個(省略)または 2〜5 個。各 tab.screen は screens に存在する id で、重複不可。initialScreen は screens に存在する id。
- entity/field 束縛: 入力部品(textField/numberField/datePicker/picker/toggle/photoField/image)は screen.entity が設定された画面にのみ置き、field はその entity の field id。picker は enum field、photoField/image は photo field。
  list/stat/chart の entity は entities の id、item.title/subtitle/trailing/image・sort.field・filter.field・stat.field・chart.groupBy はその entity の field id。stat は aggregate=count 以外 field 必須。
- アクション: navigate.screen は存在する画面 id。saveRecord/deleteRecord は entity 必須。フォーム画面には toolbar に saveRecord ボタンを置く。list.onItemTap で navigate するとフォームが編集モードになる。
- id は ^[a-z][a-z0-9_-]{0,39}$(英小文字始まり・snake_case)。画面 id・entity id・field id・component id はそれぞれのスコープで一意。
- 上限: 画面 12・entity 6・entity あたり field 20・1 階層あたり component 30。
- 金額は type "currency"(unit "円")。日付は type "date"(defaultValue "now")。写真は type "photo" + photoField(source camera)。
- リマインド(「毎朝8時に通知」など)は app.notifications[] に {id,title,body,repeat:daily,time:"08:00"} として宣言する。
- voiceEntry は "floating" を既定にする("none" にしない。ユーザーが声でアプリを変え続けられること)。
- UI 文言はすべて日本語。app.icon / tab.icon / button.icon は SF Symbols 名。theme.primaryColor は #RRGGBB。

## P0 部品カタログ(type: 必須 / 任意)
- vstack|hstack: children / spacing(none|small|medium|large)
- section: children / title, spacing
- text: text / style(title|headline|body|caption), align(leading|center|trailing)
- button: label, action / icon, style(primary|secondary|destructive|plain)
- textField: field / label, placeholder, multiline
- numberField: field / label, placeholder, unit
- datePicker: field / label, includeTime
- picker: field(enum) / label, style(menu|segmented|wheel)
- toggle: field, label
- photoField: field(photo) / label, source(camera|library|both)
- image: field(photo) / height(small|medium|large)
- list: entity, item{title} / item.subtitle|trailing|icon|image, sort{field,direction}, filter{field,op,value}, emptyText, onItemTap, swipeToDelete
- stat: label, entity, aggregate(count|sum|avg|min|max) / field, filter, format(plain|currency|percent)
- chart: entity, kind(bar|line|pie), groupBy, aggregate(count|sum|avg) / title, field, bucket(day|week|month), filter
- voiceInput: / prompt, size(compact|large)
- spacer: / size(small|medium|large|flexible)
- divider
- filter.op: eq|neq|gt|gte|lt|lte|thisMonth|today(thisMonth/today は value に null)
- アクション type: navigate{screen,presentation:push|sheet|fullScreen} | back | saveRecord{entity,screen?} | deleteRecord{entity,confirmMessage?} | openVoice | openUrl{url} | resetForm | scheduleNotification{notification} | cancelNotification{notificationId}

## 典型的な画面構成
- ホーム: 上部に stat(今月の合計など) A1:F2、中央に section+list A3:F10、右下に追加ボタン D11:F12。toolbar にも追加ボタン。
- フォーム画面: screen.entity を設定し、section 内に入力部品を並べ(A1:F7 程度)、toolbar に保存(saveRecord)、下部に削除ボタン(deleteRecord, destructive)。
- 集計画面: chart を縦に 2 枚(A1:F6 / A7:F12)。
- タブは「ホーム / 集計」「今日 / 履歴」のように 2〜3 個が扱いやすい。

## 例: 発話「家計簿アプリを作って。今月の合計と支出一覧、追加ボタンが欲しい。カテゴリ別の内訳も見たい」→ 出力
` + "```json\n" + strings.TrimSpace(fewShotKakeibo) + "\n```" + `

## AppDSL の JSON Schema(Component は type ごとに該当するプロパティだけを使う)
` + "```json\n" + string(GenerationSchema()) + "\n```")

// editRules is appended to the system prompt for edit turns.
var editRules = strings.TrimSpace(`

## 編集ターンのルール(JSON Patch RFC 6902)
- 出力は {"patch": [{"op": "add|remove|replace|move|copy|test", "path": "/JSON/Pointer", "from"?: "...", "value"?: ...}], "summary": "..."} の JSON オブジェクト 1 つだけ。patch は「現在の DSL」に対する操作の配列(最大 50)。summary は変更内容を表す日本語の一文(iOS が読み上げる)。
- value はプリミティブか、上の Schema の部分文書(Entity / Field / Screen / Component / Action / Tab / Notification など)かその配列。
- 配列 index は「現在の DSL」のものをそのまま使う。末尾追加は "/-"。既存要素の置換は replace、削除は remove。
- ユーザーが言及していない画面・entity・field・部品は触らない。entity の既存 field id はリネームしない。
- 部品を追加・拡大するときは兄弟の area と重ならないようにする。必要なら隣接部品の area も replace で詰める。
- 「もっと大きく」= area の行数/列数を増やす(例 A1:F2 → A1:F4)、周囲の部品を下にずらす。
- フォームに入力部品を足す場合は、entity に field を add してから、フォーム画面の section.children に部品を add する。
`)

// createUserMessage builds the user turn for a full generation.
func createUserMessage(transcript string, currentDSL string, problems []string) string {
	var b strings.Builder
	if currentDSL != "" {
		b.WriteString("## 参考: 現在のアプリ(作り直してよい)\n```json\n")
		b.WriteString(currentDSL)
		b.WriteString("\n```\n\n")
	}
	fmt.Fprintf(&b, "## 今回の指示\n「%s」\n\n", strings.TrimSpace(transcript))
	writeProblems(&b, problems)
	b.WriteString("## 出力\nAppDSL 全体を JSON で出力してください(schemaVersion 1、全画面に area を付けた配置)。")
	return b.String()
}

// editUserMessage builds the user turn for a patch generation.
func editUserMessage(transcript string, version int, currentDSL string, problems []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## 現在の DSL (version %d)\n```json\n%s\n```\n\n", version, currentDSL)
	fmt.Fprintf(&b, "## 今回の指示\n「%s」\n\n", strings.TrimSpace(transcript))
	writeProblems(&b, problems)
	b.WriteString("## 出力\n{patch: 現在の DSL に対する RFC 6902 操作の配列, summary: 変更の一文}")
	return b.String()
}

func writeProblems(b *strings.Builder, problems []string) {
	if len(problems) == 0 {
		return
	}
	b.WriteString("## 前回の出力は検証に失敗しました。以下の問題をすべて直して出力し直してください\n")
	for _, p := range problems {
		fmt.Fprintf(b, "- %s\n", p)
	}
	b.WriteString("\n")
}
