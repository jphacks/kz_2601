package ai

import "strings"

// jphacksProducts4Rules: the fourth batch of past JPHACKS works
// (eval/jphacks_products4_cases.json) whose parts exist but had no mapping.
// Appended so prompt.go stays untouched.
var jphacksProducts4Rules = strings.TrimSpace(`
## レベルに合わせた言い換え・会議の振り返り・種類ごとの係数・今日の曜日・必要なカロリー・する日としない日の比べ
- 「自分の英語力・学年・レベルに合わせて言い換える」(難しい語だけ日本語・やさしい日本語): reading{original: text(required), level: enum [中学生, 高校生, 大学生], result: text}のフォームに textField(original・multiline)・picker(level)・aiButton{task: rewrite, input: original, output: result, prompt: 「次の英文を、英語力が {{level}} くらいの人が読めるように、難しい語だけ日本語に置き換えた日英まじりの文にする: {{input}}」}・textField(result・multiline)。**prompt に {{level}} などの field を入れる時は {{input}} も必ず書く**(無いと端末内 AI に本文が渡らない)。長い文なら length: long。開いている Web ページをその場で書き換えることはできない(文は貼る)と summary で伝える。
- 「会議の話を文字にして、AI が論点や話が止まっていた所を振り返る」: meeting{title: string(required), date: datetime(defaultValue now), transcript: text, summary: text}(参加者で見るなら shared)のフォームに textField(title)・transcribeField(transcript)・aiButton{task: summarize, input: transcript, output: summary, length: long, prompt: 「会議の文字起こし {{input}} を、論点・決まったこと・結論が出ないまま同じ話が続いた所(停滞)に分けて書く」}・textField(summary・multiline)。会議の途中で AI が自動で割り込むことはできないと summary で伝える。
- 「種類ごとに決まった量をかける」(ゴミの種類ごとの CO2 削減量・ポイント・料金): 記録 trash{kind: enum [ペットボトル, 缶, 紙, プラ], count: integer(defaultValue 1), date: date(defaultValue now), photo: photo}に computed co2(number・unit g) = {"op":"mul","args":[{"ref":"count"},{"if":{"op":"eq","args":[{"ref":"kind"},"ペットボトル"]},"then":30,"else":{"if":{"op":"eq","args":[{"ref":"kind"},"缶"]},"then":40,"else":{"if":{"op":"eq","args":[{"ref":"kind"},"紙"]},"then":10,"else":20}}}]}(係数は例・頼まれた値に)。今月の合計 = stat.value {"agg":"sum","entity":"trash","field":"co2","filter":{"field":"date","op":"thisMonth","value":null}}。写真から種類を入れるならフォームの photoField に analyze [{kind: classify, into: kind}] と picker(kind)。
- 「今日の曜日の時間割・持ち物・当番だけを出す」: lecture{subject, weekday: enum [月, 火, 水, 木, 金], period: integer, room: string, items: string}に computed wd(integer) = {"if":{"op":"eq","args":[{"ref":"weekday"},"月"]},"then":2,"else":{"if":{"op":"eq","args":[{"ref":"weekday"},"火"]},"then":3,"else":{"if":{"op":"eq","args":[{"ref":"weekday"},"水"]},"then":4,"else":{"if":{"op":"eq","args":[{"ref":"weekday"},"木"]},"then":5,"else":{"if":{"op":"eq","args":[{"ref":"weekday"},"金"]},"then":6,"else":0}}}}}(weekday の op と同じ 1 = 日 … 7 = 土。土日も使うなら足す)を作り、list に filter {"field":"wd","op":"eq","value":{"op":"weekday","args":[{"op":"today","args":[]}]}} と sort period asc、item{title: subject, subtitle: room, trailing: items}。filter の value に曜日の名前を if で入れ子にしない(式が深すぎて通らない)。週全体は別の画面の gridTable。通知の文は決まった文で、その日の持ち物を通知に入れることはできない。
- 「体の情報から 1 日に必要なカロリーを出す」(献立・食事の目安): 自分の情報 body{sex: enum [男性, 女性], age: integer, height: number(unit cm), weight: number(unit kg), activity: enum [低い, ふつう, 高い]}に computed kcal(integer・unit kcal) = {"op":"round","args":[{"op":"mul","args":[{"op":"add","args":[{"op":"sub","args":[{"op":"add","args":[{"op":"mul","args":[10,{"ref":"weight"}]},{"op":"mul","args":[6.25,{"ref":"height"}]}]},{"op":"mul","args":[5,{"ref":"age"}]}]},{"if":{"op":"eq","args":[{"ref":"sex"},"男性"]},"then":5,"else":-161}]},{"if":{"op":"eq","args":[{"ref":"activity"},"低い"]},"then":1.5,"else":{"if":{"op":"eq","args":[{"ref":"activity"},"ふつう"]},"then":1.75,"else":2}}]}]}(基礎代謝 × 活動の係数)。今日の残り = stat.value {"op":"sub","args":[{"agg":"latest","entity":"body","field":"kcal"},{"agg":"sum","entity":"meal","field":"kcal","filter":{"field":"date","op":"today","value":null}}]}。献立は aiButton{task: generate, entity: body, limit: 1, prompt: 「{{input}} の人が 1 日にとる目安のカロリーに合う献立」, shape: fields, items: [朝, 昼, 夜]}。食べ物のカロリーの推定はできない(手で入れる)。
- 「○○した日としなかった日で比べる」(エナドリ・コーヒー・運動と作業の出来・睡眠): 1 日 1 件の記録 day{date: date(defaultValue now), drinks: integer(defaultValue 0), sleep: number(unit 時間), score: integer(1〜5 の出来)}に、飲んだ日の平均(小数 1 桁) = stat.value {"op":"div","args":[{"op":"round","args":[{"op":"mul","args":[{"agg":"avg","entity":"day","field":"score","filter":{"field":"drinks","op":"gt","value":0}},10]}]},10]}・飲まなかった日の平均 = 同じ形で filter {"field":"drinks","op":"eq","value":0}。chart で日ごとの score を出し、AI の助言は aiButton{task: generate, entity: day, limit: 14, prompt: 「{{input}} の記録から、飲み方と睡眠についての助言」}。比べるだけで、原因かどうかは分からないと summary で伝える。
`)

