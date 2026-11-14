package ai

import "strings"

// weatherLogRules: a daily history of the weather / pressure is a plain
// record entity the server-rules pass fills with saveFeed
// (server_rules_vocab_savefeed.go). The line is separate from the feed rules
// (it does not need the feed component, so it stays when feed is restricted).
var weatherLogRules = strings.TrimSpace(`
- 「毎日の気圧・天気・警報を記録に残して体調と比べたい」「気圧の履歴」は、記録の entity(day:date・note:string・low / high:number・falling:boolean)と list / chart で履歴を見せる画面を作る(この entity の入力フォームは作らない)。毎日の値はサーバーの自動の動き(serverRules の saveFeed)が足す。feed の部品は予報の表示だけで、その値は保存できない。
`)

func init() { systemPrompt += "\n\n" + weatherLogRules }
