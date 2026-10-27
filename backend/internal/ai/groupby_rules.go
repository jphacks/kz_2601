package ai

import "strings"

// groupByRules (#261): a ranking of shops listed every review one by one
// sorted by score ("降順の一覧で代替"). table.groupBy + aggregate columns
// fold the same shop into one row. Appended like visualRules so prompt.go
// stays untouched.
var groupByRules = strings.TrimSpace(`
## グループごとの集計の一覧(#261)
- 「店ごとの平均点」「人ごとの合計」「〜別のランキング」「〜ごとに何件」のように**同じ値の記録を 1 行にまとめて並べる**時は table の groupBy + 集計列。例(ラーメン屋の評価を投稿して点数のランキング): review{shop: string(店名), score: integer(rating max 5), comment: text, date: date}・scope public、ホームに {"type":"table","entity":"review","groupBy":"shop","columns":[{"field":"shop","title":"店"},{"field":"score","aggregate":"avg","title":"平均点"},{"field":"shop","aggregate":"count","title":"件数"}],"sort":{"field":"score","direction":"desc"},"limit":10}。投稿 1 件ずつの list(onItemTap でフォーム)は新着として別に置いてよい。
- 列は groupBy の field の列(集計なし・グループ名)+ aggregate 付きの列だけ。aggregate は count / sum / avg / min / max。sum / avg / min / max は数値(integer / number / currency)field、count は groupBy の field を書く。groupBy にできるのは string / enum / ref / boolean / integer / date。
- 並び順は sort.field = 集計列の field(平均点順なら score・件数順なら groupBy の field)・direction desc。上位 N 件なら limit。
- 投稿 1 件ずつを点数順に並べた list でランキングを代替しない・「降順の一覧で代替しました」と申告しない。人や店のマスタ entity があり ref で指している時は、その ref field で groupBy してよい(マスタ側の computed agg + table でもよい)。
`)

func init() { systemPrompt += "\n\n" + groupByRules }
