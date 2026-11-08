package ai

import "strings"

// masterReachRules: 62 of 737 eval apps showed a master (warikan members, gym
// exercises, tournament players, chore members, a trip plan) only in a table
// or timeline, so saved rows could not be opened, edited or deleted. The
// prompt says "一覧(list / table)" and "人の table を sort desc", and the
// catalog says to choose table / timeline over list, but nothing says their
// rows cannot be tapped. dsl lint savedRecordReach catches it after the fact;
// this rule prevents the retry. Appended like groupByRules so prompt.go stays
// untouched.
var masterReachRules = strings.TrimSpace(`
## 保存した記録を開ける一覧(マスタも)
- フォーム画面(screen.entity)を作った entity は、マスタ(メンバー・プレイヤー・家族・運動の種目・旅程など)も含めて、**onItemTap でそのフォームを開く list**(または calendar / mapView / gridTable)を行ける画面に必ず置く。例(割り勘のメンバー): {"type":"list","entity":"members","item":{"title":"name","trailing":"balance"},"swipeToDelete":true,"onItemTap":{"type":"navigate","screen":"member_form"}}
- table と timeline の行はタップできない(保存した記録を開けず、直せず、消せない)。ランキング・精算表・旅程の時系列などの見渡しには使ってよいが、それだけにしない。同じ entity の list(onItemTap でフォーム)を同じ画面か別のタブに足す。
`)

func init() { systemPrompt += "\n\n" + masterReachRules }
