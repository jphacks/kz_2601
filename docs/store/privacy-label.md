# App のプライバシー(栄養ラベル)の答え

#328。App Store Connect の「App のプライバシー」に入れる答えと、その根拠のコード。`ios/Metamon/PrivacyInfo.xcprivacy` の `NSPrivacyCollectedDataTypes` と 1 対 1 で揃える(片方を変えたらもう片方も変える)。

- 対象: App Store に出す**審査用の版**(Review 構成・`APP_STORE_REVIEW`)。外した機能は [../ios/review-build.md](../ios/review-build.md)。
- ポリシー: [../legal/privacy-policy.md](../legal/privacy-policy.md)(ずれは 5 章)。2026-11 の改定(PR #663)で増えた購入・通知・自動の動きは、通常の版だけにある(4 章)。2026-11-08 の改定で、サーバーの利用の集計(1.12・全部の版)を「製品の操作(アナリティクス)」として 2 章に足した。
- ASC の答えを直す手順: [asc-privacy-update-steps.md](asc-privacy-update-steps.md)。
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

ASC はデータの種類ごとに「用途 → ユーザーに紐づくか → トラッキングに使うか」を聞く。全部「紐づく」「トラッキングしない」。用途は「App の機能」で、製品の操作とユーザー ID だけ「アナリティクス」がある(サーバーの利用の集計)。

| データの種類(ASC の表記) | 集める物 | 送り先 | 用途 | 紐づく | トラッキング | 根拠のコード |
|---|---|---|---|---|---|---|
| 連絡先情報 > 名前 | Sign in with Apple が渡した時の名前 | Firebase Authentication | App の機能 | はい | いいえ | `FirebaseAuthService.signInWithApple(fullName:)` |
| 連絡先情報 > メールアドレス | Sign in with Apple が渡した時のメール(非公開を選ぶと中継用のアドレス) | Firebase Authentication | App の機能 | はい | いいえ | 同上(Firebase が Apple の ID トークンから保存) |
| ユーザーコンテンツ > 写真またはビデオ | 記録に入れた写真の縮小画像(800px 以下の JPEG)・手書きの絵(`drawingField` で描いた絵。写真と同じ 800px 以下の JPEG で photo field に入る)・動画のサムネイル。動画そのものは端末内 | Firestore | App の機能 | はい | いいえ | `PhotoCodec`・`DrawingCodec`・`VideoFieldLeaf`(`video:<id>\|<サムネイル>`)・`HTTPAppConfigClient` の records |
| ユーザーコンテンツ > その他のユーザーコンテンツ | 記録帳の設計(DSL)と版の履歴・記録・AI に送る依頼の文と会話 | Firestore / Anthropic(サーバー経由) | App の機能 | はい | いいえ | `HTTPAppConfigClient`(`v1/apps/…`)・`HTTPAIClient`(`v1/generate`・`v1/chat`) |
| 位置情報 > 正確な位置情報 | 本人が記録に入れた場所(`locationButton` の緯度経度・小数 6 桁) | Firestore | App の機能 | はい | いいえ | `LocationButtonLeaf`(`%.6f,%.6f`) |
| ID > ユーザー ID | Firebase の uid(利用の集計では uid から作った `uid_hash`) | Firebase Authentication / Firestore / Crashlytics / Cloud Logging | App の機能・アナリティクス | はい | いいえ | `FirebaseAuthService`・`FirebaseCrashReporter.setUserID`・`backend/internal/metrics`(`UIDHash`) |
| 使用状況データ > 製品の操作 | サーバーが受けたリクエストから出す利用のイベント(`session.start`・`app.opened`・`app.shared`・`app.published`・`share.imported`・`ai.generate`)と成功・失敗。`uid_hash`・`app_hash` 付き。端末からの新しい送信は無い | 自前のサーバー(Cloud Logging・30 日) | アナリティクス | はい | いいえ | `backend/internal/metrics`・[../metrics/product-metrics.md](../metrics/product-metrics.md) 2 章 |
| 診断 > クラッシュデータ | 落ちた場所・種類・日時 | Firebase Crashlytics | App の機能 | はい | いいえ | `FirebaseCrashReporter`(設定でオフにできる) |
| 診断 > パフォーマンスデータ | 落ちた時の空きメモリ・空き容量など | Firebase Crashlytics | App の機能 | はい | いいえ | 同上(5 章の要判断) |
| 診断 > その他の診断データ | 機種・OS・版・画面の向き、Firebase のインストール識別子 | Firebase Crashlytics / Installations / Auth | App の機能 | はい | いいえ | 同上。SDK のマニフェスト(Crashlytics・Auth・Installations)も宣言している |

- 「紐づく」の理由: どれも uid と一緒に保存される(Crashlytics も `setUserID` で uid を付ける)。製品の操作の `uid_hash` は塩なしで、uid の一覧を持つ運営者は照合できるので「紐づく」とする。
- 「App の機能」の理由: 本人のデータを本人だけが使えるようにする・端末を変えても使える・AI で記録帳を用意する・不具合を直す(Apple の定義ではクラッシュを減らすのも App の機能)。
- Anthropic には uid を送らない(`backend/internal/ai/client.go` の送信内容は system・messages だけ)。ただ、同じ文はサーバーで uid と一緒に扱うので「紐づく」とする。

## 3. 集めないもの

| 種類 | 理由 |
|---|---|
| 音声データ | 端末内で文字にする(WhisperKit。モデルの準備中は Apple の端末内の音声認識 `requiresOnDeviceRecognition = true`・U5)。どちらも声を端末の外に送らない。録音はこの版に無い |
| 健康とフィットネス | ヘルスケア・センサーはこの版に無い |
| 財務情報・購入 | この版は App 内課金なし(#450) |
| 連絡先・閲覧履歴・検索履歴・機密情報 | 読まない・送らない |
| デバイス ID | IDFA・IDFV を使わない。Firebase のインストール識別子は「その他の診断データ」に入れた |
| おおよその位置情報 | 位置は本人が入れた正確な値だけ |
| カスタマーサポート | アプリ内のフィードバック(`v1/feedback`)はこの版に無い |

- 地図・場所の検索・住所・道案内(`mapView`・`locationButton`・`placeSearch`・`addressField`・`routeGuide`)は Apple の MapKit で、端末から Apple に直接問い合わせる。運営者も委託先も受け取らないので申告しない(ポリシー 2 章)。
- 保存した場所に着いた時の通知(`notification.near`)は iOS が端末の中で判定し、位置を送らない。
- 本の情報(`lookupBook`)は ISBN だけを運営者のサーバー経由で openBD へ送る。uid と結んで保存しない(キャッシュは ISBN ごとの書誌)ので申告しない。ISBN が Cloud Run のリクエストのログ(30 日)に残りうる点は 5 章のログの IP と同じ扱い。
- Hugging Face(音声モデルの配信)へは IP とファイルのリクエストだけで、運営者は受け取らない。サーバーのログの IP(Cloud Logging・30 日)も選ばない(5 章)。
- ファイルの取り込み(`fileImport`)は、選んだ CSV・テキスト・PDF を端末内で読んで記録にするだけ。ファイルそのものは送らない・保存しない。取り込んだ記録は 2 章の「その他のユーザーコンテンツ」(記録)と同じ扱いなので、申告は変わらない。
- 仮の聞き取り(U5)は端末内で使える時だけ使い、サーバーへ送る Apple の音声認識には落とさない。そのため「音声データは集めない」は変わらない。音声認識の許可(`NSSpeechRecognitionUsageDescription`)は増える。
- Firebase App Check(App Attest / DeviceCheck)は端末とアプリが正規かの確認で、集めるデータに当たらない。

## 4. 通常の版だけで増えるもの

通常の版(Debug / Staging / Release・TestFlight)には次がある。**どれも審査用の版のバイナリに無いので、App Store に出す審査用の版の申告(1・2 章・ASC の入力)は変わらない。**審査用の版に戻す時は、このファイル・マニフェスト・ポリシーを先に足す。ポリシーの節は 2026-11 の改定(PR #663)の番号。

| 機能 | 増える種類 | 送り先 | 用途 | 紐づく | ポリシー | 通常の版のマニフェスト |
|---|---|---|---|---|---|---|
| App 内課金(`v1/me/entitlements/apple`・App Store Server Notifications) | 購入 > 購入履歴(商品 ID・取引 ID・有効期限・返金・自動更新・環境・購読とユーザー ID の対応) | 自前のサーバー(Apple の署名を確かめて保存) | App の機能(Pro の判定) | はい | 1.7・3・4 章 | 足す |
| aiButton の集計(`v1/metrics/ai-button`) | 使用状況データ > 製品の操作 | 自前のサーバー | アナリティクス | はい | 1.12 | 2 章で宣言済み(サーバーの利用の集計と同じ種類) |
| aiButton のオンライン版(`v1/text`) | その他のユーザーコンテンツ(最近の記録・入力中の文字・最大 4KB。長い答えは最大 16KB。共有・公開のアプリでは自分の記録だけ・#733) | サーバー経由で Anthropic | App の機能 | はい | 1.3 | 2 章で宣言済み |
| aiChat(`v1/text` の task chat・#719) | その他のユーザーコンテンツ(自分の会話の直近 20 発言・16KB と AI の役割の説明。会話は private の記録として Firestore) | サーバー経由で Anthropic / Firestore | App の機能 | はい | 1.3・1.4 | 2 章で宣言済み |
| aiButton の写真(`inputPhoto`・#714。サーバーの `AI_TEXT_IMAGE` が既定 off) | 写真またはビデオ(フォームの写真 1 枚・JPEG・長い辺 1024px 以下) | サーバー経由で Anthropic | App の機能 | はい | 1.3 | 2 章で宣言済み(送り先に Anthropic が増えるだけ。5 章の健康の判断) |
| 外部の公開データ(`feed`)・国会会議録の語 | 増えない(出どころへ送るのは府県予報区・代表地点の座標・分野・DSL に書いた語だけ。uid・位置・記録は送らない) | 自前のサーバー → 出どころ | — | — | 1.11 | 足さない |
| フィードバック(`v1/feedback`) | カスタマーサポート・その他の診断データ・写真(スクショ) | 自前のサーバー([../legal/feedback-privacy-draft.md](../legal/feedback-privacy-draft.md)) | App の機能 | はい | 1.13・3・4 章 | カスタマーサポートを足す(ほかは宣言済み) |
| 共有・公開・仲間探し・通報 | その他のユーザーコンテンツ(他の人に見える)・名前(表示名) | 自前のサーバー | App の機能 | はい | 1.6 | 宣言済み |
| 背面の位置の記録(`locationTracker`) | 正確な位置情報(連続した点) | Firestore | App の機能 | はい | 1.5 | 宣言済み |
| プッシュ通知の端末の登録(`v1/me/devices`・#269) | ID > デバイス ID(APNs の端末のトークン・インストールごとの識別子) | 自前のサーバー(Firestore)・APNs | App の機能(通知を届ける) | はい | 1.10・3・4 章 | 足す |
| サーバーの自動の動き(`v1/apps/{id}/automations`・runner は未デプロイ) | 増えない(読むのは記録 = その他のユーザーコンテンツ。通知の本文に記録の値 30 字まで) | 自前のサーバー・APNs | App の機能 | はい | 1.10 | 宣言済み |

- **端末のトークンを「デバイス ID」にする理由**: Apple の定義は「Such as the device's advertising identifier, or other device-level ID」([App privacy details](https://developer.apple.com/app-store/app-privacy-details/))。APNs のトークンとインストールごとの識別子は、端末(のこのアプリ)を指す値で、サーバーにサインアウト・退会まで残す(「collect」= リクエストに応えるのに要る時間より長く残す、に当たる)。広告 ID ではないが「other device-level ID」に入るので、申告する側に倒す。審査用の版の Firebase のインストール識別子(診断の付属)は 2 章のとおり「その他の診断データ」のまま。
- **マニフェスト**: `ios/Metamon/PrivacyInfo.xcprivacy` の元のファイルは審査用の版の申告(2 章と 1 対 1)のまま。通常の版は、ビルドの最後に `ios/scripts/extend-privacy-manifest.sh` がバンドルの写しに上の表の「足す」の種類(`PurchaseHistory`・`CustomerSupport`・`DeviceID`)を足す(`ProductInteraction` はサーバーの利用の集計のため元のファイルで宣言済み)(Review 構成では何もしない)。この表とスクリプトの一覧を 1 対 1 に保つ。
- 通常の版を App Store に出すことにした時は、ASC の入力にこの章の「増える種類」を足す(人の作業・7 章)。

## 5. 要判断

- **パフォーマンスデータ**: Crashlytics 自身のマニフェストは「クラッシュデータ」「その他の診断データ」だけで、パフォーマンスデータを宣言していない。Apple の定義(起動時間・ハング率・電力)に当たる物は送っていないので、外してよい見込み **(推測)**。外すならマニフェストからも消す。
- **AI の 1 日の回数**: ポリシー 1.4 のとおり、uid ごとの回数(匿名は IP を変換した値ごと)を日ごとに保存する。制限のためだけで分析に使わないので「使用状況データ」には入れない見込み **(推測)**。
- **サーバーのログの IP アドレス**(Cloud Logging・30 日): 障害の調査のためで、栄養ラベルの対象外と考えて選ばない **(推測)**。
- **写真のボタンと健康**(ポリシー 1.3): 処方箋・薬の袋の写真を AI が読む用途は、Apple の「健康とフィットネス > 健康」(ユーザーが入れた健康・医療のデータ)に当たるかもしれない **(推測)**。`AI_TEXT_IMAGE` を on にする前に、用途を絞るか申告するかを人が決める。
- **aiButton の集計の「紐づく」**: uid は送らないが認証付きのリクエストで、同じ種類(製品の操作)のサーバーの集計が「紐づく」なので、種類としては「紐づく」で申告している。[../analytics/ai-button-metrics.md](../analytics/ai-button-metrics.md) の「本人に紐づかない」は、この集計だけを見た説明。
- **通知の受け取りの同意(アプリごと)・自動の動きの回数**(ポリシー 1.10): 同意の有無は設定、回数は使いすぎを防ぐためだけで分析に使わないので、AI の 1 日の回数と同じく申告しない見込み **(推測)**。iOS の通知の登録が入る時に見直す。

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
