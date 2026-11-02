# 通報・ブロック・運営による非公開化(#321)

App Store Review Guidelines 1.2 / 1.2.1(a)(利用者が作った内容を共有するアプリ)への対応です。審査用の版(`APP_STORE_REVIEW`)は共有を外しているので、この仕組みは**共有を戻す版の前提**です。

> この文書の gcloud の手順は**人の作業**です(エージェントは GCP・本番 Firestore を変えない)。

## 目次
- [仕組み](#仕組み)
- [通報を受けたら(運用)](#通報を受けたら運用)
- [cmd/moderate の使い方](#cmdmoderate-の使い方)
- [通報のアラート(設定の手順)](#通報のアラート設定の手順)
- [まだ決めていないこと](#まだ決めていないこと)

## 仕組み

| 機能 | API / 保存先 | 効き方 |
| --- | --- | --- |
| 通報 | `POST /v1/reports` → `reports/{id}` | 対象 = 「探す」の掲載・シェアリンク・共有アプリ・共有アプリの投稿。理由は 7 種(spam / sexual / violence / harassment / illegal / privacy / other)。同じ人が同じ対象を通報しても 1 件。1 人 1 日(JST)30 件まで。保存するのは通報者 uid・対象の種類と ID・対象の持ち主 / 投稿者 uid・理由・日時・状態(`open` / `done` / `dismissed`)だけ |
| 運営への知らせ | Cloud Logging の `moderation: report`(WARNING) | uid と内容は出さない(reportId・種類・対象 ID・理由)。下のアラートで運営に届く |
| ブロック | `GET/POST /v1/me/blocks`・`DELETE /v1/me/blocks/{uid}` → `users/{uid}/blocks/{blockedUid}` | ブロックした人の投稿(GET records)と掲載(GET /public-apps)を、その人にだけ返さない。iOS はブロックした瞬間に端末でも隠す。相手には知らせない |
| 掲載の非公開化 | `public_apps/{id}` の `hidden`・`hiddenReason`・`hiddenAt` | 掲載者以外の検索に出ず、install は 403 `unpublished`。掲載者には「運営により非公開」と理由が出る。掲載者が公開し直しても非公開のまま |
| 共有アプリの非公開化 | `apps_public/{appId}` の同じ項目 | その appId の共有データ(public / shared の entity の記録・いいね)が全員 403 `unpublished`(理由つき)。シェアリンクのプレビュー・参加、「探す」からの参加、他人が appId を開いた時の DSL の配布も止まる。各自の端末のアプリ(自分の木のコピー)と自分だけの記録は残る |
| 投稿の削除 | `apps_public/{appId}/entities/{e}/records/{id}` | `delete-record` で消す |
| シェアリンクの停止 | `shares/{token}.revoked` | `revoke-share`。共有データの無いアプリのリンクはこれで止める |

退会の消去(`cmd/purge`)は、本人の通報・本人が対象の通報・ブロック一覧も消す([account-purge.md](account-purge.md))。

## 通報を受けたら(運用)

1. アラート(下)が来たら、`moderate reports` で開いている通報を見る。
2. 対象を Firestore のコンソールで確かめる(読むだけ)。
   - `listing` → `public_apps/{targetId}`
   - `app` → `apps_public/{targetId}`
   - `share` → `shares/{targetId}`(`appId` の DSL)
   - `record` → `apps_public/{appId}/entities/{entityId}/records/{recordId}`(`targetId` = `appId/entityId/recordId`)
3. 規約(`docs/legal/terms.md`)の禁止事項に当たれば止める(`hide-listing` / `hide-app` / `delete-record` / `revoke-share`)。理由は作者が読む文で書く(例「性的な内容が含まれるため非公開にしました」)。
4. `resolve <reportId>`(当たらなければ `-status dismissed`)。
5. 悪質な利用者は、Firebase Auth のコンソールでアカウントを無効にする(この仕組みには無い)。

## cmd/moderate の使い方

書き込むコマンドは `-yes` を付けた時だけ書く(付けなければ何をするかを出すだけ)。

```bash
cd backend
# 運営のアカウントで ADC を作る(Firestore に書ける権限が要る)
gcloud auth application-default login
export GOOGLE_CLOUD_PROJECT=jphacks-kz2601 APP_ENV=prod

go run ./cmd/moderate reports                     # 開いている通報(-all で対応済みも)
go run ./cmd/moderate hide-listing <id> -reason "性的な内容が含まれるため非公開にしました" -yes
go run ./cmd/moderate unhide-listing <id> -yes
go run ./cmd/moderate hide-app <appId> -reason "..." -yes
go run ./cmd/moderate unhide-app <appId> -yes
go run ./cmd/moderate delete-record <appId> <entityId> <recordId> -yes
go run ./cmd/moderate revoke-share <token> -yes
go run ./cmd/moderate resolve <reportId> -yes     # -status dismissed で「該当なし」
```

ローカル(エミュレータ)で試す: `make emulator` の後に `make -C backend run-moderate ARGS="reports"`。

## 通報のアラート(設定の手順)

ログベースの指標と通知のポリシーで、通報が 1 件でも来たらメールを送る案です(未設定)。

```bash
gcloud logging metrics create moderation_reports --project=jphacks-kz2601 \
  --description="POST /v1/reports (#321)" \
  --log-filter='resource.type="cloud_run_revision" AND resource.labels.service_name="appconfig-prod" AND jsonPayload.msg="moderation: report"'
# 通知のポリシー: Cloud Console > Monitoring > Alerting で
#   指標 logging/user/moderation_reports の合計 > 0(5 分)→ 運営のメールの通知チャネル
```

## まだ決めていないこと

- 24 時間以内に対応する当番(誰がアラートを受け、休日はどうするか)。
- 投稿の NG ワードのフィルタ(#321 の残り)と、規約の禁止事項(#246)。
- 止めた時に作者へ届ける手段(今は掲載者の「探す」・共有アプリを開いた時の 403 の理由だけ)。
