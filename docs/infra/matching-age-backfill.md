# 仲間探しのプロフィールに申告の印を付ける(一度だけ)

仲間探し(match)のプロフィールの記録に、作者が「18 歳以上」を申告済みであることの印 `ownerAgeConfirmed: true` を付ける手順です。スクリプトは `backend/cmd/matching-age-backfill` です。

> **本書の手順は人が実行します。** スクリプトはエージェントが作り、エミュレータでだけ試しています。stg / prod に向けての実行(ドライランを含む)はしていません。stg と prod は同じ Firestore を使っているので、実行は 1 回だけ・承認を得てから行います。

## 目次
- [何をするか](#何をするか)
- [手順](#手順)
- [エミュレータで試す](#エミュレータで試す)

## 何をするか

- サーバーは、プロフィールの記録に印が無い時だけ作者の `users/{uid}.ageConfirmedAt` を読み、申告の無い人のプロフィールを本人以外に出しません([matching.md](../backend/features/matching.md))。印が付いていれば読みません。
- 印は、新しいプロフィールの投稿の時と、本人が申告した時(`PUT /v1/me/age-confirmation` など)に付きます。印が付く前に作られた記録には付いていません。
- このスクリプトは、公開中の仲間探しアプリのプロフィールの記録のうち、**作者が申告済みで印の無いもの**に印を付けます。申告の無い作者の記録には何もしません(本人が申告した時に付きます)。
- 実行しなくても表示は正しいままです。実行すると一覧を開く時の読み取りが減ります。
- 何度実行しても同じ結果になります。出力は件数だけです(uid は出しません)。

## 手順

0. 前提: `gcloud auth application-default login` 済みで、対象の Firestore に書ける権限があること。
1. ドライラン(書き込まない)。stg と prod は同じ DB なので `-allow-prod-db` が要ります。

   ```sh
   cd backend
   GOOGLE_CLOUD_PROJECT=jphacks-kz2601 go run ./cmd/matching-age-backfill -allow-prod-db
   ```

   出力の `would set flag` が印を付ける件数、`author not declared (left hidden)` が申告の無い作者の件数です。
2. 件数がおかしくなければ、`-apply` を足して実行します。

   ```sh
   GOOGLE_CLOUD_PROJECT=jphacks-kz2601 go run ./cmd/matching-age-backfill -allow-prod-db -apply
   ```

3. もう一度ドライランをして、`would set flag: 0` になったことを確かめます(実行中に誰かが申告した分が増えることはあります)。

途中で失敗したら、そのまま同じコマンドをもう一度実行します。付け終わった記録は数え直すだけです。

## エミュレータで試す

```sh
scripts/firestore-emulator.sh start   # 別の端末で
cd backend
FIRESTORE_EMULATOR_HOST=127.0.0.1:8089 GOOGLE_CLOUD_PROJECT=demo-kz2601 go run ./cmd/matching-age-backfill
```
