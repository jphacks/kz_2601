package ai

import "strings"

// dashcamRules (#410): a drive recorder used to come out as a videoField
// form. Appended like randomRules so prompt.go stays untouched. Every line
// naming the part carries 「…」は dashcam, so a review build (#245) drops it.
var dashcamRules = strings.TrimSpace(`
## ドラレコ・運転中の録画(#410)
- 「ドラレコ」「ドライブレコーダー」「車載カメラ」「運転中に録画」「スマホスタンドに置いたら自動で録画」「事故・急ブレーキの時の映像を残す」は dashcam(アプリに 1 つだけ)。**videoField・list で代替しない**。
- 「置いたら自動で」「スタンドに置いたら」「乗ったら勝手に」は dashcam の autoStart [landscape, still]、「充電したら」「ケーブルを挿したら」は dashcam の autoStart に charging を足す。「音も」は dashcam の audio true。「10 分残す」は keepMinutes 10、「容量は 1GB まで」は maxStorageMB 1000、「感度を上げる」は impactG を小さく(既定 2.5)。言われなければ省略(既定値)。
- 既存アプリへの「ドラレコを付けて」は dashcam を置いた画面を 1 つ add してタブに足す(既存の画面・entity は作り直さない)。
- 「アプリを閉じても録画」「画面を消しても録画」「前と後ろを同時に」「速度や位置を映像に入れる」「クラウドに自動で上げる」は dashcam でも**できない**(iOS は画面を出している間だけカメラで録画できる)。求められたら summary で正直に伝える。
`)

func init() { systemPrompt += "\n\n" + dashcamRules }
