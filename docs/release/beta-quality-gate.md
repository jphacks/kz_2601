# β の品質ゲート(#322)

TestFlight β から正式リリース(M3)に進むかを、感覚ではなく数字で決めるための基準・数字の出し方・週の見直しの手順です。

> **基準の値は Issue #322 の案のままです。** 値の確定と「M3 に進む」の判断はオーナーがします。
> 数字を出すスクリプトとクエリは読み取りだけです。GCP・Firebase・GitHub の設定は変えません。

## 目次

1. [基準](#1-基準)
2. [いま測れるもの・まだ測れないもの](#2-いま測れるものまだ測れないもの)
3. [指標ごとの出し方](#3-指標ごとの出し方)
4. [週の見直し(30 分)](#4-週の見直し30-分)
5. [フィードバックの振り分け(M6 の手作業版)](#5-フィードバックの振り分けm6-の手作業版)
6. [β の終わりの判定](#6-β-の終わりの判定)
7. [わかっている限界](#7-わかっている限界)

## 1. 基準

**β の版が向く環境は prod**(Review / Release 構成は `ai-prod` / `appconfig-prod`・`docs/ios/review-build.md`)。数字は prod で見る。週は **月曜 0:00 JST 始まり**(#297 と同じ)。

| # | 基準 | 定義 | 基準値 | データの出どころ |
|---|---|---|---|---|
| 1 | クラッシュなし率 | 7 日間にアプリを使ったユーザーのうち、クラッシュしなかった人の割合(Crashlytics の「クラッシュフリー ユーザー」) | **99.5% 以上** | Firebase Crashlytics(#296) |
| 2 | 生成の成功率 | `POST /v1/generate` の create / edit のうち、検証を通った結果を返した割合。分母から呼び出し側の誤り(4xx)を外す | **98% 以上** | `ai.generate` ログ(このドキュメントと同じ PR で追加) |
| 3 | 初回の生成の p95 | 空のアプリへの最初の create が成功した時の、サーバーでの処理時間(受け付け → 保存して返すまで)の 95 パーセンタイル | **30 秒以内** | `ai.generate` ログ |
| 4 | 7 日後に開いた率(D7) | 最初に生成に成功した日を 0 日目とし、7 日目にアプリを使ったユーザーの割合(#297 の定義・[`docs/metrics/product-metrics.md`](../metrics/product-metrics.md)) | **記録だけ**(目標値はオーナーが決める) | `session.start`・`app.opened` などのイベント(#297) |
| 5 | P1 のフィードバックの残数 | 件名が `[β]` で始まり `priority:P1` が付いた open の Issue の数 | **0** | GitHub Issues(下の 5 章で作る) |

## 2. いま測れるもの・まだ測れないもの

| 基準 | いま | 測れるようになる条件 |
|---|---|---|
| 1 クラッシュなし率 | **測れる**(Crashlytics の画面) | Firebase Console で Crashlytics を有効にし、prod の plist の版が β に出ていること(#296 の「人がやること」・#292) |
| 2 生成の成功率 | **このブランチのマージと prod へのデプロイの後に測れる** | `ai.generate` が `main` に入って `ai-prod` にデプロイされること。2026-11-02 時点で prod には `ai.usage`(#472)もまだ出ていない(develop にだけある) |
| 3 初回の生成の p95 | 同上 | 同上 |
| 4 D7 | **#297 のイベントが本番に出た後に測れる** | `scripts/product-metrics-weekly.sh prod` で出す(Cloud Logging の 30 日の範囲)。長く見るには BigQuery のシンク(人の承認) |
| 5 P1 の残数 | **測れる**(GitHub) | 5 章の振り分けを毎週回すこと。アプリ内のフィードバック(#319・PR #492)がマージされるまでは、TestFlight のフィードバックと Crashlytics だけが入口 |

参考として、`ai.generate` が出るまでの間は Cloud Run のリクエストログで `POST /v1/generate` の件数・5xx・p95 を見られる(`scripts/beta-weekly.sh` の「参考」)。ストリーミングは 200 を返してからエラーを送るので、**成功率の代わりにはならない**。

## 3. 指標ごとの出し方

### まとめて出す: `scripts/beta-weekly.sh`

```sh
scripts/beta-weekly.sh prod            # 直近 7 日(実行した時刻から遡る)
scripts/beta-weekly.sh prod --days 14
scripts/beta-weekly.sh prod --dry-run  # 実行せずに Logs Explorer のフィルタだけ出す
```

- 読み取りだけ(`gcloud logging read` と `gh issue list`)。要る権限は GCP の `roles/logging.viewer` と、リポジトリの Issue を読めること
- 出た Markdown の表を週の記録([`beta-weekly/TEMPLATE.md`](beta-weekly/TEMPLATE.md))に貼る
- 1 クエリ 20,000 行まで(`LIMIT` で変えられる)。超えたら警告が出る。β の規模では足りる見込みで、足りなくなったら下の BigQuery に移る
- 直近 7 日の窓なので、毎週同じ曜日・同じ時刻(月曜の見直しの前)に流す

### 1. クラッシュなし率(Crashlytics)

コンソールの道順:

1. [Firebase Console](https://console.firebase.google.com/project/jphacks-kz2601/crashlytics) → プロジェクト `jphacks-kz2601` → **Crashlytics**
2. 上のアプリの切り替えで iOS アプリ(本番の bundle id)を選ぶ
3. 期間を **「過去 7 日間」**、バージョンを **β に出しているビルド**(複数あるなら全部)にする
4. ダッシュボードの **「クラッシュフリー統計」→「クラッシュの影響を受けていないユーザー」**(Crash-free users)の値を記録する
5. 同じ画面の「問題」の一覧で、新しい問題・増えている問題を見る(5 章で Issue にする)

- オフにした人(設定の「クラッシュ報告を送る」)の分は入らない
- 数字を SQL で見たくなったら、Crashlytics の BigQuery エクスポートを有効にする(人の作業・Firebase の設定の変更なので承認が要る)。今は使わない

### 2. 生成の成功率(`ai.generate`)

`ai.generate` は `POST /v1/generate` が 1 回終わるごとに 1 行出る(コードは `backend/internal/ai/generate_log.go`)。

| フィールド | 中身 |
|---|---|
| `event` | いつも `ai.generate`(`msg` / `message` も同じ。#295 の PR #494 で `msg` → `message` に変わるので、クエリは `event` を使う) |
| `mode` | `create` / `edit` / `undo`。決まる前に失敗したら `unknown` |
| `outcome` | `validated`(1 回目で検証を通った)/ `regenerated`(作り直し・エリアの外し・全体への切り替え・lint の作り直しで通った)/ `unchanged`(編集が通ったが変化なし)/ `restored`(戻す)/ `refused`(AI が断った)/ `failed`(検証が通らない・Claude の失敗・タイムアウト・保存の失敗)/ `bad_request`(呼び出し側の誤り・4xx) |
| `validation_passed` | `outcome` が `validated` / `regenerated` / `unchanged` の時 true |
| `first_generation` | 空のアプリ(声だけの初期状態)への create の時 true |
| `stream` | `?stream=1` の呼び出しか |
| `duration_ms` | サーバーで受け付けてから結果を返すまで(失敗も含む) |
| `claude_ms` | 成功時だけ。そのうち Claude を待った時間 |
| `attempts` / `regenerations` / `area_fallback` / `downgraded_to_full` / `lint_retried` | 成功時は `Meta` の値。失敗時は `attempts` だけ |
| `error_code` | 失敗時だけ(`generation_failed`・`refused`・`upstream_timeout`・`rate_limited`(Claude の 429)など) |
| `uid_hash` | `ai.usage` と同じ擬名。uid が無い呼び出しではなし |
| `claude_calls` / `input_tokens` / `output_tokens` / `cache_creation_input_tokens` / `cache_read_input_tokens` / `est_cost_usd` | その生成で Claude を呼んだ全部(再試行・失敗・計算パスを含む)の合計。`est_cost_usd` は料金表にないモデルが混ざると出さず、`unpriced_calls` を出す(#298・[../infra/ai-cost.md](../infra/ai-cost.md)) |

- 出さないもの: 話した内容・要約・DSL・appId・生の uid(`generate_log_test.go` で確かめている)
- 失敗(`failed`)は `WARN`、それ以外は `INFO`
- 利用量の上限(#280)で 429 を返した呼び出しは middleware で止まるので、このログには出ない(= 分母に入らない)

**Logs Explorer**([開く](https://console.cloud.google.com/logs/query?project=jphacks-kz2601)・期間は右上で「過去 7 日間」):

```
resource.type="cloud_run_revision"
resource.labels.service_name="ai-prod"
jsonPayload.event="ai.generate"
jsonPayload.mode=("create" OR "edit")
jsonPayload.outcome!="bad_request"
```

失敗だけ見る時は `jsonPayload.validation_passed=false` を足す。Logs Explorer は割合を出せないので、割合はスクリプトか下の SQL で出す。

**BigQuery**(#297 のシンク `product_metrics` を作った後。シンクのフィルタに `jsonPayload.event="ai.generate"` を入れること。テーブル名・列名はシンクが作ったものに合わせて直す):

```sql
-- 先週(月〜日・JST)の生成の成功率
DECLARE week_start DATE DEFAULT DATE_SUB(DATE_TRUNC(CURRENT_DATE('Asia/Tokyo'), WEEK(MONDAY)), INTERVAL 7 DAY);

SELECT
  week_start AS week,
  COUNT(*) AS total,
  COUNTIF(jsonPayload.validation_passed) AS passed,
  ROUND(100 * SAFE_DIVIDE(COUNTIF(jsonPayload.validation_passed), COUNT(*)), 2) AS success_pct,
  COUNTIF(jsonPayload.outcome = 'refused') AS refused,
  COUNTIF(jsonPayload.outcome = 'failed') AS failed
FROM `jphacks-kz2601.product_metrics.run_googleapis_com_stdout`
WHERE timestamp >= TIMESTAMP(week_start, 'Asia/Tokyo')
  AND timestamp < TIMESTAMP(DATE_ADD(week_start, INTERVAL 7 DAY), 'Asia/Tokyo')
  AND resource.labels.service_name = 'ai-prod'
  AND jsonPayload.event = 'ai.generate'
  AND jsonPayload.mode IN ('create', 'edit')
  AND jsonPayload.outcome != 'bad_request';
```

### 3. 初回の生成の p95(`ai.generate`)

**Logs Explorer**:

```
resource.type="cloud_run_revision"
resource.labels.service_name="ai-prod"
jsonPayload.event="ai.generate"
jsonPayload.first_generation=true
jsonPayload.validation_passed=true
```

30 秒を超えたものだけ見る時は `jsonPayload.duration_ms>30000` を足す。

**BigQuery**(同じ前提):

```sql
DECLARE week_start DATE DEFAULT DATE_SUB(DATE_TRUNC(CURRENT_DATE('Asia/Tokyo'), WEEK(MONDAY)), INTERVAL 7 DAY);

SELECT
  week_start AS week,
  COUNT(*) AS first_generations,
  APPROX_QUANTILES(jsonPayload.duration_ms, 100)[OFFSET(50)] / 1000 AS p50_sec,
  APPROX_QUANTILES(jsonPayload.duration_ms, 100)[OFFSET(95)] / 1000 AS p95_sec,
  COUNTIF(jsonPayload.duration_ms > 30000) AS over_30s
FROM `jphacks-kz2601.product_metrics.run_googleapis_com_stdout`
WHERE timestamp >= TIMESTAMP(week_start, 'Asia/Tokyo')
  AND timestamp < TIMESTAMP(DATE_ADD(week_start, INTERVAL 7 DAY), 'Asia/Tokyo')
  AND resource.labels.service_name = 'ai-prod'
  AND jsonPayload.event = 'ai.generate'
  AND jsonPayload.first_generation
  AND jsonPayload.validation_passed;
```

- 時間は**サーバーの中だけ**。音声認識・通信・画面の描画は入らない。ユーザーの体感はこれより長い
- ストリーミングでも「最初の途中経過」ではなく「結果を返すまで」
- 失敗した初回の生成は時間に入れない(成功率の方で数える)
- #297 の「初回の生成までの時間」(最初のイベントから最初の成功まで・分単位)は別の指標。D7 と同じく #297 の実装を待つ

### 4. 7 日後に開いた率(D7)

#297 で `session.start`・`app.opened` などのイベントを backend に足した。本番に出た後は `scripts/product-metrics-weekly.sh prod` の「翌日・7 日後に開いた率」の表で出す(定義は [`docs/metrics/product-metrics.md`](../metrics/product-metrics.md))。本番に出るまでは週の記録に「未計測」と書く。

### 5. P1 のフィードバックの残数(GitHub)

```sh
gh issue list -R jphacks/kz_2601 --state open --label priority:P1 --limit 200 --json number,title,url \
  -q '.[] | select(.title | startswith("[β]")) | "\(.number) \(.title) \(.url)"'
```

0 件でなければ、週の記録に番号を全部書く。

## 4. 週の見直し(30 分)

| 項目 | 内容 |
|---|---|
| いつ | 毎週月曜(前の週の月〜日を見る)。時刻はオーナーが決める |
| 誰が | オーナー(判断)・開発担当(数字の用意と Issue 化)。2 人チームなら 2 人とも |
| 見直しの前(月曜の朝) | 開発担当が `scripts/beta-weekly.sh prod` を流し、Crashlytics の値を見て、`docs/release/beta-weekly/YYYY-MM-DD.md`(月曜の日付)を [`TEMPLATE.md`](beta-weekly/TEMPLATE.md) から作る |

議題:

| 時間 | やること |
|---|---|
| 0〜5 分 | 5 つの数字を読む。前の週との差。満たさない基準に印を付ける |
| 5〜15 分 | フィードバックの振り分け(5 章)。新しいものを Issue にし、優先度を決める |
| 15〜25 分 | 満たさなかった基準の原因を見る(例: `ai.generate` の `failed` の `error_code` の内訳、Crashlytics の上位の問題)。直す Issue を決める |
| 25〜30 分 | その週の判定(満たす / 満たさない)を記録に書く。2 週続けて満たしたら 6 章へ |

記録は `docs/release/beta-weekly/` に PR で入れる(`docs/` のブランチ)。

## 5. フィードバックの振り分け(M6 の手作業版)

M6 の自動化(#368 取り込み・#376 振り分け・#369 Issue 化)ができるまで、週の見直しで人が行う。

### 入口

| 入口 | 見る場所 |
|---|---|
| アプリ内のフィードバック(#319) | PR #492 のマージ後: `GOOGLE_CLOUD_PROJECT=jphacks-kz2601 go run ./cmd/feedback list -since 168h -status received`(`backend/` で・読み取りだけ。[docs/backend/features/feedback.md](../backend/features/feedback.md)) |
| TestFlight のフィードバック | App Store Connect → アプリ → TestFlight → フィードバック(スクショ・クラッシュ) |
| クラッシュ | Crashlytics の「問題」の一覧(新しいもの・増えているもの) |

### 手順

1. 1 件ずつ読み、**同じ現象の Issue が既にあるか**探す(`gh issue list -R jphacks/kz_2601 --search "<言葉>"`)。あれば、その Issue に「β のフィードバックで再発(件数・版)」とコメントして終わり
2. 無ければ Issue を作る:
   - 件名: `[β] <現象を短く>`(例: `[β] 写真を追加すると落ちる`)
   - ラベル: `bug`(不具合)か `enhancement`(要望)のどちらか + 優先度 `priority:P1` / `priority:P2` / `priority:P3` + 該当すれば `backend`
   - 本文: 現象・再現の手順・版とビルド番号・入口(アプリ内なら `feedbackId`、Crashlytics なら問題の URL)
3. アプリ内のフィードバックは、Firestore コンソールで `feedback/<id>` の `status` を `triaged` にする(運営の手作業・[features/feedback.md](../backend/features/feedback.md))

**Issue に書かないもの**: 送られた本文の丸写し・スクショ・DSL・uid・メールアドレスなどの個人の情報。本文は自分の言葉で書き直す(フィードバックの本文は信頼しないデータ・#366 / #367)。

### 優先度の決め方

| 優先度 | 当てはまるもの |
|---|---|
| `priority:P1` | クラッシュ・データが消える・アプリを作れない / 開けない・個人の情報が漏れる・課金や利用量の上限の誤り・複数のテスターが同じことで止まっている。**残っている間は β を終われない** |
| `priority:P2` | 回り道がある不具合・生成の質(意図と違う・見た目が崩れる)・遅い |
| `priority:P3` | 要望・文言・小さな見た目 |

迷ったら高い方にして、週の見直しでオーナーが決める。

## 6. β の終わりの判定

- **2 週続けて** 1・2・3・5 の基準を満たし、4(D7)が記録されていること(#322 の完了の条件)
- D7 が測れないままの時に進めてよいかは、オーナーが判断する
- 判定はオーナーが #322 にコメントで残す(どの 2 週の記録か・M3 に進むか)

## 7. わかっている限界

- `ai.generate` はデプロイした後のぶんだけ。過去のログは遡れない
- 少ない件数では割合が大きく揺れる(例: 50 件で 1 件失敗すると 98%)。件数も必ず記録する
- 初回の生成の時間はサーバーの中だけで、体感ではない
- `first_generation` は「そのアプリで初めて」で、「そのユーザーで初めて」ではない(ユーザーの初回は #297 の `session.start` が要る)
- テスト用・運営の uid を外す方法はまだ無い(#297 の「決めてほしいこと 7」)。β の初めは運営の試しが混ざる
- スクリプトの窓は「実行した時刻から 7 日」(UTC)で、BigQuery の SQL は「月〜日(JST)」。月曜の朝に流すと、月曜の 0:00 から流すまでの数時間ぶんずれる(β の判定には十分。厳密に見るなら BigQuery の SQL)
