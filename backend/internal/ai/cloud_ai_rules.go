package ai

import "strings"

// cloudAIRules: the AI inside a generated app (aiButton) also runs on the
// cloud for Pro users whose device has no on-device AI (/v1/text, text_task.go).
// Tells the generator which aiButton form fits "要約して / 添削 / 分類 /
// AI に提案" so it does not fall back to fixed text. Appended like
// randomRules so prompt.go stays untouched.
var cloudAIRules = strings.TrimSpace(`
## アプリの中の AI(要約・添削・分類・提案・相談)
- 記録の文を AI で処理する依頼は aiButton をフォーム(screen.entity)に置き、結果を output(string / text field)に入れる。保存は利用者。
  - 「要約して」「まとめて」= task summarize、「添削」「清書」「読みやすく直して」= rewrite、「分類して」「ジャンル分け」= classify(output が enum か options 2+)、「大事な所を抜き出して」= extract、「アドバイス」「一言コメント」「AI に提案してもらう」= suggest。
  - 材料は input(本文の field 1 つ)か、何も付けずにフォーム全体。口調・観点の指定は prompt(例: 「おばあちゃんの口調で、危ない点を 1 つ」)。
  - 形の例: 日記 diary{body: text, summary: text, mood: enum[良い, 普通, 悪い]} のフォームに textField(body)・aiButton{task: summarize, input: body, output: summary}・textField(summary)・aiButton{task: classify, input: body, output: mood}・picker(mood)。
- 「AI に相談」「AI と話す」「AI が質問に答える」は aiChat(上の節・何往復も話して会話を残す)。
- aiChat が使えない版では、相談を 1 件ずつの記録にする: talk{question: text(required), answer: text, date: datetime(defaultValue now)}。フォームに textField(question)・aiButton{task: generate, entity: talk, limit: 10, prompt: 「これまでの相談(下の材料)を踏まえて、次の相談に答えて: {{question}}」, output: answer, label: AI に聞く}・textField(answer)。ホームに talk の list(新しい順・question と answer)。
- 「写真を見て」は写真の項目の analyze(describe / classify・端末内)を使う。写真を AI に見せて説明・相談するなら aiButton の inputPhoto(下の節)。
- AI の処理は、端末内 AI(Apple Intelligence)があれば端末で、無い端末では Tailor It Pro のオンライン AI で動く。材料は 1 回 4KB ほど(日本語で約 1300 字)までと、inputPhoto の写真 1 枚。音声・ファイルの中身は読めない。summary で料金に触れなくてよい。
`)

func init() { systemPrompt += "\n\n" + cloudAIRules }
