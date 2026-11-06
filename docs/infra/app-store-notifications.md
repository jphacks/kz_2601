# App Store Server Notifications v2 と照合ジョブ(#315)

自動更新・請求の失敗(猶予期間)・解約・期限切れ・返金は、端末が開かれなくても Apple からサーバーに届く通知で `users/{uid}/entitlements/pro` に反映します。通知が届かなかった分は、App Store Server API で状態を取り直す照合ジョブで拾います。権利の保存と取引の検証は #314(`docs/backend/features/pro.md`)と同じものを使います。

> **まだ設定していません。** App Store Connect への URL の登録・照合ジョブの作成は人の作業です(下の手順)。GCP・App Store Connect の設定は承認を得てから行います。

## 目次
- [受け口](#受け口)
- [種類ごとの扱い](#種類ごとの扱い)
- [App Store Connect への登録(人の作業)](#app-store-connect-への登録人の作業)
- [sandbox での確認(人の作業)](#sandbox-での確認人の作業)
- [照合ジョブ](#照合ジョブ)
- [確認と運用](#確認と運用)

## 受け口

| 項目 | 内容 |
| --- | --- |
| URL | `https://<appconfig のホスト>/v1/appstore/notifications`(`POST`・`entitlement.NotificationsPath`) |
| prod | `https://appconfig-prod-zgciuf3iha-an.a.run.app/v1/appstore/notifications` |
| 認証 | 無し(Firebase の ID トークン・App Check の対象外)。`signedPayload` の JWS を **Apple Root CA - G3**(バックエンドに埋め込み)までの証明書チェーンで検証するのが認証の代わり |
| 確かめること | 署名・チェーン(#314 と同じ)、`version` が `2.0`、`data.bundleId`(`APPSTORE_BUNDLE_IDS`)、`data.environment`(`APPSTORE_ENVIRONMENTS`)、中の `signedTransactionInfo` と `signedRenewalInfo` も同じ方法で検証 |
| uid の引き方 | `appstore_transactions/{originalTransactionId}`(iOS が購入後に `POST /v1/me/entitlements/apple` で送った時に作られる結びつき)。`appAccountToken` は uid の一方向ハッシュなので、ログで照合するだけ |
| 冪等 | `appstore_notifications/{notificationUUID}` に処理結果を記録し、同じ UUID は 2 回目以降 `duplicate` で何もしない。記録より先に反映するので、記録に失敗して Apple が再送しても結果は同じ |
| 順序 | Apple は順番を保証しない。保存済みの権利は前にしか進まない: 同じ取引は `signedDate` が保存済み以上の時だけ上書き、別の取引は期限が後のものだけ上書き(古い通知・端末に残った古い取引で巻き戻さない) |
| 応答 | 200 `{"outcome": "applied" \| "duplicate" \| "unbound" \| "ignored"}`。400 は本文が不正、422 は検証失敗(`details.reason`)、500 は一時的な失敗で Apple が再送する(最大 5 回・約 3 日) |

sandbox と本番は **同じ URL** に向けます。TestFlight と審査は本番の版でも Sandbox で買い、stg と prod は同じ Firestore を使うためです。どちらの環境を受けるかは `APPSTORE_ENVIRONMENTS`(既定 `Sandbox,Production`)で決まります。

## 種類ごとの扱い

通知の中の取引(と更新情報)をそのまま保存済みの権利に重ねる(`merge`)ので、種類ごとの分岐はほとんどありません。

| 種類 | 結果 |
| --- | --- |
| `SUBSCRIBED` | 結びつきがあれば反映。まだ無ければ `unbound`(iOS が送った時に保存される) |
| `DID_RENEW`・`RENEWAL_EXTENDED` | 期限が後の取引で上書き(Pro が続く) |
| `DID_FAIL_TO_RENEW`(`GRACE_PERIOD`) | 更新情報の `gracePeriodExpiresDate` を `graceExpiresAt` に保存。`expiresAt` を過ぎてもその時刻まで Pro |
| `GRACE_PERIOD_EXPIRED`・`EXPIRED` | 期限(と猶予)が過去になり Pro でなくなる |
| `DID_CHANGE_RENEWAL_STATUS` | `willRenew` を保存(解約の予約。期限までは Pro のまま) |
| `REFUND`・`REVOKE` | 取引の `revocationDate` を保存し、すぐに Pro でなくなる |
| `REFUND_REVERSED` | 取り消しの無い新しい署名で上書きし、Pro に戻る |
| `TEST`・Pro 以外の商品・取引の無い通知 | `ignored`(記録だけ) |

返金で Pro が止まっても、AI の回数の上限を返す処理(#548)とは別物です。ai は最大 5 分のキャッシュの後に新しい権利を読みます(`entitlement.ProChecker`)。

## App Store Connect への登録(人の作業)

1. App Store Connect → アプリ → **App 情報** → **App Store Server Notifications**
2. **本番サーバー URL** と **Sandbox サーバー URL** の両方に、上の prod の URL を入れる。バージョンは **Version 2**
3. 保存後、App Store Server API の「Request a Test Notification」(または ASC の「テスト通知を送信」)で `TEST` を送り、ログに `appstore notification: nothing to apply`(`notificationType=TEST`)が出ることを確かめる

## sandbox での確認(人の作業)

Sandbox のテスターで TestFlight の版から Pro を買い、App Store Connect の Sandbox の設定で更新間隔を短くして確かめます。

| 操作 | 数分以内に期待すること(`GET /v1/me/entitlements`) |
| --- | --- |
| 購入 | `isPro: true`(iOS の送信と `SUBSCRIBED` のどちらか早い方) |
| 自動更新を待つ | `pro.transactionId` と `expiresAt` が進む(`DID_RENEW`) |
| 設定アプリで解約 | `willRenew: false`、期限までは `isPro: true` |
| 期限切れ | `isPro: false`(`EXPIRED`) |
| 返金(Sandbox のテスターで返金を申請) | `revokedAt` が入り `isPro: false`(`REFUND`) |

Cloud Logging で `appstore notification:` の行を見ると、種類・`outcome`・`isPro` が分かります(uid はハッシュ)。

## 照合ジョブ

通知が届かなかった時(Apple 側の障害・こちらの 5xx が再送の上限を超えた)のために、`backend/cmd/appstore-reconcile` が App Store Server API の「Get All Subscription Statuses」で状態を取り直し、通知と同じ方法で重ねます。

- 対象: `appstore_transactions/*` のうち、結びついた uid の今の権利がその購読で、終わってから `-window`(既定 35 日)以内か、まだ終わっていないもの
- `-limit`(既定 1000)は 1 回の API 呼び出しの上限。`-dry-run` は書かずにログだけ
- 失敗した購読は数えて終了コード 1(次の実行でやり直す)
- 環境変数: `GOOGLE_CLOUD_PROJECT`・`FIRESTORE_DATABASE`・`APPSTORE_API_KEY_ID`・`APPSTORE_API_ISSUER_ID`・`APPSTORE_API_PRIVATE_KEY`(App Store Connect → ユーザとアクセス → 統合 → アプリ内課金 で作るキーの .p8 の中身。Secret Manager から注入し、リポジトリや手元のファイルに置かない)、`APPSTORE_BUNDLE_IDS` などは appconfig と同じ
- イメージ: `/app/appstore-reconcile`(`backend/Dockerfile` に同梱)

ローカルではエミュレータに向けて dry-run で試せます(キーは自分の環境変数で渡す)。

```bash
make -C backend emulator   # 別のターミナル
make -C backend run-appstore-reconcile   # 既定 -dry-run。APPSTORE_RECONCILE_FLAGS で変える
```

### Cloud Run Job と Scheduler(人の作業)

[account-purge.md](account-purge.md) の手順と同じ形です。stg と prod は Firestore を共有しているので、ジョブは **prod にだけ** 作ります。サービスアカウントは Firestore の読み書き(`roles/datastore.user`)と Secret の読み取りだけを持つ専用のものにします。

```bash
export PROJECT_ID=jphacks-kz2601
export REGION=asia-northeast1
export IMAGE="${REGION}-docker.pkg.dev/${PROJECT_ID}/backend/backend@sha256:<digest>"
export JOB_SA="appstore-reconcile@${PROJECT_ID}.iam.gserviceaccount.com"

gcloud run jobs create appstore-reconcile \
  --project="$PROJECT_ID" --region="$REGION" \
  --image="$IMAGE" --command=/app/appstore-reconcile --args="-dry-run" \
  --service-account="$JOB_SA" \
  --set-env-vars="GOOGLE_CLOUD_PROJECT=${PROJECT_ID},APPSTORE_API_KEY_ID=<key id>,APPSTORE_API_ISSUER_ID=<issuer id>" \
  --set-secrets="APPSTORE_API_PRIVATE_KEY=<secret 名>:latest" \
  --tasks=1 --max-retries=1 --task-timeout=30m
```

ログの `appstore-reconcile: done`(`checked`・`updated`・`failed`)を確かめてから `-dry-run` を外し、Cloud Scheduler から 1 日 4 回程度(例 `15 */6 * * *`)動かします。

## 確認と運用

| 知りたいこと | 見る所 |
| --- | --- |
| 通知が届いているか | Cloud Logging で `appstore notification:`。`appstore_notifications` の新しいドキュメント |
| 検証に失敗している | `appstore notification: rejected`(`reason`)。`environment` なら `APPSTORE_ENVIRONMENTS`、`bundle_id` なら `APPSTORE_BUNDLE_IDS` を確かめる |
| `unbound` が多い | iOS が購入後に `POST /v1/me/entitlements/apple` を送れていない |
| 記録が増え続ける | `appstore_notifications.expireAt`(処理から 30 日)に Firestore の TTL ポリシーを付けると自動で消える(人の作業・`gcloud firestore fields ttls update expireAt --collection-group=appstore_notifications --enable-ttl`) |
