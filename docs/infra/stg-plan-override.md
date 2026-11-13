# stg で Pro / 無料の判定を試す(#2085)

stg で、決めた試験用の uid のリクエストに限り、サーバーの Pro の判定をリクエストごとに `pro` / `free` に変えます。App Store の sandbox で買わずに、無料と Pro の両方の動きを試すためのものです。

## 目次
- [変わるもの・変わらないもの](#変わるもの変わらないもの)
- [効く条件](#効く条件)
- [入れる・外す](#入れる外す)
- [使い方](#使い方)
- [ログ](#ログ)

## 変わるもの・変わらないもの

変わるのは、`IsPro(ctx, uid)` を通る判定だけです。

| 判定 | 場所 |
|---|---|
| Pro の機能の 402 `pro_required` | ai `internal/ai/paid.go`・`text_payer.go`、appconfig `internal/appconfig/pro_gate.go` |
| 量の上限(記録・写真・アプリの数) | appconfig `internal/appconfig/plan_limits.go`・`app_slots.go` |
| 1 日の回数の枠 | ai `internal/ratelimit`・`internal/budget` |

変わらないもの:

- 保存した権利(`users/{uid}/entitlements/pro`)。何も書きません。判定のキャッシュにも入れません。stg は本番と同じデータベースを使っているためです。
- コインの残高・枠。帳簿に書くものなので変えません。
- 呼び出した本人以外の判定(共有のアプリの作成者の Pro など)。
- runner(prod だけで、リクエストがありません)。

## 効く条件

次の 3 つがそろった時だけ効きます。どれかが欠けると、ヘッダは何も変えず、エラーにもなりません。

1. 環境変数 `DEBUG_OVERRIDES=1`
2. 環境変数 `APP_ENV=stg`
3. トークンを検証した uid が、環境変数 `DEBUG_OVERRIDE_UIDS`(カンマ区切り)に入っている

`DEBUG_OVERRIDES` が入っているのに `APP_ENV` が `stg` でない時、ai・appconfig は起動しません。`DEBUG_OVERRIDES` が `1` 以外の値の時と、`DEBUG_OVERRIDE_UIDS` が空の時も起動しません。

`.github/workflows/deploy-cloudrun.yml` はこの 2 つを入れません([runtime-config.md](runtime-config.md) の「運用で変える値」。`scripts/check-deploy-env.sh` がデプロイで上書きしていないことを確かめます)。

## 入れる・外す

Cloud Run の設定を変える作業なので、持ち主が行います。`DEBUG_OVERRIDE_UIDS` には試験用のアカウントの uid だけを入れます(実際に使っている人のアカウントは入れません)。uid は repo・Issue・PR に書きません。ai と appconfig の両方に同じ値を入れます(ai は appconfig への呼び出しにヘッダを渡し、appconfig は自分の設定で判断します)。

```sh
# 入れる(uid はカンマ区切り。値にカンマを含むので区切りを ^@^ に替える)
gcloud run services update ai-stg --region asia-northeast1 \
  --update-env-vars '^@^DEBUG_OVERRIDES=1@DEBUG_OVERRIDE_UIDS=<uid1>,<uid2>'
gcloud run services update appconfig-stg --region asia-northeast1 \
  --update-env-vars '^@^DEBUG_OVERRIDES=1@DEBUG_OVERRIDE_UIDS=<uid1>,<uid2>'

# 外す
gcloud run services update ai-stg --region asia-northeast1 --remove-env-vars DEBUG_OVERRIDES,DEBUG_OVERRIDE_UIDS
gcloud run services update appconfig-stg --region asia-northeast1 --remove-env-vars DEBUG_OVERRIDES,DEBUG_OVERRIDE_UIDS
```

確認: 起動時のログに、ai は `ai: warning: DEBUG_OVERRIDES=1; ...`、appconfig は `debug override on` が出ます(uid の数だけを出し、uid そのものは出しません)。

デプロイは `--update-env-vars` でワークフローが決める値だけを上書きするので、入れた値はデプロイの後も残ります。使い終わったら外してください。

## 使い方

リクエストに次のヘッダを付けると効きます。端末のデバッグメニュー(Debug / Staging の版)から付けるのは今後の作業です(#2086)。今は curl などで付けて試します。

開発用メニューの「Pro として見る」(#2170)は端末の中の表示だけを変えるもので、サーバーの判定には効きません。サーバーの判定まで変えるのは、このヘッダが付いた時だけです。

```
X-Debug-Plan: pro
X-Debug-Plan: free
```

- 値はそのリクエストにだけ効きます。ヘッダの無い次のリクエストは、保存した権利のとおりに判定します。
- `pro` / `free` 以外の値は無視します。
- curl で試す例(appconfig・`<token>` は許可した uid の ID トークン):

```sh
curl -s -H "Authorization: Bearer <token>" -H 'X-Debug-Plan: free' https://<appconfig-stg の URL>/v1/me/usage
```

公開の API の約束ではないので、`docs/backend/openapi-*.yaml` には載せていません。

## ログ

判定を変えるたびに INFO の `debugoverride.plan` を 1 行出します(`uid_hash`・`plan`・`caller` = 判定した場所のファイルと行)。
