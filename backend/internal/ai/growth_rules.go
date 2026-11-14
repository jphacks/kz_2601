package ai

import "strings"

// growthRules: when a number should be felt as something that grows (歩数で木に
// 実がなる / 歩くほど花が咲く / 貯金が増えると木が育つ), pick the growth part.
// Appended like healthLiveRules so prompt.go stays untouched; the keys come
// from the schema catalog. Every line names the part as 「…」は growth so a
// build without growth (restrict.go) drops the whole block body.
var growthRules = strings.TrimSpace(`
## 数値で育つ絵
- 「歩数で木に実がなる」「歩くと実がなる」「歩くほど花が咲く」「歩いて木を育てる」は growth(metric "steps"・**"live": true**・entity もフォームも作らない)。木なら kind tree、花なら flower、芽から育てるなら plant。target は 1 日の目標(言われなければ 8000)、「1000 歩ごとに 1 つ」のように言われたら step。
- 「貯金が増えると木が育つ」「勉強した時間で花が咲く」「記録するほど育つ」は growth(entity + aggregate sum の field、回数なら aggregate count)。その記録を足すフォームと一覧は別の画面に作る。
- 「ボタンを押すたびに育つ」「カウンタで育てる」は growth(value に回数を読む式・回数を増やすボタンは既存の書き方で)。
- 育つ見た目を頼まれていない「歩数を見たい」「達成率を見たい」は growth にしない(歩数のカード・進捗バーのまま)。
`)

// growthChatRule: the same for the chat (plain words, no part names: #258).
// The demo phrasings build at once with the usual goal instead of a question.
var growthChatRule = strings.TrimSpace(`
## 数で育つ絵
- 数が増えるほど**木に実がなる・花が咲く・芽が木に育つ**絵のカードを**作れる**。育てる数は、今日の歩数(画面を開いている間も歩くたびにその場で増える)・記録の合計や回数(貯金・勉強時間)・押した回数のどれか。
- 「歩数で木に実がなる」「歩くほど花が咲く」「歩いて木を育てる」は**聞き返さずに ready**。目標を言われなければ 1 日 8000 歩・1000 歩ごとに実(花)が 1 つ・8000 歩で収穫(満開)として、brief と transcript に「今日の歩数で木に実がなる(1000 歩ごとに 1 つ・8000 歩で収穫・歩くとその場で増える)」のように書く。
`)

func init() {
	systemPrompt += "\n\n" + growthRules
	chatEditSystemPrompt += "\n\n" + growthChatRule
	// the create prompt keeps ending with the capability index (restrict.go swaps it)
	chatCreateSystemPrompt = strings.TrimSuffix(chatCreateSystemPrompt, chatCapabilityIndex) + growthChatRule + "\n\n" + chatCapabilityIndex
}
