package ai

import "strings"

// countWhereRules: the prompt taught a parent's count of its children
// (parent_child.go) but not counting only the children with one value, so
// 「候補日ごとに何人参加できるか」 could come out as the count of all answers.
// The filter list (AND) is already in the catalog's agg grammar; this gives
// the recipe (docs/dsl/examples/schedule_poll.json).
var countWhereRules = strings.TrimSpace(`
## 親ごとに、子のうち特定の値だけ数える(日程調整・出欠)
- 「候補日ごとに何人参加できるか」「練習日ごとの参加人数」「問題ごとの正解数」は、親の computed(integer・unit「人」など)の agg count の filter を**配列(AND)**にして、親を指す条件と値の条件を並べる: {"agg":"count","entity":"answers","filter":[{"field":"day","op":"eq","value":{"ref":"id"}},{"field":"status","op":"eq","value":"参加できる"}]}(answers.day は候補日への ref、status は enum。value は options と同じ文字列)。
- 親の条件だけの count(回答の総数)を「参加人数」として出さない。未定・参加できないの数も同じ形で value を変えた computed にする。親の一覧の list(onItemTap でフォーム)の trailing に出す(数が 2 つ以上なら table の列にしてよい)。
- 値ごとの人数を求められていない時は足さない。
`)

func init() { systemPrompt += "\n\n" + countWhereRules }
