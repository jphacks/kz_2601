# AI 費用の記録と見える化(#298)

ai サービスが Claude を呼ぶたびに、1 行の構造化ログ `ai.usage` を出します。これを BigQuery に流して、日ごと・ユーザーごとの費用と上位のユーザーを出します。1 日の費用が閾値を超えたら知らせます。

> **ログを出すコードは入っています。BigQuery のシンク・アラートの手順はまだ実行していません。** GCP の設定は承認を得てから、人が行います(下の「人の作業」)。

## 目次
- [ログの形(ai.usage)](#ログの形aiusage)
- [費用の見積もり](#費用の見積もり)
- [人の作業 1: BigQuery へのシンク](#人の作業-1-bigquery-へのシンク)
- [集計の SQL](#集計の-sql)
- [人の作業 2: 1 日の費用のアラート](#人の作業-2-1-日の費用のアラート)
- [わかっている限界](#わかっている限界)

## ログの形(ai.usage)

コードは `backend/internal/ai/usage_log.go`。`ai.Client.Complete` が **HTTP の試行 1 回ごと** に 1 行出します(再試行も 1 行ずつ。課金は試行ごとに起きるため)。ai サービスでは `httpx.NewLogger()`(slog の JSON、標準出力)に出すので、Cloud Logging では `jsonPayload` になります。

| フィールド | 型 | 中身 |
| --- | --- | --- |
| `msg` / `event` | string | いつも `ai.usage` |
| `operation` | string | `generate`(新規作成)/ `patch`(編集)/ `logic`(計算パス)/ `chat`(聞き返し)/ `chat_first`(最初の対話)/ `text`(aiButton の文章生成)/ `unknown`(ラベルなし。eval などのツール) |
| `model` | string | 頼んだモデル id |
| `response_model` | string | 実際に答えたモデル(フォールバックで変わることがある)。応答がないときはなし |
| `stream` | bool | ストリーミングの呼び出しか |
| `attempt` | int | 1 から数える試行番号(2 以上は再試行) |
| `outcome` | string | `ok` / `refusal`(モデルが断った)/ `error` |
| `error_kind` | string | `outcome=error` のときだけ。`<HTTP status>_<エラー型>`(例 `429_rate_limit_error`、`599_connection_error`、`500_stream_overloaded_error`)か `deadline_exceeded` / `canceled` |
| `stop_reason` | string | `end_turn` / `max_tokens` / `refusal` など |
| `input_tokens` / `output_tokens` | int | 課金トークン |
| `cache_creation_input_tokens` / `cache_read_input_tokens` | int | キャッシュの書き込み・読み出し |
| `latency_ms` | int | その試行の時間 |
| `uid_hash` | string | 呼び出したユーザーの擬名。`sha256("ai-usage:" + uid)` の先頭 8 バイト(16 桁)。未ログイン(uid なし)のときはなし |
| `est_cost_usd` | float | 見積もり費用(USD)。**料金表にないモデルではなし**(間違った値を出さない) |
| `pricing_as_of` | string | 料金表を確かめた日。`est_cost_usd` があるときだけ |

**出さないもの**: プロンプト・発話・応答の本文、生の uid、API キー。テスト(`usage_log_test.go`)で、本文と生の uid が出ないことを確かめています。

例:

```json
{"time":"2026-10-30T12:00:00+09:00","level":"INFO","msg":"ai.usage","event":"ai.usage","operation":"patch","model":"claude-sonnet-5-5","stream":true,"attempt":1,"outcome":"ok","input_tokens":1000,"output_tokens":500,"cache_creation_input_tokens":2000,"cache_read_input_tokens":4000,"latency_ms":6400,"response_model":"claude-sonnet-5-5","stop_reason":"end_turn","uid_hash":"3f1c0e9a7b2d4c51","est_cost_usd":0.0124,"pricing_as_of":"2026-10-30"}
```

## 費用の見積もり

料金表は `usage_log.go` の `modelPrices` 1 か所だけです。値は [Anthropic の料金ページ](https://platform.claude.com/docs/en/about-claude/pricing) を 2026-10-30 に見て書きました(USD / 100 万トークン)。

| モデル | 入力 | 5 分キャッシュ書き込み | キャッシュ読み出し | 出力 |
| --- | --- | --- | --- | --- |
| `claude-sonnet-5-5`(ai の既定・対話の既定) | 2 | 2.50 | 0.10 | 10 |
| `claude-sonnet-5` | 2 | 2.50 | 0.20 | 10 |
| `claude-opus-5-5` | 4 | 5 | 0.20 | 20 |
| `claude-opus-5` | 5 | 6.25 | 0.50 | 25 |

- モデルを変える・足すときは、必ず料金ページで確かめてから表と `PricingAsOf` を直す
- `claude-haiku-5-5` はプロンプトの長さで値段が変わるので、わざと入れていない(費用はなしになる)
- クライアントは 5 分のキャッシュしか書かないので、書き込みは 5 分の料金で数える

## 人の作業 1: BigQuery へのシンク

> 承認を得てから実行する。まだ実行していない。

```bash
export PROJECT_ID="jphacks-kz2601"
export REGION="asia-northeast1"
export AI_SERVICE="ai-stg"   # 本番は本番の ai サービス名に替える

# 1) データセット(生のログは 90 日で消す。プライバシーポリシーの保存期間を先に 90 日に直してから作る・#297)
bq --location="$REGION" mk --dataset \
  --default_partition_expiration=7776000 \
  "${PROJECT_ID}:ai_usage"

# 2) ai.usage だけを流すシンク(日付パーティション)
gcloud logging sinks create ai-usage-bq \
  "bigquery.googleapis.com/projects/${PROJECT_ID}/datasets/ai_usage" \
  --use-partitioned-tables \
  --log-filter="resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"${AI_SERVICE}\" AND jsonPayload.event=\"ai.usage\""

# 3) シンクの書き込み用アカウントに、データセットへの書き込みを許す
WRITER="$(gcloud logging sinks describe ai-usage-bq --format='value(writerIdentity)')"
echo "$WRITER"   # serviceAccount:... を確かめる
bq add-iam-policy-binding \
  --member="$WRITER" --role="roles/bigquery.dataEditor" \
  "${PROJECT_ID}:ai_usage"
```

- テーブルは `ai_usage.run_googleapis_com_stdout` にできる(最初のログが流れてから)。過去のログは流れない(シンクを作った後のぶんだけ)
- stg と本番を分けて見たいなら、`AI_SERVICE` ごとにシンクを作るか、フィルタを `service_name=~"^ai"` にして SQL で `resource.labels.service_name` を見る

## 集計の SQL

日付は日本時間で切ります。`PROJECT` は置き換えてください。

### 昨日の費用・1 人あたりの中央値と上位 1%(完了の条件)

```sql
DECLARE d DATE DEFAULT DATE_SUB(CURRENT_DATE('Asia/Tokyo'), INTERVAL 1 DAY);

WITH calls AS (
  SELECT
    jsonPayload.uid_hash AS uid_hash,
    jsonPayload.est_cost_usd AS cost,
    jsonPayload.operation AS operation
  FROM `PROJECT.ai_usage.run_googleapis_com_stdout`
  WHERE timestamp >= TIMESTAMP(d, 'Asia/Tokyo')
    AND timestamp < TIMESTAMP(DATE_ADD(d, INTERVAL 1 DAY), 'Asia/Tokyo')
    AND jsonPayload.event = 'ai.usage'
),
per_user AS (
  SELECT uid_hash, SUM(cost) AS cost, COUNT(*) AS calls
  FROM calls
  WHERE uid_hash IS NOT NULL
  GROUP BY uid_hash
)
SELECT
  d AS day,
  (SELECT ROUND(SUM(cost), 4) FROM calls) AS total_usd,
  (SELECT ROUND(SUM(cost), 4) FROM calls WHERE uid_hash IS NULL) AS anonymous_usd,
  (SELECT COUNTIF(cost IS NULL) FROM calls) AS unpriced_calls,  -- 0 でなければ料金表に足りないモデルがある
  (SELECT COUNT(*) FROM per_user) AS users,
  (SELECT APPROX_QUANTILES(cost, 100)[OFFSET(50)] FROM per_user) AS median_user_usd,
  (SELECT APPROX_QUANTILES(cost, 100)[OFFSET(99)] FROM per_user) AS p99_user_usd;
```

### 昨日の上位 1% のユーザー

```sql
DECLARE d DATE DEFAULT DATE_SUB(CURRENT_DATE('Asia/Tokyo'), INTERVAL 1 DAY);

SELECT uid_hash, ROUND(SUM(jsonPayload.est_cost_usd), 4) AS cost_usd, COUNT(*) AS calls
FROM `PROJECT.ai_usage.run_googleapis_com_stdout`
WHERE timestamp >= TIMESTAMP(d, 'Asia/Tokyo')
  AND timestamp < TIMESTAMP(DATE_ADD(d, INTERVAL 1 DAY), 'Asia/Tokyo')
  AND jsonPayload.event = 'ai.usage'
  AND jsonPayload.uid_hash IS NOT NULL
GROUP BY uid_hash
QUALIFY PERCENT_RANK() OVER (ORDER BY cost_usd) >= 0.99
ORDER BY cost_usd DESC;
```

### 日ごと・処理ごと(直近 30 日)

```sql
SELECT
  DATE(timestamp, 'Asia/Tokyo') AS day,
  jsonPayload.operation AS operation,
  COUNT(*) AS calls,
  COUNTIF(jsonPayload.outcome = 'error') AS errors,
  ROUND(SUM(jsonPayload.est_cost_usd), 4) AS cost_usd
FROM `PROJECT.ai_usage.run_googleapis_com_stdout`
WHERE timestamp >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
  AND jsonPayload.event = 'ai.usage'
GROUP BY day, operation
ORDER BY day DESC, cost_usd DESC;
```

### 1 人あたりの月の費用の分布(Pro 価格 #325・上限 #317 の根拠)

```sql
WITH per_user_month AS (
  SELECT
    FORMAT_DATE('%Y-%m', DATE(timestamp, 'Asia/Tokyo')) AS month,
    jsonPayload.uid_hash AS uid_hash,
    SUM(jsonPayload.est_cost_usd) AS cost
  FROM `PROJECT.ai_usage.run_googleapis_com_stdout`
  WHERE jsonPayload.event = 'ai.usage' AND jsonPayload.uid_hash IS NOT NULL
  GROUP BY month, uid_hash
)
SELECT
  month,
  COUNT(*) AS users,
  APPROX_QUANTILES(cost, 100)[OFFSET(50)] AS p50_usd,
  APPROX_QUANTILES(cost, 100)[OFFSET(90)] AS p90_usd,
  APPROX_QUANTILES(cost, 100)[OFFSET(99)] AS p99_usd,
  MAX(cost) AS max_usd
FROM per_user_month
GROUP BY month
ORDER BY month DESC;
```

## 人の作業 2: 1 日の費用のアラート

> 承認を得てから実行する。まだ実行していない。閾値(例 20 USD/日)は決めてから入れる。

### ログベースの指標 + Cloud Monitoring のアラート

```bash
# 1) est_cost_usd を値にした分布の指標
cat > /tmp/ai-cost-metric.yaml <<'YAML'
name: ai_usage_cost_usd
description: Estimated Claude cost per call (ai.usage, #298)
filter: resource.type="cloud_run_revision" AND jsonPayload.event="ai.usage" AND jsonPayload.est_cost_usd>0
valueExtractor: EXTRACT(jsonPayload.est_cost_usd)
metricDescriptor:
  metricKind: DELTA
  valueType: DISTRIBUTION
  unit: "1"
  labels:
  - key: service
    valueType: STRING
labelExtractors:
  service: EXTRACT(resource.labels.service_name)
bucketOptions:
  exponentialBuckets: {numFiniteBuckets: 30, growthFactor: 2, scale: 0.0001}
YAML
gcloud logging metrics create ai_usage_cost_usd --config-from-file=/tmp/ai-cost-metric.yaml

# 2) 直近 24 時間の合計が閾値を超えたら通知(通知チャンネルは先に作っておく)
cat > /tmp/ai-cost-alert.json <<'JSON'
{
  "displayName": "AI cost over daily threshold (#298)",
  "combiner": "OR",
  "conditions": [{
    "displayName": "24h est_cost_usd > 20",
    "conditionPrometheusQueryLanguage": {
      "query": "sum(increase(logging_googleapis_com:user_ai_usage_cost_usd_sum[1d])) > 20",
      "duration": "0s",
      "evaluationInterval": "300s"
    }
  }],
  "notificationChannels": ["projects/jphacks-kz2601/notificationChannels/CHANNEL_ID"]
}
JSON
gcloud alpha monitoring policies create --policy-from-file=/tmp/ai-cost-alert.json
```

- **実行前に確かめること**: 分布の指標を PromQL で `_sum` として読む名前と、24 時間の窓(`[1d]`)がアラートで使えるか。Cloud Monitoring のコンソールの PromQL 画面で、上の式が値を返すことを先に見る。使えなければ、下の BigQuery の方法にする
- これは「直近 24 時間」の窓で、日本時間の日付の切れ目ではない

### 代わり: BigQuery の定期クエリで日付ごとに見る

毎朝、昨日の合計が閾値を超えていたらクエリをわざと失敗させ、定期クエリの「失敗時のメール通知」で知らせます。

```sql
DECLARE d DATE DEFAULT DATE_SUB(CURRENT_DATE('Asia/Tokyo'), INTERVAL 1 DAY);
DECLARE total FLOAT64 DEFAULT (
  SELECT IFNULL(SUM(jsonPayload.est_cost_usd), 0)
  FROM `PROJECT.ai_usage.run_googleapis_com_stdout`
  WHERE timestamp >= TIMESTAMP(d, 'Asia/Tokyo')
    AND timestamp < TIMESTAMP(DATE_ADD(d, INTERVAL 1 DAY), 'Asia/Tokyo')
    AND jsonPayload.event = 'ai.usage');
IF total > 20 THEN
  SELECT ERROR(FORMAT('AI cost %t USD on %t is over 20 USD', ROUND(total, 2), d));
END IF;
```

コンソールの BigQuery → 定期クエリで、毎日 09:00(Asia/Tokyo)に設定し、「失敗時にメール通知」を有効にする。

## わかっている限界

- **見積もり**であって請求額ではない。正しい額は Anthropic のコンソール(Usage / Cost)で月に 1 回突き合わせる
- サーバー側のフォールバック(断られたとき別モデルで答え直す)の課金の内訳は 1 行では分からない。`response_model` で答えたモデルの料金にしている
- ストリーミングが途中で切れた試行は、それまでに数えられた分(主に入力)だけが出る
- 料金表にないモデルは `est_cost_usd` なし。`unpriced_calls` が 0 でなければ料金表を足す
- 未ログインの呼び出しは `uid_hash` なし(`anonymous_usd` に入る)。`/v1/generate` はレート制限が有効なとき(Cloud Run)に uid が付く
- `uid_hash` は塩なしの sha256 なので、uid の一覧を持つ人は照合できる。ログと BigQuery の閲覧権限は運営だけにする
