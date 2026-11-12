# backend (Go)

単一モジュール・複数バイナリ構成。`cmd/<名前>` が 1 つの実行ファイル(Cloud Run のサービス・Job か、人が回す道具)に対応する。
設計: `docs/backend/architecture.md`、API契約: `docs/backend/openapi-*.yaml`、DSL: `docs/dsl/README.md` + `docs/dsl/dsl.schema.json`。

`cmd/` の全部(1 行 1 本・名前順)。Cloud Run で動く 6 本(ai・appconfig・appstore-reconcile・feedback-loop・purge・runner)は同じ `backend/Dockerfile` のイメージに `/app/<名前>` として入り、`--command` で切り替える。ほかは人が手元で回す道具(smoke はデプロイの後にも回る)。

| cmd | 役割 | どこで動くか | 手元で回す | 詳しい docs |
|---|---|---|---|---|
| `cmd/ai` | 音声/文字 → Claude → DSL(初回フル / 以後 JSON Patch) → appconfig | Cloud Run サービス `ai-stg` / `ai-prod` | `make run-ai`(`make dev` で appconfig と一緒に) | [openapi-ai.yaml](../docs/backend/openapi-ai.yaml) |
| `cmd/ai-audit` | ai-eval の結果の DSL を「動くか」で採点する(押せないボタン・行けない画面など)。API は呼ばない | 手元 | `go run ./cmd/ai-audit` | [ai-eval の監査](../docs/backend/ai-eval/2026-10-24-audit.md) |
| `cmd/ai-eval` | 評価ケース(`internal/ai/eval/cases.json`)を本物の Claude で回して指標と DSL を書き出す。`-check-all` はケースの検査だけ(API なし) | 手元・`backend-ci`(`-check-all` だけ) | `go run ./cmd/ai-eval` | [ci-gate.md](../docs/backend/ai-eval/ci-gate.md) |
| `cmd/ai-session-report` | 指定したユーザーの AI とのやり取りをセッションに切り、AI が外した疑いの強い順に並べる(読むだけ) | 手元(人が回す・定期では回さない) | `go run ./cmd/ai-session-report` | [ai-session-report.md](../docs/ops/ai-session-report.md) |
| `cmd/appconfig` | アプリ DSL JSON の CRUD・JSON Patch 適用・バージョン・汎用レコード・各機能の API | Cloud Run サービス `appconfig-stg` / `appconfig-prod` | `make run-appconfig`(`make dev` で ai と一緒に) | [openapi-appconfig.yaml](../docs/backend/openapi-appconfig.yaml) |
| `cmd/appstore-reconcile` | App Store Server API から Pro の状態を取り直し、届かなかった通知の分を直す | Cloud Run Job `appstore-reconcile`(Cloud Scheduler で 1 日に数回・作るのは人) | `make run-appstore-reconcile`(エミュレータ・既定 `-dry-run`) | [app-store-notifications.md](../docs/infra/app-store-notifications.md) |
| `cmd/coin-initial-backfill` | 初回コインの付与の記録(`coin_initial_grants`)を、記録が無かった頃のアカウントに書く | 手元(人が 1 回だけ) | `go run ./cmd/coin-initial-backfill` | [coin-initial-backfill.md](../docs/infra/coin-initial-backfill.md) |
| `cmd/dslgen` | JSON Schema から Android(Kotlin)の DSL の型を作る。生成物は commit し、古いと `TestGeneratedKotlinIsUpToDate` が落ちる | 手元 | `make dsl-gen` | [351-dsl-cross-platform.md](../docs/decisions/351-dsl-cross-platform.md) |
| `cmd/feedback` | アプリ内のフィードバックを読む(一覧・1 件)。書かない | 手元(運用する人) | `make run-feedback` | [feedback.md](../docs/backend/features/feedback.md) |
| `cmd/feedback-loop` | 自己改善ループの各段(取り込み・振り分け・Issue の下書きと人の承認・直ったお知らせ・止める仕組み) | Cloud Run Job `feedback-loop-<段>`(Cloud Scheduler・既定は作らない)と手元 | `make run-feedback-loop`(エミュレータ・`LOOP_ARGS`) | [feedback-loop.md](../docs/backend/features/feedback-loop.md) |
| `cmd/matching-age-backfill` | 18 歳以上の申告をした作者のマッチングのプロフィールに `ownerAgeConfirmed` を書く(既定は件数だけ) | 手元(人が 1 回だけ) | `go run ./cmd/matching-age-backfill` | [matching-age-backfill.md](../docs/infra/matching-age-backfill.md) |
| `cmd/moderate` | 通報の一覧・処理と、掲載・共有アプリの公開の停止(運用する人の道具) | 手元(運用する人) | `make run-moderate` | [moderation.md](../docs/infra/moderation.md) |
| `cmd/product-metrics` | ログの出力からプロダクトの指標(生成の成功・D1/D7 など)を週ごとの Markdown の表にする。API は呼ばない | 手元 | `go run ./cmd/product-metrics` | [product-metrics.md](../docs/metrics/product-metrics.md) |
| `cmd/purge` | 退会して一定期間たったアカウントと、古いフィードバックを消す | Cloud Run Job `account-purge`(Cloud Scheduler で毎日・`infra/purge.tf`) | `make run-purge` | [account-purge.md](../docs/infra/account-purge.md) |
| `cmd/runner` | 自動の動き(serverRules)を outbox から実行する(#341)。`POST /internal/tick` | Cloud Run サービス `runner-prod`(prod だけ・非公開。`RUNNER_DEPLOY=1` と `runner_enabled = true` の時だけ出す)と Scheduler `runner-tick` | `make run-runner` | [runner.md](../docs/infra/runner.md) |
| `cmd/smoke` | デモの発話を動いている ai(+ appconfig)に流して、ステップごとの時間とトークンを表にする | 手元・`deploy-cloudrun` のデプロイの後(非ブロッキング) | `make smoke-local` / `make smoke-stg` / `make smoke-prod` | [deploy-cloudrun](../docs/02_github_workflow.md#deploy-cloudrunbackend-の自動デプロイ) |
| `cmd/stg-cleanup` | stg のテストのデータ(smoke のアプリ・回数制限のカウンタ・指定したテストユーザー)を消す(既定は件数だけ) | 手元(人が回す・定期では回さない) | `make run-stg-cleanup` | [stg-cleanup.md](../docs/infra/stg-cleanup.md) |

共有型は `pkg/dsl`:

| API | 用途 |
|---|---|
| `dsl.Parse(raw) (*App, error)` | JSON Schema + 意味検証。全書き込みの入口 |
| `dsl.ApplyPatch(current, ops) (next, *App, error)` | RFC 6902 を適用して `Parse`。appconfig の `PATCH /dsl` と ai の事前検証が同じ関数を使う |
| `dsl.SchemaJSON()` / `dsl.PatchSchemaJSON()` | Claude 構造化出力に渡すスキーマ(初回 / 編集) |
| `dsl.Initial()` | 新規ユーザーの初期 DSL(音声画面のみ) |
| `*ValidationError.Problems` | 失敗理由の全列挙(422 の details / AI 再生成プロンプトに流用) |

## パッケージ構成(internal)

`internal/` の全部(1 行 1 つ・名前順)。サブパッケージは親の行のすぐ下に置く。テスト用のデータだけのフォルダ(`testdata`・`ai/eval`・`ai/examples`)は載せない。`internal/moderation` は中が `textfilter` だけなので、その行だけを置く。

| パッケージ | 役割 | 詳しい docs |
|---|---|---|
| `internal/ai` | 発話 → DSL の変更(初回はフルの AppDSL・以後 JSON Patch)。`pkg/dsl` で検証し、問題があれば回数を決めて作り直す | [openapi-ai.yaml](../docs/backend/openapi-ai.yaml) |
| `internal/appcheck` | Firebase App Check のトークンを確かめる(`APP_CHECK_MODE` で扱いを決める) | [app-check.md](../docs/infra/app-check.md) |
| `internal/appconfig` | `openapi-appconfig.yaml` のハンドラ(core は apps / dsl / versions / entities / records。各機能のルートは機能のファイルから足す) | [openapi-appconfig.yaml](../docs/backend/openapi-appconfig.yaml) |
| `internal/appconfigclient` | ai が appconfig を叩く薄いクライアント(HTTP は Bearer 転送・Memory はプロセス内の代わり) | - |
| `internal/appstore` | StoreKit 2 の署名付き取引(JWS)を Apple を呼ばずに手元で確かめる | [app-store-notifications.md](../docs/infra/app-store-notifications.md) |
| `internal/appstore/appstoretest` | テスト用: 偽の CA で署名した StoreKit 形式の JWS を作る | - |
| `internal/auth` | `Middleware(verifier, mode)` + `UID(ctx)`。verifier は `FirebaseVerifier`(Admin SDK)/ `DevVerifier`(dev 専用・Bearer 値=uid) | - |
| `internal/books` | DSL の `lookupBook` のため、ISBN で本を引く(決まった 1 つの取得先) | [book-lookup.md](../docs/backend/features/book-lookup.md) |
| `internal/budget` | その日の AI の費用がしきい値を超えたら、無料プランの AI をその日の残りだけ止める(`AI_FREE_DAILY_BUDGET_USD` が無ければ off) | [rate-limit.md](../docs/backend/rate-limit.md) |
| `internal/calls` | 共有アプリの音声・ビデオ通話。ルームのトークンを出し、使った分数を数える | [calls.md](../docs/backend/features/calls.md) |
| `internal/coins` | コインの財布と台帳。LLM の呼び出しは先に上限を押さえ、終わったら確定する | [630-token-billing.md](../docs/decisions/630-token-billing.md) |
| `internal/coins/coinsapi` | コインの財布の appconfig の HTTP API | - |
| `internal/entitlement` | Tailor It Pro のサーバー側の記録(購入・復元の取引を受けて持つ・照合) | [pro.md](../docs/backend/features/pro.md) |
| `internal/extapi` | runner から外部 API(カレンダー・天気・翻訳など)を呼ぶ入口 | [external-apis.md](../docs/backend/features/external-apis.md) |
| `internal/feeds` | DSL の feed のため、決まった取得先の一覧から公開データを読む | [external-feeds.md](../docs/backend/features/external-feeds.md) |
| `internal/fsenv` | どの Firestore の DB に繋ぐかを決める 1 か所。stg が本番の DB を使うなら起動を止める | - |
| `internal/googleplay` | Google Play の定期購入(Android の Pro)を Developer API で読む | [play-billing.md](../docs/android/play-billing.md) |
| `internal/gtfsstatic` | 静的な GTFS-JP の zip から、バスに要る部分(停留所・路線の名前・各便の予定時刻)を読む | [external-apis.md](../docs/backend/features/external-apis.md) |
| `internal/httpx` | JSON 書き出し・エラー封筒(`WriteJSON` / `Error` / `ErrorWithDetails` / `DecodeBody`)・エラーのコードの一覧・RequestID・Log・Recover・`/health` | - |
| `internal/loop` | 自己改善ループの土台(受け箱・止める仕組み・保存)。各段はサブパッケージ | [feedback-loop.md](../docs/backend/features/feedback-loop.md) |
| `internal/loop/draft` | 振り分けたまとまりを Issue の下書きにし、人の確認を待つ | [feedback-loop.md](../docs/backend/features/feedback-loop.md) |
| `internal/loop/fixed` | 直った Issue のもとになった声の送り主に「あなたの声で直りました」を届ける | [feedback-loop.md](../docs/backend/features/feedback-loop.md) |
| `internal/loop/github` | ループ用の小さな GitHub クライアント(人が承認した Issue を作る・閉じられ方を読む) | [feedback-loop.md](../docs/backend/features/feedback-loop.md) |
| `internal/loop/ingest` | 各取得先のフィードバックを共通の形にし、伏せてから受け箱に写す | [feedback-loop.md](../docs/backend/features/feedback-loop.md) |
| `internal/loop/pii` | 保存の前にフィードバックの個人情報を伏せる(LLM は使わない) | [feedback-loop.md](../docs/backend/features/feedback-loop.md) |
| `internal/loop/triage` | 受け箱を LLM で分類・まとめ・優先度付けする | [feedback-loop-triage-agreement.md](../docs/backend/features/feedback-loop-triage-agreement.md) |
| `internal/loop/triage/eval` | 振り分けを人のラベル付きの組と比べて一致率を測る | [feedback-loop-triage-agreement.md](../docs/backend/features/feedback-loop-triage-agreement.md) |
| `internal/metrics` | プロダクトの指標のイベントを構造化ログに書く(入力の文や DSL の中身は書かない) | [product-metrics.md](../docs/metrics/product-metrics.md) |
| `internal/moderation/textfilter` | ほかの人が見る文の明らかに不適切な語を断る(最小の語の一覧) | [moderation.md](../docs/infra/moderation.md) |
| `internal/paid` | Pro の機能の一覧(`paid_features.json`)と、それを使うサーバー側の DSL の検査 | [pro.md](../docs/backend/features/pro.md) |
| `internal/plan` | プランごとに使える量の表(無料と Pro) | [rate-limit.md](../docs/backend/rate-limit.md) |
| `internal/ratelimit` | 1 人(と全体)が 1 日に起こせる Claude の仕事の量を制限する | [rate-limit.md](../docs/backend/rate-limit.md) |
| `internal/runner` | 自動の動き(serverRules)の実行。outbox からきっかけを取り、実行の直前に動かしてよいか確かめて、engine で評価する | [runner.md](../docs/infra/runner.md) |
| `internal/runner/engine` | 1 つのルールを 1 つのきっかけに当てて、することを返す(自分では I/O しない) | [automations.md](../docs/backend/features/automations.md) |
| `internal/runner/notify` | 予定した通知の宛先を決めて送る | [notifications.md](../docs/backend/features/notifications.md) |
| `internal/runner/notify/apns` | APNs へ送る(トークン認証) | [runner.md](../docs/infra/runner.md) |
| `internal/runner/notify/fcm` | FCM HTTP v1 で Android へ送る | [runner.md](../docs/infra/runner.md) |
| `internal/runner/queue` | outbox の仕事を取る・終える・間を空けて再試行する・上限で諦める | [runner.md](../docs/infra/runner.md) |
| `internal/runner/quota` | runner の実行・書いたレコード・通知の宛先・時刻のルールの数を数えて制限する | [runner-usage.md](../docs/infra/runner-usage.md) |
| `internal/runner/schedule` | 時刻がきっかけのルールがいつ動くかを計算する(I/O なし) | [automations.md](../docs/backend/features/automations.md) |
| `internal/runner/scope` | 1 つのきっかけから作り、その 1 つのアプリだけに触れる runner の取っ手 | [runner.md](../docs/infra/runner.md) |
| `internal/runner/status` | runner の結果を作成者の画面に出す決まった言葉にする(appconfig からも使う) | [automations.md](../docs/backend/features/automations.md) |
| `internal/store` | `Store` interface(`users/{uid}/apps/{appId}` の下のアプリ・DSL の版・レコード)。`Firestore`(本番・エミュレータ)と `Memory`(テスト・ローカル) | - |

機能(共有・メンバー・通報など)のルートは、その機能のファイルの `init()` から `registerRoutes` で足す(AI のプロンプトと同じ形)。`handler.go` の `New()` には core(apps / dsl / versions / entities / records)だけを置く。足したルートは `routes_test.go` の `wantRoutes` にも書く。

並行 PR の衝突を減らす書き方(#516): slog の属性は 1 行に 1 つ。purge の新しい件数は `store.AccountPurgeResult` のフィールド(コメントは行の上)と `LogAttrs` に足す。`store.Memory` の新しい状態は空行で区切った自分の機能のブロックに置く(gofmt の桁揃えがほかの行に波及しない)。

middleware の組み立ては `httpx.Chain` で 1 行に 1 つ(外側から順、理由のコメントは行の上)。順番はテストで固定している(`cmd/ai`・`cmd/appconfig` の `main_test.go`)。

## 起動

```sh
cd backend
make build vet test      # CI と同じ(golangci-lint run ./... も通す)
make emulator            # 別ターミナル: Firestore :8089(gcloud・CI と同じ。Auth エミュレータは無し)
make test-emulator       # エミュレータ試験(CI の backend-emulator と同じ判定。無ければ一時起動して止める)
make dev                 # appconfig :8080 / ai :8081 (AUTH_MODE=dev, X-Debug-Uid で叩ける)
make schema-sync         # docs/dsl/dsl.schema.json → pkg/dsl/schema.json (スキーマ変更時)

# エミュレータ無しで最速起動(インメモリ store・Bearer 値をそのまま uid として扱う)
STORE=memory AUTH_MODE=dev PORT=8080 go run ./cmd/appconfig
export DEV_UID=dev-smoke
curl -H "Authorization: Bearer $DEV_UID" localhost:8080/v1/apps/my-device/dsl   # 未知 appId は version 1 を自動作成
```

運用コマンド(`run-purge` などの機能ごとのターゲット)は `mk/<feature>.mk` に置く(Makefile が末尾で `-include` する)。
新しく足す時は `mk/<feature>.mk` を新規に作り、ターゲットの直上に専用の `.PHONY: <target>` を書く。Makefile の共有行には足さない。

ai → appconfig の結合をローカルで確認する時は、appconfig を上記で起動し、ai を
`AUTH_MODE=dev APPCONFIG_BASE_URL=http://localhost:8080 ANTHROPIC_API_KEY=... PORT=8082 go run ./cmd/ai` で起動して
`curl -H "Authorization: Bearer $DEV_UID" -d '{"appId":"my-device","transcript":"家計簿が欲しい"}' localhost:8082/v1/generate`
を叩く(ai は Authorization をそのまま転送する)。

## 環境変数

| 変数 | appconfig | ai | 既定 / 備考 |
|---|---|---|---|
| `PORT` | o | o | Cloud Run が注入。ローカル 8080 / 8081 |
| `GOOGLE_CLOUD_PROJECT` | o | o | Firestore と Firebase Admin の初期化。**appconfig は未設定ならインメモリ store** |
| `AUTH_MODE` | o | o | `firebase`(既定・本番)/ `dev`。dev では Bearer 値を uid として受理(`DevVerifier`)、`FIREBASE_AUTH_EMULATOR_HOST` も設定時は `X-Debug-Uid` も可 |
| `FIRESTORE_DATABASE` | o | o | Firestore の DB ID。既定 `(default)`。全バイナリ(appconfig / ai の回数制限 / cmd/purge / cmd/stg-cleanup)が `internal/fsenv` から読む(#290) |
| `APP_ENV` | o | o | `stg` / `prod`。`stg` が本番の接続先(`PROD_FIRESTORE_PROJECT`/`PROD_FIRESTORE_DATABASE`、既定 `jphacks-kz2601`/`(default)`)を使うと**起動を止める**(エミュレータ時は対象外) |
| `ALLOW_SHARED_PROD_DB` | o | o | `1` のときだけ上の停止を外す(警告ログは出る)。stg が本番と DB を共有している間(#291・#293 の分離まで)は stg に必須。deploy-cloudrun.yml が stg=`1` / prod=`0` を入れる |
| `STORE` | o | - | `memory` でインメモリ store を強制(既定は `GOOGLE_CLOUD_PROJECT` があれば Firestore) |
| `FIRESTORE_EMULATOR_HOST` | o | - | 設定時は Firestore クライアントが自動でエミュレータへ |
| `FIREBASE_AUTH_EMULATOR_HOST` | o | o | ローカルのみ(`X-Debug-Uid` の有効条件) |
| `LOG_LEVEL` | o | o | `info` |
| `FEEDBACK_DAILY_LIMIT` | o | - | `POST /v1/feedback` の 1 uid 1 日(JST)の件数(既定 5・0 で無制限・#319) |
| `APPCONFIG_BASE_URL` | - | o | ai → appconfig。未設定なら ai 内のインメモリ stub(永続なし)。staging は `https://appconfig-stg-378953239426.asia-northeast1.run.app` |
| `ANTHROPIC_API_KEY` | - | o | Secret Manager。ローカルは `~/Desktop/JPHACKS2026/.env`(リポ外) |
| `APPSTORE_BUNDLE_IDS` / `APPSTORE_PRODUCT_IDS` | o | - | StoreKit 取引の許可リスト(#314)。既定 `jp.kz2601.metamon` / `jp.kz2601.metamon.pro.monthly` |
| `APPSTORE_ENVIRONMENTS` | o | o | 受け入れる App Store の環境。既定 `Sandbox,Production`(審査・TestFlight は Sandbox)。ai は Pro の判定(#449)に使う |

Cloud Run 上の appconfig は `AUTH_MODE` 未設定(= `firebase`)で、`run-backend` SA の ADC で Firestore と Firebase Admin を使う。
