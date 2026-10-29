package ai

import "strings"

// aiGenerateRules (#417): "a button that makes the AI write X" is aiButton
// task generate with a fixed instruction and a shape. Appended like
// randomRules so prompt.go stays untouched.
var aiGenerateRules = strings.TrimSpace(`
## AI に決まった形で書かせるボタン(#417)
- 「罰ゲームのアイデアを出してくれるボタン」「今日の献立を提案」「励ましの一言」「お題・ネタ・名前の案を考えて」は **aiButton{task: generate, prompt: 固定の指示, shape}**。押すたびに AI が新しく書き、結果はボタンの下に出る。**randomPicker や固定の文言・候補の list で代替しない**(候補を自分で用意する必要は無い)。
- prompt は利用者の目的を 1〜2 文の具体的な指示にする(500 字まで・安全で誰でもできる内容に寄せる)。例: 「飲み会や家族で盛り上がる、誰でも安全にできる軽い罰ゲームの案」。label は動詞(「罰ゲームを考えて」「献立を提案して」)。
- shape: 1 文なら line(既定)。「5 つ」「いくつか」なら list + count(1〜10・言われなければ 3)。「主菜・副菜・汁物」「場所・持ち物」のように項目名があるなら fields + items(1〜10 個)。
- 記録を材料にする(「冷蔵庫の中身から献立」「過去の日記から励まし」)なら entity(+limit ≤ 20)を指定し、その entity の一覧・フォームも作る(例 food{name, amount}・ホームに aiButton{task: generate, entity: food, prompt: 「{{input}} の食材で作れる今日の献立」, shape: fields, items: [主菜, 副菜, 汁物]})。prompt の {{input}} が材料に置き換わる。
- 結果を記録に残したい時だけ、フォーム画面(screen.entity)に置いて output(string/text field)を指定する(ドラフトに入り、保存は利用者)。表示だけなら output も screen.entity も不要。
- area は結果が見える高さを取る: line は 3 行(例 A1:F3)、list / fields は 5〜6 行(例 A1:F6)。
- 端末内 AI(Apple Intelligence)があれば無料で、無い端末では Tailor It Pro のオンライン AI で書く。summary でこの点に触れなくてよい。
`)

func init() { systemPrompt += "\n\n" + aiGenerateRules }
