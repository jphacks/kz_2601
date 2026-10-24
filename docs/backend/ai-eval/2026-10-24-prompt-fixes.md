# ai eval: 生成内容の品質修正(プロンプト規則 + lint)(2026-10-24・#87 / #107 §6 / 親 #99)

品質監査(#107・[2026-10-24-audit.md](2026-10-24-audit.md))で残った「画面はあるが中身が嘘・欠けている」問題を、**プロンプト規則**と**検証(`pkg/dsl.Lint`)+ 1 回だけの再生成**で減らした。式ブランチ(#100)単体を改善前として、同じ条件で比べた。

- ブランチ: `feature/gen-quality`(base `origin/feature/logic-expr-backend` + `origin/feature/quality-audit` をマージ)
- モデル: `claude-sonnet-5-5`・effort low・logic pass off(既定)
- ケース: `backend/internal/ai/eval/audit_cases.json`(20 本)+ `cases.json`(既存 28 本。新 5 本 `logic_*` を含む)
- 生 JSON(`raw/`): `2026-10-24-claude-sonnet-5-5-low-{before,after1,after2}-{audit,cases}.json`
  - before = 式ブランチ単体(本ブランチの最初のコミット `d2bc294` 時点)
  - after1 = プロンプト §6 + lint(`0d8cb83`)、after2 = 1 周目で残った欠落への追加規則(`e42d5eb`)。最終コミット `100ab95` は lint の誤検出修正だけで、プロンプトは after2 と同じ。

```sh
cd backend
go run ./cmd/ai-eval -env-file ../../.env -cases internal/ai/eval/audit_cases.json -tag after2-audit
go run ./cmd/ai-eval -env-file ../../.env -cases internal/ai/eval/cases.json -tag after2-cases
go run ./cmd/ai-audit -run ../docs/backend/ai-eval/raw/2026-10-24-claude-sonnet-5-5-low-after2-audit.json \
  -compare ../docs/backend/ai-eval/raw/2026-10-24-claude-sonnet-5-5-low-before-audit.json
```

## 目次
1. [結論](#1-結論)
2. [ケース別の採点(手判定)](#2-ケース別の採点手判定)
3. [入れたプロンプト規則](#3-入れたプロンプト規則)
4. [入れた検証(lint)と再生成](#4-入れた検証lintと再生成)
5. [生成成功率とレイテンシ](#5-生成成功率とレイテンシ)
6. [残る問題と次の手](#6-残る問題と次の手)

## 1. 結論

| 指標(audit 20 本・needs 全 69 項目) | 改善前(式ブランチ) | after1 | **after2(最終)** | 目標 |
|---|---|---|---|---|
| needs ○/△/×(手判定) | 53 / 4 / 12(○ 77%) | 55 / 4 / 10(○ 80%) | **59 / 4 / 6(○ 86%)** | ○ 85% 以上 |
| 要求を全部満たしたアプリ | 9/20 | 8/20 | **11/20** | — |
| 嘘ラベル(中身とラベルが違う・壊れた式) | 4/20 | 2/20 | **0/20** | 0〜1/20 |
| 表せない要求の代替を申告 | 0/20 | 8/20 | 6/20(要る 6 本は全部申告) | — |
| 生成成功(audit / 既存) | 20/20 / 28/28 | 20/20 / 28/28 | **20/20 / 28/28** | 悪化させない |
| create p50(audit / 既存) | 6,201 / 5,467 ms | 6,084 / 5,406 ms | **6,603 / 5,727 ms**(+0.4 / +0.3 秒) | +1 秒以内 |
| create p95(audit / 既存) | 8,707 / 7,105 ms | 15,719 / 7,807 ms | 18,735 / 14,983 ms | (下記の懸念) |
| lint で再生成したケース(audit / 既存) | — | 1 / 1 | 2 / 2 | — |

> 注: 計測後に「1 件だけの設定 entity(予算・目標など、stat.value / computed の agg や stat 等からしか読まれず ref もされない)の一覧なし」を lint から外した(API 再実行なし)。既存の再生成 2 本(logic_budget・logic_oshi)はこの警告だけが原因なので、再生成されなくなる見込み(既存の lint 再生成 2/28 → 0/28 相当・raw に lint を当て直して確認)。

- 改善前の嘘ラベルは監査レポートの 2/20 より多い 4/20 になった(同じブランチでも 1 周ごとに揺れる。今回は「最新の体重 = 今日の平均」「筋トレ連続日数 = 直近7日の件数」も嘘に数えた厳しめの基準。after も同じ基準)。
- **嘘ラベル 0** の内訳: 時刻差を `toNumber("18:00")` で埋めた壊れた式(a03)→ 入力 + 申告、「今年の合計」= 全期間(a11)→ `year == year(today)` で本当に今年、「最新の体重」(a12)→「平均体重」、「連続日数」(a08)→「直近7日の筋トレ日数」+ 申告。
- **充足 +6**: 科目別・月別の合計(a04 / a11 の chart groupBy)、今年の絞り込み(a09 / a11)、種目ごとの自己ベスト表(a06)。
- ai-audit の機械判定は after2 54/4/11・改善前 56/1/12 で逆転して見えるが、機械判定は「式があるか」までしか見ない(改善前 a03 の壊れた式を ○ と数える、chart groupBy の科目別を △ にする等)。表の値は監査レポートと同じく DSL を読んで手で直した値。

## 2. ケース別の採点(手判定)

| id | 改善前 ○/△/× | after2 ○/△/× | 変わったところ(after2) |
|---|---|---|---|
| a01_warikan | 4/0/0 | 4/0/0 | — |
| a02_circle_dues | 4/0/0 | 4/0/0 | members の一覧が無く lint で再生成 → 未払い/支払済の 2 一覧 |
| a03_shift_wage | 0/0/4 ✗嘘 | 3/0/1 | 壊れた時刻差の式 → 勤務時間を入力 + `pay = hours × 1100`、「勤務時間の自動計算は入力で代替しました。」 |
| a04_exam_countdown | 3/0/1 | **4/0/0** | 科目ごとの合計 chart、目標までの残り時間 stat、試験の一覧 |
| a05_cooking_cost | 3/0/0 | 3/0/0 | — |
| a06_gym_pr | 2/1/0 | **3/0/0** | 種目 entity + `best = max(weight where exercise == id)` の表、更新判定は「その日より前の最大」と比較 |
| a07_pomodoro | 3/0/1 | 3/0/1 | 自動切り替えは申告。記録一覧は開けない画面に置かれた(lint を到達可能判定に修正済み・§6) |
| a08_habit_streak | 1/0/1 ✗嘘 | 1/0/1 | 「直近7日の○○日数」+ 7 日 filter(1 周目は filter が無く嘘 → lint 追加)、連続日数は申告 |
| a09_reading_month | 2/1/0 | **3/0/0** | computed `year` + filter `year == year(today)` |
| a10_saving_goal | 3/0/0 | 3/0/0 | — |
| a11_part_time_income | 2/0/2 ✗嘘 | **4/0/0** | 今年の合計・103 万円までの残りを今年で計算、月ごとの合計 chart(bucket month) |
| a12_bmi | 2/0/0 ✗嘘 | 2/0/0 | 「最新の体重(今日の平均)」→「平均体重」 |
| a13_medicine | 3/0/0 | 3/0/0 | 今日飲んだ回数を `taken == true` で数えるようになった |
| a14_shopping_clear | 2/0/1 | 2/0/1 | 一括削除は「スワイプ削除で代替しました」と申告 |
| a15_game_ranking | 4/0/0 | 3/1/0 | 改善前は偶然 `rank = 1 + count(total > 自分)` を作った。after2 は降順の表 + 申告(揺れ) |
| a16_trip_plan | 3/0/0 | 2/1/0 | 時刻が string で入力欄なし・timeline は日付順だけ(揺れ。規則とは無関係) |
| a17_oshi_spend | 3/0/1 | 3/0/1 | カテゴリ別の合計が出ない(chart 規則が効かなかった) |
| a18_plant_water | 3/0/1 | 3/0/1 | 「教えて」を通知と解釈しない(通知は付けない規則が優先) |
| a19_calorie_en | 3/0/0 | 3/0/0 | 残りを `max(…, 0)` に |
| a20_chores_en | 3/2/0 | 3/2/0 | 家事マスタを ref で使い、追加フォームと一覧が付いた(改善前は行き止まり)。今週 = 直近7日・順位 = 降順を申告 |

既存 `cases.json` の新 5 本(logic_*)は after2 で 5/5 成功。割り勘・BMI・カウントダウンは改善前と同じ形、予算・推し活は目標/予算 entity に一覧が付いた(どちらも lint で 1 回再生成)。

## 3. 入れたプロンプト規則

`backend/internal/ai/prompt.go` の設計原則に「中身を正しく」節を追加(`TestSystemPromptHasQualityRules` で消えないようにした)。

| 規則 | 狙い(監査 / #87) |
|---|---|
| ラベルは計算した中身どおり。計算できないならラベルを実際の値(合計・全期間)に | 嘘ラベル |
| 今月/今日のラベルは filter thisMonth/today 必須。今年 = computed `year` + `filter year == year(today)`、今週 = 直近7日 filter + 申告。「直近7日」と書いたら filter 必須 | #87-1・a09/a11/a20 |
| 「最新」を返す集計は無い。今日の値は「今日の○○」+ today、それ以外は降順 list の先頭 | a12 |
| date field の gt/lte に `"now"` や日付文字列を書かない(式 or `days_left` の computed) | a18(develop) |
| 「計算して」と言われた値を入力欄にしない・説明文に式を書かない | a03/a12(develop) |
| 人は entity + ref、人ごとの合計は computed、順位はその computed で table を降順 + 申告 | a01/a15/a20 |
| 目標があれば `stat.value = max(目標 − sum, 0)` で残りを数値に | a04/a10/a19 |
| entity には追加フォームと一覧を必ず置く。マスタは ref で使う | a04/a20 |
| 時刻差は意味の無い式で埋めず、時間を入力 + それを使う計算は computed + 申告 | a03 |
| 「○○ごと/別/月ごと」の合計はホームに chart(bar, groupBy / bucket month)を 1 つ(可視化を求められていなくても) | a04/a11/a17 |
| count の単位: 件が合わない数は `stat.value = {"agg":"count",...}`(単位なし)+ ラベルに単位 | #87-2 |
| フォームは photoField だけ(同じ field の image を重ねない)・section の area は子の数 + 1 行か area 省略 | #87-3/#87-6 |
| 数字は半角 | #87-5 |
| 代替の申告を部品以外(計算・状態・期間・操作)に広げ、例(連続日数 → 直近7日、順位 → 降順、一括削除 → スワイプ、今年 → 全期間、時刻差 → 入力)を示す | 黙って落とす |

カタログ(`catalog.go`)はスキーマ(`docs/dsl/dsl.schema.json` の description / x-recipes)から自動生成なので変えていない。few-shot(examples/*.json)は docs/dsl/examples のバイトコピーでテストが一致を見ているため変えず、規則の中に短い JSON 片を書いた。

## 4. 入れた検証(lint)と再生成

`backend/pkg/dsl/lint.go` の `(*App).Lint() []string`。`Validate` のエラーではなく**警告**(保存済みの DSL は今までどおり読める)。

| 警告 | 例(監査) |
|---|---|
| date/datetime field への gt/gte/lt/lte filter に文字列・数値リテラル(画面部品の filter と agg の filter の両方)。正しい書き方(`today` / `addDays` / `days_left` の computed)を文中で示す | a18 `lte "now"` |
| 追加フォームが無い / フォームに行けない / 到達できる画面に一覧が無い entity | a04 exam・a20 chores・a07 |
| 常に定数になる式(`mul(x, 0)` を含む、全体が定数に畳める) | a03 `x*0+0-0` |
| stat / progressBar / gauge のラベルに今月・今日があるのに filter が無い・別の期間(「今日まで」「今月の予算」は除外) | #87-1 |
| ラベルに今週・今年があるのに式の date filter が無い / 直近N日なのに date filter が無い / 「最新」 | a08・a11・a12・a20 |

再生成(`backend/internal/ai/generate.go`):
- **create**: 検証を通った出力に lint 警告があれば、その警告を「表示できますが中身に問題があります」として 1 回だけ再生成。再生成が検証で落ちたら最初の出力を返す(lint では生成失敗にしない)。
- **edit**: パッチで**新たに増えた**警告だけを対象に同じく 1 回(配列 index の違いは無視して比べる)。既存アプリの古い警告で毎回作り直さない。
- `Meta.lintRetried` / `Meta.lint`(最終出力に残った警告)をレスポンスに追加(加算のみ)。
- 決定的な後処理: フォーム画面で photoField と同じ field の image を除去(#87-3)、summary の全角数字を半角に(#87-5)。

after2 の出力に lint を当てると audit 0/20・既存 0/28(改善前は 4/20・3/28)。

## 5. 生成成功率とレイテンシ

| | 改善前 | after1 | after2 |
|---|---|---|---|
| 成功(audit / 既存) | 20/20・28/28 | 20/20・28/28 | 20/20・28/28 |
| 1 回で通過(audit / 既存) | 20・27 | 18・26 | 18・25 |
| create p50(ai-eval 集計) | 6,201 / 5,467 ms | 6,084 / 5,406 ms | 6,603 / 5,727 ms |
| create p95 | 8,707 / 7,105 ms | 15,719 / 7,807 ms | 18,735 / 14,983 ms |
| edit p50 | 1,540 ms | 1,451 ms | 1,915 ms(全 8 本 1 回で通過・揺れ) |
| 平均出力トークン(create, audit) | 1,219 | 1,463 | 1,549 |

- p50 は +0.3〜0.4 秒で目標(+1 秒)内。出力トークンは +27%(computed / chart / 代替の申告が増えた分)。
- **p95 は約 2 倍**: lint で再生成したケース(全 48 本中 4 本)は 2 回分(13〜24 秒)かかる。うち 2 本は予算・目標の 1 件だけの entity に一覧が無い警告(logic_budget / logic_oshi)。

## 6. 残る問題と次の手

| 残り | 対応 |
|---|---|
| a07: 記録一覧を開けない画面に置いた | lint を到達可能な画面だけで判定するよう修正済み(`100ab95`・API 再実行なし) |
| a17: カテゴリ別の合計 chart が出ない | chart 規則の揺れ。few-shot に「カテゴリ別の合計」を入れる(docs/dsl/examples の更新が要る) |
| a18: 「教えて」で通知が付かない | 「発話に無い通知を足さない」規則との兼ね合い。要るなら「今日〜が必要な○○を教えて」を通知の合図に足す |
| a16: 時刻を string にして入力欄なし・日付順だけ | 「時刻は datetime(includeTime)」の規則を足す候補 |
| 連続日数・順位番号・一括削除・タイマー切り替え・今週 | 表せない(DSL の拡張が要る)。今回は申告で手当て。#108 / #109 / 時刻差の関数は後続 |
| p95 の増加 | 1 件だけの設定 entity の「一覧なし」を lint から外した(再生成は計測時の 4/48 から 2/48 相当に) |

> **2026-10-24 追記(ユーザー判断)**: lint 警告による再生成は既定で OFF にした(`AI_LINT_RETRY=1` で有効)。「1 回で完璧でなくても、声で直していけばよい」ため。警告は `Meta.lint` に載せるだけ。上の改善幅の大半はプロンプト規則によるもので、再生成が走ったのは計測時 48 本中 4 本(設定 entity の除外後は 2 本相当)。
