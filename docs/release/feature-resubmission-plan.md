# 最初の版で外した機能を、改めて審査に出す計画

#334(ロードマップの R-48)。最初の App Store の版(審査用の版 `Metamon-Review` / `APP_STORE_REVIEW`・#245)で外した機能を、リリースの後に 1 つずつ版に戻して審査に出す計画。

- 外した機能の中身と仕組みは [../ios/review-build.md](../ios/review-build.md)、提出の点検と却下の返し方は [app-review-playbook.md](app-review-playbook.md)(#332)、版とブランチの流れは [process.md](process.md)、栄養ラベルの答えは `docs/store/privacy-label.md`(#328・PR #586)。ここでは**中身を写さず、戻す順番と手順だけを書く**。
- 戻す順番・戻すかどうかは人が決める(7 章)。この文書は案。
- ガイドラインの番号と文は、2026-11-03 に [App Review Guidelines](https://developer.apple.com/app-store/review/guidelines/) と [Apple Developer Program License Agreement](https://developer.apple.com/support/terms/apple-developer-program-license-agreement/)(DPLA)で確かめた(8 章)。
- 審査の結果を保証するものではない。推測には **(推測)** と書く。

## 目次

1. [原則](#1-原則)
2. [外した機能の一覧(コード上の根拠)](#2-外した機能の一覧コード上の根拠)
3. [戻す順番の案](#3-戻す順番の案)
4. [機能ごとの条項・前提・審査メモ](#4-機能ごとの条項前提審査メモ)
5. [1 機能を 1 つの版で出す手順](#5-1-機能を-1-つの版で出す手順)
6. [却下された時に外して出し直す手順](#6-却下された時に外して出し直す手順)
7. [判断してほしいこと](#7-判断してほしいこと)
8. [条項の要旨(確かめた文)](#8-条項の要旨確かめた文)

## 1. 原則

- **戻す = バイナリに入れて、新しい版として審査に出す**。審査の時にバイナリにある機能は、審査メモに書き、審査で使える状態にする(2.5.19・2.3.1(a))。
- **審査の後で切り替えるスイッチを作らない**。リモート設定・サーバーの判定・日付などで、審査の後に機能を出す作りにしない(2.5.19)。機能の有無はコンパイル条件(`APP_STORE_REVIEW`)とビルドの中身だけで決める。
- **外す = バイナリから消す**。却下で戻す時も `#if !APP_STORE_REVIEW` に戻し、コード・部品の登録・権限の説明・entitlement ごと外す(review-build.md の「外れたことの確かめ方」で確かめる)。
- **1 つの版に戻す機能は 1 つ**。却下の理由を 1 つの機能に絞れ、外して出し直すのも 1 つの revert で済む。ただし互いに無いと成り立たない機能は 1 つの単位にまとめる(3 章の「単位」)。
- **審査で求められる物(通報・ブロック・同意・ラベル・ポリシー)を先に入れる**。機能を戻す PR より前か、同じ版に入れる。
- 前の版が通ってから次の版を出す。2 つの版を同時に審査に出さない。

## 2. 外した機能の一覧(コード上の根拠)

`git grep -l APP_STORE_REVIEW -- ios` で見つかったファイルのうち、機能ごとの代表。外し方の詳細は [review-build.md](../ios/review-build.md) の「審査用の版で外すもの」の表。共通の置き場所は次の 4 つ。

- 部品の allowlist: `ios/Packages/MetamonDSL/Sources/MetamonDSL/Model/ClientCapabilities.swift` の `reviewExcludedComponents` / `reviewDisabledFeatures` / `supportedFeatures`
- 部品の登録: `ios/Packages/MetamonRenderer/Sources/MetamonRenderer/Registry/ComponentRegistry.swift`(`#if !APP_STORE_REVIEW`)
- Info.plist・同梱の例: `ios/scripts/strip-review-plist.sh`、`DSLExamples.reviewExcluded`(`ios/Packages/MetamonDSL/Sources/MetamonDSL/Examples/DSLExamples.swift`)
- entitlement: `ios/project.yml` の `configs.Review`(`Metamon/Metamon-Review.entitlements`)

| # | 機能 | コード上の根拠(`#if APP_STORE_REVIEW` / `#if !APP_STORE_REVIEW` のあるファイル) | 外したもの(上の共通の置き場所) |
|---|---|---|---|
| F1 | アプリ内のフィードバック(#319) | `ios/Metamon/Features/Feedback/*`、`MetamonServices/Feedback/FeedbackClient.swift`・`Services+Feedback.swift` | — |
| F2 | aiButton の端末内 AI の集計(#446) | `ios/Metamon/App/AppRuntime+AIButtonMetrics.swift`、`Features/Consent/AIButtonMetrics+Wiring.swift`・`PrivacySettingsSection.swift`、`MetamonServices/Consent/AIButtonMetrics.swift` | — |
| F3 | aiButton のオンライン版(`POST /v1/text`・#417) | `Features/Purchase/AppRuntime+AIText.swift`、`MetamonServices/Live/HTTPAIClient+Text.swift`、`MetamonDSL/Model/AIText.swift` | — |
| F4 | 端末のセンサー(`sensorValue`・#419) | `MetamonRenderer/.../Leaves/SensorValueLeaf.swift`・`Support/SensorData.swift` | `.sensorValue`、`NSMotionUsageDescription`、例 `sensors` |
| F5 | アプリ内の Web 表示(`webView`) | `MetamonRenderer/.../Leaves/WebViewLeaf.swift` | `.webView`、例 `gallery-web` |
| F6 | DSL から使うカメラの撮影(`photoField` / `videoField` の撮影) | `MetamonRenderer/.../Support/CameraPicker.swift`・`VideoCameraPicker.swift`・`Leaves/PhotoFieldLeaf.swift`・`VideoFieldLeaf.swift` | `cameraCapture`、カメラの説明文の差し替え |
| F7 | マイクの録音(`audioRecorder`・#420) | `Leaves/AudioRecorderLeaf.swift`・`Support/AudioRecording.swift` | `.audioRecorder`、マイクの説明文の差し替え |
| F8 | ドラレコ(`dashcam`・#410) | `Leaves/DashcamLeaf.swift`・`Support/DashcamRecorder.swift` | `.dashcam`、例 `dashcam` |
| F9 | HealthKit(`healthStat` / `Field.health`) | `Leaves/HealthStatLeaf.swift`・`Support/HealthData.swift`、`ios/Metamon/Features/Health/AppRuntime+Health.swift` | `.healthStat`、`fieldHealth`、`NSHealth*`、entitlement、例 `health` |
| F10 | 背面での位置の記録(`locationTracker`) | `Leaves/LocationTrackerLeaf.swift`・`Support/LocationTracking.swift`・`LocationTrackingStatus.swift`、`Features/Location/TrackingBar.swift` | `.locationTracker`、`UIBackgroundModes` の `location`、`NSLocationAlways*`、例 `walk_route` |
| F11 | 近くの端末との共有・すれ違い(`nearbyShare`・#425)と曲のリクエスト(#412) | `ios/Metamon/Features/Nearby/*`、`Leaves/NearbyShareLeaf.swift`・`NearbyPassbyLeaf.swift`・`Support/NearbyPassbyHost.swift`・`Leaves/MusicPlayerLeaf.swift` | `.nearbyShare`、`NSLocalNetworkUsageDescription`・`NSBonjourServices`・`NSNearbyInteractionUsageDescription`、例 `attendance_nearby` |
| F12 | App 内課金(Tailor It Pro・#77 #414) | `ios/Metamon/Features/Purchase/*`(`ProGate`・`PaywallSheet`・`ProSettingsSection`・`AppRuntime+PaidFeature`・`AppRuntime+PaidGate`)、`MetamonServices/Purchase/*` | — |
| F13 | 共有の単位: 共有・公開のデータ(`sharedData`)・`chatThread`・共有リンク・通報 / ブロック(#321) | `ios/Metamon/Features/Share/*`(`ShareAppSheet`・`JoinShareSheet`・`AppRuntime+Sharing`・`AppRuntime+Moderation`・`SharedMembersSection`)、`Leaves/ChatThreadLeaf.swift`、`MetamonServices/Live/HTTPSharingClient*.swift`・`Protocols/ModerationClient.swift`・`Protocols/SharingClient.swift` | `.chatThread`、`sharedData`、`CFBundleURLTypes`、例 `chat` / `board` / `ranking` / `post_comments` / `recipes` |
| F14 | 仲間探し(#507・`matchList`) | `ios/Metamon/Features/Matching/*`、`Leaves/MatchListLeaf.swift`・`Support/InterestButton.swift`・`Host/Capabilities/MatchingHost.swift`、`MetamonServices/Matching/*`・`Live/HTTPMatchingClient.swift`、`MetamonDSL/Model/Matching.swift` | `.matchList`、`supportedFeatures` の `matching` |
| F15 | 「探す」の公開カタログ(公開・インストール) | `ios/Metamon/Features/Share/DiscoverSheet.swift`、`Protocols/SharingClient.swift` の publish / install | — |
| F16 | 単体アプリとしてのリリース申請 | `ios/Metamon/Features/Share/ReleaseRequestSheet.swift`・`ReleaseRequests+Wiring.swift`、`MetamonServices/Live/HTTPReleaseRequestClient.swift`・`ReleaseRequests/*` | — |
| — | β の初回の案内(#313) | `ios/Metamon/Features/Onboarding/BetaEnvironment.swift`・`BetaWelcomeStore.swift`・`BetaWelcomeView.swift` | **戻さない**(2.2: App Store の版に β の物を入れない。TestFlight だけで出す) |

注:
- backend(`backend/internal/ai/restrict.go`)は、iOS が送る `allowedComponents` / `disabledFeatures` / `supportedFeatures` で生成を絞る。iOS の版で部品を戻せば、その版からの依頼で自動的に使えるようになる。ストアに残る古い版は古い値を送るので、古い版には出ない。
- #351(OS ごとの A / B / C の分類)は未決。F4・F6〜F11 は「iOS だけ」に入る見込み **(推測)**。戻す順番とは独立に決めてよい。

## 3. 戻す順番の案

リスク = 審査で問題になりやすい条項の重さ × 先に要る物の多さ。低い順に出す。

| 順 | 単位 | リスク | 理由 |
|---|---|---|---|
| 1 | F1 フィードバック | 低 | 本人が運営に送るだけで、他の人には見えない(1.2 の対象外の見込み **(推測)**)。要るのは栄養ラベル・ポリシー・マニフェストの追加だけ。サポートへの連絡の入口が増え、1.2 の「連絡先」にも役立つ |
| 2 | F2 aiButton の集計 | 低 | 送るのは数だけ。設定でオフにできる。要るのは「使用状況データ」のラベルとポリシー |
| 3 | F3 aiButton のオンライン版 | 低〜中 | 記録の文を第三者 AI に送る(5.1.2(i))。同意の文の「記録の中身は送らない」を直す必要がある(ポリシー・同意は人) |
| 4 | F5 webView | 中 | AI が決めた URL を開く。4.7 の「ソフト」と見られる心配は小さいが、審査メモの「WebView を使わない」を消す必要がある |
| 5 | F4 センサー | 中 | 4.7.2(ネイティブ API を DSL に出す)の論点が最初に出る。権限はモーション 1 つで、データは端末内。4.7.2 の答え方を一番軽い機能で試せる |
| 6 | F6 カメラの撮影 | 中 | 4.7.2・2.5.14。撮影は本体の UI(システムのカメラ)で、毎回本人が押す |
| 7 | F7 録音 | 中 | 4.7.2・2.5.14。録音中の表示(2.5.14 の「はっきりした表示」)を確かめる |
| 8 | F8 ドラレコ | 中〜高 | 常時の録画。2.5.14 の表示と、F6 が通っていることが前提 |
| 9 | F9 HealthKit | 高 | 5.1.3 の特に厳しい規則・entitlement・ラベルの「健康とフィットネス」。4.7.2 で最も名指しされやすい |
| 10 | F10 背面の位置 | 高 | 2.5.4(背面は本来の目的だけ)・5.1.5。背面の位置は審査で理由を細かく聞かれる |
| 11 | F11 近くの端末・曲のリクエスト | 高 | 4.7.2 に加え、知らない人との通信になりうる(1.2 の論点)。ローカルネットワークの権限 |
| 12 | F12 App 内課金 | 中(別の流れ) | 3.1.1・3.1.2 は形の決まった論点だが、ASC の商品登録・規約の有料部分・特定商取引法の表記(#330)など人の作業が多い。事業の判断で前後させてよい |
| 13 | F13 共有の単位 | 高 | 1.2 の 4 つ(フィルタ・通報と速い対応・ブロック・連絡先)と、運営の 24 時間の対応が要る。年齢区分(UGC あり)も変わる |
| 14 | F14 仲間探し | 高 | F13 が前提。知らない人と連絡先を交換する(1.2・年齢)。18 歳以上の確認がある |
| 15 | F15 「探す」 | 最も高い | DPLA 3.3.1(B)(c)(他のアプリの店を作らない)・4.7.4(索引)・4.2.6 の論点。戻すかどうかから決める(7 章) |
| 16 | F16 リリース申請 | 最も高い | 4.2.6(アプリを作るサービスが利用者の代わりに出さない)・2.5.2。アプリの中では戻さない案を推す(7 章) |

単位をまとめる理由:
- F13: 共有リンク・共有データ・`chatThread` は通報・ブロックが無いと 1.2 を満たさない。通報・ブロックは共有が無いと対象が無い(review-build.md の通報・ブロックの行)。分けられないので 1 つの版で出す。
- F11: 曲のリクエストは近くの端末の通信で送るので、近くの端末と一緒に戻る。
- β の案内は App Store の版に戻さない(2 章の表の最後)。

## 4. 機能ごとの条項・前提・審査メモ

「ラベル」は `docs/store/privacy-label.md` の 4 章「通常の版だけで増えるもの」を指す。審査メモの案は ASC の「App Review に関する情報 > メモ」に足す英語の文。今のメモは 3,994 字で上限(4,000 字)に近いので、足す時は外した機能の説明でない古い文を削って字数を合わせる([app-review-notes.md](../store/app-review-notes.md))。メモに足すのは**その版のバイナリにある機能だけ**。

### F1 フィードバック

- 条項: 5.1.1(i)(ii)・5.1.2(i)(スクショ・診断を送る)。
- 前提: ラベルに「カスタマーサポート」「その他の診断データ」「写真(スクショ)」。ポリシーへの追記(`docs/legal/feedback-privacy-draft.md` を元に人が決める)。`PrivacyInfo.xcprivacy` の `NSPrivacyCollectedDataTypes`。
- メモの案: `NEW IN THIS VERSION: Settings > Send Feedback (or shake the device) lets the user send a message to us. A screenshot and device information (model, OS, app version) are attached only if the user leaves them selected before sending.`

### F2 aiButton の集計

- 条項: 5.1.1(ii)・5.1.2(i)。
- 前提: ラベルに「使用状況データ > 製品の操作」。ポリシーへの追記(人)。設定の「AI ボタンの利用状況を送る」でオフにできること。
- メモの案: `NEW IN THIS VERSION: The app sends counts of how often the on-device AI button is used, without the text. Users can turn this off in Settings > Privacy.`

### F3 aiButton のオンライン版

- 条項: 5.1.2(i)・5.1.1(ii)。
- 前提: 同意の画面とポリシーの文(今は「記録の中身は送らない」)を、記録の文(最大 4KB)を送る形に直す(**人**・`docs/legal/*` はこの計画では変えない)。ラベルの「その他のユーザーコンテンツ」の説明。端末内 AI が無い端末だけで使うのか、選べるのかを決める。
- メモの案: `NEW IN THIS VERSION: On devices without on-device AI, the AI button can send the selected record text (up to 4 KB) through our server to Anthropic's AI. This happens only after the AI consent screen and only when the user taps the button.`

### F4 センサー / F6 撮影 / F7 録音 / F8 ドラレコ / F9 HealthKit / F10 背面の位置 / F11 近くの端末

共通の条項は 4.7.2(ネイティブ API をソフトに出さない)・4.7.3(毎回の同意)。答え方は playbook の 4.3 の案 A(記録帳は本体の部品の組み合わせで、権限は本体が本体の UI で求める)。その上で機能ごとに次を足す。

| 機能 | ほかの条項 | 前提(先に入れる物) | メモの案(足す 1 文) |
|---|---|---|---|
| F4 センサー | 5.1.1(ii) | `NSMotionUsageDescription` を審査用の版に戻す(`strip-review-plist.sh` から消す)。データは端末内で、ラベルは増えない見込み **(推測)** | `A built-in sensor component can show the barometer and motion values on screen. It asks for Motion & Fitness permission when first used. The values stay on the device.` |
| F6 撮影 | 2.5.14 | カメラの説明文を QR だけの文から戻す。撮る時はシステムのカメラ画面で、毎回本人が押す | `Photo and video fields can now also take a photo or video with the system camera, after the user taps the camera button.` |
| F7 録音 | 2.5.14 | マイクの説明文を音声入力だけの文から戻す。録音中の表示(赤い印・経過時間)。ラベルに「音声データ」(サーバーに送るなら) | `A recorder component records audio only while the user keeps it started, with a visible recording indicator and timer.` |
| F8 ドラレコ | 2.5.14・2.5.4(背面) | F6 が通っていること。録画中の表示。背面で録画しないこと(するなら 2.5.4 の説明) | `A dashcam component records video only while it is on screen and started by the user, with a recording indicator.` |
| F9 HealthKit | 5.1.3(i)(ii)・2.5.1 | entitlement(`Metamon.entitlements` の HealthKit)を審査用の版に戻す。ラベルに「健康とフィットネス」。ポリシーに「健康のデータを広告・第三者・iCloud に使わない」(5.1.3)を書く(人)。健康のデータを AI に送らない作りの確認 | `HealthKit: a built-in component can read step count and similar values the user allows in the Health permission sheet, and show them in the user's tracker. Health data is not sent to our server or to the AI, and is not used for advertising.` |
| F10 背面の位置 | 2.5.4・5.1.5 | `UIBackgroundModes: location` と `NSLocationAlways*` を戻す。記録中の表示(TrackingBar)。止め方。ラベルの「正確な位置情報」の説明に連続した点を足す | `Route recording: when the user starts recording a walk or run, the app records the route in the background until the user stops it. A bar shows that recording is on.` |
| F11 近くの端末 | 1.2(知らない人)・5.1.1 | `NSLocalNetworkUsageDescription`・`NSBonjourServices`・`NSNearbyInteractionUsageDescription` を戻す。相手が見える前に本人が受け入れる流れ。知らない人と使う作り(すれ違い)なら通報・ブロック(F13 の後に回す案も) | `Nearby sharing: two users who both open the same tracker and both accept can exchange entries over the local network. Nothing is sent to people who have not accepted.` |

注: メモの文は、その版の実装を見てから事実に合わせて直す(2.3.1(a))。上は形の案で、実装を確かめていない。

### F12 App 内課金

- 条項: 3.1.1・3.1.2(自動更新のサブスクリプション)・2.3.1(サーバーが Pro を判定する)。
- 前提: ASC にサブスクリプションを登録して版と一緒に審査に出す(playbook 2.4 の #326 の行)。ペイウォールの表記(価格・期間・自動更新・取り消し方・規約とポリシーのリンク)。規約の有料部分・特定商取引法の表記(#330・人)。ラベルに「購入 > 購入履歴」。
- メモの案: `IN-APP PURCHASE: Tailor It Pro is an auto-renewable subscription (<product IDs>). Without it, the app works with a daily limit on AI requests.`

### F13 共有の単位(共有データ・chatThread・共有リンク・通報 / ブロック)

- 条項: 1.2(フィルタ・通報と速い対応・ブロック・連絡先)・1.2.1・5.1.1。
- 前提:
  - #321 の作り(通報・ブロック・運営による非公開化)が入っていること(PR #490 でマージ済み)。2 アカウントでの確認(#321 の完了の条件)が済んでいること。
  - 通報に 24 時間以内に応える運用と担当(人)。規約の禁止事項(`docs/legal/moderation-policy-draft.md` を元に人が決める)。
  - NG ワードのフィルタ(#321 の「最低限」)。
  - 年齢区分の質問票の「ユーザー生成コンテンツ」を「はい」に直す([listing.md](../store/listing.md) の 4 章)。
  - ラベルに「その他のユーザーコンテンツ(他の人に見える)」「名前(表示名)」。
  - `CFBundleURLTypes`(`tailorit://`)を戻す。共有ページ(Web)も同じ通報の入口を持つこと。
  - 審査で試せるよう、共有を受ける側の 2 つ目のアカウントか、匿名の 2 台目で試せる手順。
- メモの案: `SHARING: A user can share a tracker with people they invite by link. Shared entries and chat messages can be reported (long-press > Report) and their authors can be blocked (Settings > Blocked Users). Reports reach our team and are handled within 24 hours; we can hide shared content.<Only after the word filter (#321) is confirmed: " Posts are filtered for objectionable words.">`

### F14 仲間探し

- 条項: 1.2・1.2.1(a)(年齢で制限)・5.1.1。
- 前提: F13 が通っていること。18 歳以上の確認(`AdultConfirmSheet`)と年齢区分の答え(18+ の扱い)を人が決める。連絡先の交換は双方の同意だけで起きること。ラベルの「名前」など。
- メモの案: `MATCHING: In a tracker, users aged 18 or older can mark interest in an item. Contact details are shown only when both users have marked interest in each other. Users can report and block each other.`

### F15 「探す」の公開カタログ

- 条項: DPLA 3.3.1(B)(c)(App Store の版はほかのアプリの店を作らない)・2.5.2・4.7.4(索引)・4.2.6・1.2。
- 前提: F13 が通っていること。カタログに出る物が「アプリ」でなく、本体の部品の組み合わせのテンプレート(コンテンツ)だと示せる形(名前・画面の言い方・インストールの言い方を [wording.md](../store/wording.md) に合わせる)。掲載の審査(運営の確認)と通報。年齢で絞る仕組み(1.2.1(a))。
- メモの案: 形が決まってから書く。少なくとも「テンプレートを自分の記録帳にコピーするだけで、外のアプリやコードを配らない」「掲載は通報でき、運営が止められる」の 2 点。
- 戻す前に人が決める(7 章の 3)。

### F16 リリース申請

- 条項: 4.2.6(アプリを作るサービスは利用者の代わりに出さない)・DPLA 3.3.1(B)・2.5.2。
- 案: アプリの中には戻さず、アプリの外(Web・問い合わせ)で扱う。戻すなら、運営が利用者の代わりに出す形をやめ、「コンテンツの持ち主が自分で出す」形にする必要がある **(推測)**。7 章の 4。

## 5. 1 機能を 1 つの版で出す手順

前の版が審査を通り、公開してから始める。名前の例は F1 で、版は `1.1.0`。

1. **戻す PR(develop へ)**: `feature/resubmit-<機能>` を develop から切り、1 つの PR で次をすべて行う。
   - コード: その機能の `#if !APP_STORE_REVIEW` / `#if APP_STORE_REVIEW` を外す(2 章の根拠のファイル)。ほかの機能のガードは触らない。
   - `ClientCapabilities` の `reviewExcludedComponents` / `reviewDisabledFeatures` / `supportedFeatures` から外す。
   - `strip-review-plist.sh` から、その機能の権限のキーを消す処理と同梱の例の削除を外す。`DSLExamples.reviewExcluded` も合わせる。entitlement が要るなら `Metamon-Review.entitlements` に足す。
   - [review-build.md](../ios/review-build.md): 「審査用の版で外すもの」の表から行を消し、「外れたことの確かめ方」の grep からその機能の文字列を消す。
   - `ReviewBuildTests` の期待を直す。
   - `PrivacyInfo.xcprivacy` と `docs/store/privacy-label.md`(4 章の行を 2 章へ移す)を合わせる。ポリシー・同意の文が変わるなら、`docs/legal/*` の変更は**人が別の PR で**先に入れる。
   - [app-review-notes.md](../store/app-review-notes.md) に 4 章のメモを足す(4,000 字以内)。[listing.md](../store/listing.md) の機能の行・年齢区分・スクショ。
   - PR 本文に「戻す機能」「関係する条項」「審査メモの差分」「ラベルの差分」を書く。
2. **版を切る**: [process.md](process.md) の 6 章。`release/ios-1.1.0` を切り、`ios/scripts/bump-build.sh --version 1.1.0`(版は Y を上げる。ビルド番号は全期間で増やし続ける)。
3. **確かめる**: `./scripts/build-review.sh build` が通る(機能を戻した版で、残りの外した機能がまだ外れていることを「外れたことの確かめ方」で確かめる)。戻した機能が審査用の版で実際に動くことを TestFlight で確かめる(2.1・審査で試せること)。
4. **【人】** ASC: 栄養ラベル(privacy-label.md の答えを足す)・年齢区分・審査メモ・「このバージョンの新機能」の文を直す。栄養ラベルは版でなくアプリ単位の答えなので、提出の直前に足す **(推測: 反映のタイミングは ASC のヘルプで確かめる)**。
5. **【人】** 審査に提出(playbook の 2 章の点検表を通す)。
6. 通ったら process.md の 6 章の 8〜9(main へ・タグ `ios-v1.1.0`)。公開のしかたは playbook の 5 章。
7. 次の機能は、公開した版が落ち着いてから(クラッシュの率を Crashlytics で見る)1 から。

## 6. 却下された時に外して出し直す手順

まず playbook の 4.1 で、説明で足りるか・機能が問題かを分ける。説明で足りるなら同じバイナリのまま返信する。

機能が問題で、その機能を外して版を出す時:

1. **外す**: `release/ios-1.1.0` で、5 章の 1 の PR を `git revert` する PR を作る。コード・権限の説明・entitlement・例・review-build.md・テストが、すべて前の版と同じ「外した」状態に戻る。**機能のコードを残して実行時に止める形にはしない**(2.5.19)。
2. **版番号**: 版(`MARKETING_VERSION`)は審査を通っていないので `1.1.0` のまま。ビルド番号だけ上げる(`bump-build.sh`。同じ番号は二度と上げられない)。版に何も残らないなら、その版の審査を取り下げて次の機会に回してもよい。
3. **確かめる**: 「外れたことの確かめ方」の grep で、その機能の文字列が出ないこと。
4. **【人】** ASC を戻す:
   - 栄養ラベル: 5 章の 4 で足した答えを消す(privacy-label.md の行も 4 章に戻す)。
   - 審査メモ・新機能の文・年齢区分・スクショ: その機能の文を消す。
   - 返信で「Build <番号> removes <機能>.」と、何を外したかを書く(playbook の返信の原則)。
5. **develop にも戻す**: release → develop の取り込みで revert も develop に入る。develop は「次に出せる状態」= ストアと同じ外し方を保つ。もう一度挑む時は、却下の理由を直した上で「revert の revert」と直しを 1 つの PR にして、5 章の 1 からやり直す。
6. 却下の理由・返信・直したことを playbook の 4 章に追記する(次の版の審査メモに活かす)。同じ機能で 2 回却下されたら、申し立て(Appeal)にするか、その機能を戻すのを止めるかを人が決める。

## 7. 判断してほしいこと

| # | 決めること | 案 | 推奨 |
|---|---|---|---|
| 1 | 戻す順番(3 章) | 3 章の順 / 価値の高い順(例: F13 共有・F12 課金を先に) | 3 章の順。最初は F1 フィードバック |
| 2 | 1 つの版に 1 単位とするか | 1 単位ずつ / 低リスクの F1〜F3 はまとめて 1 版 | 1 単位ずつ(却下の理由と戻し方が 1 つで済む) |
| 3 | 「探す」(F15)を戻すか | 戻す(テンプレートのコピーとして作り直してから)/ 戻さない / Web だけで出す | 決める前に、DPLA 3.3.1(B)(c) に当たらない形を設計する Issue を立てる |
| 4 | リリース申請(F16)をアプリに戻すか | 戻さない(アプリの外で扱う)/ 持ち主が自分で出す形に作り直して戻す | 戻さない |
| 5 | 課金(F12)をいつ出すか | 3 章の順(12 番目)/ 事業の都合で前に出す | 事業の判断(#326・#330 の人の作業と一緒に) |
| 6 | 却下で外す時、develop からも外すか(6 章の 5) | develop も外す / develop には残して release だけで外す | develop も外す(ストアと develop の差を作らない) |
| 7 | ポリシー・同意の文の変更(F1・F2・F3・F9・F13) | 機能ごとに人が `docs/legal/*` を直す / まとめて先に直す | 機能ごと(まとめて先に直すと、今の版の実態とずれる) |

## 8. 条項の要旨(確かめた文)

2026-11-03 に公式の文で確かめた要旨。ほかの番号(5.1.1・5.1.2・4.7.1〜4.7.5・1.2.1 など)は [app-review-playbook.md](app-review-playbook.md) の 7 章。

| 番号 | 要旨 |
|---|---|
| 1.2 | ユーザーが作る・生成する・投稿・共有するアプリ(SNS・UGC・AI でコンテンツを作るアプリを含む)は、フィルタ・通報と速い対応・ブロック・連絡先の公開を持つこと。違反の内容を消すのは開発者の責任 |
| 1.2.1(a) | クリエイターのアプリは、年齢区分を超えるコンテンツを見分けられ、年齢で制限すること |
| 2.2 | デモ・β・試用版は App Store に置かない。TestFlight を使う |
| 2.3.1(a) | 新しい機能・変更は、審査メモに具体的に書き、審査で使える状態にする。ない機能を宣伝しない |
| 2.5.2 | バンドルの中で完結し、機能を足す・変えるコードをダウンロード・インストール・実行しない |
| 2.5.4 | 背面のサービスは本来の目的(VoIP・音声の再生・位置・タスクの完了・ローカル通知など)だけに使う |
| 2.5.14 | 記録する時(カメラ・マイク・画面の録画など)は、はっきりした同意と、目で見える / 耳で分かる表示を出す |
| 2.5.19 | 隠した・眠らせた・書いていない機能を入れない。機能は利用者と App Review にとって明らかであること。隠すと削除・アカウント停止の理由になる |
| 3.1.1 | アプリの中の機能を解放するなら App 内課金を使う。独自の仕組み(ライセンスキー・QR など)で解放しない |
| 3.1.2 | 自動更新のサブスクリプションを出せる(続く規則に従う) |
| 4.2.6 | テンプレートやアプリを作るサービスで作ったアプリは、コンテンツの持ち主が出すのでなければ却下。サービスが利用者の代わりに出さない |
| 4.7 | HTML5 / JavaScript のミニアプリ・ミニゲーム・チャットボット・プラグイン等はバイナリの外から出せるが、出す側が全責任を持ち、4.7.1〜4.7.5 を守る |
| 4.7.2 | Apple の許可なく、ネイティブの API・技術をそのソフトに広げない・出さない |
| 4.7.3 | 個々のソフトに、データや権限を、毎回のはっきりした同意なしに渡さない |
| 4.7.4 | 出しているソフトの索引(universal link つき)を持つ |
| 5.1.3(i) | HealthKit・モーションなどの健康のデータを、広告・マーケティング・データの分析に使ったり第三者に出したりしない(健康管理・健康の研究のためで、許可がある時だけ) |
| 5.1.5 | 位置情報は、アプリの機能に直接関係する時だけ使う |
| DPLA 3.3.1(B) | 実行コードをダウンロード・インストールしない。解釈されるコードは、(a) アプリの主な目的を変えない (b) 署名・サンドボックス等を迂回しない (c) App Store のアプリなら、ほかのアプリの店を作らない、の場合だけダウンロードしてよい |
