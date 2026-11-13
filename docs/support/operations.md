# 問い合わせの返答の運用

#320。support@tailor-it.app に来た問い合わせを、誰が・いつまでに・どう返すか。FAQ の原稿は [faq.md](faq.md)。

## 目次

1. [窓口](#1-窓口)
2. [返答の目安と担当](#2-返答の目安と担当)
3. [仕分け](#3-仕分け)
4. [不具合を Issue にする](#4-不具合を-issue-にする)
5. [返金](#5-返金)
6. [データの削除の依頼](#6-データの削除の依頼)
7. [通報・悪用](#7-通報悪用)
8. [返信のテンプレ](#8-返信のテンプレ)
9. [人の作業](#9-人の作業)

## 1. 窓口

| 入口 | 行き先 |
|---|---|
| メール | support@tailor-it.app |
| 公式サイト | https://tailor-it.app/contact(App Store のサポート URL)・FAQ は https://tailor-it.app/faq(公開は [faq.md](faq.md) 1 章) |
| アプリ | 設定の「お問い合わせ」(`/contact` を開く)・自動の動きの停止の案内の「お問い合わせ」・「通報する」・「フィードバックを送る」(利用できる版のみ) |

- 設定の「詳細(サポート用)」>「詳細をコピー」で、利用者は版・接続先などの識別子を送れる。調べる時に最初に頼む。
- 利用者の個人情報(メールアドレス・識別子)は Issue・PR・チャットに貼らない。Issue には「問い合わせ #日付-連番」のような手元の番号だけを書く。

## 2. 返答の目安と担当

| 項目 | 内容 |
|---|---|
| 最初の返信 | **2 営業日以内(提案)**。土日・祝日は数えない。まだ答えが出なくても「受け取りました・調べています」を返す |
| 通報・悪用・個人情報の漏れ | 営業日に関係なく、気づいたら先に止める(7 章)。返信は止めた後 |
| 担当 | 受信箱を見る人 1 人(主)と、休みの時の代わり 1 人。誰にするかは人が決める(9 章) |
| 見る頻度 | 営業日に 1 回以上(提案) |
| 閉じる | 利用者に答えた・Issue にした・7 日返事が無い、のどれかで閉じる(提案) |

## 3. 仕分け

受信箱のラベル(メールのラベル・フォルダ)。1 通に 1 つの種類と、今の状態を付ける。

| 種類 | 例 | 行き先 |
|---|---|---|
| `使い方` | 変え方・元に戻し方・共有のしかた | FAQ のリンクで返す(テンプレ A) |
| `不具合` | 落ちる・用意できない・同期しない | 再現を聞いて Issue にする(4 章・テンプレ B) |
| `要望` | こんな部品がほしい | 礼を返し、`idea` の Issue にまとめる(テンプレ C) |
| `課金` | 解約・返金・購入が反映されない | 返金は Apple(5 章・テンプレ D)。反映されないのは「購入を復元」を案内し、直らなければ不具合 |
| `データ` | 削除・引き継ぎ・何を送っているか | 6 章・テンプレ E。送るものはプライバシーポリシーの範囲で答える |
| `通報` | 不適切な共有・通知・自動の動き | 7 章・テンプレ F |
| `その他` | 取材・協業など | 担当が個別に返す |

| 状態 | 意味 |
|---|---|
| `未返信` | 最初の返信がまだ |
| `調査中` | 返信済み・答え待ち |
| `Issue 済み` | GitHub の Issue にした(番号を手元に控える) |
| `完了` | 閉じた |

## 4. 不具合を Issue にする

1. 再現の手順・端末と iOS の版・「詳細をコピー」の内容・起きた日時を聞く(テンプレ B)。
2. 手元で再現を試す。サーバーの記録を見る時は、日時と識別子で探す([ops/incident-runbook.md](../ops/incident-runbook.md))。
3. GitHub に `bug` ラベルの Issue を作る。**利用者のメールアドレス・名前・記録の中身は書かない**(公開の repo)。識別子も、要るなら一部だけにする。
4. 多くの人に影響する・データが消える・お金に関わる時は、障害として扱う([ops/incident-runbook.md](../ops/incident-runbook.md) の SEV)。
5. 直って配信したら、利用者に返す。
6. 弱点(認証・ほかの人のデータが見える等)の報告は、公開の Issue にしない。手元で直しを準備し、公開の時期を人が決める。

## 5. 返金

- 支払いは Apple が処理する。運営者は返金できず、カード情報も持たない。返金は利用者が Apple に申し込む(https://reportaproblem.apple.com/)。テンプレ D。
- 返金されると Apple から知らせが来て、サーバーが Pro を止める([infra/app-store-notifications.md](../infra/app-store-notifications.md))。こちらの操作は要らない。
- 解約は iPhone の「設定」>「Apple ID」>「サブスクリプション」。アカウントを削除しても自動では解約されないことを必ず添える。
- 料金の値段は返信に書かない(購入画面を見てもらう)。

## 6. データの削除の依頼

- まずアプリの設定の「アカウントを削除」を案内する(テンプレ E)。サーバーのデータは論理削除の後、30 日以内に消える([infra/account-purge.md](../infra/account-purge.md)・プライバシーポリシー 4 章)。
- 端末を失くした・アプリに入れない時は、本人であることを確かめてから運営が消す。確かめ方(送ってもらう識別子など)は人が決める(9 章)。確かめられない依頼では消さない。
- 共有の記録帳にした投稿は、アカウントを消しても残る(プライバシーポリシー 4 章)。消したい投稿があれば、記録帳の作成者に頼むか、通報として扱う。

## 7. 通報・悪用

- 手順は [infra/moderation.md](../infra/moderation.md) の「通報を受けたら」。規約の禁止事項に当たるかで決める([legal/terms.md](../legal/terms.md) 5・6 章、方針は [legal/moderation-policy-draft.md](../legal/moderation-policy-draft.md))。
- 自動の動き・通知が絡む時は、先に止める([ops/incident-runbook.md 手順 F](../ops/incident-runbook.md#13-手順-f-悪用自動の動き))。
- 通報した人には「受け取った・確認する」まで返す。誰をどう処分したかは書かない(テンプレ F)。
- 運営が自動の動きを止めた作成者からの申し立て(アプリの停止の案内から届く)は、止めた理由を確かめて、再開するか・止めたままにするかを返す。

## 8. 返信のテンプレ

署名は「Tailor It サポート」/ "Tailor It Support"。`{}` は書き換える。

作れなかった依頼・データの書き出し・その他のテンプレと、不具合の時に聞く項目の表は [reply-templates.md](reply-templates.md)。

### A. 使い方(FAQ を案内)

```text
{お名前}様

お問い合わせありがとうございます。Tailor It サポートです。

{答えを 1〜2 文で}
くわしくは、よくある質問もご覧ください。
https://tailor-it.app/faq

ほかにも分からないことがあれば、このメールにご返信ください。

Tailor It サポート
```

```text
Hi {name},

Thanks for contacting Tailor It Support.

{Answer in 1-2 sentences}
You can also find more in our FAQ:
https://tailor-it.app/faq

If you have any other questions, just reply to this email.

Tailor It Support
```

### B. 不具合(くわしく聞く)

```text
{お名前}様

不具合のご連絡ありがとうございます。ご不便をおかけしています。
調べるために、次を教えていただけますか。

1. 何をした時に起きたか(手順)
2. 起きた日時
3. iPhone の機種と iOS の版
4. 設定 >「詳細(サポート用)」>「詳細をコピー」で写した内容

分かり次第、ご連絡します。

Tailor It サポート
```

```text
Hi {name},

Thanks for letting us know, and sorry for the trouble.
To look into it, could you send us:

1. What you did when it happened (steps)
2. When it happened (date and time)
3. Your iPhone model and iOS version
4. The text from Settings > "Details (for support)" > "Copy details"

We'll get back to you as soon as we know more.

Tailor It Support
```

### C. 要望

```text
{お名前}様

ご要望ありがとうございます。いただいた内容は、開発のメンバーで共有しました。
すぐにお応えできるとは限りませんが、今後の改善の参考にします。

Tailor It サポート
```

```text
Hi {name},

Thanks for the suggestion. We've shared it with the team.
We can't promise when or whether it will be added, but it helps us decide what to improve.

Tailor It Support
```

### D. 返金・解約

```text
{お名前}様

お問い合わせありがとうございます。
Tailor It の支払いは Apple が処理しているため、返金は Apple への申し込みになります。
次のページから申し込めます。
https://reportaproblem.apple.com/

解約は、iPhone の「設定」>「Apple ID」>「サブスクリプション」から行えます。
アカウントを削除しても、購読は自動では解約されませんのでご注意ください。

Tailor It サポート
```

```text
Hi {name},

Thanks for reaching out.
Payments for Tailor It are handled by Apple, so refunds are requested from Apple here:
https://reportaproblem.apple.com/

To cancel, go to iPhone Settings > Apple ID > Subscriptions.
Please note that deleting your account does not cancel your subscription.

Tailor It Support
```

### E. データの削除

```text
{お名前}様

お問い合わせありがとうございます。
アカウントとデータは、アプリの設定の「アカウントを削除」から消せます。
サーバーのデータは、削除から 30 日以内に消去します。
共有の記録帳に入れた記録は、ほかの参加者のために残ります。

アプリを開けない場合は、このメールにご返信ください。ご本人の確認の上で対応します。

Tailor It サポート
```

```text
Hi {name},

Thanks for reaching out.
You can delete your account and data from "Delete Account" in the app's Settings.
Data on our server is erased within 30 days of deletion.
Records you added to shared trackers remain for the other participants.

If you can't open the app, please reply to this email and we'll help after confirming it's you.

Tailor It Support
```

### F. 通報の受け取り

```text
{お名前}様

お知らせいただき、ありがとうございます。
内容を確認し、利用規約に沿って対応します。
対応の内容はお伝えできないことがありますが、ご了承ください。

Tailor It サポート
```

```text
Hi {name},

Thank you for the report.
We will review it and take action under our Terms of Use.
We may not be able to share the details of the action we take.

Tailor It Support
```

## 9. 人の作業

- [ ] support@tailor-it.app が受け取れるか確かめる(外のアドレスからテストを送り、受信と返信ができる)。
- [ ] 主の担当と代わりの担当を決める。
- [ ] 最初の返信の目安(2 営業日・提案)と見る頻度を決める。決まったら 2 章の「提案」を外す。
- [ ] 本人確認のしかた(6 章)を決める。
- [ ] 受信箱に 3 章のラベルを作る。
- [ ] 公式サイトに `/faq` を公開し、`/contact` からリンクする([faq.md](faq.md) 1 章)。
- [ ] テストの問い合わせを送り、テンプレで返して流れを確かめる(#320 の完了の条件)。
