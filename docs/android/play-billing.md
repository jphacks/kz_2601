# Android の Tailor It Pro(Google Play Billing)

iOS の「Tailor It Pro」(月額の自動更新・`jp.kz2601.metamon.pro.monthly`)を Android でも買えるようにする仕組みと、まだ足りないもの。

## 今の動き

| 場面 | 動き |
|---|---|
| Pro かどうか | サーバーの答え(`GET /v1/me/entitlements`)。iOS で買った Pro も同じアカウントなら Android で効く |
| AI ボタン・AI と話す | 同意が無ければ同意の画面。同意があり Pro でなければ課金画面(サーバーの 402 でも出す) |
| 購入 | Play の購入の画面。`obfuscatedAccountId` に uid から作った値(iOS の `appAccountToken` と同じ計算)を入れる |
| 購入の後 | 購入をサーバーに送り(`POST /v1/me/entitlements/google`)、サーバーが Pro と認めた後だけ承認(acknowledge)する(サーバーも承認する) |
| 確かめられない時 | 承認しない。Play が 3 日以内に自動で返金する(お金を取りっぱなしにしない)。画面にもそう出す |
| 復元 | Play の購入を読み直して送り、サーバーの Pro を読み直す |
| モック | Play に触れず、購入はその場で Pro になる(iOS のモックと同じ) |

コード: `android/app/src/main/kotlin/jp/kz2601/metamon/purchase/`(`PlayBilling`・`ProStore`・`PaywallSheet`)、`android/services/src/main/kotlin/jp/kz2601/metamon/services/purchase/ProPurchases.kt`。

## backend(できた)

`POST /v1/me/entitlements/google` と `POST /v1/googleplay/notifications` がある。契約は `docs/backend/openapi-appconfig.yaml`。

| 場面 | 動き |
|---|---|
| 購入を受ける | 本文 `{"productId", "purchaseToken"}`。Google Play Developer API の `purchases.subscriptionsv2.get` で読み直して確かめ、`Entitlements`(Apple の口と同じ形)を返す |
| Pro にする条件 | `productId` が許可リストにあり購入に入っている・状態が ACTIVE / IN_GRACE_PERIOD / CANCELED(自動更新を止めただけ)・`expiryTime` まで |
| アカウントの照合 | `obfuscatedExternalAccountId` が `entitlement.AccountToken(uid)`(iOS の `appAccountToken` と同じ関数)と合わなければ 409 `account_mismatch` |
| 1 つの購入 = 1 つの uid | `googleplay_purchases/{sha256(purchaseToken)}` に uid を記録(token そのものは保存しない)。別の有効な uid なら 409 `transaction_owned_by_other`。`linkedPurchaseToken` の前の購入も同じ規則 |
| 保存先 | `users/{uid}/entitlements/pro`(App Store と同じ文書・`platform: googlePlay`)。`GET /v1/me/entitlements` と ai の Pro 判定にそのまま効く |
| 承認 | 未承認の購入は、Pro にした後でサーバーも `purchases.subscriptions.acknowledge` で承認する(端末の承認と二重でも害は無い) |
| テスト購入 | ライセンス テスターの購入は `environment: Sandbox`。`APPSTORE_ENVIRONMENTS` で Production だけにするとテスト購入は Pro にならない |
| 更新・解約・返金 | Real-time developer notifications(Pub/Sub の push)を `POST /v1/googleplay/notifications` で受け、購入を読み直して保存する |
| 設定が無い時 | 鍵が無いサーバーは 503 `not_configured`。通知の口は 3 つの設定がそろった時だけ有効 |

コード: `backend/internal/googleplay/`(API の呼び出し)、`backend/internal/entitlement/google.go`・`google_notify.go`、`backend/cmd/appconfig/googleplay.go`(設定)。

### appconfig の設定

| 名前 | 中身 |
|---|---|
| `GOOGLE_PLAY_SERVICE_ACCOUNT_JSON` | サービスアカウントの鍵(JSON)。Secret Manager から `--set-secrets` で入れる。リポジトリ・手元のファイルに置かない |
| `GOOGLE_PLAY_PACKAGE_NAME` | 既定 `jp.kz2601.metamon`(stg は `jp.kz2601.metamon.stg`) |
| `GOOGLE_PLAY_PRODUCT_IDS` | 既定 `jp.kz2601.metamon.pro.monthly`(カンマ区切り) |
| `GOOGLE_PLAY_RTDN_AUDIENCE` | Pub/Sub の push サブスクリプションに設定する OIDC の audience(例: 通知の口の URL) |
| `GOOGLE_PLAY_RTDN_SERVICE_ACCOUNT` | push サブスクリプションが OIDC トークンに使うサービスアカウントのメール |

## 人がすること

1. Play Console で定期購入 `jp.kz2601.metamon.pro.monthly` と基本プラン(月額・自動更新)を作り、価格を決める。stg(`jp.kz2601.metamon.stg`)にも作るか決める
2. Play Console のライセンス テスターを入れ、内部テストの版で購入を試す
3. Google Cloud で Play の確認用のサービスアカウントを作り、Play Console の「ユーザーと権限」で招待して「財務データの閲覧」と「注文と定期購入の管理」を与える(Google Play Android Developer API を有効にする)
4. そのサービスアカウントの鍵を Secret Manager に置き、appconfig の Cloud Run に `--set-secrets="GOOGLE_PLAY_SERVICE_ACCOUNT_JSON=<secret 名>:latest"` で渡す。stg に入れるなら `GOOGLE_PLAY_PACKAGE_NAME` も
5. Real-time developer notifications: Pub/Sub のトピックを作り、`google-play-developer-notifications@system.gserviceaccount.com` に発行の権限を与えて Play Console(収益化のセットアップ)に設定する。push サブスクリプションを `https://<appconfig>/v1/googleplay/notifications` 宛てに認証つき(OIDC・サービスアカウントを指定)で作り、`GOOGLE_PLAY_RTDN_AUDIENCE`・`GOOGLE_PLAY_RTDN_SERVICE_ACCOUNT` を appconfig に設定する。Play Console の「テスト通知を送信」で `outcome: test` が返るのを確かめる
6. プライバシーポリシー・利用規約・データセーフティの「購入履歴」の記載を確かめる(docs/legal は人が直す)
