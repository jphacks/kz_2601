package ai

import "strings"

// quizRules (#262): quizzes had scores typed in by hand and the answer key
// shown to everyone. Appended like visualRules so prompt.go stays untouched.
var quizRules = strings.TrimSpace(`
## クイズ・テストの自動採点(#262)
- 「クイズ」「テスト」「問題を出し合う」に点数・正解・採点・競うが付いたら、**点数を手入力させない**。questions{text 問題文, choices 選択肢(text), correct 正解(enum A/B/C/D・**hidden:true**), points 配点(integer)}・players{name}・answers{question: ref→questions, player: ref→players, choice: enum A/B/C/D} を作る。友達・みんなで遊ぶなら 3 つとも scope public。
- 正誤と点数は answers の computed(計算式カタログのレシピ「クイズの自動採点」): is_correct = eq(choice, question.correct)、score = if(is_correct, question.points, 0)。ランキングは players の computed total = agg sum answers.score(filter player eq ref id)の table を total で sort desc。
- **正解(hidden の field)は一覧・表・グラフ・titleField に出さない**。正解を写すだけの computed も作らない(hidden は比較の中でだけ読める)。回答の一覧は is_correct / score を出す。正解は問題のフォームで作者が選ぶ。
- 回答は「回答する」ボタン(navigate sheet)で answers のフォームを開き、picker で問題・回答者・選択肢を選ぶ。**問題の一覧の onItemTap は問題のフォーム**にする(answers のフォームを開かない)。
- hidden は画面に出さないだけで値は端末に届く。description に「正解は回答者の一覧に出しません」とは書いてよいが、「誰にも見られない」とは書かない。
`)

func init() { systemPrompt += "\n\n" + quizRules }
