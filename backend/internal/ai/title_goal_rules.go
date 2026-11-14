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
	// The single build already says where goals live and how to read them
	// (prompt.go settingsSingleLine + singleRules): only the max / min part.
	titleGoalSingleLine = `- 後から変える目標・予算を max / min で読まない(下げた値が反映されない。置き方と読み方は上の「設定値の置き場は single の entity」)。`
	titleGoalLatestLine = `- 目標・予算のような後から変える値は記録を {"agg":"latest","entity":"budget","field":"amount"} の latest で読む(by は省けば作った順。by に書けるのはその entity の date / datetime field だけで、$createdAt や無い field は書けない)。max / min で読まない(下げた目標・予算が反映されない)。`

	goalRateSetting = `{"type":"stat","label":"目標の達成率","format":"percent","value":{"op":"div","args":[{"agg":"sum","entity":"study","field":"minutes"},{"setting":"target_minutes"}]}}`
	goalRateLatest  = `{"type":"stat","label":"目標の達成率","format":"percent","value":{"op":"div","args":[{"agg":"sum","entity":"study","field":"minutes"},{"agg":"latest","entity":"goal","field":"target_minutes"}]}}`

	// The Expr catalog's 達成率 recipe (schema.json x-recipes) paired the rate
	// stat with progressBar{target:<目標>}: a changeable goal copied into a
	// literal target, the opposite of the goal-rate bullet above.
	goalRecipeBar  = `(percent は 0..1 を % 表示)+ progressBar{entity,field,target:<目標>}。`
	goalRecipeRate = `(percent は 0..1 を % 表示)。後から変える目標・予算は <目標> を式で読み、この stat だけにする。progressBar{entity,field,target:<目標の数>} は目標が依頼文で決まっていて変えない時だけ添える(target は数値だけで、変えた目標に追従しない)。`
)

// fixGoalRecipe rewrites the 達成率 recipe in a rendered catalog (restrict.go
// applies it to the restricted ones). The new text names no {"setting"}, so
// it also fits builds without single.
func fixGoalRecipe(s string) string { return strings.Replace(s, goalRecipeBar, goalRecipeRate, 1) }

func init() {
	systemPrompt = fixGoalRecipe(systemPrompt) + "\n\n" + titleGoalRules
	chatSystemPrompt = fixGoalRecipe(chatSystemPrompt)
	chatEditSystemPrompt = fixGoalRecipe(chatEditSystemPrompt)
	chatCreateSystemPrompt = fixGoalRecipe(chatCreateSystemPrompt)
	logicSystemPrompt = fixGoalRecipe(logicSystemPrompt)
	// restrictPrompt swaps catalog for the restricted one: keep them equal
	catalog = fixGoalRecipe(catalog)
	exprCatalog = fixGoalRecipe(exprCatalog)
}
