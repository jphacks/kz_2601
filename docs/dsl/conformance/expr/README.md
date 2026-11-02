# Expr の共有テストデータ

計算式(`Expr`・[`../../expressions.md`](../../expressions.md))の評価結果を、iOS と Go で同じにするためのケース集(#545)。

- **iOS が正**。`expected` は iOS の評価器(`ExprEvaluator.swift`)の結果をそのまま書いたもの。手で書き換えない。
- iOS: `ios/Packages/MetamonDSL/Tests/MetamonDSLTests/ExprConformanceTests.swift` が全ケースを回して一致を確かめる。
- Go: `backend/pkg/dsl/expreval/conformance_test.go` が同じファイルを読み、`expreval` の結果が完全に一致することを確かめる(op・agg・filter の全種類を使っているかも確かめる)。

## ケースを足す・iOS の評価を変えた時

1. `*.json` にケースを足す(`expected` は書かない)。
2. iOS の結果で `expected` を書く:
   ```
   cd ios/Packages/MetamonDSL && EXPR_CONFORMANCE_UPDATE=1 swift test --filter ExprConformanceTests
   ```
   ファイルは整形し直される(キー順・1 行にまとめる幅が決まっている)。差分を見て意図どおりか確かめる。
3. `go test ./pkg/dsl/expreval/` を通す(合わなければ Go 側を iOS に合わせる)。

## 形

```json
{
  "description": "このファイルの範囲",
  "defaults": {"now": "2026-10-28T15:30:45+09:00", "timeZone": "Asia/Tokyo", "entities": [...], "records": {"<entityId>": [Record, ...]}},
  "cases": [
    {
      "name": "sum/basic",
      "expr": {"agg": "sum", "entity": "expenses", "field": "amount"},
      "row": {"entity": "members", "record": {"id": "m1", "data": {...}}},
      "expected": {"type": "number", "value": 2850, "text": "2850"}
    }
  ]
}
```

| キー | 意味 |
|---|---|
| `now` / `timeZone` | 評価の「今」(RFC 3339)と端末のタイムゾーン(IANA 名)。ケースに書けば defaults を上書き |
| `entities` / `records` | DSL の entity 定義と、entity ごとのレコード(appconfig の Record 形: `id`・`data`・`createdAt`・`author`・`authorName`・`marks`)。ケースに書けば defaults を丸ごと置き換え |
| `row` | 行コンテキスト(`ref` の読み先)。無ければ行なし(stat.value と同じ) |
| `expected.type` | `number` / `string` / `bool` / `date` / `nil`(評価できない =「—」) |
| `expected.value` | 値。number の無限大・NaN は文字列 `"inf"` `"-inf"` `"nan"`。date は `timeZone` での RFC 3339 |
| `expected.text` | `concat` / `toString` が使う文字列化(日付は `YYYY-MM-DD`) |

設定はタイムゾーンだけ(評価器はロケールを使わない)。
