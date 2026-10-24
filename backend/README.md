# backend (Go)

単一モジュール・複数サービス構成。`cmd/<service>` が1つの Cloud Run サービスに対応する。

| cmd | 役割 |
|---|---|
| `cmd/appconfig` | アプリ DSL JSON の CRUD |
| `cmd/ai` | 音声/文字 → Claude → DSL JSON 生成 |

共有型は `pkg/dsl`。

```sh
cd backend
go run ./cmd/appconfig   # :8080
go run ./cmd/ai          # PORT=8081 go run ./cmd/ai
go test ./...
```
