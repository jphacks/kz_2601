package ai

import "strings"

// jphacksProductsRules: patterns of past JPHACKS works (eval/jphacks_products_cases.json)
// that the prompt had the parts for but no mapping. Appended so prompt.go stays untouched.
var jphacksProductsRules = strings.TrimSpace(`
## 枠の予約・優先度マトリクス・制覇率・HP
- 「枠を選んで申し込む」「1 つの枠に 1 人だけ」(面談・相談・レッスン): 枠 slot{start: datetime, memo}と申し込み booking{slot: ref → slot, note}を分け(どちらも scope public)、booking に "unique":["slot"]。枠の一覧から navigate + prefillRef で申し込みフォームを開く。
- 「大事か × 急ぎか」「優先度マトリクス」「4 象限」: task に enum importance [重要,重要でない]・urgency [急ぐ,急がない] を作り、ホームに gridTable{entity: task, columnField: urgency, rowField: importance, titleField: title}。
- 「制覇」「コンプリート」「全部のうち何 %」: マスタ(メニュー・名所)に boolean done(list.item の toggle)、制覇率 = stat{format: percent, value: {"op":"div","args":[{"agg":"count","entity":"menu","filter":{"field":"done","op":"eq","value":true}},{"agg":"count","entity":"menu"}]}}。「まだの中からおすすめ」は randomPicker の filter done eq false。
- 「キャラの HP・体力・元気度」が記録で増減: 記録に integer points(良い習慣 +・悪い習慣 −。enum からなら computed if)、HP = stat.value {"op":"clamp","args":[{"op":"add","args":[50,{"agg":"sum","entity":"habit_log","field":"points","filter":{"field":"date","op":"today","value":null}}]},0,100]}。動くキャラの絵は作れないと summary で伝える。
`)

func init() { systemPrompt += "\n\n" + jphacksProductsRules }
