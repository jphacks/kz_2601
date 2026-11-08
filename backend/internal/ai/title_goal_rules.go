package ai

import "strings"

// titleGoalRules: two defects of generated apps that lint now reports
// (dsl list_title.go / setting_agg.go). A list title bound to an optional
// memo shows a blank bold line for a voice entry without one (23% of the
// eval corpus); a goal / budget entity read with max / min ignores a
// lowered goal. Appended like groupByRules so prompt.go stays untouched;
// restrict.go swaps the goal line for builds without single.
var titleGoalRules = strings.TrimSpace(`
## 一覧の見出しと目標値
- list の item.title(timeline の titleField)は**必須の field**(カテゴリ・種類・名前など required の string / enum / ref)にする。任意のメモ(memo / note)は空のまま保存されると見出しが空行になるので item.subtitle に回す(「list の日付」の「メモを title に」は必須の field が無い時だけ)。例: {"type":"list","entity":"expense","item":{"title":"category","subtitle":"memo","trailing":"amount"}}。
` + titleGoalSingleLine)

const (
	titleGoalSingleLine = `- 目標・予算のような後から変える値は "single": true の設定にする(single が使えない版では記録を {"agg":"latest","entity":"budget","field":"amount"} の latest で読む(by は省けば作った順。by に書けるのはその entity の date / datetime field だけで、$createdAt や無い field は書けない))。max / min で読まない(下げた目標・予算が反映されない)。`
	titleGoalLatestLine = `- 目標・予算のような後から変える値は記録を {"agg":"latest","entity":"budget","field":"amount"} の latest で読む(by は省けば作った順。by に書けるのはその entity の date / datetime field だけで、$createdAt や無い field は書けない)。max / min で読まない(下げた目標・予算が反映されない)。`
)

func init() { systemPrompt += "\n\n" + titleGoalRules }
