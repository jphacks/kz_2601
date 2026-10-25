# 配列(1 件に複数の値)の扱い — 調査 #235

2026-10-25 時点の develop(fb24ccc)。

## 結論

- **DSL に配列の field 型は無い。** field の type は `string text integer number boolean date datetime enum currency photo video location ref` の 13 種で、どれも値は 1 つ(schema `Field.type`・Go `dsl.Field*`・iOS `FieldCoding`)。
- 今は **「値 1 つ = 1 件の子 entity(親を ref で指す)」** で表す。割り勘の `payer`(ref 1 つ)と同じ仕組みを、中間の entity に使う形。
- 子 entity なら集計・グラフ・件数はおおむね作れる。できないのは **複数選択の入力部品** と **親の画面に、その親の子だけの一覧を出すこと**。

## できる / できない

| 項目 | 今 | 補足 |
|---|---|---|
| 配列の field 型(`tags` / 複数 ref) | ✗ | 型が無い。iOS の `JSONValue.array` は内部用(毎日チェックの履歴 `<date>__dates` だけ) |
| 複数選択の入力部品 | ✗ | `picker` は enum / ref を 1 つ選ぶだけ。子を 1 件ずつ追加するフォームで代わりにする |
| 一覧での表示 | △ | 子の list(subtitle に親の見出し)は出せる。親の行に「小説, 技術」とまとめて出すことはできない(件数の computed なら出せる) |
| 親の画面にその親の子だけ出す | ✗ | list の `filter.value` に `{"ref":"id"}` が書けない(画面の部品には行が無い。`checkComponentFilterValue`) |
| 絞り込み(〜を含む) | △ | 子の list を `tag eq "小説"` で絞ることはできる(enum の時)。親の list を「このタグを含む本」で絞ることはできない(`contains` の op が無い) |
| 集計: 親ごとの件数・合計 | ○ | 親の computed `{"agg":"count","entity":<子>,"filter":{"field":<親 ref>,"op":"eq","value":{"ref":"id"}}}` |
| 集計: 値(タグ)ごとの件数 | ○ | 子の `chart groupBy <enum>`、またはタグのマスタ entity の computed |
| グラフ: ref で groupBy | △→○ | **ラベルがレコード id のまま表示されていた**(下の「直したもの」)。この PR で参照先の見出しを出すように直した |
| appconfig の値の検証 | ○ | 宣言した field に配列を送ると **422 `invalid_record`**(500 にも、黙って保存にもならない)。`TestNormalizeDataRejectsArrays` で固定した。配列を受けるのは `<date>__dates` だけ |
| 文字列に詰める(`"小説, 技術"`) | △ | string として保存はできるが、絞り込み・集計に使えない |

## AI の作り方(本物の API・claude-sonnet-5-5 effort low)

前 = develop のまま、後 = この PR のプロンプトの規則を足したあと。どちらも 5/5 が検証を通った(後の recipe は無関係な `placeholder` で 1 回作り直し)。

| 依頼 | 前: 作られた形 | 前: 要望は満たせるか | 後: 作られた形 | 後: 要望は満たせるか |
|---|---|---|---|---|
| 読んだ本にタグを複数 | `book.tag` = ref 1 つ + タグのマスタ。「1 冊に 1 つのタグで代替」と申告 | ✗ 複数付けられない。タグ別グラフは id のラベルになっていた | `book_tag{book: ref, tag: enum}` + 本ごとのタグ数の computed + `chart groupBy tag` | ○ 複数付けられ、タグ別の冊数も出る(1 件ずつ追加。申告あり) |
| レシピの材料のリスト | `ingredient{recipe: ref, name, amount}`、材料は別タブの一覧 | △ 登録はできるが、レシピの画面に材料が出ない。申告なし | 同じ形 + レシピごとの材料数 + 申告 | △ 同じ制約(申告するようになった) |
| 割り勘の対象メンバーを複数 | `targets{expense: ref, member: ref}` + 1 人あたりの computed + settlement | ○ 計算は合う。対象を 1 人ずつ追加するので手間 | `splits{expense, member}` + `target_count` + settlement | ○ 同じ(計算は合う) |
| 持ち物チェックリスト | `item{name, packed: boolean}` + `list.item.toggle` | ○ 1 項目 = 1 件で配列は要らない | 未チェック / 済みの 2 つの list | ○ |
| 1 日のやることを複数 | `task{title, date, done}` + toggle | ○ 1 項目 = 1 件で配列は要らない | 今日で絞った list + 履歴タブ | ○ |

