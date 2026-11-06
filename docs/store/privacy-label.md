# App のプライバシー(栄養ラベル)の答え

#328。App Store Connect の「App のプライバシー」に入れる答えと、その根拠のコード。`ios/Metamon/PrivacyInfo.xcprivacy` の `NSPrivacyCollectedDataTypes` と 1 対 1 で揃える(片方を変えたらもう片方も変える)。

- 対象: App Store に出す**審査用の版**(Review 構成・`APP_STORE_REVIEW`)。外した機能は [../ios/review-build.md](../ios/review-build.md)。
- ポリシー: [../legal/privacy-policy.md](../legal/privacy-policy.md)(ここでは変えない。ずれは 5 章)。
- ASC への入力は人の作業(6 章)。

## 目次

1. [前提の答え](#1-前提の答え)
2. [集めるデータ(ASC の質問の順)](#2-集めるデータasc-の質問の順)
3. [集めないもの](#3-集めないもの)
4. [通常の版だけで増えるもの](#4-通常の版だけで増えるもの)
5. [要判断](#5-要判断)
6. [理由が要る API(プライバシーマニフェスト)](#6-理由が要る-apiプライバシーマニフェスト)
7. [入力と確認の手順](#7-入力と確認の手順)

## 1. 前提の答え

| 質問 | 答え | 根拠 |
|---|---|---|
| データを集めますか | はい | 下の 2 章 |
| トラッキングに使いますか | いいえ | `NSPrivacyTracking` false・トラッキングのドメインなし・広告 ID なし・広告 SDK なし(Firebase Analytics も入れていない) |

## 2. 集めるデータ(ASC の質問の順)

ASC はデータの種類ごとに「用途 → ユーザーに紐づくか → トラッキングに使うか」を聞く。全部「App の機能」「紐づく」「トラッキングしない」。

| データの種類(ASC の表記) | 集める物 | 送り先 | 用途 | 紐づく | トラッキング | 根拠のコード |
|---|---|---|---|---|---|---|
| 連絡先情報 > 名前 | Sign in with Apple が渡した時の名前 | Firebase Authentication | App の機能 | はい | いいえ | `FirebaseAuthService.signInWithApple(fullName:)` |
| 連絡先情報 > メールアドレス | Sign in with Apple が渡した時のメール(非公開を選ぶと中継用のアドレス) | Firebase Authentication | App の機能 | はい | いいえ | 同上(Firebase が Apple の ID トークンから保存) |
| ユーザーコンテンツ > 写真またはビデオ | 記録に入れた写真の縮小画像(800px 以下の JPEG)・動画のサムネイル。動画そのものは端末内 | Firestore | App の機能 | はい | いいえ | `PhotoCodec`・`VideoFieldLeaf`(`video:<id>\|<サムネイル>`)・`HTTPAppConfigClient` の records |
| ユーザーコンテンツ > その他のユーザーコンテンツ | 記録帳の設計(DSL)と版の履歴・記録・AI に送る依頼の文と会話 | Firestore / Anthropic(サーバー経由) | App の機能 | はい | いいえ | `HTTPAppConfigClient`(`v1/apps/…`)・`HTTPAIClient`(`v1/generate`・`v1/chat`) |
| 位置情報 > 正確な位置情報 | 本人が記録に入れた場所(`locationButton` の緯度経度・小数 6 桁) | Firestore | App の機能 | はい | いいえ | `LocationButtonLeaf`(`%.6f,%.6f`) |
| ID > ユーザー ID | Firebase の uid | Firebase Authentication / Firestore / Crashlytics | App の機能 | はい | いいえ | `FirebaseAuthService`・`FirebaseCrashReporter.setUserID` |
| 診断 > クラッシュデータ | 落ちた場所・種類・日時 | Firebase Crashlytics | App の機能 | はい | いいえ | `FirebaseCrashReporter`(設定でオフにできる) |
| 診断 > パフォーマンスデータ | 落ちた時の空きメモリ・空き容量など | Firebase Crashlytics | App の機能 | はい | いいえ | 同上(5 章の要判断) |
| 診断 > その他の診断データ | 機種・OS・版・画面の向き、Firebase のインストール識別子 | Firebase Crashlytics / Installations / Auth | App の機能 | はい | いいえ | 同上。SDK のマニフェスト(Crashlytics・Auth・Installations)も宣言している |

- 「紐づく」の理由: どれも uid と一緒に保存される(Crashlytics も `setUserID` で uid を付ける)。
- 「App の機能」の理由: 本人のデータを本人だけが使えるようにする・端末を変えても使える・AI で記録帳を用意する・不具合を直す(Apple の定義ではクラッシュを減らすのも App の機能)。
- Anthropic には uid を送らない(`backend/internal/ai/client.go` の送信内容は system・messages だけ)。ただ、同じ文はサーバーで uid と一緒に扱うので「紐づく」とする。

## 3. 集めないもの

| 種類 | 理由 |
|---|---|
| 音声データ | 端末内で文字にする(WhisperKit)。録音はこの版に無い |
| 健康とフィットネス | ヘルスケア・センサーはこの版に無い |
| 財務情報・購入 | この版は App 内課金なし(#450) |
| 使用状況データ | 分析ツールなし。aiButton の集計(`v1/metrics/ai-button`)はこの版に無い |
| 連絡先・閲覧履歴・検索履歴・機密情報 | 読まない・送らない |
| デバイス ID | IDFA・IDFV を使わない。Firebase のインストール識別子は「その他の診断データ」に入れた |
| おおよその位置情報 | 位置は本人が入れた正確な値だけ |
| カスタマーサポート | アプリ内のフィードバック(`v1/feedback`)はこの版に無い |

- Hugging Face(音声モデルの配信)へは IP とファイルのリクエストだけで、運営者は受け取らない。サーバーのログの IP(Cloud Logging・30 日)も選ばない(5 章)。
- ファイルの取り込み(`fileImport`)は、選んだ CSV・テキスト・PDF を端末内で読んで記録にするだけ。ファイルそのものは送らない・保存しない。取り込んだ記録は 2 章の「その他のユーザーコンテンツ」(記録)と同じ扱いなので、申告は変わらない。
- Firebase App Check(App Attest / DeviceCheck)は端末とアプリが正規かの確認で、集めるデータに当たらない。

## 4. 通常の版だけで増えるもの

通常の版(Debug / Release・TestFlight)には次がある。審査用の版に戻す時は、このファイル・マニフェスト・ポリシーを先に足す。

| 機能 | 増える種類(案) | 送り先 |
|---|---|---|
| App 内課金(`v1/me/entitlements/apple`) | 購入 > 購入履歴 | 自前のサーバー(署名付きの取引) |
| aiButton の集計(`v1/metrics/ai-button`) | 使用状況データ > 製品の操作 | 自前のサーバー |
| aiButton のオンライン版(`v1/text`) | その他のユーザーコンテンツ(記録の文・最大 4KB を Anthropic へ) | サーバー経由で Anthropic |
| フィードバック(`v1/feedback`) | カスタマーサポート・その他の診断データ・写真(スクショ) | 自前のサーバー([../legal/feedback-privacy-draft.md](../legal/feedback-privacy-draft.md)) |
| 共有・公開・仲間探し・通報 | その他のユーザーコンテンツ(他の人に見える)・名前(表示名) | 自前のサーバー |
| 背面の位置の記録(`locationTracker`) | 正確な位置情報(連続した点) | Firestore |

## 5. 要判断

- **パフォーマンスデータ**: Crashlytics 自身のマニフェストは「クラッシュデータ」「その他の診断データ」だけで、パフォーマンスデータを宣言していない。Apple の定義(起動時間・ハング率・電力)に当たる物は送っていないので、外してよい見込み **(推測)**。外すならマニフェストからも消す。
- **AI の 1 日の回数**: ポリシー 1.4 のとおり、uid ごとの回数(匿名は IP を変換した値ごと)を日ごとに保存する。制限のためだけで分析に使わないので「使用状況データ」には入れない見込み **(推測)**。
- **サーバーのログの IP アドレス**(Cloud Logging・30 日): 障害の調査のためで、栄養ラベルの対象外と考えて選ばない **(推測)**。

## 6. 理由が要る API(プライバシーマニフェスト)

#328 でアプリ本体・ウィジェット・ローカルパッケージを grep した結果。

| API の種類 | 使う所 | 理由コード | 本体 | ウィジェット |
|---|---|---|---|---|
| UserDefaults | 設定・同意・タイマー・通知の予約など(`UserDefaults.standard`・`@AppStorage`) | CA92.1 | ○ | |
| UserDefaults | App Group でウィジェットに渡す `WidgetSnapshot` | 1C8F.1 | ○ | ○ |
| 空き容量 | `ModelDownloadAdvice`(音声モデルの前の確認と不足分の表示)・`DashcamRecorder`(通常の版だけ) | E174.1・85F4.1 | ○ | |
| ファイルの日時 | 使わない(`fileImport` もファイルの中身を読むだけで、作成日・更新日は読まない) | - | | |
| システムの起動時間 | 使わない(`ContinuousClock` は対象外) | - | | |
| アクティブなキーボード | 使わない | - | | |

依存パッケージ(`Package.resolved`・firebase-ios-sdk 13.1.1・argmax-oss-swift 1.1.1):

| パッケージ | 自前のマニフェスト | メモ |
|---|---|---|
| FirebaseCore / CoreInternal / CoreExtension / Auth / Crashlytics / Installations | あり | Auth・Crashlytics は UserDefaults(CA92.1)と集めるデータを宣言 |
| GoogleUtilities / GoogleDataTransport / nanopb / Promises / GTMSessionFetcher | あり | |
| FirebaseAppCheck / FirebaseSessions(Crashlytics の依存)/ AppCheckCore(app-check 12.0.0) | なし | UserDefaults を使う。静的にリンクされるので本体のマニフェストの CA92.1 で覆う。どれも Apple の「よく使われる SDK」の一覧に無い |
| WhisperKit / ArgmaxCore | なし | 理由が要る API を使わない(`attributesOfItem` はサイズだけ) |

## 7. 入力と確認の手順

1. **【人】** ASC > App のプライバシー > 「データの収集」で 1 章・2 章のとおりに入れる(5 章を決めてから)。
2. **【人】** プライバシーポリシーの URL に https://tailor-it.app/privacy を入れる。
3. アーカイブの後、Xcode の Organizer > Generate Privacy Report で、2 章の表と食い違いが無いか見る(SDK のマニフェストもまとめて出る)。
4. アップロードの後、Apple からプライバシーの警告メール(ITMS-91053 など)が来ていない。
