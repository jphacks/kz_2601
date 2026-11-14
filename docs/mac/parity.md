# Mac の機能の突き合わせの表(iOS の Features の入口 × Mac)

iOS の `ios/Metamon/Features/` の入口を 1 行ずつ並べ、Mac 版(`ios/MetamonMac`・`ios/Shared`)の状態と Issue 番号を書く。iOS の `AppRuntime` と Mac の `MacAppRuntime` の型ごとの対応は [architecture.md](architecture.md)、OS の仕組みとの連携は [system-integration.md](system-integration.md)、操作は [interactions.md](interactions.md)。親: #1040。

- 調べた元: `origin/epic/god`(8cd11b78b)の `ios/MetamonMac`・`ios/Shared`・`ios/Packages/MetamonDSL`(`ClientCapabilities`)と `docs/mac/*`。開いている PR は 2026-11-13 に `gh pr list` で見た。
- コード・決定事項は変えていない。「推測」と書いた所は確かめていない。

## 目次

- [見方](#見方)
- [表](#表)
- [Mac に無い物の数え方](#mac-に無い物の数え方)
- [決めること](#決めること)

## 見方

| 状態 | 意味 |
| --- | --- |
| 有る | `origin/epic/god` に入っている(iOS と同じ型を `ios/Shared` から使う物を含む) |
| 一部 | 一部だけ入っている(何が残りかを書く) |
| PR #N | まだ入っておらず、開いている PR #N にある |
| 代わり | iOS と別の形で同じ目的を満たす |
| 無い | 入っておらず、開いている PR も見つからない。Issue があれば番号を書く |
| 作らない | Mac に当たる物が無い、または決まりで作らない |

## 表

1 行 1 入口・iOS の Features のフォルダ名順。フォルダの中の個別の入口(`AuthorProfile`・`ExtDenialSheet`・`HeldBuild`)は別の行にした。

| iOS の入口 | 何をするか | Mac | 根拠・Issue・PR |
| --- | --- | --- | --- |
| Accessibility | Assistive Access(iOS の簡易モード)と VoiceOver の知らせ | 一部 | VoiceOver の知らせは有る(`Shell/MacVoiceOver.swift`・`Theme/MacAccessibility.swift`)。Assistive Access は iOS の機能なので作らない(推測)。文字の大きさ(⌘+ / ⌘-)は PR #3187(#2854)、点検の記録は PR #3189 |
| AppLimit | アプリの数の上限に当たった時の置き換えの選び方 | 有る | `Compose/MacAppLimitSheet.swift`(置き換え候補は共有の `AppReplacePlan`)。コインの案内は #2281 |
| Auth | サインイン・ログアウト・アカウントの削除 | 一部 | ログアウトとアカウントの削除は有る(`Settings/MacSettingsView.swift`・`Services/MacAccount.swift`)。サインインは Apple のボタン(`MacAppleSignInButton`)だけ。Google など他の方法は無い(推測・iOS の `AccountLink` との差は未確認) |
| Automations | 自動の動きの一覧・止める・続けて失敗した時の「直しますか?」 | 一部 | 一覧は有る(`Settings/MacAppSettingsSheet.swift`・共有の `AutomationsModel`)。「直しますか?」は PR #3319。依頼文の組み立ては有る(`+FixRequest`) |
| Books | 本の検索(lookupBook) | 有る | 共有の `BookLookupSteps` |
| Calls | 通話の画面(CallView) | 無い | Mac のコードに通話の画面が見当たらない。Issue は見つからない。運転中・通話の方針は [424-calls-and-realtime.md](../decisions/424-calls-and-realtime.md) |
| Coins | コインの残高・買う・履歴・プラン比べ | 無い | 残高と履歴は #2280。足りない時のシートとお店は #2281。「約 N コイン」の目安は #3409。プランを比べるシートは #3410 |
| Connections | つないだサービスの設定 | 有る | `Settings/MacConnectedServicesSettings.swift` |
| Connections/ExtDenialSheet | 外の API の断りの案内 | 無い | #3078 |
| Consent | AI に送る同意・計測・プライバシーの設定 | 有る | `Onboarding/MacAIConsentSheet.swift`・設定の「プライバシー」・`Services/MacAIButtonMetrics.swift` |
| Debug | 開発用メニュー | 一部 | DEBUG の環境変数の上書きだけ有る(`METAMON_AI_CONSENT`・`METAMON_TUTORIAL`・`METAMON_BETA_WELCOME`・`METAMON_FORCE_UPDATE`)。メニューは無い(#2087・#2247・#2248・#3246・#3247)。作り物の項目は #3413 #3414 #3415 #3417。ヘッドレスの移動は #3412。docs は #3226 |
| Feedback | フィードバックを送る画面 | 有る | `Feedback/MacFeedbackComposerView.swift`。「直ったら知らせてほしい」は PR #3098 |
| Feeds | 外の公開データの feed | 有る | `Runtime/MacAppRuntime+Feeds.swift`。provider の稼働状況の確認は #1952 |
| Follows | フォロー | 作らない | アクションが来たら「フォローは Mac では使えません」と一言で知らせる([architecture.md](architecture.md)) |
| GridEdit | 画面の部品の並び(area)を直す編集 | PR #2701 | #2323 |
| Health | ヘルスケアの数字 | 作らない | healthStat は「この Mac では使えません」(`ClientCapabilities.macUnavailableComponents`)。`Field.health` には書かず一言で知らせる(`MacHealthNote`・#958・[system-integration.md](system-integration.md#mac-でしないこと一言で知らせる)) |
| Intents | ショートカット・Siri・Spotlight からの入口・フォーカスフィルタ | 一部 | ショートカット・Siri の「開く / 話しかける」・記録の追加は有る(`Intents/MacIntentHost.swift`・#1034)。集中モード中かを読む口は PR #2485(#2328)、フィルタで絞るのは #3081。ウィジェットのボタンは #2279。docs は PR #3438 |
| Location | 位置の追跡バー | 作らない | locationTracker は「この Mac では使えません」 |
| Matching | 仲間探し(matchList) | 有る | `Runtime/MacAppRuntime+Social.swift`・`MacSocial.swift`([architecture.md](architecture.md) の対応表) |
| Nearby | 近くの端末への送り・すれ違い | 一部 | 近くへ送る・受けるは有る(`Runtime/MacNearby.swift`)。すれ違い通信は無く、Mac で作るアプリに入らないようにするのは #3248 |
| NewItems | 共有のアプリを開いた直後の「新着 N 件」 | 有る | `Shell/MacSidebarSections.swift`・`MacPostBuildNotices.swift` |
| Notices | 直ったお知らせの一覧 | PR #3097 | 設定の「お知らせ」に出す。未読の数をサイドバーと Dock のバッジに出すのは #3245 |
| Notifications | 通知がオフの時の帯・予約した通知 | 有る | `Runtime/MacRuntimeNotifications.swift`・`MacUpcomingNotifications.swift`・ツールバーのベル(`Shell/AppCanvas.swift`)。通知のボタンと返信は #2267 |
| Onboarding | 初回の案内・コーチマーク・ベータの案内 | 一部 | 初回の流れとベータの案内は有る(`Onboarding/`)。コーチマークは #3249 |
| Privacy | 生成の直後の「個人の情報も公開されます」の確認 | 有る | `Shell/MacPostBuildNotices.swift`・`MacPostBuildPrompts.swift`(共有の `PostBuildNotice` が出す順番に入っている) |
| Purchase | Pro・課金の画面 | 有る | `Purchase/MacPaywallView.swift`・`MacPro.swift`(StoreKit)。プランを比べるシートは #3410 |
| Purchase/HeldBuild | 上限で作れなかった依頼を残し「続きから作る」を出す | 無い | #3079 |
| Push | 他の人の行動で届く通知 | 一部 | `Push/MacPushHost.swift` は有る。アプリごとの切り替えは #2274 |
| Retention | 保存期間のある entity の確認と、期限切れの記録の削除 | 有る | `Runtime/MacAppRuntime+Retention.swift`(共有の `RetentionPrompt`) |
| ScreenMap | 画面マップ | 有る | `ScreenMap/MacScreenMapView.swift`(共有の `ScreenMapLayout`) |
| Settings | 設定の画面 | 有る | `Settings/MacSettingsView.swift`(一般・アカウント・通知・プライバシー・詳細)。ライセンスは `MacLicensesView.swift`。アプリのアイコンの選択は #3418。サインインの方法は PR #3318。Action ボタンの案内は iPhone の機能なので作らない(推測) |
| Settings/AuthorProfile | 共有の投稿に出す表示名 | 無い | #3077 |
| Share | 共有のリンク・公開・公開アプリ探し・参加者・Handoff | 一部 | 共有・公開・「アプリを探す」・参加者・Handoff は有る(`Links/`)。リリース申請のシートは #2249、状況のバッジは #2250(#2071)。共有に書けなかった時の文言は #3411。docs は PR #3436 |
| Spotlight | 作ったアプリと記録の検索(CoreSpotlight) | 無い | Mac は `CSSearchableItem` を使っていない。#2269。App Intents 経由の入口は有る(Intents の行) |
| Switcher | アプリの切り替え・並べ替え・サムネイル | 有る | `Shell/AppSidebar.swift`・`MacAppThumbnailStore.swift`・名前の変更は `MacAppLibrary.swift`。サイドバーの「ひと目の数字」は #2271、Dock の右クリックは #2270 |
| Tips | 頼み方のコツ・自動の動きの案内 | 一部 | 頼み方のコツは有る(`Compose/PromptTipsList.swift`)。「ショートカットで自動にする」の案内は #3080 |
| Update | 強制更新・新しい版の案内・サービスのお知らせ | 有る | `Shell/MacUpdateGate.swift`・`MacRootView.swift` の帯(`MacServiceNoticeBar`)。お知らせの対象の OS を見るのは #3244 |
| Usage | 使っている量・上限に近い時の帯 | 有る | `Settings/MacUsageSettings.swift`・`MacRootView.swift` の帯(`MacUsageNoticeBar`) |
| Voice | 声・文字でアプリを作る会話・生成の画面・音声入力・読み上げ | 有る | `Compose/`(会話・生成)・`Voice/`(長押しで話す・声の入力・読み上げ・マイクの選択)。要約の位置の強調は #3082。読み上げの声と速さの選択は #2266 |

## Mac に無い物の数え方

表の「無い」「作らない」のうち、**作らないと決まっている物**(Follows・Health・Location)と、**Issue はあるが入っていない物**(Coins・Debug・ExtDenial・HeldBuild・AuthorProfile・Spotlight・コーチマーク)、**Issue が見つからない物**(Calls)を分けて読むこと。最後の 1 つは決めることに出す。

## 決めること

| 番号 | 決めること | 案 |
| --- | --- | --- |
| 1 | Calls(通話の画面)を Mac に作るか | 作らない(OS の電話・FaceTime へ渡す)/ 作る |
| 2 | Calls を作らない場合、callButton を Mac でどう描くか | 番号をコピーして `tel:` を OS に渡す(推奨・推測)/ 「この Mac では使えません」の枠 |