// Sent only when the request matches (on_demand_rules.go), one trigger per bullet.
func init() {
	onDemandRules(jphacksProducts4Rules,
		recipe("level_rewrite", []string{"言い換え", "書き換え", "書きかえ", "やさしい日本語", "やさしく", "易しく", "わかりやすく+直|書", "分かりやすく+直|書", "分かる言葉", "レベルに合わせ", "英語力", "学年+合わせ", "難しい単語", "難しい語", "難しい言葉", "英文", "日英", "rewrite", "paraphrase", "simplify"},
			`"task":"rewrite"`),
		recipe("meeting_review", []string{"会議", "ミーティング", "打ち合わせ", "打合せ", "議事録", "論点", "文字起こし", "停滞", "meeting", "transcri"},
			`"type":"transcribeField"`),
		recipe("kind_coef", []string{"co2", "二酸化炭素", "削減", "種類ごと", "係数", "リサイクル", "単価", "排出量", "エコ", "ゴミ+種類|分別", "分別", "carbon", "emission"},
			`"id":"co2"`),
		recipe("today_weekday", []string{"曜日", "時間割", "持ち物", "当番", "月曜+火曜|水曜|木曜|金曜", "weekday", "timetable", "day of the week"},
			`"op":"weekday"`),
		recipe("need_kcal", []string{"カロリー", "エネルギー", "摂る", "kcal", "代謝", "摂取", "献立", "身長+体重", "calorie", "bmr"},
			`"id":"kcal"`),
		recipe("did_vs_didnt", []string{"日と+比|違|差", "なかった日", "しなかった日", "ない日", "抜いた日", "なかった+比|違|差", "した日+比", "エナドリ", "エナジードリンク", "比べ+日", "比較+日", "compare+days"},
			`"id":"drinks"`),
	)
}
