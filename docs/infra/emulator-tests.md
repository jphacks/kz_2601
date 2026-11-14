# Firestore エミュレータで回す Go のテストの書き方(#546・#550)

`backend/` の Firestore を使うテストは、エミュレータがある時だけ回り、無い時は skip する。CI は 2 本に分かれている: `backend-ci`(`go test ./... -race`・エミュレータ無し = skip)と `backend-emulator`(エミュレータを立てて、エミュレータ試験のあるパッケージだけを回す・`.github/workflows/backend-emulator.yml`)。起動と判定は `scripts/firestore-emulator.sh` が CI と手元で共用する。

## 目次
- [回し方](#回し方)
- [skip の書き方](#skip-の書き方)
- [ヘルパー](#ヘルパー)
- [並行で同じエミュレータを使う時](#並行で同じエミュレータを使う時)
- [黙って skip しない判定](#黙って-skip-しない判定)
- [新しいパッケージに足す時](#新しいパッケージに足す時)

## 回し方

| 入口 | 中身 |
|---|---|
| `make -C backend emulator` | エミュレータを前面で起動(`scripts/firestore-emulator.sh start`・Ctrl-C で止まる)。`make dev` と組む |
| `make -C backend test-emulator` | 起動していなければ一時的に起動 → 試験 → 自分で起動した分だけ止める(`test-all`)。CI と同じ判定 |
| `scripts/firestore-emulator.sh test` | 起動済みのエミュレータに対して試験だけ |
| `scripts/firestore-emulator.sh wait` | 起動済みのエミュレータが応答するまで待つ(最大 90 秒) |

- 接続先は `FIRESTORE_EMULATOR_HOST`(既定 `127.0.0.1:8089`)。8089 が塞がっている時は `FIRESTORE_EMULATOR_HOST=127.0.0.1:18089 make -C backend test-emulator` のように変える
- プロジェクトは `demo-kz2601` 固定(`demo-` で始まるプロジェクトは実の GCP に繋がらない)。テスト側もこの名前を直に書く。staging・本番には決して向けない
- 要る物: gcloud(`cloud-firestore-emulator` コンポーネント)・Java 11 以上・Go・jq・curl
- 1 つのパッケージだけ: `cd backend && FIRESTORE_EMULATOR_HOST=127.0.0.1:8089 GOOGLE_CLOUD_PROJECT=demo-kz2601 go test ./internal/store/ -count=1`(エミュレータは先に起動しておく)

## skip の書き方

skip の文言は **`FIRESTORE_EMULATOR_HOST not set` から 1 文字も変えない**。スクリプトはこの文字列で「エミュレータ試験のあるパッケージ」を探す(`grep -rl ... 'FIRESTORE_EMULATOR_HOST not set'`)ので、文言を変えるとそのパッケージは CI で回らない。

形は 2 つ。名前の付け方も判定に使う(下の「[黙って skip しない判定](#黙って-skip-しない判定)」)。

| 形 | 名前 | 例 |
|---|---|---|
| memory と Firestore の両方で同じテスト | サブテスト `memory` / `firestore` | `internal/store/store_pair_test.go` の `bothStores`・`internal/coins/coins_test.go`・`internal/runner/queue/queue_test.go` |
| Firestore だけのテスト | `TestFirestore...` | `internal/ratelimit/firestore_test.go` の `TestFirestoreConsume`・`peek_test.go` の `TestFirestorePeek` |

```go
t.Run("firestore", func(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	// ...
})
```

## ヘルパー

パッケージごとに小さなヘルパーを持つ(テストのファイルはパッケージをまたいで import できないため)。

| ヘルパー | 場所 | 中身 |
|---|---|---|
| `bothStores` | `internal/store/store_pair_test.go` | memory と、エミュレータがあれば Firestore で `f(t, s, sfx)` を回す。Firestore の時だけ `sfx` に回ごとの接尾辞 |
| `emulatorClient` | `cmd/ai-session-report/firestore_test.go` | テストごとに新しいプロジェクト(`demo-asr-<時刻>`)の client |
| `emulatorFS` | `cmd/stg-cleanup/main_test.go` | テストごとに新しいプロジェクト(`demo-sc-<時刻>`)。消す対象を探すテストなので、他のテスト・他のパッケージのデータが混ざらないように |
| `emulatorFirestore` | `internal/store/matching_age_flag_test.go` | `emulatorStore` と同じ形 |
| `emulatorStore` | `internal/store/store_pair_test.go` | skip → `NewFirestore(ctx, "demo-kz2601", "(default)")` → `t.Cleanup` で Close |
| `newEmulatorStore` | `internal/ratelimit/firestore_test.go` | ratelimit の Firestore(コレクション `rateLimitsTest`)と生の client |
| `runSuffix` | `internal/store/store_pair_test.go` | `"-" + 36 進の UnixNano`。ID に足して回ごとに分ける |

## 並行で同じエミュレータを使う時

`go test` は複数のパッケージを同時に回し、どれも同じエミュレータの同じプロジェクトを使う。エミュレータは止めるまで前の回のデータを残す。なので ID は「回ごと」と「パッケージごと」の両方で分ける。

| 工夫 | いつ使う | 例 |
|---|---|---|
| ID に回ごとの接尾辞を足す | ほぼ全部(uid・appId・文書の ID)。前の回のデータと混ざらないように | `runSuffix()`・`"-"+strconv.FormatInt(time.Now().UnixNano(), 36)`(`internal/appconfig/share_test.go`・`internal/runner/runner_test.go`) |
| パッケージごとの乱数(salt)を値に混ぜる | 全体で 1 つの索引を引く値。uid が違っても同じ値なら他のパッケージの文書を消し合う | `devTokenSalt`(`internal/appconfig/devices_test.go`・`internal/store/store_device_test.go`)・`tokenSalt`(`internal/runner/runner_devices_test.go`)。端末のトークンの索引 `device_tokens/{sha256(token)}` は全体で 1 つなので、同じトークンを別のパッケージが登録すると自分の端末が消える |
| テストごとに別のプロジェクト | コレクション全体を数える・探すテスト(他のテストの文書が見えると結果が変わる) | `emulatorFS`(`demo-sc-...`)・`emulatorClient`(`demo-asr-...`) |
| 環境の名前を回ごとに変える | `APP_ENV` を ID に使う数(`rateLimits/{env}_...`) | `internal/runner/quota/quota_test.go`(`"test"+UnixNano`)・`internal/ratelimit/peek_test.go`(`"peek_"+UnixNano`) |

- 固定の ID(`"u1"` など)は memory の時だけにする。Firestore の時は必ず接尾辞を足す
- 全体のコレクション(`device_tokens`・`public_apps`・`rateLimits` など)を数えるテストは、数え方を「自分が作った ID だけ」にするか、別のプロジェクトを使う
- 上のヘルパーの `t.Cleanup` は client を閉じるだけで、書いた文書は消さない。回ごとに ID が違うので、残っても次の回とぶつからない。エミュレータを止めればデータは消える

## 黙って skip しない判定

`scripts/firestore-emulator.sh test` は `go test -json` の出力を見て、次のどれかなら失敗にする(エミュレータがあるのに回らなかったことを見逃さないため)。

| 判定 | 失敗の文言 |
|---|---|
| Firestore の試験が 1 件も通っていない | `no Firestore emulator test ran` |
| skip の理由に `FIRESTORE_EMULATOR_HOST not set` が出た | `emulator tests were skipped` |
| `TestFirestore...` か、名前が `/firestore` で終わるサブテストが 1 件でも skip | `skipped Firestore tests:` と名前の一覧 |
| エミュレータ試験のあるパッケージが見つからない | `no emulator test packages found` |

- だから Firestore のテストは `TestFirestore...` か `firestore` のサブテストにする。別の名前(`TestEmulator...` など)だと、通っても「通った数」に入らず、エミュレータ以外の理由の skip も見逃す(skip の文言での判定には引っかかる)
- エミュレータ以外の理由で skip する Firestore のテストは作らない(判定で落ちる)
- 通った数は `Firestore emulator tests passed: N` として出す(CI では job summary にも)

## 新しいパッケージに足す時

ループ・コイン・extapi などに足す時の手順。

1. skip を上の形で書く(文言はそのまま)。パッケージが見つかれば、スクリプトと CI に足す物は無い
2. テストの名前を `TestFirestore...` か `firestore` のサブテストにする
3. ID に回ごとの接尾辞を足す。全体の索引を引く値があれば、パッケージごとの salt も混ぜる
4. `make -C backend test-emulator` で、出力の `packages:` に自分のパッケージが出て、`Firestore emulator tests passed` の数が増えたことを確かめる
5. エミュレータ無しの `go test ./...` で skip になる(落ちない)ことも確かめる
