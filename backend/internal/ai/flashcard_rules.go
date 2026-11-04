package ai

import "strings"

// flashcardRules (#264): "vocabulary cards, flip for the meaning" became a
// disclosure (honest substitution). Appended like randomRules so prompt.go
// stays untouched.
var flashcardRules = strings.TrimSpace(`
## 単語カード・暗記カード(#264)
- 「英単語・単語帳・暗記カード・フラッシュカード」「めくると意味・答えが出る」「一問一答で覚える」「覚えたかどうか記録」「覚えていないものだけ出題」は **flashcards**(1 枚ずつ出し、タップで表裏をめくり、右スワイプ=覚えた・左=まだ を boolean に保存)。**disclosure・list・carousel で代替しない**・「折りたたみで代替」と書かない。
- 形: カードの entity(例 word{term: string(required), meaning: string(required), learned: boolean(defaultValue false)})+ ホームに flashcards{entity, front: term, back: meaning, toggle: learned, filter: {field: learned, op: eq, value: false}}(A1:F7)+ カードの list(A8:F10・item{title: term, subtitle: meaning, toggle: learned}・onItemTap はフォームへ navigate(sheet))+ 追加ボタン(D11:F12)+ カードのフォーム画面(screen.entity + saveRecord)。
- 「覚えていないものだけ」「覚えたものは出さない」は filter {field: toggle の field, op: eq, value: false}。覚えた・まだを記録しない(ただめくるだけ)なら toggle と filter を省く。科目・単元で分けたいなら enum field を足して filter を使う(filter は 1 つ)。
- 点数を競う・正解を選ぶクイズ(友達と出し合う・採点)は「クイズ・テストの自動採点」の形。flashcards は自分で覚えるため(一問一答・暗記)。
- 既存アプリへの「カードでめくって覚えられるようにして」は、その entity を指す flashcards を add する(entity・list は作り直さない)。覚えた記録の boolean が無ければ field を add する。
- 間隔反復(忘れた頃にもう一度出す・次の出題日の計算)・出題順のシャッフル・正答率の記録は**できない**。求められたら summary で正直に伝える。
`)

func init() { systemPrompt += "\n\n" + flashcardRules }
