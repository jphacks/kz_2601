package ai

import "strings"

// jphacksProductsRules: patterns of past JPHACKS works (eval/jphacks_products_cases.json)
// that the prompt had the parts for but no mapping. Appended so prompt.go stays untouched.
var jphacksProductsRules = strings.TrimSpace(`
## 枠の予約・優先度マトリクス・制覇率・HP
- 「枠を選んで申し込む」「1 つの枠に 1 人だけ」(面談・相談・レッスン): 枠 slot{start: datetime, memo}と申し込み booking{slot: ref → slot, note}を分け(どちらも scope public)、booking に "unique":["slot"]。枠の一覧から navigate + prefillRef で申し込みフォームを開き、申し込みの一覧(list booking)も置く。
- 「大事か × 急ぎか」「優先度マトリクス」「4 象限」: task に enum importance [重要,重要でない]・urgency [急ぐ,急がない] を作り、ホームに gridTable{entity: task, columnField: urgency, rowField: importance, titleField: title}。
- 「制覇」「コンプリート」「全部のうち何 %」: マスタ(メニュー・名所)に boolean done(defaultValue false・list.item の toggle)、制覇率 = stat{format: percent, value: {"op":"div","args":[{"agg":"count","entity":"menu","filter":{"field":"done","op":"eq","value":true}},{"agg":"count","entity":"menu"}]}}。「まだの中からおすすめ」は randomPicker の filter done eq false。
- 「キャラの HP・体力・元気度」が記録で増減: 記録に integer points(良い習慣 +・悪い習慣 −。enum からなら computed if)、HP = stat.value {"op":"clamp","args":[{"op":"add","args":[50,{"agg":"sum","entity":"habit_log","field":"points","filter":{"field":"date","op":"today","value":null}}]},0,100]}。動くキャラの絵は作れないと summary で伝える。
`)

// Sent only when the request matches (on_demand_rules.go), one trigger per bullet.
func init() {
	onDemandRules(jphacksProductsRules,
		recipe("slot_booking", []string{"枠", "空きコマ", "コマ+取|予約|選|押さえ", "早い者勝ち", "先着", "押さえ", "予約", "申し込", "申込", "空き時間", "空いている時間", "空いてる時間", "時間帯+選", "ブッキング", "1人だけ", "1人まで", "一人だけ", "面談", "レッスン+選|申|取", "booking", "reserv", "appointment", "slot"},
			`"id":"slot"`, `"id":"booking"`),
		recipe("priority_matrix", []string{"マトリクス", "2軸", "4分割", "四分割", "大切+締め切り|締切|急", "マトリックス", "象限", "4つのマス", "4つに分", "マス目", "アイゼンハワー", "重要+急", "大事+急", "優先+急", "緊急+重要", "matrix", "quadrant", "eisenhower", "urgent+important"},
			`"type":"gridTable"`),
		recipe("complete_rate", []string{"制覇", "全種類", "尽く", "割合+食べた|行った|読んだ|登った|集めた", "まだ+ランダム", "コンプ", "何%", "何パーセント", "パーセント+チェック|印|行った|食べた|読んだ", "何割", "達成率", "達成度", "全部のうち", "図鑑", "割合+チェック|印", "踏破", "まだ+おすすめ", "complet+%|percent|rate", "percent+check"},
			`"format":"percent"`),
		recipe("char_hp", []string{"hp", "げんき", "元気+増|減|ふえ|へる|上が|下が|変わ", "弱る", "体力", "ライフ", "元気度", "健康度", "ゲージ", "キャラ", "ペット+増|減|上が|下が", "育成", "health point"},
			`"op":"clamp"`),
	)
}
