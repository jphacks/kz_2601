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

```sh
cd backend
make build vet test      # CI と同じ
make emulator            # 別ターミナル: Firestore :8089 / Auth :9099
make dev                 # appconfig :8080 / ai :8081 (AUTH_MODE=dev, X-Debug-Uid で叩ける)
make schema-sync         # docs/dsl/dsl.schema.json → pkg/dsl/schema.json (スキーマ変更時)
```