→ 「チェックリスト」「やること」は 1 項目 = 1 件で足りていて、問題は **1 件に複数の値を付けたい時(タグ・材料・対象メンバー)** だけ。前は対応がばらばらで、タグは黙って単数に減らしていた。

## 直したもの(この PR)

1. **プロンプト**(`backend/internal/ai/prompt.go`): 「1 件に複数の値」の規則を足した。子 entity + 親 ref で作る / カンマで詰めない / 単数に減らさない / 親ごとの件数は computed / 「1 件ずつ追加する形で代替しました」と申告する。
2. **iOS のグラフ**(`Aggregator.buckets` に `label`・`ChartLeaf`): ref の field で groupBy した時、軸と凡例が参照先の見出しになる(前は record id が出ていた)。
3. **テスト**: `TestNormalizeDataRejectsArrays`(Go)、`testBucketsRelabelRefGroupBy`(MetamonDSL)。

## 提案(優先順)

#231(割り勘の対象メンバー)で配列の型が入るなら、**その型に合わせる**。同じ意味の型を 2 つ作らない。

1. **複数の ref(`refs` / `ref` + `multiple: true`)** — 値は record id の配列。入力は複数選択の picker(チェック付きの一覧シート)、一覧は見出しを「、」でつなぐ。検証は id 文字列の配列(上限 50 件など)。割り勘の対象・本のタグ(タグのマスタ)・参加者がこれ 1 つで書ける。
2. **filter の `contains` op** — 配列の field に「この値を含む」。chart の groupBy は値ごとに 1 回数える(1 件が複数のグループに入る)。computed の `agg` も同じ。
3. **複数選択の enum(`enum` + `multiple: true`)** — 選択肢が決まっているタグ向け。1 と同じ仕組みで、値が選択肢の文字列なだけ。1 を入れれば後回しでよい。
4. **親の画面の子一覧**(list の `filter.value` に画面のレコード id を使う。例 `{"ref":"$record"}`)— レシピの材料のように、子 entity のままで済むものはこれで足りる。配列の型より小さく、1〜3 とは別に効く。

## 新しい Issue の本文案

> **タイトル**: DSL: 複数の値を持つ field(複数の ref / 複数選択)と `contains` の絞り込み
>
> ## 背景
> #235 の調査(`docs/dsl/arrays-audit.md`)で、DSL に配列の field 型が無く、「タグを複数」「対象メンバーを複数」は子 entity を 1 件ずつ追加する形で代わりにしていると分かった。入力の手間が大きく、親の一覧に値をまとめて出せない。
>
> ## やること
> - [ ] field に `multiple: true`(type ref / enum)を足す。値は文字列の配列(schema・Go `dsl`・iOS `Field`)
> - [ ] appconfig `normalizeValue`: 配列を検証する(要素の型・件数の上限・enum は選択肢に含まれること)。単数の値が来たら 1 要素の配列にする
> - [ ] iOS: 複数選択の入力部品(チェック付きの一覧シート)、一覧・表での表示(見出しを「、」でつなぐ)
> - [ ] filter に `contains` op。chart の groupBy・`agg` は要素ごとに数える
> - [ ] プロンプトとカタログ: 「1 件に複数の値」の規則を子 entity から `multiple` に切り替える(#235 で足した規則を置き換える)
> - [ ] ai-eval: タグ複数・割り勘の対象メンバーのケースを足す
>
> ## 注意
> - #231 で割り勘の対象メンバー用に型が入るなら、その型を広げる形にして二重に作らない。
> - 単数から複数に変える DSL の編集で、古い記録(文字列 1 つ)を壊さないこと。

> **タイトル(小さい方・別 Issue にする場合)**: list: 親の画面に、その親の子レコードだけを出す
>
> レシピの画面に材料の一覧を出したいが、list の `filter.value` で画面のレコード id を参照できない。`{"ref":"$record"}`(画面を開いた recordId)を filter.value に書けるようにする(Go の検証・iOS の評価・プロンプト)。子の追加フォームでは、親の ref を自動で入れる。
