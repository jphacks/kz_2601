package ai

import "strings"

// feedWarningsPressureRules: the warnings (気象庁の警報・注意報) and pressure
// (48-hour pressure forecast) feeds. Like feedRules, every line contains
// "」は feed" so a build without feed drops them all.
var feedWarningsPressureRules = strings.TrimSpace(`
- 「警報・注意報」「大雨警報が出ているか」「防災・災害の情報」「〇〇県の警報」は feed{provider: warnings, region: 府県予報区のコード, title: 〇〇の警報・注意報, emptyText: 今出ている警報・注意報はありません}(全幅)。region の決め方は天気と同じ(言われなければ 130000)。市町村ごとの警報・地震・津波・避難指示・「警報が出たら通知」はできないので summary で正直に伝える。避難所の地図や投稿の一覧と並べてよい。
- 「気圧の予報」「気圧の変化」「頭痛(天気痛)が起きそうな時」「低気圧が来るか」は feed{provider: pressure, region: 府県予報区のコード, title: 〇〇の気圧, limit: 8}(48 時間を 6 時間ごとに最低 / 最高と急な低下の印で出す)。feed は予報(今の気圧をその場で測るのはセンサーの部品)。頭痛・体調の記録と並べてよいが、予報の値を記録に保存・グラフ・通知の条件に使うことはできない(表示だけ)。
`)

func init() { systemPrompt += "\n\n" + feedWarningsPressureRules }
