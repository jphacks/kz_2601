package ai

import (
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// FeatureBracket is the supportedFeatures value of the bracket component
// (pkg/dsl/bracket.go). A build that sends allowedComponents without it does
// not draw bracket, so the AI does not use it there.
const FeatureBracket = "component.bracket"

// bracketRules: "make the tournament bracket from the entrants" (past JPHACKS
// tk_2421). Appended like randomRules so prompt.go stays untouched; dropped
// with the feature key.
var bracketRules = strings.TrimSpace(`
## トーナメント表(bracket)
- 「トーナメント表」「勝ち抜き戦」「出場者・エントリーから対戦表を自動で作る」「勝ち上がりを表示」は **bracket**(出場者から 1 回戦を自動で組み、勝った方をタップすると winsField に保存して次の回戦へ進める)。**list・table で代替しない**・対戦の entity を別に作らない。
- 形: 出場者の entity(例 entrant{name: string(required), wins: integer(defaultValue 0), seed: integer(任意)}・titleField name)+ ホームに bracket{entity: entrant, titleField: name, winsField: wins, seedField?: seed}(A1:F7)+ 出場者の list(A8:F10・onItemTap はフォームへ navigate(sheet))+ 追加ボタン(D11:F12)+ 出場者のフォーム画面(screen.entity + saveRecord)。wins はフォームに出さない(表のタップで変わる)。
- シード・強さ順で組むなら seedField(小さいほど上位・上位から不戦勝)。無ければ登録順。2 のべき乗に足りない分は不戦勝になる。出場者は 64 まで。
- 勝者を決められるのは記録を変更できる人だけ。運営者が出場者を登録する形(scope は省略 = 自分だけ)を基本にする。
- リーグ戦(総当たり)・敗者復活・3 位決定戦・試合のスコアの記録は**できない**。求められたら summary で正直に伝える。
- 最初の結果が付くと組み合わせは固定され、後から足した人は表に入らない。entrant に started: datetime(フォームに出さない)を作り startedField: started にする。
`)

var bracketFeature = clientFeature{
	key:       FeatureBracket,
	rules:     []string{bracketRules},
	note:      "- bracket(トーナメント表)は使わない(この版では描けない)。出場者の list で代替し「トーナメント表はこの版では作れないため、出場者の一覧にしました。」と申告する。\n",
	schema:    dropBracketSchema,
	places:    func(root map[string]any) []string { return componentPlaces(root, isBracket) },
	strip:     func(root map[string]any) int { return dropComponents(root, isBracket) },
	violation: "component type \"bracket\" is not available in this app build: remove it and show the entrants with a list",
}

func isBracket(c map[string]any) bool { return isType(c, dsl.CompBracket) }

// dropBracketSchema removes Bracket from the Leaf union and $defs.
func dropBracketSchema(defs map[string]any) {
	if leaf, _ := defs["Leaf"].(map[string]any); leaf != nil {
		arr, _ := leaf["anyOf"].([]any)
		kept := []any{}
		for _, v := range arr {
			if sub, _ := v.(map[string]any); sub != nil && sub["$ref"] == "#/$defs/Bracket" {
				continue
			}
			kept = append(kept, v)
		}
		leaf["anyOf"] = kept
	}
	delete(defs, "Bracket")
}

func init() {
	systemPrompt += "\n\n" + bracketRules
	clientFeatures = append(clientFeatures, bracketFeature)
}
