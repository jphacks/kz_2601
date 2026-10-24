# 計算式(Expr)仕様 — DSL v1 拡張(#100)

> 正典は `dsl.schema.json` の `$defs/Expr` `ExprRef` `ExprAgg` `ExprOp` `ExprIf` `Settlement`、`Field.computed` / `Field.refEntity` / `Stat.value`。
> 本書はその読み方と、backend 検証(`backend/pkg/dsl/expr.go`)・iOS 評価の共通ルール。矛盾したらスキーマが勝つ。
> 実例: `examples/warikan.json`(割り勘)・`examples/bmi.json`(BMI)。

## 目次
- [1. なぜ](#1-なぜ)
- [2. 文法](#2-文法)
- [3. op 一覧](#3-op-一覧)
- [4. 置き場所](#4-置き場所)
- [5. 評価ルール(iOS)](#5-評価ルールios)
- [6. 検証ルール(backend)](#6-検証ルールbackend)
- [7. レシピ](#7-レシピ)
- [8. 割り切り(やらないこと)](#8-割り切りやらないこと)

## 1. なぜ

「旅行の割り勘アプリ」で支出登録画面はできたが、裏の「割る」計算が DSL に書けず精算ができなかった。v1 の計算手段は `stat/chart/progressBar/gauge` の `aggregate`(count/sum/avg/min/max)+ 単一 `filter` だけ。
そこで **小さな式ツリー** を足し、四則演算・ルート・比較・if・日付・集計を JSON で組み合わせられるようにする。汎用プログラミング言語にはしない(§8)。

## 2. 文法

```
Expr := number | string | boolean
      | {"ref": "<fieldId>"}
      | {"agg": "count|sum|avg|min|max", "entity": "<entityId>", "field"?: "<fieldId>", "filter"?: Filter | [Filter]}
      | {"op": "<name>", "args": [Expr, ...]}
      | {"if": Expr, "then": Expr, "else": Expr}
```

| 形 | 意味 |
|---|---|
| リテラル | 数値・文字列・真偽値。`null` は式ではない(スキーマで拒否) |
| `ref` | 行コンテキスト(§4)のレコードのフィールド値。`"id"` はレコード id |
| `agg` | エンティティ全レコード(filter で絞る)の集計。count 以外は `field` 必須。`field` に computed フィールドも可 |
| `op` | 関数呼び出し。§3 の閉じた一覧のみ |
| `if` | `if` が true なら `then`、それ以外は `else` |

`agg.filter` は `Filter` 1 つ、または配列(AND)。`Filter.value` には Expr を書ける。computed 内の agg では `value` の `ref` は**外側のレコード**を指す:

```json
{"agg":"sum","entity":"expenses","field":"amount","filter":{"field":"payer","op":"eq","value":{"ref":"id"}}}
```
= 「このメンバー(外側のレコード)が払った支出の合計」。

## 3. op 一覧

| 分類 | op | 引数 | 結果 |
|---|---|---|---|
| 算術 | `add` `mul` `min` `max` | 数値 2 個以上 | 数値 |
| | `sub` `div` `mod` `pow` | 数値 2 個 | 数値(`div`/`mod` の 0 割りは評価エラー) |
| | `neg` `abs` `sqrt` | 数値 1 個 | 数値(`sqrt` の負数はエラー) |
| | `round` `floor` `ceil` | (x) または (x, n) | 数値(n = 小数桁・省略で整数。`round(x,1)` = 小数 1 桁) |
| | `clamp` | (x, lo, hi) | 数値 |
| 比較 | `eq` `neq` | 任意 2 個 | 真偽(型が違っても nil にせず、数値か文字列に寄せて比べる: レコード id と ref 値の比較用) |
| | `gt` `gte` `lt` `lte` | 同じ型(数値同士・日付同士)2 個 | 真偽 |
| 論理 | `and` `or` | 真偽 2 個 | 真偽 |
| | `not` | 真偽 1 個 | 真偽 |
| 文字列 | `concat` | 任意 2 個(文字列化して連結) | 文字列 |
| | `len` | 文字列 1 個 | 数値(文字数) |
| 日付 | `now` `today` | なし(`args: []`) | 日時 / 今日の日付(端末ローカル) |
| | `daysBetween` | (a, b) | 数値 = b − a の日数 |
| | `daysUntil` | (d) | 数値 = d − 今日 の日数(過ぎたら負) |
| | `addDays` | (d, n) | 日付 = d の n 日後 |
| | `year` `month` `day` `weekday` | (d) | 数値(month 1-12、weekday 1=日 … 7=土) |
| 変換 | `toNumber` `toString` | 任意 1 個 | 数値(変換不可はエラー)/ 文字列 |
| その他 | `coalesce` | (a, b) | a が空(nil・空文字)/評価エラーなら b |

日付引数は date / datetime フィールド、`today`/`now`/`addDays` の結果、`"YYYY-MM-DD"` 文字列を受け付ける。

## 4. 置き場所

| 場所 | 行コンテキスト | `ref` | 用途 |
|---|---|---|---|
| `Field.computed` | そのレコード | 可(同 entity のフィールド・`"id"`) | 算出列: 支払済・残高・BMI・残り日数 |
| `Stat.value` | なし(画面スカラー) | **不可**(`agg` を使う) | 1 人あたり・残り予算・達成率 |
| `Filter.value`(agg 内) | 外側の行(computed なら そのレコード) | 外側に行があれば可 | 「このメンバーが払った」 |
| `Filter.value`(list/stat/chart 等の画面部品) | なし | 不可 | `{"op":"addDays","args":[{"op":"today","args":[]},-7]}` = 直近 7 日 |

- **computed フィールド**: `type` は `number|integer|currency|string|boolean|date`。保存されない読み取り専用列。list の item / table の columns / stat・chart の field / sort / filter から通常フィールドと同じに参照できる。入力部品(textField 等)は束縛できない。`required` / `defaultValue` は不可。
- 部品レベルの `filter`(list / stat / chart …)はスキーマ上 1 つだけ(iOS は配列も読めるが、backend は出さない・AND が要るなら stat.value の agg で)。
- **`Stat.value`**: `aggregate` の代わり。`aggregate` と `value` は**排他でどちらか必須**。`value` のときは `entity/field/filter` を書かない(agg の中に書く)。`format` はそのまま効く(percent は 0..1 → %)。
- **`Field.type = "ref"` + `refEntity`**: 他エンティティのレコード参照。保存値はレコード id の文字列。入力は **`picker`**(選択肢 = refEntity のレコード、表示は titleField)。list/table では参照先の titleField を表示する。
- **`settlement`(Leaf)**: `{"type":"settlement","entity":"members","nameField":"name","balanceField":"balance"}` + `id?` `area?` `title?`。balance(正=受け取る・負=払う)から貪欲法(最大の債務者 → 最大の債権者に min(|負|, 正) を送る、を繰り返す)で「A → B ¥1,200」を列挙。割り勘専用の割り切り部品。合計 0 にならない端数(1 円未満の丸め差)は無視してよい。全員 0 なら「精算は不要です」。
- progressBar / gauge の `value` は従来どおり**数値リテラルのみ**(Expr は不可。後述 §8)。

## 5. 評価ルール(iOS)

- 型は 数値(Double)/ 文字列 / 真偽 / 日付 / 空。currency・integer フィールドは数値。ref フィールドの値は参照先レコード id(文字列)。
- 空のフィールド・評価エラー(0 割り・型不一致・参照先なし・`sqrt` の負数)は **nil → 表示は「—」**。nil を受けた op は nil を返す(`coalesce` だけは b を返す)。`agg` は 0 件なら count/sum は 0、avg/min/max は nil。
- computed 同士の依存は backend が DAG を保証する(循環は来ない)。評価順は依存順でも都度再帰でもよい。
- `today`/`now`/`thisMonth` は端末ローカル時刻(従来の stat filter と同じ)。

## 6. 検証ルール(backend)

`dsl.Parse` / `App.Validate` が以下を全部列挙して返す(AI の再生成プロンプトにそのまま渡る):

| ルール | エラー例 |
|---|---|
| op 名が §3 の一覧内 | `... unknown op "median" (allowed: abs add ...)` |
| 引数個数 | `... op sub takes exactly 2 args (got 1)` / `op add needs at least 2 args` |
| `ref` の存在 | `ref "nope" is not a field of entity "members" (fields: name, paid, ..., or "id")` |
| `ref` の行コンテキスト | `{"ref":"amount"} is not allowed here (no current record; stat.value is a screen value). Use {"agg":...} over an entity instead` |
| `agg` の entity/field/filter | `entity "ghost" does not exist` / `field "zzz" not in entity "expenses"` / `sum needs a numeric field but "title" is string` |
| 循環禁止(DAG は可・agg 経由も検出) | `computed field cycle: members.balance -> members.paid -> members.balance; ...` |
| 深さ ≤ 8(リテラル単体 = 1) | `expression is nested deeper than 8 levels; simplify it ...` |
| 粗い型検査 | `sub expects a number but got a string (wrap it in {"op":"toNumber",...})` / `computed expression yields a string but the field type is currency` |
| computed の型・読み取り専用 | `computed field type must be one of ...` / `remove required` / `inputs cannot bind to it` |
| `ref` 型 | `type ref needs refEntity` / `refEntity "people" does not exist` / `use a picker to choose the members record` |
| settlement | `settlement balanceField "name" must be number/integer/currency` |
| Stat 排他 | `stat takes either aggregate or value, not both` / `stat needs aggregate (with entity) or value` |

## 7. レシピ

AI のカタログにも同じものが載る(`$defs/Expr.x-recipes`)。

1. **割り勘**(`examples/warikan.json`): members{name} + expenses{title, amount:currency, payer:ref→members, date}。
   - `paid` = `{"agg":"sum","entity":"expenses","field":"amount","filter":{"field":"payer","op":"eq","value":{"ref":"id"}}}`
   - `share` = `{"op":"round","args":[{"op":"div","args":[{"agg":"sum","entity":"expenses","field":"amount"},{"agg":"count","entity":"members"}]}]}`
   - `balance` = `{"op":"sub","args":[{"ref":"paid"},{"ref":"share"}]}`
   - ホーム: stat(合計・1 人あたり)+ members の table(名前/支払済/残高)+ settlement + 支出 list + 追加ボタン。
2. **BMI**(`examples/bmi.json`): `bmi` = weight / (height/100)^2 を小数 1 桁に(`round(x*10)/10`)。
3. **残り予算**: stat.value = `sub(予算, agg sum thisMonth)`。予算を後で変えたいなら budget entity を作り `agg max budget.amount`。
4. **達成率**: stat{format:percent, value: `div(agg sum, 目標)`} + progressBar{entity, field, target}。
5. **カウントダウン**: computed `days_left` = `daysUntil(ref date)`、最も近い予定 = `agg min days_left filter days_left gte 0`。

## 8. 割り切り(やらないこと)

「0 と 1 を管理して PC 上に PC を作る」状態にしないため、以下は**やらない**:

- 変数・代入・ループ・ユーザー定義関数・ラムダ・再帰。式はツリー 1 本で、深さ 8 まで。
- 配列/オブジェクト値、map/filter/reduce、文字列の部分抽出・正規表現・フォーマット指定(表示は `format`/`unit` に任せる)。
- 状態を持つ計算(累積・前回値との差・移動平均)、レコード間の順序依存(「最新の 1 件」)。必要になったら専用の `agg` 名を足す方針。
- 副作用(式からの保存・通知・遷移)。
- サーバー側評価・集計 API(評価は iOS。件数が増えたら README 決定表 #10 の集計 API と合わせて検討)。
- progressBar/gauge の `value`/`target` を Expr にすること(stat.value で代替可能・iOS 契約の拡大を避けた。必要なら後段で)。
- タイムゾーン指定・通貨換算。

op を増やすときは `dsl.schema.json`(`ExprOp.op` の enum と x-enumDescriptions)、`backend/pkg/dsl/expr.go`(`Ops`)、本書 §3、iOS の評価器を同時に更新する。
