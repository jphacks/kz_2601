# 新しい cmd(運用コマンド・Cloud Run Job)を足す時の手順

`backend/cmd/` に運用のコマンド(退会アカウントの消去・試験データの掃除・一度だけの backfill など)を足す時の決まりです。足し方を揃えておくと、「何に繋がるのか」「何を消すのか」をコードを全部読まずに確かめられます。

> **deploy と IAM は人の作業です。** エージェントはコードとテスト・`mk/<feature>.mk`・手順書までを書き、エミュレータでだけ試します。Cloud Run Job の作成・サービスアカウントと権限・Cloud Scheduler の登録・stg / prod への実行は、承認を得てから人が行います([account-purge.md の「設定の手順(人の作業)」](../infra/account-purge.md#設定の手順人の作業)が実例)。

## 目次
- [1. フォルダの形](#1-フォルダの形)
- [2. 接続先は internal/fsenv で決める](#2-接続先は-internalfsenv-で決める)
- [3. 消すコマンドはドライランを既定にする](#3-消すコマンドはドライランを既定にする)
- [4. ログに uid と利用者の書いたものを出さない](#4-ログに-uid-と利用者の書いたものを出さない)
- [5. mk/\<feature\>.mk を置く](#5-mkfeaturemk-を置く)
- [6. イメージに同梱する(Cloud Run Job にする時だけ)](#6-イメージに同梱するcloud-run-job-にする時だけ)
- [7. テスト](#7-テスト)
- [8. 手順書を書く](#8-手順書を書く)
- [9. 既存の cmd(読む順)](#9-既存の-cmd読む順)

## 1. フォルダの形

`backend/cmd/<name>/` を作り、`package main` を置きます。名前はハイフン区切りで、何をするかが分かるもの(`purge`・`stg-cleanup`・`matching-age-backfill` のような動詞か対象)にします。

```
backend/cmd/<name>/
  main.go        # doc コメント・flag の定義と検査・配線・run
  main_test.go   # run のテスト(store は fake)
  flags_test.go  # 引数の表のテスト(任意・引数が増えたら分ける)
```

ファイルは最初は `main.go` 1 本で足り、テストが増えたら分けます([`cmd/matching-age-backfill`](../../backend/cmd/matching-age-backfill) は `flags_test.go` / `mode_test.go` に分かれています)。

`main.go` の先頭の doc コメントに、次を必ず書きます(`go doc ./cmd/<name>` で読めるため、ここが一次情報になります)。

- 何を消す / 書き換えるのか、何を残すのか
- 人が実行するのか、Cloud Run Job + Scheduler で動くのか
- 読む環境変数の一覧(下の `fsenv` の 4 つ + そのコマンド固有のもの)
- 手順書への参照(`docs/infra/<name>.md`)

[`cmd/purge/main.go`](../../backend/cmd/purge/main.go) と [`cmd/stg-cleanup/main.go`](../../backend/cmd/stg-cleanup/main.go) の doc コメントがそのままお手本です。

`main` は**配線だけ**にして、実際の処理は `run(ctx, st, ..., logger, now)` のような関数に出します。`st` は `store` の interface(必要な口だけ)で受け、`now` は引数で渡します。こうするとテストで fake と固定時刻を差し替えられます。

## 2. 接続先は internal/fsenv で決める

Firestore に繋ぐコマンドは、プロジェクトと DB を自分で `os.Getenv` から組み立てず、[`internal/fsenv`](../../backend/internal/fsenv/fsenv.go) を通します(#290)。読む環境変数は次の 4 つです。

| 環境変数 | 意味 |
| --- | --- |
| `GOOGLE_CLOUD_PROJECT` | プロジェクト。運用コマンドでは必須(空なら exit 2) |
| `FIRESTORE_DATABASE` | DB ID。既定は `(default)` |
| `APP_ENV` / `ALLOW_SHARED_PROD_DB` | stg の守り。`stg` が本番の DB を指していたら止める(`ALLOW_SHARED_PROD_DB=1` で許す) |
| `FIRESTORE_EMULATOR_HOST` | エミュレータ。この時は守りを掛けない |

書き方:

```go
target := fsenv.FromEnv(os.Getenv)
if err := guard(target, o.allowProd, os.Getenv); err != nil { /* exit 2 */ }
fmt.Printf("target: %s (emulator=%t)\n", target, target.Emulator != "")
st, err := store.NewFirestore(ctx, target.Project, target.Database)
```

`guard` の中身は、データを消す / 書き換えるコマンドでは次の 3 段にします([`cmd/matching-age-backfill/main.go`](../../backend/cmd/matching-age-backfill/main.go) の `guard`)。

1. `t.Project == ""` ならエラー(接続先が決まっていない)
2. `fsenv.Check(t, os.Getenv)` — stg が本番の DB を使っていたら止める
3. `fsenv.IsProdDatabase(t, os.Getenv)` が真で `-allow-prod-db` が無ければ止める

stg と prod は今も同じ Firestore を共有しているので([decision 288](../decisions/288-prod-staging-db-split.md))、3 を省くと「stg のつもりで本番を消す」が起こります。毎日動く Cloud Run Job(`cmd/purge`)は本番の DB に向くのが正しいので 3 を持たず、1・2 と `-older-than` の下限で守っています。**人が手で実行するコマンドには 3 を必ず付けます。**

接続先は必ず起動直後に 1 行出します(`purge: firestore` / `target: ...`)。どこに繋いだのか分からない実行ログを残さないためです。

## 3. 消すコマンドはドライランを既定にする

データを消す・書き換えるコマンドは、**引数を何も付けない実行が何も壊さない**ようにします。

- **人が実行するコマンド**: `-apply`(書き込む)・`-yes` のような肯定の flag を足し、既定は `false`。既定の実行は件数を数えて出すだけにします。`cmd/matching-age-backfill` の `-apply`、`cmd/coin-initial-backfill` の `-apply`、`cmd/moderate` の `-yes` がこの形です。`cmd/stg-cleanup` はさらに `-apply` に `-expect=<ドライランが出した件数>` を要求して、DB や対象リストが変わっていたら止めます。
- **Cloud Run Job**: `-dry-run` の **flag の既定値は `false`** で構いません(既定が `true` だと、スケジュールされた Job が毎日何も消さないことになります)。代わりに、Job を作る最初の実行だけ `--args="-dry-run,..."` で動かし、ログを確かめてから外す、という手順を手順書に書きます([account-purge.md の 2](../infra/account-purge.md))。`mk/<feature>.mk` の既定は `-dry-run` にして、ローカルの `make run-<name>` が何も消さないようにします。

どちらの場合も、取り返しのつかない引数には下限や上限を付け、`flag.Parse` の直後に検査して exit 2 で止めます。`cmd/purge` は `-older-than` / `-feedback-older-than` が 24h 未満なら拒否し、`-limit` で 1 回の件数を抑えています。検査は `parseFlags(fs *flag.FlagSet, args []string) (options, error)` に出すとテストから呼べます。

## 4. ログに uid と利用者の書いたものを出さない

- Cloud Run Job は `httpx.NewLogger()` の JSON ログ(Cloud Logging)を使い、`slog.SetDefault` します。人が実行するコマンドは `fmt.Println` で読みやすく出して構いません。
- uid はそのまま出さず、ハッシュにします(`cmd/purge` の `uidHash`)。アプリの名前・フィードバックの本文など、利用者が書いたものは出しません。件数だけを出します。
- 終了コード: 引数や接続先が不正なら 2、処理の途中で失敗したものがあれば 1。Cloud Run Jobs は 0 以外を失敗として残します。
- `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` で止められるようにします(Job のタイムアウトで途中終了しても、次の実行で続きができる作りにします)。

## 5. mk/\<feature\>.mk を置く

`backend/Makefile` は末尾で `-include $(wildcard mk/*.mk)` をしているので、`backend/mk/<feature>.mk` を置くだけで target が増えます。Makefile 本体には足しません。

```make
# backend/Makefile の末尾から -include される。レシピは backend/ を作業ディレクトリとして動く(make -C backend)。

# 何をするコマンドなのか(Issue 番号)。既定は -dry-run。
# 本番の手順は docs/infra/<name>.md(人が実行する)。この target はエミュレータにしか向けない。
<NAME>_FLAGS ?= -dry-run
.PHONY: run-<name>
run-<name>:
	FIRESTORE_EMULATOR_HOST=localhost:8089 GOOGLE_CLOUD_PROJECT=demo-kz2601 go run ./cmd/<name> $(<NAME>_FLAGS)
```

決まり:

- 1 行目に `-include` の説明を入れます(どのディレクトリで動くかが分かるように)。
- target は `run-<name>`。引数は `<NAME>_FLAGS ?=` で上書きできるようにします。
- 向き先は**エミュレータだけ**にします(`FIRESTORE_EMULATOR_HOST=localhost:8089`・`GOOGLE_CLOUD_PROJECT=demo-kz2601`)。stg / prod に向く `make` target は作りません。
- 実例: [`mk/purge.mk`](../../backend/mk/purge.mk)・[`mk/stg-cleanup.mk`](../../backend/mk/stg-cleanup.mk)・[`mk/feedback-loop.mk`](../../backend/mk/feedback-loop.mk)。

エミュレータは別の端末で `make emulator`(または `scripts/firestore-emulator.sh start`)を動かしておきます。`make emulator` は Auth のエミュレータを起動しないので、Firebase Auth を触るコマンドはローカルではその部分を切る flag(`-delete-auth=false` など)を用意しておくと試せます。

## 6. イメージに同梱する(Cloud Run Job にする時だけ)

一度だけ手で実行するコマンドは `go run ./cmd/<name>` で足ります。Cloud Run Job として動かすものだけ、[`backend/Dockerfile`](../../backend/Dockerfile) のビルド行に 1 行足します。

```dockerfile
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/<name> ./cmd/<name>
```

1 つのイメージに全部のバイナリを入れ、Cloud Run 側が `--command=/app/<name>` で切り替える形です(ビルドは 1 回)。先頭のコメントにも何のバイナリかを足します。イメージを作るのは `deploy-cloudrun.yml` で、**Job 自体は更新しません**。backend を変えた後に Job のイメージを揃えるのは人の作業です。

## 7. テスト

- `run` を fake の store で呼ぶテストを書きます(`backend/internal/store` の Memory 実装か、必要な口だけの interface を自前で fake にします)。`cmd/stg-cleanup/run_fake_test.go`・`cmd/appstore-reconcile/run_test.go` が例です。
- 引数の検査は表のテストにします(既定値・下限ちょうど・下限未満・負の値・未知の flag)。`flag.NewFlagSet(name, flag.ContinueOnError)` と `fs.SetOutput(io.Discard)` を使うと、テストから `parseFlags` を直接呼べます。
- **既定値そのものをテストで固定します。** 「引数無しの実行が何も消さない」「`-older-than` の既定が保存期間より短くない」はコードを読まないと分からないので、テストに書いて壊れたら落ちるようにします。
- `cd backend && go build ./... && go test ./...` と `gofmt -l cmd/<name>`・`go vet ./cmd/<name>` を通します。backend の CI(`backend-ci.yml`)は 5 分以内に収める決まりなので、実 API や実 Firestore を叩くテストは書きません。

## 8. 手順書を書く

人が実行するもの・GCP の設定が要るものは、`docs/infra/<name>.md` を作ります。先頭に「**本書の手順は人が実行します**」の引用を置き、次を書きます。

- 何をするか・何を消して何を残すか(表)
- flag の一覧と既定値(表)
- 手順(ドライラン → 件数の確認 → `-apply` / `-dry-run` を外す → もう一度確認)
- エミュレータで試す方法
- 止め方・確認の仕方(Cloud Run Job なら executions と Cloud Logging のクエリ)

書き方の見本: [account-purge.md](../infra/account-purge.md)(Cloud Run Job + Scheduler)・[stg-cleanup.md](../infra/stg-cleanup.md)(人が実行)・[matching-age-backfill.md](../infra/matching-age-backfill.md)(一度だけの backfill)。GCP 側の前提は [gcp-setup.md](../infra/gcp-setup.md) にあるので、重ねて書かずに参照します。

最後に [architecture.md の「8. ディレクトリ構成」](architecture.md#8-ディレクトリ構成) の `cmd/` の一覧に 1 行(1 行の用途 + Issue 番号 + 手順書)を足します。

## 9. 既存の cmd(読む順)

| cmd | 形 | 読みどころ |
| --- | --- | --- |
| [`cmd/matching-age-backfill`](../../backend/cmd/matching-age-backfill) | 人が一度だけ実行 | 一番小さい実例。`parseFlags` / `guard` / `run` の分け方 |
| [`cmd/stg-cleanup`](../../backend/cmd/stg-cleanup) | 人が実行(消す) | `-apply` + `-expect` の二重確認、対象を flag が名指ししたものだけに限る作り |
| [`cmd/purge`](../../backend/cmd/purge) | Cloud Run Job + Scheduler | 引数の下限、uid のハッシュ、JSON ログ、やり直して安全な順序 |
| [`cmd/coin-initial-backfill`](../../backend/cmd/coin-initial-backfill) | 人が一度だけ実行 | Firebase Auth の export を読む形 |
| [`cmd/moderate`](../../backend/cmd/moderate) | 人が実行(サブコマンド) | `ARGS="reports"` のようなサブコマンド型と `-yes` |
