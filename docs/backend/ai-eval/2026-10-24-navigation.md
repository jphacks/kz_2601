# AI eval: 複数画面とナビゲーション(#114 #115)

- 日付: 2026-10-24 / モデル claude-sonnet-5-5 effort=low / 非ストリーム / lint 警告での再生成は既定 OFF(`AI_LINT_RETRY` 未設定)
- 改善前 = feature/gen-quality(#124)のプロンプト + 本 PR の検証メッセージ。改善後 = 本 PR のプロンプト(画面の分け方 + 遷移を変える edit のレシピ)
- 各 1 周(36 ケース = 既存 28 + nav 8)。Claude 呼び出しは再生成込みで改善前 38 回・改善後 36 回
- raw: `raw/2026-10-24-claude-sonnet-5-5-low-nav-before.json` / `raw/2026-10-24-claude-sonnet-5-5-low-nav-after.json`
- 比較: `go run ./cmd/ai-eval -compare-before '../docs/backend/ai-eval/raw/*nav-before.json' -compare-after '../docs/backend/ai-eval/raw/*nav-after.json'`

## 追加したケース(`cases.json` の `group: nav`)

| id | 種類 | base | 発話 | expect |
|---|---|---|---|---|
| nav_study | create | - | 今日の記録、科目ごとの合計、目標の設定を別の画面に分けて下のタブで切り替え | tabsMin 3 / unreachable 0 |
| nav_recipe | create | - | レシピ一覧から開いて中身を見る。買い物リストは別タブ | tabsMin 2 / unreachable 0 |
| nav_gym | create | - | 記録・体重の推移・プロフィールの 3 画面をボトムバーで | tabsMin 3 / unreachable 0 |
| nav_edit_link | edit | example:kakeibo | 集計の画面からも支出を追加できるようにして | patch / unreachable 0 / nav stats>expense_form |
| nav_edit_add_tab | edit | example:kakeibo | 履歴のタブを足して | patch / unreachable 0 / tabs 3 |
| nav_edit_remove_tab | edit | example:kakeibo | 集計のタブは要らないので消して | patch / unreachable 0 / tabs 0 |
| nav_edit_reorder | edit | example:kakeibo | タブの順番を入れ替えて、集計を左に | patch / unreachable 0 / tabOrder stats,home |
| nav_edit_to_tab | edit | example:warikan | 精算の結果を別のタブに分けて | patch / unreachable 0 / tabsMin 2 / tabHas settlement |

`patch` = JSON Patch で保存された(全体の書き直しへの格下げ無し)、`unreachable` = `dsl.Unreachable()` の件数、`nav` = `dsl.HasNav(from,to)`。

## 結果

| 指標 | 改善前 | 改善後 |
|---|---|---|
| 成功 | 36/36 | 36/36 |
| 一発通過(再生成なし) | 34/36 | **36/36** |
| nav 8 ケースの expect 達成 | 8/8 | 8/8 |
| 到達できない画面 / 画面グラフの警告(全 36 出力) | 0 / 0 | 0 / 0 |
| 直下の部品が 8 個を超える画面 | 0 | 0 |
| create p50 (Claude ms) 全体 | 5729 | 5642 (-2%) |
| create p50 既存 28 ケース | 5521 | 5478 |
| edit p50 既存 28 ケース | 1607 | 1485 |
| edit p50 全体 | 1607 | 1397 (-13%) |
| 平均出力 tokens create / edit | 1129 / 141 | 1093 / 131 |
| expect miss | 2 | 2(どちらも既存の calendar / edit_calendar の「代替」言及漏れ。nav とは無関係) |

| id | 改善前 | 改善後 |
|---|---|---|
| nav_edit_remove_tab | 1 回目がタブ 1 個(エラー)→ 再生成 2526ms | 1 回で `/app/tabs` ごと remove + stats 画面も remove 1307ms |
| edit_remove_chart | 同上(グラフ削除で集計タブが 1 個残る)2548ms | 1 回で通過 1348ms |
| nav_edit_link | 「集計画面にツールバーの追加ボタンを付けました。」 | 「集計画面から支出を追加できるボタンを足しました。」(レシピの summary の形) |
| nav_edit_reorder | 出力 110 tokens | move 1 つで 58 tokens |

## 所見

- 改善前でも、複数画面の新規作成と遷移の edit はほぼできていた(モデルが強い)。確実に効いたのは**タブを消して残りが 1 個になる時**の扱い。レシピ「残り 1 個なら `/app/tabs` ごと remove、辿れなくなる画面も remove」で再生成 2 件(1 件あたり約 1.2 秒)が無くなった。
- 既存ケースの成功率と p50 は悪化なし(1 周ずつの比較なので、±15% 程度は揺れの範囲)。
- タブ付きの新規作成は 8 件から 6 件に減った(training_b と habit_en がタブ無しに)。どちらも発話に別画面の要求が無いケースで、「発話に無い役割を増やさない」規則どおりの変化。
- nav の expect は改善前から全部通っていたので、このケースセットでは差が出にくい。より難しい発話(画面を 3 つ以上またぐ遷移の変更など)は後続で追加する。
