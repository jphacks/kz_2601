# ストアの掲載文の案

#247。App Store Connect に入れる名前・サブタイトル・プロモーション用テキスト・説明文・キーワード・スクリーンショットの文言。

- 方針: 「アプリを作る」「コードを実行する」と読める言い方をしない。言うのは「**自分専用の記録帳・ツール**を、話して用意して、話して直す」。理由はガイドライン 2.5.2(コードのダウンロード)・4.7(アプリ内のソフトウェア)・2.5.19(書かれていない機能)と、2026 年に「アプリビルダー」「App Store に提出」という売り方が問題にされた前例(調査メモ [P1]〜[P3])。
- 審査用の版(#284)で外した機能(共有リンク・探す・リリース申請・HealthKit・道のりの記録・近くの端末・撮影)は**書かない**。書いた機能は審査用の版に必ずある。
- 避ける言い回しの一覧は [wording.md](wording.md)。審査メモは [app-review-notes.md](app-review-notes.md)。
- 文字数は App Store Connect の上限(名前・サブタイトル 30、プロモーション用テキスト 170、キーワード 100、説明文 4000)に収まることを確かめた。

## 目次

1. [日本語](#1-日本語)
2. [英語](#2-英語)
3. [スクリーンショット](#3-スクリーンショット)
4. [カテゴリと年齢](#4-カテゴリと年齢)
5. [出す前の確認](#5-出す前の確認)

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
・写真ライブラリの写真を添えて記録
・今いる場所を記録して、地図で見返す
・いくつもの記録帳を切り替えて使う

■ プライバシー
・声の書き起こしは、この端末の中で行います
・画面を用意するために、話した内容などを AI(Anthropic)に送ります。送る前に説明を表示し、同意をいただきます。記録の中身は送りません
・アカウントは設定からいつでも削除できます

■ Tailor It Pro(月額・自動更新)
・無料では、新しい記録帳を 1 日 3 つまで用意できます。Pro ではこの上限がなくなります
・すでにある記録帳を話して直すのは、無料で使えます
・お支払いは Apple ID に請求されます。期間が終わる 24 時間前までに解約しない限り、自動で更新されます。解約は App Store の設定から行えます

利用規約: <URL>
プライバシーポリシー: <URL>
```

このバージョンの新機能(初版):

```text
最初のバージョンです。話して、自分専用の記録帳を用意して、話して直せます。
```

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
- Attach photos from your photo library
- Save the place you're at and see it on a map
- Keep several trackers and switch between them

PRIVACY
- Speech is transcribed on your device
- To set up your screen, what you say is sent to an AI (Anthropic). We explain this and ask for your consent first. The contents of your records are not sent
- You can delete your account at any time in Settings

TAILOR IT PRO (monthly, auto-renewing)
- Free: set up 3 new trackers per day. Pro removes this daily limit
- Adjusting your existing trackers by voice is free
- Payment is charged to your Apple ID. The subscription renews automatically unless cancelled at least 24 hours before the end of the period. Manage or cancel it in your App Store settings

Terms of Use: <URL>
Privacy Policy: <URL>
```

What's New (first version):

```text
Our first release. Set up a tracker made just for you by talking, and adjust it by talking.
```

## 3. スクリーンショット

文言は画像の上に入れる見出し。画面の中身(アプリの UI)にも [wording.md](wording.md) の言い回しが映り込まないようにする(下の注)。

| # | 画面 | 見出し(日本語) | Caption (English) |
|---|---|---|---|
| 1 | 声の画面で話している所 | 話すだけで、自分専用の記録帳 | Say it. Get a tracker made for you. |
| 2 | 「週のグラフも」と話した後 | 話せば、その場で形が変わる | Say "add a weekly chart". Done. |
| 3 | 合計・平均・連続日数のカード | 合計も連続日数も、自動で | Totals and streaks, automatically |
| 4 | 写真つきの記録・地図 | 写真や場所を添えて記録 | Add photos and places to your entries |
| 5 | 版の履歴 | 前の形にいつでも戻せる | Go back to any earlier version |

- 使う画面は審査用の版で撮る。外した機能(共有・探す・リリース申請・ヘルスケア・道のりの記録・撮影)を映さない(2.5.19・2.3)。
- 例は個人の記録(水分・筋トレ・買い物・家計簿)にする。他社のロゴや実在の人名を映さない(5.2.1)。
- 注: 今の UI には「どんなアプリが欲しいですか?」「新しいアプリを、いくつでも」など「アプリ」と言う文言がある。1 枚目に映るので、撮る前に UI の文言を変えるか、映らない画面を選ぶ([wording.md](wording.md) の「リポジトリの中で直す所」)。

## 4. カテゴリと年齢

| 項目 | 案 | 理由 |
|---|---|---|
| プライマリ | 仕事効率化(Productivity) | 記録・チェックリスト・リマインダーが中心。「開発ツール(Developer Tools)」にはしない |
| セカンダリ | ライフスタイル(Lifestyle) | 習慣・家計簿・買い物 |
| 年齢 | 審査用の版は他人とのやり取りや公開が無いので 4+ を目安に、App Store Connect の質問に正直に答える **(推測)** | 共有を戻す版では UGC(1.2)の質問の答えが変わる。調査メモの目安は 13+ |

## 5. 出す前の確認

- [ ] 名前・サブタイトル・説明文・キーワード・スクショの見出しに、[wording.md](wording.md) の「避ける言い回し」が無い
- [ ] 書いた機能が審査用の版にある(共有・探す・ヘルスケア・道のりの記録・撮影を書いていない)
- [ ] 無料の上限の数(3)が `ProProducts.dailyFreeCreations` と、#285 のサーバーの上限と合っている
- [ ] 利用規約・プライバシーポリシーの URL を埋めた(サポート URL・マーケティング URL も同じ言い方)
- [ ] 審査メモ([app-review-notes.md](app-review-notes.md))と説明文の中身が一致している
