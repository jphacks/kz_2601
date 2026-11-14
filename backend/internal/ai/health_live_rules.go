package ai

import "strings"

// healthLiveRules (#3694): healthStat `live` is opt-in. Only when the user asks
// for リアルタイム / 今すぐ反映 / 歩きながら / live does the card keep counting
// while on screen; otherwise the key is omitted (default: refresh on open).
// Appended like liveActivityRules so prompt.go stays untouched; the key itself
// comes from the schema catalog. Lines naming the part use 「…」は healthStat
// so a build without healthStat (restrict.go) drops them.
var healthLiveRules = strings.TrimSpace(`
## 歩数のリアルタイム更新(live・#3694)
- 「歩数計」「今日の歩数」「毎日の歩数を見たい」は healthStat(metric steps)を既定のまま(**live は付けない**)。開いた時・画面に戻った時に取り直すだけで足りる(API を叩き続けない)。
- 「リアルタイムの歩数計」「リアルタイムで」「今すぐ反映」「歩きながら見たい」「歩くたびに増えて」「live」は healthStat(metric steps)に **"live": true** を付ける(steps だけ。weight には付けない)。
- live は開いている間だけ歩数計を読む(画面を離れると止まる)。新しい entity・ボタン・通知・タイマーは作らない。既存アプリへの「リアルタイムにして」は、ある歩数のカードの live を replace(無ければ add)で済ませる。
`)

func init() { systemPrompt += "\n\n" + healthLiveRules }
