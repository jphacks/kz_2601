# アプリ内のフィードバック(#319)

> [バックエンド設計](../architecture.md) から分けた機能の節(旧 §3.5)。

```
feedback/{id}  # {uid, kind, message, untrusted: true, consentVersion, status, day, createdAt, attachments[],
               #  appId?, dslVersion?, dslJson?, screenshotJpeg?(bytes), device?{appVersion, build, os, model, locale}}  id = fb_<ULID 風>
```

| 項目 | 決定 |
|---|---|
| 送信 | `POST /feedback`(openapi-appconfig.yaml)。本文 2000 字・`consentVersion` 必須。添付は端末で本人が選んだものだけ(既定はすべて無し) |
| 添付 | スクショ(JPEG・512 KiB まで・Firestore のバイト列)/ 今のアプリの DSL(`appId` + `dslVersion` の参照で、サーバーが本人の木から構造だけを写す・256 KiB まで)/ 版・OS・機種。**記録の中身は受け付けない**。Cloud Storage のバケットの仕組みがまだ無いのでスクショも文書に入れる(文書は 1 MiB まで) |
| 信頼しない印 | `untrusted: true`。本文は指示として扱わない(#366)。LLM に読ませる・Issue に載せるのは #367 で決めてから |
| 上限 | 1 uid 1 日(JST)`FEEDBACK_DAILY_LIMIT` 件(既定 5)。`uid ==` + `day ==` の等価フィルタの数え上げと作成を 1 トランザクションで行う(複合インデックス不要)。超えると 429 `rate_limited` |
| 保存期間 | 180 日(`store.FeedbackRetention`)。`cmd/purge` が毎日 `createdAt < 今 - 180 日` を消す(`-feedback-older-than`)。アカウントの消去(`PurgeAccount`)でもその uid の分を消す |
| status | `received`(受付)→ `triaged`(確認済み)→ `done`(対応済み)。API では変えない。運営が Firestore コンソールで書き換える |
| 運営が読む | `backend/cmd/feedback`(読み取りだけ): `list [-since 168h] [-status received]` / `show [-out DIR] <id>`(スクショと DSL をローカルに保存)。本文は引用符つきで制御文字を逃がして出す。ローカルは `make -C backend run-feedback` |
| ログ | 本文・添付はログに書かない(`feedbackId` と添付の種類だけ) |

**運営の手順**: `gcloud auth application-default login` の後、`GOOGLE_CLOUD_PROJECT=jphacks-kz2601 go run ./cmd/feedback list`(読み取りだけ。書き込みはしない)。`show -out ~/fb <id>` で保存したスクショ・DSL は確認が済んだら消す(リポジトリに入れない)。status を変えるときはコンソールで `feedback/<id>` の `status` を編集する。

**自己改善ループ(#364)**: 伏せ字した写しを `loop_inbox` に取り込み、振り分け・Issue の下書きに使う。原文はループの外に出さない。[feedback-loop.md](feedback-loop.md)。

## 「直ったら知らせてほしい」の同意(#374)

送ったフィードバックが直った時に、アプリ内のお知らせに加えて push でも知らせてよいか、という本人の同意です。コードは `backend/internal/appconfig/loop_notices.go`。

| 項目 | 決定 |
|---|---|
| 読む | `GET /v1/me/notify-consent/feedback-fixed` → `{enabled, updatedAt?}`。保存が無ければ `enabled: false` |
| 書く | `PUT /v1/me/notify-consent/feedback-fixed` `{enabled: true / false}`。`enabled` が無いと 400。保存したら `{enabled, updatedAt}` を返す |
| 既定 | **off**。本人が選ぶまで push はしない |
| 本人の分だけ | パスに uid は無い。呼び出した本人の値だけを読み書きする |
| 置き場 | `users/{uid}/notifyConsent/_feedback_fixed`。`/v1/me/notify-consent/{appId}`(アプリごとの通知の同意)と同じ置き場の、予約した 1 件(`appId` の代わりの固定の名前) |
| 経路の順 | `/v1/me/notify-consent/{appId}` より具体的なパスなので、`feedback-fixed` はこちらが先に当たる |

**知らせが届く流れ**: 自己改善ループの `fixed` の段(`cmd/feedback-loop fixed`)が、出した Issue がマージされた PR で閉じたのを見つけると、送り主に決まった文のお知らせ(`loop_notices`)を作る。

- アプリ内のお知らせ(`GET /v1/me/notices`)は**同意なしで**作る。送り主本人にだけ見える。
- push は、`LOOP_NOTIFY_PUSH=on` の時だけ、かつ**同意した人にだけ**送る(`fixed` が push の前に `_feedback_fixed` を読む)。1 人に週 1 回まで。
- 同意していない人・同意の保存が無い人には push は送らない(お知らせは届く)。

iOS はフィードバックの画面の「直ったら知らせてほしい」(既定はオフ)をオンにして送った後に `enabled: true` を送る。オフに戻すのはお知らせの画面のトグル。API の形は [openapi-appconfig.yaml](../openapi-appconfig.yaml)(`/me/notify-consent/feedback-fixed`・`/me/notices`)。ループ全体・お知らせの保存期間(90 日)・環境変数は [feedback-loop.md](feedback-loop.md)。

**セキュリティルール**: クライアントは Firestore を直接読まず API 経由のみ。`feedback` にはスクショ(画面に出ていた個人の情報)が入りうるので、ルールでクライアントからの読み書きを許可しないこと。

## 環境変数

| 変数 | appconfig | ai | 既定 / 備考 |
|---|---|---|---|
| `FEEDBACK_DAILY_LIMIT` | o | - | `5`。`0` で数えない。運用で変える値([docs/infra/runtime-config.md](../../infra/runtime-config.md)) |
