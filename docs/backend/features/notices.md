# お知らせ(/v1/me/notices)(#2516)

> [バックエンド設計](../architecture.md) の機能の文書。形の正は [openapi-appconfig.yaml](../openapi-appconfig.yaml) の `/me/notices`・`/me/notices/{noticeId}/read`・`/me/notify-consent/feedback-fixed`。作る側(自己改善ループ)は [feedback-loop.md](feedback-loop.md)。

## 目次

- [3 つの「お知らせ」](#3-つのお知らせ)
- [お知らせの種類と作る所](#お知らせの種類と作る所)
- [API](#api)
- [既読の付け方](#既読の付け方)
- [いつ消えるか](#いつ消えるか)
- [push で送る時](#push-で送る時)
- [端末での出し方](#端末での出し方)
- [コードの場所](#コードの場所)
- [決めること](#決めること)

## 3 つの「お知らせ」

似た名前のものが 3 つある。この文書は 1 つ目。

| もの | 誰に | 置き場 | 文書 |
|---|---|---|---|
| **本人へのお知らせ**(`GET /v1/me/notices`) | 1 人ずつ | `loop_notices/{noticeId}` | この文書 |
| 全員への 1 行の告知(障害・メンテナンス) | 全員 | appconfig の環境変数 `NOTICE_JSON` → `GET /v1/client-config` の `notice` | [incident-runbook.md §4](../../ops/incident-runbook.md#4-道具-アプリ内のお知らせ) |
| アプリのプッシュ通知(生成したアプリの通知) | 端末 | `users/{uid}/devices`・`notifyConsent` | [notifications.md](notifications.md) |

## お知らせの種類と作る所

今の種類は 1 つだけ。

| 種類 | 作る所 | 中身 |
|---|---|---|
| 「あなたの声で直りました」(#374) | `backend/cmd/feedback-loop fixed`(`backend/internal/loop/fixed`) | アプリから送ったフィードバックが元の Issue が、マージした PR で閉じられた時に、送った人ごとに 1 件。題は「あなたの声で直りました」、本文は決まった文 + 人が承認した Issue の題名(LLM は書かない) |

- 1 人に 1 つの直しで 1 件(id は `ntc_` + (下書きの id, uid)のハッシュ)。もう一度流しても 2 件目は作らない。
- アプリ(`source: inApp`)から送った人だけ。退会した人には作らない。
- 書くのは `cmd/feedback-loop` だけ。アプリは Firestore を直接読まず、appconfig の API を通す。

## API

どれも本人の分だけ(パスに uid は無い)。認証と App Check は他の `/v1/me` と同じ。

| API | すること |
|---|---|
| `GET /v1/me/notices` | 新しい順の一覧。`pageSize`(既定 50・最大 200)・`pageToken`。応答は `{items, unreadCount, nextPageToken?}`。各項目は `id`・`title`・`body`・`createdAt`・`readAt`(既読の時だけ)。`unreadCount` は全ページの未読の数 |
| `POST /v1/me/notices/{noticeId}/read` | 既読にする。応答は既読にした後のお知らせ |
| `GET /v1/me/notify-consent/feedback-fixed` | 「直ったら push でも知らせる」の同意。`{enabled, updatedAt?}`。保存が無ければ `enabled: false` |
| `PUT /v1/me/notify-consent/feedback-fixed` | 同意を変える。本文 `{"enabled": true \| false}`(必須) |

| 状態 | code | いつ |
|---|---|---|
| 400 | `bad_request` | `noticeId` の形が違う(`ntc_` + 16 進 24 文字)・`pageSize` / `pageToken` が不正・`enabled` が無い |
| 404 | `not_found` | 無いお知らせ。他の人のお知らせも同じ 404 |

- 一覧は 1 回に本人の分を最大 500 件読んでから、期限の過ぎた分を外してページに分ける。

## 既読の付け方

- 端末が `POST /v1/me/notices/{noticeId}/read` を呼ぶ。サーバーは `readAt` に今の時刻を書く。
- 冪等: もう既読なら最初の `readAt` のまま 200 を返す。
- 未読に戻す口は無い。
- iOS は一覧を閉じた時に、見た分を既読にする([feedback-loop.md「アプリで見る」](feedback-loop.md#アプリで見る))。

## いつ消えるか

| いつ | どうなるか |
|---|---|
| 作ってから 90 日(`expiresAt` = 作った時 + `loop.Retention`) | 一覧から外す(期限が過ぎた時点で、消す前でも出さない)。その後、`cmd/feedback-loop` の掃除(`purge`)で消す |
| アカウントを削除した時 | 30 日の削除を待たず、次の掃除で消す |
| 既読にした時 | 消さない(既読のまま 90 日まで残る) |

ループの置き場はどれも元のフィードバック(180 日)より短く持つ([feedback-loop.md「置き場所」](feedback-loop.md#置き場所))。

## push で送る時

お知らせ(アプリの中)は同意が無くても作る。push は次が全部そろった時だけ、お知らせを作るのと一緒に 1 回送る。

- `cmd/feedback-loop` の環境変数 `LOOP_NOTIFY_PUSH=on`
- 本人の同意(`users/{uid}/notifyConsent/_feedback_fixed`・上の `PUT`)
- その人への前の push から 7 日たっている(週 1 回まで)

送る先は [notifications.md](notifications.md) と同じ端末の登録(`users/{uid}/devices`)で、送り方は APNs だけ(`cmd/feedback-loop/fixed.go` の `APNsPusher`)。Android・デスクトップには届かない。

## 端末での出し方

| 端末 | 今の出し方 |
|---|---|
| iOS | 設定 →「お知らせ」(`ios/Metamon/Features/Notices`)。新しい順・未読の点・引っぱって更新・設定の行に未読の数のバッジ。フィードバックの画面に「直ったら知らせてほしい」(`FeedbackFixedOptIn`)。審査用の版ではフィードバックと一緒に外す([review-build.md](../../ios/review-build.md)) |
| Mac | `/v1/me/notices` を呼ぶコードは無い(`ios/MetamonMac` に見当たらない) |
| Android | `/v1/me/notices` を呼ぶコードは無い。全員への告知(`client-config` の `notice`)は出す(`update/UpdateGate.kt`) |
| デスクトップ | `/v1/me/notices` を呼ぶコードは無い |

push で届いた時の出し方は端末の通知の仕組みに従う([docs/mac/system-integration.md](../../mac/system-integration.md) など)。

## コードの場所

| ファイル | 中身 |
|---|---|
| `backend/internal/appconfig/loop_notices.go` | `/v1/me/notices`・既読・同意の HTTP |
| `backend/internal/loop/loop.go` | `Notice` の形・`NoticeID`・`Retention`(90 日)・`FeedbackFixedConsentID` |
| `backend/internal/loop/firestore.go`・`memory.go` | 読み書き(`ListNotices` / `MarkNoticeRead` / `PutNotice`) |
| `backend/internal/loop/fixed/fixed.go` | お知らせを作る・push の判断(`shouldPush`) |
| `backend/internal/loop/purge.go` | 期限・退会での掃除(`Sweep`) |

## 決めること

- Mac・Android・デスクトップで本人へのお知らせを出すか。出すなら、iOS と同じく設定の中の一覧にするか(推測: どの端末でも同じ uid の一覧が読めるので、サーバーの変更は要らない)。
- 種類が 2 つ目になる時の形: 今の応答は種類を持たない(`title` と `body` だけ)。`kind` を足すか、`loop_notices` とは別の置き場にするか。
