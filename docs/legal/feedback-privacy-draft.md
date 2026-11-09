# 下書き: フィードバック(#319)のプライバシーポリシーの文案

> 2026-11-08: この文案は `privacy-policy.md` の 1.13(と 3・4 章)に入れた。以下は経緯の記録。

**これは下書きです。`privacy-policy.md` はまだ変えていません。** 同意の範囲・伏せ字・保存期間は #367 で決めます。決まったら、この文案を直してから `privacy-policy.md`(#246)とプライバシーの表示(#328)に入れます。

## 目次

1. [今の実装(この文案の前提)](#今の実装この文案の前提)
2. [文案(日本語)](#文案日本語)
3. [Draft (English)](#draft-english)
4. [App Store のプライバシーの表示への影響](#app-store-のプライバシーの表示への影響)
5. [#367 で決めること](#367-で決めること)

## 今の実装(この文案の前提)

| 項目 | 内容 |
|---|---|
| 入口 | 設定の「フィードバックを送る」と、端末を振ったとき。**TestFlight などの通常の版だけ**。審査用の版(App Store)には入れていない |
| 必ず送るもの | 本文(2000 字まで)・種類(不具合 / 要望 / その他)・ユーザー ID(uid)・同意文の版・送った日時 |
| 本人が選んだ時だけ送るもの(既定はすべてオフ) | 今の画面のスクショ(JPEG・512 KiB まで)/ 今のアプリの設計(DSL。アプリ名・画面の文言・項目の名前を含む)/ アプリの版・OS・機種・表示言語 |
| 送らないもの | 記録(入力したデータ)の中身・名前・メールアドレス・位置 |
| 送る前 | オンにした添付は中身をその場で見せる。同意文を見せ、同意のトグルを入れるまで送れない |
| 保存先 | Google Cloud の Firestore(`feedback/{id}`)。Cloud Storage は使っていない |
| 読む人 | 運営者だけ(読み取り専用の `backend/cmd/feedback`)。AI には読ませていない。Issue にも載せていない |
| 保存期間 | 180 日。毎日のジョブ(`cmd/purge`)が消す |
| アカウントの削除 | 削除から 30 日以内の消去(ポリシー §4)のときに、その人のフィードバックも消す |
| バックアップ | Firestore のバックアップに最大 14 日残る(ポリシー §4 と同じ) |

## 文案(日本語)

§1 に足す:

> ### 1.10 フィードバック(利用できる場合)
> - アプリ内の「フィードバックを送る」(または端末を振る)から、不具合や要望を送れます。送るのは、書いた本文、種類、ユーザー ID(uid)、送った日時です。
> - 次のものは、送る画面で本人がオンにした場合だけ送ります(既定はすべてオフです)。送る前に中身を表示します。
>   - 今の画面のスクリーンショット
>   - 今のアプリの設計(DSL。アプリの名前・画面の文言・項目の名前を含みます)
>   - 本アプリの版・OS の版・端末の機種・表示言語
> - **記録(入力したデータ)の中身は送りません。** スクリーンショットには画面に表示されている内容が写るため、送る前に確認してください。
> - 使い道: 不具合を直し、本アプリを改善するために運営者が読むためだけです。広告・トラッキングには使いません。

§2 の委託先は変わらない(Google LLC の Firestore に保存。新しい委託先は無い)。

§3 に足す:

> - フィードバック(1.10)は、受け取ってから 180 日で消去します。

§4 に足す:

> - アカウントを削除すると、送ったフィードバックも、ほかのデータと同じく 30 日以内に消去します。

## Draft (English)

Add to §1:

> ### 1.10 Feedback (where available)
> - You can send bug reports and suggestions from "Send feedback" in the app (or by shaking the device). We receive the text you write, its type, your user ID (uid), and the time it was sent.
> - The following are sent only if you turn them on in the feedback screen (all are off by default). Their contents are shown before you send.
>   - A screenshot of the current screen
>   - The design of the current app (DSL, including the app name, screen text, and field names)
>   - The app version, OS version, device model, and display language
> - **We never send the contents of your records (the data you entered).** A screenshot shows whatever is on the screen, so please check it before sending.
> - Use: read by the operator only to fix bugs and improve the app. Not used for advertising or tracking.

Add to §3: "Feedback (1.10) is erased 180 days after we receive it."

Add to §4: "When you delete your account, the feedback you sent is erased within 30 days, together with your other data."

## App Store のプライバシーの表示への影響

今の表示(`docs/ios/review-build.md` の「プライバシーマニフェスト」)にあるのは、ユーザー ID・名前・その他のユーザーコンテンツ・写真・正確な位置・クラッシュのデータ・パフォーマンスのデータ。フィードバックを入れると、少なくとも次が増える。

| 増える種類(App Store Connect の名前) | 理由 |
|---|---|
| ユーザーコンテンツ > カスタマーサポート(Customer Support) | 本文そのもの |
| 診断 > その他の診断データ(Other Diagnostic Data) | 版・OS・機種(オンにした時) |
| (スクショ)写真またはビデオ / その他のユーザーコンテンツ | 画面の画像(オンにした時)。どちらに当たるかは要確認 |

そのため、**審査用の版(`APP_STORE_REVIEW`)からは外している**(`#if !APP_STORE_REVIEW`)。App Store の版に入れるときは、表示と `PrivacyInfo.xcprivacy` に上の種類を足してから出す(`PrivacyInfo.xcprivacy` はどの版にも入るので、今は変えていない)。

## #367 で決めること

- 同意の範囲に「改善のために AI が読む」を含めるか(含めるなら同意文の版 `FeedbackConsent.version` を上げ、既存のフィードバックは AI に回さない)。
- AI に回す前の伏せ字(メール・電話・名前・金額)と、スクショを AI に見せるか。
- Issue に載せてよい範囲(原文は載せず、要約と再現の手順だけ)。
- 保存期間 180 日のままでよいか(スクショだけ短くする案もある)。
