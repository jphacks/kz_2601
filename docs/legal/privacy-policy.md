# Tailor It プライバシーポリシー


施行日: 2026年10月30日
改定日: 2026年11月7日
運営者: 高岡己太朗
連絡先: support@tailor-it.app

Tailor It(以下「本アプリ」)は、話した内容や入力した文から、自分専用のアプリ(記録・一覧・集計などの画面)を作って直せる iOS アプリです。本ポリシーは、本アプリが集める情報、使い道、送り先、保存と削除について説明します。

## 1. 集める情報と使い道

### 1.1 アカウントの情報
- **Sign in with Apple** または匿名でサインインします。認証は Google の **Firebase Authentication** が行います。
- Firebase Authentication には、Firebase が発行するユーザー ID(uid)、サインインの方法、作成日時・最終サインイン日時が保存されます。Apple でサインインした場合は、Apple が提供する場合に限り、Apple のユーザー識別子・名前・メールアドレス(Apple の「メールを非公開」を選んだ場合は中継用のアドレス)も保存されます。匿名の場合、名前・メールアドレスは保存しません。
- 運営者のサーバー(Firestore)が使うのはユーザー ID(uid)で、名前・メールアドレスは保存しません。運営者から、このメールアドレスに連絡することはありません。
- 使い道: 本人の確認、作ったアプリと記録を本人だけが使えるようにするため。

