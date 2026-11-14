# 最初のコインの付与の記録を作る(一度だけ)

最初のコイン(`COIN_INITIAL_GRANT`)は、Apple / Google のサインインの相手(`sub`)ごとに 1 回です(#1429)。付与した時に `coin_initial_grants/{key}` に記録します(key は `sub` から作るハッシュ。`sub` そのものは保存しない)。#1429 の前に付与したアカウントにはこの記録がないので、修正前の付与の記録を作ります。スクリプトは `backend/cmd/coin-initial-backfill` です。

> **本書の手順は人が実行します。** スクリプトはエージェントが作り、メモリ上の store とエミュレータでだけ試しています。stg / prod に向けての実行(ドライランを含む)はしていません。stg と prod は同じ Firestore を使っているので、実行は 1 回だけ・承認を得てから行います。

## 目次
- [何をするか](#何をするか)
- [手順](#手順)
- [エミュレータで試す](#エミュレータで試す)

## 何をするか

- Firebase Auth のユーザーを全部読みます(Admin SDK・読むだけ)。`firebase auth:export` の JSON を `-export` で渡すこともできます。
- Apple / Google でサインインしていて、ウォレット(`users/{uid}/coins/wallet`)の `initialGranted` が `true` のアカウントについて、そのサインインの相手ごとの `coin_initial_grants/{key}` を作ります。key と中身(`owner` = uid のハッシュ・`at`)は付与の処理と同じ関数(`coins.InitialKey`・`coins.OwnerHash`)で作ります。
- 別のアカウントの記録が既にある相手は、そのままにします(付与の処理と同じ判断)。
- ウォレット・コインの残高は変えません。
- 既定はドライランで、書くのは `-apply` の時だけです。何度実行しても同じ結果になります。
- 出力は件数だけです(uid・`sub`・メールアドレスは出しません。失敗した時もエラーの種類だけ)。

## 手順

0. 前提: `gcloud auth application-default login` 済みで、対象のプロジェクトの Firebase Auth を読めて Firestore に書ける権限があること。
1. ドライラン(書き込まない)。stg と prod は同じ DB なので `-allow-prod-db` が要ります。

   ```sh
   cd backend
   GOOGLE_CLOUD_PROJECT=jphacks-kz2601 go run ./cmd/coin-initial-backfill -allow-prod-db
   ```

   Admin SDK を使わない時は、先に `firebase auth:export users.json --format=json --project jphacks-kz2601` で書き出し、`-export users.json` を足します。**書き出したファイルにはメールアドレスなどの個人情報が入るので、リポジトリの外に置き、終わったらすぐ消します。**

   出力の見方:

   | 行 | 意味 |
   | --- | --- |
   | `users read` | 読んだユーザー数 |
   | `no Apple / Google sign-in` | 匿名・メールなど。対象外 |
   | `initial grant not given` | ウォレットが無い・最初のコインをまだ受け取っていない |
   | `already recorded` | 記録が揃っている |
   | `subject recorded by another account (left as is)` | 別のアカウントの記録がある。触らない |
   | `accounts to record` / `claims would write` | 記録を作るアカウント数・記録の数 |

2. 件数がおかしくなければ、`-apply` を足して実行します。

   ```sh
   GOOGLE_CLOUD_PROJECT=jphacks-kz2601 go run ./cmd/coin-initial-backfill -allow-prod-db -apply
   ```

3. もう一度ドライランをして、`claims would write: 0` になったことを確かめます。

途中で失敗したら(`ERROR:` の行)、そのまま同じコマンドをもう一度実行します。作り終わった記録は `already recorded` に数えられます。

## エミュレータで試す

```sh
scripts/firestore-emulator.sh start   # 別の端末で
cd backend
FIRESTORE_EMULATOR_HOST=127.0.0.1:8089 GOOGLE_CLOUD_PROJECT=demo-kz2601 \
  go run ./cmd/coin-initial-backfill -export ./users.json
```

エミュレータの Firestore に本物の Firebase Auth を組み合わせることはできません(`FIREBASE_AUTH_EMULATOR_HOST` か `-export` が要ります)。
