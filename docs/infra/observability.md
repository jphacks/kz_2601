# 計測: 構造化ログ・Error Reporting・監視とアラート(#295)

backend(ai / appconfig)の 5xx の増加・panic・Claude の失敗・応答の遅さに気づくための仕組みと、その手順です。

> **ログの形を変えるコードは入っています。監視(`infra/monitoring.tf`)の apply と、Error Reporting の通知の設定はまだ誰も実行していません。** GCP を変える操作は、承認を得てから人が行います(下の「人の作業」)。

## 目次

1. [ログの形](#ログの形)
2. [Error Reporting](#error-reporting)
3. [監視(Terraform)](#監視terraform)
4. [人の作業 1: apply](#人の作業-1-apply)
5. [人の作業 2: stg でわざと 5xx を出して確かめる](#人の作業-2-stg-でわざと-5xx-を出して確かめる)
6. [Runbook(アラートが来たら)](#runbookアラートが来たら)
7. [わかっている限界](#わかっている限界)

## ログの形

コードは `backend/internal/httpx/logging.go`(`httpx.NewLogger()`)。ai・appconfig・purge が標準出力に [Cloud Logging の構造化ログ](https://cloud.google.com/logging/docs/structured-logging)の JSON を 1 行ずつ出します。ai は `log.Printf` の出力も同じ形になります(`slog.SetDefault`)。

| フィールド | 中身 |
| --- | --- |
| `severity` | `DEBUG` / `INFO` / `WARNING` / `ERROR` / `CRITICAL`。Cloud Logging の重大度になる(`jsonPayload` には残らない) |
| `message` | ログのメッセージ(`request`・`panic recovered`・`ai.usage` など) |
| `time` | RFC 3339 |
| `logging.googleapis.com/trace` | `projects/<GOOGLE_CLOUD_PROJECT>/traces/<trace id>`。`traceparent`(優先)か `X-Cloud-Trace-Context` から。Cloud Run のリクエストログと同じ trace でまとまる |
| `logging.googleapis.com/spanId` / `trace_sampled` | 同じヘッダから |
| `requestId` | `X-Request-Id`(無ければサーバーが作る。応答のヘッダとエラーの本文にも入る) |
| `uid_hash` | 認証できた呼び出し元の擬名。`sha256("ai-usage:" + uid)` の先頭 8 バイト(`ai.usage` と同じ値・[ai-cost.md](ai-cost.md))。**生の uid は出さない** |
| `@type` / `serviceContext` | `ERROR` 以上だけ。`ReportedErrorEvent` と `K_SERVICE` / `K_REVISION`(Error Reporting 用) |

リクエストごとのアクセスログ(`httpx.Log`)は `event="http.request"`・`method`・`path`(クエリは出さない)・`status`・`duration`・`client`。**5xx は `ERROR`**、それ以外は `INFO`。

**出さないもの**: 生の uid、リクエストの本文・クエリ・発話・プロンプト・応答の本文、トークン。テスト(`logging_test.go`・`auth/loguid_test.go`・`ai/usage_log_test.go`)で確かめています。

例(5xx のアクセスログ):

```json
{"time":"2026-11-02T12:00:00Z","severity":"ERROR","message":"request","event":"http.request","method":"POST","path":"/v1/apps","status":500,"duration":12000000,"requestId":"9f2c…","context":{"httpRequest":{"method":"POST","url":"/v1/apps","responseStatusCode":500}},"logging.googleapis.com/trace":"projects/jphacks-kz2601/traces/105445aa7843bc8bf206b12000100000","logging.googleapis.com/spanId":"0000000000000001","logging.googleapis.com/trace_sampled":true,"uid_hash":"3f1c0e9a7b2d4c51","@type":"type.googleapis.com/google.devtools.clouderrorreporting.v1beta1.ReportedErrorEvent","serviceContext":{"service":"appconfig-stg","version":"appconfig-stg-00042-abc"}}
```

Logs Explorer でよく使うクエリ:

```
resource.type="cloud_run_revision" AND resource.labels.service_name="appconfig-stg" AND severity>=ERROR
jsonPayload.event="http.request" AND jsonPayload.status>=500
jsonPayload.requestId="<X-Request-Id の値>"
jsonPayload.event="ai.usage" AND jsonPayload.outcome="error"
```

## Error Reporting

- 5xx のアクセスログと `panic recovered` は `@type: ReportedErrorEvent` 付きの `ERROR` なので、[Error Reporting](https://console.cloud.google.com/errors?project=jphacks-kz2601) に集まる。サービス・リビジョンで分かれる
- panic は `stack_trace` に Go の panic の形(`panic: …` + `goroutine N [running]:`)で出すので、Error Reporting がスタックを読める
- 5xx(panic 以外)はスタックが無いので、メッセージ(`request`)と `context.httpRequest` でまとまる。原因は同じ `requestId` / trace の他のログで見る
- ai も `httpx.Recover` の内側で動くようになった(前は panic で接続が切れていた)。panic は 500 の JSON を返す

## 監視(Terraform)

`infra/monitoring.tf` と `infra/dashboards/backend.json.tftpl`。`monitored_services`(サービス名 → ホスト名・p95 の閾値)に入れたサービスだけを見ます。今は **stg も prod と同じプロジェクト `jphacks-kz2601` にある**(2026-11-02 に `gcloud run services list` で確認)ので、prod の tfvars に 4 つとも入れます(`infra/envs/prod.tfvars.example`)。

| 種類 | 名前 | 中身 |
| --- | --- | --- |
| Uptime check | `<サービス> /health` | `GET https://<host>/health` が 2xx で `ok` を含む。間隔は `uptime_check_period`(既定 300s) |
| ログベースの指標 | `ai_claude_calls` | `jsonPayload.event="ai.usage"` の行数。ラベル `outcome` / `operation` / `error_kind`(#472 のログ) |
| ログベースの指標 | `backend_panics` | `jsonPayload.message="panic recovered"` の行数 |
| アラート | /health uptime check failing | 2 か所以上のリージョンから失敗(CRITICAL) |
| アラート | backend 5xx rate | 5 分の窓で `request_count` の 5xx / 全体 > `error_rate_threshold`(既定 5%)。サービスごと(ERROR) |
| アラート | `<サービス>` p95 latency | `request_latencies` の p95 が閾値を 5 分超える。閾値はサービスごと(ai 60s・appconfig 1.5s)(WARNING) |
| アラート | Claude failure rate | 10 分の窓で `outcome=error` / 全試行 > `claude_failure_rate_threshold`(既定 20%)**かつ** 失敗が `claude_failure_min_count`(既定 3)件以上(ERROR) |
| アラート | backend panic recovered | 5 分の窓で panic が 1 件以上(ERROR) |
| ダッシュボード | `backend (<project>)` | リクエスト数・5xx 率・p95・インスタンス数・Claude の試行(outcome / error_kind)・panic・/health |
| 通知チャンネル | メール | `alert_emails` の数だけ。Slack は下の手順でコンソールから |

- 5xx 率の分子は Cloud Run の `request_count`(コンテナの 5xx と、Cloud Run 自身が返した 503 などの両方)。ログベースではないので、ログの形が変わっても止まらない
- 費用の分布の指標 `ai_usage_cost_usd` と 1 日の費用のアラートは [ai-cost.md](ai-cost.md) の別の手順(このコードでは作らない)

## 人の作業 1: apply

> 承認を得てから実行する。まだ実行していない。

前提: `infra/README.md` の state のバケットがある。prod をまだ Terraform に取り込んでいない(import 前)なら、監視のリソースだけを `-target` で作る(他のリソースを作り直さない)。

```sh
cd infra
cp envs/prod.tfvars.example envs/prod.tfvars   # 済みなら不要。コミットしない
# envs/prod.tfvars を開いて alert_emails にメールアドレスを入れる(monitored_services は例のまま 4 つ)
terraform init -backend-config=envs/prod.backend.hcl

# import 前: 監視だけ
terraform plan -var-file=envs/prod.tfvars -out=monitoring.tfplan \
  -target=google_project_service.monitoring \
  -target=google_monitoring_notification_channel.email \
  -target=google_logging_metric.claude_calls \
  -target=google_logging_metric.panics \
  -target=google_monitoring_uptime_check_config.health \
  -target=google_monitoring_alert_policy.uptime \
  -target=google_monitoring_alert_policy.http_5xx_rate \
  -target=google_monitoring_alert_policy.latency_p95 \
  -target=google_monitoring_alert_policy.claude_failure_rate \
  -target=google_monitoring_alert_policy.panics \
  -target=google_monitoring_dashboard.backend
# plan が「add だけ・change / destroy が 0」であることを見てから
terraform apply monitoring.tfplan

# import 後: いつもどおり
# terraform apply -var-file=envs/prod.tfvars
```

apply の後:

| # | やること |
| --- | --- |
| 1 | 届いたメール「Google Cloud Monitoring の通知チャンネルの確認」を開いて確認する(しないと届かない) |
| 2 | Slack にも送るなら: コンソールの Monitoring → Alerting → Edit notification channels → Slack で連携 → できたチャンネルの name(`projects/jphacks-kz2601/notificationChannels/<n>`)を `alert_notification_channels` に入れてもう一度 apply |
| 3 | Error Reporting の通知: [Error Reporting](https://console.cloud.google.com/errors?project=jphacks-kz2601) → Configure notifications で上と同じチャンネルを選ぶ(新しい種類のエラーが出た時に届く・Terraform のリソースが無いので手で) |
| 4 | Monitoring → Uptime checks で 4 つとも緑になるのを見る(最初の結果まで数分) |
| 5 | 下の「人の作業 2」で 5xx のアラートが届くことを確かめる |

## 人の作業 2: stg でわざと 5xx を出して確かめる

Issue の完了の条件「stg でわざと 5xx を出すと、5 分以内にアラートが届く」の確かめ方です。

`APP_ENV=stg` のときだけ(`deploy-cloudrun.yml` が stg に入れる)、両サービスに `GET /debug/error` があります。prod には出ません。

| URL | 応答 |
| --- | --- |
| `/debug/error` | 500(`{"error":{"code":"internal","message":"debug: intentional 5xx"}}`) |
| `/debug/error?kind=panic` | panic → `httpx.Recover` が 500 を返す(Error Reporting にスタック付きで出る) |

1. このコードが develop にマージされ、stg にデプロイされていることを確かめる(`curl -s -o /dev/null -w '%{http_code}\n' https://appconfig-stg-378953239426.asia-northeast1.run.app/debug/error` が `500`。`404` ならまだ)
2. 時刻を控えてから、5xx を 20 回出す(5 分の窓で 5% を十分に超える数):

   ```sh
   date
   for i in $(seq 20); do
     curl -s -o /dev/null -w '%{http_code} ' https://appconfig-stg-378953239426.asia-northeast1.run.app/debug/error
   done; echo
   curl -s -o /dev/null -w '%{http_code}\n' 'https://appconfig-stg-378953239426.asia-northeast1.run.app/debug/error?kind=panic'
   ```

3. **5 分以内**に「backend 5xx rate」(`service_name=appconfig-stg`)と「backend panic recovered」のメールが届くことを確かめる。Monitoring → Alerting → Incidents にも出る
4. Error Reporting に `request`(5xx)と `panic: debug: intentional panic` が `appconfig-stg` で出ていることを確かめる。panic にはスタックがある
5. Logs Explorer で `jsonPayload.path="/debug/error"` を見て、`severity=ERROR`・`logging.googleapis.com/trace`・`requestId` が付いていることを確かめる
6. 叩くのを止めて 30 分ほどでインシデントが自動で閉じる(`auto_close`)。ai でも確かめるなら URL を `https://ai-stg-378953239426.asia-northeast1.run.app` に替える

届かなかったら: 通知チャンネルの確認(apply の後の 1)・アラートの `notification_channels`・Monitoring → Metrics explorer で `run.googleapis.com/request_count` の `response_code_class=5xx` が増えているか、の順に見る。

## Runbook(アラートが来たら)

共通: [Cloud Run](https://console.cloud.google.com/run?project=jphacks-kz2601)(リビジョンと直前のデプロイ)・[Error Reporting](https://console.cloud.google.com/errors?project=jphacks-kz2601)・[Logs Explorer](https://console.cloud.google.com/logs/query?project=jphacks-kz2601)・ダッシュボード `backend (jphacks-kz2601)`。直前のデプロイが原因なら、Cloud Run で前のリビジョンにトラフィックを戻す(戻し方と承認は [release/process.md](../release/process.md) の「人の承認が要る操作」)。stg のアラートは急がない。

### Uptime check

- どのホストか(インシデントの `host`)。`curl -i https://<host>/health` を自分でも叩く
- 404 の HTML なら Google Front End のパスの問題(`/healthz` は届かない。[staging-check](../backend/staging-check-2026-10-24.md))。5xx ならコンテナが起動できていない: Logs Explorer で起動時の `refusing to start`(Firestore の取り違え防止・#290)や `firestore init failed` を探す

### 5xx 率

- Error Reporting で新しいエラーの種類を見る。`request` の行は `context.httpRequest.url` でどの API かを見て、同じ `requestId` の他の行で原因を追う
- 503 が多くコンテナのログに無いなら、Cloud Run の起動の失敗・最大インスタンス数・メモリ不足(`Memory limit exceeded`)を疑う
- ai の 5xx は Claude の失敗のことが多い: 下の「Claude の失敗率」も見る

### p95 の待ち時間

- ダッシュボードで、遅いのが 1 つのサービスか全部か、インスタンス数が増えているか(コールドスタート)を見る
- ai: `ai.usage` の `latency_ms` と `attempt`(再試行が増えると遅くなる)。モデルや `CLAUDE_EFFORT` を最近変えていないか
- appconfig: Firestore の遅さ。同じ時間帯の Firestore の [ステータス](https://status.cloud.google.com/)も見る

### Claude の失敗率

- ダッシュボードの「Claude errors by error_kind」か、`jsonPayload.event="ai.usage" AND jsonPayload.outcome="error"` で `error_kind` を見る
  - `429_rate_limit_error`: Anthropic の上限。Console で使用量と上限を見る。アプリのレート制限([rate-limit.md](../backend/rate-limit.md))も見直す
  - `529_overloaded_error` / `5xx_*`: Anthropic 側。[status.claude.com](https://status.claude.com/) を見る。続くなら待つ
  - `401_authentication_error`: API キーの失効。Secret Manager の `ANTHROPIC_API_KEY` を人が入れ替える([credentials-inventory.md](credentials-inventory.md))
  - `deadline_exceeded`: `GENERATE_TIMEOUT_SEC` に対して生成が長い
- 費用の急増は [ai-cost.md](ai-cost.md)

### panic

- Error Reporting でスタックを見る。行番号からコードを直し、回帰テストを足して hotfix(prod)か develop(stg)へ
- 同じ `requestId` のアクセスログで、どの API の何回目か

## わかっている限界

- Claude の失敗率は **HTTP の試行ごと**(再試行で成功した試行の失敗も数える)。ユーザーから見た失敗率より高めに出る
- 5xx 率は呼び出しが少ないと 1 件で閾値を超える(stg は特に)。誤報が多ければ `error_rate_threshold` を上げるか、Claude の失敗率と同じく件数の条件を足す
- Uptime check は `/health` を 300 秒ごとに複数のリージョンから叩くので、そのぶん Cloud Run のリクエスト(と 5xx 率の分母)が増える
- ダッシュボードの JSON はコンソールで直すと Terraform との差分になる。直すときは `backend.json.tftpl` を直して apply する
- trace は `GOOGLE_CLOUD_PROJECT` が無いと付かない(Cloud Run では deploy-cloudrun.yml が入れている)
- ai の `errBody` の `log.Printf`(5xx のエラーメッセージ)は context を持たないので、trace / requestId が付かない。同じ時刻のアクセスログ(`ERROR`)で突き合わせる
