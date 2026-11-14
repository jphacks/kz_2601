package ai

import "strings"

// masterReachRules: 62 of 737 eval apps showed a master (warikan members, gym
// exercises, tournament players, chore members, a trip plan) only in a table
// or timeline, so saved rows could not be opened, edited or deleted. The
// prompt says "一覧(list / table)" and "人の table を sort desc", and the
// catalog says to choose table / timeline over list, but nothing says their
// rows cannot be tapped. dsl lint savedRecordReach catches it after the fact;
// this rule prevents the retry. It asks for what the lint checks: settings are
// exempt, every record-tap component counts, and a ranking is one sorted list
// (quiz.json) rather than a table plus a list. Appended like groupByRules so
// prompt.go stays untouched.
var masterReachRules = strings.TrimSpace(`
## 保存した記録を開ける一覧(マスタも)
- フォーム画面(screen.entity)を作った entity は、マスタ(メンバー・プレイヤー・家族・運動の種目・旅程など)も含めて、**onItemTap でそのフォームを開く list**(または calendar / mapView / gridTable など、記録をタップしてフォームを開ける部品)を行ける画面に必ず置く。例(割り勘のメンバー): {"type":"list","entity":"members","item":{"title":"name","trailing":"balance"},"swipeToDelete":true,"onItemTap":{"type":"navigate","screen":"member_form"}}
- onItemTap に prefillRef を付けると新しい記録のフォームになり、タップした記録は開けない。開くのは prefillRef なしの navigate。
- 設定(予算・目標・周期のように 1 件だけ持って stat や式で読む entity)は除く。一覧に並べず、そのフォームは toolbar かボタンの navigate で開く。
- table と timeline の行はタップできない(保存した記録を開けず、直せず、消せない)。**人やマスタの順位・合計は list 1 つで兼ねる**(sort にその computed を desc・item.trailing にその値。例 クイズの参加者 {"type":"list","entity":"players","item":{"title":"name","trailing":"total"},"sort":{"field":"total","direction":"desc"},"onItemTap":{"type":"navigate","screen":"player_form"}})。同じ entity の table と list を両方置かない。記録を groupBy でまとめた table・旅程の timeline など見渡しの部品は残してよく、その entity の list(onItemTap でフォーム)を別のタブか同じ画面に足す。
`)

func init() { systemPrompt += "\n\n" + masterReachRules }
