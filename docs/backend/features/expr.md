# 計算ロジック(Expr・Issue #100)

> [バックエンド設計](../architecture.md) から分けた機能の節(旧 §6.7)。

割り勘で「割る」計算が DSL に書けなかった問題への対応。仕様は [../dsl/expressions.md](../../dsl/expressions.md)、eval は [ai-eval/2026-10-24-logic.md](../ai-eval/2026-10-24-logic.md)。

| 項目 | 決定 |
|---|---|
| 表現 | 小さな式ツリー `Expr`(リテラル / ref / agg / op / if・op は閉じた 38 種・深さ 8)を `Field.computed`(行ごとの算出列)と `stat.value`(画面スカラー)に置く。他 entity 参照は `Field.type:"ref"`、精算は Leaf `settlement`。評価は iOS、backend は `pkg/dsl/expr.go` で検証(op/引数/ref 文脈/agg/循環/深さ/粗い型) |
| AI への教え方 | カタログ末尾に `$defs/Expr` から自動生成した「計算式 Expr」節(形・op 一覧・レシピ 5 本)+ few-shot 例 3(割り勘)+ プロンプト規則「計算が要る発話では UI だけで終わらせない」 |
| 既定 = **1 パス** | UI と計算を 1 回の生成で出す。新 5 ケースで計算 5/5・一発通過 5/5・p50 6.5s / p95 8.1s。入力は約 +5.8K tok(キャッシュ対象) |
| 2 パス(opt-in) | `AI_LOGIC_PASS=1` か `Request.logicPass:true`。create の後に計算カタログだけの小さな system prompt(約 2.9K tok)で「計算を足す JSON Patch」を 1 回(+1 回再試行)。検証に通らなければ 1 パス目を返す。計算 5/5 は同じで p50 10.7s(1 パス 6.5s)と遅いので既定にしない。edit ターンでは動かない(後続対応) |
| ストリーミング | 1 パス目の `event: app` はそのまま先に届く。2 パス目の結果は最終 DSL(`event: result`)に反映 |

## 環境変数

| 変数 | appconfig | ai | 既定 / 備考 |
|---|---|---|---|
| `AI_LOGIC_PASS` | - | o | 未設定(=off)。`1` で create 後に計算ロジックの 2 パス目を実行(§6.7) |
