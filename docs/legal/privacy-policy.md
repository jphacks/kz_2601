# Tailor It プライバシーポリシー


施行日: 2026年10月30日
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
- 文字にした内容(または直接入力した文)は、アプリを作る・直すために、次の 1.3 のとおり AI に送ります。

### 1.3 第三者の AI(Anthropic)へ送る情報
アプリを作る・直す・相談する時に限り、**事前に同意をいただいた上で**、次の情報を本アプリのサーバー(Google Cloud Run・東京リージョン)を経由して **Anthropic, PBC(米国)の AI「Claude」** に送ります。
- 話した内容・入力した文(文字にしたもの)
- AI との会話の履歴(今の相談の分)
- 今のアプリの設計(画面・項目の名前・アプリ名・選択肢など)

**アプリを作る・直す・相談する時に、記録したデータ(数値・メモ・写真・位置など)の中身は AI に送りません。** ただし、作ったアプリに「AI で文を作る」ボタンがあり、それを押した場合(利用できる場合に限ります)は、そのボタンの材料として設定された最近の記録・入力中の値の文字(最大 4KB)を、同じ同意の範囲で送ります。写真・音声・動画そのものは送りません。

送った情報は、アプリを作る・文を作るためだけに使います。本アプリは Anthropic の API を、Anthropic の商用利用規約のもとで使います。この条件では、API 経由の入力と出力は Anthropic のモデルの学習に使われません。Anthropic は、送った情報を同社の定める期間だけ保存して削除します。ただし、利用ポリシー違反の調査や法令への対応のために、より長く保存することがあります(Anthropic のプライバシーポリシーと商用利用規約に従います)。

同意は、初回起動時の画面、または設定の「プライバシー」から行えます。同意しない場合も、作ったアプリの閲覧・記録は使えます(AI を使う機能だけが使えません)。同意は設定からいつでも取り消せます。取り消し後は送りません。送信済みの情報は取り消せません。

### 1.4 作ったアプリと記録
- 作ったアプリの設計(DSL)と版の履歴、記録したデータ(写真を入れた場合は縮小した画像を含む)は、端末に保存し、接続がある時に **Google Cloud Firestore**(運営者の Google Cloud プロジェクト・東京リージョン)へ送って保存します。
- 使い道: 端末を変えても・再インストールしても使えるようにするため、ホーム画面のウィジェット・通知を表示するため。
- 相談の会話は、サーバーでは最大 30 分だけ保持し、端末には履歴として保存します。
- 1 日に AI を使える回数を数えるため、ユーザー ID ごとの利用回数(匿名の場合は IP アドレスを変換した値ごとの回数も)を日ごとに保存します。

### 1.5 端末の機能(作ったアプリが使う場合のみ・許可を求めます)
- 本アプリの版によって、使える機能が異なります。次の機能は、その版で利用できる場合に限り、作ったアプリが使う時だけ許可を求めます。
  - マイク(音声入力・録音)、カメラ・写真(写真の項目・QR コードの読み取り)、位置情報(場所の項目・地図・道案内)、モーション、ヘルスケア(歩数・体重の表示)、ローカルネットワーク(近くの端末と送り合う)、Apple Music、通知。
- ヘルスケアのデータは端末の中で表示するだけで、サーバー・AI には送りません。広告やマーケティングには使いません。App Store で配布する版にはヘルスケアの機能が無い場合があり、その場合はヘルスケアのデータを一切読み取りません。
- 位置情報・写真は、ユーザーが記録に入れた場合に限り、記録の一部として 1.4 のとおり保存します。

### 1.6 共有・公開(利用できる場合)
- 本アプリの版によっては、共有・公開の機能がありません。次は、その機能を利用できる場合の説明です。
- 「共有」を使うと、作ったアプリの設計を共有リンクで他の人が見られるようになります。共有したアプリに参加した人は、表示名(入力した場合)と記録を、そのアプリの参加者と共有します。
- 「探す」に掲載すると、作ったアプリの設計(アプリ名・画面・項目の名前など)が、本アプリの他のユーザーに表示されます。記録したデータは掲載しません。
- 作ったアプリの中に「公開」と表示されるデータ(掲示板・ランキングなど)がある場合、そこに入れた内容はそのアプリを使う他の人も見られます。入れる前に画面でお知らせします。
- 共有・掲載はいつでも取り下げられます。

### 1.7 課金(提供する場合)
- 本アプリの版によっては、アプリ内課金がありません。Tailor It Pro(自動更新サブスクリプション)を提供する場合、支払いは **Apple(App Store)** が処理します。運営者はカード情報などの支払い情報を受け取りません。購入の状態は端末で StoreKit から確かめます。

