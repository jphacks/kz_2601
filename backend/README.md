# backend (Go)

単一モジュール・複数サービス構成。`cmd/<service>` が1つの Cloud Run サービスに対応する。
設計: `docs/backend/architecture.md`、API契約: `docs/backend/openapi-*.yaml`、DSL: `docs/dsl/README.md` + `docs/dsl/dsl.schema.json`。

| cmd | 担当 | 役割 |
|---|---|---|
| `cmd/appconfig` | @2410as | アプリ DSL JSON の CRUD・JSON Patch 適用・バージョン・汎用レコード |
| `cmd/ai` | Claude Code | 音声/文字 → Claude → DSL(初回フル / 以後 JSON Patch) → appconfig |
| `cmd/runner` | Claude Code | 自動の動き(serverRules)を outbox から実行(#341・**未デプロイ**)。`POST /internal/tick`・ローカルは `make run-runner` |

共有型は `pkg/dsl`:

| API | 用途 |
|---|---|
| `dsl.Parse(raw) (*App, error)` | JSON Schema + 意味検証。全書き込みの入口 |
| `dsl.ApplyPatch(current, ops) (next, *App, error)` | RFC 6902 を適用して `Parse`。appconfig の `PATCH /dsl` と ai の事前検証が同じ関数を使う |
| `dsl.SchemaJSON()` / `dsl.PatchSchemaJSON()` | Claude 構造化出力に渡すスキーマ(初回 / 編集) |
| `dsl.Initial()` | 新規ユーザーの初期 DSL(音声画面のみ) |
| `*ValidationError.Problems` | 失敗理由の全列挙(422 の details / AI 再生成プロンプトに流用) |

## パッケージ構成(appconfig)

| パッケージ | 役割 |
|---|---|
| `internal/httpx` | JSON 書き出し・エラー封筒(`WriteJSON` / `Error` / `ErrorWithDetails` / `DecodeBody`)・RequestID・Log・Recover・`ErrorEnvelope` |
| `internal/auth` | `Middleware(verifier, mode)` + `UID(ctx)`。verifier は `FirebaseVerifier`(Admin SDK)/ `DevVerifier`(dev 専用・Bearer 値=uid) |
| `internal/store` | `Store` interface。`Firestore`(本番・エミュレータ)と `Memory`(テスト・ローカル) |
| `internal/appconfig` | `openapi-appconfig.yaml` のハンドラ(apps / dsl / versions / entities / records) |
| `internal/appconfigclient` | ai が appconfig を叩く薄い HTTP クライアント(Bearer 転送) |

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

### make のターゲット

`backend/` で `make <ターゲット>`(外からは `make -C backend <ターゲット>`)。ターゲットを足す PR では、この表にも 1 行足す(名前順)。「エミュレータ」は `localhost:8089` の Firestore エミュレータ(`make emulator`)に向くもの。

| ターゲット | 何をするか | stg / prod に触れるか | dry-run が既定か | 置き場 |
|---|---|---|---|---|
| `build` | `go build ./...` | 触れない | — | `Makefile` |
| `demo-check` | デモ前チェック兼ウォームアップ(`scripts/demo-check.sh $(ENV)`)。匿名ユーザーを作り、生成・対話・編集で Claude を 3 回呼び、最後に作った物を片付ける | 触れる(`ENV=stg` が既定・`ENV=prod` で本番) | いいえ | `Makefile` |
| `dev` | `run-appconfig`(:8080)と `run-ai`(:8081)を一緒に起動する。Ctrl-C で両方止まる | 触れない(エミュレータ) | — | `Makefile` |
| `dsl-gen` | `schema-check` の後、Android の DSL の型をスキーマから作り直す(`go run ./cmd/dslgen`) | 触れない | — | `Makefile` |
| `emulator` | Firestore エミュレータを起動する(`scripts/firestore-emulator.sh start`)。Auth エミュレータは無し | 触れない | — | `Makefile` |
| `lint` | `vet` の後、`staticcheck ./...`(入っていなければ入れ方を出すだけ)。CI の `golangci-lint run ./...` とは別 | 触れない | — | `Makefile` |
| `run-ai` | ai を `AUTH_MODE=dev`・:8081 で起動し、appconfig は `localhost:8080` を見る | 触れない | — | `Makefile` |
| `run-appconfig` | appconfig をエミュレータに向けて `AUTH_MODE=dev`・:8080 で起動する | 触れない(エミュレータ) | — | `Makefile` |
| `run-appstore-reconcile` | App Store Server API で Pro の状態を取り直す照合(#315)をエミュレータに対して回す(`APPSTORE_RECONCILE_FLAGS`) | 触れない(エミュレータ・App Store は sandbox のキーを自分で渡す) | はい(`-dry-run`) | `mk/appstore.mk` |
| `run-feedback` | アプリ内のフィードバック(#319)を読む(`FEEDBACK_ARGS`・既定 `list`) | 触れない(エミュレータ) | 読むだけ | `mk/feedback.mk` |
| `run-feedback-loop` | 自己改善ループ(#364)のコマンドを回す(`LOOP_ARGS`・既定 `ingest`)。[feedback-loop.md](../docs/backend/features/feedback-loop.md) | 触れない(エミュレータ) | dry-run の口は無い。段は `LOOP_<段>=on` の時だけ動く | `mk/feedback-loop.mk` |
| `run-moderate` | 運営の通報の確認・非公開化(#321)を試す(`ARGS`・例 `ARGS="reports"`)。[moderation.md](../docs/infra/moderation.md) | 触れない(エミュレータ) | 書き込みは `-yes` を付けた時だけ | `mk/moderate.mk` |
| `run-purge` | 退会から 30 日を過ぎたアカウントの消去(#458)を試す(`PURGE_FLAGS`)。外す時は `PURGE_FLAGS=-delete-auth=false`。[account-purge.md](../docs/infra/account-purge.md) | 触れない(エミュレータ) | はい(`-dry-run`) | `mk/purge.mk` |
| `run-runner` | 自動の動き(serverRules)の runner(#341)を :8082 で起動し、`RUNNER_TICK_INTERVAL`(既定 10s)ごとに outbox を処理する | 触れない(エミュレータ) | — | `mk/runner.mk` |
| `run-stg-cleanup` | 試験データ(smoke-* アプリ・stg の回数制限・試験ユーザー)の掃除(#293)を試す(`STG_CLEANUP_FLAGS`・既定 `-smoke -stg-ratelimits`)。stg / prod は [stg-cleanup.md](../docs/infra/stg-cleanup.md) の手順で人が回す | 触れない(エミュレータだけに向く) | はい(`-apply` を付けるまで消さない) | `mk/stg-cleanup.mk` |
| `schema-check` | `docs/dsl/dsl.schema.json` と `pkg/dsl/schema.json` が同じかを見る。違えば落ちる | 触れない | — | `Makefile` |
| `schema-sync` | `docs/dsl/dsl.schema.json` を `pkg/dsl/schema.json` に写す | 触れない | — | `Makefile` |
| `smoke-local` | appconfig(memory)と ai を :18080 / :18081 で起動し、`cmd/smoke` を回して止める(`SMOKE_ONLY=kakeibo` で 1 本)。`../.env` の `ANTHROPIC_API_KEY` で Claude を呼ぶ | 触れない(Claude は呼ぶ) | いいえ | `Makefile` |
| `smoke-prod` | 本番に `cmd/smoke` を家計簿 1 本だけ回し(Claude 2 回)、作った smoke-* アプリを片付ける(`-cleanup`) | 触れる(prod) | いいえ | `Makefile` |
| `smoke-stg` | stg に `cmd/smoke` を回す(Claude 3 回)。`FIREBASE_WEB_API_KEY` が無ければ health と 401 の確認だけ | 触れる(stg) | いいえ | `Makefile` |
| `test` | `schema-check` の後、`go test ./...` | 触れない | — | `Makefile` |
| `test-emulator` | エミュレータ試験を CI(`backend-emulator`)と同じ判定で回す。エミュレータが無ければ一時的に起動して止める | 触れない(エミュレータ) | — | `Makefile` |
| `vet` | `go vet ./...` | 触れない | — | `Makefile` |

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
| `RATE_LIMIT_ANON_DAILY` / `_USER_DAILY` / `_PRO_DAILY` | o | o | AI の 1 人 1 日の単位(既定 30 / 60 / 300)。他の `RATE_LIMIT_*` と理由は [docs/backend/rate-limit.md](../docs/backend/rate-limit.md) |
| `RATE_LIMIT_ANON_DAILY_<OS>` / `_USER_DAILY_<OS>` / `_PRO_DAILY_<OS>` | o | o | 上の 3 つを OS ごとに上書き(#2133)。`<OS>` は `IOS` / `ANDROID` / `MACOS` / `WINDOWS` / `LINUX` / `OTHER`(ヘッダ `X-Client-Platform` をそろえた値・ヘッダ無しや未知は `OTHER`)。例 `RATE_LIMIT_USER_DAILY_WINDOWS=20`。未設定 = 上書きなし(今と同じ)。ai と appconfig に同じ値を入れる |
| `FEEDBACK_DAILY_LIMIT` | o | - | `POST /v1/feedback` の 1 uid 1 日(JST)の件数(既定 5・0 で無制限・#319) |
| `LOOP_GENFAIL_SOURCE` / `LOOP_GENFAIL_PROJECT` / `LOOP_GENFAIL_SERVICES` / `LOOP_GENFAIL_MAX_LINES` | - | - | `cmd/feedback-loop` だけ。生成の失敗の急増を読む先(既定 `file` = `LOOP_GENFAIL_LOGS`・`logging` = Cloud Logging の API・#2146)。`logging` の既定は `GOOGLE_CLOUD_PROJECT` / `ai-<APP_ENV>` / 50000 行。job の SA に `roles/logging.viewer` が要る([feedback-loop.md](../docs/backend/features/feedback-loop.md#環境変数)) |
| `APPCONFIG_BASE_URL` | - | o | ai → appconfig。未設定なら ai 内のインメモリ stub(永続なし)。staging は `https://appconfig-stg-378953239426.asia-northeast1.run.app` |
| `ANTHROPIC_API_KEY` | - | o | Secret Manager。ローカルは `~/Desktop/JPHACKS2026/.env`(リポ外) |
| `APPSTORE_BUNDLE_IDS` / `APPSTORE_PRODUCT_IDS` | o | - | StoreKit 取引の許可リスト(#314)。既定 `jp.kz2601.metamon` / `jp.kz2601.metamon.pro.monthly` |
| `APPSTORE_ENVIRONMENTS` | o | o | 受け入れる App Store の環境。既定 `Sandbox,Production`(審査・TestFlight は Sandbox)。ai は Pro の判定(#449)に使う |

Cloud Run 上の appconfig は `AUTH_MODE` 未設定(= `firebase`)で、`run-backend` SA の ADC で Firestore と Firebase Admin を使う。
