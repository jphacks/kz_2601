package ai

import (
	"regexp"
	"strings"
)

// Server-rule overlap (lookup op overlaps / notOverlaps: periods that
// overlap, dsl/server_rules_overlap.go). This init runs after
// server_rules_vocab.go (files initialize in name order) and appends its
// section to the reads section and the prompt it was appended to.

var serverRulesOverlapSection = strings.TrimSpace(`
## 期間の重なり(lookup の overlaps)
- rule.lookup {"op":"overlaps|notOverlaps","entity":"<探す entity>","field":"<開始>","fieldEnd":"<終了>","equals":"<行の開始>","equalsEnd":"<行の終了>"}: 探す entity のレコードの期間(field〜fieldEnd)が、行(きっかけか on.each のレコード)の期間(equals〜equalsEnd)と重なるものがある / 無い時だけ動く。field・fieldEnd・equals・equalsEnd は date か datetime の field(開始と終了は同じ型。$id・computed は不可)。fieldEnd / equalsEnd を省くとその日 / その時刻だけ。
- date はその日全体(終わりの日も含む)、datetime は開始から終了の前まで(10:00〜11:00 と 11:00〜12:00 は重ならない)。開始が空・終了が開始より前の記録は数えない。同じ entity なら行のレコード自身は数えない。
- 予約の重なり: 「予約が重なったら知らせて」(bookings{title, start:datetime, end:datetime})= on {record:"created",entity:"bookings"}・lookup {"op":"overlaps","entity":"bookings","field":"start","fieldEnd":"end","equals":"start","equalsEnd":"end"}・notify owner(body「{title} がほかの予約と重なっています」)。印を付けるなら on {every:"daily",time:"06:00",each:"bookings"} に同じ lookup・updateRecord values [{"field":"clash","value":true}]。
- 作業できない日: 「作業期間に休みの日が入っていたら教えて」(tasks{name, from:date, due:date}・days_off{day:date})= on {every:"daily",time:"07:00",each:"tasks"}・lookup {"op":"overlaps","entity":"days_off","field":"day","equals":"from","equalsEnd":"due"}・notify。
- 重なりを見つけて知らせる・印を付けるだけ。空いている時間を探して自動で予定を組む・ずらすことはできない。
`)

func init() {
	serverRulesReadsSection += "\n\n" + serverRulesOverlapSection
	serverRulesPrompt += "\n\n" + serverRulesOverlapSection
	serverRulesAskRe = regexp.MustCompile(serverRulesAskRe.String() + `|重な(っ|る|り)|ダブルブッキング|(予定|予約|シフト)が(かぶ|被)`)
}