### 1.8 集めないもの
- 広告 ID・トラッキング・行動の分析ツール(Firebase Analytics 等)は使いません。第三者への販売はしません。

## 2. 第三者への提供
次の委託先に、上記の目的の範囲で情報を預けます。法令に基づく場合を除き、これ以外の第三者には提供しません。
- Google LLC(Firebase Authentication・Cloud Run・Firestore・Cloud Logging)
- Anthropic, PBC(AI による生成・相談。米国への移転を含みます)
- Apple Inc.(Sign in with Apple・App Store の課金)

## 3. 保存期間
- 作ったアプリ・記録・アカウントの情報は、アカウントを削除するまで保存します(削除後の扱いは 4 章)。
- 相談の会話は、サーバーで最大 30 分です。
- 1 日の利用回数は、日付ごとの回数(数値)だけで、話した内容などは含みません。
- サーバーの動作記録(ログ。リクエストの日時・経路・結果・IP アドレスなど)は、Google Cloud Logging に最大 30 日保存します。ログには、話した内容・記録の中身を書かないようにしています。
- 障害やデータの破損に備えて、Firestore のバックアップを取っています。直前 7 日間の任意の時点に戻すための記録と、1 日 1 回の複製(14 日間保持)があり、どちらも期限が来ると自動的に消えます。

## 4. アカウントと情報の削除
- 設定の「アカウントを削除」から、アプリ内で削除できます。削除すると、作ったアプリと共有リンクが使えなくなり、端末のデータを消し、Firebase のアカウントを削除し、Sign in with Apple の連携を解除(トークンの失効)します。
- サーバー上のデータは、削除の操作の時点で使えなくし(論理削除)、30 日以内に消去します。ほかの人と共有しているアプリへの投稿は残ります。
- 消去したデータも、バックアップには最大 14 日間残り、その後自動的に消えます。バックアップは障害からの復旧にだけ使い、復旧したときは消去済みのデータを改めて消去します。
- 作ったアプリを 1 つずつ削除した場合は、そのアプリのサーバー上のデータをその時点で消去します。

## 5. お子さまの利用
- 本アプリは 13 歳未満の方を対象にしていません。

## 6. 改定
- 本ポリシーを変える場合はアプリ内でお知らせします。AI に送る情報や送り先を変える場合は、改めて同意をいただきます。

## 7. お問い合わせ
- 高岡己太朗
- メール: support@tailor-it.app

---

## English summary

> This summary is for convenience. The Japanese text above prevails.

- **Operator:** Kotaro Takaoka (高岡己太朗) (individual). **Effective:** October 30, 2026.
- **Sign-in:** Sign in with Apple or anonymous, via Google Firebase Authentication. Firebase stores your user ID and, if Apple provides them, your name and email (or Apple's private relay address). Our own server (Firestore) uses only the user ID.
- **Voice:** Speech is transcribed **on the device** (WhisperKit). **Audio is never uploaded.**
- **Third-party AI:** Only **after your explicit consent**, the transcribed/typed request, the current conversation and the current app design (screen/field names) are sent through our server (Google Cloud Run, Tokyo) to **Anthropic, PBC (USA, "Claude")** solely to build or edit your app. The contents of your records are not sent when building, editing or chatting. If your app has an "AI write" button (where available) and you tap it, the recent records or form values configured for that button (up to 4KB of text) are sent. Under Anthropic's commercial terms, API inputs and outputs are not used to train their models. Without consent you can still view and use your apps. You can withdraw consent in Settings at any time.
- **Storage:** App designs, version history and records (including downscaled photos you add) are stored on the device and in Google Cloud Firestore (Tokyo). Chat state is kept on the server for up to 30 minutes. Server logs are kept for up to 30 days. We keep Firestore backups to recover from failures: point-in-time recovery data for the last 7 days and daily backups kept for 14 days. Both expire automatically.
- **Device features** (microphone, camera/photos, location, motion, Health, local network, Apple Music, notifications) are used only where available in your build, only when your app uses them, and with your permission. Health data stays on the device and is never used for advertising.
- **Sharing / public data** (where available): shared links, the "Discover" catalog and data marked "public" can be seen by others. You can withdraw sharing at any time.
- **Payments** (if offered) are processed by Apple; we never receive payment details.
- **No ads, tracking or analytics SDKs.** We do not sell data.
- **Deletion:** Delete your account in Settings. Server data becomes unavailable immediately and is erased within 30 days. Posts in apps shared with others remain. Erased data may remain in backups for up to 14 more days until they expire. Backups are used only for disaster recovery; if we restore one, we erase previously deleted data again.
- **Children:** Not intended for children under 13.
- **Contact:** support@tailor-it.app
