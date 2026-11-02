# プロダクトの指標(#297)

「作りたいアプリが作れたか」「使い続けているか」を週ごとに見るための、指標の定義・イベントの一覧・週の数字の出し方です。
設計の検討(選択肢・根拠・BigQuery の SQL の下書き)は [`docs/decisions/297-product-metrics.md`](../decisions/297-product-metrics.md)。ここは**実装した内容**と、残りの人の判断をまとめます。

> **iPhone から新しく送るデータはありません。** 指標はすべて、backend(ai / appconfig)が今も受けているリクエストから、サーバー側のログで出します。
> 端末でしか分からないもの(Pro の確認カードの表示など)は作っていません。

## 目次

1. [指標の定義](#1-指標の定義)
2. [イベントの一覧](#2-イベントの一覧)
3. [週の数字の出し方](#3-週の数字の出し方)
4. [送り先の選択肢](#4-送り先の選択肢)
5. [ダッシュボードを作る手順(人がやる)](#5-ダッシュボードを作る手順人がやる)
6. [プライバシーと同意への影響](#6-プライバシーと同意への影響)
7. [わかっている限界](#7-わかっている限界)
8. [人が決めること](#8-人が決めること)

## 1. 指標の定義

- **週**は月曜 0:00 JST 始まり。日の区切りも JST。
- **ユーザー**は `uid_hash`(uid の擬名。`ai.usage` と同じ値)。匿名ログインも 1 人。`uid_hash` の無い行は数えない。
- **作れた**は `ai.generate` の mode が `create` か `edit` で、outcome が `validated`(1 回で通過)か `regenerated`(作り直して通過)。

| 指標 | 式 | 使うイベント | backend だけで出せるか |
|---|---|---|---|
| 生成の成功率 | (validated + regenerated + unchanged)÷ 生成。生成 = `ai.generate` の create と edit(`bad_request` と undo を除く)。内訳として validated・regenerated(作り直した)・unchanged・refused(断った)・failed の割合も出す | `ai.generate` | **出せる** |
| 直しの回数 | 週ごとに、アプリ(`app_hash`)ごとの「edit で作れた」回数。直したアプリの数・合計・平均・中央値。`unchanged` は入れない | `ai.generate`(`app_hash`) | **出せる**(`app_hash` はこの変更で追加) |
| 初回の生成までの時間 | ユーザーが最初に見えた時刻(どれかのイベント)から、最初に create で作れた時刻まで(分)。最初に見えた週ごとに p50・p90 と「作れた率」 | `session.start`・`ai.generate` ほか | **出せる**(ただし「アカウントを作った時刻」ではなく「最初に見えた時刻」。7 章) |
| 翌日・7 日後に開いた率(D1 / D7) | 最初に作れた日を 0 日目とし、1 日目 / 7 日目にどれかのイベント(一覧を開く・アプリを開く・生成・共有など)があったユーザーの割合。0 日目の週ごとのコホート。その日がまだデータに無いユーザーは分母に入れない | 全部 | **出せる** |
| 共有した率 | その週に作れたユーザーのうち、同じ週に共有リンクを作ったユーザーの割合。共有リンク・取り込み・「探す」への掲載の件数も出す | `ai.generate`・`app.shared`・`share.imported`・`app.published` | **出せる**(審査用の版には共有が無いので、版を分けるには 8 章の判断が要る) |
| Pro の確認カードの表示と購入 | 表示したユーザー数・購入数・その割合 | `paywall.shown`・`paywall.purchased`(未実装) | **出せない**。端末からの送信が要る。App 内課金を出すまで作らない。購入数だけなら App Store Connect の売上レポートで見られる |

## 2. イベントの一覧

どれも slog の JSON 1 行で、`event` 属性(とメッセージ)がイベント名。Cloud Logging では `jsonPayload.event` で絞れる。

**どのイベントにも入れないもの**: 話した内容・会話・要約、DSL の中身(アプリ名・項目名など)、記録の値、生の uid・生の appId・共有トークン、IP アドレス、端末の ID・広告 ID。テストで「生の ID・名前・DSL が出ない」ことを確かめている(`backend/internal/appconfig/metrics_test.go`・`backend/internal/ai/generate_log_test.go`・`backend/internal/metrics/metrics_test.go`)。

| 名前 | いつ | 属性 | どこで |
|---|---|---|---|
| `ai.generate` | `POST /v1/generate` が 1 回終わるたび(#322 で追加済み) | mode・outcome・validation_passed・first_generation・stream・duration_ms・attempts・regenerations・area_fallback・downgraded_to_full・lint_retried・claude_ms・error_code・`uid_hash`・**`app_hash`(この変更で追加)** | ai: `internal/ai/generate_log.go` |
| `session.start` | `GET /v1/apps`(アプリの一覧)の 1 ページ目が成功した時 | `uid_hash` | appconfig: `internal/appconfig/apps.go` の `listApps` |
| `app.opened` | `GET /v1/apps/{appId}/dsl` を `If-None-Match` **なし**で受けて 200 を返した時(バックグラウンドの取り直しは `If-None-Match` を付けるので数えない) | `app_hash`・`uid_hash` | appconfig: `internal/appconfig/dsl.go` の `getDSL` |
| `app.shared` | `POST /v1/apps/{appId}/share` が成功した時 | `app_hash`・`uid_hash` | appconfig: `share.go` の `createShare` |
| `app.published` | `POST /v1/apps/{appId}/publish` が成功した時 | `app_hash`・`uid_hash` | appconfig: `share.go` の `publishApp` |
| `share.imported` | 共有リンクからの取り込み・「探す」からの追加でコピーを作った時(「探す」の入れ直しは数えない) | `app_hash`(入った先)・source(`share|public`)・joined・created・`uid_hash` | appconfig: `share.go` の `importShare`・`installPublicApp` |
| `paywall.shown` / `paywall.purchased` | (未実装・将来)確認カードの表示・購入 | feature / product | iOS からの送信が要る(人の判断) |

- `uid_hash` = `sha256("ai-usage:" + uid)` の先頭 8 バイト(16 桁)。`ai.usage`(#298)と同じ値で、1 人あたりの費用と継続を結べる。
- `app_hash` = `sha256("metrics-app:" + appId)` の先頭 8 バイト。共有のデータと直接は結べない。
- 塩なしなので、uid / appId の一覧を持つ人は照合できる(`ai.usage` と同じ限界)。ログの閲覧は運営だけにする。
- 共通の関数は `backend/internal/metrics`(`Emit`・`UIDHash`・`AppHash`)。PR #494(構造化ログ)が入ったら `httpx.UIDHash` と 1 つにまとめる。

## 3. 週の数字の出し方

```sh
scripts/product-metrics-weekly.sh prod             # 直近 28 日を Cloud Logging から読む(読み取りだけ)
scripts/product-metrics-weekly.sh stg --days 14
scripts/product-metrics-weekly.sh prod --dry-run   # フィルタだけ出す(Logs Explorer に貼れる)
scripts/product-metrics-weekly.sh --input backend/cmd/product-metrics/testdata/logs.json   # 手元の書き出しを集計
```

- 要る権限: プロジェクトの `roles/logging.viewer` だけ(`gcloud auth login` 済み)。書き込みはしない。
- 集計は `backend/cmd/product-metrics`(Go)。入力は `gcloud logging read --format=json` の配列か、JSON lines(ローカルの `make dev` のログもそのまま読める)。
- 出力は 5 つの Markdown の表(生成の成功率・直しの回数・初回の生成までの時間・D1/D7・共有した率)。`* ` の付いた週はデータの始まりの週で、それ以前からいるユーザーが「新しい」に混ざるので参考にとどめる。
- 週の記録は β の品質ゲートと同じく `docs/release/beta-weekly/YYYY-MM-DD.md` に写す(`docs/release/beta-quality-gate.md`)。
- Cloud Logging の既定の保存は 30 日なので、D7 は直近 3 週ぶんのコホートまでしか出ない。長く見るには 5 章のシンクが要る。

fixture(`backend/cmd/product-metrics/testdata/logs.json`)は 3 人・3 週ぶんで、出力は `weekly.golden.md` と一致することをテストしている。例えば 2026-10-19 の週は、生成 7 件(1 回で通過 3・作り直し 2・変更なし 1・断った 1)で成功率 85.7%、D1 は 2 人中 1 人で 50%、D7 は 2 人とも開いて 100%。

## 4. 送り先の選択肢

| 選択肢 | 意味 | iPhone からの新しい送信 | 長所 | 短所 |
|---|---|---|---|---|
| **A. backend のログ → Cloud Logging(今)→ BigQuery のシンク(推奨)** | 今のイベントを `scripts/product-metrics-weekly.sh` で週ごとに見る。長く残す・ダッシュボードにする時はシンクで BigQuery に流し、Looker Studio で見る | なし | 端末に SDK が増えない。費用はほぼ 0。今日から数字が出る | 端末でしか分からない操作は見えない |
| B. Firebase Analytics(GA4) | iOS から `logEvent` で送る | あり | 画面の滞在・離脱まで見える | SDK の自動収集・栄養ラベルの増加・同意の見直しが要る |
| C. Firestore に集計を書く | 週の集計を Firestore の文書に足していく | なし | アプリ内の管理画面から読める | 書き込みの費用と競合。分析のクエリに向かない。本番のデータベースに運営用のデータが混ざる |

**推奨は A。** 今の指標はすべてサーバーで分かり、Cloud Logging だけで週の表が出る。ダッシュボードが要る時に BigQuery のシンクを足す(5 章・人の承認)。B は Pro の確認カードなど端末の操作を見たくなった時に再検討する。

## 5. ダッシュボードを作る手順(人がやる)

Issue の「週ごとに見られるダッシュボード」を Looker Studio で作る手順。**GCP の設定を変えるので、人の承認の後に人がやる**(エージェントはやらない)。保存期間は 8 章の判断で決める。

1. データセットを作る(東京・テーブルの期限は決めた日数):
   `bq --location=asia-northeast1 mk --dataset --default_table_expiration <秒> <PROJECT_ID>:product_metrics`
2. シンクを作る(ai と appconfig の両方、指標のイベントだけ):
   `gcloud logging sinks create product-metrics bigquery.googleapis.com/projects/<PROJECT_ID>/datasets/product_metrics --use-partitioned-tables --log-filter='resource.type="cloud_run_revision" AND jsonPayload.event=("ai.generate" OR "session.start" OR "app.opened" OR "app.shared" OR "app.published" OR "share.imported")'`
3. 出力に出たシンクのサービスアカウントに、そのデータセットだけの `roles/bigquery.dataEditor` を付ける。
4. `docs/decisions/297-product-metrics.md` の「ダッシュボードと週の SQL」のビューを、シンクが作ったテーブル名・列名に合わせて作る。定義はこのページの 1 章(`backend/cmd/product-metrics` と同じ式)に合わせる。
5. Looker Studio で BigQuery のビューをデータソースにし、1 枚のレポートを作る(上段に今週の数字、下段に週ごとの折れ線)。共有は運営だけ。

シンクを作るまでは、3 章のスクリプトの表が週の数字になる。

## 6. プライバシーと同意への影響

- **ATT は要らない。** 広告 ID を使わず、他社のデータとつながず、端末から新しい送信も無い。`PrivacyInfo.xcprivacy` の `NSPrivacyTracking=false` のまま。
- **同意のダイアログは足さない**(推奨)。AI への送信の同意(`AIConsentView`)とは別物で、話した内容を含まない運営のための集計。プライバシーポリシーでの公表で足りる見込み(法的な確認は人の判断)。
- **直す必要がある表示**(公開する文言なので人の判断。この PR では変えていない):
  - `docs/legal/privacy-policy.md` の「行動の分析ツール(Firebase Analytics 等)は使いません」(1.8)。サーバーのログで利用の回数・成功/失敗を、話した内容を含めずに集計する旨に書き換える。BigQuery に残すならその保存期間も 3 章(ログ 30 日)に足す。
  - App Store Connect の栄養ラベル: 「使用状況データ > 製品の操作」を足す(用途: アナリティクス・本人に紐づく・トラッキングなし)。既存の「ユーザー ID」の用途に「アナリティクス」を足す。
  - `ios/Metamon/PrivacyInfo.xcprivacy`: 上と同じ内容(`NSPrivacyCollectedDataTypeProductInteraction` など)と、コメントの「集めないもの: 分析」。#328 と同じ PR で直すのがよい。
- 栄養ラベルとポリシーは、このイベントが本番(`main`)に出る**前か同時に**直すのが安全(ポリシーに書いていない集計をしない)。

## 7. わかっている限界

- 「初回の生成までの時間」の起点は**最初に見えた時刻**で、Firebase のアカウントを作った時刻ではない。サーバーはアカウント作成を知らない。データの始まりより前からいるユーザーも「新しい」に見える(* の週)。
- `session.start` は iOS が起動のたびに `GET /v1/apps` を呼ぶ前提。呼ぶ場所が増減すると数が変わる。D1 / D7 はどのイベントでも「使った」と数えるので影響は小さい。
- `app.opened` は、ショートカット(`If-None-Match` なしで呼ぶ)も数える。
- `ai.generate` の `uid_hash` は、ai サービスで uid を確かめた時だけ付く(Cloud Run ではレート制限が確かめる)。ローカルでは付かないので集計に入らない。
- 運営・テストの uid を外す仕組みはまだ無い(8 章)。
- 審査用の版と通常の版を分けられない(版を示すヘッダーは iOS の変更が要る。8 章)。
- 1 回の読み出しは `LIMIT`(既定 20 万行)まで。超えたら警告が出る。

## 8. 人が決めること

| # | 決めること | 推奨 |
|---|---|---|
| 1 | 送り先(A / B / C) | **A**。端末に SDK を入れない |
| 2 | プライバシーポリシー・栄養ラベル・マニフェストを直すか、いつ直すか(6 章) | このイベントを `main` に出す前に直す。#328 とまとめる |
| 3 | BigQuery のシンクとダッシュボードを作るか、保存期間(5 章) | 作る。生のイベントは 90 日、uid を含まない週の集計は無期限 |
| 4 | D1 / D7 の「開いた」をどのイベントで数えるか | 今はどのイベントでも可。持ち主のアプリを開いた時だけに絞るなら `app.opened` に役割を足す |
| 5 | 版(通常 / 審査用)を区別するヘッダーを iOS に足すか | 共有した率を通常の版だけで見たくなったら足す(iPhone の送信が増えるので人の判断) |
| 6 | 運営・テストの uid を外す方法 | 除外する `uid_hash` の一覧を持ち、集計で外す。誰が管理するかを決める |
| 7 | Pro の確認カードの表示を端末から送るか | App 内課金を出す時に決める(PR #499 の「既定でオンにするか」と同じ種類の判断) |
