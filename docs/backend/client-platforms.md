# クライアントの OS(`X-Client-Platform`)と、OS ごとに答えが変わる所

新しい OS のクライアントを足す時に、backend のどこを見るかの一覧。内容は `origin/epic/god`(abe167e0f)と #2284(Windows・Linux の最低と最新の版)のコードを grep して作った。

## 目次

- [要点](#要点)
- [ヘッダの値](#ヘッダの値)
- [backend がヘッダを使っている所](#backend-がヘッダを使っている所)
- [OS ごとに答えが変わる所](#os-ごとに答えが変わる所)
- [新しい OS を足す時に見る所](#新しい-os-を足す時に見る所)

## 要点

- `X-Client-Platform` は backend が**決まった語彙にそろえて**(`NormalizePlatform`・#2132)、ログに残し、**AI の回数制限の OS ごとの上書き**(`RATE_LIMIT_*_DAILY_<OS>`・#2133)に使う。答えをヘッダで変えているのはこの 1 か所だけ。
- それ以外の OS ごとの違いは、ヘッダではなく**本文の項目・呼ぶ API・応答のキー**で分けている(下の表)。

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
- backend の定数と語彙は `backend/internal/httpx/client.go`(`ClientPlatformHeader`・`Platform*`・`Platforms`)。`NormalizePlatform` が大文字小文字と前後の空白を無視して `ios` / `android` / `macos` / `windows` / `linux` にそろえ、空・知らない値・32 文字を超える値は `other` にする。

## backend がヘッダを使っている所

| ファイル | 何をするか |
|---|---|
| `backend/internal/httpx/client.go`(`ClientPlatform`・`ClientAttrs`) | ヘッダをそろえた値にし、4 つのヘッダをログの属性にする(`clientPlatform` はそろえた値で、ヘッダ無しでも `other` を必ず出す) |
| `backend/internal/httpx/middleware.go`(`ClientAttrs(r)` を足す所) | リクエストのログに付ける |
| `backend/internal/appcheck/appcheck.go`(`ClientAttrs(r)` を足す所) | App Check の結果のログに付ける |
| `backend/internal/ratelimit/middleware.go`(`AllowPlatform`)・`backend/internal/ratelimit/config.go`(`PlatformDailyLimit`) | ai の回数制限で、OS ごとの 1 日の上限の上書きを選ぶ(下の表) |
| `backend/internal/appconfig/usage.go`(`PlatformDailyLimit` を呼ぶ所) | `GET /v1/me/usage` の上限の表示を、ai と同じ OS ごとの上書きにそろえる |

この 5 つ以外で `X-Client-Platform` を読んでいる所は無い(`cmd/ai-session-report` はログの `clientPlatform` を OS ごとに数えるだけ)。

## OS ごとに答えが変わる所

| 何が変わるか | ファイル | 分け方 |
|---|---|---|
| AI の 1 人 1 日の上限 | `backend/internal/ratelimit/config.go`(`PlatformLimits`・`PlatformDailyLimit`) | **ヘッダで分ける唯一の所**。env `RATE_LIMIT_ANON_DAILY_<OS>` / `_USER_DAILY_<OS>` / `_PRO_DAILY_<OS>`(`<OS>` は `IOS` / `ANDROID` / `MACOS` / `WINDOWS` / `LINUX` / `OTHER`)で、そろえた値ごとに上書きする。未設定なら OS を問わず同じ上限。数える箱は OS をまたいで 1 つ。ヘッダは偽れるので下げる向きに使う([rate-limit.md](rate-limit.md#上限の既定値1-日000-jst-で切り替え)) |
| 最低の版・最新の版・ストアの URL(`GET /v1/client-config`) | `backend/internal/appconfig/clientconfig.go`(`EnvMin*Build`・`ClientConfigValues`) | 応答に OS ごとのキー(`minIOSBuild`・`minAndroidBuild`・`minMacOSBuild`・`minWindowsBuild`・`minLinuxBuild` と `latest*`・ストアの URL)を全部並べ、クライアントが自分の分を読む。env は `MIN_IOS_BUILD`・`MIN_ANDROID_BUILD`・`MIN_MACOS_BUILD`・`MIN_WINDOWS_BUILD`・`MIN_LINUX_BUILD` と `LATEST_*_BUILD`(#2131)。ストアの URL は iOS・Android・Mac だけ |
| AI に作らせる部品・機能(審査用の版・Mac で描けない部品も) | `backend/internal/ai/generate.go`(`AllowedComponents`)・`backend/internal/ai/chat.go`(同)・`backend/internal/ai/restrict.go` | 本文の `allowedComponents`・`disabledFeatures`・`supportedFeatures`。どの部品を外すかはクライアントが決めて送る(iOS は `ClientCapabilities.swift` の `reviewAllowedComponents`・`macUnavailableComponents`) |
| 申告していない部品の差し替え(取得の時) | `backend/internal/appconfig/substitute.go`(`fetchSubstitutes`) | 表は 1 行(`musicPlayer` はクライアントの代わりの表示に任せる)。`substituteUndeclared` は今はテストからしか呼ばれていない |
| プッシュの宛先の登録(`PUT /v1/me/devices/{installationId}`) | `backend/internal/appconfig/devices.go`(`deviceFrom`) | 本文の `platform`: `ios`(空も。APNs のトークン)/ `android`(FCM のトークン)。それ以外は 400 |
| プッシュを送る先 | `backend/internal/runner/notify/platform.go`(`PlatformSender.Send`)・`backend/internal/runner/scope/scope.go`(`PushDevices`) | 登録した `platform` で APNs / FCM を選ぶ。iOS は APNs の環境(`environment`)で絞り、Android は全部送る。FCM の鍵が無ければ Android 宛は送らない |
| Pro の購入の確かめ | `backend/internal/entitlement/handler.go` | API が別: `POST /v1/me/entitlements/apple`(App Store)・`POST /v1/me/entitlements/google`(Google Play) |

## 新しい OS を足す時に見る所

- ヘッダの値を決め、上の「ヘッダの値」の表に 1 行足す。backend の語彙(`httpx.Platforms`・`NormalizePlatform`)にも足す。足さないと `other` にそろえられ、回数制限の上書きも `OTHER` の値になる。
- 最低の版を強制したいなら `clientconfig.go` に env とキーを足す(今は iOS・Android・macOS・Windows・Linux)。
- プッシュを受けるなら `devices.go` の `platform` と `notify/platform.go` の送り先を足す(今は `ios` と `android` だけ)。
- 描けない部品は `allowedComponents` で送る(ヘッダで絞る作りにはなっていない)。
