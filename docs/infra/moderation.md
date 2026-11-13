# 通報・ブロック・運営による非公開化(#321)

App Store Review Guidelines 1.2 / 1.2.1(a)(利用者が作った内容を共有するアプリ)への対応です。審査用の版(`APP_STORE_REVIEW`)は共有を外しているので、この仕組みは**共有を戻す版の前提**です。

> この文書の gcloud の手順は**人の作業**です(エージェントは GCP・本番 Firestore を変えない)。

## 目次
- [仕組み](#仕組み)
- [通報を受けたら(運用)](#通報を受けたら運用)
- [cmd/moderate の使い方](#cmdmoderate-の使い方)
- [通報のアラート](#通報のアラート)
- [表示できない言葉の一覧の足し方・外し方](#表示できない言葉の一覧の足し方外し方)
- [まだ決めていないこと](#まだ決めていないこと)

## 仕組み

| 機能 | API / 保存先 | 効き方 |
| --- | --- | --- |
| 通報 | `POST /v1/reports` → `reports/{id}` | 対象 = 「探す」の掲載・シェアリンク・共有アプリ・共有アプリの投稿。理由は 7 種(spam / sexual / violence / harassment / illegal / privacy / other)。同じ人が同じ対象を通報しても 1 件。1 人 1 日(JST)30 件まで。保存するのは通報者 uid・対象の種類と ID・対象の持ち主 / 投稿者 uid・理由・日時・状態(`open` / `done` / `dismissed`)だけ |
| 運営への知らせ | Cloud Logging の `moderation: report`(WARNING) | uid と内容は出さない(reportId・種類・対象 ID・理由)。下のアラートで運営に届く |
| ブロック | `GET/POST /v1/me/blocks`・`DELETE /v1/me/blocks/{uid}` → `users/{uid}/blocks/{blockedUid}` | ブロックした人の掲載(GET /public-apps)をその人に返さない。共有アプリの投稿(GET records・id 指定・通報・マーク)は、どちら向きのブロックでもお互いに返さない。iOS はブロックした瞬間に端末でも隠す。相手には知らせない |
| 掲載の非公開化 | `public_apps/{id}` の `hidden`・`hiddenReason`・`hiddenAt` | 掲載者以外の検索に出ず、install は 403 `unpublished`。掲載者には「運営により非公開」と理由が出る。掲載者が公開し直しても非公開のまま |
| 共有アプリの非公開化 | `apps_public/{appId}` の同じ項目 | その appId の共有データ(public / shared の entity の記録・いいね)が全員 403 `unpublished`(理由つき)。シェアリンクのプレビュー・参加、「探す」からの参加、他人が appId を開いた時の DSL の配布も止まる。各自の端末のアプリ(自分の木のコピー)と自分だけの記録は残る |
| 投稿の削除 | `apps_public/{appId}/entities/{e}/records/{id}` | `delete-record` で消す |
| シェアリンクの停止 | `shares/{token}.revoked` | `revoke-share`。共有データの無いアプリのリンクはこれで止める |
| 表示できない言葉のフィルタ | `internal/moderation/textfilter`(一覧は `terms.txt`) | 他の人に見える文字だけを書き込みの時に見る: public/shared の entity の記録(string / text の項目・authorName)、参加者の表示名(`POST /shares/{token}/import`)、「探す」の掲載(name・description・tags・authorName)。当たれば 422 `text_not_allowed`(「表示できない言葉が含まれています」・`details.field`)で保存しない。作成者だけの private の記録は見ない。記録では書き込みの権限(参加・作成者だけ・管理者だけ)の判定の後に見る(403 を 422 で隠さない)。ログは `moderation: text refused`(INFO)に場所(record / member / listing)と種類(slur / sexual / threat)だけで、文字は出さない |
| フィルタの切り替え | appconfig の環境変数 `TEXT_FILTER`([runtime-config.md](runtime-config.md#運用で変える値)) | `enforce`(既定)/ `log`(保存して `moderation: text would be refused` に場所と種類だけ)/ `off`(見ない)。誤検知の時はデプロイせずに落とす。それ以外の値は起動しない |

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
   - 自動の動き(通知)が絡む時は、先に通知・アプリを止める([incident-runbook.md 手順 F](../ops/incident-runbook.md#13-手順-f-悪用自動の動き)・`stop-app` / `pause-notify`)。24 時間で 3 人の通報があったアプリは、runner が通知を自動で一時停止している(`moderate stops -env prod` で見える)。
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

## 通報のアラート

[`infra/moderation_alerts.tf`](../../infra/moderation_alerts.tf) にコードがある(コードは用意済み・`terraform apply` はまだ)。通知先は監視と同じ(`alert_emails` / `alert_notification_channels`・[observability.md](observability.md))で、`monitoring_enabled` が false なら作らない。

| 指標(ログベース) | 元のログ | アラート |
| --- | --- | --- |
| `logging/user/moderation_reports`(ラベル `kind` / `reason`) | appconfig の `message="moderation: report"`(WARNING) | 5 分の合計 > 0 → WARNING(今は少ないので 1 件でも知らせる) |
| `logging/user/runner_stops`(ラベル `by` / `level`) | runner の `event="runner.stop"`(WARNING) | `by=auto` の 5 分の合計 > 0 → ERROR(通知の自動一時停止) |

- ログの本文は `httpx.NewCloudHandler` が `msg` を `message` に変えて出すので、フィルタは `jsonPayload.message` を見る(前の案の `jsonPayload.msg` では当たらない)。
- 通報のアラートは `monitored_services` のサービスだけを見る。stg が同じプロジェクトにある間は stg の通報も届く(件名にサービス名が出る)。
- 通報が増えたら `google_monitoring_alert_policy.moderation_reports` の `threshold_value` を上げる。

適用(人の作業・[infra/README.md](../../infra/README.md) の手順): `terraform plan -var-file=envs/prod.tfvars` で指標 2 つ・ポリシー 2 つの追加だけになっていることを見てから `apply`。

## 表示できない言葉の一覧の足し方・外し方

言葉そのものはこの文書にも Issue・PR の本文にも書かない(差分の中だけに置く)。

### 置き場と形

- 一覧は [`backend/internal/moderation/textfilter/terms.txt`](../../backend/internal/moderation/textfilter/terms.txt)。`go:embed` でバイナリに入るので、変えたら**デプロイして初めて効く**(切り替えだけなら `TEXT_FILTER`・上の表)。
- 1 行 1 語で `<種類> <言葉>`。`#` で始まる行と空行は読み飛ばす。
- 種類は `slur` / `sexual` / `threat` と `allow` の 4 つ。それ以外は `Parse` がエラーにする(起動の前のテストで落ちる)。
  - `allow` は「当たる言葉を含むが問題ない言い回し」。照合の前に本文から取り除く。誤検出を直す時に使う。
- 言葉は入力と同じ `Normalize`(NFKC・小文字・ひらがな→カタカナ・記号と空白を 1 つの空白に)をかけてから比べる。ひらがな・カタカナ・全角の書き分けは 1 行でよい。
- 英数字だけの言葉は**単語の単位**で当たる。末尾の `*` は「その語で始まる単語」に当たる(英数字だけに使える。日本語に付けると `Parse` がエラー)。
- 英数字以外を含む言葉は本文の**どこにでも**当たる(単語の区切りを見ない)。だから短い日本語ほど誤検出しやすい。

### 足す・外す時の注意

1. 足す言葉が、普通の文の中に部分として出てこないかを先に考える(日本語は部分一致なので特に)。
2. 足したら、`textfilter_test.go` の `TestCheckRefuses` に**当たるべき例**を、`TestCheckAllowsCommonText` に**当たってはいけない普通の文**(誤検出の例)を足す。別の `testdata` の置き場は無く、例はこの 2 つのテストの表に書く。
3. 誤検出が出たら、言葉を外すより先に `allow` の行で直せないかを見る。直したら、その文を `TestCheckAllowsCommonText` に足す(同じ誤検出を戻さないため)。
4. 外す時は、`backend/internal/appconfig/textfilter_test.go` がその言葉を例に使っていないかを見る(使っていれば別の言葉に替える)。
5. 一覧は「短く・明らかな物だけ」(`terms.txt` の頭の注記)。判断の分かれる言葉は足さず、通報と運営の非公開化に任せる。

### 回すテスト

```bash
cd backend
go test ./internal/moderation/textfilter/   # TestDefaultParses・TestParseErrors・TestNormalize・TestCheckRefuses・TestCheckAllowsCommonText・TestModeFromEnv
go test ./internal/appconfig/ -run TestTextFilter   # 書き込みの時に 422 になるか(TestTextFilterModes ほか)
```

- `TestDefaultParses` は埋め込んだ `terms.txt` が読めて、日本語・英数字・`allow` がそれぞれ 1 行以上あることを見る。
- どちらもエミュレータは要らない。`backend/` を変える PR では backend-ci の `go test ./... -race` でも回る。
- 一覧を変える PR を誰がレビューするかは決めていない(下の「まだ決めていないこと」の見直しの担当)。

## まだ決めていないこと

- 24 時間以内に対応する当番(誰がアラートを受け、休日はどうするか)。
- 規約の禁止事項(#246)。
- 表示できない言葉の一覧(`terms.txt`)を見直す担当と頻度、誤って断られた時の問い合わせの受け方。
- フィルタの対象を広げるか(写真・共有アプリの画面の文字)。
- 止めた時に作者へ届ける手段(今は掲載者の「探す」・共有アプリを開いた時の 403 の理由だけ)。
