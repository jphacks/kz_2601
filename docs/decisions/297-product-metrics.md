# #297 プロダクトの指標とイベントの設計

> 状態: **提案(オーナーの判断待ち)**。2026-10-30 作成。この PR は設計だけで、コード・GCP・Firebase の変更はしない。
> 対象 Issue: [#297](https://github.com/jphacks/kz_2601/issues/297)(依存: #295 backend の構造化ログ、#296 iOS のクラッシュ報告。費用ログは #298 / PR #472)
> 調べた日: 2026-10-30。コードは `origin/develop`(723bb5e)と PR #472 のブランチ `origin/feat/ai-cost-log-298`(efad3b8)を読んだ。料金・ラベルの区分は変わるので、決める前に「参考」をもう一度見ること。

## 目次

1. [結論(推奨)](#結論推奨)
2. [選択肢の表](#選択肢の表)
3. [指標の定義](#指標の定義)
4. [イベントの一覧](#イベントの一覧)
5. [根拠](#根拠)
6. [ダッシュボードと週の SQL(下書き)](#ダッシュボードと週の-sql下書き)
7. [作業の順番と人の承認](#作業の順番と人の承認)
8. [決めてほしいこと](#決めてほしいこと)
9. [参考](#参考)

## 結論(推奨)

**案 A(backend の構造化ログ → BigQuery のシンク → Looker Studio)にする。iOS に分析 SDK は入れない。**

- 指標のほとんどは、iOS がすでに呼んでいる API(`POST /v1/generate`・`GET /v1/apps/{appId}/dsl`・`POST /v1/apps/{appId}/share` など)のサーバー側で 1 行のログを足せば取れる。PR #472 の `ai.usage` と同じ形(slog の JSON・`event` 属性・`uid_hash`)にそろえる。
- 送るのは**ラベル・回数・時間・擬名の ID だけ**。入力文(話した内容・会話)、DSL の中身(アプリ名・項目名を含む)、記録の値は送らない。生の uid・appId も送らない。
- 広告 ID(IDFA)を使わず、他社のデータとつながないので**トラッキングに当たらず、ATT のダイアログは要らない**。ただし「行動の分析ツールは使いません」と書いたプライバシーポリシー(1.8)・プライバシーマニフェスト・栄養ラベルの書き換えは要る(下の「根拠 3」)。
- iOS で要る変更は 2 つだけ: 版の区別のヘッダー(`X-Client-Build: standard|review`)と、課金の確認カード(将来)のための小さな送信。後者は IAP を出すまで作らない。
- Firebase Analytics(案 B)は、端末側の操作(画面の滞在・課金画面の離脱など)を細かく見たくなった時に再検討する。

## 選択肢の表

| 選択肢 | 意味 | 費用 | プライバシー・審査 | 作業量 |
|---|---|---|---|---|
| **A. backend のログ → BigQuery(推奨)** | ai / appconfig が 1 リクエスト 1 行のイベントを出し、Cloud Logging のシンクで BigQuery に流す。Looker Studio で週ごとに見る | 月 0 円に近い見込み(下の「根拠 4」)。シンクから BigQuery への書き込みの課金は**要確認** | ATT 不要。栄養ラベルに「使用状況データ: 製品の操作(分析・本人に紐づく)」を足す。マニフェストも同じ。端末に SDK が増えない | 小〜中(backend に 6 種のイベント、iOS はヘッダー 1 つ) |
| B. Firebase Analytics(GA4)を iOS に入れる | iOS から `logEvent` で送る。GA4 の画面か BigQuery エクスポートで見る | 無料枠内の見込み(エクスポートの上限は**要確認**) | IDFA を使わない設定なら ATT 不要。ただしラベルに「識別子(デバイス ID)」「製品の操作」などが増える見込み(**要確認**)。SDK が端末の情報を自動で送るので、何が出るかを Google の文書に頼ることになる | 中(SDK 追加・自動収集の止め方の確認・全画面にイベント) |
| C. 両方 | サーバーで取れるものは A、端末でしか分からないもの(課金画面の表示など)は B | A + B | B と同じラベルの変更が要る | 大(2 か所で定義がずれないよう管理) |

## 指標の定義

すべて **週(月曜 0:00 JST 始まり)** で出す。「ユーザー」は `uid_hash`(下の「根拠 2」)。匿名ログインも 1 ユーザーとして数える。テスト用の uid は除外の一覧で外す(**要確認**: 運営の uid の一覧をどこに持つか)。

| 指標 | 定義 | 使うイベント | 版 |
|---|---|---|---|
| 初回の生成までの時間 | ユーザーの最初のイベント(`session.start`)から、最初に成功した `ai.generate`(mode=create・outcome が validated か regenerated)までの時間。中央値と p90。その週に最初のイベントがあったユーザーが対象 | `session.start`・`ai.generate` | 両方 |
| 生成の成功率 | `ai.generate` のうち outcome ごとの割合。**validated**(1 回目で検証を通った)/ **regenerated**(作り直し・エリアの外し・全体の作り直しで通った)/ **refused**(AI が断った)/ **failed**(それ以外の失敗)。`rate_limited` と `bad_request` は分母から外す | `ai.generate` | 両方 |
| 直しの回数 | 1 つのアプリ(`app_hash`)に対する mode=edit の成功(`unchanged` を除く)の回数。週の中央値と平均。作った週と翌週以降を分けても見る | `ai.generate` | 両方 |
| 翌日・7 日後に開いた率(D1 / D7) | 最初に生成に成功した日(JST)を 0 日目とし、1 日目 / 7 日目に、自分が持ち主のアプリを開いた(`app.opened`・role=owner)ユーザーの割合。その週に 0 日目があったユーザーの集まり(コホート)ごと | `ai.generate`・`app.opened` | 両方 |
| 共有した率 | その週に生成に成功したユーザーのうち、共有リンクを作った(`app.shared`)ユーザーの割合。**通常の版だけ**(審査用の版 `APP_STORE_REVIEW` には共有が無い・`ios/Metamon/App/BuildConfig.swift` の `#if !APP_STORE_REVIEW`)。分母も `client_build=standard` に絞る | `ai.generate`・`app.shared` | 通常の版 |
| Pro の確認カードの表示と購入(将来) | 確認カード(`PaywallSheet`)を見たユーザー数・購入の数・その割合。**v1(審査用の版)は App 内課金が無い**ので、出すまで作らない。購入数は App Store Connect の売上レポートでも見られる | `paywall.shown`・`paywall.purchased` | 通常の版(IAP を出した後) |

補足:
- 「作り直した」は `Meta.Regenerations > 0`・`Meta.AreaFallback`・`Meta.DowngradedToFull`・`Meta.LintRetried` のどれか(`backend/internal/ai/generate.go` の `Meta`)。lint の作り直しを「作り直し」に入れるかは「決めてほしいこと 4」。
- 「断った」は `Error.Code == "refused"`(`generate.go` の `call` で `StopReason == "refusal"` の時)。
- D7 は「7 日目ちょうど」で出す。「1〜7 日目のどこかで開いた」も同じ SQL の条件を変えれば出る。

## イベントの一覧

共通の属性(すべてのイベント): `event`(名前)、`uid_hash`、`client_build`(`standard|review|unknown`。iOS のヘッダーから。入るまでは unknown)、`env`(Cloud Run のサービス名から SQL で出すので、ログには出さなくてよい)。

**どのイベントにも入れないもの**: 話した内容・会話・`transcript`・要約(`summary`)、DSL の中身(アプリ名・説明・タブ・項目名・コンポーネントの中身)、記録の値・写真・位置、生の uid・生の appId・共有トークン、IP アドレス、端末の ID・広告 ID。テストで「本文と生の ID が出ない」ことを確かめる(PR #472 の `usage_log_test.go` と同じやり方)。

| 名前 | いつ出す | 属性(共通以外) | どこで出す | 元にする今の処理(確かめた場所) |
|---|---|---|---|---|
| `ai.usage` | Claude を 1 回呼ぶたび(PR #472・**マージ前**) | operation・model・attempt・outcome・トークン数・latency_ms・est_cost_usd | backend(ai) | `backend/internal/ai/usage_log.go`(PR #472)。費用用。指標では使わないが、`uid_hash` が同じなので 1 人あたりの費用と継続を結べる |
| `ai.generate` | `POST /v1/generate` が 1 回終わるたび(成功・失敗とも) | `app_hash`・mode(`create|edit|undo`)・outcome(`validated|regenerated|refused|failed|unchanged|rate_limited|bad_request`)・error_code・attempts・regenerations・area_fallback・downgraded_to_full・lint_retried・logic_pass(bool)・restricted_count・omissions_count・locale・stream(bool)・latency_ms(total / claude) | backend(ai) | `generate.go` の `GenerateStream` の終わり(`Result.Meta` と `Error.Code` が揃う所)。`rate_limited` は `ratelimit` の middleware で 429 を返す所(**要確認**: middleware 側にログを足すか、Cloud Run のリクエストログの 429 で数えるか) |
| `session.start` | `GET /v1/apps`(アプリの一覧)を受けた時 | なし | backend(appconfig) | `internal/appconfig/handler.go` の `listApps`。iOS は `AppRuntime.appsList()` で呼ぶ。**起動ごとに必ず 1 回呼ぶかは要確認**(呼ぶ場所が複数ありうる)。初回の生成までの時間の起点と「その日に使った」の判定に使う |
| `app.opened` | `GET /v1/apps/{appId}/dsl` を `If-None-Match` **なし**で受けた時 | `app_hash`・role(`owner|member`) | backend(appconfig) | iOS はアプリの切り替え・起動時の復元で `fetchDocument`(ヘッダーなし)、バックグラウンドの取り直しで `fetchDocumentIfChanged`(`If-None-Match` あり)を使う(`HTTPAppConfigClient.swift`)。ヘッダーの有無で分けられる。ショートカット(`IntentHost.swift`)もヘッダーなしで呼ぶので少し混ざる(**要確認**: 気になるなら iOS から `X-Client-Reason` を足す) |
| `app.shared` | `POST /v1/apps/{appId}/share` が成功した時 | `app_hash` | backend(appconfig) | `handler.go` の `createShare` |
| `app.published` | `POST /v1/apps/{appId}/publish` が成功した時(「探す」に掲載) | `app_hash` | backend(appconfig) | `handler.go` の `publishApp` |
| `share.imported` | `POST /v1/shares/{token}/import`・`POST /v1/public-apps/{id}/install` が成功した時 | `app_hash`(入った先)・source(`share|public`)・joined(bool) | backend(appconfig) | `share.go` にすでに `slog.Info("appconfig: installed copy", ...)` があるが、生の appId を出している。イベントに置き換えるか、生の appId を外すかは #295 と合わせる |
| `paywall.shown`(将来) | 有料機能の確認カードを出した時 | feature(機能の名前のラベル) | iOS → backend(新しい `POST /v1/metrics`。許可した名前と属性だけ受ける) | `ios/Metamon/Features/Purchase/PaywallSheet.swift`(通常の版だけ)。**IAP を出すまで作らない** |
| `paywall.purchased`(将来) | StoreKit の購入が成功した時 | product(商品 ID)・trial(bool) | iOS → backend(同上)。サーバーでの購入の確認(#314〜#316)が入ったらそちらで出す | 同上 |

`app_hash` は `sha256("metrics-app:" + appId)` の先頭 8 バイト(16 桁)。共有や Firestore の文書と直接は結べないようにする(ただし appId の一覧を持つ人は照合できる。`uid_hash` と同じ限界)。

## 根拠

### 1. 今の backend で取れるもの(コードで確かめたこと)

- ai サービスのエンドポイントは `POST /v1/generate`・`POST /v1/chat`・`POST /v1/text`・`DELETE /v1/chat/{appId}`(`backend/internal/ai/handler.go`)。**アプリを作る・直すのは必ず `/v1/generate`**(iOS の `HTTPAIClient.swift`。`/v1/chat` は質問・提案だけで、`ready` の後に `/v1/generate` を呼ぶ)。なので生成の成功率は `/v1/generate` の 1 か所で取れる。
- `Result.Meta` に attempts・regenerations・areaFallback・lintRetried などが既にあり、失敗は `Error.Code`(`refused`・`generation_failed`・`upstream_timeout` など)で分かれている。**新しい計算は要らず、ログに出すだけ**。
- ai サービスには今、リクエストごとのログが無い(`cmd/ai/main.go` は `log.Printf` だけで `httpx.Log` を使っていない)。appconfig は `httpx.Log` で method・path・status・duration・requestId を出しているが、**uid が無いので、ユーザーごとの指標はこのログだけでは出せない**。path に生の appId が入っている点も、指標用には使わない理由。
- 指標用のイベントは、`ai.usage` と同じく `slog` の JSON で `event` 属性を付ける。シンクのフィルタを `jsonPayload.event=~"^(ai\.|app\.|session\.|share\.|paywall\.)"` のようにまとめられる。
- PR #472 はまだ **OPEN**。`ai.generate` は `ai.usage` の `UIDHash`・`WithOperation` を使う前提なので、#472 のマージを先にする。

### 2. 擬名の ID

- `uid_hash` は PR #472 の `ai.UIDHash`(`sha256("ai-usage:" + uid)` の先頭 8 バイト)を**そのまま使う**。同じ値にすると、1 人あたりの費用(#298)と継続(#297)を BigQuery で結べる。appconfig からも使うので、`internal/ai` から共通のパッケージ(例: `internal/metrics`)に移す。
- 塩なしの sha256 なので、uid の一覧を持つ人は照合できる(#472 の docs に書かれた限界と同じ)。BigQuery とログの閲覧は運営だけにする。
- `/v1/generate` は、レート制限が有効な時(Cloud Run)にだけ uid が付く(`ai-cost.md` の記載。`withRateLimit` が `auth` の検証をしている)。ローカルでは `uid_hash` が空になる。

### 3. プライバシー・ATT・栄養ラベル・同意

- **ATT**: 案 A は IDFA を使わず、他社のアプリ・サイトのデータとつながず、データブローカーにも渡さないので、Apple の言う「トラッキング」に当たらない。ATT のダイアログは要らない。`PrivacyInfo.xcprivacy` の `NSPrivacyTracking=false` のまま。案 B も、AdSupport を入れず、Google シグナル・広告のリンクを切れば ATT は要らない見込み(**要確認**: Firebase の文書で、IDFA を集めない構成の手順と、SDK 13.x での製品名)。
- **今の表示と矛盾する所**(どの案でも直す):
  - `docs/legal/privacy-policy.md` 1.8 に「行動の分析ツール(Firebase Analytics 等)は使いません」とある。案 A なら「サーバーで、利用の回数・成功/失敗などを、話した内容を含めずに集計します」に書き換える。案 B・C なら Firebase Analytics を使う旨と、委託先(2 章)の Google の項目に Analytics を足す。
  - 同じポリシーの 3 章「ログは Cloud Logging に最大 30 日」。BigQuery に流すと保存が延びるので、保存期間を書き足す(「決めてほしいこと 3」)。**PR #472 の `ai-cost.md` の手順はデータセットを 400 日で作るので、30 日の記載と合っていない**。
  - `ios/Metamon/PrivacyInfo.xcprivacy` のコメントに「集めないもの: 分析」とある。
- **栄養ラベル(App Store Connect)とマニフェスト**:
  - 案 A: 「使用状況データ > 製品の操作(Product Interaction)」を足す。用途は「アナリティクス」、本人に紐づく(`uid_hash` は uid から作るので「紐づかない」とは言いにくい)、トラッキングなし。マニフェストに `NSPrivacyCollectedDataTypeProductInteraction`(Linked=true・Tracking=false・Purpose=Analytics)を足す。既存の「ユーザー ID」の用途に「アナリティクス」を足すかは**要確認**(Apple の定義では、集めたデータを分析にも使うなら用途に足す)。
  - 案 B: 上に加えて、Firebase の文書が挙げる種類(「識別子 > デバイス ID」、「診断」など)を足す見込み(**要確認**: Firebase の「App Store のデータ開示」のページで Analytics の行を見る)。
  - #328(栄養ラベルとマニフェスト)と同じ PR で直すのがよい。
- **同意の画面**: 今の同意は「AI へのデータ送信」(`AIConsentView`・`PrivacySettingsSection`)。指標は AI への送信ではないので、この同意に混ぜない。案 A は話した内容を含まない運営のための集計なので、**ポリシーへの記載で足り、ダイアログは出さない**のを推奨。設定に「利用状況の集計を送らない」の切り替えを置くかは「決めてほしいこと 5」(置くなら iOS がヘッダーで伝え、backend がそのリクエストのイベントを出さない)。法的に同意が要らないかは**要確認**(個人情報保護法の「利用目的の通知・公表」で足りる見込み)。

### 4. 費用(案 A)

- 1 イベント 0.5 KB、1 日 1,000 人 × 50 イベントとして 1 日 25 MB、月 1 GB 未満の見込み。BigQuery の無料枠(保存 10 GB・クエリ 月 1 TB)に収まる。Looker Studio は無料。
- Cloud Logging の取り込みは月 50 GiB まで無料(今のログと合計)。シンクから BigQuery への書き込みに BigQuery 側のストリーミングの料金がかかるかは**要確認**。
- 週の SQL は日付のパーティションで絞るので、1 回の読み込みは数十 MB の見込み。

### 5. iOS の変更を小さくする理由

- iOS の依存は今 `FirebaseAuth` だけ(`ios/Packages/MetamonServices/Package.swift`・13.1.1 固定)。Analytics を足すと、自動で集める項目(画面の表示・初回起動・端末の情報)の確認と止め方、ラベルの増加、審査用の版での扱いが増える。
- 指標は「作れたか」「次の日も使ったか」で、どれもサーバーへのリクエストで分かる。端末でしか分からないのは課金の確認カードだけで、v1 には無い。

## ダッシュボードと週の SQL(下書き)

**Looker Studio に 1 枚のレポート**を作り、BigQuery の「週の集計ビュー」をデータソースにする。上段に 6 つの数字(今週と前週の差)、下段に週ごとの折れ線(成功率は outcome ごとの積み上げ棒)。stg と本番はフィルタで切り替え。

前提(下書き。シンクを作った後のテーブル名で直す):
- データセット `product_metrics`、テーブル `run_googleapis_com_stdout`(ai と appconfig の両方を 1 つのシンクで流す)。`jsonPayload.*` の列名はシンクが作るスキーマに合わせて直す。
- 週は `DATE_TRUNC(DATE(timestamp, 'Asia/Tokyo'), WEEK(MONDAY))`。
- まず共通のビューを作る。

```sql
-- product_metrics.events: 指標用のイベントだけ・JST の日付・週
CREATE OR REPLACE VIEW product_metrics.events AS
SELECT
  timestamp,
  DATE(timestamp, 'Asia/Tokyo') AS day,
  DATE_TRUNC(DATE(timestamp, 'Asia/Tokyo'), WEEK(MONDAY)) AS week,
  resource.labels.service_name AS service,
  jsonPayload.event AS event,
  jsonPayload.uid_hash AS uid_hash,
  jsonPayload.app_hash AS app_hash,
  jsonPayload.mode AS mode,
  jsonPayload.outcome AS outcome,
  jsonPayload.role AS role,
  jsonPayload.client_build AS client_build
FROM product_metrics.run_googleapis_com_stdout
WHERE jsonPayload.event IN ('ai.generate', 'session.start', 'app.opened', 'app.shared', 'app.published', 'share.imported')
  AND jsonPayload.uid_hash IS NOT NULL;
```

### 初回の生成までの時間(週ごとの中央値・p90、分)

```sql
WITH first_seen AS (
  SELECT uid_hash, MIN(timestamp) AS t0 FROM product_metrics.events GROUP BY uid_hash
),
first_gen AS (
  SELECT uid_hash, MIN(timestamp) AS t1 FROM product_metrics.events
  WHERE event = 'ai.generate' AND mode = 'create' AND outcome IN ('validated', 'regenerated')
  GROUP BY uid_hash
)
SELECT
  DATE_TRUNC(DATE(f.t0, 'Asia/Tokyo'), WEEK(MONDAY)) AS week,
  COUNT(*) AS new_users,
  COUNTIF(g.t1 IS NOT NULL) AS generated_users,
  APPROX_QUANTILES(TIMESTAMP_DIFF(g.t1, f.t0, SECOND) / 60, 100)[OFFSET(50)] AS p50_min,
  APPROX_QUANTILES(TIMESTAMP_DIFF(g.t1, f.t0, SECOND) / 60, 100)[OFFSET(90)] AS p90_min
FROM first_seen f LEFT JOIN first_gen g USING (uid_hash)
GROUP BY week ORDER BY week;
```

注意: シンクを作る前からいるユーザーは、最初のイベントがシンク後になるので「新しいユーザー」に見える。シンクを作った週は捨てる。

### 生成の成功率(週ごと・outcome ごと)

```sql
SELECT
  week,
  COUNT(*) AS total,
  SAFE_DIVIDE(COUNTIF(outcome = 'validated'), COUNT(*)) AS validated_rate,
  SAFE_DIVIDE(COUNTIF(outcome = 'regenerated'), COUNT(*)) AS regenerated_rate,
  SAFE_DIVIDE(COUNTIF(outcome = 'refused'), COUNT(*)) AS refused_rate,
  SAFE_DIVIDE(COUNTIF(outcome = 'failed'), COUNT(*)) AS failed_rate
FROM product_metrics.events
WHERE event = 'ai.generate' AND outcome NOT IN ('rate_limited', 'bad_request')
GROUP BY week ORDER BY week;
```

mode(create / edit)で分けたい時は `GROUP BY week, mode`。

### 直しの回数(アプリごと、週の中央値と平均)

```sql
WITH per_app AS (
  SELECT week, app_hash, COUNT(*) AS edits
  FROM product_metrics.events
  WHERE event = 'ai.generate' AND mode = 'edit' AND outcome IN ('validated', 'regenerated')
  GROUP BY week, app_hash
)
SELECT week, COUNT(*) AS edited_apps, AVG(edits) AS avg_edits,
       APPROX_QUANTILES(edits, 100)[OFFSET(50)] AS p50_edits
FROM per_app GROUP BY week ORDER BY week;
```

### D1 / D7(最初に生成に成功した週のコホートごと)

```sql
WITH day0 AS (
  SELECT uid_hash, MIN(day) AS d0 FROM product_metrics.events
  WHERE event = 'ai.generate' AND outcome IN ('validated', 'regenerated')
  GROUP BY uid_hash
),
opened AS (
  SELECT DISTINCT uid_hash, day FROM product_metrics.events
  WHERE event = 'app.opened' AND role = 'owner'
)
SELECT
  DATE_TRUNC(d.d0, WEEK(MONDAY)) AS cohort_week,
  COUNT(*) AS users,
  SAFE_DIVIDE(COUNTIF(EXISTS(SELECT 1 FROM opened o WHERE o.uid_hash = d.uid_hash AND o.day = DATE_ADD(d.d0, INTERVAL 1 DAY))), COUNT(*)) AS d1,
  SAFE_DIVIDE(COUNTIF(EXISTS(SELECT 1 FROM opened o WHERE o.uid_hash = d.uid_hash AND o.day = DATE_ADD(d.d0, INTERVAL 7 DAY))), COUNT(*)) AS d7
FROM day0 d
WHERE d.d0 <= DATE_SUB(CURRENT_DATE('Asia/Tokyo'), INTERVAL 8 DAY) -- 7 日目が終わったコホートだけ
GROUP BY cohort_week ORDER BY cohort_week;
```

`COUNTIF(EXISTS(...))` が BigQuery で通らなければ、`opened` を LEFT JOIN して `COUNT(DISTINCT IF(...))` に書き換える(**要確認**: 実データで流して確かめる)。

### 共有した率(通常の版だけ)

```sql
WITH gen AS (
  SELECT DISTINCT week, uid_hash FROM product_metrics.events
  WHERE event = 'ai.generate' AND outcome IN ('validated', 'regenerated') AND client_build = 'standard'
),
shared AS (
  SELECT DISTINCT week, uid_hash FROM product_metrics.events
  WHERE event = 'app.shared' AND client_build = 'standard'
)
SELECT g.week, COUNT(*) AS generated_users, COUNT(s.uid_hash) AS shared_users,
       SAFE_DIVIDE(COUNT(s.uid_hash), COUNT(*)) AS share_rate
FROM gen g LEFT JOIN shared s USING (week, uid_hash)
GROUP BY g.week ORDER BY g.week;
```

### 確認カードの表示と購入(将来・IAP を出した後)

```sql
SELECT
  DATE_TRUNC(DATE(timestamp, 'Asia/Tokyo'), WEEK(MONDAY)) AS week,
  COUNT(DISTINCT IF(jsonPayload.event = 'paywall.shown', jsonPayload.uid_hash, NULL)) AS shown_users,
  COUNTIF(jsonPayload.event = 'paywall.purchased') AS purchases,
  SAFE_DIVIDE(
    COUNT(DISTINCT IF(jsonPayload.event = 'paywall.purchased', jsonPayload.uid_hash, NULL)),
    COUNT(DISTINCT IF(jsonPayload.event = 'paywall.shown', jsonPayload.uid_hash, NULL))) AS conversion
FROM product_metrics.run_googleapis_com_stdout
WHERE jsonPayload.event IN ('paywall.shown', 'paywall.purchased')
GROUP BY week ORDER BY week;
```

## 作業の順番と人の承認

| # | やること | 誰が | 人の承認 |
|---|---|---|---|
| 1 | この設計を決める(下の「決めてほしいこと」) | オーナー | **要る** |
| 2 | PR #472(`ai.usage`)をマージ。`UIDHash` を共通のパッケージに移す | Claude | PR のレビュー |
| 3 | backend に `ai.generate`・`session.start`・`app.opened`・`app.shared`・`app.published`・`share.imported` を足す。本文と生の ID が出ないテストを付ける。#295 の構造化ログの形にそろえる | Claude | PR のレビュー |
| 4 | iOS に `X-Client-Build` ヘッダーを足す(通常 / 審査用)。オプトアウトを置くならその設定も | Claude | PR のレビュー |
| 5 | プライバシーポリシー(1.8・2 章・3 章)、`PrivacyInfo.xcprivacy`、`docs/store/` の文言を直す。#328 と合わせる | Claude(下書き) | **要る**(公開する文言) |
| 6 | BigQuery のデータセットとシンクを作る(`ai-cost.md` の手順と同じ形。保存期間を決めた値に) | 人 | **要る**(GCP の変更) |
| 7 | 共通のビューと週の SQL を BigQuery で流して直す | 人 or Claude(読み取りの権限があれば) | 実行は人の判断 |
| 8 | Looker Studio のレポートを作り、運営だけに共有 | 人 | **要る**(共有の範囲) |
| 9 | App Store Connect の栄養ラベルを直す(5 と同じ内容) | 人 | **要る**(次の審査の提出と同時) |
| 10 | IAP を出す時に `paywall.*` と `POST /v1/metrics` を足す(#314〜#316 の後) | Claude | PR のレビュー |

5 と 9 は、3 の backend を本番に出す**前**か同時に済ませる(ポリシーに書いていない集計を始めない)。

## 決めてほしいこと

| # | 決めること | 推奨 |
|---|---|---|
| 1 | 送り先(A / B / C) | **A**(backend のログ → BigQuery。iOS に分析 SDK を入れない) |
| 2 | `uid_hash` を `ai.usage` と同じ値にするか(費用と継続を結べる / 結べるぶん照合の危険も同じ) | **同じ値にする**。閲覧は運営だけ |
| 3 | 指標用の BigQuery の保存期間(ポリシーは今「ログ 30 日」。PR #472 の手順は 400 日) | **生のイベントは 90 日**、週の集計表(uid を含まない)は無期限。ポリシーに両方を書く。#472 の 400 日もこれに合わせて直す |
| 4 | lint の作り直し(`LintRetried`)を「作り直した」に入れるか | **入れる**(ユーザーの待ち時間が延びるので) |
| 5 | 設定に「利用状況の集計を送らない」の切り替えを置くか | **置かない**(ポリシーへの記載だけ)。β の声で必要なら足す |
| 6 | `app.opened` をヘッダーの有無(`If-None-Match`)で判定するか、iOS に `X-Client-Reason` を足すか | **まずヘッダーの有無**。ショートカットの混ざりが目立ったら足す |
| 7 | テスト用・運営の uid を指標から外す方法 | 除外する `uid_hash` の一覧を BigQuery の表に持つ(**要確認**: 誰が管理するか) |

## 参考

- [App のプライバシーの詳細(Apple)](https://developer.apple.com/app-store/app-privacy-details/)
- [User Privacy and Data Use / ATT(Apple)](https://developer.apple.com/app-store/user-privacy-and-data-use/)
- [プライバシーマニフェストのデータの種類(Apple)](https://developer.apple.com/documentation/bundleresources/describing-data-use-in-privacy-manifests)
- [Firebase: App Store のデータ開示の要件への準備](https://firebase.google.com/docs/ios/app-store-data-collection)
- [Cloud Logging: BigQuery へのルーティング](https://cloud.google.com/logging/docs/export/bigquery)
- [BigQuery の料金](https://cloud.google.com/bigquery/pricing) / [Cloud Logging の料金](https://cloud.google.com/stackdriver/pricing)
- [Looker Studio の BigQuery コネクタ](https://support.google.com/looker-studio/answer/6370296)
- リポジトリ内: `backend/internal/ai/generate.go`(`Meta`・`Error`)、`backend/internal/ai/handler.go`、`backend/internal/appconfig/handler.go`、`backend/internal/httpx/middleware.go`、PR #472 の `backend/internal/ai/usage_log.go` と `docs/infra/ai-cost.md`、`docs/legal/privacy-policy.md`、`ios/Metamon/PrivacyInfo.xcprivacy`、`ios/Metamon/App/BuildConfig.swift`、`ios/Packages/MetamonServices/Sources/MetamonServices/Live/HTTPAppConfigClient.swift`
