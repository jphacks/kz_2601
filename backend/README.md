# backend (Go)

単一モジュール・複数サービス構成。`cmd/<service>` が1つの Cloud Run サービスに対応する。
設計: `docs/backend/architecture.md`、API契約: `docs/backend/openapi-*.yaml`、DSL: `docs/dsl/README.md` + `docs/dsl/dsl.schema.json`。

| cmd | 担当 | 役割 |
|---|---|---|
| `cmd/appconfig` | @2410as | アプリ DSL JSON の CRUD・JSON Patch 適用・バージョン・汎用レコード |
| `cmd/ai` | Claude Code | 音声/文字 → Claude → DSL(初回フル / 以後 JSON Patch) → appconfig |

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

## 起動

```sh
cd backend
make build vet test      # CI と同じ(golangci-lint run ./... も通す)
make emulator            # 別ターミナル: Firestore :8089 / Auth :9099
make dev                 # appconfig :8080 / ai :8081 (AUTH_MODE=dev, X-Debug-Uid で叩ける)
make schema-sync         # docs/dsl/dsl.schema.json → pkg/dsl/schema.json (スキーマ変更時)

# エミュレータ無しで最速起動(インメモリ store・Bearer 値をそのまま uid として扱う)
STORE=memory AUTH_MODE=dev PORT=8080 go run ./cmd/appconfig
export DEV_UID=dev-smoke
curl -H "Authorization: Bearer $DEV_UID" localhost:8080/v1/apps/my-device/dsl   # 未知 appId は version 1 を自動作成
```

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
| `FIRESTORE_DATABASE` | o | o | Firestore の DB ID。既定 `(default)`。全バイナリ(appconfig / ai の回数制限 / cmd/purge)が `internal/fsenv` から読む(#290) |
| `APP_ENV` | o | o | `stg` / `prod`。`stg` が本番の接続先(`PROD_FIRESTORE_PROJECT`/`PROD_FIRESTORE_DATABASE`、既定 `jphacks-kz2601`/`(default)`)を使うと**起動を止める**(エミュレータ時は対象外) |
| `ALLOW_SHARED_PROD_DB` | o | o | `1` のときだけ上の停止を外す(警告ログは出る)。stg が本番と DB を共有している間(#291・#293 の分離まで)は stg に必須。deploy-cloudrun.yml が stg=`1` / prod=`0` を入れる |
| `STORE` | o | - | `memory` でインメモリ store を強制(既定は `GOOGLE_CLOUD_PROJECT` があれば Firestore) |
| `FIRESTORE_EMULATOR_HOST` | o | - | 設定時は Firestore クライアントが自動でエミュレータへ |
| `FIREBASE_AUTH_EMULATOR_HOST` | o | o | ローカルのみ(`X-Debug-Uid` の有効条件) |
| `LOG_LEVEL` | o | o | `info` |
| `APPCONFIG_BASE_URL` | - | o | ai → appconfig。未設定なら ai 内のインメモリ stub(永続なし)。staging は `https://appconfig-stg-378953239426.asia-northeast1.run.app` |
| `ANTHROPIC_API_KEY` | - | o | Secret Manager。ローカルは `~/Desktop/JPHACKS2026/.env`(リポ外) |

Cloud Run 上の appconfig は `AUTH_MODE` 未設定(= `firebase`)で、`run-backend` SA の ADC で Firestore と Firebase Admin を使う。
