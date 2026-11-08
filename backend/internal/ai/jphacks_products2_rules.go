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
- 「しばらく連絡していない人」「疎遠を防ぐ」「最後に会って・送ってから何日」: friend{name} + 記録 card{friend: ref→friend, date}。friend の computed last_sent(date) = {"agg":"latest","entity":"card","field":"date","by":"date","filter":{"field":"friend","op":"eq","value":{"ref":"id"}}}・days_since(integer・unit 日) = {"op":"daysBetween","args":[{"ref":"last_sent"},{"op":"today","args":[]}]}。friend の一覧を days_since の desc で並べる。
- 「いくつかの質問に答えると、おすすめ・タイプを診断」(選ぶ答えが決まっている): 質問ごとに enum / boolean の field、結果は computed(string)の if の入れ子 {"if":{"op":"eq","args":[{"ref":"acidity"},"好き"]},"then":"エチオピア(浅煎り)","else":"マンデリン(深煎り)"}。AI は要らない(説明の文も作るなら aiButton を足す)。答えの記録は list(item.title = 結果)で残す。
`)

func init() { systemPrompt += "\n\n" + jphacksProducts2Rules }
