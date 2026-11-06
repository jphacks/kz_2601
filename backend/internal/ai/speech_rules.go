package ai

import "strings"

// speechRules: transcription (transcribeField) and reading aloud
// (speakButton). Appended like audioRules so prompt.go and catalog.go stay
// untouched. Both run on the device only, so the review build keeps them.
var speechRules = strings.TrimSpace(`
## 文字起こし・読み上げ
- 「文字起こし」「話して文字で残す」「声で書く・声でメモ(文字で)」「議事録を声で」「声で日記」「しゃべった内容をテキストに」は entity に type "text" の field + フォーム画面(screen.entity)に **transcribeField**{field, label}(押して話すと端末の中で文字になり、その欄の後ろに足される。欄は複数行で手で直せる)。**同じ field に textEditor を重ねない**(transcribeField が欄も持つ)。**voiceInput にしない**(声でアプリを作り変える入口で、記録に残らない)。
- 形: entity 例 note{title: string(required), body: text, date: datetime(defaultValue now)}・titleField title。ホームに list{entity, item{title: title, subtitle: date}} + 追加ボタン。フォームに textField(title)・transcribeField(body, label: 「本文(話して入力)」)・datePicker(date)+ saveRecord。
- 「ボイスメモ」「録音したい」だけなら従来どおり audioRecorder(音声を残す)。「録音して文字起こしも」「音声も文字も残したい」は同じフォームに audioRecorder(audio field)と transcribeField(text field)を並べる。**録音した音声を後から文字にはできない**(文字にしたい分は transcribeField で話す)。求められたら summary で正直に伝える。
- 「読み上げ」「声で聞きたい」「音声で読んで」「発音を聞く」「手順・レシピを読み上げて」は **speakButton**(押すと端末の声で読み上げ、もう一度押すと止まる。声は UI の言語)。記録の値を読むならフォーム・詳細画面(screen.entity)に speakButton{field}(例: 単語帳の word、日記の body、レシピの steps)。固定の案内ならホームに speakButton{text}。文をつなぐなら text の {{fieldId}}(例 「{{word}}。意味は {{meaning}}」)か value の concat。label は「読み上げる」「発音を聞く」など。
- 既存アプリへの「声で入力できるようにして」は、その entity の string / text field に対して、フォームの textField / textEditor を transcribeField に replace する(field は同じ)。「読み上げられるようにして」はそのフォームに speakButton{field} を add する。
- どちらも端末の中だけで動く(声はサーバーに送らない)。音声認識のモデルはアプリの音声入力と同じもので、初回は用意(ダウンロード)が要る。
`)

var (
	transcribeFieldTerm = dslTerm{"transcribeField", "話して入力する欄", "voice-to-text field"}
	speakButtonTerm     = dslTerm{"speakButton", "読み上げボタン", "read-aloud button"}
)

func init() {
	systemPrompt += "\n\n" + speechRules
	for _, t := range []dslTerm{transcribeFieldTerm, speakButtonTerm} {
		dslTerms = append(dslTerms, t)
		termIndex[strings.ToLower(t.term)] = t
	}
}
