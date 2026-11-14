package ai

import (
	"fmt"
	"strings"
)

// appDefault is one row of the "よくある依頼の既定値" table: what to fill in
// when a common request leaves the items, units, totals, goal or reminder
// time unsaid. The chat uses it to build instead of asking (clarify_rules.go),
// generate uses it to fill the same gaps. Plain words only: the chat prompts
// must not teach DSL names (#258).
type appDefault struct {
	kind   string // the request, as people say it
	record string // items to record (unit)
	home   string // what the home screen shows (goal when asked without a number)
	notify string // reminder time when one is asked for without a time
}

var appDefaults = []appDefault{
	{"家計簿・支出・出費", "金額(円)・カテゴリ(食費・日用品・交通・娯楽・その他)・日付・メモ", "今月の合計・支出の一覧(予算の額が無ければ月3万円)", "21:00"},
	{"買い物リスト", "品名・個数(既定 1)・買った(チェック)", "まだ買っていない物の一覧(行の丸で買った)", "17:00"},
	{"ToDo・やること", "やること・期限(日付)・済み(チェック)", "未完了の一覧(期限の近い順)", "09:00"},
	{"習慣・毎日やること", "習慣の名前・その日やった(毎日リセットのチェック)", "今日できた数・習慣ごとの連続日数", "21:00"},
	{"服薬・薬", "薬の名前・飲む量・飲む時刻・その日飲んだ(毎日リセットのチェック)", "今日まだ飲んでいない数・薬の一覧", "薬ごとの時刻(無ければ 08:00 と 20:00)"},
	{"水分・水を飲む", "量(ml・既定 200)・日時", "今日の合計・今日の一覧(目標の量が無ければ 1日2000ml とその残り)", "10:00・15:00・20:00"},
	{"筋トレ・トレーニング", "種目(腕立て伏せ・スクワット・腹筋・懸垂。言われた種目を優先)・回数(回)・セット数(セット)・日付。ジム・ベンチプレスなら重量(kg)も", "今日の合計回数・記録の一覧", "19:00"},
	{"体重", "体重(kg・小数 1 桁)・日付", "今の体重・最初からの増減・記録の一覧", "07:00"},
	{"睡眠", "寝た時刻・起きた時刻・睡眠時間(計算)・日付", "直近7日の平均睡眠時間・記録の一覧(目標が無ければ 7時間)", "22:30"},
	{"勉強・学習時間", "科目(英語・数学・国語・理科・社会。言われた科目を優先)・時間(分)・日付・メモ", "今日の合計(分)・今週の合計・記録の一覧(目標が無ければ 1日60分)", "20:00"},
	{"読書", "タイトル・著者・読み終えた日・感想", "今月読んだ冊数・本の一覧", "21:00"},
	{"日記", "日付・本文(長文)。気分・写真は言われた時だけ", "新しい順の一覧", "22:00"},
	{"貯金", "金額(円)・日付・メモ", "合計(目標の額が無ければ 10万円とその残り)", "21:00"},
}

// timeWords: a reminder asked with only a part of the day.
const timeWords = "時刻が「朝」「夜」だけなら 朝 07:00・昼 12:00・夕方 18:00・夜 21:00・寝る前 22:00(en: morning 07:00・noon 12:00・evening / night 21:00・before bed 22:00)。「N 時間おき」は 08:00〜22:00 の間に N 時間おきの毎日の通知を並べる(6 件まで)。"

// appDefaultsTable renders the table as Markdown.
func appDefaultsTable() string {
	var b strings.Builder
	b.WriteString("| 依頼 | 記録する項目(単位) | ホームに出すもの | 通知を頼まれて時刻が無い時 |\n|---|---|---|---|\n")
	for _, d := range appDefaults {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", d.kind, d.record, d.home, d.notify)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// appDefaultsRules (generate): fill what the request left unsaid from the
// table instead of guessing differently each time (or dropping a reminder
// that came without a time, #257 notify omission).
var appDefaultsRules = "## よくある依頼の既定値(察し)\n" + strings.TrimSpace(`
- 依頼が下の表の種類で、記録する項目・単位・ホームの集計・目標の数・通知の時刻が発話で決まっていなければ、表の値で埋める。**発話にある値が常に優先**(「目標は1.5リットル」「夜10時に」)。表に無い種類も同じ考え方で、よくある形に埋める。
- 表は**頼まれたことの中身を埋めるため**のもの。発話に無い機能(グラフ・集計タブ・写真・通知・目標)を表を理由に足さない(上の「発話に無い機能を足さない」が優先)。目標の数は目標・残りを頼まれた時だけ、通知の時刻は通知・リマインドを頼まれた時だけ使う。
- **通知を頼まれて時刻が無い時は、通知を省かずに表の時刻で入れる**(「忘れないように通知して」で通知が 0 件にならないようにする)。`+timeWords+`
- 選択肢(カテゴリ・種目・科目)は表の候補を初期値にし、使う人が後から増やせる形を前提に 4〜6 個にとどめる。
- en の依頼も同じ表を使う(ラベル・選択肢は自然な英語に訳す。金額は円のまま)。
`) + "\n\n" + appDefaultsTable()

// appDefaultsChat: the same table for the chat prompts (the clarify rules
// point at it).
var appDefaultsChat = "## よくある依頼の既定値(聞かずに埋める)\n" + strings.TrimSpace(`
- 依頼の種類が下の表にあれば、言われていない項目・単位・集計・目標の数・通知の時刻は表で埋める(発話にある値が優先)。brief の記録する項目に単位つきの項目名(「量(ml)」)、見たいものに埋めた集計・目標の数・通知の時刻(「今日の合計と1日2000mlまでの残り」「毎日21時に通知」)を書き、transcript にも同じ中身を書く(ready の要約カードにそのまま出るので、使う人は作る前に何で埋めたか分かる)。
- 表は中身を埋めるためのもの。言われていない機能(グラフ・写真・通知・目標)は足さない。
- `+timeWords) + "\n\n" + appDefaultsTable()

func init() { systemPrompt += "\n\n" + appDefaultsRules }
