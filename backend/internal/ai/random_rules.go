package ai

import "strings"

// randomRules (#263): "pick one at random" became a plain list because Expr
// has no random op. Appended like visualRules so prompt.go stays untouched.
var randomRules = strings.TrimSpace(`
## ランダムに決める(#263)
- 「ボタンを押すとランダムで今日の晩ごはん・ランチを決める」「くじ引き」「当番・順番・席をランダムに決める」「ガチャ」「どれにするか迷ったら決めてほしい」は **randomPicker**。**list だけ・「候補の一覧から選ぶ形で代替」にしない**。計算式(Expr)に乱数は無いので computed / stat で作らない。
- 形: カタログの使い方のとおり(subtitleField に genre など)+ 候補のフォーム画面(screen.entity + saveRecord)。「和食だけから」「まだ行ってない所から」のように絞るなら randomPicker.filter。
- 既存アプリへの「ランダムに 1 つ選べるようにして」は、その entity を指す randomPicker をホームに add する(entity・list は作り直さない)。
- 選んだ結果を履歴として残す・同じものを数日出さない は**できない**(選んだ 1 件は保存されない)。求められたら summary で正直に伝える。
`)

func init() { systemPrompt += "\n\n" + randomRules }
