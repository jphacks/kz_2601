# Mac の OS との連携(メニューバー・ウィジェット・Handoff など)

Mac 版が macOS の仕組み(メニューバー・ウィジェット・Handoff・Quick Look・サービスメニュー・連係カメラ・プッシュ通知など)とどうつながるかの決まりです。部品ごとの右クリック・ドラッグ&ドロップ・コピー&ペーストは [interactions.md](interactions.md) にあります。

- どれも **生成したアプリの DSL は変えません**。アプリの殻(`ios/MetamonMac/`)と、iOS と共有するファイル(`ios/Shared/`・`ios/MetamonWidget/` など)だけで作っています。
- 審査用の版(`APP_STORE_REVIEW`)では、iOS と同じく Handoff・プッシュ・近くの端末に送る・つないだサービスを外します([review-build.md](../ios/review-build.md))。
- iOS 側の仕組みは [architecture.md](../ios/architecture.md) の各章にあります。ここでは Mac で違う所だけを書きます。

## 目次

- [メニューバーとログイン時に開く](#メニューバーとログイン時に開く)
- [ウィジェット](#ウィジェット)
- [Handoff](#handoff)
- [アプリごとのウインドウ](#アプリごとのウインドウ)
- [Quick Look](#quick-look)
- [サービスメニュー「Tailor It に送る」](#サービスメニューtailor-it-に送る)
- [連係カメラ(iPhone で撮る・書類をスキャン)](#連係カメラiphone-で撮る書類をスキャン)
- [プッシュ通知](#プッシュ通知)
- [近くの端末に送る(nearbyShare)](#近くの端末に送るnearbyshare)
- [ショートカット・Siri(App Intents)](#ショートカットsiriapp-intents)
- [設定の「つないだサービス」](#設定のつないだサービス)
- [Mac でしないこと(一言で知らせる)](#mac-でしないこと一言で知らせる)
- [テスト(UI テスト・見た目の比較)](#テストui-テスト見た目の比較)

## メニューバーとログイン時に開く

設定 > 一般 にあります。どちらも既定は OFF です(#1848)。

| 項目 | 決まり |
| --- | --- |
| メニューバーに表示する | アイコンを押すと窓が開き、選んだ作ったアプリの stat を最大 3 個と「アプリを開く」「声・文字で頼む」を出す。選んだアプリは次も同じ(消したアプリなら一覧の先頭) |
| stat の並びと数え方 | ウィジェットと同じ(`WidgetSnapshotWriter`。最初の画面 → タブ順 → 残り・`Aggregator`)なので、アプリの中の数字とそろう。記録をまだ読めていない時は「—」。ショートカットで記録が増えた時も数え直す |
| 開く・頼む | ショートカットの「開く / 話しかける」と同じ道(`IntentHost`)。ウインドウが無ければ作る |
| 作り | SwiftUI の `MenuBarExtra` ではなく `NSStatusItem` + `NSPopover`(`App/MenuBar/`)。`MenuBarExtra` の scene は出し分けられず、OFF でも起動時に固まったため |
| ログイン時に開く | `SMAppService.mainApp`。自分からは登録せず、状態は毎回 OS に聞く(システム設定で外されても合わせる)。許可待ち・失敗は一言で出す。未署名のビルドでは登録できないことがある |

## ウィジェット

拡張 `MetamonMacWidget` を本体に埋め込みます(`Contents/PlugIns/`・#984)。iOS の [§12 ホーム画面ウィジェット](../ios/architecture.md#12-ホーム画面ウィジェットwidgetkit実装済みissue-39) と同じ仕組みで、違う所は次のとおりです。

| 項目 | Mac |
| --- | --- |
| 大きさ | 小・中・大。accessory(ロック画面)は Mac に無い。特大は入れていない |
| 見た目・受け渡し | iOS の `MetamonWidget/Views/`・`Shared/WidgetSnapshot.swift`・文言を共有する |
| 出すアプリ | 最後に記録が動いた作ったアプリ。同梱の例は書かない(ウインドウを閉じると記録が消えるため) |
| 押した時 | `tailorit://widget?app=<appId>` でそのアプリを開く(`MacWidgetLinkReceiver` → `IntentHost` の「開く」)。appId が無ければ本体を前に出すだけ |
| ID | 拡張の Bundle ID・App Group は iOS と同じ(`jp.kz2601.metamon(.stg).widget`・`group.jp.kz2601.metamon`) |
| 消す | サインアウト・アカウント削除で App Group を空にし、`reloadAllTimelines` |

## Handoff

開いている生成アプリの画面を、同じ Apple アカウントの iPhone と Mac で引き継ぎます(#1851)。

| 項目 | 決まり |
| --- | --- |
| 種類 | `jp.kz2601.metamon.view`(両方の Info.plist の `NSUserActivityTypes`)。中身は `appId`・`screenId`・`recordId` だけ(`ios/Shared/Runtime/HandoffActivity.swift`) |
| 載せないもの | 記録の中身・題名・書きかけのフォーム。題名はアプリ名だけ。Handoff だけに出し、検索・予測には出さない |
| 出す時 | Mac はサーバーにある自分のアプリだけ(同梱の例・端末だけのアプリは iPhone で開けない)。iOS は生成したアプリを開いている時だけ |
| `webpageURL` | この端末で発行済みのシェアリンク(https)がある時だけ付ける |
| 受ける | Mac は `MacLinkReceiver` が受け、押された通知と同じ道でアプリを開いてから画面と記録を開く。iOS はシェアリンクの受け取りと同じ所で受け、サインインと bootstrap の後に開く。保存していない入力があれば破棄の確認を通す |
| 保留 | 開けない間は 60 秒だけ持ち、過ぎたら捨てる。無い画面ならアプリを開くだけ |

## アプリごとのウインドウ

生成アプリを別のウインドウで開けます(#1853・`App/Shell/MacAppWindows.swift`)。

- 開き方: サイドバーの行の右クリック「新しいウインドウで開く」か、行の ⌥クリック。同じアプリのウインドウがあれば前に出す。
- 中身はメインと同じ `MacRootView` で、サイドバーは畳んで開く(⌃⌘S で出せる)。選んでいるアプリ・お知らせ・新着の数はウインドウごと。
- 同じアプリを 2 枚で開いた時は、片方が端末に書いたらもう片方がすぐ読み直す(`RecordBook.onLocalWrite` → `MacRecordEngineLease.onLocalChange`)。
- 大きさと位置はアプリごとに覚える。ここでアプリを選び直しても、次の起動で開くアプリは変えない。

## Quick Look

開いている記録の写真・動画・録音を Quick Look で見ます(#1850・`App/Runtime/MacQuickLook.swift`)。

- 開き方: ファイル > クイックルック(⌘Y)か、中央の列でスペース(入力欄が受け取らなかった時だけ。見る物が無ければふつうのスペース)。
- 見せる物は一番上の画面の記録から集める。フォームでは入力中の値。隠す項目と https の写真は出さない。
- 写真は一時フォルダ(`metamon-mac-preview-`)に書き出し、ランタイムを閉じた時と起動・終了の片付けで消す。

## サービスメニュー「Tailor It に送る」

ほかのアプリで選んだ文字を、右クリック > サービス > 「Tailor It に送る」(英語 Send to Tailor It)で送れます(#1849・`App/Compose/MacTextService.swift`)。

- 届いた文字は ⌘K(文字で頼む)と同じ会話の入力欄に入れる。**送信はしない**(送るかは本人が決める)。記録にはしない。
- 中央にアプリが開いていればそのアプリを直す会話、無ければ新しく作る会話。会話が開いていれば書きかけの次の行に足す。
- 受けるのは文字だけ。前後の空白を除いて空なら何もしない。

## 連係カメラ(iPhone で撮る・書類をスキャン)

photoField に「iPhone から写真を撮る / 書類をスキャン」のボタンがあります(#1852・`ContinuityCameraImport.swift`)。

- 押すと近くの iPhone ごとに「写真を撮る / 書類をスキャン」のメニューが出る(`NSMenuItem.importFromDeviceIdentifier`)。
- 受けるのは画像(png / tiff / jpeg / heic)だけで、ファイル選択・貼り付けと同じ data URL(800px 以下の JPEG)にする。`analyze` があればすぐ読み取る。
- Mac のカメラのボタンとは別。カメラの無い Mac でも隠さない。

## プッシュ通知

iOS の [§5.1 プッシュ通知](../ios/architecture.md#51-プッシュ通知269) と同じ共有の `PushDeviceRegistrar` を使い、OS とのやり取りだけを `App/Push/MacPushHost.swift` に持ちます(#982)。

| 項目 | Mac |
| --- | --- |
| 登録 | 通知が許可済みでサインインしている時だけ、サインインと前面に戻った時に `NSApplication.registerForRemoteNotifications`。トークンは `MacAppDelegate` で受ける |
| platform | サーバーに `macos` を送らない(サーバーは `ios` / `android` だけ受ける)。Bundle ID が iOS と同じなので送り先も同じで、Mac かどうかは `os`(例 `macOS 15.1`)で分かる |
| 消す | サインアウト・アカウント削除・アカウントの切り替えの前。サーバーでの削除や切り替えに失敗したら登録し直す |
| タップ | 本文の `appId` のアプリを開く(`MacNotificationRouter` の `.app(appId)`)。起動直後で一覧にまだ無ければ、一覧が載るまで最大 30 秒待つ。過ぎたら開かない。サインアウト中は開かない |
| 同意 | アプリの設定シートの「他の人の行動で届く通知」(作成者には出さない) |
| entitlement | `MetamonMac.entitlements` の `com.apple.developer.aps-environment` = `$(METAMON_APS_ENV)`。審査用には入れない。環境の判定は `Contents/embedded.provisionprofile` を読む |

## 近くの端末に送る(nearbyShare)

iOS の `NearbyService`(MultipeerConnectivity)をそのまま使い、Mac の `nearbyShare` で近くの iPhone・iPad・Mac に記録を送れます(曲のリクエストも同じ・#964)。

- 相手選びは `MacNearbySendSheet`、届いた時の「受け取る / 受け取らない」はアラート。確認は一番最近前面にしたウインドウにだけ出す。
- 待ち受けは、Mac が前面で、前面のウインドウのアプリが `nearbyShare` か曲のリクエストを使っている間だけ(使わないアプリではローカルネットワークの許可を求めない)。
- Info.plist に `NSLocalNetworkUsageDescription`・`NSBonjourServices`(`_tailorit-mc._tcp` / `_udp`)、通常の版の entitlements に `com.apple.security.network.server`。
- すれ違い通信(mode:passby)は Mac に UWB が無いので「この Mac では使えません」の枠にする。

## ショートカット・Siri(App Intents)

iOS と同じ Intent を Mac でも使います。Mac 用は本体との橋渡し `MacIntentHost` だけです([§15 Siri / ショートカット](../ios/architecture.md#15-siri--ショートカットapp-intents実装済みissue-80) の Mac の行・#1034)。メニューバー・ウィジェット・プッシュのタップも同じ `IntentHost` を通ります。

## 設定の「つないだサービス」

設定(⌘,)の「通知」と「プライバシー」の間のタブです(#1714・`App/Settings/MacConnectedServicesSettings.swift`)。

- 一覧・つなぐ・つなぎ直す・切る・状態は iOS の `ConnectedServices.swift` を共有する。トークンは runner だけが持つ([external-apis.md](../backend/features/external-apis.md))。
- つなぐ時は `ASWebAuthenticationSession`(前面のウインドウ・ephemeral)。戻り先 `tailorit://connected?...` は `MacLinkReceiver` が受けて一覧を取り直す。
- サーバーが外の API を止めている時は「この端末では使えません」。Pro でない人は課金画面を出す。審査用の版には出さない。

## Mac でしないこと(一言で知らせる)

| 場面 | 決まり |
| --- | --- |
| `Field.health` のある記録を保存 | ヘルスケアには書かない。新しい記録で health の項目に値がある時、アプリごとに 1 回だけ「この Mac ではヘルスケアに書き込みません」を出し、VoiceOver でも読む(#958・`MacHealthNote`) |
| 日本語の変換中に送る・保存する | 変換中の文字を確定してから送る・保存する(`NSTextView.commitMarkedTextNow()`)。会話の入力欄で変換中の Return は確定だけで、送らない(#1047) |

## テスト(UI テスト・見た目の比較)

どちらも既定では回しません(CI は `MetamonMacTests` だけ)。

| テスト | 回し方 | 中身 |
| --- | --- | --- |
| UI テスト `MetamonMacUITests` | scheme では `skipped`。Xcode で手元から回す(署名が要る) | キーボードだけで殻を通す `KeyboardShellUITests`(#1140)、carousel・disclosure・見出しの読み上げの木 `AccessibilityTreeUITests`(#1205・`METAMON_MOCK=1` で同梱の例を開く) |
| 見た目の比較 `MacSnapshotTests` | `TEST_RUNNER_METAMON_SNAPSHOTS=1 xcodebuild -scheme MetamonMac test -only-testing:MetamonMacTests CODE_SIGNING_ALLOWED=NO`。撮り直しは `TEST_RUNNER_METAMON_SNAPSHOT_RECORD=1` | デモの DSL 8 つの最初の画面を 3 つの大きさ × ライト / ダークで描き、`ios/MetamonMacTests/__Snapshots__/` と比べる(#1136)。macOS・フォントの版で文字が変わるので CI では回さない。比べ方だけの `MacSnapshotDiffTests` は CI でも回る |
