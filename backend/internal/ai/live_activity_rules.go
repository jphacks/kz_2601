package ai

import "strings"

// liveActivityRules (#423): "show it on the lock screen" for timers and
// "now doing X" states. Appended like randomRules so prompt.go stays
// untouched; the keys themselves come from the schema catalog.
var liveActivityRules = strings.TrimSpace(`
## ロック画面・Dynamic Island(ライブアクティビティ・#423)
- 「タイマーをロック画面に出したい」「画面を消しても残り時間を見たい」「Dynamic Island に出して」: その timerDisplay に **"liveActivity": true** を付けるだけ。開始で出て、一時停止で止まった時間、リセット・終了で消える。アプリを閉じても時間は進む。新しい entity・ボタン・通知は作らない(終わった時の通知は timerDisplay が自動で出す)。
- 「◯◯中(運転中・ランニング中・行列に並び中・勉強中)をロック画面に出したい」: 開始ボタン {type: startActivity, activity:{title: 「運転中」のように状態, icon?: SF Symbols, minutes?: 目安の分数(残り時間を数える・省くと経過時間), stat?: 一緒に出す stat / progressBar の id}} と終了ボタン {type: endActivity} を**2 つ並べて**置く(例 A6:C7 と D6:F7)。stat を出すならその stat に id を付ける。
- 既存アプリへの「ロック画面に出して」: 既存の timerDisplay があれば replace .../liveActivity(無ければ add)。無ければ startActivity / endActivity のボタン 2 つを add。部品や entity を作り直さない。
- ホーム画面・ロック画面のウィジェット(数字の小窓)は、アプリの stat の先頭 3 つが**自動で**出る。「ウィジェットに◯◯を出したい」は、その stat をホーム画面の先頭に置く(新しい部品は要らない)。
- 外からの更新(サーバーからの push)は無い。数字は記録を足した時に端末が更新する。
- 裏に回っても(画面ロック・別アプリ)続くのは、位置の継続記録と、タイマーの時間(ロック画面の表示と終了通知)だけ。カメラ・録画・QR 読み取り・音声入力は止まる。「裏でも撮り続けて」「画面を消しても録画」と頼まれたら部品はそのまま作り、summary で「カメラは画面を出したまま使ってください(自動ロックをオフに)」と正直に伝える。
`)

func init() { systemPrompt += "\n\n" + liveActivityRules }
