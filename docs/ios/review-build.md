# App Store 審査用の版(Review 構成・APP_STORE_REVIEW)

#245 で追加。背景: App Store Review Guidelines 2.5.19(2026-10-16 新設)は、隠した機能・眠らせた機能を禁止している。そのため、リモート設定でオフにしておき、審査の後でオンにする方法は使えない。審査で危ない機能は**バイナリから外して**出す。

## 目次

1. [版の種類](#版の種類)
2. [審査用の版で外すもの](#審査用の版で外すもの)
3. [仕組み](#仕組み)
4. [ビルドの手順](#ビルドの手順)
5. [外れたことの確かめ方](#外れたことの確かめ方)
6. [AI 側の制限](#ai-側の制限)
7. [部品・機能を足す / 戻すとき](#部品機能を足す--戻すとき)

## 版の種類

| 版 | scheme / 構成 | 用途 | 機能 |
|---|---|---|---|
| 通常 | `Metamon` / Debug・Release | 開発・デモ・TestFlight | 全部(#245 の前と同じ) |
| 審査用 | `Metamon-Review` / Review | App Store に出す版 | 下の表の機能を外す |

## 審査用の版で外すもの

| 機能 | 外すもの | 理由(ガイドライン) |
|---|---|---|
| 単体アプリとしてのリリース申請 | `ReleaseRequestSheet` / `ReleaseUI` / `HTTPReleaseRequestClient` / カードメニューの項目 | 2.5.2・4.2.6 |
| 「探す」の公開カタログ(公開・インストール) | `DiscoverSheet` / `DiscoverEntry` / `SharingClient` の publish・install | DPLA 3.3.1(B)(c)・1.2 |
| 誰でも開ける共有リンク(アプリ側の入口) | `ShareAppSheet` / `SharingHooks`(URL を受けて取り込む処理)/ `HTTPSharingClient`、Info.plist の `CFBundleURLTypes`(`tailorit://`) | 1.2(通報・ブロックが無い) |
| HealthKit(`healthStat` / `Field.health`) | 部品と登録 / `HealthData` / `AppRuntime+Health`、`NSHealth*` / `NSMotionUsageDescription`、entitlement `com.apple.developer.healthkit*` | 4.7.2・5.1.3 |
| バックグラウンドでの位置の記録(`locationTracker`) | 部品と登録 / `LocationTracking`、`UIBackgroundModes` の `location` | 4.7.2・5.1.5・2.5.4 |
| 端末のセンサー(`sensorValue`・CoreMotion の device motion / CMAltimeter・#419) | 部品と登録 / `SensorData`、`NSMotionUsageDescription`(healthStat と共通) | 4.7.2 |
| 近くの端末との共有(`nearbyShare`・MultipeerConnectivity。すれ違い通信と UWB の距離 #425 を含む) | 部品と登録 / `Features/Nearby` 全部 / `NearbyPassbyLeaf`・`NearbyPassbyHost`、`NSLocalNetworkUsageDescription` / `NSBonjourServices` / `NSNearbyInteractionUsageDescription` | 4.7.2 |
| 音楽プレーヤーの友だちからの曲リクエスト(`musicPlayer.allowRequests`・#412) | 部品の中のリクエストの UI(`MusicPlayerLeaf` の `requests`)。送受信は上の `Features/Nearby` と一緒に外れる | 4.7.2(近くの端末との通信) |
| DSL から使うカメラの撮影(`photoField` / `videoField`) | `CameraPicker` / `VideoCameraPicker`、撮影の選択肢 | 4.7.2・2.5.14 |
| ドラレコ(`dashcam`・#410。カメラの常時録画・CoreMotion) | 部品と登録 / `DashcamRecorder`、写真への追加の説明文から「ドラレコ」を外す | 4.7.2・2.5.14 |
| DSL から使うマイクの録音(`audioRecorder`・#420) | 部品と登録 / `AudioRecorderController` / `RemoteRecorderCommands`(イヤホンのボタン)。マイクの説明文は音声入力だけの文に差し替え済み | 4.7.2・2.5.14 |
| アプリ内の Web 表示(`webView`・WKWebView) | 部品と登録 / `WebViewLeaf`。サイトは `openUrlButton` で Safari に開く | 審査メモ「WebView を使わない」(#452 G3) |
| App 内課金(Tailor It Pro・#77 #414) | `ProGate` / `PaywallSheet` / `ProSettingsSection`(設定の「Tailor It Pro」)/ `AppRuntime+PaidFeature`(有料機能の確認カード)/ `AppRuntime+PaidGate` の課金画面、MetamonServices の `ProEntitlementStore`(StoreKit)/ `ProProducts`(商品 ID)/ `CreationQuota`(端末の 1 日 3 回)。1 日の上限はサーバーの `ratelimit` だけで、当たると状態カード「今日はここまでです / 今日の上限に達しました。〜からまた使えます」(課金画面は出ない)。backend も chat の `requiresPro` / `paidFeatures` を付けない | 最初の版は課金なしで出す(#450 G1・2026-10-30 決定)。3.1.1・3.1.2・2.3.1(サーバーが Pro を判定しない) |
| 共有・公開のデータ(entity の `scope: shared / public`・`edit`・`marks`)と `chatThread` | `chatThread` の部品と登録 / `ChatThreadLeaf`。`ClientCapabilities.reviewDisabledFeatures` の `sharedData` で、backend が scope を private に直す(下の「AI 側の制限」) | 1.2(通報・ブロック・フィルタが無い)(#451 G2) |
| aiButton のオンライン版(`POST /v1/text`・#417) | `AppRuntime+AIText` のオンライン経路と Pro の入口 / `HTTPAIClient+Text`(`v1/text`)。端末内 AI(Foundation Models)は残る。端末内 AI が無い端末では理由つきで無効 | 5.1.2(i)。同意文の「記録の中身は送らない」(#452 G4) |
| 外した部品・共有 / 公開の記録を使う同梱の例 | `health` / `walk_route` / `attendance_nearby` / `dashcam` / `sensors` / `gallery-web` / `chat` / `board` / `ranking` / `post_comments` / `recipes`(`DSLExamples.reviewExcluded`。`strip-review-plist.sh` もバンドルから消す) | 上と同じ |

**残すもの**
- `photoField` / `videoField` は、**写真ライブラリから選ぶ**機能だけ残す。PhotosPicker は別プロセスで動き、権限の確認も要らない。
  - `photoField.analyze`(#418・写真の文字・金額・分類・骨格の読み取り)は残す。選んだ写真を端末内の Vision / Foundation Models で読むだけで、カメラも権限も使わない。カメラの映像を見ながらの読み取り(回数カウントなど)を足す時は、撮影と同じく審査用の版から外す。
- 現在地を 1 回だけ取る `locationButton` と、地図の `mapView`(使用中の位置だけ)は残す。
- 目的地までの道案内 `routeGuide`(#411)も残す。使うのは使用中の位置だけで、背面で案内を続けない(`CLBackgroundActivitySession` も `UIBackgroundModes: location` も使わない)。読み上げ(AVSpeechSynthesizer)に権限は要らない。
- QR を読む `qrScanner` は残す(カメラの説明文はこれに合わせて差し替える)。
- QR を作って表示する `qrCode`(#422)も残す。CoreImage で端末内で作るだけで、写真への保存は追加のみの権限(`NSPhotoLibraryAddUsageDescription`)。審査用の版の説明文にも QR の保存を書く(#452 G8)。
- 声でアプリを作り変える音声入力(マイク FAB)は残す。`audio` field の再生(一覧の行の再生ボタン)も残す(マイクを使わない。この版では録音できないので鳴る録音は無い)。
- 裏で定期的に同期する `fetch` は残す。
- `musicPlayer`(Apple Music の再生・検索)は残す。`NSAppleMusicUsageDescription` も残す(#412)。
- 最初の版のスコープは、調査メモ(リポジトリ外の `調査_ライセンスとApp Store審査.md`)の「最初の版で削る機能」の表に合わせている。
- 最初の版は App 内課金なし・共有 / 公開のデータなしで出す(2026-10-30 決定・#450 #451)。通常の版(Debug / Release)の課金と共有は今まで通り。

## 仕組み

- **コード**
  - 外す機能のコードは `#if !APP_STORE_REVIEW` で囲む。ファイル全体を囲むか、呼び出し箇所だけを囲む。
  - 部品のレジストリ(`ComponentRegistry.standard`)にも登録しない。
  - それでも DSL に外した部品が来た場合は、未知の type としてプレースホルダで描く。
- **ローカル Swift Package への条件の渡し方**
  - MetamonDSL / Renderer / Services は、project の build setting を継承しない。
  - そのため `scripts/build-review.sh` が `SWIFT_ACTIVE_COMPILATION_CONDITIONS=$(inherited) APP_STORE_REVIEW` をコマンドラインで全ターゲットに渡す。
  - 渡し忘れたときは、アプリ本体が参照する `MetamonRenderer.ReviewBuildPackageMarker` が見つからず、**コンパイルエラーで止まる**。たとえば Xcode の GUI だけで Review をビルドした場合。
- **Info.plist**
  - 元のファイルは 1 つ(通常の版と共有)。
  - Review 構成だけ、最後のビルドフェーズ `scripts/strip-review-plist.sh` が、出来上がった Info.plist からキーを消し、説明文を差し替える。署名の前に走る。
  - Debug / Release では何もしない。判定は `METAMON_APP_STORE_REVIEW=YES` で行う。
- **接続先(#452 G7)**
  - Review 構成は `project.yml` の `settings.configs.Review` で `METAMON_AI_BASE_URL` / `METAMON_APPCONFIG_BASE_URL` を prod にする(Release も同じ・#292)。
    - ai: `https://ai-prod-378953239426.asia-northeast1.run.app`
    - appconfig: `https://appconfig-prod-378953239426.asia-northeast1.run.app`
    - 出典は `docs/release/2026-10-25-runbook.md` の「prod の URL」と `backend/Makefile`。
  - Debug / Staging は stg(`settings.base`)。
  - GoogleService-Info.plist(Firebase)も Review / Release は prod の plist を入れる(#292・`docs/ios/architecture.md`「ビルド構成と接続先」)。
- **プライバシーマニフェスト(#452 G6)**
  - `Metamon/PrivacyInfo.xcprivacy`(アプリ)と `MetamonWidget/PrivacyInfo.xcprivacy`(ウィジェット)。どの版にも入る(xcodegen が各ターゲットの Resources に入れる)。
  - 理由が要る API: UserDefaults(`CA92.1` アプリだけ / `1C8F.1` App Group でウィジェットと共有)、空き容量(`E174.1` / `85F4.1`。音声モデルのダウンロード前の確認と不足分の表示)。ファイルの日時・起動からの時間・キーボードは使っていない。
  - 集めるデータ: ユーザー ID・名前(Sign in with Apple の表示名)・その他のユーザーコンテンツ・写真・正確な位置・クラッシュのデータ・パフォーマンスのデータ(Crashlytics・#296。審査用の版でも入れる)。すべて本人に紐づき、トラッキングなし、用途はアプリの機能。`docs/legal/privacy-policy.md` と App Store Connect の栄養ラベルに合わせる。
  - API やデータの扱いを変えたら、このファイルも直す。
- **entitlement**
  - Review 構成は `Metamon/Metamon-Review.entitlements` を使う。HealthKit を除いたもの。
  - 通常の版は今まで通り `Metamon/Metamon.entitlements` を使う。
- **AI への依頼**
  - `MetamonDSL.ClientCapabilities` が、審査用の版のときだけ `allowedComponents` と `disabledFeatures` を返す。`HTTPAIClient` はこれを generate / chat の body に載せる。
  - 通常の版は nil なので、何も送らない。

## ビルドの手順

前提: `xcodegen`、Xcode 26 系、自動署名できる Team(有料の Developer Program)。

```sh
cd ios
# 1) シミュレータ向け(署名なし)で通るか
./scripts/build-review.sh build -derivedDataPath ../dd

# 2) 実機向けアーカイブ(App Store Connect に出す前段)
./scripts/build-review.sh archive -allowProvisioningUpdates \
  DEVELOPMENT_TEAM=<TEAM_ID> CODE_SIGN_STYLE=Automatic
#   → ios/build/TailorIt-Review.xcarchive(ARCHIVE_PATH で変更可)
```

`build-review.sh` は `xcodegen generate` を先に実行する。そのあと `xcodebuild -scheme Metamon-Review -configuration Review` を呼ぶ。

並行ビルドで取り合わないよう、ロックのファイルのディレクトリがある機械では `nice -n 15 /usr/bin/lockf -k <ロック> xcodebuild …` で直列にする(#452 G9)。ロックの場所は環境変数 `XCODEBUILD_LOCK` で変えられる。ディレクトリが無い機械では、今まで通りそのまま `xcodebuild` を呼ぶ。

アーカイブの後(この文書の範囲外。手作業で行う):
1. Xcode Organizer を開くか、`xcodebuild -exportArchive` で App Store Connect 向けに書き出す。
2. App Store Connect で次を確認する。
   - プライバシーの表示(栄養ラベル)が、外した機能を含まないこと。健康・位置の常時・ローカルネットワークを書かない。
   - 審査メモ(調査メモの下書き)が、このバイナリと一致していること。
3. 審査の期間中は、backend(ai / appconfig)を動かしておく。

## 外れたことの確かめ方

```sh
APP=<DerivedData>/Build/Products/Review-iphoneos/Metamon.app
plutil -p "$APP/Info.plist" | grep -E 'NSHealth|NSMotion|NSLocalNetwork|Bonjour|NSNearbyInteraction|CFBundleURLTypes|UIBackgroundModes' -A2
#   → UIBackgroundModes が [fetch] だけで、他は出ないこと
plutil -p "$APP/Info.plist" | grep -E 'MetamonAIBaseURL|MetamonAppConfigBaseURL|NSPhotoLibraryAddUsageDescription'
#   → URL が ai-prod / appconfig-prod、写真への追加の説明文に QR コードがあること
ls "$APP/PrivacyInfo.xcprivacy" "$APP/PlugIns/MetamonWidget.appex/PrivacyInfo.xcprivacy"
#   → どちらもあること
codesign -d --entitlements :- "$APP"
#   → com.apple.developer.healthkit が無いこと
otool -L "$APP/Metamon" | grep -E 'HealthKit|MultipeerConnectivity|NearbyInteraction|CoreMotion'
#   → 何も出ないこと
strings "$APP/Metamon" | grep -E 'HKHealthStore|MCSession|NISession|CMPedometer|CMMotionManager|CMAltimeter|UIImagePickerController|AVAudioRecorder|MPRemoteCommand|release-requests|public-apps|WebViewLeaf|v1/text|ChatThreadLeaf|jp\.kz2601\.metamon\.pro|PaywallSheet|ProSettingsSection|ProEntitlementStore|購入して作る|Pro にする'
#   → 何も出ないこと
#   注: WKWebView / WebKit の参照は FirebaseAuth(AuthWebViewController。OAuth・reCAPTCHA 用)から残る。
#       この版のサインイン(Sign in with Apple・匿名)では使わない。アプリのコードからは WebKit を使わない
grep -a -c -F -e 'Tailor It Pro' -e '購入して作る' -e 'オンラインの AI で書く' "$APP/Metamon"
#   → 0(日本語の文字列は strings では出ないので grep -a で見る)
otool -L "$APP/Metamon" | grep StoreKit
#   → 何も出ないこと(App 内課金なし・#450 G1)
ls "$APP" | grep -E '^(health|walk_route|attendance_nearby|dashcam|sensors|gallery-web|chat|board|ranking|post_comments|recipes)\.json$'
#   → 何も出ないこと
grep -l '"scope" *: *"\(public\|shared\)"' "$APP"/*.json
#   → 何も出ないこと(共有・公開の例が残っていない・#451 G2)
```

テスト:
- `swift test --package-path Packages/MetamonDSL --filter ReviewBuildTests -Xswiftc -DAPP_STORE_REVIEW` を実行する。
- これで、審査用の条件での allowlist と、主要な例(家計簿・習慣・日記・TODO・割り勘ほか)が外した部品を使わないことを確かめる。
- 通常の版のテストは、今まで通り `xcodebuild -scheme Metamon test`。

## AI 側の制限

契約は `docs/backend/openapi-ai.yaml` の `AllowedComponents` / `DisabledFeatures`。backend は `backend/internal/ai/restrict.go`。

- **リクエスト**: `POST /v1/generate` と `POST /v1/chat` に、省略可能な 2 つを足す。
  - `allowedComponents`: 使ってよい部品の type の一覧。
  - `disabledFeatures`: `fieldHealth` / `cameraCapture` / `sharedData`(#451 G2)。
- **省略したとき**(通常の版・旧 iOS): 今まで通り全部使える。プロンプトもスキーマも、#245 の前とバイト単位で同じ。
- **指定したとき**
  - 部品カタログ・チャットの「できること」の索引・生成スキーマから、許可外の部品を外す。
  - fieldHealth が無効なら `health` 属性を、cameraCapture が無効なら source の camera / both も外す。
  - `sharedData` が無効なら(審査用の版・#451 G2)、記録はすべて private にする。
    - 生成スキーマの `Entity.scope` を `private` だけにし、`marks` を外す。カタログの scope / edit の説明も「この版では使わない」に替える。
    - プロンプトから、共有の範囲(`shareRules`)・いいね / 保存(`marksRules`)・投稿者 / edit / 「みんなで使う」の行を外し、制限の注記に「記録はすべて自分だけ」を足す。チャットの「共有の範囲」は「この版ではできない」に替える。
    - 「家族だけ」などの依頼で shared を求める再生成(#254 / #259 の `shareCreateProblem` / `shareEditProblem`)はしない。
    - 生成後の検証で、新しく shared / public の entity があれば再生成に回す。最後の試行でも残っていれば、scope / edit / marks と list の `item.mark` を落とす。
    - chat の ready / 案に `requiresPro` / `paidFeatures` を付けない(この版に課金は無い・#450 G1)。
  - `chatThread` が許可外なら、チャットの規則(`chatThreadRules`)も外す。
  - `webView` が許可外なら(審査用の版・#452 G3)、「サイトを見たい」の規則・チャットの「できないこと」の代わり・`openUrlButton` の説明を、`openUrlButton` で Safari に開く形に書き換える。
  - 生成後の検証で、許可外の部品は再生成に回す。最後の試行でも残っていれば、その部品を取り除く。`health` 属性は消し、source は library に直す。
  - edit で、今の DSL に既に許可外の部品がある場合は取り除く(DSL 全体で保存)。何をしたかは `meta.restricted` に入る。

## 部品・機能を足す / 戻すとき

- **新しい部品がネイティブ API を使う場合**(カメラ・位置・健康・近距離通信など)
  1. 部品のファイルと `ComponentRegistry` の登録を `#if !APP_STORE_REVIEW` で囲む。
  2. `ClientCapabilities.reviewExcludedComponents` に足す。
  3. 権限の説明が要るなら、`strip-review-plist.sh` に消す処理を足す。
- **審査用の版に戻す場合**(例: v1.1 で「探す」を戻す)
  - `#if` を外す。
  - **通報・ブロック・フィルタなど、審査で求められるものを先に入れてから**、新しい版として審査に出す。審査の後でフラグを切り替えるだけ、はしない(2.5.19)。
