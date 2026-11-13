# 端末の登録・プッシュのトークン・他の人の行動で届く通知の同意(#269・#339・#918)

> [バックエンド設計](../architecture.md) から分けた機能の文書。API は [openapi-appconfig.yaml](../openapi-appconfig.yaml) の `/me/devices/{installationId}`・`/me/notify-consent/{appId}`。送る側(runner・宛先の判定・APNs / FCM)は [プッシュ通知の端末と受け取りの同意](notifications.md) と [runner.md](../../infra/runner.md#プッシュ通知を端末に届ける) にあり、ここでは繰り返さない。

## 目次
- [1. 何をするか](#1-何をするか)
- [2. API](#2-api)
- [3. 保存する物と消える時](#3-保存する物と消える時)
- [4. 端末ごとの違い](#4-端末ごとの違い)
- [5. 決めること](#5-決めること)

## 1. 何をするか

- 端末(アプリの 1 回のインストール)が、プッシュを受けるトークンを appconfig に預ける。runner は宛先の判定を通った人の分だけそれを読んで送る
- 他の人の行動(shared のアプリで誰かが記録を足した等)で届く通知を、アプリごとに受け取るかを本人が決める(同意)
- どちらも本人の分だけ。パスに uid は無く、トークンはログにもレスポンスにも出さない(`backend/internal/appconfig/devices.go`)

## 2. API

| API | 本文・返り | 失敗 |
|---|---|---|
| `DELETE /v1/me/devices/{installationId}` | 無し → 204。無くても 204(冪等)。サインアウト・退会・通知の許可を切った時 | `installationId` の形が違う 400 |
| `PUT /v1/me/devices/{installationId}` | `{token, platform?, environment, appVersion?, os?}` → 200 で保存した端末(トークンは返さない)。同じ `installationId` は上書き(`createdAt` は保つ) | 形が違う 400(`field` に欄の名前) |
| `PUT /v1/me/notify-consent/{appId}` | `{allowed}` → 200 で `{appId, allowed, updatedAt}`。無効はいつでも通る(アプリが消えた後も) | `allowed` が無い 400・自分のコピーが無い 404・shared のデータがあって参加していない 403 `not_member`(どちらも有効にする時だけ) |
| `PUT /v1/me/notify-consent/feedback-fixed`・`GET` 同 | 送ったフィードバックが直った時の push の同意(#374)。`{appId}` より具体的なパスなので先に当たる | [feedback-loop.md](feedback-loop.md) |

欄の決まり(`deviceFrom`):

| 欄 | 決まり |
|---|---|
| `appVersion`・`os` | 任意・前後の空白を落として 32 文字まで |
| `environment` | `sandbox` / `production`。Android は無ければ `production`(Android のトークンはこの値で絞らない) |
| `installationId` | `^[A-Za-z0-9_-]{8,64}$` |
| `platform` | `ios`(既定・APNs)/ `android`(FCM・#918)。それ以外は 400 |
| `token`(android) | FCM の登録トークン。`^[A-Za-z0-9_:.-]{32,1000}$`。大文字小文字を区別するので送られたまま保存 |
| `token`(ios) | APNs の 16 進(32〜100 バイト)。小文字にして保存 |

## 3. 保存する物と消える時

| 置き場 | 中身 |
|---|---|
| `device_tokens/{sha256(token)}` | `{uid, installationId}`。今そのトークンを持つ端末の索引(トークンそのものは ID にしない) |
| `users/{uid}/devices/{installationId}` | `store.Device`: `token`・`platform`・`environment`・`appVersion`・`os`・`createdAt`・`updatedAt` |
| `users/{uid}/notifyConsent/{appId}` | `{allowed, updatedAt}`。文書が無い = 受け取らない |

| 消える時 | 何が消えるか |
|---|---|
| 1 人 10 台を超えた(`store.MaxDevicesPerUser`) | その人の端末で最後の更新が古いもの |
| アカウントの削除(`DELETE /v1/me`・論理削除) | その人の端末を全部(#337: プッシュをすぐ止める)。30 日後の消去で残りの端末と同意も消す([account-deletion.md](account-deletion.md)・[account-purge.md](../../infra/account-purge.md)) |
| 同じトークンが別の (uid, installationId) で登録された | 前の方の端末(アカウントを替えた端末に前の人の通知が出ないように) |
| 端末が `DELETE` を送った | その端末 |
| トークンの無効(APNs 410 Unregistered・400 BadDeviceToken / FCM UNREGISTERED・SENDER_ID_MISMATCH) | runner がその uid のそのトークンの端末を消す(`notify.Result.Unregistered`・`runner.dropUnregistered`) |

## 4. 端末ごとの違い

| 端末 | トークン・`platform` | `environment` | 同意 | 登録を消す時 |
|---|---|---|---|---|
| Android | FCM・`android`(#918。runner が FCM で送るのは #1193 から) | 常に `production`(FCM に区別は無い) | `NotifyConsentToggle` が呼ぶ。控えは端末に置き、サインアウトで忘れる | iOS と同じ(`PushDeviceRegistrar.unregister`・`authorizationRevoked`) |
| iOS | APNs・`ios`(欄を送らない = `ios`) | 署名の `aps-environment` から(`PushEnvironment.current`・シミュレータは `sandbox`) | 読む API が無いので、この端末で変えた値を `UserDefaults` に控える(ログアウトで消える) | サインアウト・退会・通知の許可を切った時(`PushDeviceRegistrar.unregister`) |
| Mac | APNs・`ios`(#982。iOS と同じ Bundle ID = 同じトピックで届くので `platform` を分けない) | `Contents/embedded.provisionprofile` の `aps-environment` から | iOS と同じ共有パッケージ | iOS と同じ(`MacPushHost`) |
| Windows・Linux(`desktop/`) | 登録しない(プッシュの受け口が無い) | - | - | - |

- 審査用の版(`APP_STORE_REVIEW`)は iOS・Mac ともプッシュの通信ごと外す
- 端末側の `installationId` は端末に 1 回作って保存する(`metamon.push.installationId`・サインアウトでも変えない)
- iOS の流れは [iOS アーキテクチャ §5.1](../../ios/architecture.md#51-プッシュ通知269)

## 5. 決めること

- 同意を読む API が無い(端末は自分で変えた値を控えるだけ)。別の端末で変えた同意を揃える時に `GET /v1/me/notify-consent/{appId}` を足すか(推測: 今は 1 人 1 端末が多いので要っていない)
- Windows・Linux でプッシュを受けるか(受けるなら `platform` に値を足す)
