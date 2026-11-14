# ai-audit の使い方(#287)

`backend/cmd/ai-audit` は、`cmd/ai-eval` が書いた生の JSON を読み、作られた DSL が**動くか**(押しても何も起きないボタン・行けない画面・入れたものが見えない entity など)と、発話に要る機能(`needs`)がそろっているかを数える。API も stg も呼ばない。手で読んで採点した結果は `2026-10-24-audit*.md` にある。

## 目次

1. [回し方](#1-回し方)
2. [入力](#2-入力)
3. [判定](#3-判定)
4. [出力の読み方](#4-出力の読み方)
5. [できないこと](#5-できないこと)

## 1. 回し方

`backend/` で打つ(`-cases` の既定が `backend/` からの相対)。repo にある結果だけで回せる。

```sh
cd backend
go run ./cmd/ai-audit -run ../docs/backend/ai-eval/raw/2026-10-24-claude-sonnet-5-5-low-audit3-before.json
# 2 つの実行を並べる(右の列が -compare の ○/△/×)
go run ./cmd/ai-audit -run ../docs/backend/ai-eval/raw/2026-10-24-claude-sonnet-5-5-low-audit3-after.json \
  -compare ../docs/backend/ai-eval/raw/2026-10-24-claude-sonnet-5-5-low-audit3-before.json
# 英語のケース
go run ./cmd/ai-audit -cases internal/ai/eval/audit_en_cases.json -run ../docs/backend/ai-eval/raw/2026-10-24-claude-sonnet-5-5-low-audit3-en-before.json
```

新しく測る時は、先に ai-eval で同じケースのファイルを回して生の JSON を作る(こちらは Claude API を呼ぶ。回し方は `backend/cmd/ai-eval/main.go` の先頭のコメント)。

```sh
go run ./cmd/ai-eval -env-file ../.env -cases internal/ai/eval/audit_cases.json -tag audit-local
go run ./cmd/ai-audit -run ../docs/backend/ai-eval/raw/<日付>-claude-sonnet-5-5-low-audit-local.json
```

| フラグ | 既定 | 意味 |
|---|---|---|
| `-cases` | `internal/ai/eval/audit_cases.json` | `needs` を持つケースのファイル |
| `-compare` | 空 | 並べるもう 1 つの生の JSON。表に「比較 run ○/△/×」の列が足される |
| `-run` | (必須) | ai-eval の生の JSON |

## 2. 入力

- `-run`: ai-eval の生の JSON(`raw/*.json`)。読むのは `model`・`tag` と、ケースごとの `id`・`ok`・`error`・`dsl` だけ。
- `-cases`: ケースの `id`・`transcript`・`needs`。`needs` は「この発話に要る機能」の名前の配列で、ai-eval は読まない。
  - 日本語: `internal/ai/eval/audit_cases.json`、英語: `internal/ai/eval/audit_en_cases.json`
- 表に出るのは `-cases` のケースだけ。`-run` にあって `-cases` に無い id は出ない。`-cases` にあって `-run` に無い id は「生成 FAIL」(理由は空)と出る。ケースを足した後に古い結果を読むと、足した分が FAIL になる。
- DSL は `map[string]any` で緩く読む。スキーマが違うブランチの結果も読める。配列の中のオブジェクトでない物は `malformed` の lint として数える。

## 3. 判定

実装は `backend/cmd/ai-audit/main.go`。

### lint(すべてのケース)

DSL の機械的な穴。種別は出力の「lint 種別の件数」の名前。

| 種別 | 条件 |
|---|---|
| `bad_nav` | navigate の先の画面が無い |
| `dead_button` | action に `type` が無い |
| `field_no_input` | フォームに入力欄の無い field(computed・date・datetime は除く。常に空) |
| `form_unreachable` | entity のフォームにどこからも行けない |
| `label_filter_mismatch` | stat / progressBar / gauge / chart / list のラベルが今月・今日・今週・今年・直近 7 日と言うのに、それを表す filter の op(`thisMonth` / `today` / `thisWeek` / `thisYear` / `gte`)が無い。今年は式の `year` でもよい。直近 7 日は `doneInDays` を使っていれば見ない |
| `label_promises_calc` | stat のラベルが残り・あたり・順位・連続・BMI・精算などと言うのに `value` が無い(単純な集計だけ) |
| `malformed` | entities / fields / screens / components / children / toolbar / tabs の中にオブジェクトでない物がある・`dsl` が null |
| `no_add_form` | entity を足すフォームの画面(`screen.entity`)が無い |
| `no_list` | entity を一覧する部品(list・table・timeline・calendar・mapView・settlement・gridTable・chatThread・aiChat)が無い |
| `no_save` | フォームの画面に saveRecord が無い |
| `required_no_input` | 必須の field にフォームの入力欄が無い(保存できない) |
| `static_value_text` | text / label / markdown の固定の文が、計算した値に見える語と数字を含む |
| `toggle_invisible` | toggle の field が、どの部品の filter にも表示(`item`)にも使われない |
| `unreachable` | `initialScreen`・タブ・navigate・saveRecord の `screen` のどこからも行けない画面 |

### needs(`-cases` に書いた分だけ)

`need()` の switch で、部品の `type`・filter の op・式(Expr)の op・`computed` の有無を見て ○ / △ / × を付ける。

| need | ○ の条件(△ があるものは △ も) |
|---|---|
| `agg` | stat / progressBar / gauge / chart がある |
| `bulk_delete` | いつも ×(条件で一括で消す action が DSL に無い) |
| `calc` | computed か stat の `value` がある |
| `chart` | chart がある |
| `date_add` / `days_until` | 式で `daysUntil` / `addDays` / `daysBetween` を使う |
| `filter_bool` | boolean の field で絞った部品がある |
| `filter_due` | 式で `daysUntil` / `addDays` / `today` / `lte` を使う |
| `filter_month` / `filter_today` | filter の op `thisMonth` / `today` がある |
| `filter_week` / `filter_year` | filter の op `thisWeek` / `thisYear`(今年は式の `year` も)。△ = 日付の関数(`year` / `daysBetween` / `weekday` / `addDays`)で近づけている |
| `group_by` / `group_by_month` | ○ は無い。△ = chart の `groupBy` / `bucket` |
| `group_by_member` | computed の agg。△ = chart だけ |
| `notify` | `app.notifications` か scheduleNotification の action |
| `pb_detect` | computed の agg。△ = `aggregate: max` だけ |
| `progress` | progressBar / gauge |
| `rank` | いつも ×(順位を出す手段が DSL に無い) |
| `ref` | type `ref` の field |
| `remaining` | stat の `value`。△ = progressBar / gauge だけ |
| `settlement` | settlement の部品 |
| `sort_time` | `sort` を持つ list |
| `streak` | 式の `streak`。△ = `doneInDays` だけ |
| `tabs3` | タブが 3 つ以上 |
| `time_diff` | 式で `hoursBetween` を使う |
| `timer` | timerDisplay |
| `timer_cycle` | いつも ×(集中 → 休憩の自動の切り替えが DSL に無い) |
| `toggle` | `list.item.toggle`(一覧から押せる)。△ = フォームの toggle だけ |
| `two_entities` | entity が 2 つ以上 |

上の表に無い名前は `?`(未定義の need)と出て、× と同じに数える。

## 4. 出力の読み方

Markdown を標準出力に出す。結果の文書に貼る時は `> ファイル` で受ける。

1. **機械判定** — ケースごとに 1 行。`生成`(OK / FAIL)・`needs ○/△/×` の数・欠けた needs(△ は `(△)` つき)・lint の件数。`-compare` があると右に比べる実行の ○/△/×。
2. **needs 別の充足** — need ごとの ○ / △ / × の数。× の多い順。
3. **lint 種別の件数** — 種別ごとの件数。多い順。
4. **ケース別の詳細** — 発話・need ごとの印と理由・lint の中身。生成が失敗したケースはエラーだけ。

数え方の目安: 1 ケースの充足は ○ の数 / needs の数。手で採点した充足(例 [2026-10-24-audit3.md](2026-10-24-audit3.md))とは一致しない(下の「できないこと」)。

## 5. できないこと

- 式が**正しいか**は見ない。「その種類の式があるか」までなので、最後は人が DSL を読んで確かめる([2026-10-24-audit.md の判定ルール](2026-10-24-audit.md#8-ai-audit-の判定ルール)。ただし、そこの表は当時のもので、今の種別は上の表)。
- 終了コードで赤にする仕組みは無い(基準で落とすのは ai-eval の [ci-gate.md](ci-gate.md))。
- 決めること: ai-audit の判定を CI に入れるか・`needs` の名前を `expect` と揃えるか。
