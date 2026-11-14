package ai

import "strings"

// aiChatSaveToRules: aiChat saveTo (dsl/ai_chat_save_to.go) turns what the
// user tells the AI into a proposed record (text_chat_suggest.go). Appended
// like aiChatRules so prompt.go and catalog.go stay untouched; every line
// routes with 「…」は aiChat so a client without aiChat drops it (restrict.go).
var aiChatSaveToRules = strings.TrimSpace(`
## 話しかけて記録する(aiChat saveTo)
- 「AI に話しかけるだけで記録してくれる」「『今日カレー食べた』と言えば食事を記録」「会話で家計簿をつけたい」「話した内容を記録に残したい」は aiChat に saveTo{entity, fields}。AI が返事と一緒に記録の案(fields の値)を出し、吹き出しの下に「この内容で記録しますか?」(追加 / 直す)。利用者が「追加」を押した時だけ 1 件保存する(勝手には保存しない)。
  - 形は aiChat の節の turn{text, role} に加えて、記録用の別の entity(例 meal{dish: string(required), date: date, kind: enum[朝食, 昼食, 夕食, 間食]}・private か shared)。fields は AI が埋める field id 1〜8 個(string / text / number / date / enum だけ。金額や回数は number)。fields に入れない required field は defaultValue を付ける。会話の entity(turn)を saveTo に指定しない。
  - aiChat の画面は A1:F12 のまま他の部品を置かない。記録を見る list・stat は別のタブの画面に置き、記録のフォーム画面と追加ボタンも置いてよい(手で直す・足す用)。persona に「食事を聞いて記録を手伝う相棒」など記録の役を書く。
- 「AI に相談したい」「英会話の練習」など記録の話が無い会話には saveTo を付けない。ボタン 1 回で書かせた結果を 1 件ずつ保存するのは aiButton の saveTo(entity, field)で、aiChat の saveTo とは別。
`)

func init() { systemPrompt += "\n\n" + aiChatSaveToRules }
