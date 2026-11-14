package ai

import "strings"

// aiInputFeedRules: aiButton generate + inputFeeds (dsl/ai_input_feed.go) also
// reads the rows of feeds on the same screen. Like feedRules, every line
// contains "」は feed" so a build without feed drops them all.
var aiInputFeedRules = strings.TrimSpace(`
- 「天気と気圧から体調のアドバイス」「今日の天気に合う服装を AI に」「警報を見て今日の注意点を」「次の祝日の過ごし方を提案」は feed と aiButton{task: generate, prompt, inputFeeds: [その feed の id]} を同じ画面に置く(feed には id を付ける・1〜3 個)。AI は画面に読み込んだ行(合わせて最大 20 行・2KB)を材料に読む。
- 「AI が天気を見て〜」のように feed の内容を AI に読ませる時だけ」は feed の id を inputFeeds に書く(表示だけなら付けない)。task は generate だけ。entity + limit の記録と一緒に読ませてもよい。体調・健康は断定させず「目安」として書かせる。
- 例「今日の天気と気圧から体調のアドバイス(金沢)」は feed{id: wx, provider: weather, region: 170000, title: 石川県の天気, limit: 3, area A1:F5}・feed{id: hpa, provider: pressure, region: 170000, title: 石川県の気圧, limit: 8, area A6:F12}・aiButton{task: generate, label: 体調のアドバイス, prompt: 「天気と気圧の変化から今日の体調の注意点と過ごし方」, shape: list, count: 3, inputFeeds: [wx, hpa], area A13:F18}。
`)

func init() { systemPrompt += "\n\n" + aiInputFeedRules }
