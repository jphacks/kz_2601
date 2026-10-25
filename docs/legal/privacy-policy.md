# Tailor It プライバシーポリシー

> **下書き・要確認(#246)**: この文書は実装を読んで作った下書きです。法的な確認を経た最終版ではありません。公開前に、運営者名・連絡先・施行日・保存期間・準拠法を人が確かめて埋めてください(#241 / #244)。

<!-- 人が確かめる点: [要確認] の箇所。公開 URL が決まったら ios/Metamon/Features/Consent/LegalDocuments.swift の publicURL と App Store Connect に登録する -->

施行日: [要確認: 公開日]
運営者: [要確認: 運営者名(チーム kz_2601 / 個人名 / 団体名)]
連絡先: [要確認: 問い合わせ用メールアドレス]

Tailor It(以下「本アプリ」)は、話した内容や入力した文から、自分専用のアプリ(記録・一覧・集計などの画面)を作って直せる iOS アプリです。本ポリシーは、本アプリが集める情報、使い道、送り先、保存と削除について説明します。

## 1. 集める情報と使い道

### 1.1 アカウントの情報
- **Sign in with Apple** または匿名でサインインします。認証は Google の **Firebase Authentication** が行います。
- 運営者が受け取るのは、Firebase が発行するユーザー ID(uid)です。Apple でサインインした場合、Apple が提供する場合に限り表示名を Firebase に保存することがあります。メールアドレスは求めません([要確認: Firebase コンソール側の保存項目])。
- 使い道: 本人の確認、作ったアプリと記録を本人だけが使えるようにするため。

### 1.2 話した内容・入力した文(音声)
- **音声は端末の中で文字にします**(WhisperKit。音声認識のモデルは端末に保存します)。**音声そのもの(録音データ)はサーバーに送りません。**
- 文字にした内容(または直接入力した文)は、アプリを作る・直すために、次の 1.3 のとおり AI に送ります。

### 1.3 第三者の AI(Anthropic)へ送る情報
アプリを作る・直す・相談する時に限り、**事前に同意をいただいた上で**、次の情報を本アプリのサーバー(Google Cloud Run・東京リージョン)を経由して **Anthropic, PBC(米国)の AI「Claude」** に送ります。
- 話した内容・入力した文(文字にしたもの)
- AI との会話の履歴(今の相談の分)
- 今のアプリの設計(画面・項目の名前・アプリ名・選択肢など)

**記録したデータ(数値・メモ・写真・位置など)の中身は AI に送りません。** 送った情報は、アプリを作るためだけに使います。Anthropic における取り扱いは、Anthropic の商用利用規約・プライバシーポリシーに従います(API 経由の入力は、原則として Anthropic のモデルの学習に使われません。[要確認: 契約プランの条件と保存期間])。

同意は、初回起動時の画面、または設定の「プライバシー」から行えます。同意しない場合も、作ったアプリの閲覧・記録は使えます(生成と相談だけが使えません)。同意は設定からいつでも取り消せます。取り消し後は送りません。送信済みの情報は取り消せません。

### 1.4 作ったアプリと記録
- 作ったアプリの設計(DSL)と版の履歴、記録したデータ(写真を入れた場合は縮小した画像を含む)は、端末に保存し、接続がある時に **Google Cloud Firestore**(運営者の Google Cloud プロジェクト)へ送って保存します。
- 使い道: 端末を変えても・再インストールしても使えるようにするため、ホーム画面のウィジェット・通知を表示するため。
- 相談の会話は、サーバーでは最大 30 分だけ保持し、端末には履歴として保存します。

### 1.5 端末の機能(作ったアプリが使う場合のみ・許可を求めます)
- マイク(音声入力)、カメラ・写真(写真の項目)、位置情報(場所の項目・地図)、モーション、ヘルスケア(歩数・体重の表示)、ローカルネットワーク(近くの端末と送り合う)、通知。
- ヘルスケアのデータは端末の中で表示するだけで、サーバー・AI には送りません。広告やマーケティングには使いません([要確認: 審査用ビルドで HealthKit を外すか #245])。
- 位置情報・写真は、ユーザーが記録に入れた場合に限り、記録の一部として 1.4 のとおり保存します。

### 1.6 共有
- 「共有」を使うと、作ったアプリの設計を共有リンクで他の人が見られるようになります([要確認: 「探す」への掲載の有無と範囲])。共有はいつでも取り下げられます。

### 1.7 課金
- Tailor It Pro(自動更新サブスクリプション)の支払いは **Apple(App Store)** が処理します。運営者はカード情報などの支払い情報を受け取りません。購入の状態は端末で StoreKit から確かめます。

### 1.8 集めないもの
- 広告 ID・トラッキング・行動の分析ツール(Firebase Analytics 等)は使いません。第三者への販売はしません。

## 2. 第三者への提供
次の委託先に、上記の目的の範囲で情報を預けます。法令に基づく場合を除き、これ以外の第三者には提供しません。
- Google LLC(Firebase Authentication・Cloud Run・Firestore)
- Anthropic, PBC(AI による生成・相談。米国への移転を含みます)
- Apple Inc.(Sign in with Apple・App Store の課金)

## 3. 保存期間
- アカウントを削除するまで保存します。相談の会話はサーバーで最大 30 分です。[要確認: バックアップ・ログの保存期間]

## 4. アカウントと情報の削除
- 設定の「アカウントを削除」から、アプリ内で削除できます。削除すると、作ったアプリと共有リンクが使えなくなり、端末のデータを消し、Firebase のアカウントを削除し、Sign in with Apple の連携を解除(トークンの失効)します。
- サーバー上のデータは、削除の操作の時点で使えなくし(論理削除)、[要確認: 物理削除までの期間]に消去します。ほかの人と共有しているアプリへの投稿は残ります。

## 5. お子さまの利用
- 本アプリは 13 歳未満の方を対象にしていません。[要確認: 年齢区分]

## 6. 改定
- 本ポリシーを変える場合はアプリ内でお知らせします。AI に送る情報や送り先を変える場合は、改めて同意をいただきます。

## 7. お問い合わせ
- [要確認: 連絡先]

---

## English summary (draft)

> **Draft — to be reviewed.** Not a final legal text.

- **Sign-in:** Sign in with Apple or anonymous, via Google Firebase Authentication. We receive a Firebase user ID; no email address is requested.
- **Voice:** Speech is transcribed **on the device** (WhisperKit). **Audio is never uploaded.**
- **Third-party AI:** Only **after your explicit consent**, the transcribed/typed request, the current conversation and the current app design (screen/field names) are sent through our server (Google Cloud Run, Tokyo) to **Anthropic, PBC (USA, "Claude")** solely to build or edit your app. **The contents of your records are not sent to the AI.** Without consent you can still view and use your apps; only generation and chat are disabled. You can withdraw consent in Settings at any time.
- **Storage:** App designs, version history and records (including downscaled photos you add) are stored on the device and in Google Cloud Firestore. Chat state is kept on the server for up to 30 minutes.
- **Device features** (microphone, camera/photos, location, motion, Health, local network, notifications) are used only when your app uses them and with your permission. Health data stays on the device and is never used for advertising.
- **Payments** are processed by Apple; we never receive payment details.
- **No ads, tracking or analytics SDKs.** We do not sell data.
- **Deletion:** Delete your account in Settings. This removes your apps and share links, wipes local data, deletes the Firebase account and revokes the Sign in with Apple token.
- **Contact:** [TBD]
