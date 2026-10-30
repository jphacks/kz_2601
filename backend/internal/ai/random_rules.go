package ai

import "strings"

// randomRules (#263): "pick one at random" became a plain list because Expr
// has no random op. Appended like visualRules so prompt.go stays untouched.
var randomRules = strings.TrimSpace(`
## ランダムに決める(#263)
- 「ボタンを押すとランダムで今日の晩ごはん・ランチを決める」「くじ引き」「当番・順番・席をランダムに決める」「ガチャ」「どれにするか迷ったら決めてほしい」は **randomPicker**(押すたびに候補の entity から 1 件を選んで大きく出す・結果は保存しない)。**list だけ・「候補の一覧から選ぶ形で代替」にしない**。計算式(Expr)に乱数は無いので computed / stat で作らない。
- 形: 候補の entity(例 dish{name: string(required), genre: enum(任意)}・titleField name)+ ホームに randomPicker{entity, titleField: name, subtitleField?: genre, label: 「今日の晩ごはんを決める」のように動詞, onItemTap: フォームへ navigate(sheet)}(A1:F5)+ 候補の list(A6:F10・onItemTap は同じフォーム)+ 追加ボタン(D11:F12)+ 候補のフォーム画面(screen.entity + saveRecord)。「和食だけから」「まだ行ってない所から」のように絞るなら randomPicker.filter。
- 既存アプリへの「ランダムに 1 つ選べるようにして」は、その entity を指す randomPicker をホームに add する(entity・list は作り直さない)。
- 選んだ結果を履歴として残す・同じものを数日出さない は**できない**(選んだ 1 件は保存されない)。求められたら summary で正直に伝える。
`)

func init() { systemPrompt += "\n\n" + randomRules }
