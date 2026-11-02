# AI の一時停止スイッチ(#511)

Claude の障害・費用の急増・不正利用の時に、AI の生成だけを止める。記録の読み書き(appconfig)は止めない。

- 環境変数: ai サービスの `AI_PAUSED=1`(`true` / `on` も可)。起動時に読む。値を変えると Cloud Run の新しいリビジョンになるので、それで切り替わる。
- 止まるもの: `POST /v1/generate`(`mode=undo` を除く)・`POST /v1/chat`・`POST /v1/text`。Claude を呼ばず、利用上限も数えずに 503 `ai_paused` を返す。
- 止まらないもの: `GET /health`・`DELETE /v1/chat/{appId}`・generate の `mode=undo`・appconfig の全部。
- アプリ: 「AI を一時的に止めています。しばらくしてからもう一度お試しください」を出す(上限の文言とは別)。
- ログ: 弾いた依頼ごとに `ai.paused`(INFO・method と path だけ。uid・本文は出さない)。起動時に `ai: AI_PAUSED is on` が出る。

これまでの代用(`RATE_LIMIT_GLOBAL_DAILY=1`)は「上限に達しました」と出て理由が違うので使わない。

## 入れる・切る

GCP の設定変更なので、人が承認を得てから行う。`<env>` は `stg` / `prod`。

```sh
# 止める
gcloud run services update ai-<env> --region asia-northeast1 --update-env-vars AI_PAUSED=1

# 戻す
gcloud run services update ai-<env> --region asia-northeast1 --remove-env-vars AI_PAUSED
```

確認: `curl -s -o /dev/null -w '%{http_code}\n' -X POST https://<ai の URL>/v1/text -H 'Content-Type: application/json' -d '{}'` が止めている間は `503`(戻した後は 401 など 503 以外)。`/health` はどちらでも 200。

> 注意: 今の `.github/workflows/deploy-cloudrun.yml` は `--set-env-vars` で env を全部置き換えるので、**止めている間に ai がデプロイされるとスイッチが消えて AI が再開する**。止めている間は develop / main へのマージを控えるか、デプロイ後に入れ直す。運用で変える環境変数の扱いは #509 で決める。
