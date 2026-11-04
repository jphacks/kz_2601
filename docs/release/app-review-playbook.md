# 審査の提出と、却下された時の対応(App Review の手順書)

#332(ロードマップの R-46)。最初の版を App Store の審査に出す前の点検と、却下された時の返し方をまとめる。

- 版を作る手順は [process.md](process.md) の「6. チェックリスト: iOS を出す」、審査用の版の中身は [../ios/review-build.md](../ios/review-build.md)、審査メモの本文は [../store/app-review-notes.md](../store/app-review-notes.md)、掲載物・年齢区分・栄養ラベルの答えは [../store/listing.md](../store/listing.md)、人の作業の一覧は [m0-early-release-readiness.md](m0-early-release-readiness.md) の 5 章。ここでは**中身を写さず、確かめる場所だけを指す**。
- 前提: 最初の版は審査用の版(`Metamon-Review` / `APP_STORE_REVIEW`)で、App 内課金なし・共有 / 公開のデータなし(2026-10-30 決定・#450 #451)。
- ガイドラインの番号と文は、2026-11-03 に [App Review Guidelines](https://developer.apple.com/app-store/review/guidelines/) で確かめた。段階的なリリースは [Apple のヘルプ](https://developer.apple.com/help/app-store-connect/update-your-app/release-a-version-update-in-phases) で確かめた。App Store Connect(ASC)の画面名は日本語の表示に合わせたが、変わることがある。
- 法律・審査の結果を保証するものではない。推測には **(推測)** と書く。

## 目次

1. [流れ](#1-流れ)
2. [提出前の点検表](#2-提出前の点検表)
3. [審査メモに入れる説明](#3-審査メモに入れる説明)
4. [却下された時の対応](#4-却下された時の対応)
5. [段階的なリリース(7 日)](#5-段階的なリリース7-日)
6. [判断してほしいこと](#6-判断してほしいこと)
7. [ガイドラインの要旨(確かめた文)](#7-ガイドラインの要旨確かめた文)

## 1. 流れ

1. [process.md](process.md) の 6 章の 1〜6(release ブランチ・審査用の版のアーカイブ・TestFlight)。
2. この文書の 2 章の点検表を上から全部埋める。1 つでも「未」があれば出さない。
3. **【人】** ASC で審査に提出する(process.md の 6 章の 7)。
4. 結果を待つ。審査の期間中は prod の ai / appconfig を止めない(2.1(a)「back-end service を動かしておく」)。
5. 通ったら 5 章の公開のしかたで出す。却下なら 4 章。

提出・返信・公開は、すべて ASC のアカウントを持つ人が行う(process.md の 8 章)。エージェントは下書きと点検だけをする。

## 2. 提出前の点検表

「どこで確かめるか」のファイルは repo の中、画面は ASC か Apple Developer の画面。

### 2.1 削った機能(#245)

| 項目 | どこで確かめるか | 済 |
|---|---|---|
| 外した機能(リリース申請・探す・共有リンク・HealthKit・背面の位置・センサー・近くの端末・撮影・ドラレコ・録音・webView・課金・共有 / 公開のデータ・chatThread・仲間探し・フィードバック・β の案内)がバイナリに無い | [review-build.md](../ios/review-build.md) の「外れたことの確かめ方」をアーカイブした `.app` で実行 | [ ] |
| 外した機能が AI の生成にも出ない | 同「AI 側の制限」。`ReviewBuildTests`(`-DAPP_STORE_REVIEW`)が通っている | [ ] |
| 審査の後でオンにするフラグが無い(2.5.19) | 同「部品・機能を足す / 戻すとき」。戻す時は新しい版で出す(#334) | [ ] |
| 接続先が prod | 同「外れたことの確かめ方」の `MetamonAIBaseURL` の行 | [ ] |

### 2.2 同意とポリシー(#246)

| 項目 | どこで確かめるか | 済 |
|---|---|---|
| プライバシーポリシーと利用規約の [要確認] が埋まり、公開 URL で開ける | `docs/legal/privacy-policy.md`・`docs/legal/terms.md`(ここでは直さない)、ブラウザで https://tailor-it.app/privacy ・ /terms | [ ] |
| アプリの設定と初回から、ポリシー・規約を開ける | TestFlight の版で 設定 > プライバシー | [ ] |
| AI に送る前の同意画面が出て、設定から取り消せる(5.1.2(i)・5.1.1(ii)) | TestFlight の版で初回起動。文言は `AIConsentView.swift` | [ ] |
| アカウントの削除と Sign in with Apple のトークンの失効(5.1.1(v)) | TestFlight で 設定 > アカウントを削除 → iPhone の 設定 > Apple ID > Apple でサインイン から Tailor It が消える。鍵の登録は `docs/legal/apple-token-revocation.md` | [ ] |
| 音声モデル(約 500MB)をダウンロード前に確認する | TestFlight の版でマイクを初めて押す | [ ] |

### 2.3 審査メモ(#247)

| 項目 | どこで確かめるか | 済 |
|---|---|---|
| 審査メモの 1 章の表がすべて満たされている | [app-review-notes.md](../store/app-review-notes.md) の 1 章 | [ ] |
| 英語の版を貼った・4,000 字以内 | ASC: App のバージョンのページ > App Review に関する情報 > メモ | [ ] |
| 連絡先(名前・メール・電話)を入れた | ASC: App Review に関する情報 > 連絡先情報(repo には書かない) | [ ] |
| サインイン情報の扱い | 3.2 のとおり | [ ] |
| 掲載文・スクショに「アプリを作る」の言い方が無い | [wording.md](../store/wording.md)・[listing.md](../store/listing.md) の 6 章 | [ ] |

### 2.4 #326〜#331

| Issue | 項目 | どこで確かめるか | 済 |
|---|---|---|---|
| #326 課金 | 最初の版は課金なし。ASC にサブスクリプションを登録していない・審査メモに IAP の段落が無い。**課金ありに変えるなら**、ペイウォールの表記(3.1.2)と商品を版と一緒に審査に出す | ASC: 収益化 > サブスクリプション / App 内課金、[review-build.md](../ios/review-build.md) の課金の行 | [ ] |
| #327 掲載物 | スクショ(審査用の版で撮る)・プレビュー動画・カテゴリ・キーワード・サブタイトルが入っている | ASC: App のバージョンのページ(プレビューとスクリーンショット)・App 情報(カテゴリ)、[listing.md](../store/listing.md) の 3・4 章 | [ ] |
| #328 栄養ラベル | 「App のプライバシー」の答えと `PrivacyInfo.xcprivacy`・ポリシーが一致。アップロード時にプライバシーの警告メールが来ていない | ASC: App のプライバシー、[listing.md](../store/listing.md) の 5 章、`ios/Metamon/PrivacyInfo.xcprivacy` | [ ] |
| #329 年齢区分 | 質問票に答え、4+ / 13+ を決めた(ポリシーの「13 歳未満は対象外」と合わせる) | ASC: App 情報 > 年齢制限指定、[listing.md](../store/listing.md) の 4 章 | [ ] |
| #330 法務 | 特定商取引法の表記の要否が決まった(課金なしの版では要らない見込み **(推測)**)・規約の有料部分は課金を入れる版まででよいか決めた | 公式サイト・#330 | [ ] |
| #331 公式サイト | サポート URL・マーケティング URL・プライバシーポリシー URL が開け、サポートのページに問い合わせ先がある(1.2「連絡先の公開」・5.1.1(i)) | ブラウザ、ASC: App 情報(プライバシーポリシー URL)・バージョンのページ(サポート URL) | [ ] |

### 2.5 輸出のコンプライアンス(暗号化の申告)

| 項目 | どこで確かめるか | 済 |
|---|---|---|
| `ITSAppUsesNonExemptEncryption` が `false` | `ios/Metamon/Resources/Info.plist`。アーカイブした `.app` でも `plutil -p .../Info.plist \| grep ITSApp` | [ ] |
| 申告の根拠が今も正しい: 通信は OS の HTTPS(URLSession・Firebase)だけ、独自の暗号化は持たない。Sign in with Apple の nonce の hash は認証のためで、データの暗号化ではない | 新しく暗号化のライブラリや独自の暗号化を足していないか(PR の差分)。足したら申告を見直す | [ ] |
| ビルドに「輸出コンプライアンスの情報がありません」が出ていない | ASC: TestFlight > ビルドの一覧 | [ ] |

注: 申告の正しさは法的な判断で、ここでは確かめていない **(推測)**。暗号化を足す版では、ASC の App 情報 > App 暗号化に関する書類 で見直す。

### 2.6 その他(2.1 の完全性)

| 項目 | どこで確かめるか | 済 |
|---|---|---|
| prod の ai / appconfig が動き、Claude の利用上限・Cloud Run の上限に余裕がある | `docs/ops/incident-runbook.md`・GCP のコンソール | [ ] |
| MusicKit が App ID で有効(無いなら掲載文と審査メモの Apple Music の行を消す) | Apple Developer: Identifiers > `jp.kz2601.metamon` > App Services | [ ] |
| 実機で主要な流れ(声で作る・直す・同意・削除・QR・道案内・Apple Music の検索)が落ちない | TestFlight の内部テスト([m0-early-release-readiness.md](m0-early-release-readiness.md) の 5 章) | [ ] |
| 版の公開のしかたを選んだ | 5 章・ASC: App のバージョンのページ > バージョンのリリース | [ ] |

## 3. 審査メモに入れる説明

### 3.1 DSL は実行コードではない

英語の本文は [app-review-notes.md](../store/app-review-notes.md) の 2 章 `HOW IT WORKS - NO DOWNLOADED CODE` に入っている(3,994 字で上限に近いので、ここから文を足さない)。言うことは次の 4 つで、返信(4 章)でも同じ順で言う。

1. 画面の部品は、すべてバイナリに入ったネイティブの SwiftUI 部品。
2. サーバーが返す JSON は「どの部品を出すか・文言・配置・記録の項目」を選ぶ設定のデータ。
3. 計算は、決まった演算だけを持つ組み込みの評価器が行う(繰り返し・独自の関数・通信・ファイルの読み書きは無い)。
4. 知らない部品は空の枠になるだけ。新しい部品は App Store の更新(審査)でしか増えない。

聞かれた時のために、**JSON の例**(同梱の例のうち審査用の版に残るもの、例: 水分の記録)と、声で作って直す 30 秒ほどの画面録画を用意しておく。ASC の App Review に関する情報 > 添付ファイル に入れられる **(推測: 欄の名前)**。

### 3.2 デモアカウント(置き場所と渡し方)

- サインインは「Sign in with Apple」と「ログインせずに使う(匿名)」だけで、匿名ですべての機能を試せる。審査メモの HOW TO TEST も匿名で書いている。そのため ASC の「サインインが必要」は**オフ**のままにする想定(2.1(a) はログインがある場合にデモアカウントを求める)**(推測)**。
- デモアカウントが要る場合(Apple に求められた・メールのログインを足した 等):
  - 置き場所: ASC の App Review に関する情報 > サインイン情報 **だけ**。
  - 元の値は、アカウントの持ち主の手元(repo の外の保管場所)にだけ置く。
  - 渡し方: 持ち主が自分で ASC に入れる。Issue・PR・チャット・審査メモ・この repo には書かない。
  - 審査の後もアカウントを消さず、パスワードを変えたら ASC の欄も直す。

## 4. 却下された時の対応

### 4.1 共通の手順

1. ASC の App Review のメッセージ(旧 Resolution Center)で、**ガイドラインの番号・指摘の文・添付のスクショ**を全部読む。番号が 4 章の表のどれかを決める。
2. 指摘が「説明が足りない」なのか「機能が問題」なのかを分ける。
   - 説明で足りる → 同じバイナリのまま返信する(4.2〜4.5 の下書き)。
   - 機能が問題 → 版を直す(外す・足す)→ ビルド番号を上げて出し直し、返信で何を変えたかを書く。
3. 同じ指摘で何度も却下されると審査が遅くなる(After You Submit の Timing)。説明だけで 2 回通らなければ、版を直すか申し立てる。
4. 結果に納得できない時は **申し立て(Appeal)** を出す。App Review Board が見る。
5. 既に配信している版の修正(バグ直し)は、法律・安全以外の指摘では止められない(Bug Fix Submissions)。最初の版には使えない。
6. 返信の文・何を直したかを、この文書の 4 章に追記する(次の版の審査メモに活かす)。

返信の原則:
- 英語で、指摘の番号ごとに答える。審査メモと**同じ事実**だけを書く(食い違うと 2.3.1)。
- 「審査の後で機能を戻す」とは書かない・しない(2.5.19)。
- 下書きの `<...>` は返信の時に埋める。

### 4.2 2.5.2(コードのダウンロード・実行)

| 想定される指摘 | 対応の案 |
|---|---|
| サーバーから受け取る内容で、アプリの機能が変わっている | 案 A(推奨・最初): 3.1 の 4 点で返信し、JSON の例と画面録画を添付する。<br>案 B: 指摘された部品・式を名指しされたら、その部品を審査用の版から外す(review-build.md の手順)。<br>案 C: 説明で 2 回通らなければ申し立て。前例は app-review-notes.md の 5 章 |

```text
Hello App Review team,

Thank you for the review. We would like to explain how Tailor It works regarding Guideline 2.5.2.

Tailor It does not download, install or execute code.
1. Every screen element is a native SwiftUI component that is compiled into the submitted binary.
2. Our server returns a JSON configuration created with an AI model. The JSON only selects which of these built-in components to show, with their labels, layout and record fields. It is data, not code.
3. Values such as totals and averages are computed by a fixed evaluator inside the binary, with a closed list of arithmetic and date operations. It has no loops, user-defined functions, network access or file access.
4. If the JSON names a component the binary does not know, the app shows an empty placeholder. New component types can only be added through an App Store update.

We have attached an example of the JSON and a short screen recording that shows a tracker being set up and adjusted by voice.

If a specific screen or behavior raised the concern, could you tell us which one? We will explain it or change it in a new build.

Best regards,
<name>
```

### 4.3 4.7(アプリの中で配るソフトウェア)

| 想定される指摘 | 対応の案 |
|---|---|
| 記録帳を「ミニアプリ / プラグイン」とみなす(4.7・4.7.1 の通報・ブロック、4.7.4 の索引、4.7.5 の年齢) | 案 A(推奨・最初): 記録帳は HTML5 / JavaScript のミニアプリではなく、本体の部品の組み合わせという個人のコンテンツ(1.2.1 の「コンテンツ」)だと返信する。外では動かず、書き出し・提出・共有もできない。<br>案 B: 4.7 として扱うと言われたら、満たす作り(索引・通報・年齢)の費用を見積もり、人が判断する。 |
| 端末の機能を記録帳に渡している(4.7.2・4.7.3) | 案 A: 権限は本体が、本体の UI で、使う時に求める。部品はネイティブで、JSON は置き場所を決めるだけ。HealthKit・背面の位置・近くの端末・撮影・録音は外したと返信する。<br>案 B: 名指しされた機能(例: 位置・Apple Music)を外した版を出す。掲載文と審査メモからもその行を消す |

```text
Hello App Review team,

Thank you for the review. We would like to explain why Tailor It does not offer software under Guideline 4.7.

- A tracker in Tailor It is not an HTML5 or JavaScript mini app, game, plug-in or other software. It is a personal arrangement of native components that are built into the binary, together with the user's own records. In our understanding, this is personal content inside the app (Guideline 1.2.1), not a separate app.
- A tracker cannot run outside Tailor It. It cannot be installed, exported, published or submitted anywhere. Only the user's own records can be exported, as a CSV or PDF file through the system share sheet.
- Tailor It does not expose native APIs to the content (Guidelines 4.7.2 and 4.7.3). Tailor It itself requests each permission (microphone, camera for QR codes, photos, location while in use, Apple Music, notifications) in its own UI when the user uses the feature. The JSON configuration only decides where a built-in component appears.
- This version does not include HealthKit, background location, nearby device communication, camera capture or audio recording.

If a specific feature raised the concern, please let us know which one. We can remove it in a new build.

Best regards,
<name>
```

### 4.4 1.2(ユーザーが作るコンテンツ)

1.2 は「AI でコンテンツを作るアプリ」も対象に含み、フィルタ・通報・ブロック・連絡先の公開を求めている。

| 想定される指摘 | 対応の案 |
|---|---|
| AI が作る内容に通報・ブロック・フィルタが無い | 案 A(推奨・最初): 作った物は本人だけが見る(共有・公開・チャットはこの版に無い)ので、他の人に届くコンテンツは無いと返信する。生成の前後で不適切な内容を作らないようにしていること(#329)と、サポート URL の連絡先を添える。<br>案 B: 求められたら、AI が作った画面を本人が報告できる入口と、報告の受け口を足して出し直す(報告の流れは `docs/legal/moderation-policy-draft.md` と #321 を元にする)。<br>年齢区分の答え(UGC「いいえ」・[listing.md](../store/listing.md) の 4 章)と食い違わないよう、案 B にした時は質問票も直す |

```text
Hello App Review team,

Thank you for the review. Regarding Guideline 1.2:

- Everything a user sets up in Tailor It is visible only to that user. This version has no public catalog, no share links, no data shared between users and no chat between users, so content made by one user is never shown to another user.
- The AI only returns the layout and labels of the user's own tracker, based on the user's own request.
<Only after #329 has confirmed it: "Requests and outputs are checked so that the app does not create objectionable screens or text.">
<!-- 確かめていない事実は書かない(2.3.1)。#329 で生成内容のフィルタを確かめるまで、上の行は入れない -->

- Users can contact us through the support page: https://tailor-it.app/contact.

If you would still like us to add a way to report AI-generated content, please let us know. We will add it in a new build.

Best regards,
<name>
```

### 4.5 5.1.1(データの収集と保存)

| 想定される指摘 | 対応の案 |
|---|---|
| (i) プライバシーポリシーの場所・中身(集めるデータ・第三者・保存期間・取り消し方) | 案 A: ASC の URL と、アプリの中の場所(設定 > プライバシー・同意画面)を返信で示す。中身が足りなければ**ポリシーを人が直してから**返信する(ポリシーの文言はこの手順で変えない) |
| (ii) 同意・権限の説明文 | 案 A: 説明文を直して出し直す(`scripts/strip-review-plist.sh` の審査用の文)。AI への送信の同意は 設定 > プライバシー で取り消せると返信する |
| (v) アカウントの削除・ログイン | 案 A: 匿名で使えること、設定(歯車)> アカウントを削除 で削除と Sign in with Apple の失効をすることを、手順つきで返信する |

```text
Hello App Review team,

Thank you for the review. Regarding Guideline 5.1.1:

- Privacy Policy: <privacy policy URL>. It is also available in the app from Settings > Privacy and from the AI consent screen.
- Consent: before the first AI request, the app shows a consent screen. It explains that what the user says or types, the chat and the tracker's layout are sent through our server to Anthropic's AI. The contents of the user's records are not sent. Consent can be withdrawn in Settings > Privacy.
- Sign-in: the app can be used without signing in ("Use without signing in (anonymous)").
- Account deletion: Settings (gear icon) > Delete Account. This deletes the account and the user's trackers and records as described in the Privacy Policy, clears the data on the device, and revokes the Sign in with Apple token.
<If we changed anything: "In build <number>, we changed <what>.">

Best regards,
<name>
```

## 5. 段階的なリリース(7 日)

確かめた事実(Apple のヘルプ):
- 7 日で 1% → 2% → 5% → 10% → 20% → 50% → 100%。対象は**自動アップデート**の人だけで、App Store から手で入れる人はいつでも入れられる。
- 止められる日数は合わせて 30 日まで(回数の制限なし)。いつでも「すべてのユーザーにリリース」を押せる。
- **版のアップデート**で選べる機能。最初の版(1.0)で効くかはヘルプに書かれていない。新しく入れる人は手で入れるので、1.0 では意味が無い **(推測)**。

| 選択肢 | 1.0 | 1.0.1 以降 | 良い所 | 悪い所 |
|---|---|---|---|---|
| ① 承認後に手動で公開・段階なし | 手動 | 手動・段階なし | 公開の時刻を選べる(backend を見られる時間に出す) | 不具合が全員に一度に届く |
| ② 1.0 は手動で公開・以降は 7 日の段階的なリリース(**推奨**) | 手動 | 手動 + 段階的 | 1.0 は時刻を選べる。更新の不具合は 1〜5% のうちに気づいて止められる(クラッシュは Crashlytics・#296) | 全員に届くまで 7 日かかる。DSL・API の互換を 2 つの版で保つ必要がある |
| ③ 承認後に自動で公開 | 自動 | 自動 | 手間が少ない | 夜中などに公開され、すぐに見られない |

推奨は ②。ただし backend の変更と組の版(新しい API が要る版)は、段階の途中でも古い版が残る前提で、backend を先に出す(process.md の 5 章)。急ぎの修正(hotfix)は「すべてのユーザーにリリース」で一気に出してよい。

## 6. 判断してほしいこと

| # | 決めること | 案 | 推奨 |
|---|---|---|---|
| 1 | 公開のしかた(5 章) | ① / ② / ③ | ② |
| 2 | ASC の「サインインが必要」をオフにしてよいか(3.2・匿名ですべて試せる) | オフ / デモアカウントを用意してオン | オフ |
| 3 | 1.2 の指摘が来た時、説明で返すか、最初から AI の生成物を報告する入口を入れて出すか(4.4) | 説明で返す / 先に入れる | 説明で返す(この版は本人しか見ないため) |
| 4 | 4.7 を満たす作り(索引・通報・年齢)を求められた時の線引き(4.3 の案 B) | 作る / 名指しの機能を外す / 申し立て | 名指しの機能を外す → それでも通らなければ申し立て |
| 5 | 輸出のコンプライアンスの申告(`false`)をそのままでよいか(2.5) | そのまま / 専門家に確かめる | そのまま **(推測・法的な判断ではない)** |

## 7. ガイドラインの要旨(確かめた文)

2026-11-03 に [App Review Guidelines](https://developer.apple.com/app-store/review/guidelines/) で確かめた要旨。番号が変わることがあるので、却下の時は原文を見直す。

| 番号 | 要旨 |
|---|---|
| 1.2 | ユーザーが作る・**生成する**・投稿するコンテンツのあるアプリ(SNS・UGC・**AI でコンテンツを作るアプリ**を含む)は、フィルタ・通報と速い対応・ブロック・連絡先の公開を持つこと |
| 1.2.1 | クリエイターのコンテンツは、本体の機能を変えずに、決まった体験にコンテンツを足すもの。アプリではなくアプリの中のコンテンツとして、UGC として扱われる |
| 2.1(a) | 審査に出すのは完成品。ログインがあるならデモアカウントの情報を入れ、backend を動かしておく |
| 2.3.1(a) | 新しい機能は審査メモに具体的に書く。ない機能を宣伝しない |
| 2.5.2 | バンドルの中で完結し、アプリの機能を足す・変えるコードをダウンロード・インストール・実行しない(教育用の例外あり) |
| 2.5.19 | 隠した・眠らせた・書いていない機能を入れない。隠すと削除・アカウント停止の理由になる |
| 4.2.6 | テンプレート・アプリを作るサービスで作ったアプリは、コンテンツの持ち主が出すのでなければ却下 |
| 4.7 | HTML5 / JavaScript のミニアプリ・ミニゲーム・ストリーミングのゲーム・チャットボット・プラグイン等は、バイナリの外から出せるが、出す側が全責任を持ち、4.7.1〜4.7.5 を守る |
| 4.7.1 | そのソフトも 5.1 のプライバシー・フィルタ / 通報 / ブロック・3.1 の課金を守る |
| 4.7.2 | Apple の許可なく、ネイティブの API をそのソフトに渡さない |
| 4.7.3 | 個々のソフトに、データや権限を、毎回のはっきりした同意なしに渡さない |
| 4.7.4 | 出しているソフトの索引(universal link つき)を持つ |
| 4.7.5 | アプリの年齢区分を超えるソフトを見分けられ、年齢で制限できること |
| 5.1.1(i) | プライバシーポリシーへのリンクを ASC とアプリの中に置く。集めるデータ・使い方・第三者・保存と削除・同意の取り消し方を書く |
| 5.1.1(ii) | データを集めるなら同意を取り、取り消しやすくする。権限の説明文ははっきり書く |
| 5.1.1(v) | アカウントの機能が大きくなければログインなしで使えること。アカウントを作れるならアプリの中で削除できること |
| 5.1.2(i) | 本人の許可なく個人データを使ったり送ったり共有したりしない。どこでどう使うかを示す |
| After You Submit | 却下の質問・補足は ASC で App Review に直接送る。不服は申し立て(Appeal)。配信中の版のバグ直しは、法律・安全以外の指摘では止めない |
