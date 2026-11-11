# よくある質問(FAQ)の原稿

#320。公式サイト `https://tailor-it.app/faq` に載せる FAQ の原稿(日本語・英語)。

- 置き場: 公式サイトのソースはこの repo に無い(別の repo `2410as/tailor-it-web`・private)。この原稿を写して `/faq` を作り、`/contact` からリンクするのは人の作業(下の「公開の前に」)。
- 書き方: 答えは短く。値段は書かない(購入画面に出る)。言い回しは [store/wording.md](../store/wording.md) に合わせ、「アプリを作る」ではなく「記録帳(ツール)を用意する」と書く。
- 根拠: [legal/privacy-policy.md](../legal/privacy-policy.md)・[legal/terms.md](../legal/terms.md)・[ios/architecture.md](../ios/architecture.md)(元に戻す・共有・アカウント削除)・[infra/moderation.md](../infra/moderation.md)。規約・ポリシーと食い違ったら、規約・ポリシーが正しい。

## 目次

1. [公開の前に(人の作業)](#1-公開の前に人の作業)
2. [日本語](#2-日本語)
3. [English](#3-english)

## 1. 公開の前に(人の作業)

- [ ] `2410as/tailor-it-web` に `/faq` を足し、2 章と 3 章を写す(サイトの見た目に合わせる)。`/contact` から `/faq` へリンクする。
- [ ] 「持てる記録帳の数」(無料 2 つ・Pro 5 つ。2026-11-10 の決定)は、**その上限が本番に入ってから**載せる。入る前に公開するなら、その問いを外す。
- [ ] 「コイン」の問いは、コインを始める前は「準備中」の文のまま。始めたら中身を足す(値段は書かない)。
- [ ] 審査用の版(共有・App 内課金なし)の間は、共有・Pro・自動の動きの問いに「利用できる版のみ」と書いてあることを確かめる([store/wording.md](../store/wording.md) 3 章)。
- [ ] support@tailor-it.app にテストのメールを送り、受け取れて返信できることを確かめる。

## 2. 日本語

### 使い方

**Q. 何ができるアプリですか?**
A. 「水を飲んだ量を記録したい」のように話すだけで、自分専用の記録帳・ツールが用意されます。入力欄・一覧・合計・グラフ・リマインダーなどがそろった画面です。

**Q. 声を使わずに使えますか?**
A. はい。文字で入力しても同じように用意・変更できます。

**Q. 用意した記録帳を変えたい時は?**
A. 使いながら「月の合計も出して」「ボタンを大きく」のように話すと、その場で形が変わります。

**Q. 変更を取り消したい時は?**
A. 「元に戻して」と話すと、ひとつ前の形に戻ります。記録を消した直後は、画面の下に出る「元に戻す」で戻せます。

**Q. いくつも記録帳を使えますか?**
A. はい。一覧から切り替えて使えます。持てる数は、無料では 2 つ、Tailor It Pro では 5 つまでです。

**Q. AI を使う回数に上限はありますか?**
A. 1 日あたりの上限があります。上限に達した時はアプリの中でお知らせします。上限は運営の状況で変わることがあります。

### 共有・参加(利用できる版のみ)

**Q. 記録帳をほかの人と使えますか?**
A. 記録帳の一覧でカードの「…」から「シェア・公開」を選ぶと、QR コードとリンクが出ます。相手がそれを開くと参加できます。共有はいつでも取り消せます。

**Q. 参加するのにお金はかかりますか?**
A. かかりません。みんなで使うデータなどの有料の機能は、用意した人の Tailor It Pro で使えます。

**Q. 参加した記録帳から通知が来ます。止められますか?**
A. はい。ほかの人の操作がきっかけの通知は、記録帳ごとに設定の「他の人の行動で届く通知」で止められます。

### Tailor It Pro・コイン

**Q. Tailor It Pro で何が変わりますか?**
A. みんなで使うデータ、サーバーの自動の動き(通知・決めた時刻の処理)、記録帳の中のオンラインの AI が使えるようになります。内容と料金は購入画面に表示します。

**Q. Pro が切れたら、記録は消えますか?**
A. 消えません。開く・見ることはできます。有料の機能は使えなくなり、自動の動きは止まります。

**Q. 解約したい。**
A. iPhone の「設定」>「Apple ID」>「サブスクリプション」から解約してください。更新日の 24 時間前までに解約すると、次の更新は行われません。アカウントを削除しても、購読は自動では解約されません。

**Q. 返金してほしい。**
A. 支払いは Apple が処理しているため、返金は Apple に申し込みます([reportaproblem.apple.com](https://reportaproblem.apple.com/))。運営者からは返金できません。

**Q. コインとは何ですか?**
A. 準備中です。始める時にアプリとこのページでお知らせします。

### 自動の動き・通知

**Q. 「自動の動き」とは?**
A. 記録が増えた時や決めた時刻に、通知を送る・記録を書く・集計するしくみです(Tailor It Pro・利用できる版のみ)。サーバーが実行します。

**Q. 自動の動きが止まりました。**
A. 決められた上限(回数など)に達すると止まり、後からまとめて実行はしません。入れた記録は残ります。運営が止めた場合は、アプリの案内からお問い合わせください。

**Q. リマインダーが届きません。**
A. iPhone の「設定」で Tailor It の通知が許可されているか確かめてください。通知は遅れたり届かなかったりすることがあるため、大切な連絡には使わないでください。

### データとプライバシー

**Q. 声はどこかに送られますか?**
A. 送りません。声は端末の中で文字にします。録音データはサーバーに送りません。

**Q. AI には何が送られますか?**
A. 記録帳を用意・変更する時に、話した内容(文字にしたもの)と今の画面の形を、同意をいただいた上で AI(Anthropic)に送ります。記録の中身は送りません。記録帳の中の AI の部品を使った時だけ、押した時に必要な分を送ります。

**Q. 送った内容は AI の学習に使われますか?**
A. 使われません。詳しくは[プライバシーポリシー](https://tailor-it.app/privacy)をご覧ください。

**Q. アカウントとデータを消したい。**
A. 設定の「アカウントを削除」から消せます。サーバーのデータは 30 日以内に消去します。記録帳を 1 つだけ消すこともできます。

**Q. 機種変更したら記録は残りますか?**
A. 同じアカウント(Sign in with Apple)でサインインすれば残ります。匿名で使っている場合は引き継げないことがあります。

### お問い合わせ

**Q. 問い合わせ先は?**
A. support@tailor-it.app です。設定の「詳細(サポート用)」>「詳細をコピー」の内容を添えていただくと、早く調べられます。

**Q. 不適切な共有や通知を見つけた。**
A. アプリの「通報する」か、support@tailor-it.app でお知らせください。運営が確認して対応します。

## 3. English

### Using the app

**Q. What does Tailor It do?**
A. Say something like "I want to track how much water I drink" and a tracker made for you is set up, with inputs, lists, totals, charts and reminders.

**Q. Can I use it without voice?**
A. Yes. Typing works the same way.

**Q. How do I change a tracker?**
A. Just say what you want, like "show the monthly total too" or "make the button bigger", and it changes right away.

**Q. How do I undo a change?**
A. Say "undo" to go back one step. Right after deleting a record, tap "Undo" at the bottom of the screen.

**Q. Can I have more than one tracker?**
A. Yes, switch between them from the list. You can keep up to 2 on the free plan and up to 5 with Tailor It Pro.

**Q. Is there a limit on AI use?**
A. Yes, there is a daily limit. The app tells you when you reach it. The limit may change.

### Sharing and joining (where available)

**Q. Can I use a tracker with other people?**
A. In the tracker list, tap "…" on a card and choose "Share / Publish" to get a QR code and a link. Others join by opening it. You can stop sharing at any time.

**Q. Does joining cost anything?**
A. No. Paid features such as shared data run on the creator's Tailor It Pro.

**Q. A shared tracker keeps notifying me. Can I stop it?**
A. Yes. Notifications triggered by other people can be turned off per tracker in Settings ("Notifications from other people's activity").

### Tailor It Pro and coins

**Q. What does Tailor It Pro add?**
A. Shared data, server automations (notifications and scheduled actions) and online AI inside your trackers. Details and prices are shown on the purchase screen.

**Q. If Pro ends, do I lose my records?**
A. No. You can still open and view them. Paid features stop working and automations stop.

**Q. How do I cancel?**
A. In iPhone Settings > Apple ID > Subscriptions, at least 24 hours before renewal. Deleting your account does not cancel your subscription.

**Q. Can I get a refund?**
A. Payments are handled by Apple, so please request refunds from Apple ([reportaproblem.apple.com](https://reportaproblem.apple.com/)). We cannot issue refunds ourselves.

**Q. What are coins?**
A. Coming later. We will announce them in the app and on this page.

### Automations and notifications

**Q. What are automations?**
A. They send notifications, write records or calculate totals when records change or at set times (Tailor It Pro, where available). They run on our server.

**Q. My automation stopped.**
A. Automations stop when they reach set limits and are not run later. Your records are kept. If we stopped it, contact us from the notice in the app.

**Q. I don't get reminders.**
A. Check that notifications for Tailor It are allowed in iPhone Settings. Notifications can be delayed or missed, so do not rely on them for critical alerts.

### Data and privacy

**Q. Is my voice sent anywhere?**
A. No. Speech is turned into text on your device. Audio is never uploaded.

**Q. What is sent to AI?**
A. With your consent, when you set up or change a tracker, what you said (as text) and the current screen layout are sent to AI (Anthropic). Your records are not sent. AI parts inside a tracker send only what they need, only when you use them.

**Q. Is my data used to train AI?**
A. No. See our [Privacy Policy](https://tailor-it.app/privacy).

**Q. How do I delete my account and data?**
A. Use "Delete Account" in Settings. Server data is erased within 30 days. You can also delete a single tracker.

**Q. Will my records move to a new iPhone?**
A. Yes, if you sign in with the same account (Sign in with Apple). Anonymous use may not carry over.

### Contact

**Q. How do I contact you?**
A. Email support@tailor-it.app. Including the text from Settings > "Details (for support)" > "Copy details" helps us look into it faster.

**Q. I found inappropriate shared content or notifications.**
A. Use "Report" in the app or email support@tailor-it.app. We will review it.
