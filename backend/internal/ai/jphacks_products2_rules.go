package ai

import "strings"

// jphacksProducts2Rules: the second batch of past JPHACKS works
// (eval/jphacks_products2_cases.json) whose parts exist but had no mapping.
// Appended so prompt.go stays untouched.
var jphacksProducts2Rules = strings.TrimSpace(`
## 期限切れで回復する HP・各自の済み・前のタスク・同じ気分の人数・疎遠・診断
- 「タスクを終えると敵(モンスター)の HP が減り、期限切れで回復」: task{title, due: date, done: boolean defaultValue false}・HP = stat.value {"op":"clamp","args":[{"op":"add","args":[{"op":"sub","args":[100,{"op":"mul","args":[10,{"agg":"count","entity":"task","filter":{"field":"done","op":"eq","value":true}}]}]},{"op":"mul","args":[10,{"agg":"count","entity":"task","filter":[{"field":"done","op":"eq","value":false},{"field":"due","op":"lt","value":{"op":"today","args":[]}}]}]}]},0,100]}(期限切れ = 未完了かつ due が今日より前)。動く敵の絵は作れないと summary で伝える。
- 「みんなで共有する課題・やることを、終わったかは各自でつける」: 課題の entity は shared(または public)にして boolean の done を置かない(1 人が押すと全員の分が済みになる)。marks [{"id":"done","icon":"check","label":"済み"}]・list.item.mark "done"、「まだの課題」の一覧は filter {"field":"$marks.done","op":"neq","value":{"me":true}}(同じ item.mark)。
- 「前のタスクが終わってから」「順番・前後関係のある作業」: task に prev(type ref・refEntity: task 自身・任意)と done(boolean defaultValue false)、computed ready(boolean) = {"op":"and","args":[{"op":"not","args":[{"ref":"done"}]},{"op":"coalesce","args":[{"ref":"prev.done"},true]}]}(自分がまだで、前が無いか済み)。「今できる」一覧は filter {"field":"ready","op":"eq","value":true}(list の filter は 1 つなので条件は computed にまとめる)。
- 「今日、同じ気分・同じ答えの人が何人いるか」: 投稿 post{feeling: enum, date: date defaultValue now}(scope public)の computed same_today(integer) = {"agg":"count","entity":"post","filter":[{"field":"feeling","op":"eq","value":{"ref":"feeling"}},{"field":"date","op":"today","value":null}]}(自分を含む)を list.item.trailing に。全体の内訳は chart(groupBy feeling・filter date today)。
- 「しばらく連絡していない人」「疎遠を防ぐ」「最後に会って・送ってから何日」: friend{name} + 記録 card{friend: ref→friend, date}。friend の computed last_sent(date) = {"agg":"latest","entity":"card","field":"date","by":"date","filter":{"field":"friend","op":"eq","value":{"ref":"id"}}}・days_since(integer・unit 日) = {"op":"coalesce","args":[{"op":"daysBetween","args":[{"ref":"last_sent"},{"op":"today","args":[]}]},9999]}(まだ一度も送っていない友達は 9999 にして一番上へ・空のままだと desc の一番下に落ちる)。friend の一覧を days_since の desc で並べ、trailing は last_sent(未送信は空欄。9999 日を見せない)。
- 「いくつかの質問に答えると、おすすめ・タイプを診断」(選ぶ答えが決まっている): 質問ごとに enum / boolean の field、結果は computed(string)の if の入れ子 {"if":{"op":"eq","args":[{"ref":"acidity"},"好き"]},"then":"エチオピア(浅煎り)","else":"マンデリン(深煎り)"}。AI は要らない(説明の文も作るなら aiButton を足す)。答えの記録は list(item.title = 結果)で残す。
`)

// Sent only when the request matches (on_demand_rules.go), one trigger per bullet.
func init() {
	onDemandRules(jphacksProducts2Rules,
		recipe("monster_hp", []string{"モンスター", "敵", "ボス", "ダメージ", "倒", "hp", "体力+期限|締め切り|締切|タスク|課題|宿題|todo|やること", "ライフ+期限|締め切り|締切|タスク|課題|宿題|todo|やること", "回復+期限|締め切り|締切", "monster", "enemy", "boss"},
			`"op":"clamp"`),
		recipe("own_done", []string{"各自", "自分の分", "自分だけ+チェック|済|完了|終わ", "それぞれ+チェック|済|完了|終わ|読|用意", "ひとりひとり", "一人一人", "一人ひとり", "自分がまだ", "自分が+まだ", "未読+共有|みんな|全員", "個人ごと", "メンバーごと+済|完了|チェック", "each+own"},
			`"$marks.`),
		recipe("prev_task", []string{"前の作業", "先行", "完了したら", "解放", "前のタスク", "前の工程", "先にやる", "前後関係", "依存", "着手", "取りかかれ", "取り掛かれ", "始められる", "順番+タスク|作業|工程|手順", "工程", "後続", "prerequisite", "depend", "blocked"},
			`"ref":"prev.done"`),
		recipe("same_feeling", []string{"同じ気分", "おなじ+何人|人数|人の数|人が|人は|仲間", "きもち+投稿|つぶや|何人", "同じ+何人|人数|人の数|人が|人は", "同じ答え", "気分+投稿|sns|みんな", "same+people"},
			`"id":"same_today"`),
		recipe("keep_in_touch", []string{"疎遠", "最終+訪問|連絡|来店|会", "時間がたって", "最後に", "しばらく+連絡|会|話|送", "連絡していない", "連絡してない", "話していない", "会っていない", "会ってない", "間が空", "日にちが空", "久しぶり", "前回から", "keep in touch", "last contact", "haven't"},
			`"op":"daysBetween"`, `"id":"last_sent"`),
		recipe("diagnosis", []string{"診断", "質問に答え", "質問+おすすめ|判定|タイプ|合う|向いて|提案", "タイプ+判定|診断", "答え+おすすめ", "yes+no", "チャート式", "心理テスト", "性格", "quiz+recommend"}),
	)
}
