package ai

import "strings"

// aiSaveToRules: aiButton generate list + saveTo (dsl/ai_save_to.go) saves
// each generated item as its own record. Appended like textHighlightRules so
// prompt.go and catalog.go stay untouched.
var aiSaveToRules = strings.TrimSpace(`
## AI が作った項目を 1 件ずつ記録に足す(aiButton saveTo)
- 「問題を作って flashcards に」「単語カードを AI に作らせて足したい」「手順を書き出して一覧に」「アイデアを出してリストに貯めて」は aiButton{task: generate, shape: list, count, saveTo: {entity, field}}。AI の箇条書き 1 項目 = その entity の 1 件(field に入る)。利用者が結果を見て「全部保存」か 1 つずつ保存する(勝手には保存しない)。
- field は entity の string / text field。ほかの field は defaultValue か空で保存されるので、required は field だけにする(boolean・enum・date は自動で入る)。count は 1〜10(言われなければ 5)。
- 足し先は private か shared の entity(public・single・match・addressedTo には保存できない)。editableBy admins の field も不可。output とは一緒に使わない(saveTo は記録を足す・output はフォームに入れる)。
- 足した記録を見る部品も置く: 暗記なら flashcards{entity, front: field, back, toggle}、手順・アイデアなら list。
- 例 問題を作って暗記カードに: entity card{question: string(required), answer: text, learned: boolean}。ホームに aiButton{task: generate, label: 「問題を作る」, prompt: 「中学英語の単語の問題(英単語の意味を問う)」, shape: list, count: 5, saveTo: {entity: card, field: question}, area A1:F6}・flashcards{entity: card, front: question, back: answer, toggle: learned}。答えは利用者がカードを開いて書く。
- 例 記録を材料に: 授業ノート note{body: text} から問題 → aiButton{entity: note, prompt: 「{{input}} から確認の問題」, shape: list, saveTo: {entity: card, field: question}}。
`)

func init() { systemPrompt += "\n\n" + aiSaveToRules }
