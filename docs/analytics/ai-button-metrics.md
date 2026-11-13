# aiButton の端末内 AI の集計(#446)

> 対象 Issue: [#446](https://github.com/jphacks/kz_2601/issues/446)。使い道は [#426](https://github.com/jphacks/kz_2601/issues/426) の再検討(`docs/decisions/426-on-demand-local-llm.md` の「決めてほしいこと #2」)。
> 形は `docs/decisions/297-product-metrics.md` の案 A(backend の構造化ログ → BigQuery)にそろえた。iOS に分析 SDK は入れない。

## 目次

1. [何を知りたいか](#何を知りたいか)
2. [集めるもの・集めないもの](#集めるもの集めないもの)
3. [流れ](#流れ)
4. [ラベル](#ラベル)
5. [ログの形](#ログの形)
6. [割合の出し方(Cloud Logging / BigQuery)](#割合の出し方cloud-logging--bigquery)
7. [プライバシーの表示と人の作業](#プライバシーの表示と人の作業)
8. [限界](#限界)

## 何を知りたいか

| 問い | 出す数 |
|---|---|
| 端末内 AI を使えない人はどれくらいか | aiButton を表示した「端末・日」のうち、availability ごとの割合(非対応端末・Apple Intelligence が OFF・準備中・iOS 26 未満) |
| 端末内 AI で足りない依頼は何か | task(DSL の `aiButton.task`)ごとの失敗率と、失敗の理由(文脈が長すぎる・安全の仕組み・答えを読めない など) |

非対応端末が多ければ #426 の案 C / D(モデルを後から配る)、`context_too_long` が多ければ案 B(PCC・32K)を考える材料になる。

## 集めるもの・集めないもの

| 集める | 集めない |
|---|---|
| 端末のローカル日(`YYYY-MM-DD`) | uid・`uid_hash`・端末の ID・広告 ID |
| その日に aiButton を表示した時の、最後に見た availability | 利用者が書いた文・プロンプト・AI の答え |
| task ごとの、端末内 AI で実行した回数 | 記録の値・アプリ名・項目名・DSL の中身 |
| task × 理由ごとの、答えを出せなかった回数 | 例外の文面(`localizedDescription`)。決まった理由のラベルに寄せる |
| 日ごとの乱数 `reportId`(再送の重複を消すため) | appId・IP アドレス(Cloud Run のリクエストログには出るが、このイベントには書かない) |
| `client_build`(CFBundleVersion。どの版の数かを見るため) | |

- 数えないもの: 入力が空・記録が無い(利用者の入力待ち)。オンライン版(`POST /v1/text`)の失敗(#426 の判断材料ではない)。
- `bad_config` は DSL の設定の不足(task・候補・指示が無い)。モデルの力不足ではないので、SQL で分けて見る。

## 流れ

1. Renderer の `AiButtonLeaf` / `AiGenerateLeaf` が、表示した時(`.shown`)と端末内 AI で 1 回実行した時(`.ran`)に `DSLRuntime.recordAIButton` を呼ぶ。
2. ホスト(`AppRuntime+AIButtonMetrics.swift`)が `AIButtonMetricsRecorder` に渡す。端末のローカル日ごとに UserDefaults に数だけ貯める(`metamon.aiButtonMetrics.buckets`)。
3. 起動と前面に戻った時に、**今日より前の日の分**をまとめて `POST /v1/metrics/ai-button`(appconfig)に送る。1 日分は 1 回しか送らない。送れたら消す。400 も消す。通信の失敗は次回に回す。7 日より古い分は送らずに捨てる。
4. appconfig は決まったラベルと回数の範囲だけを確かめ、**ログに書くだけ**(Firestore には書かない)。未知のキー・未知のラベルは 400。
5. Cloud Logging → BigQuery のシンク(#297 の手順 6・人の作業)で集計する。

| 何 | 場所 |
|---|---|
| ラベル(Swift) | `ios/Packages/MetamonDSL/Sources/MetamonDSL/Model/AIButtonMetric.swift` |
| 数える所 | `ios/Packages/MetamonRenderer/Sources/MetamonRenderer/Components/Leaves/AiButtonLeaf.swift`・`AiGenerateLeaf.swift`、理由の振り分けは `Support/OnDeviceAI.swift` |
| 貯める・送る | `ios/Packages/MetamonServices/Sources/MetamonServices/Consent/AIButtonMetrics.swift` |
| 設定の切り替え | `ios/Metamon/Features/Consent/PrivacySettingsSection.swift`(「AI ボタンの利用状況を送る」) |
| 受ける所 | `backend/internal/appconfig/aibutton_metrics.go`、契約は `docs/backend/openapi-appconfig.yaml` の `/metrics/ai-button` |

### 送り手(OS ごと)

`POST /v1/metrics/ai-button` を送る OS と、送っているコードの場所。数える所・貯める所・送る時点は上の流れと同じ形(Android と Mac は iOS に合わせて作った)。

| OS | 送るか | 送るコード | 送る時点 | 端末内 AI の availability |
|---|---|---|---|---|
| Android | 送る | 本文と送信: `android/services/src/main/kotlin/jp/kz2601/metamon/services/consent/AiButtonMetrics.kt`(`AiButtonMetricsRecorder`・送り手)。数える所: `android/renderer/.../components/ai/AiButtonLeaf.kt`。つなぐ所: `android/app/.../consent/AiButtonMetricsWiring.kt`・`AppContainer.kt`(`flushAiButtonMetrics`)。設定: `android/app/.../settings/SettingsSheet.kt` | 起動(`MetamonApplication.kt`)と前面に戻った時(`lifecycle/ForegroundSteps.kt`) | 端末内 AI が無い(`UnsupportedOnDeviceAi`)ので、いつも `device_not_eligible` |
| iOS | 送る(審査用の版は送らない) | 上の「何 / 場所」の表のとおり(`MetamonServices/Consent/AIButtonMetrics.swift`・`ios/Metamon/App/AppRuntime+AIButtonMetrics.swift`) | 起動と前面に戻った時 | Foundation Models の状態(`Support/OnDeviceAI.swift`) |
| Mac | 送る(審査用の版は送らない) | `ios/MetamonMac/App/Services/MacAIButtonMetrics.swift`(送る時点・送り手は iOS と同じ `HTTPAIButtonMetricsSender`)。数える所: `ios/MetamonMac/App/Runtime/MacAppRuntime+AIText.swift`(`recordAIButton`)。設定: `ios/MetamonMac/App/Settings/MacSettingsView.swift` | 起動と前面に戻った時(`NSApplication.didBecomeActiveNotification`) | Foundation Models の状態(iOS と同じ `Support/OnDeviceAI.swift`。`os_too_old` は macOS 26 未満) |
| Windows・Linux(`desktop/`) | 未対応 | 無い。aiButton をまだ描かない(#2387) | — | — |

- ログの行(下の「ログの形」)には OS が無い。Android の分が入ると `device_not_eligible` の割合が上がるので、#426 の判断に使う時は OS を分けて見る必要がある。分け方は**決めること**(行に OS を足す / `client_build` の形で見分ける など)。
- デスクトップに aiButton が入る時(#2387)は、この表に 1 行足し、端末内 AI の有無と送るかを決める。

### 版と切り替え

- **審査用の版(`APP_STORE_REVIEW`)には入れない**(数えない・送らない・設定も出さない)。栄養ラベルに「使用状況データ」が増えるため。β(TestFlight)と開発の版だけ。
- 通常の版では既定オン。設定 → プライバシー →「AI ボタンの利用状況を送る」でオフにでき、オフにすると貯めた分も消す。AI への送信の同意(`AIConsentView`)とは別(AI に送るものではないため・#297 の方針どおり)。
- 送るのは appconfig が live の時だけ。モック(`METAMON_MOCK=1`)では貯めるだけで送らない。

## ラベル

backend(`aibutton_metrics.go`)と iOS(`AIButtonMetric`)で同じ。足す時は両方と OpenAPI を同じ PR で直す。

| 種類 | 値 |
|---|---|
| availability | `available` / `device_not_eligible`(非対応端末)/ `apple_intelligence_off`(設定で OFF)/ `model_not_ready`(準備中)/ `os_too_old`(iOS 26 未満)/ `other` |
| task | `summarize` / `classify` / `rewrite` / `extract` / `suggest` / `generate` / `other`(DSL の未知の値) |
| reason | `unavailable`(押した時に使えなくなっていた)/ `invalid_response`(答えを読めない・分類が候補外・構造化出力の失敗)/ `context_too_long`(文脈 約 4K を超えた)/ `guardrail` / `unsupported_language` / `refusal` / `rate_limited`(上限・同時実行)/ `assets_unavailable` / `model_error`(その他)/ `bad_config`(DSL の設定の不足) |

Foundation Models の `LanguageModelSession.GenerationError` との対応は `OnDeviceAIError.reason(for:)`。

## ログの形

slog の JSON(`ai.usage` と同じ形)。1 回の送信で、日ごとに次の行が出る。共通の属性: `event`・`report_id`・`day`・`availability`・`client_build`。

| event | 1 行の単位 | 追加の属性 |
|---|---|---|
| `ai.button.availability` | 端末・日 | なし |
| `ai.button.task` | 端末・日・task | `task`・`attempts`・`failures`(理由の合計) |
| `ai.button.failure` | 端末・日・task・理由(0 回は出さない) | `task`・`reason`・`count` |

Cloud Logging のフィルタ: `resource.labels.service_name=~"appconfig" AND jsonPayload.event=~"^ai\.button\."`。#297 のシンクのフィルタ(`^(ai\.|app\.|…)`)にそのまま入る。

## 割合の出し方(Cloud Logging / BigQuery)

### Cloud Logging(シンクを作る前・ざっと見る)

ログ エクスプローラで次を流し、「ログ分析」(Log Analytics)で数える。

```
resource.type="cloud_run_revision"
resource.labels.service_name=~"appconfig"
jsonPayload.event="ai.button.availability"
```

Log Analytics の SQL(バケットのビューの名前はプロジェクトに合わせて直す):

```sql
SELECT
  JSON_VALUE(json_payload.availability) AS availability,
  COUNT(DISTINCT JSON_VALUE(json_payload.report_id)) AS device_days
FROM `PROJECT.global._Default._AllLogs`
WHERE JSON_VALUE(json_payload.event) = 'ai.button.availability'
  AND timestamp >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
GROUP BY availability
ORDER BY device_days DESC;
```

### BigQuery(#297 のシンクの後)

前提は #297 と同じ(データセット `product_metrics`、テーブル `run_googleapis_com_stdout`。列名はシンクが作るスキーマに合わせて直す)。**再送の重複は `report_id` で消す**(同じ日の同じ端末の分は同じ `report_id`)。

```sql
-- 共通: ai.button.* を report_id ごとに 1 回だけ
CREATE OR REPLACE VIEW product_metrics.ai_button AS
SELECT * EXCEPT (rn) FROM (
  SELECT
    jsonPayload.event AS event,
    jsonPayload.report_id AS report_id,
    SAFE.PARSE_DATE('%Y-%m-%d', jsonPayload.day) AS day,
    jsonPayload.availability AS availability,
    jsonPayload.task AS task,
    jsonPayload.reason AS reason,
    SAFE_CAST(jsonPayload.attempts AS INT64) AS attempts,
    SAFE_CAST(jsonPayload.failures AS INT64) AS failures,
    SAFE_CAST(jsonPayload.count AS INT64) AS count,
    jsonPayload.client_build AS client_build,
    ROW_NUMBER() OVER (
      PARTITION BY jsonPayload.event, jsonPayload.report_id, jsonPayload.task, jsonPayload.reason
      ORDER BY timestamp) AS rn
  FROM product_metrics.run_googleapis_com_stdout
  WHERE jsonPayload.event IN ('ai.button.availability', 'ai.button.task', 'ai.button.failure')
)
WHERE rn = 1;
```

#### 1. 端末内 AI を使えない割合(週ごと・端末・日の単位)

```sql
SELECT
  DATE_TRUNC(day, WEEK(MONDAY)) AS week,
  COUNT(*) AS device_days,
  SAFE_DIVIDE(COUNTIF(availability = 'available'), COUNT(*)) AS available_rate,
  SAFE_DIVIDE(COUNTIF(availability = 'device_not_eligible'), COUNT(*)) AS not_eligible_rate,
  SAFE_DIVIDE(COUNTIF(availability = 'apple_intelligence_off'), COUNT(*)) AS ai_off_rate,
  SAFE_DIVIDE(COUNTIF(availability = 'model_not_ready'), COUNT(*)) AS not_ready_rate,
  SAFE_DIVIDE(COUNTIF(availability = 'os_too_old'), COUNT(*)) AS os_too_old_rate
FROM product_metrics.ai_button
WHERE event = 'ai.button.availability'
GROUP BY week ORDER BY week;
```

#### 2. task ごとの失敗率(β の期間全体)

```sql
SELECT
  task,
  SUM(attempts) AS attempts,
  SUM(failures) AS failures,
  SAFE_DIVIDE(SUM(failures), SUM(attempts)) AS failure_rate
FROM product_metrics.ai_button
WHERE event = 'ai.button.task'
GROUP BY task ORDER BY failure_rate DESC;
```

#### 3. 失敗の理由の内訳(task × 理由・設定の不足は分ける)

```sql
SELECT
  task, reason,
  SUM(count) AS failures,
  SAFE_DIVIDE(SUM(count), SUM(SUM(count)) OVER (PARTITION BY task)) AS share_in_task
FROM product_metrics.ai_button
WHERE event = 'ai.button.failure'
GROUP BY task, reason
ORDER BY task, failures DESC;
```

#426 の再検討の目安(案): `not_eligible_rate` が高い → 案 C / D、`context_too_long` が task の失敗の多くを占める → 案 B(PCC)、`bad_config` が多い → モデルでなく生成側(DSL)の問題として別 Issue。

## プライバシーの表示と人の作業

| # | やること | 誰が | 人の承認 |
|---|---|---|---|
| 1 | この集計を β の版で始めてよいか決める(#297 の「決めてほしいこと」と合わせる) | オーナー | **要る** |
| 2 | プライバシーポリシーに書き足す。1.8 の後に「AI ボタンの利用状況: 端末内の AI が使えるかと、答えを出せなかった回数をボタンの種類ごとに 1 日 1 回送ります。ユーザー ID・入力した文・記録は含めません。設定でオフにできます」。3 章の保存期間(ログ 30 日・BigQuery は #297 の決定に合わせる) | Claude(下書き)→ 人 | **要る**(公開する文言) |
| 3 | 通常の版を App Store に出す時は、栄養ラベルとマニフェストに「使用状況データ > 製品の操作」(本人に紐づかない・トラッキングなし・用途はアナリティクス)を足す。審査用の版は集めないので今は変えない | 人(#328) | **要る** |
| 4 | BigQuery のシンク(#297 の手順 6) | 人 | **要る**(GCP の変更) |
| 5 | β の後に、上の SQL の結果を #426 に貼って再検討する | 人 or Claude | — |

- 認証は付ける(アプリ以外からの書き込みを減らすため)が、uid はログに出さない。サーバーは数を保存しない(ログだけ)。
- ATT は要らない(IDFA を使わず、他社のデータとつながない)。

## 限界

- 単位は「端末・日」。uid を持たないので、同じ人を日をまたいで数えられない(「何人が」ではなく「何日分の利用のうち」)。割合の判断には足りる。
- availability はその日の**最後の値**。1 日の中で「準備中 → 使える」に変わった端末は「使える」に数える。
- 送るのは翌日以降に開いた時。β の最終日の分は、次に開くまで届かない。7 日開かなければ捨てる。
- 端末の時計・タイムゾーンに頼る(サーバーは今日(UTC)の 9 日前〜翌日の日付だけ受ける)。
- 設定でオフにした人・審査用の版の人は数に入らない。偏りはありうる。
