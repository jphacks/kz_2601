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
      | {"ref": "<fieldId>"} | {"ref": "<refFieldId>.<fieldId>"}
      | {"agg": "count|sum|avg|min|max|latest|earliest", "entity": "<entityId>", "field"?: "<fieldId>", "by"?: "<dateFieldId>", "filter"?: Filter | [Filter]}
      | {"op": "<name>", "args": [Expr, ...]}
      | {"if": Expr, "then": Expr, "else": Expr}
```

| 形 | 意味 |
|---|---|
| リテラル | 数値・文字列・真偽値。`null` は式ではない(スキーマで拒否) |
| `ref` | 行コンテキスト(§4)のレコードのフィールド値。`"id"` はレコード id。`"payer.name"` のように **ドット 1 つ**で、type ref の field が指す先のレコードのフィールド値(lookup・#134)。`"<ref>.id"` は ref の値そのもの。2 段以上(`a.b.c`)は不可 |
| `agg` | エンティティ全レコード(filter で絞る)の集計。count 以外は `field` 必須。`field` に computed フィールドも可。`latest` / `earliest`(#133)は `by`(date/datetime field)で並べた最新 / 最古の 1 件の `field` の値(型は field のまま・数値以外も可) |
| `op` | 関数呼び出し。§3 の閉じた一覧のみ |
| `if` | `if` が true なら `then`、それ以外は `else` |

`agg.filter` は `Filter` 1 つ、または配列(AND)。`Filter.value` には Expr を書ける。computed 内の agg では `value` の `ref` は**外側のレコード**を指す:

```json
{"agg":"sum","entity":"expenses","field":"amount","filter":{"field":"payer","op":"eq","value":{"ref":"id"}}}
```
= 「このメンバー(外側のレコード)が払った支出の合計」。

**latest / earliest**(#133): `{"agg":"latest","entity":"body","field":"weight","by":"date"}` = 「今の体重」。

- `by` 省略時は entity の最初の date/datetime field、それも無ければ作成順(createdAt)。
- 同じ日(`by` の値が同じ)は作成順で後の方を新しいとみなす。0 件・値が空は nil(「—」)。
- stat の `aggregate: "latest"` + `by` も同じ(stat の `field` が返す値)。chart / progressBar / gauge では使えない。

**lookup**(#134): ドット記法を選んだ理由 — `{"op":"lookup","args":[{"ref":"chore"},"points"]}` より短く、生成 AI が普段書く「オブジェクト.プロパティ」と同じ形で書き間違えにくい。行コンテキストの規則(stat.value 不可)と依存・循環の検出も `ref` のものをそのまま使える。文字列リテラル引数を field 名として特別扱いする必要もない。

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
| 時刻 | `hoursBetween` | (start, end) | 数値 = end − start の時間数(小数。30 分 = 0.5)。**end < start は日付またぎとして +24h**(22:00→05:00 = 7)。同じ時刻は 0 |
| | `year` `month` `day` `weekday` | (d) | 数値(month 1-12、weekday 1=日 … 7=土) |
| 変換 | `toNumber` `toString` | 任意 1 個 | 数値(変換不可はエラー)/ 文字列 |
| その他 | `coalesce` | (a, b) | a が空(nil・空文字)/評価エラーなら b |
| 毎日チェック(#172) | `streak` | (`{"ref": <toggle field>}`) | 数値 = 連続日数。今日チェック済みなら今日から、まだなら**昨日から**遡って続いた日数(今日未チェックでも昨日までの連続は切らない)。昨日も無ければ 0 |
| | `doneInDays` | (`{"ref": <toggle field>}`, n) | 数値 = 直近 n 日(今日を含む)にチェックした日数(直近 7 日 = n 7)。n は整数リテラル 1..366 |

日付引数は date / datetime フィールド、`today`/`now`/`addDays` の結果、`"YYYY-MM-DD"` 文字列を受け付ける。

`streak` / `doneInDays` の第 1 引数は**値ではなく field の指定**: `list.item.resets: "daily"` の toggle に使われている同 entity の date field を `{"ref": ...}` で渡す(iOS はその行の「押した日の履歴」を読む・§7.5)。computed(integer)で使い、全体の値は stat.value の `agg sum / max` でその computed を集計する。stat.value 直下では行が無いので使えない。

時刻引数(`hoursBetween`)は `"HH:mm"`(`"H:mm"`・`"HH:mm:ss"` も可)の文字列 = `timePicker` が string field に保存する形。datetime を渡したときは**その時刻部分だけ**を使う(日付は無視)。数値・真偽は backend が拒否する(`toNumber("09:30")` は 9.5 にならないので、時刻を数値化して引き算しない)。日付をまたいで 24 時間以上働く・2 日以上空く計算は対象外(§8)。iOS の `timePicker` は未入力の時も今の時刻を表示するので、表示中の時刻をそのまま保存値にする(触らずに保存して時刻が空 = 勤務時間「—」にならない・#229)。

## 4. 置き場所

| 場所 | 行コンテキスト | `ref` | 用途 |
|---|---|---|---|
| `Field.computed` | そのレコード | 可(同 entity のフィールド・`"id"`) | 算出列: 支払済・残高・BMI・残り日数 |
| `Stat.value` | なし(画面スカラー) | **不可**(`agg` を使う) | 1 人あたり・残り予算・達成率 |
| `Filter.value`(agg 内) | 外側の行(computed なら そのレコード) | 外側に行があれば可 | 「このメンバーが払った」 |
| `Filter.value`(list/stat/chart 等の画面部品) | なし | 不可 | `{"op":"addDays","args":[{"op":"today","args":[]},-7]}` = 直近 7 日 |

- **computed フィールド**: `type` は `number|integer|currency|string|boolean|date`。保存されない読み取り専用列。list の item / table の columns / stat・chart の field / sort / filter から通常フィールドと同じに参照できる。入力部品(textField 等)は束縛できない。`required` / `defaultValue` は不可。
- 部品レベルの `filter`(list / stat / chart …)はスキーマ上 1 つだけ(iOS は配列も読めるが、backend は出さない・AND が要るなら stat.value の agg で)。
- **グループごとの集計の一覧**(店ごとの平均点・人ごとの合計)は Expr ではなく `table.groupBy` + 列の `aggregate`(README §3.2 の設計メモ・#261)。グループの値ごとに `agg` を書く必要は無い。
- **`Stat.value`**: `aggregate` の代わり。`aggregate` と `value` は**排他でどちらか必須**。`value` のときは `entity/field/filter` を書かない(agg の中に書く)。`format` はそのまま効く(percent は 0..1 → %)。
- **`Field.type = "ref"` + `refEntity`**: 他エンティティのレコード参照。保存値はレコード id の文字列。入力は **`picker`**(選択肢 = refEntity のレコード、表示は titleField)。list/table では参照先の titleField を表示する。
- **`settlement`(Leaf)**: `{"type":"settlement","entity":"members","nameField":"name","balanceField":"balance"}` + `id?` `area?` `title?`。balance(正=受け取る・負=払う)から貪欲法(最大の債務者 → 最大の債権者に min(|負|, 正) を送る、を繰り返す)で「A → B ¥1,200」を列挙。割り勘専用の割り切り部品。合計 0 にならない端数(1 円未満の丸め差)は無視してよい。全員 0 なら「精算は不要です」。
- progressBar / gauge の `value` は従来どおり**数値リテラルのみ**(Expr は不可。後述 §8)。

## 5. 評価ルール(iOS)

- 型は 数値(Double)/ 文字列 / 真偽 / 日付 / 空。currency・integer フィールドは数値。ref フィールドの値は参照先レコード id(文字列)。
- 空のフィールド・評価エラー(0 割り・型不一致・参照先なし・`sqrt` の負数)は **nil → 表示は「—」**。nil を受けた op は nil を返す(`coalesce` だけは b を返す)。`agg` は 0 件なら count/sum は 0、avg/min/max は nil。
- computed 同士の依存は backend が DAG を保証する(循環は来ない)。評価順は依存順でも都度再帰でもよい。
- `today`/`now` と期間 filter(`today`/`thisWeek`/`thisMonth`/`thisYear`)は端末ローカル時刻(従来の stat filter と同じ)。
- **`thisWeek` は月曜 0:00 〜 次の月曜 0:00**。端末ロケールの週の始まり(日本の `Calendar` 既定は日曜)に従わず月曜に固定する: 日本の暦・学校・職場の「今週」は月〜日で数えるのが一般的で、日曜始まりだと日曜の記録が「来週」側に入り、週末に振り返る用途(家事当番・週の目標)で直感とずれるため。`thisYear` は 1/1 〜 12/31。

## 6. 検証ルール(backend)

`dsl.Parse` / `App.Validate` が以下を全部列挙して返す(AI の再生成プロンプトにそのまま渡る):

| ルール | エラー例 |
|---|---|
| op 名が §3 の一覧内 | `... unknown op "median" (allowed: abs add ...)` |
| 引数個数 | `... op sub takes exactly 2 args (got 1)` / `op add needs at least 2 args` |
| `ref` の存在 | `ref "nope" is not a field of entity "members" (fields: name, paid, ..., or "id")` |
| lookup(`a.b`) | `ref "title.name": "title" is string, not a ref field` / `"ghost" is not a field of entity "members"` / `only one level of "<refField>.<fieldId>"` |
| latest / earliest | `latest by "title" must be a date or datetime field` / `by is only for agg latest/earliest` |
| `ref` の行コンテキスト | `{"ref":"amount"} is not allowed here (no current record; stat.value is a screen value). Use {"agg":...} over an entity instead` |
| `agg` の entity/field/filter | `entity "ghost" does not exist` / `field "zzz" not in entity "expenses"` / `sum needs a numeric field but "title" is string` |
| 循環禁止(DAG は可・agg 経由も検出) | `computed field cycle: members.balance -> members.paid -> members.balance; ...` |
| 深さ ≤ 8(リテラル単体 = 1) | `expression is nested deeper than 8 levels; simplify it ...` |
| 粗い型検査 | `sub expects a number but got a string (wrap it in {"op":"toNumber",...})` / `computed expression yields a string but the field type is currency` |
| computed の型・読み取り専用 | `computed field type must be one of ...` / `remove required` / `inputs cannot bind to it` |
| `ref` 型 | `type ref needs refEntity` / `refEntity "people" does not exist` / `use a picker to choose the members record` |
| settlement | `settlement balanceField "name" must be number/integer/currency` |
| 期間 filter | `filter op thisWeek takes value null` / `filter op thisYear needs a date or datetime field but "name" is string` |
| `hoursBetween` の引数 | `hoursBetween expects a "HH:mm" time string (or a datetime field) but got a number` |
| `list.item.toggle` | `list item.toggle "name" must be a boolean field (got string)` / `... is computed (read-only)` |
| `list.item.resets` | `... with resets daily must be a date field (got boolean)` / `... is a date field; add "resets":"daily"` / `... must not have defaultValue` |
| `streak` / `doneInDays` | `streak needs {"ref": <daily check field>} of this entity` / `"started" is not a daily check; streak only works on a field used as list item.toggle with "resets":"daily"` / `number of days must be an integer literal 1..366` |
| Stat 排他 | `stat takes either aggregate or value, not both` / `stat needs aggregate (with entity) or value` |

## 7. レシピ

AI のカタログにも同じものが載る(`$defs/Expr.x-recipes`)。

1. **割り勘**(`examples/warikan.json`): members{name} + expenses{title, amount:currency, payer:ref→members, date}。
   - `paid` = `{"agg":"sum","entity":"expenses","field":"amount","filter":{"field":"payer","op":"eq","value":{"ref":"id"}}}`
   - `share` = `{"op":"round","args":[{"op":"div","args":[{"agg":"sum","entity":"expenses","field":"amount"},{"agg":"count","entity":"members"}]}]}`
   - `balance` = `{"op":"sub","args":[{"ref":"paid"},{"ref":"share"}]}`
   - ホーム: stat(合計・1 人あたり)+ members の table(名前/支払済/残高)+ settlement + 支出 list + 追加ボタン。
2. **BMI**(`examples/bmi.json`): `bmi` = weight / (height/100)^2 を小数 1 桁に(`round(x*10)/10`)。
3. **残り予算**: stat.value = `sub(予算, agg sum thisMonth)`。予算を後で変えたいなら budget{amount, date} entity を作り `agg latest budget.amount by date`(max だと下げた予算が反映されない)。
3.5. **今の値**: 今の体重 = stat `aggregate: latest, field: weight, by: date`。今の BMI = latest の field に computed bmi。増減 = `sub(latest, earliest)`。
3.6. **マスタの値**: 家事の点数 = log の computed `{"ref":"chore.points"}`、売上 = `mul({"ref":"item.price"}, {"ref":"qty"})`。
4. **達成率**: stat{format:percent, value: `div(agg sum, 目標)`} + progressBar{entity, field, target}。
5. **シフト・バイト代**: shift{date, start/end: string `"HH:mm"`(timePicker), wage: currency}。computed `hours` = `hoursBetween(ref start, ref end)`、`pay` = `round(hours * wage)`。今月の勤務時間 / 給料 = stat sum hours / pay filter date thisMonth。
   - **休憩を引く**(#229): `break_min: integer`(unit「分」)を足し、`hours` = `max(sub(hoursBetween(start, end), div(coalesce(ref break_min, 0), 60)), 0)`。coalesce が無いと、休憩欄を足す前の記録が「—」になる。休憩は `hours` の式に入れ、`pay` は `round(hours * wage)` のまま(currency の computed が `hoursBetween` を使って `mul` しないと backend が拒否する)。休憩を開始・終了の時刻で持つなら `sub(hoursBetween(start, end), coalesce(hoursBetween(break_start, break_end), 0))`。
   - **夜勤**: `hoursBetween` が日付またぎ(22:00→06:00 = 8)を扱うので式は変えない。今月の判定は `date`(出勤日)で行う。
   - 時給は記録の field(`wage` の defaultValue に時給)・式の定数(`mul(ref hours, 1100)`)・マスタ(`ref job.wage`)のどれでもよい。
   - 表示: hours は小数 2 桁まで(7.25 時間)・pay は ¥ と桁区切り(currency の unit「円」は付け足さない・#150)。
   - 時刻を `sub` で引くと backend は `hoursBetween` を使うよう返す(`toNumber("09:30")` は 9.5 にならない)。
6. **カウントダウン**: computed `days_left` = `daysUntil(ref date)`、最も近い予定 = `agg min days_left filter days_left gte 0`。
7. **毎日チェックの連続日数・直近 7 日**(`examples/medicine.json`): medicine{name, taken_on: date} + list `item: {"toggle":"taken_on","resets":"daily","trailing":"streak"}`。
   - `streak`(integer・unit 日) = `{"op":"streak","args":[{"ref":"taken_on"}]}`
   - `last7`(integer・unit 回) = `{"op":"doneInDays","args":[{"ref":"taken_on"},7]}`
   - 全体: 最長の連続 = stat.value `{"agg":"max","entity":"medicine","field":"streak"}`、直近 7 日の合計回数 = `{"agg":"sum","entity":"medicine","field":"last7"}`。実施記録 entity は作らない(丸を押しても増えず二重入力になる)。

## 7.5 一覧のワンタップ チェック(`list.item.toggle`, #108)

式ではないが、同じ「状態を変える最小の仕組み」としてここに置く。

- `list.item.toggle: "<boolean fieldId>"`。iOS は行の先頭にチェック丸を出し、タップでそのレコードの boolean を反転して**その場で保存**する(フォームを開かない・`PATCH records/{id}` 相当の 1 フィールド更新。オフライン DB に即反映 → 同期キューが送る)。
- 楽観的 UI: 丸は先に切り替わり、保存に失敗したら元に戻す。チェック済みの行は打ち消し線 + 薄く。
- `filter {field: done, op: eq, value: false}` で未完了だけ出している一覧なら、チェックした行はその場で消える(「買ったもの」一覧を `eq true` で並べれば戻せる)。
- backend 検証: toggle は同 entity の**保存される** boolean field(computed 不可)。
- 実例: `examples/shopping.json`。

### 毎日くり返すチェック(`list.item.resets: "daily"`, #132)

- 服薬・習慣のように「登録した項目が毎日未チェックで並ぶ」一覧。`item: {"toggle": "taken_on", "resets": "daily"}`。
- toggle は **保存される date field**(defaultValue なし。backend の既定値補完でも `"now"` を入れない)。丸を押すと今日の日付(端末ローカル `YYYY-MM-DD`)を保存、もう一度押すと空に戻す。**値が今日なら済み**と表示するので、日付が変わると全行が未チェックに戻る。
- 新しい entity は作らない。今日やった数 = `{"agg":"count","entity":"medicine","filter":{"field":"taken_on","op":"today","value":null}}`。
- **押した日の履歴(#172)**: iOS は丸を押すたびにレコードの `"<toggle field>__dates"`(例 `taken_on__dates`)に押した日を残す。DSL には宣言しない(appconfig が date field の隣のキーとして受け付ける)。
  ```json
  {"name": "血圧の薬", "taken_on": "2026-10-24", "taken_on__dates": ["2026-10-22", "2026-10-23", "2026-10-24"]}
  ```
  - 付けたら今日を足す・**外したら今日だけ消す**。toggle field は後方互換で「最後にやった日」のまま(付けたら今日、外したら残った最新日・無ければ空)なので、「値が今日なら済み」の判定と `filter op today` は従来どおり。
  - 昇順・重複なし・最大 400 日(古い方から捨てる)。appconfig は `YYYY-MM-DD` 以外を 422 で拒否し、並べ替え・重複除去して保存する。
  - 履歴の無い旧レコード(#132 の lastDoneDate だけ)は toggle の値を履歴の 1 件として読む(次に押した時に配列が作られる)。
  - 連続日数 = computed `{"op":"streak","args":[{"ref":"taken_on"}]}`、直近 7 日の回数 = `{"op":"doneInDays","args":[{"ref":"taken_on"},7]}`(§3・§7 の 7)。
- 割り切り: 1 日に同じ項目を 2 回以上は持てない(朝・夜は項目を分ける)。履歴は押した日だけで時刻は持たない。別端末で同じ日に同時に押した場合は後勝ち(配列ごと上書き)。
- iOS の「今日」は DEBUG ビルドだけ環境変数 `METAMON_DEBUG_TODAY=YYYY-MM-DD` で差し替えられる(日付境界の確認用)。
- 実例: `examples/medicine.json`。

## 8. 割り切り(やらないこと)

「0 と 1 を管理して PC 上に PC を作る」状態にしないため、以下は**やらない**:

- 変数・代入・ループ・ユーザー定義関数・ラムダ・再帰。式はツリー 1 本で、深さ 8 まで。
- 配列/オブジェクト値、map/filter/reduce、文字列の部分抽出・正規表現・フォーマット指定(表示は `format`/`unit` に任せる)。
- 状態を持つ計算(累積・前回値との差・移動平均)。「最新 / 最古の 1 件」だけは専用の `agg`(latest / earliest)で足した。
- lookup の多段(`a.b.c`)。必要なら参照先 entity に computed を作って 1 段で読む。
- 副作用(式からの保存・通知・遷移)。
- サーバー側評価・集計 API(評価は iOS。件数が増えたら README 決定表 #10 の集計 API と合わせて検討)。
- progressBar/gauge の `value`/`target` を Expr にすること(stat.value で代替可能・iOS 契約の拡大を避けた。必要なら後段で)。
- タイムゾーン指定・通貨換算。

op を増やすときは `dsl.schema.json`(`ExprOp.op` の enum と x-enumDescriptions)、`backend/pkg/dsl/expr.go`(`Ops`)、本書 §3、iOS の評価器を同時に更新する。
