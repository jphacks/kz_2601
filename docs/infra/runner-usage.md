# runner の使った量を見る(#345)

runner(自動の動き・[runner.md](runner.md))は 1 回の実行ごとに 1 行の構造化ログ `runner.usage` を出します(`backend/internal/runner/quota/usage_log.go`)。これを [ai-cost.md](ai-cost.md)(#298)と同じ方法で、アプリごと・作成者ごと・日 / 月ごとに見ます([#338 の決定](../decisions/338-server-feature-cost-model.md))。

> **見るだけです。** 止めるのは runner の上限(`RUNNER_LIMIT_*`・[runtime-config.md](runtime-config.md#運用で変える値))で、日本時間の 1 日ごとに数えます。月の上限はかけていません(月ごとの集計は眺めて上限の値を直すためのもの)。
>
> runner は #288 の分離まで **prod だけ**で動きます。stg を見ても空です。

## 目次
- [すぐ見る: scripts/runner-usage.sh](#すぐ見る-scriptsrunner-usagesh)
- [ログの形(runner.usage)](#ログの形runnerusage)
- [わかっている限界](#わかっている限界)

## すぐ見る: scripts/runner-usage.sh

Cloud Logging を `gcloud logging read` で読んで集計します。GCP の設定は何も変えません(要る権限は `roles/logging.viewer` だけ)。ログが残っている期間(既定 30 日)なら見られます。

```bash
scripts/runner-usage.sh prod                    # 昨日(日本時間)と直近 7 日
scripts/runner-usage.sh prod --days 30          # 月ごとの合計と、期間の上位のアプリ・作成者
scripts/runner-usage.sh prod --day 2026-11-01   # 見る日を指定
scripts/runner-usage.sh prod --threshold 500    # 1 アプリ 1 日の実行回数の閾値。超えたら終了コード 3
scripts/runner-usage.sh prod --dry-run          # フィルタだけ出す(Logs Explorer に貼れる)
```

出るもの(Markdown の表):

| 節 | 中身 |
| --- | --- |
| 1 日の合計 | 見る日(既定は昨日・日本時間)の実行(error を含む)・止めた回数・書き込み・通知の宛先・読み取り・上限で落とした分・アプリ数・作成者数・見積もり USD |
| 止めた・削った理由 | `outcome` と `limit`(`app_exec`・`app_writes`・`needs_pro` …)ごとの回数 |
| 上位のアプリ / 作成者 | 見る日の `app_id` / `creator_uid_hash` ごとの実行・書き込み・通知の宛先(実行の多い順、既定 10、`TOP=20` で変更) |
| 日ごと | 期間の日ごとの合計 |
| 月ごと | 日本時間の暦月ごとの合計。期間の途中から始まる月には印 |
| 期間の上位のアプリ / 作成者 | 期間全体の上位(`--days 30` で月の上位) |
| 閾値 | 見る日に実行回数の最も多いアプリが閾値を超えたか(既定 2,000 = Pro の 1 アプリ 1 日の上限。`RUNNER_USAGE_APP_EXEC_THRESHOLD` でも変えられる) |

数え方:
- **実行** = `outcome` が `ok` か `error`(上限が実行として数えるもの)。`skipped_*` は「止めた」に数える。
- **USD** は `est_cost_usd`(#338 の単価 × 数。`PricingAsOf` の日付の見積もり)。請求額ではない。
- 集計の確かめ: `scripts/runner-usage-test.sh`(固定の入力 `scripts/testdata/runner-usage/` で期待値と比べる。jq だけで動く。scripts-ci でも回る)。

## ログの形(runner.usage)

`jsonPayload` のうち集計に使うもの:

| フィールド | 中身 |
| --- | --- |
| `event` | `runner.usage` |
| `app_id` / `rule_id` / `trigger` | どのアプリのどのルールが、何で動いたか(`record_created` / `record_updated` / `schedule`) |
| `outcome` | `ok` / `error` / `skipped_quota` / `skipped_global` / `skipped_paused` / `skipped_plan` / 月の枠(#630)の `skipped_allowance`(枠を使い切り、超えた後のコイン払いが OFF)・`skipped_overflow_cap`(コインの月の上限)・`skipped_coins`(コインが足りない) など |
| `limit` | 止めた・削った上限の名前(あれば)。月の枠は `allowance_runs` / `allowance_recipients` / `allowance_writes` / `allowance_reads` |
| `reads` / `writes` / `notify_recipients` / `tasks_ops` / `cpu_ms` | 使った量 |
| `writes_dropped` / `recipients_dropped` | 上限で落とした分(あれば) |
| `creator_uid_hash` / `actor_uid_hash` | 作成者・動かした人の擬名(`ai.usage` の `uid_hash` と同じ鍵)。**生の uid は出さない** |
| `est_cost_usd` / `pricing_as_of` | 見積もり費用と単価の日付 |

値・通知の本文・生の uid は出しません。

## わかっている限界

- `gcloud logging read` の `LIMIT`(既定 50,000 行)を超えると少なく出ます(警告が出る)。その時は `LIMIT` を上げるか日数を減らす。
- Cloud Logging の保存は 30 日なので、それより前の月は出ません。長く見るなら [ai-cost.md の BigQuery へのシンク](ai-cost.md#人の作業-1-bigquery-へのシンク)と同じ形で `runner.usage` も流す(人の作業・保存期間は #297 の決定に従う)。
- ログベースの指標とダッシュボードの図はまだ作っていません(`infra/monitoring.tf` は触っていない)。
