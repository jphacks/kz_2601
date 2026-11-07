package ai

import "strings"

// fileImportAudioRules: "授業の録音ファイルを文字にしたい" used to be answered
// with transcribeField (speak again) or 「できません」. fileImport accept audio
// transcribes a picked recording on the device. The file name sorts after
// file_import_rules.go, so this section follows it in the prompt.
var fileImportAudioRules = strings.TrimSpace(`
## 録音ファイルの文字起こし
- 「授業・会議・インタビューの録音ファイル(ボイスメモ・m4a・mp3・wav)を文字にしたい」「録った音声を議事録・ノートにしたい」は **fileImport{entity, accept: [audio], field: 本文の text field}**(ファイル選択で録音を選ぶと、端末の中で文字起こしして 1 件足す。titleField(string)にはファイル名が入る)。label は「録音ファイルを文字にする」など。**transcribeField(その場で話す)や「できません」で代替しない**。
- entity は 例 lecture{title: string, transcript: text, added: date(defaultValue now)}(titleField: title)。field は type text だけ(string は不可)。要約・要点は同じ entity のフォームに aiButton{task: summarize, input: transcript}。PDF も入れたいなら accept: [pdf, text, audio] にまとめてよい。
- 1 ファイル 30 分・200MB まで。文字起こしはアプリが前面にある間だけ進み、途中でやめられる。音声も文字もサーバーや AI には送らず、音声ファイルは残さない(後から聞き返すなら別に audioRecorder で録る)。話者の区別・時刻付きの字幕・動画の音声は無い。
- その場で話して文字にするなら transcribeField、録音を残すなら audioRecorder のまま。
`)

func init() { systemPrompt += "\n\n" + fileImportAudioRules }
