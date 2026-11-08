package ai

import (
	"fmt"
	"strings"
)

// FeatureExprAddMinutes: the Expr op addMinutes ("HH:mm" / date + minutes).
// A build without it evaluates the op as "—", so it is not generated there.
const FeatureExprAddMinutes = "expr.addMinutes"

// addMinutesRules: departure = arrival − travel minutes (JPHACKS sp_2403 /
// OL_2331). Appended like routeRules so prompt.go stays untouched.
var addMinutesRules = strings.TrimSpace(`
## 時刻に分を足す・引く(addMinutes)
- 「着きたい時刻から出発時刻を逆算」「○分前に出る」「終わる時刻 = 始まり + 所要分」は computed の {"op":"addMinutes","args":[時刻, 分]}。時刻が "HH:mm"(timePicker の string field)なら "HH:mm" を返し 24 時間で一周する(00:20 の 90 分前 = 22:50)。date / datetime なら日時(date はその日の 0:00)を返す。日時を渡すなら field は datetime 型(string / date の field だと日付しか出ず時刻が消える)。時刻だけなら HH:mm 文字列(string field)を渡す。分は負でもよく、数値 field や式でよい(引くなら {"op":"neg",...})。時刻を toNumber / sub で計算しない。
- 例「着きたい時刻と移動時間から出発時刻を出す(電車で)」: entity trip{dest: string(required), arrive: string(timePicker), travel_min: integer(unit 分), depart: string computed {"op":"addMinutes","args":[{"ref":"arrive"},{"op":"neg","args":[{"op":"coalesce","args":[{"ref":"travel_min"},0]}]}]}}。一覧は dest と depart。経路・乗換はアプリの中で調べられないので openUrlButton{label: 乗換案内を開く, url: "https://transit.yahoo.co.jp/"} を置く。
- 日付も要るとき(「○月○日の何時に着く便から出発日時を出す」): arrive_at: datetime(datePicker includeTime:true), depart_at: datetime computed {"op":"addMinutes","args":[{"ref":"arrive_at"},{"op":"neg","args":[{"ref":"travel_min"}]}]}。depart_at を string や date にしない。HH:mm の結果を daysBetween / add などの日付・数値の計算に渡さない(時間の差は hoursBetween)。
`)

func init() {
	systemPrompt += "\n\n" + addMinutesRules
	clientFeatures = append(clientFeatures, clientFeature{
		key:    FeatureExprAddMinutes,
		rules:  []string{addMinutesRules},
		note:   "- 計算式の addMinutes(時刻に分を足す・引く)は使わない(この版では動かない)。出発時刻などは timePicker で手で入れる欄にし、頼まれたら「時刻の自動計算はこの版ではできないため、入力する形にしました。」と申告する。\n",
		schema: func(defs map[string]any) { dropEnumValues(defs, "ExprOp", "op", "addMinutes") },
		places: func(root map[string]any) []string {
			var out []string
			walkAddMinutes(root, "", func(path string, _ map[string]any) any { out = append(out, path); return nil })
			return out
		},
		strip: func(root map[string]any) int {
			n := 0
			// "" is what such a build showed anyway ("—"), and keeps the document valid
			walkAddMinutes(root, "", func(string, map[string]any) any { n++; return "" })
			return n
		},
		violation: `op addMinutes is not available in this app build: let the user enter the time (timePicker) instead`,
	})
}

// walkAddMinutes calls fn on every {"op":"addMinutes"} expression under v
// (outermost first, not inside a match). A non-nil result replaces it.
func walkAddMinutes(v any, path string, fn func(path string, e map[string]any) any) any {
	switch x := v.(type) {
	case map[string]any:
		if x["op"] == "addMinutes" {
			return fn(path, x)
		}
		for k, c := range x {
			if r := walkAddMinutes(c, path+"."+k, fn); r != nil {
				x[k] = r
			}
		}
	case []any:
		for i, c := range x {
			if r := walkAddMinutes(c, fmt.Sprintf("%s[%d]", path, i), fn); r != nil {
				x[i] = r
			}
		}
	}
	return nil
}
