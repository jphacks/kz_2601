# ai-eval の回し方(#371)

`backend/cmd/ai-eval` は、発話のケースを実際の Claude API に流し、`cmd/ai` と同じ `ai.Service` で DSL を作って、ケースごとの指標と DSL を JSON に書く。このフォルダには日付ごとの結果(`YYYY-MM-DD-*.md`)と生の JSON(`raw/`)を置く。

## 目次

1. [何を呼ぶか](#1-何を呼ぶか)
2. [回し方](#2-回し方)
3. [ケースの置き場と形](#3-ケースの置き場と形)
4. [結果の置き場](#4-結果の置き場)
5. [-compare の読み方](#5--compare-の読み方)
6. [フラグ](#6-フラグ)
7. [費用](#7-費用)
8. [関係する文書](#8-関係する文書)

## 1. 何を呼ぶか

- 呼ぶのは Claude API だけ。保存先はメモリ上の store(`appconfigclient.NewMemory()`)で、HTTP サーバーも Firestore も使わない。stg・本番のサービスやデータには触れない。
- 「stg で回す」専用のモードは無い。stg と同じモデル・設定で試したい時は、`-model` / `-effort` / `-logic-pass` / `-server-rules` などのフラグで合わせる(推測: stg の設定値は Cloud Run の環境変数で決まるので、そちらを見てから合わせる)。
- `-check` / `-check-all` / `-score` / `-compare-*` は API を呼ばない。CI(`backend-ci` の「ai-eval cases (offline)」)は `-check-all` だけを回している。

## 2. 回し方

コマンドはすべて `backend/` で打つ(既定のパスが `backend/` からの相対になっている)。

### API を呼ばない

```sh
cd backend
go run ./cmd/ai-eval -check -cases internal/ai/eval/m0disc_cases.json   # 1 ファイルの形を確かめる
go run ./cmd/ai-eval -check-all internal/ai/eval                         # 全ファイル(CI と同じ)
go run ./cmd/ai-eval -score '../docs/backend/ai-eval/raw/*-jphacks*.json' # 保存した結果のグループ別 pass%
```

### 実 API で回す

API キーは環境変数 `ANTHROPIC_API_KEY` から読む。`.env` を使う時は 2 通りある。

```sh
cd backend
# 1) シェルに読み込む
set -a; source ../.env; set +a
go run ./cmd/ai-eval -effort low -out ../docs/backend/ai-eval/raw

# 2) -env-file で読ませる(値は出力しない・すでにある環境変数は上書きしない)
go run ./cmd/ai-eval -env-file ../.env -only kakeibo_a,walk_a -tag local
```

`-env-file` の読み方(`loadEnvFile`): 1 行に `KEY=VALUE`。先頭の `export `・値の前後の `"` `'`・`#` で始まる行は読み飛ばす。

よく使う形:

```sh
go run ./cmd/ai-eval -cases internal/ai/eval/feed_cases.json -tag feed          # ケースファイルを選ぶ
go run ./cmd/ai-eval -stream -repeat 2 -only kakeibo_a,walk_a                 # SSE の経路・最初のイベントまでの時間
go run ./cmd/ai-eval -chat                                                    # 対話(聞き返し)のフェーズ
go run ./cmd/ai-eval -chat -cases internal/ai/eval/feed_chat_cases.json       # 対話のケースファイルを選ぶ
go run ./cmd/ai-eval -server-rules -cases internal/ai/eval/server_rules_cases.json  # 自動の動きの呼び出しも入れる
```

- `-only` で編集(`kind: edit`)のケースだけを選ぶと、元になる `base` のケースが同じ実行に無いので `internal/ai/examples/kakeibo.json`(英語は `expense_en.json`)で代わりに始める。元のケースも `-only` に入れる。
- 実行の最後に、ケースごとの表とグループごとの pass% が出る。基準を下回ったら赤にする使い方は [ci-gate.md](ci-gate.md)。

## 3. ケースの置き場と形

置き場は `backend/internal/ai/eval/*.json`。ファイルの形は 3 つあり、`-check-all` が中身から見分け、名前と合わなければ失敗にする(`checkall.go` の `detectShape`)。

| 形 | ファイルの例 | 回し方 |
|---|---|---|
| 生成のケース | 名前が `cases.json` で終わる(`cases.json`・`feed_cases.json` など) | 既定(`-cases`) |
| 対話のケース | 上のうち名前に `chat` を含む(`chat_cases.json`・`feed_chat_cases.json` など) | `-chat -cases` |
| DSL(編集の元) | 上のどちらでもない名前(`base_warikan_v1.json`) | ケースの `base` に `example:../eval/<名前>` と書いて読む |

生成のケース 1 件の形(`Case`・`main.go`):

```json
{
  "id": "kakeibo_a",
  "kind": "create",
  "group": "demo-A",
  "transcript": "毎日の出費をメモできる家計簿アプリを作って。…",
  "expect": { "photo": true, "chart": true, "oneAdd": true }
}
```

| キー | 意味 |
|---|---|
| `base` | `kind: edit` の元。同じ実行の前のケースの `id`、または `example:<名前>`(`internal/ai/examples/<名前>.json`) |
| `expect` | 結果の判定。使えるキーは [expect-keys.md](expect-keys.md) |
| `group` | pass% を数える単位 |
| `id` | ケースの名前。ファイルの中で重ねない |
| `kind` | `create` か `edit` |
| `locale` | `ja` / `en`。無ければ発話から決める(`-locale` で全件を上書き) |
| `needs` | `cmd/ai-audit` が読む「要る機能」。ai-eval は判定しない |
| `note` | 人向けのメモ |
| `pendingExpect` | 開いている PR が足すキー。キーが `expect.go` に入るまでは判定しない(`coverage.go`) |
| `textImage` | このケースだけ `AI_TEXT_IMAGE=1` と同じにする |
| `transcript` | 発話 |

- `expect` の無いケースは pass% に数えない(表の「no expect」に出る)。
- 対話のケースは 1 件が複数の `steps` を持ち、各 step の `expect` は `question` / `ready` / `answer` / `proposal`。ほかのキーは `chat.go` の `ChatStep` を見る。
- 知らない `expect` のキー・型の違い・前に無い `base` は、API を呼ぶ前に止まる(`checkCasesFile`)。

## 4. 結果の置き場

| 物 | 置き場 | 名前 |
|---|---|---|
| 生の JSON(生成) | `-out`(既定 `../docs/backend/ai-eval/raw`) | `<日付>-<model>-<effort>[-<tag>].json` |
| 生の JSON(対話) | 同じ | `<日付>-chat-<model>-<effort>[-<tag>].json` |
| まとめ | このフォルダ | `<日付>-<内容>.md`(人が書く) |

- 同じ日・同じ model / effort / tag で回すと上書きする。比べる時は `-tag` を変える(`before` / `after` など)。
- 生の JSON にはケースごとの `ok`・`claudeMs`・`usage`・`expectMisses`・`dsl` が入る(`Record`)。
- 手元で試しただけの結果は commit しない(推測: `raw/` に置くのは文書から参照する結果だけ、という今までのやり方に合わせる)。

## 5. -compare の読み方

```sh
go run ./cmd/ai-eval -compare-before '../docs/backend/ai-eval/raw/*-before*.json' -compare-after '../docs/backend/ai-eval/raw/*-after*.json'
```

- API を呼ばない。glob に合うファイルを全部読み、同じ `id` の記録をまとめる(`-repeat` の複数回も 1 行になる)。
- 1 つ目の表はケースごと。

| 列 | 意味 |
|---|---|
| runs (before/after) | その id の記録の数 |
| 一発通過 | 作り直し 0 回・area のやり直し無しで通った数 / 成功した数 |
| Claude ms 中央値 | Claude の呼び出しにかかった時間の中央値と増減(%) |
| 出力 tokens 中央値 | 出力 tokens の中央値と増減(%) |
| first event ms (after) | `-stream` の時だけ。最初のイベントまでの時間 |

- 2 つ目の表は全体: 一発通過・create の p50 / p95・edit の p50・平均出力 tokens・area 付与率・expect miss の件数。
- 増減は before を 0 とした割合。マイナスが速い・少ない。1 回ずつの比較は揺れるので、`-repeat` で数を増やしてから判断する。

## 6. フラグ

名前順。既定は `main.go` の `flag.*` の値。

| フラグ | 既定 | 意味 |
|---|---|---|
| `-capabilities` | 空 | `-chat` で、`mode` / `capabilities` を持たないケースに送る能力(`answer,proposal`) |
| `-cases` | `internal/ai/eval/cases.json` | ケースのファイル。`-chat` で指定しない時は `chat_cases.json` |
| `-chat` | false | 対話(聞き返し)のフェーズを `Service.Chat` で回す |
| `-chat-effort` | 空(`ai.DefaultChatEffort`) | `-chat` の effort |
| `-chat-model` | 空(`ai.DefaultChatModel`) | `-chat` のモデル(`AI_CHAT_MODEL` と同じ) |
| `-chat-thinking` | 空(`ai.DefaultChatThinking`) | `-chat` の thinking(`adaptive` / `disabled` / `between_tools`) |
| `-check` | false | `-cases` のファイルの形だけを確かめて終わる(API なし) |
| `-check-all` | 空 | このフォルダの JSON を全部、形に合う読み方で確かめる(API なし) |
| `-compare-after` | 空 | 比べる「後」の生 JSON の glob |
| `-compare-before` | 空 | 比べる「前」の生 JSON の glob。`-compare-after` と組で表を出して終わる(API なし) |
| `-effort` | `low` | `output_config.effort` |
| `-env-file` | 空 | 先に読む `KEY=VALUE` のファイル(例 `../.env`) |
| `-locale` | 空 | 全ケースを `ja` / `en` にする |
| `-logic-pass` | false | 作成の後に計算だけの 2 回目の呼び出しをする(`AI_LOGIC_PASS=1` と同じ) |
| `-min-pass` | 0 | 全体の pass% がこれ未満なら終了コード 1([ci-gate.md](ci-gate.md)) |
| `-model` | `claude-sonnet-5-5` | 生成のモデル |
| `-on-demand-rules` | true | レシピをキーワードか選ぶ呼び出しで選んだ時だけ送る(`cmd/ai` と同じ。false = `AI_ON_DEMAND_RULES=0`) |
| `-only` | 空 | 回すケースの id(カンマ区切り) |
| `-out` | `../docs/backend/ai-eval/raw` | 生の JSON を書くフォルダ |
| `-recipe-model` | 空(`ai.DefaultRecipeModel`) | レシピを選ぶ呼び出しのモデル(`AI_RECIPE_MODEL` と同じ) |
| `-repeat` | 1 | 選んだケースをこの回数ずつ回す(p50 / p95 は全回数で出す) |
| `-report` | 空 | `md` で、グループごとの表と通らなかったケースを Markdown で出す |
| `-report-out` | 空 | `-report md` の書き先(空 = 標準出力) |
| `-score` | 空 | 保存した生 JSON の glob。グループごとの pass% を出して終わる(API なし) |
| `-server-rules` | false | 自動の動きを頼む発話で serverRules の呼び出しもする(`AI_SERVER_RULES=1` と同じ) |
| `-stream` | false | `GenerateStream`(SSE)を使い、最初のイベントまでの時間を記録する |
| `-tag` | 空 | 出力ファイル名の末尾 |
| `-thresholds` | 空 | グループごとの基準の JSON(`{"group": 0.9, "total": 0.8}`) |
| `-timeout` | `60s` | Claude 1 回の呼び出しの上限 |

表とコードのずれは `grep -n 'flag\.' backend/cmd/ai-eval/main.go` で見つける。

## 7. 費用

1 回の費用の目安はここに書かない(モデル・ケースの数・プロンプトで変わるため)。実行の後の生 JSON の `usage`(ケースごとの tokens)を見る。単価と見積もりの出し方は [ai-cost.md の費用の見積もり](../../infra/ai-cost.md#費用の見積もり)。件数を絞る時は `-only` と `-cases` を使う。

## 8. 関係する文書

- [ci-gate.md](ci-gate.md) — `-min-pass` / `-thresholds` / `-report`
- [expect-keys.md](expect-keys.md) — `expect` のキーの一覧
- [m0disc-regression.md](m0disc-regression.md) — M0 のケースの回し方と期待値
- [../architecture.md](../architecture.md) — 生成の流れ
