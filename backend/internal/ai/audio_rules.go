package ai

import "strings"

// audioRules (#420): voice memos / meeting recordings. Appended like
// randomRules so prompt.go and catalog.go stay untouched.
var audioRules = strings.TrimSpace(`
## 録音・ボイスメモ(#420)
- 「録音したい」「ボイスメモ」「声でメモを残す」「会議・授業・商談を録音」「音声で残す」は entity に type "audio" の field + フォーム画面(screen.entity)に **audioRecorder**{field, label: 「録音」}(録音・停止・再生・録り直し・削除ができ、録音中は経過時間と音量のメーターが出る)。**textEditor だけ・文字起こしで代替・voiceInput にしない**(voiceInput は声でアプリを作り変える入口で、録音は残らない)。
- 形: entity 例 memo{title: string(required), voice: audio, note: text(任意), date: datetime(defaultValue now)}・titleField title。ホームに list{entity, item{title: title, subtitle: date, trailing: voice}}(trailing の audio field は行の右端に再生ボタンと長さを出す)+ 追加ボタン(D11:F12)。フォームに textField(title)・audioRecorder(voice)・textEditor(note)・datePicker(date)+ saveRecord。
- 「会議を録音してメモと一緒に残したい」は同じフォームに audioRecorder と textEditor(メモ)を並べる(録音とメモを 1 件のレコードに)。
- maxDuration は秒(既定 600 = 10 分・最大 7200)。「1 時間の会議」など長さが言われたら合わせる(3600)。言われなければ省略。
- 既存アプリへの「録音もできるようにして」は、その entity に audio field を add し、そのフォームに audioRecorder を add する(entity・画面は作り直さない)。
- イヤホン(AirPods など)のボタンで録音の開始・停止ができる(録音の部品がある画面を開いている間)。
- できないこと: 文字起こし(録音を文字にする)・他の端末での再生(音声は録音した端末にだけ保存し、他の端末では長さだけ出る)。求められたら summary で正直に伝える。
`)

var audioRecorderTerm = dslTerm{"audioRecorder", "録音の欄", "voice recorder"}

func init() {
	systemPrompt += "\n\n" + audioRules
	dslTerms = append(dslTerms, audioRecorderTerm)
	termIndex[strings.ToLower(audioRecorderTerm.term)] = audioRecorderTerm
}
