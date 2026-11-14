# Play Console: 裏での位置情報(ACCESS_BACKGROUND_LOCATION)の申告

Android 版は「場所に着いたら知らせる」(生成したアプリの通知の `near`)のために `ACCESS_BACKGROUND_LOCATION` を持つ。
Play Console の「アプリのコンテンツ > 機密情報へのアクセス権限 > 位置情報の利用許可」で申告する。下の文を貼り、動画は人が撮って付ける。

## 使っているところ

| 項目 | 中身 |
|---|---|
| 機能 | 生成したアプリに保存した場所(記録の場所の欄)の近くに着いたら、ローカル通知を出す |
| 仕組み | Google Play 開発者サービスの Geofencing API。着いたかは OS が端末の中で判定する |
| 見張る数 | 全アプリで 15 か所まで(近い順・新しい順) |
| 位置の送信 | しない。位置・着いた記録は端末の外に出さない(サーバーにも送らない) |
| 許可の聞き方 | 場所の通知があるアプリを開いた時に、まず前面の位置を聞く。許可されたら説明の画面を出し、そのあと OS の設定の画面で「常に許可」を選んでもらう。断られても他の機能は使える |
| 使わない時 | 場所の通知が無ければ見張らない。許可を外されたら見張りを全部外す |
| コード | `android/app/src/main/kotlin/jp/kz2601/metamon/notifications/LocalNotifications.kt`・`PlacesFirstAsk.kt`・`android/services/src/main/kotlin/jp/kz2601/metamon/services/notifications/GeofencePlanner.kt` |

## 申告の文(日本語)

> Tailor It は、利用者が話しかけて作ったアプリの中に「場所に着いたら知らせる」通知を設定できます。利用者が保存した場所(例: スーパー、駅、ジム)の近くに着いた時に、アプリを閉じていてもローカル通知でお知らせします。この機能は、アプリを開いていない時に着いたことを知るために、バックグラウンドの位置情報を使います。着いたかどうかは Google Play 開発者サービスのジオフェンスで端末の中で判定し、位置情報を端末の外(当社のサーバーを含む)に送ることはありません。場所の通知が無い時は位置情報を使いません。許可は、場所の通知があるアプリを開いた時に、説明の画面を出してから求めます。許可しなくても、それ以外の機能はすべて使えます。

## 申告の文(英語)

> Tailor It lets users add "notify me when I arrive" reminders to the apps they create by voice. When the user arrives near a place they saved (for example a supermarket, a station or a gym), the app shows a local notification even if it is closed. This feature uses background location so that arrival can be detected while the app is not open. Arrival is detected on the device with the Geofencing API of Google Play services, and location is never sent off the device, including to our servers. Location is not used when no place reminders are set. The permission is requested only when the user opens an app that has place reminders, after an in-app screen that explains why it is needed. Every other feature works without it.

## 人がすること

1. 申告の画面で、上の文を貼る(日本語の掲載情報なら日本語、英語なら英語)
2. 動画(30 秒ほど)を撮って YouTube の限定公開などに置き、その URL を付ける。映すもの:
   - 場所の通知があるアプリを開く → 位置の理由のダイアログ → OS のダイアログで許可
   - 説明の画面(「場所に着いた時に知らせるには」)→ 続ける → OS の設定で「常に許可」と「正確な位置情報を使用」
   - 保存した場所に着いて通知が出るところ(エミュレータの位置の変更でよい)
3. プライバシーポリシーとデータセーフティの「位置情報」は、端末の中だけで使い送らない(収集しない)扱いのまま。表現を変える時は docs/legal を人が直す
4. 審査で却下されたら、`AndroidManifest.xml` の `ACCESS_BACKGROUND_LOCATION` を外し、`DeviceCapabilities` の `notification.near` を `implemented = false` に戻す(AI が場所の通知を作らなくなる)
