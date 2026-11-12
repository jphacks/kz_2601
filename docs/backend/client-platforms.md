# クライアントの OS(`X-Client-Platform`)と、OS ごとに答えが変わる所

新しい OS のクライアントを足す時に、backend のどこを見るかの一覧。内容は `origin/develop`(748767fd6)のコードを grep して作った。

## 目次

- [要点](#要点)
- [ヘッダの値](#ヘッダの値)
- [backend がヘッダを使っている所](#backend-がヘッダを使っている所)
- [OS ごとに答えが変わる所](#os-ごとに答えが変わる所)
- [新しい OS を足す時に見る所](#新しい-os-を足す時に見る所)

## 要点

- `X-Client-Platform` は**ログに残すだけ**。今の backend に、このヘッダの値で答えを変えている所は無い。
- OS ごとの違いは、ヘッダではなく**本文の項目・呼ぶ API・応答のキー**で分けている(下の表)。

## ヘッダの値

全部のリクエストに `X-Client-Platform`・`X-Client-Version`・`X-Client-Build`・`X-Client-DSL-Version` を付ける。

| 値 | 送るクライアント | 付けている所 |
|---|---|---|
| `ios` | iOS アプリ | `ios/Packages/MetamonDSL/Sources/MetamonDSL/Model/ClientCapabilities.swift`(`platform`)→ `ios/Packages/MetamonServices/Sources/MetamonServices/Live/ClientInfo.swift` |
| `macos` | Mac アプリ(同じ Swift のコードを `os(macOS)` でビルド) | 同上 |
| `android` | Android アプリ | `android/services/src/main/kotlin/jp/kz2601/metamon/services/http/ClientInfo.kt`(`PLATFORM`) |
| `windows` | デスクトップ版(Windows) | `desktop/app/src/main/kotlin/jp/kz2601/metamon/desktop/net/ClientPlatform.kt`(共有の `ClientInfo` の `android` を OkHttp の段で上書き) |
| `linux` | デスクトップ版(Linux。BSD など知らない OS もこれ) | 同上 |
| `macos` | デスクトップ版を macOS で動かした時 | 同上 |

- `macos` は Mac アプリとデスクトップ版の両方が送る。ログで分けたい時は `User-Agent`(デスクトップ版は `TailorIt-Desktop/<版> (<OS> <OS の版>; <arch>)`)を見る。
- backend の定数は `backend/internal/httpx/client.go`(`ClientPlatformHeader`)。値の一覧は持っておらず、来た値をそのまま 32 文字まで残す。

## backend がヘッダを使っている所

| ファイル | 何をするか |
|---|---|
| `backend/internal/httpx/client.go`(`ClientAttrs`) | 4 つのヘッダをログの属性(`clientPlatform` など)にする |
| `backend/internal/httpx/middleware.go`(`ClientAttrs(r)` を足す所) | リクエストのログに付ける |
| `backend/internal/appcheck/appcheck.go`(`ClientAttrs(r)` を足す所) | App Check の結果のログに付ける |

この 3 つ以外で `X-Client-Platform` を読んでいる所は無い。

## OS ごとに答えが変わる所

| 何が変わるか | ファイル | 分け方 |
|---|---|---|
| 最低の版・最新の版・ストアの URL(`GET /v1/client-config`) | `backend/internal/appconfig/clientconfig.go`(`EnvMin*Build`・`ClientConfigValues`) | 応答に OS ごとのキー(`minIOSBuild`・`minAndroidBuild`・`minMacOSBuild` と `latest*`・ストアの URL)を全部並べ、クライアントが自分の分を読む。env は `MIN_IOS_BUILD`・`MIN_ANDROID_BUILD`・`MIN_MACOS_BUILD` など。Windows・Linux のキーはまだ無い |
| AI に作らせる部品・機能(審査用の版・Mac で描けない部品も) | `backend/internal/ai/generate.go`(`AllowedComponents`)・`backend/internal/ai/chat.go`(同)・`backend/internal/ai/restrict.go` | 本文の `allowedComponents`・`disabledFeatures`・`supportedFeatures`。どの部品を外すかはクライアントが決めて送る(iOS は `ClientCapabilities.swift` の `reviewAllowedComponents`・`macUnavailableComponents`) |
| 申告していない部品の差し替え(取得の時) | `backend/internal/appconfig/substitute.go`(`fetchSubstitutes`) | 表は 1 行(`musicPlayer` はクライアントの代わりの表示に任せる)。`substituteUndeclared` は今はテストからしか呼ばれていない |
| プッシュの宛先の登録(`PUT /v1/me/devices/{installationId}`) | `backend/internal/appconfig/devices.go`(`deviceFrom`) | 本文の `platform`: `ios`(空も。APNs のトークン)/ `android`(FCM のトークン)。それ以外は 400 |
| プッシュを送る先 | `backend/internal/runner/notify/platform.go`(`PlatformSender.Send`)・`backend/internal/runner/scope/scope.go`(`PushDevices`) | 登録した `platform` で APNs / FCM を選ぶ。iOS は APNs の環境(`environment`)で絞り、Android は全部送る。FCM の鍵が無ければ Android 宛は送らない |
| Pro の購入の確かめ | `backend/internal/entitlement/handler.go` | API が別: `POST /v1/me/entitlements/apple`(App Store)・`POST /v1/me/entitlements/google`(Google Play) |

## 新しい OS を足す時に見る所

- ヘッダの値を決め、上の「ヘッダの値」の表に 1 行足す。backend はログに出るだけなので、値を足すだけなら backend の変更は要らない。
- 最低の版を強制したいなら `clientconfig.go` に env とキーを足す(今は iOS・Android・macOS だけ)。
- プッシュを受けるなら `devices.go` の `platform` と `notify/platform.go` の送り先を足す(今は `ios` と `android` だけ)。
- 描けない部品は `allowedComponents` で送る(ヘッダで絞る作りにはなっていない)。
