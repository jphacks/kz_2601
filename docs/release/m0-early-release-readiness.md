# M0 を早く App Store に出すかの判断材料(#241)

- 調べた日: 2026-10-30。対象: `origin/develop` = `ad5ccb4`(PR #445 まで)
- 元の調査: リポジトリ外の `調査_ライセンスとApp Store審査.md`(10/25・`a207e75` 時点)。その後に入った部品(ドラレコ・すれ違い・センサー・録音・musicPlayer・ライブアクティビティ・写真の読み取り・書き出し・aiButton など)を、今のコードで照合し直した。
- 関連: [審査用の版](../ios/review-build.md)・[審査メモ](../store/app-review-notes.md)・[掲載文](../store/listing.md)・[言い回し](../store/wording.md)
- 注意: 法律・審査の結果の保証ではない。推測には **(推測)** と書く。
- 追記(2026-10-30): 1 章の 1・2 の方針で出すと決めた(#450 / #451)。G1〜G4・G6〜G9 は PR #455 に入れた(4 章の「状態」)。2 章は調べた日の結果のまま。

## 目次

1. [結論](#1-結論)
2. [審査用の版のビルド結果](#2-審査用の版のビルド結果)
3. [ガイドラインごとの状態](#3-ガイドラインごとの状態)
4. [コードの穴(Issue にする単位)](#4-コードの穴issue-にする単位)
5. [人の作業のチェックリスト](#5-人の作業のチェックリスト)

## 1. 結論

**早く出す(推奨)。ただし、次の 3 つを満たしてから出す。**

1. **課金なしの版で出す**: Pro(App 内課金)は入れない。Issue の方針(課金は Organization に移ってから)に合わせる。今のコードでは、サーバーが Pro を判定できない(`ProChecker` が無い)。そのため「Pro で上限が無くなる」と書くと、事実と違う説明になる(4 章の G1)。
2. **審査用の版から、さらに外す**: 他の人とデータを見る作り(entity の `scope: shared / public`・`chatThread`)、`webView`、aiButton のオンライン版。
   - どれも、今の審査用の版に残っている。
   - 残すと、1.2(通報・ブロックが無い)、審査メモの「WebView を使わない」「記録の中身は送らない」、審査メモと課金の説明の食い違い、のどれかに当たる(4 章の G2〜G4)。
3. **法務の文書・接続先・プライバシーマニフェストを本番用にする**: アプリに入っているプライバシーポリシー・利用規約は「下書き・[要確認]」のまま表示される。審査用の版の接続先は stg。`PrivacyInfo.xcprivacy` も無い(4 章の G5〜G7)。

これらは、2.5.2・4.7・1.2 の大きな論点を避けられる。このとき最初の版で外す機能は、次の 2 つを合わせたもの。

- 既に外しているもの(`docs/ios/review-build.md` の表)
  - リリース申請・探す・共有リンク
  - HealthKit・背面の位置
  - 近くの端末(すれ違いを含む)
  - センサー・撮影・ドラレコ・録音
- 上の 2 で外すもの
  - 共有・公開のデータ・chatThread
  - webView
  - aiButton のオンライン版
  - Pro

**開発者アカウントの名義(#309)は、早く出す前に決める。** 個人の名義で出して後で Organization に移すと、App Transfer が要る。Sign in with Apple のユーザーの移行(transfer identifier)も要る **(推測。Apple の手順は要確認)**。法人の登録が近いなら、App Store Connect への登録はそちらの名義で行う。コードの作業(4 章)は、名義を待たずに進められる。

## 2. 審査用の版のビルド結果

- コマンド: `ios/scripts/build-review.sh build -derivedDataPath <scratchpad>/dd-shared`
  - スクリプト自体にはロックが無い。そのため、`nice -n 15 /usr/bin/lockf -k <scratchpad>/xcodebuild.lock` を前に付けて実行した。
- 結果: **`** BUILD SUCCEEDED **`**。終了コードは 0。警告は 12 件で、非推奨の API・使っていない値などだけ。
- できた `Review-iphonesimulator/Metamon.app` を、`docs/ios/review-build.md` の「外れたことの確かめ方」で確かめた。
  - `UIBackgroundModes` が `[fetch]` だけになっている。
  - `NSHealth*` / `NSMotion*` / `NSLocalNetwork*` / `NSBonjourServices` / `NSNearbyInteraction*` / `CFBundleURLTypes` が無い。
  - カメラ・マイク・位置・写真への追加の説明文が、審査用の文に替わっている。
  - `HKHealthStore|MCSession|NISession|CMPedometer|CMMotionManager|CMAltimeter|UIImagePickerController|AVAudioRecorder|MPRemoteCommand|release-requests|public-apps` の文字列が無い。
  - 外した例(`health` / `walk_route` / `attendance_nearby` / `dashcam` / `sensors`)が無い。
- **残っていたもの**(4 章の穴に対応する)
  - `WKWebView` が残っている(webView 部品)。
  - `NSAppleMusicUsageDescription` と `NSSupportsLiveActivities` が残っている。どちらも残す想定のもの。
  - 共有・公開の例(`chat` / `board` / `ranking` / `post_comments`。`scope: public`)が同梱されている。
  - `PrivacyInfo.xcprivacy` が無い。
- 実機向けのアーカイブ(`archive`)は、署名に Team が要るので実行していない。

## 3. ガイドラインごとの状態

状態: **OK** = 今のコードで足りる / **要対応** = コードの穴(4 章)/ **人の作業** = 5 章

| 条項 | 論点 | 状態 | 根拠(コード) |
|---|---|---|---|
| **2.5.2** 実行コード | DSL は JSON。部品はバイナリに入っている。知らない部品は空の枠で出す。式は閉じた評価器で計算する | OK | `Packages/MetamonDSL/.../Model/Component.swift`(`ComponentType` は固定)、`Packages/MetamonRenderer/.../Registry/ComponentRegistry.swift` |
| 2.5.2 | WhisperKit の音声モデル(約 500MB)はデータで、確認してからダウンロードする | OK | `Metamon/Features/Voice/VoiceInputView.swift`(`downloadButton`)、`Metamon/App/BuildConfig.swift` |
| 2.5.2 | aiButton は端末内の Foundation Models を使う。推論のコードは OS 側にある | OK | `Packages/MetamonRenderer/.../Support/AIGenerate.swift`・`OnDeviceAI.swift` |
| 2.5.2 / 審査メモ | `webView` が審査用の版に残っている。審査メモの「WebView を使わない」と食い違う | **要対応**(G3) | `ComponentRegistry.swift:106`、`ClientCapabilities.reviewExcludedComponents` に `.webView` が無い |
| **4.7 / 4.7.2** ネイティブ API | HealthKit・背面の位置・近くの端末・センサー・撮影・ドラレコ・録音はバイナリから外れている | OK | `ClientCapabilities.swift:8`、`scripts/strip-review-plist.sh`、2 章の確かめ方 |
| 4.7.2 | 残っている権限: QR(カメラ)・現在地 / 道案内(使用中だけ)・写真ピッカー・Apple Music・通知・ライブアクティビティ。どれも本体が求めて、本体の UI で使う | OK(審査メモで説明する) | `RouteGuideSession.swift`(`CLBackgroundActivitySession` を使わない)、`AppleMusicPlayback.swift`、`AppRuntime+LiveActivity.swift` |
| 4.7 / 4.2.6 | リリース申請・探す(storefront)が外れている | OK | `Metamon/Features/Share/ReleaseRequestSheet.swift`・`DiscoverSheet.swift`(`#if !APP_STORE_REVIEW`) |
| 4.7 / 1.2.1 | 書き出し(#421)は**記録**を CSV / PDF にするだけ。記録帳(DSL)は書き出せない | OK(審査メモで言い分ける) | `Metamon/App/AppRuntime+Export.swift` |
| **1.2** UGC | 共有リンク・探す・近くの端末は外れている | OK | `RootView.swift:120`(`SharingHooks` / `NearbySheets`) |
| 1.2 | AI は審査用の版でも `scope: public / shared`(掲示板・ランキング・chatThread)を作る。画面には「このデータは誰でも見られます(公開)」が出る。通報・ブロック・フィルタの API は backend に無い | **要対応**(G2) | `backend/internal/ai/prompt.go:53`・`chat_thread_rules.go`、`backend/internal/ai/restrict.go`(`fieldHealth` / `cameraCapture` だけ)、`PublicDataNotice.swift`、`backend/internal/appconfig/handler.go`(report / block の経路が無い) |
| **3.1.1 / 3.1.2** 課金 | ペイウォールに価格・自動更新の説明・復元・規約がある | OK(出す場合) | `Metamon/Features/Purchase/PaywallSheet.swift` |
| 3.1.2 / 2.3.1 | 「Pro で上限が無くなる」と書いているが、サーバーは Pro を判定しない。そのため Pro でも 1 日 60 単位(作成 20 回)で止まる | **要対応**(G1) | `backend/internal/ratelimit/ratelimit.go:59,67`(`ProChecker` の実装が無い)、#314 / #316 |
| 3.1.1 | 審査用の版の有料機能は「共有データ」「オンラインの AI で書く」。どちらも審査用の版では意味が無いか、隠れている | **要対応**(G1・G2・G4) | `PaidGate.swift`、`backend/.../paid_features.json`、`AppRuntime+AIText.swift` |
| 3.1.1 | 生成アプリの中での決済はしない(案 B・外のリンクだけ) | OK | `docs/decisions/413-generated-app-payments.md` |
| **5.1.1(i)** ポリシー | 設定と同意画面からポリシーを開ける。ただし公開 URL が nil で、同梱の文に「下書き・[要確認]」が残っている | **要対応**(G5)+人の作業 | `Metamon/Features/Consent/LegalDocuments.swift:12`、`docs/legal/privacy-policy.md:3-9` |
| **5.1.1(v)** 削除 | アカウント削除・`DELETE /v1/me`・Sign in with Apple の失効(Firebase `revokeToken`)がある | OK(コード)+人の作業(鍵の登録) | `Metamon/Features/Settings/SettingsSheet.swift:91`、`MetamonServices/AccountDeletionFlow.swift`、`docs/legal/apple-token-revocation.md` |
| **5.1.2(i)** 第三者 AI | 初めて使う前に同意画面を出し、設定から取り消せる。同意文は「記録の中身は送らない」 | OK(G4 の後) | `Metamon/Features/Consent/AIConsentView.swift:32` |
| 5.1.2(i) | aiButton のオンライン版は `input`(最近の記録・フォームの値、4KB まで)を送る。同意文・ポリシー・審査メモと食い違う | **要対応**(G4) | `Packages/MetamonDSL/.../Model/AIText.swift`(`input`)、`Metamon/Features/Purchase/AppRuntime+AIText.swift` |
| **5.1** 権限の説明 | 審査用の版の説明文は、残る機能に合っている | OK(G8 は軽微) | `scripts/strip-review-plist.sh`、2 章 |
| 5.1 / ITMS-91053 | `PrivacyInfo.xcprivacy` が無い。UserDefaults(required reason API)を使っているので、アップロード時に警告か却下になる | **要対応**(G6) | `ios/` 以下に `PrivacyInfo.xcprivacy` が 0 件、`UserDefaults` を参照する箇所が 40 |
| **2.5.19** 隠した機能 | 外すのはバイナリから。サーバーのフラグで後から入れる機能は無い | OK | `docs/ios/review-build.md` |
| **2.1** 完全性 | 審査用の版の接続先が stg(`project.yml:40-41`) | **要対応**(G7)+人の作業 | `ios/project.yml:40` |
| 2.1 | musicPlayer は MusicKit の App Service が無いと、検索も再生もできない。Apple Music の契約が無い審査員には `.noSubscription` を出す | 人の作業 | `AppleMusicPlayback.swift:26`、`docs/ios/architecture.md:882` |

## 4. コードの穴(Issue にする単位)

状態(2026-10-30・PR #455): G1(#450)・G2(#451)・G3・G4・G6・G7・G8・G9(#452)は対応済み。G5 は値が決まるのを待つ。

| ID | 内容 | 大きさ | 関連 |
|---|---|---|---|
| G1 | **課金なしの審査用の版**(対応済み・#450。端末の無料枠も StoreKit の商品 ID もバイナリに無い。上限に当たった時は既存の状態カード「今日の上限に達しました。〜からまた使えます」)。`APP_STORE_REVIEW` では次をする。<br>・`ProGate` の課金画面と設定の「Tailor It Pro」を外す(`ProSettingsSection` / `PaywallSheet` の入口)<br>・StoreKit の取引の監視をしない<br>・端末の 1 日 3 回の上限の代わりに、サーバーの上限(`ratelimit`)に当たった時の文言だけを出す<br>・掲載文と審査メモから IAP の段落を外す版を用意する | 中 | #326・#314・#316 |
| G2 | **共有・公開のデータを審査用の版で作らせない**(対応済み・#451。`recipes` も scope public なので外した)。<br>・`ClientCapabilities.reviewDisabledFeatures` に `sharedData` を足す<br>・backend の `restrict.go` で、その指定があれば `scope` を外して private にし、プロンプトの scope / chatThread の規則も外す<br>・`chatThread` を `reviewExcludedComponents` に足す<br>・同梱の例 `chat` / `board` / `ranking` / `post_comments` を `DSLExamples.reviewExcluded` と `strip-review-plist.sh` に足す | 中 | #321(戻す時の前提)・#329 |
| G3 | **`webView` を審査用の版から外す**。<br>・`reviewExcludedComponents` に足し、`ComponentRegistry` の登録を `#if !APP_STORE_REVIEW` で囲む<br>・`gallery-web` の例も外す<br>・外したら、審査メモ 1 章の確認の行が満たされる | 小 | `docs/store/app-review-notes.md` 1 章 |
| G4 | **aiButton のオンライン版(`POST /v1/text`)を審査用の版で無効にする**。<br>・`supportsOnlineAIText` を `APP_STORE_REVIEW` では false にする<br>・端末内の AI が無い端末では、今までどおり理由つきで無効にする<br>・通常の版で残すなら、同意文とポリシーに「AI で書くボタンでは、そのボタンの材料(最近の記録)を送る」を足す | 小 | #417 |
| G5 | **法務の文書を本番用にする**。<br>・`docs/legal/privacy-policy.md` / `terms.md` の [要確認] を、人の決めた値で埋める<br>・審査用の版から外した機能の記述(HealthKit・共有)を、版に合わせて直す<br>・`LegalDocuments.publicURL` に公開 URL を入れる<br>・同梱の文から「下書き」の注記を外す | 小(値が決まれば) | #246・#331 |
| G6 | **`PrivacyInfo.xcprivacy` を足す**。<br>・UserDefaults(CA92.1)などの required reason API を書く<br>・集めるデータの種類を書く<br>・栄養ラベルと合わせる<br>・Firebase などの SDK のマニフェストが入っているかも確かめる | 小 | #328 |
| G7 | **審査用の版の接続先を prod にする**。<br>・Review 構成だけ `METAMON_AI_BASE_URL` / `METAMON_APPCONFIG_BASE_URL` を prod にする。今は base が stg<br>・または `build-review.sh archive` で prod の値を渡す<br>・GoogleService-Info.plist の切り替えは #292 | 小 | #292・#289 |
| G8 | (軽微)審査用の写真への追加の説明文に、`qrCode` の保存を足す。今は「共有ボタンから」だけ | 小 | `scripts/strip-review-plist.sh` |
| G9 | (軽微)`build-review.sh` に `xcodebuild` のロックが無い。並行でビルドすると取り合いになる。`lockf` を使う形にするか、手順書に書く | 小 | `ios/scripts/build-review.sh` |

G1〜G7 を入れたら、`swift test ... --filter ReviewBuildTests -Xswiftc -DAPP_STORE_REVIEW` と `build-review.sh build` を通す。さらに 2 章の確かめ方で、`WKWebView` と同梱の公開の例が消えたことを確かめる。

## 5. 人の作業のチェックリスト

**決めること**
- [x] 早く出すか(この文書の結論)と、課金なしで出すか(G1)を決める(2026-10-30: 課金なし・共有なしで出す。#450 / #451)
- [ ] Apple Developer Program の名義(個人 / 法人)を決める(#309)。法人なら D-U-N-S の取得を待つか決める
- [ ] 運営者名・連絡先メール・施行日・物理削除までの期間・年齢区分・管轄を決める(ポリシーと規約の [要確認])

**Apple Developer(Certificates, Identifiers & Profiles)**
- [ ] App ID `jp.kz2601.metamon` の Capabilities を確かめる
  - 有効にするもの: Sign in with Apple・App Groups(`group.jp.kz2601.metamon`)
  - HealthKit は審査用の entitlement に無いので、無くてもよい
- [ ] App ID `jp.kz2601.metamon` の **App Services → MusicKit** を有効にする(`docs/ios/architecture.md` §musicPlayer)。entitlement のファイルには無いので、`-allowProvisioningUpdates` では付かない
- [ ] ウィジェットの App ID `jp.kz2601.metamon.widget` と、App Group を確かめる
- [ ] Sign in with Apple の鍵(.p8)を作り、Firebase の Apple のプロバイダに登録する(`docs/legal/apple-token-revocation.md`)。**.p8 はリポジトリ・チャットに置かない**

**App Store Connect**
- [ ] アプリを登録する
  - 名前: Tailor It
  - バンドル ID: `jp.kz2601.metamon`
  - SKU を決める
  - 主言語: 日本語
- [ ] 掲載文・キーワード・カテゴリ(仕事効率化)を入れる([listing.md](../store/listing.md))。課金なしで出すので Pro の段落を外す(審査メモの IAP の段落は外した)
- [ ] プライバシーポリシーの URL・サポートの URL を入れる(#331・#320)。サポートのページに問い合わせ先を書く
- [ ] App のプライバシー(栄養ラベル)を入れる(#328)
  - 外した機能(健康・常時の位置・ローカルネットワーク)は書かない
  - 書くもの: ユーザーのコンテンツ・音声(端末内で書き起こし、文字だけを送る)・識別子(uid)
- [ ] 年齢区分の質問票に答える(#329。G2 を入れたので、他人とのやり取りは無い)
- [ ] 輸出のコンプライアンス: `ITSAppUsesNonExemptEncryption = false` が入っていることを確かめる
- [ ] 審査用のサインイン情報(デモアカウント)を「サインイン情報」欄に入れる。リポジトリには書かない
- [ ] 審査メモ([app-review-notes.md](../store/app-review-notes.md) 2 章)を貼る
  - 1 章の表を全部確かめてから貼る
  - 連絡先は貼る時に埋める
- [ ] (課金ありで出す場合だけ)次をして、版と一緒に審査に出す(#326)
  - 有料 App 契約・口座・税の登録
  - サブスクリプション `jp.kz2601.metamon.pro.monthly` の登録
  - 審査用のスクリーンショットの用意

**ビルドと提出**
- [ ] G1〜G7 が入った develop で、`ios/scripts/build-review.sh archive -allowProvisioningUpdates DEVELOPMENT_TEAM=<TEAM_ID> CODE_SIGN_STYLE=Automatic` を実行し、prod 向けに作る
- [ ] Organizer からアップロードし、TestFlight の内部テストで確かめる。確かめること:
  - 声で作る・直す
  - 同意画面
  - アカウント削除と Apple 側の連携の解除
  - Apple Music の検索
  - QR を読む
  - 道案内
- [ ] スクリーンショットは審査用の版で撮る(外した機能を映さない)
- [ ] 審査の期間中は、prod の ai / appconfig を動かしておく。Claude の利用上限・Cloud Run の上限も確かめる
- [ ] 審査に出す。却下された時の対応は #332 の想定問答に従う
