# App Review への説明文(審査メモ)

#247。App Store Connect の「App Review に関する情報 > メモ」に貼る文。審査には英語の版を出し、日本語の版は中身の確認用。

- 前提の版: 審査用の版(`Metamon-Review` / `APP_STORE_REVIEW`、#245 / PR #284、`docs/ios/review-build.md`)。最初の版は **App 内課金なし**(#450)・**共有 / 公開のデータなし**(#451)で出す(2026-10-30 決定)。外した機能をこのメモで説明しない・書いた機能は必ずバイナリにある、の両方を守る(2.5.19・2.3.1)。
- 下書きの元: リポジトリ外の調査メモ `調査_ライセンスとApp Store審査.md` の「審査メモの下書き」。そこから、今のコードと合わない所を直した(下の「下書きから直した所」)。
- ストアの掲載文は [listing.md](listing.md)、避ける言い回しは [wording.md](wording.md)。

## 目次

1. [出す前に確かめること](#1-出す前に確かめること)
2. [英語の版(App Store Connect に貼る)](#2-英語の版app-store-connect-に貼る)
3. [日本語の版(確認用)](#3-日本語の版確認用)
4. [下書きから直した所](#4-下書きから直した所)
5. [承認された前例との違いと共通点](#5-承認された前例との違いと共通点)
6. [審査で聞かれたときの答え方](#6-審査で聞かれたときの答え方)

## 1. 出す前に確かめること

| 項目 | 確かめ方 | メモでの扱い |
|---|---|---|
| `webView` 部品を審査用の版から外したか | `ClientCapabilities.reviewExcludedComponents` に `.webView` がある(#452 G3 で外した) | 英語の版のまま(「WebView を使わない」と書ける)。2 章の [WEB PAGES] の段落は使わない |
| 外した機能がバイナリに無い | `docs/ios/review-build.md` の「外れたことの確かめ方」 | 書かない(リリース申請・探す・共有リンク・HealthKit・道のりの記録・近くの端末・撮影) |
| 1 日の上限 | 審査用の版は端末で数えない(`ProGate` ごと外した・#450)。上限はサーバーの `ratelimit` だけで、当たると「今日はここまでです / 今日の上限に達しました。〜からまた使えます」の状態カード(課金画面は出ない) | 数は書かない(`daily limit on AI requests`)。数を変えてもメモを直さずに済む |
| 削除の後の消去 | `docs/legal/privacy-policy.md` の 4 章の「[要確認: 物理削除までの期間]」が埋まっていること | 「Privacy Policy のとおり消去」と書くので、ポリシー側が先 |
| 審査用アカウント | App Store Connect の「サインイン情報」欄に入れる(このファイルやリポジトリには書かない) | `<demo account>` のまま貼らない |
| バックエンド | 審査の期間中、ai / appconfig が動いていて、Claude の利用上限に余裕があること | — |
| 連絡先 | 名前・メール・電話 | 公開リポジトリには書かない。貼る時に埋める |
| 共有・公開のデータ(`scope: shared / public`・`chatThread`)を審査用の版で作らせないか | `ClientCapabilities.reviewDisabledFeatures` に `sharedData`、`reviewExcludedComponents` に `.chatThread` がある(#451 G2)。backend の `restrict.go` が scope を private に直す。同梱の例 `chat` / `board` / `ranking` / `post_comments` / `recipes` も外した | 「記録はすべて本人だけ」「他の人とのデータの共有・チャットは無い」と書ける(2 章 HOW IT WORKS の最後の箇条) |
| aiButton のオンライン版(`POST /v1/text`)を審査用の版で止めたか | `supportsOnlineAIText` が審査用の版で false(#452 G4) | PRIVACY の「記録の中身は送らない」と合う |
| 課金(Pro) | **入れない**(#450・2026-10-30 決定)。審査用の版から課金画面・設定の「Tailor It Pro」・有料機能の確認カード・StoreKit(商品 ID)を外した。App Store Connect にもサブスクリプションを登録しない | IN-APP PURCHASE の段落は無い。「No in-app purchases」と書く |
| MusicKit | App ID の App Services で MusicKit が有効か | 無いと musicPlayer の検索・再生が失敗する(2.1) |

注: `webView` は AI が決めた https の URL を WKWebView で開く部品(`WebViewLeaf.swift`)。審査用の版からは外した(#452 G3。サイトは `openUrlButton` で Safari に開く)。通常の版に残す理由と、戻す時の差し替えは 2 章の [WEB PAGES]。

## 2. 英語の版(App Store Connect に貼る)

```text
Thank you for reviewing Tailor It.

WHAT THE APP DOES
Tailor It gives each user a personal tracker or tool, set up by voice or text.
The user says what they want to keep track of (for example "a water intake log
with a daily goal of 2 liters" or "a shopping list grouped by store"), and Tailor It
shows a matching screen with input fields, lists, totals, charts and reminders.
The user can then adjust it by talking ("make the button bigger", "add a weekly total")
and go back to any earlier version.

HOW IT WORKS - NO DOWNLOADED CODE (Guideline 2.5.2)
- Every screen element is a native SwiftUI component compiled into this binary
  (lists, forms, charts, buttons, calendars, maps and so on).
- Our server asks an AI model (Anthropic Claude) to return a JSON configuration.
  The JSON only selects which built-in components to show, their labels and layout,
  and which fields the user's records have.
- Totals and similar values (sums, averages, days between dates, streaks) are
  computed by a fixed, built-in evaluator with a closed list of arithmetic and date
  operations. There are no loops, user-defined functions, network or file access.
- The app never downloads or runs code, scripts, JavaScript, WebViews or plug-ins.
  The server checks every JSON against our schema. In the app, a component type the
  binary does not know is shown as an empty placeholder and does nothing, so new
  component types can only arrive through an App Store update.
- A user's trackers are personal content inside Tailor It (Guideline 1.2.1).
  They are not separate apps: a tracker cannot be installed, exported or run outside
  Tailor It, and nothing is submitted to the App Store. (Users can export their own
  records as a CSV or PDF file through the system share sheet.)
- Every tracker and every record is private to the user who made it. This version
  has no public catalog, no store of other users' content, no share links, no data
  shared between users and no chat between users.

DEVICE FEATURES
Tailor It itself requests every permission, at the moment the user uses the feature:
- Microphone: voice input. Speech is transcribed on the device (WhisperKit).
- Photos: attaching a picture from the photo library (system photo picker).
- Camera: scanning a QR code into a field.
- Location (while using the app only): saving the current place once, shown on a map,
  and walking/driving directions to a destination while the screen is open
  (no background location).
- Apple Music: searching and playing songs in a music player the user added
  (MusicKit; playback needs the user's own Apple Music subscription).
- Notifications: local reminders the user asks for (for example every morning at 8:00).
- Live Activities: a timer or one number from the tracker on the Lock Screen,
  only after the user starts it. Updated on the device, no push.
- On-device processing: reading text or amounts from a photo the user picked
  (Vision), and an optional "write with AI" button that uses Apple's on-device
  model (Foundation Models). Neither sends the photo or the records anywhere.

PRIVACY
- Before the first AI request we show a consent screen. It explains that what the
  user says or types, the chat, and the tracker's layout are sent to Anthropic's AI
  through our server to set up the screen (Guideline 5.1.2(i)). The contents of the
  user's records are not sent. Without consent the user can still view and use
  existing trackers; only setting up and changing them by AI is unavailable.
  Consent can be withdrawn in Settings > Privacy.
- The speech model (about 500 MB) is downloaded only after the user confirms.
  Text input works without it.
- Account deletion: Settings (gear icon) > Delete Account. It deletes the account,
  makes all of the user's trackers and records unavailable at once (then erases them
  as described in the Privacy Policy), clears the data on the device, and revokes the
  Sign in with Apple token.
- Privacy Policy and Terms of Use: Settings > Privacy, and in the App Store metadata.

NO IN-APP PURCHASES
- This version is free and has no in-app purchases or subscriptions.
- To keep the AI cost on our server under control, each account has a daily limit
  on AI requests. When it is reached, the app says that the limit for today has been
  reached and when it can be used again (the next day). Existing trackers and records
  can still be viewed and used.

HOW TO TEST (about 2 minutes)
1. Sign in with the demo account in the sign-in information, or tap
   "Use without signing in (anonymous)".
2. Accept the AI consent screen.
3. Tap the mic or type: "Make a water intake log with a daily goal of 2 liters."
4. Add a few entries, then say or type: "Show a weekly chart."
5. Settings shows Privacy and Delete Account.

Our backend runs for the whole review period.
Contact: <name>, <email>, <phone>
```

### [WEB PAGES] `webView` を外さない場合の差し替え

`HOW IT WORKS` の 4 つ目の箇条を次に替える(「WebView を使わない」とは書かない)。

```text
- The app never downloads or runs code, scripts or plug-ins. The server checks every
  JSON against our schema. In the app, a component type the binary does not know is
  shown as an empty placeholder and does nothing, so new component types can only
  arrive through an App Store update.
- One component can show a single https web page inside the tracker, the same way a
  browser would. The page has no bridge to the app: no injected scripts, no message
  handlers, and no access to the user's records or device features.
```

## 3. 日本語の版(確認用)

```text
Tailor It の審査をありがとうございます。

このアプリでできること
声か文字で、自分専用の記録帳・ツールを用意できます。記録したいこと(例:「1 日 2 リットルを
目標に水分を記録したい」「お店ごとに分けた買い物リスト」)を話すと、入力欄・一覧・合計・
グラフ・リマインダーをそろえた画面が表示されます。「ボタンを大きく」「週の合計も」と話して
直せて、前の形にもいつでも戻せます。

仕組み(コードをダウンロードしない・2.5.2)
- 画面の部品は、すべてこのバイナリに入ったネイティブの SwiftUI 部品です。
- サーバーが AI(Anthropic の Claude)に JSON の設定を作らせます。JSON は、どの部品を出すか・
  文言・配置・記録の項目を選ぶだけです。
- 合計などの値(合計・平均・日付の差・連続日数)は、アプリに入った決まった計算の仕組みが、
  決まった四則演算と日付の操作だけで計算します。繰り返し・独自の関数・通信・ファイルの読み書きは
  ありません。
- コード・スクリプト・JavaScript・WebView・プラグインをダウンロードも実行もしません。
  サーバーはすべての JSON をスキーマで確かめます。アプリは、バイナリが知らない部品を空の枠として
  出すだけで何もしないので、新しい部品はアプリの更新でしか増えません。
- 記録帳は Tailor It の中の個人のコンテンツです(1.2.1)。別のアプリではなく、記録帳を
  Tailor It の外でインストール・書き出し・実行はできず、App Store に何かを出すこともありません
  (自分の記録は、共有シートから CSV / PDF のファイルにできます)。
- 記録帳と記録は、すべて作った本人だけのものです。この版には、公開の一覧・他人の作ったものの店・
  共有リンク・他のユーザーとのデータの共有・ユーザー同士のチャットはありません。

端末の機能
権限はすべて Tailor It 本体が、その機能を使う時に求めます。
- マイク: 声の入力。書き起こしは端末の中(WhisperKit)。
- 写真: 写真ライブラリから選んで添える(システムの写真ピッカー)。
- カメラ: QR コードを読んで項目に入れる。
- 位置(使用中のみ): 今いる場所を 1 回だけ保存し、地図で見る。画面を出している間だけ、
  目的地までの道案内(背面の位置は使わない)。
- Apple Music: ユーザーが置いた音楽プレーヤーで曲を探して再生する(MusicKit。再生には
  本人の Apple Music の契約が要る)。
- 通知: ユーザーが頼んだリマインダー(例: 毎朝 8 時)を端末で出す。
- ライブアクティビティ: ユーザーが始めた時だけ、タイマーか記録帳の数字 1 つをロック画面に出す。
  更新は端末の中だけ(push なし)。
- 端末の中での処理: 選んだ写真から文字や金額を読む(Vision)、任意の「AI で書く」ボタン
  (Apple の端末内モデル Foundation Models)。どちらも写真や記録を外に送らない。

プライバシー
- 最初に AI を使う前に同意画面を出します。話した・入力した内容、会話、記録帳の設計を、
  画面を用意するためにサーバー経由で Anthropic の AI に送ると説明します(5.1.2(i))。
  記録の中身は送りません。同意しなくても、記録帳の閲覧と記録はできます(AI での用意と変更だけ
  使えません)。同意は 設定 > プライバシー で取り消せます。
- 音声モデル(約 500MB)は確認してからダウンロードします。文字の入力ならダウンロード不要です。
- アカウントの削除: 設定(歯車)> アカウントを削除。アカウントを削除し、すべての記録帳と記録を
  すぐに使えなくして(その後プライバシーポリシーのとおり消去)、端末のデータを消し、
  Sign in with Apple のトークンを失効させます。
- プライバシーポリシーと利用規約: 設定 > プライバシー と、App Store のメタデータ。

App 内課金なし
- この版は無料で、App 内課金・サブスクリプションはありません。
- サーバーでの AI の費用を抑えるため、アカウントごとに AI への依頼の 1 日の上限があります。
  上限に達すると「今日の上限に達しました」と、また使える時刻(翌日)を表示します。
  すでにある記録帳と記録は、そのまま見て使えます。

試し方(約 2 分)
1. サインイン情報のデモアカウントでサインインするか、「ログインせずに使う(匿名)」を押す。
2. AI の同意画面で同意する。
3. マイクを押すか入力する:「1 日 2 リットルを目標に水分を記録したい」
4. 何件か記録してから、話すか入力する:「週のグラフも見たい」
5. 設定に、プライバシー・アカウントを削除 がある。

審査の期間中、バックエンドは動かしておきます。
連絡先: <名前>・<メール>・<電話>
```

## 4. 下書きから直した所

| 下書き(調査メモ) | 直した後 | 理由 |
|---|---|---|
| 「request text and the tool's content are sent」 | 話した・入力した内容、会話、記録帳の設計。**記録の中身は送らない** | 同意画面(`AIConsentView.swift`)の説明に合わせた。食い違うと 5.1.2(i) で問われる |
| 「Free users can create N new tools per day」 | NO IN-APP PURCHASES の段落(数は書かない) | 最初の版は課金なし(#450)。上限はサーバーの `ratelimit` だけで、審査用の版に端末の無料枠は無い |
| 「Settings > Account > Delete Account」「Settings > About」 | 設定(歯車)> アカウントを削除、設定 > プライバシー | 今の設定画面の並び(`SettingsSheet.swift`) |
| 「tap "Try without account"」 | 「Use without signing in (anonymous)」 | 実際のボタンの文言 |
| 権限の説明なし | DEVICE FEATURES を足した(10/30: 道案内・Apple Music・ライブアクティビティ・端末内の処理を追加。#411 #412 #418 #423 #417) | 審査用の版に残るカメラ(QR)・位置(1 回)・写真・通知を、本体の機能として説明する(4.7.2・4.7.3 の論点) |
| 計算の説明なし | 決まった計算の仕組み(閉じた演算の一覧)を書いた | DSL に式(`computed` / `stat.value`)がある。「データだけ」と書いて式を伏せると、後で 2.3.1 の「誤解させる説明」になりうる |
| 「never ... WebViews」 | そのまま(審査用の版から `webView` を外した・#452 G3)。通常の版で戻す時の差し替えを用意 | #284 の時点では `webView` 部品が残っていた |
| 「no store of other users' content」だけ | 記録はすべて本人だけ・共有 / チャットが無いことまで書く | 審査用の版で共有・公開のデータと `chatThread` を外した(#451 G2)。1.2(通報・ブロック)の論点を避ける |
| 「Demo account: reviewer@<domain> / <password>」 | サインイン情報の欄に入れる | 公開リポジトリに書かない |
| 「personal tracker or tool」「create tools」 | 「set up a tracker」 | 「create」は「アプリを作る」と読まれやすい([wording.md](wording.md)) |

## 5. 承認された前例との違いと共通点

出典は調査メモの [P1]〜[P6]。判断のばらつきが大きいので、「これが通ったから通る」とは言えない **(推測)**。

| 前例 | どうなったか | Tailor It との共通点 | Tailor It との違い |
|---|---|---|---|
| Bitrig(端末内の Swift インタプリタ) | やり取りの末に承認 | 自然言語から画面を作る。結果は本体の中だけで動く | Bitrig は**コードを解釈して実行する**。Tailor It は決まった部品を選ぶ JSON だけで、コードは無い(リスクはより低い側) |
| Build Minis(プロンプトから遊べるものを作り、フィードで共有。13+・UGC あり) | 配信中 | AI が本体の中のコンテンツを作る | Build Minis は**公開のフィード**があり UGC の対策(1.2)を備える前提。Tailor It の審査用の版は共有を外したので、1.2 の UGC の論点はより小さい |
| Emergent | Anything が削除された週に更新が承認 | AI でものを作る | 報道の範囲では、Emergent は「App Store に提出」を売りにしていない **(推測)**。Tailor It もリリース申請を外し、「アプリを作る」と言わない |
| (却下)Anything・Replit・Vibecode | 更新の停止・削除 | AI でものを作る | 問題にされたのは「iPhone のアプリビルダー」という売り方、「ワンタップで App Store に提出」、ダウンロードしたコードで機能が変わること。Tailor It は 3 つとも持たない(売り方は [listing.md](listing.md)、リリース申請は #284 で外す、コードは無い) |

まとめ: 通った例の共通点は「結果が本体の中にとどまる」「App Store への提出をうたわない」。Tailor It はこの 2 つに加え、「コードを実行しない」「共有が無い」ので、却下の理由に当たる所を減らしている。残る論点は 4.7.2(端末の機能)で、本体が権限を求める作りと 1.2.1 の説明で答える。

## 6. 審査で聞かれたときの答え方

| 聞かれそうなこと | 答えの要点 |
|---|---|
| 「ダウンロードしたコードで機能が変わっている」(2.5.2) | JSON は部品の選択と設定だけ。部品・計算の種類はバイナリに固定で、増えるのは更新(審査)の時だけ。サーバーはスキーマで確かめ、アプリは知らない部品を空の枠で出すだけ。JSON の例を添付できる |
| 「アプリを作るアプリではないか」(4.2.6・4.7) | 作ったものは Tailor It の外では動かず、書き出しも提出もできない。ユーザー自身の記録帳(1.2.1 のコンテンツ) |
| 「端末の機能を作ったものに渡している」(4.7.2) | 権限は Tailor It 本体が、本体の UI で求める。部品はネイティブで、JSON は「どこに置くか」を決めるだけ。HealthKit・バックグラウンドの位置・近くの端末・撮影は、この版から外した |
| 「課金は無いのか / 上限は何か」(3.1) | この版は App 内課金なし(バイナリに StoreKit の商品も課金画面も無い)。上限はサーバーの費用を抑えるためのもので、当たると翌日まで待つ旨を表示する |
| 「隠した機能がある」(2.5.19) | 審査用の版はバイナリから外していて、サーバーのフラグで後からオンにする機能は無い。戻すときは新しい版として審査に出す |
| 「AI にどんなデータを送るか」(5.1.2(i)) | 同意画面の文言どおり。記録の中身は送らない。同意の取り消しは 設定 > プライバシー |
