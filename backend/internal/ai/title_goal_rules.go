package ai

import "strings"

// titleGoalRules: defects of generated apps that lint now reports (dsl
// list_title.go / setting_agg.go / goal_target.go). A list title bound to an
// optional memo shows a blank bold line for a voice entry without one (23%
// of the eval corpus); a goal / budget entity read with max / min ignores a
// lowered goal; a progressBar target (gauge max) is a number literal, so a
// bar next to a goal entity ignores the goal the user edits (nav_study:
// target 120 beside goal.target_minutes). Appended like groupByRules so
// prompt.go stays untouched; restrict.go swaps both goal bullets for builds
// without single. No snippet has "by": most goal entities have no date
// field, and omitted by falls back to creation order.
var titleGoalRules = strings.TrimSpace(`
## 一覧の見出しと目標値
- list の item.title(timeline の titleField)は**必須の field**(カテゴリ・種類・名前など required の string / enum / ref)にする。任意のメモ(memo / note)は空のまま保存されると見出しが空行になるので item.subtitle に回す(「list の日付」の「メモを title に」は必須の field が無い時だけ)。例: {"type":"list","entity":"expense","item":{"title":"category","subtitle":"memo","trailing":"amount"}}。
` + titleGoalSingleLine + `
- progressBar の target・gauge の max は数値しか書けない。上のように後から変えられる目標・予算は、その数を target / max に書き写さない(目標を変えてもバーが動かない)。達成率は stat の format percent で目標を読む: ` + goalRateSetting + `。progressBar{entity,field,target} は目標が依頼文で決まっていて変えない時だけ。
`)

const (
	titleGoalSingleLine = `- 目標・予算のような後から変える値は "single": true の設定にする(single が使えない版では記録を {"agg":"latest","entity":"budget","field":"amount"} の latest で読む(by は省けば作った順。by に書けるのはその entity の date / datetime field だけで、$createdAt や無い field は書けない))。max / min で読まない(下げた目標・予算が反映されない)。`
	titleGoalLatestLine = `- 目標・予算のような後から変える値は記録を {"agg":"latest","entity":"budget","field":"amount"} の latest で読む(by は省けば作った順。by に書けるのはその entity の date / datetime field だけで、$createdAt や無い field は書けない)。max / min で読まない(下げた目標・予算が反映されない)。`

	goalRateSetting = `{"type":"stat","label":"目標の達成率","format":"percent","value":{"op":"div","args":[{"agg":"sum","entity":"study","field":"minutes"},{"setting":"target_minutes"}]}}`
	goalRateLatest  = `{"type":"stat","label":"目標の達成率","format":"percent","value":{"op":"div","args":[{"agg":"sum","entity":"study","field":"minutes"},{"agg":"latest","entity":"goal","field":"target_minutes"}]}}`
)

func init() { systemPrompt += "\n\n" + titleGoalRules }
