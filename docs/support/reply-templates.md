# 問い合わせの返答のテンプレート

#2309(親 #320)。support@tailor-it.app への返信の文(日本語・英語)。誰が・いつまでに返すか、仕分け、Issue にする手順は [operations.md](operations.md)、FAQ の原稿は [faq.md](faq.md)。

- 書き方: 新しい約束をしない(返事の速さ・直る時期・返金・特別な扱い)。手続きは今の文書にある手順をそのまま案内する。値段は書かない。
- 規約・プライバシーポリシーと食い違ったら、規約・ポリシーが正しい([legal/terms.md](../legal/terms.md)・[legal/privacy-policy.md](../legal/privacy-policy.md))。
- 署名は「Tailor It サポート」/ "Tailor It Support"。`{}` は書き換える。利用者のメールアドレス・記録の中身は Issue・PR・チャットに貼らない([operations.md 1 章](operations.md#1-窓口))。

## 目次

1. [種類とテンプレの対応](#1-種類とテンプレの対応)
2. [不具合の時に聞くこと](#2-不具合の時に聞くこと)
3. [作れなかった依頼](#3-作れなかった依頼)
4. [データの書き出し](#4-データの書き出し)
5. [その他](#5-その他)
6. [決めること](#6-決めること)

## 1. 種類とテンプレの対応

使い方・不具合・要望・返金 / 解約・データの削除・通報の 6 つは、[operations.md 8 章](operations.md#8-返信のテンプレ) の A〜F にある。ここには残りの種類を足す。同じ文を 2 か所に持たない。

| 種類([operations.md 3 章](operations.md#3-仕分け) のラベル) | テンプレ | 手続きの元の文書 |
|---|---|---|
| `使い方` | operations.md 8 章 A | [faq.md](faq.md) |
| `不具合` | operations.md 8 章 B(聞く項目は下の 2 章) | [operations.md 4 章](operations.md#4-不具合を-issue-にする) |
| `不具合`(用意できない・形が違う) | 下の 3 章 | [ios/architecture.md のエラー状態表](../ios/architecture.md#エラー状態表602026-10-24-深夜)・[legal/terms.md 2 章・5 章](../legal/terms.md#5-禁止事項) |
| `要望` | operations.md 8 章 C | — |
| `課金`(返金・解約) | operations.md 8 章 D | [operations.md 5 章](operations.md#5-返金)・[legal/terms.md 4 章](../legal/terms.md#4-料金) |
| `データ`(削除) | operations.md 8 章 E | [operations.md 6 章](operations.md#6-データの削除の依頼)・[legal/privacy-policy.md 4 章](../legal/privacy-policy.md#4-アカウントと情報の削除)・[backend/features/account-deletion.md](../backend/features/account-deletion.md) |
| `データ`(書き出し) | 下の 4 章 | [dsl/README.md の `export`](../dsl/README.md) |
| `通報` | operations.md 8 章 F | [operations.md 7 章](operations.md#7-通報悪用) |
| `その他` | 下の 5 章 | — |

## 2. 不具合の時に聞くこと

テンプレ B で聞く項目。調べる人がそろっているかをこの表で確かめる。

| 項目 | 何に使うか | 利用者の取り方 |
|---|---|---|
| 何をした時に起きたか(手順) | 手元で再現する | 文で書いてもらう |
| 起きた日時(だいたいでよい) | サーバーの記録を時刻で探す | — |
| 端末の機種・OS の版 | 機種・版だけの不具合か分ける | iPhone の「設定」>「一般」>「情報」 |
| 「詳細をコピー」の内容 | アプリの版(`バージョン`・ビルド番号)・今の記録帳の版・接続先・ユーザー ID・音声モデル | 設定 >「詳細(サポート用)」>「詳細をコピー」(`ios/Shared/Settings/SupportInfo.swift`。Mac は音声モデル・接続先の行が無い) |
| 画面の写真(あれば) | 出た文言を見る | 任意。記録の中身が写る時は送らなくてよいと伝える |

- ユーザー ID は Issue に全部は書かない。要るなら一部だけ([operations.md 4 章](operations.md#4-不具合を-issue-にする))。
- 安全に関わる不具合(ほかの人のデータが見える等)は公開の Issue にしない(operations.md 4 章の 6)。

## 3. 作れなかった依頼

話した・書いた依頼で記録帳が用意できなかった、または思った形にならなかった時。アプリが出した文言(「うまく伝わりませんでした」「うまく作れませんでした」「混み合っています」など)を聞くと、どの失敗か分けられる([ios/architecture.md のエラー状態表](../ios/architecture.md#エラー状態表602026-10-24-深夜))。規約で用意できない内容(禁止事項)だった時は 3.2 を使う。

### 3.1 うまく作れなかった

```text
{お名前}様

お問い合わせありがとうございます。Tailor It サポートです。
ご希望の形にならず、ご不便をおかけしました。

次のことを試していただけますか。
- 1 回で全部を言わず、「まず〇〇を記録したい」のように小さく分けて話す
- できた後に「〇〇も足して」「〇〇を大きく」と少しずつ直す
- 「混み合っています」と出た時は、少し時間をおいてもう一度

それでも難しい時は、次を教えてください。調べます。
1. 話した(書いた)依頼の文
2. 画面に出た文言
3. 設定 >「詳細(サポート用)」>「詳細をコピー」で写した内容

いただいた内容は、改善の参考にします。

Tailor It サポート
```

```text
Hi {name},

Thanks for contacting Tailor It Support, and sorry it didn't turn out as you hoped.

Could you try the following?
- Start small, like "I want to track X", instead of describing everything at once
- Then adjust step by step, like "also add X" or "make X bigger"
- If you see "busy", wait a moment and try again

If it still doesn't work, please send us:
1. What you said (or typed)
2. The message shown on screen
3. The text from Settings > "Details (for support)" > "Copy details"

We'll use it to improve the app.

Tailor It Support
```

### 3.2 規約で用意できない内容だった

禁止事項は [legal/terms.md 5 章](../legal/terms.md#5-禁止事項)。どの条に当たるかの判断の中身は書かない。

```text
{お名前}様

お問い合わせありがとうございます。
いただいた内容は、利用規約の定めにより、Tailor It で用意・共有できないものでした。
ご理解いただけますと幸いです。

利用規約: https://tailor-it.app/terms

Tailor It サポート
```

```text
Hi {name},

Thanks for reaching out.
Under our Terms of Use, this kind of content can't be set up or shared in Tailor It.
Thank you for your understanding.

Terms of Use: https://tailor-it.app/terms

Tailor It Support
```

## 4. データの書き出し

今のアプリには、アカウントの記録をまとめて書き出す機能は無い。記録帳の中に書き出し(CSV・PDF)のボタンを置くことはできる([dsl/README.md](../dsl/README.md) の `export`)。アカウント全部の写しを運営が作って渡すかは決まっていない(6 章)。決まるまでは、下の文で記録帳ごとの書き出しを案内し、それで足りない依頼は担当がオーナーに回す。

```text
{お名前}様

お問い合わせありがとうございます。
記録は、記録帳ごとに CSV か PDF で書き出せます。
記録帳を開いて「書き出しのボタンを付けて」と話すと、ボタンが付きます。
押すと、ファイル・メールなどに保存・送信できます(写真・動画は入りません)。

ほかの形での書き出しが必要な場合は、このメールにご返信ください。

Tailor It サポート
```

```text
Hi {name},

Thanks for reaching out.
You can export records from each tracker as CSV or PDF.
Open the tracker and say "add an export button" to add one.
Tapping it lets you save or send the file (photos and videos are not included).

If you need your data in another form, just reply to this email.

Tailor It Support
```

- 「話すとボタンが付く」は AI の生成に頼る(**推測**: たいてい付くが、必ずではない)。付かなかった時は 3.1 のとおり聞く。

## 5. その他

取材・協業・どれにも当たらない問い合わせ。受け取ったことだけを返し、中身は担当が個別に返す([operations.md 3 章](operations.md#3-仕分け))。

```text
{お名前}様

お問い合わせありがとうございます。Tailor It サポートです。
内容を確認し、担当からあらためてご連絡します。

Tailor It サポート
```

```text
Hi {name},

Thanks for contacting Tailor It Support.
We've received your message and the right person will get back to you.

Tailor It Support
```

## 6. 決めること

| 項目 | 今 | 決める人 |
|---|---|---|
| アカウントの記録をまとめて書き出してほしい、という依頼に応じるか | 機能は無い。応じるなら手順(本人の確かめ方・渡し方)を operations.md に書いてからテンプレを足す | オーナー |
| 3.2 の文で、どの条に当たったかを伝えるか | 伝えない(テンプレ F と同じ考え) | オーナー |
| 返事の目安を返信の文に書くか | 書かない。目安は operations.md 2 章で「提案」のまま | オーナー |