### 1.2 話した内容・入力した文(音声)
- **音声は端末の中で文字にします**(WhisperKit。音声認識のモデルは端末に保存します)。**音声そのもの(録音データ)はサーバーに送りません。**
- 音声認識のモデルが端末に無い時だけ(初回など)、サイズをお知らせして確認した上で、**Hugging Face, Inc.(米国)** から HTTPS でダウンロードします。このとき Hugging Face に届くのは、通常の通信に伴う情報(IP アドレスと、取得するファイルのリクエスト)だけで、音声・話した内容・記録・アカウントの情報は送りません。モデルが端末にあれば Hugging Face には接続しません。Hugging Face での扱いは [Hugging Face のプライバシーポリシー](https://huggingface.co/privacy) によります。
- 音声認識のモデルを準備している間(ダウンロード・読み込みの間)は、iPhone に Apple が用意している**端末の中の音声認識**で、仮に文字にします。この時も端末の中だけで処理し、**声を端末の外(Apple・運営者のサーバーを含みます)に送らず、録音も残しません。**端末の中で認識できない機種・言語では使わず、文字の入力をご案内します。最初に使う時に「音声認識」の許可を求めます。許可しない場合は、モデルの準備ができるまで文字で入力してください。
- 文字にした内容(または直接入力した文)は、アプリを作る・直すために、次の 1.3 のとおり AI に送ります。

### 1.3 第三者の AI(Anthropic)へ送る情報
アプリを作る・直す・相談する時に限り、**事前に同意をいただいた上で**、次の情報を本アプリのサーバー(Google Cloud Run・東京リージョン)を経由して **Anthropic, PBC(米国)の AI「Claude」** に送ります。
- 話した内容・入力した文(文字にしたもの)
- AI との会話の履歴(今の相談の分)
- 今のアプリの設計(画面・項目の名前・アプリ名・選択肢など)

**アプリを作る・直す・相談する時に、記録したデータ(数値・メモ・写真・位置など)の中身は AI に送りません。**

作ったアプリの「AI で書く」ボタン(文を作る・要約する・分類する・書き直す・抜き出す・提案する。利用できる場合に限ります)を押した場合、端末の中の AI(Apple Intelligence)が使える端末では端末の中で処理し、送りません。端末の中の AI が使えない端末では、Tailor It Pro のオンラインの AI として、そのボタンの材料として設定された最近の記録・入力中の値の文字(最大 4KB)を、押した時だけ、同じ同意の範囲で送ります。**写真・音声・動画そのものは送りません。**

送った情報は、アプリを作る・ボタンの文を作る(要約などを含みます)ためだけに使います。本アプリは Anthropic の API を、Anthropic の商用利用規約のもとで使います。この条件では、API 経由の入力と出力は Anthropic のモデルの学習に使われません。Anthropic は、送った情報を同社の定める期間だけ保存して削除します。ただし、利用ポリシー違反の調査や法令への対応のために、より長く保存することがあります(Anthropic のプライバシーポリシーと商用利用規約に従います)。

同意は、初回起動時の画面、または設定の「プライバシー」から行えます。同意しない場合も、作ったアプリの閲覧・記録は使えます(AI を使う機能だけが使えません)。同意は設定からいつでも取り消せます。取り消し後は送りません。送信済みの情報は取り消せません。

### 1.4 作ったアプリと記録
- 作ったアプリの設計(DSL)と版の履歴、記録したデータ(写真を入れた場合は縮小した画像を含む)は、端末に保存し、接続がある時に **Google Cloud Firestore**(運営者の Google Cloud プロジェクト・東京リージョン)へ送って保存します。
- 使い道: 端末を変えても・再インストールしても使えるようにするため、ホーム画面のウィジェット・通知を表示するため。
- 相談の会話は、サーバーでは最大 30 分だけ保持し、端末には履歴として保存します。
- 1 日に AI を使える回数を数えるため、ユーザー ID ごとの利用回数(匿名の場合は IP アドレスを変換した値ごとの回数も)を日ごとに保存します。

### 1.5 端末の機能(作ったアプリが使う場合のみ・許可を求めます)
- 本アプリの版によって、使える機能が異なります。次の機能は、その版で利用できる場合に限り、作ったアプリが使う時だけ許可を求めます。
  - マイク(音声入力・録音)、音声認識(1.2 の準備中の仮の聞き取り)、カメラ・写真(写真の項目・QR コードの読み取り)、位置情報(場所の項目・地図・道案内)、モーション、ヘルスケア(歩数・体重の表示)、ローカルネットワーク(近くの端末と送り合う)、Apple Music、通知。
- ヘルスケアのデータは端末の中で表示するだけで、サーバー・AI には送りません。広告やマーケティングには使いません。App Store で配布する版にはヘルスケアの機能が無い場合があり、その場合はヘルスケアのデータを一切読み取りません。
- 位置情報・写真は、ユーザーが記録に入れた場合に限り、記録の一部として 1.4 のとおり保存します。

### 1.6 共有・公開(利用できる場合)
- 本アプリの版によっては、共有・公開の機能がありません。次は、その機能を利用できる場合の説明です。
- 「共有」を使うと、作ったアプリの設計を共有リンクで他の人が見られるようになります。共有したアプリに参加した人は、表示名(入力した場合)と記録を、そのアプリの参加者と共有します。
- 「探す」に掲載すると、作ったアプリの設計(アプリ名・画面・項目の名前など)が、本アプリの他のユーザーに表示されます。記録したデータは掲載しません。
- 作ったアプリの中に「公開」と表示されるデータ(掲示板・ランキングなど)がある場合、そこに入れた内容はそのアプリを使う他の人も見られます。入れる前に画面でお知らせします。
- 共有・掲載はいつでも取り下げられます。

### 1.7 課金(提供する場合)
- 本アプリの版によっては、アプリ内課金がありません。Tailor It Pro(自動更新サブスクリプション)を提供する場合、支払いは **Apple(App Store)** が処理します。運営者はカード情報などの支払い情報を受け取りません。
- 購入の状態は**本アプリのサーバーで確かめます。**購入・復元の後とアプリの起動時に、Apple が署名した取引の情報を端末からサーバーに送り、サーバーが Apple の署名を確かめた上で、ユーザー ID と一緒に次を保存します。Apple からの更新・返金の知らせ(App Store Server Notifications)と、Apple への問い合わせ(App Store Server API)でも同じ情報を更新します。
  - 商品 ID、取引 ID と最初の取引 ID、有効期限、取り消し(返金)の日時、自動更新の状態、支払いの猶予期間の終わり、本番かテストか(環境)、更新日時
  - 1 つの購読をどのユーザー ID が使っているかの対応(最初の取引 ID ごと)
- 購入には、どのアカウントの購入かを確かめるため、ユーザー ID から作った値(元のユーザー ID には戻せない値)を付けます。
- 使い道: Tailor It Pro が使えるかを判定するため(有料の機能はサーバーで判定します)。

### 1.8 クラッシュ報告
- 本アプリが異常終了(クラッシュ)した時に、原因を調べて直すため、Google の **Firebase Crashlytics** に次の情報を送ります。
  - 落ちた場所(プログラムの位置・スタックトレース)、異常終了の種類、発生日時
  - 端末の機種・OS の版・本アプリの版、発生時の空きメモリ・空き容量・画面の向きなどの端末の状態
  - Crashlytics がインストールごとに作る識別子と、ユーザー ID(uid。1.1 のもの)
- **話した内容・入力した文・記録・作ったアプリの設計(DSL)は送りません。**名前・メールアドレスも送りません。
- 既定はオンです。設定の「プライバシー」の「クラッシュ報告を送る」からいつでもオフにできます。オフにすると、以後は送らず、まだ送っていない報告も端末から消します。
- 使い道: 不具合の原因を調べて直すためだけです。広告・トラッキングには使いません。

### 1.9 集めないもの
- 広告 ID・トラッキング・行動の分析ツール(Firebase Analytics 等)は使いません。第三者への販売はしません。

### 1.10 通知とサーバーの自動の動き(提供する場合)
- 本アプリの版によっては、この機能がありません。次は、その機能を利用できる場合の説明です。
- プッシュ通知を受け取るため、通知を許可した端末について、Apple が発行する端末のトークン(デバイストークン)、インストールごとに作る識別子、配信の環境、本アプリの版、OS の版を、ユーザー ID と一緒にサーバー(Firestore)に保存します。1 人 10 台までで、超えると古いものから消します。トークンはログに書かず、ほかの人には見せません。通知の配信には Apple のプッシュ通知の仕組み(APNs)を使います。
- 同じ端末で別のアカウントにサインインした場合、前のアカウントの通知が届かないよう、前のアカウントに保存したその端末のトークンを消します。サインアウトや通知をやめた時も、その端末のトークンを消します。
- 他の人の操作がきっかけで届く通知は、**アプリごとに受け取る人の同意がある時だけ**送ります。アプリごとの同意の有無を保存し、いつでも取り消せます。共有のアプリでは参加する時に確認した上で有効になり、公開のアプリでは既定で無効です。
- 作成者が Tailor It Pro で設定した「自動の動き」(記録が作られた・更新された時や、決めた時刻に、条件に合えば通知を送る・記録を書く・集計する)は、本アプリのサーバーが実行します。この時、サーバーはそのアプリの記録を読み、条件の判定と通知の文を作るのに使います。この処理で記録を AI に送ることはありません。
- 共有のアプリに参加すると、作成者が設定した自動の動きによって、通知が届くことがあります。通知の本文には、作成者が決めた文と、記録の値の一部(30 字まで)が入ることがあります。URL・電話番号・メールアドレスは入れません。
- 使いすぎを防ぐため、自動の動きの実行・書き込み・通知の数を、アプリごと・受け取る人ごとに数えて保存します。

### 1.11 外部の公開データ(利用できる場合)
- 本アプリの版によっては、この機能がありません。
- 作ったアプリに、国や行政機関が公開しているデータ(天気予報・祝日・お知らせ)を表示できます。本アプリのサーバーが、運営者が決めた出どころだけから取得し、端末へ届けます。端末から出どころへ直接はつながりません。
- 取得する時に出どころへ送るのは、府県予報区のコードと、取得するファイルの指定だけです。**ユーザー ID・位置・記録・話した内容は送りません。**
- 出典(いずれも[公共データ利用規約(第1.0版)](https://www.digital.go.jp/resources/open_data/public_data_license_v1.0)により利用しています)
  - 天気予報: 気象庁ホームページ(https://www.jma.go.jp/)の府県天気予報を加工して作成(天気の種類を言葉と絵に置き換え、表示する日数を絞っています)
  - 祝日: 内閣府ホームページ(https://www8.cao.go.jp/chosei/shukujitsu/gaiyou.html)の「国民の祝日」を加工して作成(これから来る祝日だけを表示しています)
  - お知らせ: デジタル庁ホームページ(https://www.digital.go.jp/)の新着・更新の RSS、消防庁ホームページ(https://www.fdma.go.jp/)の新着情報・災害情報の RSS を加工して作成(題・日時・リンクを件数を絞って表示しています)

## 2. 第三者への提供
次の委託先に、上記の目的の範囲で情報を預けます。法令に基づく場合を除き、これ以外の第三者には提供しません。
- Google LLC(Firebase Authentication・Firebase Crashlytics・Cloud Run・Firestore・Cloud Logging)
- Anthropic, PBC(AI による生成・相談。米国への移転を含みます)
- Apple Inc.(Sign in with Apple・App Store の課金と取引の確認・プッシュ通知の配信)
- Hugging Face, Inc.(音声認識のモデルの配信。1.2 のダウンロードの通信だけで、音声・話した内容は送りません。米国)

1.2 の端末の中の音声認識では、声を Apple に送りません。1.11 の外部の公開データの出どころ(気象庁・内閣府・デジタル庁・消防庁)には、利用者の情報を送りません。

## 3. 保存期間
- 作ったアプリ・記録・アカウントの情報は、アカウントを削除するまで保存します(削除後の扱いは 4 章)。
- 相談の会話は、サーバーで最大 30 分です。
- 購入の記録(1.7)と通知の受け取りの同意(1.10)は、アカウントを削除するまで保存します。
- 通知のための端末のトークン(1.10)は、サインアウト・通知をやめる・アカウントの削除の操作のいずれかまで保存します。
- 1 日の利用回数は、日付ごとの回数(数値)だけで、話した内容などは含みません。
- クラッシュ報告(1.8)は、Firebase Crashlytics の標準の保存期間(受け取ってから 90 日)が過ぎると Google が消去します。
- サーバーの動作記録(ログ。リクエストの日時・経路・結果・IP アドレスなど)は、Google Cloud Logging に最大 30 日保存します。ログには、話した内容・記録の中身を書かないようにしています。
- 障害やデータの破損に備えて、Firestore のバックアップを取っています。直前 7 日間の任意の時点に戻すための記録と、1 日 1 回の複製(14 日間保持)があり、どちらも期限が来ると自動的に消えます。

## 4. アカウントと情報の削除
- 設定の「アカウントを削除」から、アプリ内で削除できます。削除すると、作ったアプリと共有リンクが使えなくなり、端末のデータを消し、Firebase のアカウントを削除し、Sign in with Apple の連携を解除(トークンの失効)します。
- サーバー上のデータは、削除の操作の時点で使えなくし(論理削除)、30 日以内に消去します。ほかの人と共有しているアプリへの投稿は残ります。
- 通知のための端末のトークンは、削除の操作の時点で消し、通知を止めます。作成したアプリの自動の動きも止まります。購入の記録と通知の受け取りの同意は、30 日以内に消去します。
- Tailor It Pro の購読は、アカウントを削除しても自動では解約されません。iPhone の「設定」から解約してください(利用規約 4 章)。
- 消去したデータも、バックアップには最大 14 日間残り、その後自動的に消えます。バックアップは障害からの復旧にだけ使い、復旧したときは消去済みのデータを改めて消去します。
- 作ったアプリを 1 つずつ削除した場合は、そのアプリのサーバー上のデータをその時点で消去します。

## 5. お子さまの利用
- 本アプリは 13 歳未満の方を対象にしていません。
- 13 歳以上 18 歳未満の方は、保護者の同意を得た上で使ってください。

## 6. 改定
- 本ポリシーを変える場合はアプリ内でお知らせします。AI に送る情報や送り先を変える場合は、改めて同意をいただきます。

## 7. お問い合わせ
- 高岡己太朗
- メール: support@tailor-it.app

---

## English summary

> This summary is for convenience. The Japanese text above prevails.

- **Operator:** Kotaro Takaoka (高岡己太朗) (individual). **Effective:** October 30, 2026. **Revised:** November 7, 2026.
- **Sign-in:** Sign in with Apple or anonymous, via Google Firebase Authentication. Firebase stores your user ID and, if Apple provides them, your name and email (or Apple's private relay address). Our own server (Firestore) uses only the user ID.
- **Voice:** Speech is transcribed **on the device** (WhisperKit). **Audio is never uploaded.** Only when the speech model is not on the device (e.g. first use), and after asking you, it is downloaded over HTTPS from **Hugging Face, Inc. (USA)**. Hugging Face receives only standard connection data (your IP address and the file request), never audio, what you say, your records or account details. See [Hugging Face's privacy policy](https://huggingface.co/privacy). While the model is being prepared, Apple's **on-device** speech recognition transcribes your speech temporarily (with the "Speech Recognition" permission); your voice still never leaves the device and is not recorded.
- **Third-party AI:** Only **after your explicit consent**, the transcribed/typed request, the current conversation and the current app design (screen/field names) are sent through our server (Google Cloud Run, Tokyo) to **Anthropic, PBC (USA, "Claude")** solely to build or edit your app. The contents of your records are not sent when building, editing or chatting. If your app has an "AI write" button (write, summarize, classify, rewrite, extract or suggest; where available) and you tap it, it runs on the device where on-device AI (Apple Intelligence) is available and nothing is sent. Otherwise, as a Tailor It Pro online AI feature, the recent records or form values configured for that button (up to 4KB of text) are sent when you tap it. Photos, audio and video are never sent. Under Anthropic's commercial terms, API inputs and outputs are not used to train their models. Without consent you can still view and use your apps. You can withdraw consent in Settings at any time.
- **Storage:** App designs, version history and records (including downscaled photos you add) are stored on the device and in Google Cloud Firestore (Tokyo). Chat state is kept on the server for up to 30 minutes. Server logs are kept for up to 30 days. We keep Firestore backups to recover from failures: point-in-time recovery data for the last 7 days and daily backups kept for 14 days. Both expire automatically.
- **Device features** (microphone, speech recognition, camera/photos, location, motion, Health, local network, Apple Music, notifications) are used only where available in your build, only when your app uses them, and with your permission. Health data stays on the device and is never used for advertising.
- **Sharing / public data** (where available): shared links, the "Discover" catalog and data marked "public" can be seen by others. You can withdraw sharing at any time.
- **Payments** (if offered) are processed by Apple; we never receive payment details. Our server verifies Apple's signed transactions (and Apple's server notifications) and stores, with your user ID, the product ID, transaction IDs, expiry, revocation/refund time, auto-renew status, grace period, environment and which account a subscription belongs to. Purchases carry a value derived from your user ID that cannot be turned back into it.
- **Notifications and automations** (where available): to deliver push notifications we store your device token, a per-installation ID, environment, app and OS version with your user ID (up to 10 devices), and deliver them through Apple Push Notification service. Notifications triggered by other people are sent only if you agreed per app (on when you join a shared app after confirming; off by default for public apps); you can turn this off any time. Automations set by an app's creator (Tailor It Pro) run on our server on record changes or at set times and may notify participants; notification text may include up to 30 characters of record values but never URLs, phone numbers or email addresses. Records are not sent to AI for this.
- **Public data** (where available): our server fetches weather forecasts, national holidays and notices from the Japan Meteorological Agency, Cabinet Office, Digital Agency and Fire and Disaster Management Agency, used and modified under the Public Data License (Version 1.0). We send them only a forecast-area code or the feed to fetch, never your user ID, location or records.
- **Crash reports:** When the app crashes, Google's Firebase Crashlytics receives the crash location (stack trace), crash time, device model, OS and app version, device state (free memory/disk, orientation), a per-installation identifier and your user ID (uid). **What you say, your records and your app designs are never included.** On by default; turn it off in Settings > Privacy ("Send crash reports"). Google deletes crash reports 90 days after receipt (Crashlytics' standard retention period).
- **No ads, tracking or analytics SDKs.** We do not sell data.
- **Deletion:** Delete your account in Settings. Server data becomes unavailable immediately and is erased within 30 days. Device tokens are deleted immediately and your apps' automations stop; purchase records and notification consents are erased within 30 days. Deleting your account does not cancel a Tailor It Pro subscription; cancel it in iOS Settings. Posts in apps shared with others remain. Erased data may remain in backups for up to 14 more days until they expire. Backups are used only for disaster recovery; if we restore one, we erase previously deleted data again.
- **Children:** Not intended for children under 13. Users aged 13 to 17 must have consent from a parent or guardian.
- **Contact:** support@tailor-it.app
