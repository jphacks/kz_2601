# ストアの掲載文の案

#247。App Store Connect に入れる名前・サブタイトル・プロモーション用テキスト・説明文・キーワード・URL・年齢区分・プライバシーの答え・スクリーンショットの文言。

- 前提の版: 審査用の版(`Metamon-Review` / `APP_STORE_REVIEW`、[review-build.md](../ios/review-build.md))。最初の版は **App 内課金なし**(#450)・**共有 / 公開のデータなし**(#451)・**`webView` なし**・**aiButton のオンライン版なし**(#452)で出す(2026-10-30 決定)。
- 方針: 「アプリを作る」「コードを実行する」と読める言い方をしない。言うのは「**自分専用の記録帳・ツール**を、話して用意して、話して直す」。理由はガイドライン 2.5.2(コードのダウンロード)・4.7(アプリ内のソフトウェア)・2.5.19(書かれていない機能)と、2026 年に「アプリビルダー」「App Store に提出」という売り方が問題にされた前例(調査メモ [P1]〜[P3])。
- 審査用の版で外した機能(共有リンク・探す・リリース申請・HealthKit・道のりの記録・近くの端末・センサー・撮影・録音・ドラレコ・Web 表示・人どうしのチャット・App 内課金・共有 / 公開の記録)は**書かない**。書いた機能は審査用の版に必ずある(1 章の照合表)。
- 1 日の上限は数を書かない。審査用の版は端末で数えず、サーバーの `ratelimit` だけで止まる。数を書くと、上限を変えた時に掲載文が事実と違ってしまう([app-review-notes.md](app-review-notes.md) と同じ)。
- 避ける言い回しの一覧は [wording.md](wording.md)。審査メモは [app-review-notes.md](app-review-notes.md)。

## 目次

1. [日本語](#1-日本語)
2. [英語](#2-英語)
3. [スクリーンショット](#3-スクリーンショット)
4. [カテゴリと年齢区分](#4-カテゴリと年齢区分)
5. [App のプライバシー(栄養ラベル)の答え](#5-app-のプライバシー栄養ラベルの答え)
6. [出す前の確認](#6-出す前の確認)

## 1. 日本語

| 項目 | 文 |
|---|---|
| 名前 | Tailor It |
| サブタイトル | 話して用意する、自分専用の記録帳 |
| プロモーション用テキスト | 「水を飲んだ量を記録したい」と話すだけで、自分専用の記録帳ができます。「週の合計も見たい」と話せば、その場で形が変わります。 |
| キーワード | 記録,習慣,日記,家計簿,トラッカー,チェックリスト,音声,メモ,目標,筋トレ,水分,買い物リスト,ルーティン,カスタマイズ |

説明文:

```text
Tailor It は、話すだけで自分専用の記録帳・ツールが手に入るアプリです。

「毎日の水分を記録して、1 日 2 リットルを目標にしたい」
「筋トレの回数をつけて、週ごとのグラフで見たい」
「買い物リストを、お店ごとに分けたい」

話した内容に合わせて、入力欄・一覧・合計・グラフ・リマインダーをそろえた画面が用意されます。使いながら「ボタンを大きく」「月の合計も出して」と話せば、その場で形が変わります。

■ できること
・声か文字で、記録帳・チェックリスト・家計簿などを用意
・話して直す(項目を足す・並べ替える・グラフを足す)
・前の形にいつでも戻せる
・合計・平均・連続日数などを自動で集計
・毎日のリマインダー(この端末の通知)
・タイマーや数字をロック画面に表示(ライブアクティビティ)
・写真ライブラリの写真を添えて記録
・今いる場所を記録して、地図で見返す
・目的地までの道案内
・QR コードを読み取って入力
・記録を CSV / PDF に書き出す
・Apple Music の曲を探して再生(Apple Music の契約が必要です)
・いくつもの記録帳を切り替えて使う

■ プライバシー
・声の書き起こしは、この端末の中で行います
・画面を用意するために、話した内容などを AI(Anthropic)に送ります。送る前に説明を表示し、同意をいただきます。記録の中身は送りません
・アカウントは設定からいつでも削除できます

■ 料金
・このバージョンは無料で、App 内課金はありません
・AI を使う回数には 1 日の上限があります。上限に達した時は、アプリ内でお知らせします

利用規約: https://tailor-it.app/terms
プライバシーポリシー: https://tailor-it.app/privacy
```

このバージョンの新機能(初版):

```text
最初のバージョンです。話して、自分専用の記録帳を用意して、話して直せます。
```

### 機能の行と審査用の版の照合

説明文の「できること」の各行が、審査用の版のどこにあるか。外した部品は `ClientCapabilities.reviewExcludedComponents`(`healthStat` / `locationTracker` / `nearbyShare` / `dashcam` / `sensorValue` / `audioRecorder` / `webView` / `chatThread`)と `reviewDisabledFeatures`(`fieldHealth` / `cameraCapture` / `sharedData`)。Info.plist は `ios/scripts/strip-review-plist.sh` が削る。

| 行 | 審査用の版にあるもの | 外れていないことの根拠 |
|---|---|---|
| 用意する・話して直す・前の形に戻す | 音声入力(マイク FAB)・文字入力・版の履歴 | review-build.md の「残すもの」 |
| 集計 | `stat` / `chart` / `table` | 除外リストに無い |
| リマインダー | `UserNotificationScheduler`(ローカル通知) | `#if` で囲まれていない |
| ライブアクティビティ | `AppRuntime+LiveActivity`(`timerDisplay.liveActivity`) | `#if` で囲まれていない。`NSSupportsLiveActivities` は残す(m0 readiness) |
| 写真ライブラリ | `photoField` / `videoField` の PhotosPicker | 撮影(`cameraCapture`)だけ無効 |
| 場所と地図 | `locationButton` / `mapView`(使用中の位置だけ) | 外したのは背面の `locationTracker` |
| 道案内 | `routeGuide`(使用中の位置だけ) | 除外リストに無い |
| QR | `qrScanner`(カメラは QR だけ) | 除外リストに無い。カメラの説明文は QR 用に差し替え |
| 書き出し | `export` アクション(`AppRuntime+Export`。記録だけで、記録帳は書き出せない) | `#if` で囲まれていない |
| Apple Music | `musicPlayer`(友だちからのリクエストの UI だけ外れる) | `NSAppleMusicUsageDescription` を残す。**App ID で MusicKit を有効にしないと動かない**(6 章) |
| 切り替え | 記録帳の一覧(`AppSwitcherSheet`) | 「探す」の入口だけ外れる |

書かないもの:
- 共有・シェア(`shareButton` は審査用の版にもあるが、[wording.md](wording.md) の「共有」を避ける)。
- 写真の文字の読み取り(`photoField.analyze`)と、端末内 AI の「AI で書く」(Apple Intelligence の無い端末では使えない)。

## 2. 英語

| 項目 | 文 |
|---|---|
| Name | Tailor It |
| Subtitle | Personal trackers, by voice |
| Promotional Text | Say "track how much water I drink" and get a tracker that's yours alone. Say "show a weekly total" and it changes right away. |
| Keywords | tracker,habit,log,journal,budget,checklist,voice,goal,workout,water,shopping list,routine,personal |

Description:

```text
Tailor It gives you a tracker made just for you. Say what you want to keep track of, and it's ready.

"Log the water I drink, with a goal of 2 liters a day."
"Count my push-ups and show a weekly chart."
"A shopping list grouped by store."

Tailor It sets up a screen with the fields, lists, totals, charts and reminders you asked for. As you use it, say "make the button bigger" or "show the monthly total", and it changes right away.

WHAT YOU CAN DO
- Set up trackers, checklists and budgets by voice or text
- Adjust them by talking: add fields, reorder, add charts
- Go back to any earlier version
- See totals, averages and streaks automatically
- Get daily reminders (local notifications on this device)
- Show a timer or a number on the Lock Screen (Live Activities)
- Attach photos from your photo library
- Save the place you're at and see it on a map
- Get directions to a destination
- Scan a QR code into a field
- Export your records as CSV or PDF
- Search and play songs from Apple Music (Apple Music subscription required)
- Keep several trackers and switch between them

PRIVACY
- Speech is transcribed on your device
- To set up your screen, what you say is sent to an AI (Anthropic). We explain this and ask for your consent first. The contents of your records are not sent
- You can delete your account at any time in Settings

PRICE
- This version is free and has no in-app purchases
- There is a daily limit on AI use. When you reach it, the app lets you know

Terms of Use: https://tailor-it.app/terms
Privacy Policy: https://tailor-it.app/privacy
```

What's New (first version):

```text
Our first release. Set up a tracker made just for you by talking, and adjust it by talking.
```

### ASC の欄

日本語・英語で共通。

| 欄 | 値 |
|---|---|
| サポート URL | https://tailor-it.app/ |
| マーケティング URL | https://tailor-it.app/ |
| 著作権 | 2026 高岡己太朗 |
| プライバシーポリシー URL | https://tailor-it.app/privacy |
| 利用規約 | 説明文に https://tailor-it.app/terms を書く。App Store Connect の「使用許諾契約(EULA)」は標準のまま **(推測)**。独自の EULA にするなら同じ URL の本文を入れる |

### 文字数

App Store Connect の上限と、上の文の文字数(Python の `len` で数えた。説明文・新機能は改行を含む)。

| 欄 | 上限 | 日本語 | 英語 |
|---|---|---|---|
| 名前 | 30 | 9 | 9 |
| サブタイトル | 30 | 16 | 27 |
| プロモーション用テキスト | 170 | 62 | 125 |
| キーワード | 100 | 62 | 98 |
| 説明文 | 4000 | 783 | 1569 |
| このバージョンの新機能 | 4000 | 37 | 91 |

## 3. スクリーンショット

文言は画像の上に入れる見出し。画面の中身(アプリの UI)にも [wording.md](wording.md) の言い回しが映り込まないようにする(下の注)。

| # | 画面 | 見出し(日本語) | Caption (English) |
|---|---|---|---|
| 1 | 声の画面で話している所 | 話すだけで、自分専用の記録帳 | Say it. Get a tracker made for you. |
| 2 | 「週のグラフも」と話した後 | 話せば、その場で形が変わる | Say "add a weekly chart". Done. |
| 3 | 合計・平均・連続日数のカード | 合計も連続日数も、自動で | Totals and streaks, automatically |
| 4 | 写真つきの記録・地図 | 写真や場所を添えて記録 | Add photos and places to your entries |
| 5 | 版の履歴 | 前の形にいつでも戻せる | Go back to any earlier version |

- 使う画面は審査用の版で撮る。外した機能(共有・探す・リリース申請・ヘルスケア・道のりの記録・撮影・Web 表示・課金画面)を映さない(2.5.19・2.3)。
- 例は個人の記録(水分・筋トレ・買い物・家計簿)にする。他社のロゴや実在の人名を映さない(5.2.1)。
- 注(UI の文言): **まだ直っていない**(2026-10-30 に `ios/Metamon/Resources/Localizable.xcstrings` を確認)。
  - 1 枚目の声の画面に出る「どんなアプリが欲しいですか?」「話しかけるだけで、アプリが生まれ変わる」「話すと、アプリができる」が残っている。
  - 切り替えの「新しいアプリ」「アプリを切り替えています…」「変身一覧」、同意画面の「アプリを作る・直すときは…」も残っている。
  - 撮る前に UI の文言を直すか、これらが映らない画面を選ぶ([wording.md](wording.md) の 4 章)。
  - 審査員が見る権限の確認も同じ。`strip-review-plist.sh` が差し替える説明文が「生成したアプリで…」「話しかけた内容からアプリを生成するために…」のまま。

## 4. カテゴリと年齢区分

| 項目 | 案 | 理由 |
|---|---|---|
| プライマリ | 仕事効率化(Productivity) | 記録・チェックリスト・リマインダーが中心。「開発ツール(Developer Tools)」にはしない |
| セカンダリ | ライフスタイル(Lifestyle) | 習慣・家計簿・買い物 |

### 年齢区分の質問票の答え(審査用の版)

項目は 2025 年に改定された App Store Connect の質問票の並び **(推測。画面の項目名と違えば、意味の近い項目に読み替える)**。

前提:
- 審査用の版には、他人とのやり取り・公開・アプリ内の Web 表示が無い。
- AI(Claude)は、ユーザーの依頼から記録帳の画面(項目名・見出し・説明の文)を作る。作った物は本人しか見ない。

| 区分 | 項目 | 答え | 理由 |
|---|---|---|---|
| アプリ内の管理 | ペアレンタルコントロール | いいえ | 機能が無い |
| | 年齢の確認(Age Assurance) | いいえ | 機能が無い |
| 機能 | 無制限の Web アクセス | いいえ | `webView` を外した。リンクは `openUrlButton` で Safari に渡すだけ |
| | ユーザー生成コンテンツ | いいえ | 記録・記録帳は本人だけが見る(`sharedData` 無効で backend が private に直す。共有リンク・探すを外した)。AI が作る画面の文も本人の依頼の結果で、ほかの人には出ない **(推測。AI の生成物を UGC と数えるなら「はい」にして、審査メモに 1.2 の説明を足す)** |
| | メッセージとチャット | いいえ | 人どうしのチャット(`chatThread`)を外した。AI に相談するチャットはあるが、相手は人ではない **(推測)** |
| | 広告 | いいえ | 広告の SDK なし |
| 成熟したテーマ | 不敬な言葉・下品なユーモア | なし | アプリの文言に無い。AI の出力は Anthropic の利用ポリシーとプロンプトの範囲 |
| | ホラー・恐怖 | なし | |
| | アルコール・タバコ・薬物の使用や言及 | なし | 記録の例に無い **(推測。ユーザーが「お酒の量を記録」と頼めば項目名に出るが、アプリが扱う題材ではない)** |
| 医療・ウェルネス | 医療・治療の情報 | いいえ | 医療の情報を出さない |
| | 健康・ウェルネスの話題 | いいえ | HealthKit を外した。水分・筋トレは記録帳の例で、健康の助言はしない **(推測。例として前に出すので「はい」もありうる)** |
| 性的な内容・ヌード | 成熟した・示唆的なテーマ / 性的な内容・ヌード / 露骨な性的内容 | なし / なし / なし | |
| 暴力 | アニメ・空想の暴力 / 現実の暴力 / 長く露骨な暴力 / 銃・武器 | なし / なし / なし / なし | |
| 偶然に左右される活動 | 疑似ギャンブル / コンテスト / ギャンブル / ルートボックス | なし / なし / いいえ / いいえ | `randomPicker`(くじ引き)は本人の選択を助けるだけで、賭けも景品も無い |
| その他 | キッズ向け(Made for Kids) | いいえ | |

- 答えの結果は **4+** の見込み **(推測)**。
- ただしプライバシーポリシー 5 章は「13 歳未満の方を対象にしていません」。4+ と食い違う。質問票の後で年齢区分を **13+ に引き上げる**(App Store Connect で上の区分を選べる)か、ポリシーの書き方を変えるかを決める(#329・要判断)。
- 共有を戻す版では、ユーザー生成コンテンツ・メッセージの答えが「はい」になり、通報・ブロック(1.2)が先に要る。

## 5. App のプライバシー(栄養ラベル)の答え

`ios/Metamon/PrivacyInfo.xcprivacy` と [privacy-policy.md](../legal/privacy-policy.md) から。審査用の版が対象(ヘルスケア・背面の位置・近くの端末・課金は入っていない)。

- データを集めるか: **はい**。
- トラッキング: **いいえ**(`NSPrivacyTracking` false・トラッキングのドメインなし・広告 ID なし)。

| データの種類(ASC の表記) | 集める物 | ユーザーに紐づく | トラッキング | 用途 |
|---|---|---|---|---|
| 連絡先情報 > 名前 | Sign in with Apple が渡した時の表示名(Firebase Authentication) | はい | いいえ | App の機能 |
| ID > ユーザー ID | Firebase の uid | はい | いいえ | App の機能 |
| ユーザーコンテンツ > 写真またはビデオ | 記録に添えた写真の縮小画像(Firestore) | はい | いいえ | App の機能 |
| ユーザーコンテンツ > その他のユーザーコンテンツ | 記録帳の設計と版の履歴・記録・AI に送る依頼の文と会話(Anthropic に渡す) | はい | いいえ | App の機能 |
| 位置情報 > 正確な位置情報 | ユーザーが記録に入れた場所(Firestore) | はい | いいえ | App の機能 |

選ばないもの: 音声(端末内で文字にする)・健康とフィットネス・財務情報・検索履歴・閲覧履歴・使用状況データ(分析ツールなし)・診断(クラッシュの収集なし)・購入(課金なし)。

要判断 **(推測)**:
- **メールアドレス**: ポリシー 1.1 では、Apple が渡した時は Firebase Authentication にメール(または中継アドレス)が保存される。xcprivacy には無い。運営者が Firebase のコンソールで読めるので、「連絡先情報 > メールアドレス(紐づく・トラッキングなし・App の機能)」も選ぶのが安全。選ぶなら xcprivacy にも足す。
- サーバーのログの IP アドレス(Cloud Logging・30 日)は、栄養ラベルの対象外と考えて選ばない。
- 「その他のユーザーコンテンツ」は、通常の版の共有・公開を含まない(審査用の版は `sharedData` 無効)。

## 6. 出す前の確認

- [ ] 名前・サブタイトル・説明文・キーワード・スクショの見出しに、[wording.md](wording.md) の「避ける言い回し」が無い
- [ ] 書いた機能が審査用の版にある(1 章の照合表)。共有・探す・ヘルスケア・道のりの記録・撮影・Web 表示・課金を書いていない
- [ ] Pro の段落と、1 日の上限の数(「1 日 3 つ」)を書いていない。App Store Connect にサブスクリプションを登録していない
- [ ] 文字数が上限に収まる(2 章の「文字数」。文を変えたら数え直す)
- [ ] 利用規約・プライバシーポリシー・サポート・マーケティングの URL が開ける(https://tailor-it.app/ ・ /terms ・ /privacy)。サポートのページに問い合わせ先がある(#331・#320)
- [ ] 著作権「2026 高岡己太朗」を入れた
- [ ] App ID で MusicKit を有効にした(しないなら説明文の Apple Music の行を消す)
- [ ] 年齢区分の質問票に 4 章のとおり答え、13+ に上げるかを決めた(#329)
- [ ] 栄養ラベルを 5 章のとおり入れ、メールアドレスを選ぶかを決めた。xcprivacy・ポリシーと一致している
- [ ] スクショの 1 枚目に「アプリ」と言う UI の文言が映っていない(3 章の注。UI の文言の修正は別の Issue)
- [ ] 審査メモ([app-review-notes.md](app-review-notes.md))と説明文の中身が一致している
