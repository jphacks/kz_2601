# 仲間探しのアプリ(相互承認でつながる・#507)

「同じ趣味の人を探して、お互いに OK したら連絡できるアプリを作って」で、相互承認つきの仲間探しアプリを作れるようにする。この文書は仲間探しの設計の正典。phase 1 = backend・AI・`entity.match`、phase 2 = iOS の描画と部品(`list.item.interest`・`matchList`)。

## 目次
- [1. 対象範囲と理由](#1-対象範囲と理由)
- [2. 既存の仕組みとの関係](#2-既存の仕組みとの関係)
- [3. データモデル](#3-データモデル)
- [4. API](#4-api)
- [5. 見える範囲の規則](#5-見える範囲の規則)
- [6. 安全とプライバシー](#6-安全とプライバシー)
- [7. App Store と法律のメモ](#7-app-store-と法律のメモ)
- [8. AI](#8-ai)
- [9. DSL の部品(phase 2)](#9-dsl-の部品phase-2)
- [10. phase 2(iOS)の計画](#10-phase-2ios-の計画)
- [11. 人の判断が要ること](#11-人の判断が要ること)
- [12. 分かっている制約](#12-分かっている制約)

## 1. 対象範囲と理由

| 対象 | 例 |
|---|---|
| **作る** | 趣味の仲間探し(ボードゲーム・登山・楽器)・勉強仲間・同じイベントの参加者同士 |
| **作らない(AI が理由を添えて断る)** | 恋人・結婚相手・デートの相手・異性との出会いを探すもの(恋愛マッチング・婚活・出会い系・お見合い) |
| **普通に作る(断らない)** | 恋人・夫婦・家族の間で使う記録(デートの記録・記念日・二人の家計簿)。出会いの仕組みではない |
| **match を使わない** | 中学生・高校生の仲間探しのように 18 歳未満の人を知らない人と引き合わせるもの → 知っている人だけの `shared`(シェアリンクで招待)にする |

恋愛・出会いを対象外にする理由:
- **出会い系サイト規制法**: 異性との出会いを目的に、知らない人同士が連絡を取り合える場を提供すると「インターネット異性紹介事業」に当たり、公安委員会への届出・利用者が 18 歳以上かの確認などの義務がかかる可能性がある。ユーザーが作ったアプリでも、場を提供するのは私たち。
- **App Store**: 出会い系は年齢区分が上がり(17+ / 18+)、ガイドライン 1.2(ユーザー生成コンテンツ)・4.3(似たアプリが多い分野)で厳しく見られる。本体の年齢区分(今は 13+)に影響させない。

## 2. 既存の仕組みとの関係

できるだけ今ある共有の仕組みに載せる。新しく足すのは「誰が誰に送ったか」と「見える範囲の絞り込み」だけ。

| 使うもの | 既存 | 仲間探しでの使い方 |
|---|---|---|
| 共有アプリ | `apps_public/{appId}`(公開 DSL)・`scope: public / shared`(#47 #254) | プロフィールと連絡先は共有の entity のレコード。置き場も API も records のまま |
| 参加者 | `apps_public/{appId}/members`(#254) | プロフィールが `shared` なら、作成者とリンクで参加した人だけが探せる・送れる(「同じ大学の人だけ」) |
| 投稿者 | レコードの `author` / `authorName`・`$author`(#250) | 名前の入力欄は作らず、設定の表示名を出す。「つながりたい」の宛先はプロフィールの `author` |
| 通報・ブロック | `POST /v1/reports`・`/v1/me/blocks`(#321) | プロフィールの通報は既存の `kind: record`。ブロックは**どちら向きでも**お互いを隠す(下) |
| 運営の非公開化 | `apps_public/{appId}.hidden`(#321) | 非公開のアプリでは仲間探しの API も 403 `unpublished` |
| 退会の消去 | `cmd/purge`(#458) | 本人を含むペアと、本人のプロフィール・連絡先を消す(下) |

## 3. データモデル

### 3.1 DSL: `entity.match`

| 値 | 役割 | 制約(`backend/pkg/dsl/matching.go` の `Validate`) |
|---|---|---|
| `profile` | みんなが見て「つながりたい」を送る 1 人 1 件のプロフィール | scope public / shared・アプリに 1 つまで・location / photo / video の field は不可・`edit` は書かない |
| `matched` | 書いた本人と、つながった相手だけが読める(連絡先・次の相談) | scope public / shared・profile の entity が要る・location の field は不可・`edit` は書かない |

例(`backend/pkg/dsl/testdata/matching.json`): プロフィール = 好きなゲーム・地域(enum: 石川県 / 富山県 / 福井県 / オンライン)・ひとこと。連絡先 = 連絡方法。

### 3.2 appconfig: ペア

```
apps_public/{appId}/match_pairs/{pairId}   pairId = sha256(小さい uid, 大きい uid) の先頭 16 byte
  uids        [uidA, uidB](並べ替え済み。array-contains で引く: 単一フィールドの自動インデックス)
  from        最初に送った人
  status      pending | matched | declined
  declinedBy  見送った・取り下げた・解除した人(ブロックされた送信では相手)
  profiles    {uid: その人のプロフィールのレコード id}
  createdAt / updatedAt / matchedAt
match_quota/{uid}   {day, n}  送った回数(JST の 1 日ごと)
```

1 組 1 文書。状態はトランザクションで変える(`store.Matching.ChangeMatchPair`)。

| いま | 操作 | 次 |
|---|---|---|
| (無し) | A が送る | pending(from A) |
| pending(from B) | A が送る / A が承認 | **matched** |
| pending(from A) | A が送り直す | 変えない |
| pending(from B) | A が見送る | declined(by A)。**B には pending のまま見える** |
| pending(from A) | A が取り下げる | declined(by A) |
| declined(by A) | A が送る | pending(from A)(考え直し) |
| declined(by B) | A が送る | 変えない(B には届かない。A には pending と返す) |
| declined(from B・by B・未 matched) | A が送る | pending(from A)(B が取り下げた後) |
| matched | A が解除 | declined(by A)。B からの送り直しは届かない |
| (無し) | A が送るが B は A をブロック中 | declined(by B)。A には pending と返す |

## 4. API

`docs/backend/openapi-appconfig.yaml` の tag `matching`。実装は `backend/internal/appconfig/matching.go`。

| メソッド・パス | 内容 |
|---|---|
| `POST /v1/apps/{appId}/interests` | `{recordId, fromRecordId, ageConfirmed}`。相手のプロフィールに送る。相手が送っていれば承認になり `matched` |
| `GET /v1/apps/{appId}/interests?box=incoming\|outgoing` | 届いた / 送った未回答のもの |
| `POST /v1/apps/{appId}/interests/{uid}/accept` | `{ageConfirmed}`。届いたものを承認 → `matched` |
| `DELETE /v1/apps/{appId}/interests/{uid}` | 届いたものを見送る / 送ったものを取り下げる(冪等・相手に伝えない) |
| `GET /v1/apps/{appId}/matches` | つながった人 |
| `DELETE /v1/apps/{appId}/matches/{uid}` | つながりを解除(どちらからでも) |

共通: アプリの公開 DSL に `match:"profile"` が無ければ 404(`no_matching`)。プロフィールが `shared` なら参加者以外は 403 `not_member`。運営が非公開にしたアプリは 403 `unpublished`。

## 5. 見える範囲の規則

- **公開 DSL が正**: entity の役割は `apps_public/{appId}` の DSL(作成者のもの)で決める。各自のコピーの DSL を書き換えても、見える範囲は広がらない(`TestMatchingUsesPublishedDSL`)。
- `profile` のレコード: お互いにブロックしている人(どちら向きでも)の分は GET records に出ない・id を指定しても 404。
- `matched` のレコード: 本人と、つながった相手(ブロックしていない)の分だけ。それ以外は一覧に出ず、id を指定しても 404。
- 届いた・送った・つながったの一覧も、お互いにブロックしている人を出さない。
- 書き込み(作る・変える・消す)は既存の records と同じ(本人だけ。`edit:"members"` は使えない)。

## 6. 安全とプライバシー

| 項目 | 規則 | どこで守るか |
|---|---|---|
| 位置 | 正確な位置は持たない。地域は都道府県や「オンライン」までの enum | `Validate`(location の field 不可)・AI の規則 |
| 写真 | プロフィールに写真・動画を置かない(知らない人が見る) | `Validate` |
| 連絡先 | つながるまで見せない。連絡先は `matched` の entity にだけ書く | サーバーの絞り込み・AI の規則・lint(下)・ai-eval `matchSafe` |
| 年齢・性別・学校・本名 | プロフィールに入れない | AI の規則・lint(下)・ai-eval `matchSafe` |
| 18 歳未満 | 送る・承認するには本人の「18 歳以上」の確認(`ageConfirmed: true`)が要る。サーバーは申告したことだけを `users/{uid}.ageConfirmedAt` に記録する(`PUT /v1/me/age-confirmation`・送る / 承認する時の `true` でも記録)。申告の無い人には、interests・matches の一覧とプロフィールの投稿が 400 `age_unconfirmed`、プロフィール・連絡先の records は自分の分だけ。年齢そのものは保存しない。中高生の仲間探しは match を使わず招待制の shared にする | サーバー(400 `age_unconfirmed`)・AI の規則 |
| 断られたことを知らせない | 見送り・ブロックされた送信も、送った人には pending のまま | `pairView` |
| しつこい送信 | 見送られた相手への送り直しは届かない。1 人 1 日 30 回まで(送り直しも数える) | ペアの状態・`match_quota` |
| ブロック | #321 のブロックを両方向に効かせる | `BlockedEither` |
| 通報 | プロフィール・連絡先のレコードは既存の `POST /v1/reports`(`kind: record`)で通報できる | 既存 |
| 運営の非公開化 | 非公開のアプリでは仲間探しの API も止まる | `matchApp` |
| 退会 | 本人を含むペア・`match_quota/{uid}`・本人のプロフィールと連絡先のレコードを消す(「共有アプリの投稿は残す」の例外: 本人を表す情報のため) | `PurgeAccount`(ログ `matchPairs` / `matchRecords`)・[account-purge.md](../../infra/account-purge.md) |
| 自由文のメッセージ | 「つながりたい」に文章は付けない(嫌がらせの経路を作らない) | API |

### 6.1 プロフィールの項目の lint

`match:"profile"` の field が連絡先・本人を特定する情報に見えると、lint の警告を出す(`backend/pkg/dsl/matching_lint.go`)。警告なので保存はでき、AI は警告を受けて 1 回作り直す。ai-eval の `matchSafe` も同じ判定(`dsl.ProfileSensitiveField`)を使う。DSL に電話・メールの型は無いので、field の id と名前で見る(日本語は部分一致、英語は `_`・空白・camelCase で区切った単語で一致)。

| 種類 | 日本語 | 英語 | 直す先 |
|---|---|---|---|
| 電話 | 電話・携帯番号 | phone・tel・telephone | `matched` へ |
| メール | メール | email・e mail・mail | `matched` へ |
| 連絡先 | 連絡先 | contact | `matched` へ |
| LINE・SNS | インスタ・ツイッター | line・sns・twitter・instagram・insta・discord・facebook・tiktok・handle | `matched` へ |
| 住所 | 住所・番地・郵便番号 | address・postal・postcode・zipcode | `matched` へ |
| 年齢・誕生日 | 年齢・生年月日・誕生日(年齢層は除く) | age・birthday・birthdate・birth・dob(age range・age group は除く) | 消す |
| 学校 | 学校・高校・大学名 | school・university・college | 消す |
| 本名 | 本名・氏名・実名・フルネーム・名字・苗字 | full name・real name・first name・last name・family name・given name・surname | 消す |
| 性別 | 性別 | gender・sex | 消す |

警告しないもの: `computed`・boolean の field。`hidden` の field は警告する(`hidden` は画面に出さないだけで、値は全員の端末に届く)。enum は連絡先の種類では警告しない(「連絡方法: LINE / メール」は選択肢で、連絡先そのものではない)が、年齢・性別などは enum でも警告する。ニックネーム・名前・自己紹介・年齢層の好みは警告しない。

## 7. App Store と法律のメモ

- 恋愛・出会いを作らないので、出会い系サイト規制法の届出・年齢確認の対象にしない方針。ただし**最終的な判断は人**(下の 11)。
- App Store の年齢区分: 知らない人同士が連絡先を交換できる機能は、年齢区分の質問票の回答に影響しうる。13+ を維持できるかは公開前に App Store Connect の質問票で確かめる(人の作業)。
- ガイドライン 1.2: 通報・ブロック・運営の非公開化(#321)が前提。仲間探しはその上に載せる。審査用の版(`APP_STORE_REVIEW`)は共有を外しているので、仲間探しも出ない(`sharedData` を外す版は `match` も外す)。
- App Privacy: プロフィール・連絡先は「ユーザーが入力したその他の内容」。他の利用者に見える。ラベルの更新が要るかは人が確かめる。

## 8. AI

`backend/internal/ai/matching_rules.go`(`init()` で足す)。

| 規則 | 中身 | 出す版 |
|---|---|---|
| `matchingRules` | プロフィール(match profile・地域は大まか・写真/位置/連絡先/年齢/性別なし)と連絡先(match matched)の 2 entity、一覧とフォーム。いいね用の entity・承認の boolean は作らない | `supportedFeatures` に `matching` を送る版と、何も送らない開発版(ai-eval) |
| `datingRules` | 恋愛・出会い目的は作らない。断りの一文を `app.description` / `summary` に書く。恋人同士の記録は作る。中高生の仲間探しは招待制の shared | **全部の版** |
| `datingChatRule` | チャットは ready にせず、理由と「趣味の仲間探し」「勉強仲間を探す」を案内 | 全部の版 |
| 省略の点検 `dating` | 依頼が恋愛・出会い目的(`datingAskRe`、「恋愛目的じゃない」は除く)なら、断りの一文が無ければサーバーが足す | 全部の版 |

- 断りの一文: 「恋愛や出会いを目的に知らない人同士を引き合わせることはできません(出会い系サイト規制法の届出や年齢確認が必要になるため)。趣味・勉強・イベントの仲間探しなら作れます。」
- `allowedComponents` を送るのに `matching` が無い版(今の iOS 全部): `restrict.go` が生成スキーマから `Entity.match` を外し、規則を落とし、生成結果に新しく付いた match は作り直させ、残れば取り除く(#482 の `retentionDays` と同じ)。
- ai-eval: `internal/ai/eval/matching_cases.json`(作成 9 件。「恋愛目的じゃない」仲間探し・中学生どうし = match を使わず shared も)・`matching_chat_cases.json`(チャット 3 件)。expect は `matchProfile` / `matchMatched` / `matchSafe` / `matchUI`(`cmd/ai-eval/matching.go`)。断るケース(`matchProfile: false` + `noOmission`)が `datingAsked` に当たり、仲間探しのケースが当たらないことはモデル無しで確かめる(`TestMatchingCasesAgreeWithDatingAsked`)。

## 9. DSL の部品(phase 2)

iOS の描画と同じ PR で schema に足した(iOS の `ComponentRegistryTests` が「schema にある type が iOS に無い」を赤にするため・README §4)。

| 部品 | 形 | 役割 |
|---|---|---|
| `list.item.interest`(省略できるキー) | `true` | `match:"profile"` の一覧の行に「つながりたい」ボタン(自分の行には出さない・送った後は「送りました」)。初回は 18 歳以上の確認を出す |
| `matchList`(新しい Leaf) | `{entity, show: incoming \| outgoing \| matches, title?, emptyText?, item?: {title, subtitle, trailing, icon}, onItemTap?}` | `incoming` は承認 / 見送る、`outgoing` は取り下げ、`matches` は長押しで解除。どの行も長押しで通報・ブロック。行はプロフィールのレコード(`profileRecordId`)で描く |

- Action は足さない(README §3.2: `ActionType` を足すとホストの改修が要る)。送る・承認は部品の中で完結する(`shareButton` などと同じ)。
- 古い iOS: 未知のキー(`match`・`item.interest`)は無視、未知の type(`matchList`)はプレースホルダ(RELEASE は何も描かない)で落ちない(README §1)。backend は `allowedComponents` に無い部品・`matching` の無い版に match を出さない。

## 10. phase 2(iOS)

実装の場所: 部品 `MetamonRenderer` の `MatchListLeaf`・`InterestButton`・`Host/Capabilities/MatchingHost`、API と楽観的な状態 `MetamonServices` の `HTTPMatchingClient`・`MatchingStore`・`MatchingAgeGate`、ホスト `Metamon/Features/Matching/AppRuntime+Matching.swift`(18 歳以上の確認のシート・失敗の理由)。全部 `#if !APP_STORE_REVIEW`。


1. schema に `list.item.interest` と `matchList` を足す(`dsl.go` の定数・`validate.go`・例)。iOS `ComponentType` / `ComponentRegistry` / Leaf の View を同じ PR で。
2. iOS のサービス層に matching の API クライアント(上の 6 本)。楽観的に反映し、失敗したら戻す。オフラインでは送れない。
3. 18 歳以上の確認(初回だけ・端末に覚える)。断る・解除・通報・ブロックのメニュー。
4. iOS が `supportedFeatures` に `matching` を足す(足した版から AI が仲間探しを作る)。AI の規則に部品の使い方を足す。
5. 審査用の版で外れること(`build-review.sh`)を確かめる。
6. 実機で、2 台(2 アカウント)で 送る → 承認 → 連絡先が見える → 解除 → 見えない を確かめる(人の作業・未実施)。

1〜5 は済み。18 歳以上の確認は `UserDefaults` の `matching.ageConfirmed.v1`(真偽だけ)。「18 歳未満です」は覚えない(次に押した時もまた聞く)。サーバーが 400 `age_unconfirmed` を返したら覚えた確認を消して聞き直す。

## 11. 人の判断が要ること

- 対象範囲(恋愛・出会いは対象外・趣味/勉強/イベントは対象)を正典 `docs/00_decisions.md` の判断ログに載せるか。
- 年齢: 「18 歳以上の自己申告」で足りるか(13+ のまま 13〜17 歳は仲間探しを使えない、になる)。年齢確認をしない方針でよいか。
- 出会い系サイト規制法に当たらないことの最終確認(必要なら専門家)。
- プライバシーポリシー §1.6(共有・公開)に仲間探し(プロフィール・連絡先はつながった相手に見える・退会でプロフィールと連絡先は消える)を書き足すこと。利用規約の禁止事項(恋愛・出会い目的の利用)を足すこと。App Privacy・年齢区分の質問票の見直し。
- 運営の通報対応(24 時間の当番)は #321 の未決と同じ。

## 12. 分かっている制約

- 退会の消去は `users/{uid}/apps` にあるアプリをたどる。アプリだけ消した(DELETE /apps)人のペアは残る(相手からはつながりが見え続ける)。collection group のインデックスを足すか、アプリの削除でもペアを消すかは phase 2 で決める。
- プロフィールの一覧は、書いた人ごとに「その人が自分をブロックしているか」を読む(最大 2000 件・300 件ずつ)。人数が増えたら逆向きの索引を考える。
- プロフィールの 1 人 1 件はサーバーで強制していない(iOS が自分のプロフィールを編集する画面にする)。
