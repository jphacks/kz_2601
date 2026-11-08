package ai

import "strings"

// titleGoalRules: two defects of generated apps that lint now reports
// (dsl list_title.go / setting_agg.go). A list title bound to an optional
// memo shows a blank bold line for a voice entry without one (23% of the
// eval corpus); a goal / budget entity read with max / min ignores a
// lowered goal. Appended like groupByRules so prompt.go stays untouched.
var titleGoalRules = strings.TrimSpace(`
## 一覧の見出しと目標値
- list の item.title(timeline の titleField)は**必須の field**(カテゴリ・種類・名前など required の string / enum)にする。任意のメモ(memo / note)は空のまま保存されると見出しが空行になるので item.subtitle に回す。例: {"title":"category","subtitle":"memo","trailing":"amount"}。
- 目標・予算のような後から変える値は "single": true の設定にする(single が使えない版では記録を {"agg":"latest","entity":"budget","field":"amount","by":"date"} の latest で読む)。max / min で読まない(下げた目標・予算が反映されない)。
`)

func init() { systemPrompt += "\n\n" + titleGoalRules }
