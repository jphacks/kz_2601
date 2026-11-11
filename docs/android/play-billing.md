# Android の Tailor It Pro(Google Play Billing)

iOS の「Tailor It Pro」(月額の自動更新・`jp.kz2601.metamon.pro.monthly`)を Android でも買えるようにする仕組みと、まだ足りないもの。

## 今の動き

| 場面 | 動き |
|---|---|
| Pro かどうか | サーバーの答え(`GET /v1/me/entitlements`)。iOS で買った Pro も同じアカウントなら Android で効く |
| AI ボタン・AI と話す | 同意が無ければ同意の画面。同意があり Pro でなければ課金画面(サーバーの 402 でも出す) |
| 購入 | Play の購入の画面。`obfuscatedAccountId` に uid から作った値(iOS の `appAccountToken` と同じ計算)を入れる |
| 購入の後 | 購入をサーバーに送り(`POST /v1/me/entitlements/google`)、サーバーが Pro と認めた後だけ承認(acknowledge)する |
| 確かめられない時 | 承認しない。Play が 3 日以内に自動で返金する(お金を取りっぱなしにしない)。画面にもそう出す |
| 復元 | Play の購入を読み直して送り、サーバーの Pro を読み直す |
| モック | Play に触れず、購入はその場で Pro になる(iOS のモックと同じ) |

コード: `android/app/src/main/kotlin/jp/kz2601/metamon/purchase/`(`PlayBilling`・`ProStore`・`PaywallSheet`)、`android/services/src/main/kotlin/jp/kz2601/metamon/services/purchase/ProPurchases.kt`。

## まだ足りないもの

### backend: Play の購入を確かめる口(必須)

これが無いと、Android で買っても Pro にならず、3 日で返金される。案:

- `POST /v1/me/entitlements/google` 本文 `{"productId": "...", "purchaseToken": "..."}`、返りは `Entitlements`(Apple の口と同じ形)
- Google Play Developer API の `purchases.subscriptionsv2.get`(パッケージ名・purchaseToken)で確かめる。サービスアカウントで呼ぶ
- `lineItems[].productId` が許可リスト、`subscriptionState` が ACTIVE / IN_GRACE_PERIOD なら Pro。`expiryTime` を `expiresAt` に
- `externalAccountIdentifiers.obfuscatedExternalAccountId` が `entitlement.AccountToken(uid)` と合うかを確かめる(合わなければ 409 か、ログだけ)
- 1 つの購読を 1 つの uid にだけ結ぶ(Apple の `appstore_transactions` と同じ考え。`linkedPurchaseToken` もたどる)
- サーバーで承認(`purchases.subscriptions.acknowledge`)してもよい。その場合は端末の承認は二重でも害は無い
- 更新・解約・返金は Real-time developer notifications(Pub/Sub)で受ける(Apple の `POST /appstore/notifications` に当たる)
- 契約は `docs/backend/openapi-appconfig.yaml` に足す(この PR では足していない)

### 人がすること

1. Play Console で定期購入 `jp.kz2601.metamon.pro.monthly` と基本プラン(月額・自動更新)を作り、価格を決める。stg(`jp.kz2601.metamon.stg`)にも作るか決める
2. Play Console のライセンス テスターを入れ、内部テストの版で購入を試す
3. Google Cloud のサービスアカウントを作り、Play Console で「財務データの閲覧」と「注文と定期購入の管理」を与える。鍵は Secret Manager に置く(リポジトリに入れない)
4. Real-time developer notifications の Pub/Sub のトピックを作り、Play Console に設定する
5. プライバシーポリシー・利用規約・データセーフティの「購入履歴」の記載を確かめる(docs/legal は人が直す)
