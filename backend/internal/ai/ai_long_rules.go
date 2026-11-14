package ai

import "strings"

// aiLongRules: when the generator should set aiButton length long (a longer
// cloud answer, text_long.go). Appended like cloudAIRules so prompt.go and
// catalog.go stay untouched.
var aiLongRules = strings.TrimSpace(`
## アプリの中の AI の長い文章(aiButton length long)
- 答えが数文では足りない時だけ aiButton に length: long を付ける。長い文の要約(規約・論文・議事録・貼り付けた長文)、手紙・お礼状・スピーチ、旅行・学習・1 週間の計画、ラジオや動画の台本、日記の本文、「詳しく」「しっかり」と言われた助言・解説を書かせる時。一言・タグ・短い案・分類は付けない(既定の short)。
  - 形: 長い本文を貼るなら entity に body: text と結果用の text field(例 terms{body: text, risks: text})。フォームに textField(body)・aiButton{task: extract, input: body, output: risks, length: long, prompt: 「利用者に不利な点を、理由つきで全部」}・textField(risks)。
  - 書かせるなら aiButton{task: generate, length: long, prompt: 「…」}(shape は line だけ・付けない)。結果を残すなら output は text field(string は不可)。高さは 6 行以上(例 A1:F8)。
  - 読み上げたい時は、結果を入れた text field を speakButton{field: …} で読む。
- length: long は classify に付けない。材料は 1 回 16KB(日本語で約 5000 字)まで、答えは約 3000 字まで。
- length: long は端末内 AI では動かず、いつも Tailor It Pro のオンライン AI で動く。summary で料金に触れなくてよい。
`)

func init() { systemPrompt += "\n\n" + aiLongRules }
