# App Review への説明文(審査メモ)

#247。App Store Connect の「App Review に関する情報 > メモ」に貼る文。審査には英語の版を出し、日本語の版は中身の確認用。

- 前提の版: 審査用の版(`Metamon-Review` / `APP_STORE_REVIEW`、#245 / PR #284、`docs/ios/review-build.md`)。最初の版は **App 内課金なし**(#450)・**共有 / 公開のデータなし**(#451)で出す(2026-10-30 決定)。外した機能をこのメモで説明しない・書いた機能は必ずバイナリにある、の両方を守る(2.5.19・2.3.1)。
- 下書きの元: リポジトリ外の調査メモ `調査_ライセンスとApp Store審査.md` の「審査メモの下書き」。そこから、今のコードと合わない所を直した(下の「下書きから直した所」)。
- ストアの掲載文は [listing.md](listing.md)、避ける言い回しは [wording.md](wording.md)。
- 提出前の点検と、却下された時の返信の下書きは [../release/app-review-playbook.md](../release/app-review-playbook.md)(#332)。

## 目次

1. [出す前に確かめること](#1-出す前に確かめること)
2. [英語の版(App Store Connect に貼る)](#2-英語の版app-store-connect-に貼る)
3. [日本語の版(確認用)](#3-日本語の版確認用)
4. [下書きから直した所](#4-下書きから直した所)
5. [承認された前例との違いと共通点](#5-承認された前例との違いと共通点)
6. [審査で聞かれたときの答え方](#6-審査で聞かれたときの答え方)
7. [M4: サーバーの自動の動きを含む版(下書き)](#7-m4-サーバーの自動の動きを含む版下書き)

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

**ASC の上限は 4,000 字。超えたら入らない。** 下のブロックが実際に App Store Connect に入れた文(2026-10-31 時点・3,994 字。Python の `len()`・改行も 1 字)。直したら字数を数え直し、4,000 字を超えないこと。元の長い版(4,808 字)から、挨拶と連絡先の行・行の折り返し・補足の文(サーバーのスキーマ確認、音声モデルのダウンロード、同意しない場合、AI の費用の理由、デモアカウント、例の時刻など)を削った。連絡先は ASC の連絡先欄に入れる。

```text
WHAT THE APP DOES
Tailor It gives each user a personal tracker or tool, set up by voice or text.
The user says what they want to keep track of (for example "a water intake log with a daily goal of 2 liters" or "a shopping list grouped by store"), and Tailor It shows a matching screen with input fields, lists, totals, charts and reminders.
The user can then adjust it by talking ("make the button bigger", "add a weekly total") and go back to any earlier version.

HOW IT WORKS - NO DOWNLOADED CODE (Guideline 2.5.2)
- Every screen element is a native SwiftUI component compiled into this binary.
- Our server asks an AI model (Anthropic Claude) to return a JSON configuration. The JSON only selects which built-in components to show, their labels and layout, and which fields the user's records have.
- Totals and similar values (sums, averages, days between dates, streaks) are computed by a fixed, built-in evaluator with a closed list of arithmetic and date operations. There are no loops, user-defined functions, network or file access.
- The app never downloads or runs code, scripts, JavaScript, WebViews or plug-ins. In the app, a component type the binary does not know is shown as an empty placeholder and does nothing, so new component types can only arrive through an App Store update.
- A user's trackers are personal content inside Tailor It (Guideline 1.2.1). They are not separate apps: a tracker cannot be installed, exported or run outside Tailor It. (Users can export their own records as a CSV or PDF file through the system share sheet.)
- Every tracker and every record is private to the user who made it. This version has no public catalog, no store of other users' content, no share links, no data shared between users and no chat between users.

DEVICE FEATURES
Tailor It itself requests every permission, at the moment the user uses the feature:
- Microphone: voice input. Speech is transcribed on the device (WhisperKit).
- Photos: attaching a picture from the photo library.
- Camera: scanning a QR code into a field.
- Location (while using the app only): saving the current place once, shown on a map, and walking/driving directions to a destination while the screen is open (no background location).
- Apple Music: searching and playing songs in a music player the user added (MusicKit; playback needs the user's own Apple Music subscription).
- Notifications: local reminders the user asks for.
- Live Activities: a timer or one number from the tracker on the Lock Screen, only after the user starts it. Updated on the device, no push.
- On-device processing: reading text or amounts from a photo the user picked (Vision), and an optional "write with AI" button that uses Apple's on-device model (Foundation Models). Neither sends the photo or the records anywhere.

PRIVACY
- Before the first AI request we show a consent screen. It explains that what the user says or types, the chat, and the tracker's layout are sent to Anthropic's AI through our server to set up the screen (Guideline 5.1.2(i)). The contents of the user's records are not sent. Consent can be withdrawn in Settings > Privacy.
- Account deletion: Settings (gear icon) > Delete Account. It deletes the account, makes all of the user's trackers and records unavailable at once (then erases them as described in the Privacy Policy), clears the data on the device, and revokes the Sign in with Apple token.

NO IN-APP PURCHASES
- This version is free and has no in-app purchases or subscriptions.
- Each account has a daily limit on AI requests. When it is reached, the app says that the limit for today has been reached and when it can be used again (the next day).

HOW TO TEST
1. Tap "Use without signing in (anonymous)".
2. Accept the AI consent screen.
3. Tap the mic or type: "Make a water intake log with a daily goal of 2 liters."
4. Add a few entries, then say or type: "Show a weekly chart."
5. Settings shows Privacy and Delete Account.

Our backend runs for the whole review period.
```

### [WEB PAGES] `webView` を外さない場合の差し替え

`HOW IT WORKS` の 4 つ目の箇条を次に替える(「WebView を使わない」とは書かない)。

```text
- The app never downloads or runs code, scripts or plug-ins. In the app, a component type the binary does not know is shown as an empty placeholder and does nothing, so new component types can only arrive through an App Store update.
- One component can show a single https web page inside the tracker, the same way a browser would. The page has no bridge to the app: no injected scripts, no message handlers, and no access to the user's records or device features.
```

## 3. 日本語の版(確認用)

2 章の英語の版(実際に入れた文)と同じ中身に揃えている。

```text
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
  アプリは、バイナリが知らない部品を空の枠として出すだけで何もしないので、新しい部品は
  アプリの更新でしか増えません。
- 記録帳は Tailor It の中の個人のコンテンツです(1.2.1)。別のアプリではなく、記録帳を
  Tailor It の外でインストール・書き出し・実行はできません
  (自分の記録は、共有シートから CSV / PDF のファイルにできます)。
- 記録帳と記録は、すべて作った本人だけのものです。この版には、公開の一覧・他人の作ったものの店・
  共有リンク・他のユーザーとのデータの共有・ユーザー同士のチャットはありません。

端末の機能
権限はすべて Tailor It 本体が、その機能を使う時に求めます。
- マイク: 声の入力。書き起こしは端末の中(WhisperKit)。
- 写真: 写真ライブラリから選んで添える。
- カメラ: QR コードを読んで項目に入れる。
- 位置(使用中のみ): 今いる場所を 1 回だけ保存し、地図で見る。画面を出している間だけ、
  目的地までの道案内(背面の位置は使わない)。
- Apple Music: ユーザーが置いた音楽プレーヤーで曲を探して再生する(MusicKit。再生には
  本人の Apple Music の契約が要る)。
- 通知: ユーザーが頼んだリマインダーを端末で出す。
- ライブアクティビティ: ユーザーが始めた時だけ、タイマーか記録帳の数字 1 つをロック画面に出す。
  更新は端末の中だけ(push なし)。
- 端末の中での処理: 選んだ写真から文字や金額を読む(Vision)、任意の「AI で書く」ボタン
  (Apple の端末内モデル Foundation Models)。どちらも写真や記録を外に送らない。

プライバシー
- 最初に AI を使う前に同意画面を出します。話した・入力した内容、会話、記録帳の設計を、
  画面を用意するためにサーバー経由で Anthropic の AI に送ると説明します(5.1.2(i))。
  記録の中身は送りません。同意は 設定 > プライバシー で取り消せます。
- アカウントの削除: 設定(歯車)> アカウントを削除。アカウントを削除し、すべての記録帳と記録を
  すぐに使えなくして(その後プライバシーポリシーのとおり消去)、端末のデータを消し、
  Sign in with Apple のトークンを失効させます。

App 内課金なし
- この版は無料で、App 内課金・サブスクリプションはありません。
- アカウントごとに AI への依頼の 1 日の上限があります。上限に達すると「今日の上限に達しました」と、
  また使える時刻(翌日)を表示します。

試し方
1. 「ログインせずに使う(匿名)」を押す。
2. AI の同意画面で同意する。
3. マイクを押すか入力する:「1 日 2 リットルを目標に水分を記録したい」
4. 何件か記録してから、話すか入力する:「週のグラフも見たい」
5. 設定に、プライバシー・アカウントを削除 がある。

審査の期間中、バックエンドは動かしておきます。
```

## 4. 下書きから直した所

| 下書き(調査メモ) | 直した後 | 理由 |
|---|---|---|
| 「request text and the tool's content are sent」 | 話した・入力した内容、会話、記録帳の設計。**記録の中身は送らない** | 同意画面(`AIConsentView.swift`)の説明に合わせた。食い違うと 5.1.2(i) で問われる。審査用の版は「AI で書く」のオンライン版を外す(#455)ので言い切れる。同意画面は両方の版で同じ文言で、「『AI で書く』がサーバーの AI を使う場合は、押した時だけ最近の記録を送る」と条件つきで書く(通常の版向け・#459) |
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

## 7. M4: サーバーの自動の動きを含む版(下書き)

#349。**serverRules(サーバーの自動の動き)を含む版のための下書き。今の審査用の版には serverRules が無いので、2 章の文には足さない。**(今の審査用の版では AI が serverRules を作らず、iOS の `Features/Automations` と `Features/Push` も外している・#316 #614 #269・`docs/ios/review-build.md` の 46・47 行)

根拠は 2026-11-09 の `origin/develop`(`e988c309`)の `ファイル:行`。コードが変わったら 7.4 の表を確かめ直す。

### 7.1 何が端末に届き、何が届かないか

| 届くもの | 届かないもの |
|---|---|
| 記録帳の JSON(DSL)。serverRules もその中のデータとして入る | コード・スクリプト・式の実行環境。ルールを実行する仕組みはサーバーにだけある |
| ルールが送った通知(APNs) | 新しい部品・新しい処理の種類。どちらもバイナリ(部品)とサーバーのコード(処理)に固定で、増えるのは更新の時だけ |
| 「自動の動き」の一覧と、ルールごとのオン / オフの切り替え(通常の版だけ・#347) | — |

iOS は serverRules の中身を解釈しない。数を数えるだけ(設定に「自動の動き」を出すかの判断)で、オン / オフは DSL の `enabled` を書き換える patch を送るだけ。

### 7.2 使う前に確かめること

| 項目 | 確かめ方 | 理由 |
|---|---|---|
| その版のバイナリに serverRules の機能がある | `APP_STORE_REVIEW` で `Features/Automations` と `Features/Push` を外す `#if` を外した版か | 書いた機能は必ずバイナリにある(2.5.19・2.3.1) |
| 課金(Pro)が入っている | serverRules は Pro の機能(`paid.ServerRules`・#316)。課金を入れた版なら 2 章の NO IN-APP PURCHASES の段落も差し替える | 課金なしの版では serverRules が作られない |
| `PRO_GATE_MODE` が `enforce` | ai・appconfig・runner の 3 つ(`docs/infra/runtime-config.md`) | 審査用の版に serverRules が無いと言えるのは、AI が作らないことと、Pro でない人の保存が 402 になることの両方による |
| プッシュの entitlement | 通常の版の `ios/Metamon/Metamon.entitlements` には `aps-environment` がある(#683。`METAMON_APS_ENV` で Debug / Staging は development、Release は production)。審査用の版は `Features/Push`(PushHost)を外し、`Metamon-Review.entitlements` にも `aps-environment` が無い。この章を使う版では両方を戻す(`docs/ios/review-build.md`) | 無いと APNs のトークンが取れず、通知が届かない(`PushHost.swift` の 61 行のコメント) |
| 通知の許可の求め方 | 本体が、本体の UI で、使う時に OS の許可を求めるか | 4.5.4(必須にしない)・4.7.3 |
| 共有のデータ(`sharedData`)が入っているか | `ClientCapabilities.reviewDisabledFeatures` に `sharedData` が残っていれば共有は無い | 共有が無い版では英語の版の「Other people receive them only if …」の文を消す(外した機能を書かない) |
| `saveFeed` を使うか | その版で天気の記録を残す作りを出すか | 出さないなら英語の版の「save today's public weather forecast」と「The only outside data …」を消す |
| runner が動いている | 審査の期間中、runner がデプロイされて動いていること(`docs/infra/runner.md`) | 審査で自動の動きを試されたときに届かないと 2.1 |
| ASC の 4,000 字 | 7.3 の段落は 1,631 字(Python の `len()`)。2 章の文は 3,994 字なので、そのまま足すと超える。足す時は 2 章のほかの所を削り、合計を数え直す | ASC に入らない |

### 7.3 英語の版(2 章の HOW IT WORKS の後に足す)

```text
SERVER AUTOMATIONS - NO DOWNLOADED CODE (Guidelines 2.5.2, 4.7, 4.5.4)
- A tracker can have simple automatic actions, such as a reminder at a set time or a count that goes up when a record is added. They are declarative rules stored as data in the tracker's JSON configuration: a trigger (a record is added or changed, or a daily, weekly or monthly time), an optional condition over the tracker's own records, and up to five actions from a closed list (send a notification, add or update a record, add to a number, save today's public weather forecast as a record).
- The rules run on our servers, not on the device. The device receives only the JSON configuration (data) and the notifications. The app does not run the rules, and it never downloads or runs code. The kinds of actions are fixed in our server's code; a rule can only choose from them and cannot add new ones.
- Our server checks every rule against a fixed schema before it is saved, and checks again who may receive each notification when it runs. Rules cannot send email or messages to other services, open web pages, or notify arbitrary people. The only outside data is the public weather forecast for a fixed region, which our server reads from a fixed list of sources.
- Notifications go to the tracker's owner. Other people receive them only if they take part in a shared tracker and have turned on notifications for that tracker. Notification text cannot contain links or claim to be official. We do not use notifications for advertising or marketing.
- Our operators can stop all automatic actions, all notifications, or a single tracker or rule at any time.
```

### 7.4 日本語の版(確認用)

```text
サーバーの自動の動き(コードをダウンロードしない・2.5.2 / 4.7 / 4.5.4)
- 記録帳に、決まった時刻のリマインダーや、記録を足すと増える数などの簡単な自動の動きを
  付けられます。これは記録帳の JSON の設定にデータとして書かれた宣言的なルールです。きっかけ
  (記録が足された・変わった、毎日・毎週・毎月の時刻)、その記録帳の記録についての任意の条件、
  決まった一覧から選ぶ 5 つまでの処理(通知を送る・記録を足す / 直す・数を足す・今日の公開の
  天気予報を記録に残す)だけでできています。
- ルールは端末ではなく、私たちのサーバーで動きます。端末に届くのは JSON の設定(データ)と
  通知だけです。アプリはルールを実行せず、コードをダウンロードも実行もしません。処理の種類は
  サーバーのコードに固定で、ルールはその中から選ぶだけで、新しい処理は足せません。
- サーバーは、保存の前にすべてのルールを決まったスキーマで確かめ、動かす時にも通知を受け取って
  よい人かを確かめ直します。ルールから、メールや他のサービスへの送信・Web ページを開くこと・
  任意の人への通知はできません。外のデータは、決まった地域の公開の天気予報だけで、サーバーが
  決まった取得先の一覧から読みます。
- 通知は記録帳の作成者に届きます。ほかの人に届くのは、共有の記録帳に参加して、その記録帳の
  通知を有効にした場合だけです。通知の文にはリンクを入れられず、公式を名乗れません。
  通知を広告や宣伝には使いません。
- 運営は、すべての自動の動き・すべての通知・1 つの記録帳・1 つのルールを、いつでも止められます。
```

### 7.5 文とコードの対応

| 文 | 根拠(`origin/develop` `e988c309`) |
|---|---|
| ルールは JSON の中のデータ。きっかけ・条件・処理の形 | `backend/pkg/dsl/server_rules.go:24-40`(`ServerRule`)・`:44-55`(`RuleTrigger`。`ruleEvery` = daily / weekly / monthly は `:140`)・`:58-65`(`RuleAction` は notify / createRecord / updateRecord / increment / saveFeed の 5 つの欄) |
| 1 つの処理は決まった一覧の 1 つだけ | `server_rules.go:378-389`(5 つのうち 1 つでなければ却下) |
| 5 つまでの処理 | `server_rules.go:128`(`MaxRuleActionsPro = 5`)・`:221-223`(超えたら却下)。無料の枠は 3(`:127`・`ServerRuleQuotaProblems` `:716`) |
| 条件は記録帳の記録についての式 | `server_rules.go:208-214`(`if` は真偽の式だけ)。式は `backend/pkg/dsl/expreval/ops.go:23-338` の決まった演算(四則・比較・日付)だけを `switch` で計算する。外部のスクリプトの実行環境(JS など)は `backend/go.mod` に無い |
| 天気予報を記録に残す・外のデータは決まった地域と取得先だけ | `backend/pkg/dsl/server_rules_feed.go:9-24`(地域は決まったコードで、記録の値は使わない)・`:49`(weather / pressure / warnings だけ)。取得は `backend/internal/runner/feed.go:54` から `internal/feeds` を通る(`backend/internal/feeds/client.go:135-147` の許可した取得先だけ・転送先も同じ) |
| サーバーで動く・端末はルールを実行しない | 実行は `backend/internal/runner/engine/plan.go:358-412`(処理ごとの `switch`)。iOS は serverRules を数えるだけ(`ios/Packages/MetamonDSL/Sources/MetamonDSL/Model/AppDocument.swift:49-50`・`:114`)。数は設定の表示に使うだけ(`ios/Metamon/Features/Automations/AutomationsView.swift:13`) |
| 保存の前にスキーマで確かめる | `backend/pkg/dsl/validate.go:69-82`(JSON Schema → 知らない欄を拒む → `Validate`)・`:273`(`checkServerRules`)。保存の入口: `backend/internal/appconfig/apps.go:76`・`backend/internal/appconfig/dsl.go:108`(全体)・`:177`(patch。`backend/pkg/dsl/patch.go:62` で `Parse`)・`:285`(前の版に戻す) |
| Pro でない人は保存できない | `backend/internal/appconfig/pro_gate.go:66-86`(`PRO_GATE_MODE=enforce` の時に 402)・`backend/internal/paid/paid.go:147` |
| 審査用の版では AI が serverRules を作らない | `backend/internal/ai/server_push_wording.go:50-52`(`sharedData` を無効にした依頼では pass を飛ばす)・`backend/internal/ai/server_rules_pass.go:313-318`。iOS の審査用の版は `sharedData` を無効にして送る(`ios/Packages/MetamonDSL/Sources/MetamonDSL/Model/ClientCapabilities.swift:20`・`:56-61`) |
| 動かす時に、受け取ってよい人かを確かめ直す | `backend/internal/runner/notify/recipients.go:194`(宛先の種類をデータの範囲で確かめ直す・`:127-133`)・`:302-351`(退会・参加者・ブロック・新しいアカウント・同意)。宛先付きの規則も `backend/internal/runner/exec.go:124`・`:195-197` で確かめ直す |
| 任意の宛先・外部への送信が無い | 宛先の種類は owner / author / authorOf / members / recipients だけ(`server_rules.go:79`・`:540-547`)。データの範囲ごとの表は `:524-528`(自分だけの記録帳は owner だけ)。外部へ送る処理は `RuleAction`(`:58-65`)に無い(#339) |
| 作成者以外は同意した人だけ | `recipients.go:342-350`(作成者以外は `Consent.Allows` が true の人だけ)・`:81-87`(既定は誰も同意していない)。iOS の同意の画面は通常の版の `ios/Metamon/Features/Push/PushConsentSection.swift`(`PUT /v1/me/notify-consent/{appId}`) |
| リンクを入れられない・公式を名乗れない | 保存の時: `server_rules.go:142`・`:605`(URL)・`:145`・`:622`(公式・運営・Tailor It・Apple)。動かす時: 差し込む値の URL・メール・電話番号を伏せる(`backend/internal/runner/engine/render.go:30`・`:104-120`) |
| 広告や宣伝に使わない | APNs を呼ぶのは runner だけ(`backend/cmd/runner/push.go:38-57` 以外に `apns.` の呼び出しは無い)で、運営が一斉に送る仕組みは無い。規約が作成者の宣伝目的の通知を禁じる(`docs/legal/terms.md:46`) |
| 運営が止められる | `RUNNER_PAUSED`(全部)・`RUNNER_NOTIFY_PAUSED`(通知だけ): `backend/cmd/runner/main.go:14-15`・`:130-131`、`backend/internal/runner/exec.go:50-53`。アプリとルールの停止: `backend/internal/runner/stops.go:12-20`(#346) |
| (2 章)知らない部品は描かない | `ios/Packages/MetamonDSL/Sources/MetamonDSL/Model/Component.swift:78-80`・`:292`(知らない type は `.unknown`)・`ios/Packages/MetamonRenderer/Sources/MetamonRenderer/Screen/DSLView.swift:37-41`(登録に無い部品は `UnavailableComponent` の枠)・`ios/Packages/MetamonRenderer/Sources/MetamonRenderer/Support/UnavailableComponent.swift:4-22`(「アプリを更新すると表示できます」) |
| (2 章)審査用の版で外した部品・機能 | `ClientCapabilities.swift:17`(`reviewExcludedComponents`)・`:20`(`reviewDisabledFeatures`)・`:28-34`(`APP_STORE_REVIEW` で allowlist を送る)。ビルドの設定は `ios/project.yml:147-150` |

### 7.6 聞かれたときの答え方(足す分)

| 聞かれそうなこと | 答えの要点 |
|---|---|
| 「サーバーの機能でアプリの動きを後から変えている」(2.5.2) | ルールは JSON のデータで、サーバーで動く。処理の種類はサーバーのコードに固定で、端末にコードは届かない。端末は今までと同じ JSON と通知を受け取るだけ。新しい部品は App Store の更新でしか増えない |
| 「作ったものが別のアプリとして振る舞っている」(4.7) | 記録帳と自動の動きは Tailor It の中だけにあり、通知も Tailor It の通知として届く。外で動かす・書き出す方法は無い。記録帳はネイティブの部品の組み合わせで、HTML5 / JavaScript のミニアプリではない(`webView` を戻す版は 2 章の [WEB PAGES] の差し替えを使う) |
| 「プッシュ通知を宣伝に使っていないか・必須にしていないか」(4.5.4) | 通知は利用者が頼んだルールの知らせだけ。ほかの人には記録帳ごとの同意の後だけ。運営からの宣伝の通知は無い。iOS の通知の許可は Tailor It 本体が求める(求める時と、断った時の動きは出す版で確かめる・7.2) |
| 「外部のサービスに送っていないか」(5.1.2) | 外へ出るのは決まった地域の天気予報の取得だけで、記録の値も利用者の情報も送らない |

### 7.7 チームへ: 残る論点(ASC には貼らない)

審査で通るかは Apple の判断で、ここに書いたことは「通る」を保証しない **(推測)**。

| # | 論点 | 今の状態 | 次の一手 |
|---|---|---|---|
| 1 | 4.7 の「ソフト」とみなされるか | サーバーで動くことは 2.5.2 の答えにはなるが、4.7 は配り方(ミニアプリ・プラグイン)の論点で、どこで動くかだけでは答えにならない。答えは「記録帳は本体の部品の組み合わせの個人のコンテンツ」(4.3 の案 A・[playbook](../release/app-review-playbook.md))に寄る | 自動の動きを含む版は、まず共有なし(owner への通知だけ)で出し、4.7 の指摘が無いか見る |
| 2 | 条件・値の式 | 条件や値に式があり、決まった演算だけだがループのない小さな計算はできる。「データだけ」と書いて式を伏せると 2.3.1 の論点になるので、英語の版は「condition」と書いている | 聞かれたら 2 章の「fixed, built-in evaluator」と同じ説明で答える |
| 3 | 動かす時の確かめ | 前の規則で保存した版も動かせるよう、runner は保存の時の確かめをすべてはやり直さない(`exec.go:208-216`)。動かす時に確かめ直すのは宛先・同意・宛先付きの規則・差し込む値。英語の版は「checks again who may receive each notification」までしか言っていない | 「動かす時も同じ制限」とは書かない。言い回しを強める時はコードを先に足す |
| 4 | 通知が届くか | 通常の版には `aps-environment` がある(Release は production・#683)が、審査用の版は PushHost も `aps-environment` も無い(7.2)。runner も未デプロイなら、審査で試されて届かない(2.1) | 出す版で PushHost・entitlement・runner(`RUNNER_APNS_ENVIRONMENT=production`)を確かめてから、この段落を足す |
| 5 | 4,000 字 | 2 章 3,994 字 + 7.3 の 1,631 字で超える | 7.3 を足す版では、2 章の DEVICE FEATURES のうちその版に無い行などを削る。足す前に字数を数える |
| 6 | 審査用の版に serverRules が無いことの根拠 | AI が作らないこと(`server_push_wording.go:50-52`)と、Pro の保存の判定(`pro_gate.go:66-86`)に寄る。`PRO_GATE_MODE` を `log` / `off` にすると後者が外れる | 審査の期間中は `enforce` のままにする(7.2) |
| 7 | 天気予報(`saveFeed`) | 外のデータを読む唯一の処理。取得先は許可の一覧だけ | 天気の記録を出さない版では、英語の版の該当の文を消す |
