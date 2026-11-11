# 外の API(runner の口・数え方・OAuth のトークン)

> #1493 の土台の実装。設計と決定は [decisions/external-apis.md](../../decisions/external-apis.md)(2026-11-11 決定)。provider ごとの中身は子 Issue(#1494〜#1501)、appconfig の受け口と契約・DSL の部品・iOS は別の子 Issue。**値段は書かない**(原価と押さえの量は env・値は非公開の資料)。

## 目次

1. [できていること・まだのこと](#できていることまだのこと)
2. [置き場所](#置き場所)
3. [provider の一覧(閉じた一覧)](#provider-の一覧閉じた一覧)
4. [呼び出しの流れと数え方](#呼び出しの流れと数え方)
5. [OAuth のトークン](#oauth-のトークン)
6. [runner の口](#runner-の口)
7. [appconfig の口(端末から)](#appconfig-の口端末から)
8. [スプレッドシートの取り込み(自動の動き・#1712)](#スプレッドシートの取り込み自動の動き1712)
9. [偽の provider(stg と試験)](#偽の-providerstg-と試験)
10. [人の作業](#人の作業)

## できていること・まだのこと

| もの | 状態 |
|---|---|
| provider の閉じた一覧・op の定義・無料 / 有料・OAuth の要否 | 済(`internal/extapi/registry.go`)。本物の provider は Frankfurter(為替・#1499・`frankfurter.go`)。ほかは子 Issue で 1 provider = 1 ファイル |
| 数え方(無料 = `runs` 1・有料 = `runs` 1 + コインの押さえ → 確定 → 差額の戻し・失敗で両方戻す) | 済(`Service.Call`・`quota.ExtRun`・履歴 `spend_ext`) |
| Pro だけ(決定 1) | 済(作成者の Pro。`PRO_GATE_MODE` に関係なく) |
| OAuth の始める・戻り先・トークンの保管・取り消し・90 日で消す | 済(`oauth.go`・`connections.go`)。本物の provider の `OAuthClient` は子 Issue |
| KMS で暗号化 | 済(`KMSSealer`・`EXT_TOKEN_KMS_KEY`)。鍵と権限は人の作業 |
| runner の口 | 済(`/internal/ext/*`・`/oauth/callback/{provider}`)。Cloud Run の設定(戻り先を認証なしで受ける)は人の作業 |
| 出口の許可リスト・本文の上限 | 済(`NewHTTPClient`・`ReadBody`)。provider のファイルが使う |
| 退会の 30 日後の消去(`cmd/purge`) | 済(`users/{uid}/connections/*` を消す・ログ `extConnections`) |
| appconfig `POST /v1/apps/{appId}/ext/{provider}/{op}`・`/v1/me/connections/*`・エラーコード | 済(`internal/appconfig/ext.go`・runner の呼び出しは `extapi.Client`・`openapi-appconfig.yaml` の tag `ext`)。下の「appconfig の口」 |
| DSL `externalCall` / `externalList`・自動の動きからの呼び出し | **まだ**(DSL の子 Issue。`CallRequest.Run` に run の `quota.Decision` を渡すだけで数え方は同じ) |
| スプレッドシートを決まった時間に取り込む(#1712) | 実行する部分は済(`internal/runner/sheet_import.go`)。DSL の書き方と runner への組み込みは **まだ**(#1643 待ち)。下の「スプレッドシートの取り込み」 |
| 退会(`DELETE /v1/me`)の時の provider 側の取り消し | 済(`account.go` の `deleteMe` が `extapi.Client.RevokeAll` で `/internal/ext/connections/revokeAll` を呼ぶ・#1716)。失敗してもログだけで退会は 204 |
| キャッシュ | 済(`cache.go`・op の `CacheTTL`。鍵だけ・OAuth なしの読む op だけ。台の中のメモリ) |
| 回数の上限 `EXT_LIMIT_*`・`EXT_GLOBAL_DAILY_<PROVIDER>`・provider ごとの 1 分止め | 済(`limits.go`・#1715)。下の「回数の上限」 |
| iOS の「つないだサービス」・402 / 409 のシート・履歴の `spend_ext` の文言 | **まだ**(iOS の子 Issue。今は履歴に `spend_ext` がそのまま出る) |

## 置き場所

| もの | 場所 |
|---|---|
| provider の定義・登録 | `backend/internal/extapi/registry.go`(`register` は同じパッケージの `init()` からだけ = 閉じた一覧) |
| op の形(`Operation`・`Input`・`Output`・`Token`・`OAuthClient`) | `operation.go` |
| 呼び出しの流れ・数え方・理由(`Reason*`) | `service.go` |
| 回数の上限・1 分止め | `limits.go`(数えるのは `ratelimit.Store`。runner では Firestore の `rateLimits`) |
| 月の枠 `runs` を 1 足す | `backend/internal/runner/quota/external.go`(`ExtRun`) |
| コインの op `ext`・履歴 `spend_ext` | `backend/internal/coins`(`OpExt`・`KindSpendExt`) |
| トークンの保管・取り消し・90 日 | `connections.go`(`Connections`・`Sealer`・`LocalSealer`)・`kms.go`(`KMSSealer`)・`firestore.go` |
| OAuth の state / PKCE | `oauth.go` |
| 出口 | `egress.go` |
| runner への組み込み | `backend/cmd/runner/ext.go` |
| appconfig の口・runner の呼び出し | `backend/internal/appconfig/ext.go`・`internal/extapi/client.go`・`backend/cmd/appconfig/ext.go`(`EXT_RUNNER_URL`) |

## provider の一覧(閉じた一覧)

provider は `Provider` で定義し、そのファイルの `init()` で `register` する。DSL が書けるのは provider と op の名前と、op が決めた引数の名前だけ(URL・ヘッダー・本文は書けない。URL に見える値の引数は 400)。

| 項目 | 中身 |
|---|---|
| `ID` | DSL の provider(lowerCamel)。env の名前は大文字のスネーク(`googleCalendar` → `GOOGLE_CALENDAR`) |
| `Billing` | `free`(API 自体は無料)/ `paid`(呼ぶたびに原価) |
| `NeedsOAuth` | 利用者ごとのトークンが要るか。審査用の版の機能のキーに入れない(決定 6・`ReviewBuildFeatures`) |
| `Ops` | op の名前・書く op か・要る scope・取る引数 |
| `Hosts` | 出口の許可リスト(`https://<host>` だけ) |
| `New` / `NewOAuth` | 起動時に鍵(`EXT_<PROVIDER>_API_KEY` / `_CLIENT_ID` / `_CLIENT_SECRET`)から op と OAuth のクライアントを作る |

機能のキーは `externalCall.<provider>`(`FeatureKey`)。provider は既定で off(`EXT_<PROVIDER>_ENABLED`)、`EXT_DISABLED=1` で全部止まる。

op の `Check` は引数の値を確かめる(appconfig と runner の両方で、数える前に。だめなら 400)。`CacheTTL` は同じ引数の答えを持つ時間(下の「キャッシュ」)。

### 入っている provider

| provider | 種類 | op | 出口 | キャッシュ | 入れ方 |
|---|---|---|---|---|---|
| `asana` の `myTasks`(自分のタスクを見る・#1683) | 無料・OAuth(`runs` だけ・scope `tasks:read` `workspaces:read`) | `myTasks(workspaceId?, limit?)`。自分に割り当てられた未完了のタスク(`assignee=me`・`completed_since=now`)。`workspaceId` は数字の id。無い時は利用者のワークスペースが 1 つならそれを使い、0 か 2 つ以上なら 400 `bad_request`(`workspaces:read` はこのためだけ)。`limit` 1〜50(既定 20)。行はタスク(見出し = 名前・値 = 期限(`due_at`、無ければ `due_on`)・リンク = `https://app.asana.com/` の permalink だけ)。規約: Asana API Terms(出典の表示・利用者の同意・データを売らない・Asana の機能そのものに課金しない)。回数: トークンごとに 1 分 150 回(無料の組織)/ 1,500 回(有料)・同時 GET 50。429 は `Retry-After` | `https://app.asana.com` だけ(認可・トークン・API が同じホスト) | なし | `EXT_ASANA_ENABLED=1`・`EXT_ASANA_CLIENT_ID` / `_CLIENT_SECRET` |
| `asana` の `projectTasks`(プロジェクトのタスクを見る・#1683) | 無料・OAuth(`runs` だけ・scope `tasks:read`) | `projectTasks(projectId, limit?)`。プロジェクトの未完了のタスク。`projectId` は数字の id(Asana の URL の `/0/<id>/`)。持っていない id(Asana の 400 / 404)は 400 `bad_request`。行は `myTasks` と同じ。規約・回数は同上 | 同上 | なし | 同上 |
| `asana` の `complete`(タスクを完了にする・#1683) | 無料・OAuth(`runs` だけ・scope `tasks:write`。初めて使う時につなぎ直す。Asana の scope は足し算にならないので、つなぎ直しでも読む scope を一緒に求める)・書く(確認必須) | `complete(taskId)`。`taskId` は数字の id。答えは `resultId`(= `taskId`)と 1 件(補足 = 「完了」)。完了済みをもう一度完了にしても変わらない。持っていないタスクは 400 `bad_request`。規約は同上。回数: 書き込みは同時 15 | 同上 | なし | 同上 |
| `frankfurter`(為替・欧州中央銀行・#1499) | 無料・鍵なし(`runs` だけ) | `latest(from?, to?, amount?)`・`historical(date, from?, to?, amount?)`。`from` / `to` は大文字 3 文字の通貨(`to` はカンマ区切りで 10 個まで・重ならない)・`amount` は正の数・`date` は 1999-01-04 から今日まで。行は通貨ごと(見出し = 通貨・値 = レート・補足 = 「日付・量 基準」) | `https://api.frankfurter.app` だけ・本文 64 KiB まで | latest 1 時間・historical 1 日 | `EXT_FRANKFURTER_ENABLED=1` |
| `openWeather` の `airQuality`(空気の汚れ・OpenWeatherMap Air Pollution・#1656) | 無料・運営の鍵(`runs` だけ。OpenWeather の契約の範囲) | `airQuality(lat, lng)`。`lat` / `lng` は小数(10 進だけ)で小数 2 桁(約 1 km)に丸めて送る。答えは 1 件(見出し = 「空気 <AQI の段階>(目安)」・値 = `AQI n/5`・補足 = PM2.5・PM10・O₃・NO₂ の µg/m³)。AQI は OpenWeather の 1〜5(良い・まあまあ・ふつう・悪い・とても悪い)。1〜5 でない答えは 502 `upstream_unavailable`。健康の判断ではなく目安。出典「Weather data © OpenWeather (ODbL)」を出す。AI に選ばせる決まり(`prompt.go`・`catalog.go`)はまだ | `https://api.openweathermap.org` だけ(`/data/2.5/air_pollution`。天気と同じホスト・同じ鍵) | なし | `EXT_OPEN_WEATHER_ENABLED=1`・`EXT_OPEN_WEATHER_API_KEY` |
| `todoist` の `listTasks`(タスクを見る・#1496・`project` / `label` は #1681) | 無料・OAuth(`runs` だけ・scope `data:read`) | `listTasks(filter?, projectId?, project?, label?, limit?)`。`filter`・`projectId`・`project`(プロジェクトの名前)はどれか 1 つ。`label` はどれとも組める(`project` か `filter` と組むと Todoist のフィルタ `#名前 & @ラベル`、ほかは `label` の引数)。名前は 120 文字まで・1 行・`& \| ! ( ) , \ * # @` は使えない。カンマを含む `filter` に `label` は付けられない。`limit` 1〜50(既定 20)。行はタスク(見出し = 内容・補足 = 期限の言葉・値 = 期限・リンク) | `https://api.todoist.com` だけ | なし | `EXT_TODOIST_ENABLED=1`・`EXT_TODOIST_CLIENT_ID` / `_CLIENT_SECRET` |
| `todoist` の `projects`(プロジェクトの一覧・#1681) | 無料・OAuth(`runs` だけ・scope `data:read`) | `projects(limit?)`。`limit` 1〜50(既定 20)。行はプロジェクト(見出し = 名前。`listTasks` の `project` に渡せる・リンク) | 同上 | なし | 同上 |
| `todoist` の `addTask`(タスクを足す・#1496) | 無料・OAuth(`runs` だけ・scope `task:add`)・書く(確認必須) | `addTask(content, description?, dueString?, dueDate?, priority?, projectId?)`。答えは `resultId` と 1 件 | 同上 | なし | 同上 |
| `todoist` の `complete`(タスクを完了にする・#1681) | 無料・OAuth(`runs` だけ・scope `data:read_write`。Todoist にこれより狭い scope が無いので、初めて使う時につなぎ直す)・書く(確認必須) | `complete(taskId)`。`taskId` は英数字と `_` `-` の 64 文字まで。答えは `resultId`(= `taskId`)だけ。完了済みをもう一度完了にしても変わらない(やり直しても二重にならない)。持っていないタスク(Todoist の 404)は 400 `bad_request` | 同上 | なし | 同上 |
| `notion` の `queryDatabase` / `addRow`(データベースを見る・足す・#1497) | 無料・OAuth(`runs` だけ)・`addRow` は書く(確認必須) | `queryDatabase(databaseId, limit?, subtitleProperty?, valueProperty?)`・`addRow(databaseId, title, properties?)` | `https://api.notion.com` だけ | なし | `EXT_NOTION_ENABLED=1`・`EXT_NOTION_CLIENT_ID` / `_CLIENT_SECRET` |
| `notion` の `updateProperties`(ページのプロパティを直す・#1680) | 無料・OAuth(`runs` だけ)・書く(確認必須) | `updateProperties(pageId, properties)`。`pageId` は 32 桁の 16 進(ハイフン有り無しどちらも)。`properties` は「プロパティ名 → 値」の JSON(1〜20 個・8 KiB まで)。直せる型は `title`・`rich_text`・`number`・`select`・`checkbox`・`date` だけで、ほか(`status`・`relation`・`people` など)・ページに無い名前・型に合わない値は Notion に送らず 400 `bad_request`。値の読み方はページの型で決める(ページを 1 回読んでから直す)。同じ値でやり直しても変わらない。答えは `resultId`(ページ id)と 1 件(見出し = タイトル・リンク) | 同上 | なし | 同上 |
| `notion` の `pageText`(ページの本文を読む・#1680) | 無料・OAuth(`runs` だけ) | `pageText(pageId)`。ページの一番上の段のブロックを文字だけにする(見出し・段落・引用・コールアウト・トグル・コード・箇条書き `•`・番号付き `1.`・ToDo `☑` / `☐`・子ページの名前・区切り `---`。画像や表など文字の無いもの・入れ子の中身は読まない)。答えは 1 件(見出し = ページのタイトル・値 = 本文・リンク)。本文は 64 KiB(`MaxBody` の 1/16)か 5 回の読み出し(500 ブロック)で切り、切った時は末尾に `…` | 同上 | なし | 同上 |
| `googleTasks` の `lists`(ToDo のリストの一覧・#1677) | 無料・OAuth(`runs` だけ・scope `tasks.readonly`) | `lists(limit?)`。`limit` 1〜50(既定 20)。行はリスト(見出し = 名前・値 = リストの id。`tasks` / `addTask` / `complete` の `listId` に渡せる) | `https://tasks.googleapis.com` と `https://oauth2.googleapis.com` だけ | なし | `EXT_GOOGLE_TASKS_ENABLED=1`。OAuth のクライアントは Google カレンダーのもの(`EXT_GOOGLE_CALENDAR_CLIENT_ID` / `_CLIENT_SECRET`)。`EXT_GOOGLE_TASKS_CLIENT_ID` / `_CLIENT_SECRET` を入れた時だけそちら(`Provider.ClientFrom`) |
| `googleTasks` の `tasks`(リストのタスクを見る・#1677) | 無料・OAuth(`runs` だけ・scope `tasks.readonly`) | `tasks(listId?, due?, status?, limit?)`。`listId` は英数字と `_` `-` の 128 文字まで(無ければ既定のリスト `@default`)。`due` は `today`(今日・日本時間)・`overdue`(今日より前)・`week`(今日から 7 日)・`YYYY-MM-DD`(その日)。`status` は `open`(既定・まだのもの)・`completed`(済んだもの)・`all`。`limit` 1〜50(既定 20)。行はタスク(見出し = タイトル・補足 = メモの 1 行目(120 文字まで。済んだものは頭に「完了」)・値 = 期限の日・リンク) | 同上 | なし | 同上 |
| `googleTasks` の `addTask`(タスクを足す・#1677) | 無料・OAuth(`runs` だけ・scope `tasks`。Google にこれより狭い書く scope が無い)・書く(確認必須) | `addTask(title, notes?, due?, listId?)`。`title` 1024 文字まで・`notes` 8192 文字まで(制御文字は不可)・`due` は `YYYY-MM-DD`(Google は日付だけ持つ)。答えは `resultId`(タスクの id)と 1 件。Google に冪等のキーが無いので、二重にしないのは書く op をやり直さない決まりだけ | 同上 | なし | 同上 |
| `googleTasks` の `complete`(タスクを済みにする・#1677) | 無料・OAuth(`runs` だけ・scope `tasks`)・書く(確認必須) | `complete(taskId, listId?)`。id は英数字と `_` `-` の 128 文字まで。答えは `resultId`(= `taskId`)と 1 件。済みをもう一度済みにしても変わらない。持っていないリスト・タスク(Google の 400 / 404)は 400 `bad_request` | 同上 | なし | 同上 |
| `github` の `contributions`(草・#1674) | 無料・OAuth(GitHub App の利用者のトークン。scope は無く、App の権限で決まる・下の注) | `contributions(from?, to?)`。日付は `YYYY-MM-DD`(日本時間)・既定は今日までの 7 日・`to` は今日まで・`from` から `to` は 366 日まで・2008 年から。GraphQL の `contributionsCollection`。行は 1 日 1 行で古い順(見出し = 日付・値 = 数)、1 件(`item`)は期間の合計(見出し = 「from〜to」・値 = 合計)。private の分は本人のプロフィールの設定どおりに数える | `https://github.com`(トークン)・`https://api.github.com` だけ | なし | `EXT_GITHUB_ENABLED=1`・`EXT_GITHUB_CLIENT_ID` / `_CLIENT_SECRET` |
| `github` の `pullRequests`(レビュー待ち・自分の PR・#1674) | 同上・読むだけ | `pullRequests(role?, state?, limit?)`。`role` は `reviewRequested`(既定・レビューを頼まれた)か `author`(自分の)。`state` は `open`(既定)・`closed`・`merged`・`all`。`limit` 1〜50(既定 20)。検索(`GET /search/issues`・`is:pr`・アーカイブは除く・更新の新しい順)。行は PR(見出し = 題名・補足 = `owner/repo#番号`・値 = 更新日時・リンク)、1 件(`item`)の値は件数(GitHub の上限 1,000) | 同上 | なし | 同上 |
| `github` の `issues`(担当の Issue・#1674) | 同上・読むだけ | `issues(state?, limit?)`。`state` は `open`(既定)・`closed`・`all`。`limit` 1〜50(既定 20)。検索(`is:issue assignee:@me`)。行と件数は `pullRequests` と同じ形 | 同上 | なし | 同上 |
| `linear` の `myIssues`(自分の Issue・#1684) | 無料・OAuth(`runs` だけ・scope `read`) | `myIssues(state?, limit?)`。`state` は `open`(既定・完了と取り消し以外)・`all`・状態の種類(`triage`・`backlog`・`unstarted`・`started`・`completed`・`canceled`)。`limit` 1〜50(既定 20)。担当の Issue を更新の新しい順。行は Issue(見出し = 題名・補足 = 「`ENG-12` · 状態の名前」・値 = 期限・リンク) | `https://api.linear.app` だけ | なし | `EXT_LINEAR_ENABLED=1`・`EXT_LINEAR_CLIENT_ID` / `_CLIENT_SECRET` |
| `linear` の `createIssue`(Issue を足す・#1684) | 無料・OAuth(`runs` だけ・scope `issues:create`。つなぎ直す時も `read` を一緒に頼む)・書く(確認必須) | `createIssue(title, description?, team?)`。`title` は 1 行・255 文字まで・`description` は Markdown・10,000 文字まで。`team` はチームのキー(`ENG`・英字で始まる 7 文字まで)かチームの id。書かなければ本人のチームが 1 つの時だけそこ(2 つ以上は 400 `bad_request`)。無いチーム・Linear が受けない入力(`INVALID_INPUT`)も 400。状態はチームの最初の状態(Backlog か Triage)。答えは `resultId`(`ENG-123`)と 1 件 | 同上 | なし | 同上 |
| `youtube` の `channelStats`(チャンネルの公開の数字・YouTube Data API v3・#1699) | 無料・運営の鍵(`runs` だけ。YouTube の 1 日の割り当てで 1 回 1 単位) | `channelStats(channel)`。`channel` はチャンネル id(`UC` + 22 文字)か `@ハンドル`(3〜30 文字・文字と数字と `.` `_` `-`)。答えは 1 件(見出し = チャンネル名・値 = 「登録者 n 人」・補足 = 「動画 n 本・再生 n 回」・リンク = チャンネル)。数は API の値をそのまま 3 桁区切りで出す(丸めない・作り直さない)。登録者を隠しているチャンネルは「登録者 非公開」。無いチャンネルは 400 `bad_request`。出典「YouTube」を出す(下の「YouTube の規約」)。AI に選ばせる決まり(`prompt.go`・`catalog.go`)はまだ | `https://www.googleapis.com` だけ(`/youtube/v3/channels`)・鍵は `X-Goog-Api-Key` ヘッダー | 1 時間 | `EXT_YOUTUBE_ENABLED=1`・`EXT_YOUTUBE_API_KEY` |
| `youtube` の `latestVideos`(チャンネルの新しい動画・#1699) | 同上(1 回 2 単位: チャンネル + アップロードの再生リスト。`search` の 100 単位は使わない) | `latestVideos(channel, limit?)`。`channel` は同上・`limit` 1〜20(既定 5)。行は公開の動画だけ・新しい順(見出し = 題・補足 = 公開の日時(日本時間)・リンク = 動画)。動画が 1 本も無いチャンネルは 0 行 | 同上(`/youtube/v3/channels`・`/youtube/v3/playlistItems`) | 30 分 | 同上 |
| `podcasts` の `searchPodcasts`(ポッドキャストの番組を探す・Apple の iTunes Search API・#1703) | 無料・鍵なし(`runs` だけ) | `searchPodcasts(term, limit?)`。`term` は 1〜100 文字・1 行(URL は 400)・`limit` 1〜20(既定 10)。日本のストア(`country=JP`・`lang=ja_jp`)の番組(`media=podcast`・`entity=podcast`)。行は番組ごと(見出し = 番組名・補足 = 作者・値 = 「RSS あり」/「RSS なし」・リンク = Apple Podcasts の番組のページ(クエリは落とす))。**RSS の URL そのものは返さない**(新しい回を読むのは #1702 の決定待ちで、URL は引数にも渡せないため)。アートワークの URL(`*.mzstatic.com`)と番組の id は `Output.Cells`(サーバーの中だけ)にあり、端末の行の形(`ExtRow`)に画像の欄が無いので今は端末に出ない。ストアのページが無い・`https` でない結果は出さない。出典 `PodcastsAttribution`(「provided courtesy of iTunes」)を出す(下の「iTunes Search API の規約」)。AI に選ばせる決まり(`prompt.go`・`catalog.go`)はまだ | `https://itunes.apple.com` だけ(`/search`) | 6 時間 | `EXT_PODCASTS_ENABLED=1` |
| `withings` の `weight`(体重計・#1669) | 無料・OAuth(`runs` だけ・scope `user.metrics`)・読むだけ | `weight(days?, limit?)`。`days` 1〜90(既定 30・何日前まで)・`limit` 1〜50(既定 20)。行は 1 回の計測(見出し = 日時(日本時間)・値 = `65.75 kg`・補足 = 体脂肪率(測った時だけ))。新しい順。本物の計測(category 1)だけで、体重計が家族の誰のものか決められなかった計測(attrib 1)は出さない | `https://wbsapi.withings.net` だけ | なし(健康のデータ・利用者ごと) | `EXT_WITHINGS_ENABLED=1`・`EXT_WITHINGS_CLIENT_ID` / `_CLIENT_SECRET` |
| `withings` の `bloodPressure`(血圧計・#1669) | 同上 | `bloodPressure(days?, limit?)`。引数は `weight` と同じ。行は 1 回の計測(見出し = 日時・値 = `128/82 mmHg`(上/下)・補足 = 脈拍(測った時だけ))。上と下の両方が無い計測は出さない | 同上 | なし | 同上 |
| `slack` の `setStatus`(自分の状態を変える・#1679) | 無料・OAuth(`runs` だけ・ユーザーのトークン・scope `users.profile:write`)・書く(確認必須) | `setStatus(text?, emoji?, minutes?)`。`text` は 1 行 100 文字まで・`emoji` は `:tomato:` の形(肌の色 `:skin-tone-2:`〜`6` を 1 つ付けてよい)・`minutes` は 1〜10080(1 週間。無ければ消すまで)。`text` も `emoji` も無ければ状態を消す(その時 `minutes` は付けない)。同じ状態をもう一度入れても変わらない。答えは 1 件(見出し = 文・補足 = 絵文字・値 = 消える時刻)。Slack の上限は 1 人の状態を 1 分に 10 回まで | `https://slack.com` だけ(`/api/...`) | なし | `EXT_SLACK_ENABLED=1`・`EXT_SLACK_CLIENT_ID` / `_CLIENT_SECRET` |
| `slack` の `postMessage`(チャンネルに本人として投稿・#1679) | 無料・OAuth(`runs` だけ・ユーザーのトークン・scope `chat:write`)・書く(確認必須) | `postMessage(channel, text)`。`channel` は会話の id(`C…` / `G…` / `D…`・名前は取らない)・`text` は 4000 文字まで(改行可・制御文字不可)。`<!channel>` `<!here>` `<!subteam^…>` などの一斉メンション(`<!`)は Slack に送らず 400 `bad_request`。`parse: none`・`link_names: false` で送る。本人が入っていない・無いチャンネル(`not_in_channel` / `channel_not_found`)は 400 `bad_request`。Slack に冪等のキーが無いので、二重にしないのは書く op をやり直さない決まりだけ。答えは `resultId`(メッセージの `ts`)と 1 件(見出し = 1 行目 120 文字まで・補足 = チャンネル)。本文は保存しない。Slack の上限は 1 チャンネルに 1 秒 1 通 | 同上 | なし | 同上 |
| `slack` の `unread`(DM の未読の数・#1679) | 無料・OAuth(`runs` だけ・ユーザーのトークン・scope `im:read`) | `unread(channel)`。`channel` は DM の id(`D…`)だけ。`conversations.info` の `unread_count_display` を返す(Web API が未読の数を返すのは DM だけ。チャンネルの未読を数えるには本文を `conversations.history` で読むことになり、Marketplace に無い配布アプリは 1 分 1 回・15 件までなので入れない)。数が無い答えは 400 `bad_request`。答えは 1 件(見出し = 「未読」・補足 = id・値 = 数)。本文は読まない | 同上 | なし | 同上 |
| `googleSheets` の `readRange`(スプレッドシートの範囲を見る・#1685) | 無料・OAuth(`runs` だけ・scope `drive.file`。利用者が Google Picker で選んだファイルとアプリが作ったファイルだけ・センシティブでない) | `readRange(spreadsheetId, range, limit?)`。`spreadsheetId` は英数字と `_` `-` の 20〜128 文字(Picker で選んだファイルの id)。`range` はシート名か A1 の範囲(`Sheet1!A1:D50`・`'売上'!A:D`・シート名を省くと最初のシート)で 200 文字まで。`limit` 1〜100(既定 50)。行の範囲が無い・`limit` より広い範囲は `limit` 行に縮めて頼む。行はシートの 1 行(見出し = 1 列目・値 = 2 列目・補足 = 3〜10 列目を「・」でつなぐ。11 列目からは捨てる・空の行は飛ばす)。選んでいないファイル(Google の 404)は 400 `bad_request`(「Picker で選んで」の理由付き)・無いシートや範囲(400)も 400 `bad_request` | `https://sheets.googleapis.com` と `https://oauth2.googleapis.com` だけ | なし(シートの中身を持たない) | `EXT_GOOGLE_SHEETS_ENABLED=1`。OAuth のクライアントは Google カレンダーのもの(`EXT_GOOGLE_CALENDAR_CLIENT_ID` / `_CLIENT_SECRET`)。`EXT_GOOGLE_SHEETS_CLIENT_ID` / `_CLIENT_SECRET` を入れた時だけそちら(`Provider.ClientFrom`) |
| `googleSheets` の `appendRow`(スプレッドシートに 1 行足す・#1685) | 無料・OAuth(`runs` だけ・scope `drive.file`)・書く(確認必須) | `appendRow(spreadsheetId, range, values)`。`range` は `readRange` と同じ(表のある範囲。その下に足す)。`values` は文字列の JSON 配列(1〜26 個・1 つ 1000 文字まで・改行とタブ以外の制御文字は不可)。値はそのまま入れる(`RAW`。`=` で始まっても式にしない)。答えは `resultId`(書いた範囲・例 `Sheet1!A5:C5`)と 1 件(リンク = そのシート)。Google に冪等のキーが無いので、二重にしないのは書く op をやり直さない決まりだけ | 同上 | なし | 同上 |
| `odpt` の `trainStatus`(電車の運行情報・公共交通オープンデータセンター・#1691) | 無料・運営の鍵(`runs` だけ。ODPT の開発者登録の consumer key) | `trainStatus(operator?, railway?)`。`operator` は ODPT の事業者 id・`railway` は路線 id で、どちらも下の「ODPT の事業者と規約」の許可リストだけ(ほかは 400 `bad_request`)。両方あれば路線がその事業者のものであること。無ければ許可リストの全事業者。行は路線ごと(見出し = 路線名(路線の無い事業者全体の情報は事業者名)・補足 = 「状態: 案内の文」(状態が無ければ文だけ・200 文字で切る)・値 = 「M/D HH:MM 更新」(`dc:date`・日本時間))。並びは許可リストの順。`dct:valid` を過ぎた行・時刻の無い行・許可リストに無い事業者の行は出さない。出典(下の表)を出す。AI に選ばせる決まり(`prompt.go`・`catalog.go`)はまだ | `https://api.odpt.org` だけ(`/api/v4/odpt:TrainInformation`)・鍵は `acl:consumerKey` のクエリ(API の決まり。エラーに URL は出さない) | 1 分 | `EXT_ODPT_ENABLED=1`・`EXT_ODPT_API_KEY` |

Google(カレンダー・ToDo・スプレッドシート)の 401 は接続を `error` にする(つなぎ直す)。403 は理由で分ける: scope が足りない(`insufficientPermissions`・`ACCESS_TOKEN_SCOPE_INSUFFICIENT`)だけ 409 `ext_not_connected`(つなぎ直す)。回数の上限(`rateLimitExceeded`・`userRateLimitExceeded`・`quotaExceeded`・`dailyLimitExceeded`)は 429 として扱い、502 `upstream_unavailable` にして provider を止める(次の呼び出しは 429 `rate_limited` と `retryAfterSec`)。キーで呼ぶ YouTube などの 403 も同じ。API が有効でない(`accessNotConfigured`・`SERVICE_DISABLED`)はこちらの設定なので 502 `upstream_unavailable`。ほかの 403(そのリスト・シートを使えない)は ToDo とスプレッドシートでは 400 `bad_request`(つなぎ直しても直らない)。429 / 5xx は 502 `upstream_unavailable`。

Asana の 403 も、メッセージが scope の不足を言う時だけ 409 `ext_not_connected`。ほかの 403(そのプロジェクトを見られないなど)は 400 `bad_request`。

Slack は利用者ごとの OAuth v2 でユーザーのトークン(`user_scope` だけ・ボットのトークンは取らない)。PKCE は使わない(Slack の PKCE はアプリを公開クライアントに戻せない形で変えるため。runner はクライアントの秘密を Basic 認証で送る)。新しい scope は同じトークンに足されるので、書く op のためにつなぎ直しても読む op は残る。トークンの回転(12 時間・使い捨ての refresh token)を入れた時も入れない時も動く。Slack はほとんどの失敗を 200 の `ok:false` で返すので、コードで分ける: `invalid_auth`・`not_authed`・`token_revoked`・`token_expired`・`account_inactive`(と HTTP 401)は接続を `error` にする(つなぎ直す)。`missing_scope`・`not_allowed_token_type`・`no_permission`・`access_denied` など(と HTTP 403)は 409 `ext_not_connected`。`ratelimited`(HTTP 429)・`fatal_error`・`internal_error`・`service_unavailable`・`request_timeout`(と 5xx)は 502 `upstream_unavailable`。ほかのコードは 400 `bad_request`。Slack の API 規約: 商用の配布は Marketplace(か別の契約)が要る・データは機能に要る間だけ持つ・モデルの学習に使わない・プライバシーポリシーに読むものを書く。だから op は本人の状態・本人の投稿・DM の未読の数だけで、メッセージの本文を読む op・ためる op は入れない。

Asana の `myTasks`・`projectTasks`・`complete`、Todoist の `complete`・`projects`・`project` / `label`、Notion の `updateProperties`・`pageText`、Google ToDo の全部の op、GitHub と Linear の op、YouTube の `channelStats`・`latestVideos`、ポッドキャストの `searchPodcasts`、Slack の全部の op、ODPT の `trainStatus` は runner の op だけ。生成アプリから呼ぶ部品と AI に選ばせる決まり(`prompt.go`・`catalog.go`・`docs/dsl`)は #1643 の判断の後に足す。

provider が 404 / 422(持っていない通貨・日)を返した時は 400 `bad_request`(`runs` は戻す)。429 / 5xx は 502 `upstream_unavailable`。

### Trello を入れていない理由(#1682・設計のメモ)

Trello の認可は今の接続の形(`OAuthClient`: 同意の URL を作る → 戻り先の `state` と `code` を受けて交換 → 更新 → 取り消し)に合わないので、runner の provider は作っていない。

| Trello の方式 | 合わないところ |
|---|---|
| `/1/authorize`(API キー + 利用者のトークン) | トークンは戻り先の URL の `#` の後ろ(`callback_method=fragment`)か `postMessage` で返る。`#` の後ろはサーバーに届かないので、runner の `/oauth/callback/{provider}` では受け取れない。`state` も無い |
| OAuth 1.0a(`OAuthGetRequestToken` → `OAuthAuthorizeToken` → `OAuthGetAccessToken`) | 同意の URL を作る前に runner からリクエストトークンを取る(`AuthURL` は通信しない作り)。戻り先は `oauth_token` と `oauth_verifier` で `state` / `code` ではない。リクエストトークンの秘密を state と一緒に持つ所が要る。署名(HMAC-SHA1)も要る |

入れる時の案(どちらもオーナーの判断が要る):

1. **OAuth 1.0a を足す(推奨)**: `OAuthClient` に「始める時に通信してよい」版(`Begin(ctx, state, redirectURI) (authURL, secret string, err error)`)を足し、`OAuthState` にリクエストトークンの秘密を暗号化して持つ。戻り先は `oauth_token` と `oauth_verifier` も受ける。手に入るトークンは期限なし(`expiration=never`)か 30 日で、更新は無い(切れたらつなぎ直す)。API は `Authorization: OAuth oauth_consumer_key="<API キー>", oauth_token="<トークン>"` で呼ぶ(URL にトークンを載せない)。取り消しは `DELETE /1/tokens/{token}`。
2. **`#` を読む小さな戻り先のページ**: runner が返す静的な HTML が `#token=` を読み、`state` と一緒に runner へ POST する。トークンがブラウザーの履歴・JS を通るので 1 より弱い。

op の案: `boards()`(自分のボード・`GET /1/members/me/boards`。`/1/members` は 900 秒に 100 回と厳しい)・`cards(boardId? / listId?)`(`GET /1/lists/{id}/cards`・`GET /1/boards/{id}/cards`)・`addCard(listId, name, desc?, due?)`(書く・確認必須・`POST /1/cards`)。scope は `read`(足す時だけ `write`。`account` は要らない)。出口は `https://api.trello.com`(認可は `https://trello.com` でブラウザーだけ)。回数: API キーごとに 10 秒 300 回・トークンごとに 10 秒 100 回(429 の本文 `API_TOKEN_LIMIT_EXCEEDED` / `API_KEY_LIMIT_EXCEEDED`)。規約: Atlassian Developer Terms(利用者の同意・データを売らない・Trello の機能そのものに課金しない・セキュリティの事故は 48 時間以内に知らせる)。

### GitHub と Linear の注(#1674・#1684)

- **GitHub は GitHub App**(OAuth App ではない)。OAuth App の `repo` scope は private の repo に書く権限まで付くので使わない。GitHub App は scope が無く、App に付けた権限(Issues: 読む・Pull requests: 読む・Metadata: 読む)と、本人が App を入れた repo にだけ届く(private の repo は入れたものだけ読める)。PKCE は S256。利用者のトークンは 8 時間・リフレッシュトークンは 6 か月で、使うたびに入れ替わる。トークンの口は失敗も 200 と `{"error"}` で返す(`bad_verification_code` / `bad_refresh_token` は つなぎ直し)。切る時は `DELETE /applications/{client_id}/grant`(本人の許可ごと消える)。
- **GitHub の上限と決まり**: 利用者 1 人 1 時間 5,000 回(本人のほかのトークンと共有)・検索は 1 分 30 回・同時に 100 本まで。GitHub は 1 人ぶんを並べて呼ばないよう求めるので、同じトークンの呼び出しは台の中で 1 本ずつにする。上限の 403(`x-ratelimit-remaining: 0` か `retry-after` つき)と 429 と GraphQL の `RATE_LIMITED` は 429 として扱い、provider を 1 分止める(上限を超えて呼び続けると連携を止められることがある)。上限でない 403 は App の権限が足りない(つなぎ直し)。`User-Agent` を必ず付け、`X-GitHub-Api-Version: 2026-03-10`。検索は GitHub App のトークンでは `is:pr` / `is:issue` が要る(無いと 422)。
- **Linear**: scope は `read`(既定)と `issues:create`(Issue を足すだけ。直す・消すはできない)。`write`・`admin` は頼まない。PKCE は S256・`actor=user`。アクセストークンは 24 時間・リフレッシュトークンは 30 日で入れ替わる。切る時は `POST /oauth/revoke` にリフレッシュトークン(無ければアクセストークン)。400(もう取り消し済み)・401 は済んだものとする。
- **Linear の上限**: OAuth の App は利用者 1 人 1 時間 5,000 回・複雑さ 2,000,000 点(1 回 10,000 点を超える問い合わせは断られる)。上限は 429 と `RATELIMITED`。問い合わせは見せる項目だけ・50 件までにする。Linear はエラーを 200 / 400 と `extensions.code` で返すので、`AUTHENTICATION_ERROR` は接続のエラー、`FORBIDDEN` はつなぎ直し、`INVALID_INPUT` は 400 にする(エラーの code の一覧は公開の文書に無いので、知らない code は 502 `upstream_unavailable`)。

### Withings の注(#1669)

- **認可**: `https://account.withings.com/oauth2_user/authorize2`(scope はカンマ区切り。`user.metrics` だけ頼む)。Withings は PKCE を受けないので、コードの引き換えはクライアントシークレットで runner がする。アクセストークンは 3 時間・リフレッシュトークンは使うたびに入れ替わる。
- **答えの形**: Withings は失敗も HTTP 200 と本文の `status` で返す(0 = 成功)。トークンを受けない(100〜102・200・401)は接続のエラー、トークンの口の 503(引数が正しくない = コードかリフレッシュトークンが無効)はつなぎ直し、601(回数の上限)は 429 として扱う。2555(Withings 側の不明なエラー)と 5xx はやり直せるエラー、ほかの status は 502 `upstream_unavailable`。
- **数の扱い**: 値は `value × 10^unit` を丸めずにそのまま書く(`65750`・`-3` → `65.75`)。`Cells` は取り込み(#1712)用で、体重は「日時(RFC 3339)・kg・体脂肪率」、血圧は「日時・上・下・脈拍」。
- **切る時**: Withings の取り消しは Withings の利用者 id と署名つきの nonce が要り、今の `Token` に利用者 id の置き場が無いので、runner のトークンを消すだけ(`ErrNoRevoke`)。Withings 側の許可は利用者が Withings の設定から外す。
- **ヘルスケア / Health Connect との関係**: 端末の体重は今もヘルスケア(`healthStat` の `weight`)で読める。Withings のアプリはヘルスケア / Health Connect に体重と血圧を書けるので、血圧も端末から読めば Withings をつながなくて済むことがある。どちらを使うかはオーナーが決める(この provider は既定 off)。

### 入れないもの(黄砂・そらまめ君)

#1656 で調べ、外の API としては入れない。

| もの | 入れない理由 |
|---|---|
| 黄砂(気象庁の黄砂情報) | 予測図が画像で出るだけで、数値で読める公式の API・使い方の決まり(仕様と規約)が無い。画像を読み取るのは壊れやすく、出典の決まりの外になる。数値の公式の出どころができたら入れ直す |
| 環境省「そらまめ君」(大気汚染物質広域監視システム) | 公式の API の仕様が無く、画面とデータのダウンロードだけ。機械から呼ぶ使い方の規約も無いので、runner から定期に読む形にできない。PM2.5 は上の `airQuality` で見る |


### YouTube の規約(#1699)

[YouTube API サービス利用規約](https://developers.google.com/youtube/terms/api-services-terms-of-service)と[開発者ポリシー](https://developers.google.com/youtube/terms/developer-policies)(2026-11-12 に読んだ)から、`youtube` を出す時に守ること。

| 決まり | 中身 | この作りでの扱い |
|---|---|---|
| 出どころを見せる(規約 10.3・ポリシー III.F.2) | YouTube の中身を出す画面は、YouTube が出どころだと分かるように YouTube のブランド(Branding Guidelines)を出す。他の出どころとまとめた 1 つの出典にしない | 出典「YouTube」(`YouTubeAttribution`)を数字・行のそばに出す。ロゴの出し方は部品(#1643)と iOS で決める |
| 数字をそのまま出す(ポリシー III.E.4.h・III.E.4.f) | API の数字を、自分で計算した別の数字に置き換えない。出すのは手に入る一番新しい値。過去の値は「いつの値か」を添えるなら出せる | 数は API の値をそのまま(3 桁区切りだけ)。隠された登録者は出さない |
| 持つ期間(ポリシー III.E.4.b・c・d・e) | 許可されていないデータ(運営の鍵で読む公開の数字)は 30 日まで。統計(登録者の数など)も 30 日を超えて持たない。持つ間も今の値に合わせる努力をする | キャッシュは 1 時間 / 30 分だけ。**生成アプリが毎日の登録者を記録して 30 日より長く持つのは規約の外**。推移を出す部品は 30 日で古い記録を消す形にする(#1643 で決める) |
| 規約へのリンク(ポリシー III.A.1) | YouTube の利用規約(https://www.youtube.com/t/terms)へのリンクを出す | 部品と iOS の作業(#1643)。`docs/legal` にも書く(人の作業) |
| プライバシーポリシー(規約 7・ポリシー III.A.2) | YouTube API サービスを使うと書き、Google のプライバシーポリシー(https://policies.google.com/privacy)へリンクする | 人の作業(`docs/legal`) |
| 割り当て(規約 15・ポリシー III.D.3) | 割り当てを超えない・避けない。増やすには API Compliance Audit。認められた使い道にだけ使う | 1 回 1〜2 単位。回数の上限 `EXT_GLOBAL_DAILY_YOUTUBE` を 1 日の割り当てに合わせる(上限は回数で数えるので、`latestVideos` を 2 単位として見積もる・人の作業) |

### iTunes Search API の規約(#1703)

[Search API の説明](https://performance-partners.apple.com/search-api)(2026-11-12 に読んだ)から、`podcasts` を出す時に守ること。

| 決まり | 中身 | この作りでの扱い |
|---|---|---|
| 回数 | 「おおよそ 1 分 20 回(変わることがある)」。多く使うなら Enterprise Partner Feed。大きなサイトは検索の答えを持つ(キャッシュする) | キャッシュは 6 時間(同じ `term`・`limit`)。runner の台は出口の IP を共有するので、`EXT_GLOBAL_DAILY_PODCASTS` で 1 日の全体を絞る(人の作業) |
| 使い道 | 中身はストアの中身を広めるためだけ・広める中身のページにだけ置く・別の商品やサービスの宣伝に使わない・それだけで楽しむ使い方にしない | 行は必ずその番組の Apple Podcasts のページへリンクする(ページの無い結果は出さない)。「通勤のお供」の一覧のように番組を開きに行く使い方にする |
| バッジ | 「Download on iTunes」などの Apple が認めたバッジを近くに置き、ストアのページへリンクする | 出し方(バッジの画像・Listen on Apple Podcasts)は部品(#1643)と iOS で決める |
| 出典 | 試聴(preview)は「provided courtesy of iTunes」と出す | 試聴は使わないが、行のそばに `PodcastsAttribution`(同じ文)を出す |
| 音声 | 試聴は流すだけで、ダウンロード・保存・キャッシュしない | 音声のファイルはサーバーで読まない・置かない。再生は Podcasts アプリを開く |
| 求めに応じて消す | Apple から求められたら中身をすぐ消す・苦情は Apple に回す | キャッシュは 6 時間で消える。止める時は `EXT_PODCASTS_ENABLED` を切る |

**新しい回(番組の RSS の `enclosure`)はまだ作らない。** 利用者やアプリが渡す RSS の URL を runner から読むかは #1702 の決定待ち。決まったら、Search API の lookup(`/lookup?id=<番組の id>`)で RSS の URL をサーバーの中で引く形か、許可リストの形で足す(番組の id は今 `Output.Cells` に入れてある)。

### ODPT の事業者と規約(#1691)

[公共交通オープンデータセンター利用規約](https://developer.odpt.org/terms/center_use_rules.html)・[公共交通オープンデータ基本ライセンス](https://developer.odpt.org/terms/data_basic_license.html)・[開発者ガイドライン](https://developer.odpt.org/terms/data_basic_use_guideline.html)・[チャレンジ限定ライセンス](https://developer.odpt.org/challenge_license)と、[データカタログ](https://ckan.odpt.org/dataset?tags=%E9%81%8B%E8%A1%8C%E6%83%85%E5%A0%B1-train_status_information)の各事業者の「運行情報」のライセンス(2026-11-12 に読んだ)から決めた。

入れた事業者(許可リスト・`odpt.go` の `odptOperators`):

| 事業者 id | 路線 id(`odpt.Railway:` の後) | ライセンス | データセット |
|---|---|---|---|
| `odpt.Operator:TokyoMetro` | `TokyoMetro.Ginza`・`.Marunouchi`・`.MarunouchiBranch`・`.Hibiya`・`.Tozai`・`.Chiyoda`・`.Yurakucho`・`.Hanzomon`・`.Namboku`・`.Fukutoshin` | 基本ライセンス | [東京メトロ 運行情報](https://ckan.odpt.org/dataset/r_train_status-tokyometro) |
| `odpt.Operator:Toei` | `Toei.Asakusa`・`.Mita`・`.Shinjuku`・`.Oedo`・`.Arakawa`・`.NipporiToneri` | CC BY 4.0 | [東京都交通局 運行情報](https://ckan.odpt.org/dataset/r_train_status-toei) |
| `odpt.Operator:TWR` | `TWR.Rinkai` | 基本ライセンス | [東京臨海高速鉄道 運行情報](https://ckan.odpt.org/dataset/r_train_status-twr) |
| `odpt.Operator:MIR` | `MIR.TsukubaExpress` | 基本ライセンス | [首都圏新都市鉄道 運行情報](https://ckan.odpt.org/dataset/r_train_status-mir) |
| `odpt.Operator:TamaMonorail` | `TamaMonorail.TamaMonorail` | 基本ライセンス | [多摩都市モノレール 運行情報](https://ckan.odpt.org/dataset/r_train_status-tamamonorail) |
| `odpt.Operator:YokohamaMunicipal` | `YokohamaMunicipal.Blue`・`.Green` | 基本ライセンス | [横浜市交通局 運行情報](https://ckan.odpt.org/dataset/r_train_status-yokohamamunicipal) |

入れない事業者(運行情報が「公共交通オープンデータチャレンジ限定ライセンス」。チャレンジの期間中・作品の応募が前提で、常に動くアプリには使えない):

| 事業者 | データセット |
|---|---|
| JR 東日本(JR東日本アイステイションズ提供・特定利用条件つき) | [JR東日本アイステイションズ 運行情報](https://ckan.odpt.org/dataset/jr-train-status-information-by-jr-east-i-stations) |
| 東急電鉄 | [東急電鉄 運行情報](https://ckan.odpt.org/dataset/tokyu__r_train_status) |
| 東武鉄道 | [東武鉄道 運行情報](https://ckan.odpt.org/dataset/tobu__r_train_status) |
| 京王電鉄 | [京王電鉄 運行情報](https://ckan.odpt.org/dataset/keio__r_train_status) |
| 西武鉄道 | [西武鉄道 運行情報](https://ckan.odpt.org/dataset/seibu__r_train_status) |
| 京急電鉄 | [京急電鉄 運行情報](https://ckan.odpt.org/dataset/keikyu__r_train_status) |

ライセンスが基本ライセンスか CC BY に変わったら、カタログで確かめてから許可リストに足す(コードと上の表を一緒に直す)。

| 決まり | 中身 | この作りでの扱い |
|---|---|---|
| 利用者への通知(ガイドライン「利用者への通知」) | センターのデータであること・正確・完全とは限らないこと・事業者に直接問い合わせないこと・アプリの問い合わせ先を出す | 出典 `ODPTAttribution`(「公共交通オープンデータセンターのデータです。正確・完全とは限りません。遅れの情報は事業者の公式の案内を確かめてください(事業者への問い合わせはしないでください)」)を行のそばに出す。アプリの問い合わせ先の出し方は部品(#1643)と iOS で決める |
| CC BY のクレジット(都営・FAQ) | 「提供者名、名称、CC BY 4.0(リンク)」 | 都営の行を出す時は `ODPTToeiCredit`(「東京都交通局・公共交通オープンデータ協議会、運行情報、CC BY 4.0」)も出す |
| 時刻の表示(ガイドライン 2.1.1) | 動的データは `dc:date` を画面に出す | 行の値 = 「M/D HH:MM 更新」。時刻の無い行は出さない |
| 新しい情報だけ(ガイドライン 2.1.2) | `odpt:frequency` の間隔で更新・`dct:valid` を過ぎたデータは使わない | `dct:valid` を過ぎた行は出さない。キャッシュは 1 分だけで、行の一番早い `dct:valid` を過ぎたら捨てる(`Output.ValidUntil`) |
| 元の意味を変えない(基本ライセンス 4 条 2 項) | 分解・修正で意味を損なわない・成果物の外で表示しない | 案内の文はそのまま(長い時だけ末尾を切って `…`)。生のデータは端末に返さない(決まった行の形だけ) |
| 再配布しない(基本ライセンス 8 条 4 項) | データ(復元できる派生データを含む)を第三者が再利用できる形で公開しない | 生成アプリの画面に出すだけ。**生成アプリが運行情報を一覧として外に出す(書き出し・公開の API)のは規約の外**(部品の決まりは #1643) |
| 鍵(利用規約 5 条 1 項 5 号) | アクセストークンを第三者に開示しない | 鍵は Secret Manager の `EXT_ODPT_API_KEY` だけ・エラーとログに URL を出さない |
| 営利 | 基本ライセンス 4 条 7 項で営利・非営利を問わず使える | Pro の機能として出してよい |

## 呼び出しの流れと数え方

`Service.Call` の順:

| 段 | 中身 | 失敗した時 |
|---|---|---|
| 1. 確かめる | provider・op・引数・request id・止め弁・(有料で enforce)押さえの量があるか | 400 `bad_request` / 503 `ext_unavailable`。何も数えない |
| 2. 同時の数 | 1 台で `EXT_MAX_CONCURRENT` まで(tick と別) | 503 `ext_busy` |
| 2.5 回数の上限 | provider の 1 分止めの中か・利用者 / アプリ / 全体の回数(下の「回数の上限」) | 429 `rate_limited`(`scope`・`retryAfterSec`)。何も数えない |
| 3. 枠 | 作成者の Pro を確かめ、作成者の月の `runs` を 1 足す(`quota.ExtRun`。超えたら今の「超えた後のコイン払い」でブロックを買う) | 402 `pro_required` / `allowance_exhausted` / `overflow_cap_reached` / `coins_insufficient` |
| 4. トークン | OAuth の provider は**押した人**(自動の動きは**作成者**)のトークンを読む。期限切れは更新 | 409 `ext_not_connected`。`runs` を戻す |
| 5. 押さえる | 有料の provider だけ。押した人(自動の動きは作成者)のコインを `COIN_RESERVE_EXT_<PROVIDER>` だけ押さえる(op `ext`) | 402 `coins_insufficient`。同じ request id の押さえがもうある時は呼ばずに 409 `duplicate_request`。どちらも `runs` を戻す |
| 6. 呼ぶ | `EXT_TIMEOUT`(既定 8 秒) | 時間切れ・5xx は押さえを全部戻し、`runs` も戻す。provider がトークンを拒んだら接続を `error` にして 409 |
| 7. 確定 | 原価(op が測った値、無ければ `EXT_COST_USD_<PROVIDER>_<OP>`)× `COIN_PER_USD` を押さえの量まで引き、残りを戻す。どちらも無い時は押さえた量を引く(`coins.unpriced_model`)。履歴は `spend_ext`(`usd` つき) | 費用のかかった 4xx(`ProviderError.Billable`)だけは戻さず確定する |

- `COIN_BILLING_MODE=off` は枠もコインも数えない(Pro の確認だけ)。`log` は数えるが止めず、コインは `coins.would_spend` を出すだけ。
- ログは 1 回 1 行 `ext.call`(provider・op・結果・ms・`runs` か `coins` か `cache` か・引いたコイン・app id・uid のハッシュ)。引数・答え・トークンは出さない。上限で止めた時は `limit`(scope)・`limit_max`・`limit_count`・`retry_after_sec`、provider の 429 で止めた時は `paused: provider_paused` も出す(コードと数だけ)。
- **キャッシュ**: `CacheTTL` のある op(鍵だけ・OAuth なしの読む op。登録の時に確かめる)は、同じ引数の答えを台の中のメモリに持つ(1 台 2048 件まで)。キャッシュから返した時は `runs` もコインも数えない。ただし Pro の確認(`quota.ExtPro`)はする。OAuth の op・書く op は持たない。

### 回数の上限(#1715)

暴走の歯止め(`runs` とコインとは別)。provider を呼ぶ 1 回を、**provider ごと**に下の数え方で 1 数える(全部まとめて数えるか、1 つでも超えるなら何も数えない。`ratelimit.Store` の Consume)。値は env だけで決める(既定は安全側の小さめの値をコードに持つ。値はここに書かない)。0 はその数えを止める。

| scope | 数えるもの | env(全部の provider / 1 つの provider) | 戻る時 |
|---|---|---|---|
| `user_hourly` | 押した人(自動の動きは作成者)の 1 時間 | `EXT_LIMIT_USER_HOURLY` / `EXT_LIMIT_<PROVIDER>_USER_HOURLY` | 次の正時 |
| `user_daily` | 押した人の 1 日(JST) | `EXT_LIMIT_USER_DAILY` / `EXT_LIMIT_<PROVIDER>_USER_DAILY` | 次の 0:00 JST |
| `app_hourly` | アプリの 1 時間 | `EXT_LIMIT_APP_HOURLY` / `EXT_LIMIT_<PROVIDER>_APP_HOURLY` | 次の正時 |
| `app_daily` | アプリの 1 日 | `EXT_LIMIT_APP_DAILY` / `EXT_LIMIT_<PROVIDER>_APP_DAILY` | 次の 0:00 JST |
| `global_daily` | その provider の環境全体の 1 日(provider の割り当ての手前で止める) | `EXT_GLOBAL_DAILY` / `EXT_GLOBAL_DAILY_<PROVIDER>` | 次の 0:00 JST |
| `provider_paused` | provider が 429 を返した後の 1 分(台ごと・メモリ) | なし(1 分で固定) | 1 分たった時 |

- 1 つの provider の env が全部の provider の env より勝つ。env が無ければコードの既定(`DefaultLimits`)。
- 数えるのは provider を呼んだ時だけ。キャッシュから返した時は数えない。トークンが無い・コインが足りない・同じ request id・枠で止まった時(provider を呼ばなかった)は戻す。provider が失敗した時(5xx・時間切れ・429)は呼んだので戻さない。
- 文書は `rateLimits/{APP_ENV}_{日 or 日T時}_ext_{provider}_{uid_… / app_… / global}`(TTL `expireAt`)。全体の数えは `ratelimit.GlobalShards` に分ける。Firestore が落ちた時は台のメモリで数える(開けっぱなしにしない)。
- runner の答えは `{"reason": "rate_limited", "scope": "<scope>", "retryAfterSec": <秒>}`(429)。appconfig は 429 `rate_limited`・`Retry-After`・`details`(`scope`・`provider`・`retryAfterSec`)にする。
- 偽の provider は `fail=429` で 1 分止めを試せる。
- appconfig の `EXT_USER_PER_HOUR`(provider をまたいだ 1 人 1 時間)はそのまま残る。

## OAuth のトークン

| もの | 場所・中身 |
|---|---|
| トークン | `users/{uid}/connections/{provider}`: `status`(`active` / `revoked` / `error`)・`sealed`(暗号化したトークン)・`scopes`・`connectedAt`・`lastUsedAt`・`revokedAt`・`deleteAt` |
| state / PKCE | `oauthStates/{state}`: uid・provider・`code_verifier`・scope・`expireAt`(10 分)。1 回だけ使える(取り出しと削除を 1 つのトランザクションで) |

- **暗号化**: `Sealer` が暗号化する。本番は Cloud KMS の対称鍵(`KMSSealer`・`EXT_TOKEN_KMS_KEY`)で、鍵の encrypt / decrypt は runner の SA だけに渡す。uid と provider を追加の認証データにするので、別の人・別の provider の文書に写しても開けない。エミュレータでは起動ごとの使い捨ての鍵(`LocalSealer`)。鍵が無い時は OAuth の provider が動かない(暗号化しないで保存する道は無い)。
- **クライアントから見えない**: Firestore のルールで許していない(サーバーだけ)。`GET /v1/me/connections` には provider・状態・scope・日付だけを返す(`ConnectionView`)。トークンの型(`Token`・`Secrets`)はログ・`%v`・JSON に出すと `[redacted]` になる。
- **切る(取り消し)**: provider の取り消しの口を呼び(無い provider は呼ばない)、**すぐ**トークンを消して `revoked` の印だけ残す(決定 7)。印は 90 日で消える。
- **Google は 1 つの許可**: カレンダー・ToDo・スプレッドシートは同じ OAuth クライアント(`Provider.ClientFrom`)で `include_granted_scopes=true` なので、Google 側では 1 つの許可にまとまる。どれか 1 つのトークンを取り消すと全部が切れる。だから 1 つを切る時は、同じクライアントのほかの provider がまだつながっていれば Google の取り消しの口を呼ばない(トークンはすぐ消す)。最後の 1 つを切る時に取り消す。引き換え: ほかがつながっている間は、切った provider の scope も Google のアカウントの「接続したアプリ」に残る(こちらはトークンを持たない)。`EXT_GOOGLE_TASKS_CLIENT_ID` などで別のクライアントにした時も、同じグループとして扱う(取り消しが最後まで遅れるだけ)。接続の一覧が読めない時も取り消さない(ほかを壊さない方を取る)。
- **90 日使わない**: 使うたび(1 時間に 1 回まで書く)に `deleteAt` を 90 日先にする。過ぎたものは次に読んだ時に消し、`POST /internal/ext/expire`(`ExpireUnused`)と Firestore の TTL(`deleteAt`)でも消える。
- **Pro が切れた**: トークンは残し、使わない(呼び出しが枠の段で止まる)。使わなければ 90 日で消える。
- **退会**: 30 日後の消去(`cmd/purge`)で `users/{uid}/connections/*` を消す。退会の時は appconfig が runner の `revokeAll` を呼び、provider 側の許可も取り消す(#1716)。runner が失敗・落ちていても退会は止めず(ログ `revoking external connections on delete failed`)、30 日後の消去でトークンは必ず消える。

## runner の口

`/internal/*` は appconfig の SA だけが呼ぶ(Cloud Run の IAM・OIDC)。答えが失敗の時は `{"reason": "<理由>"}` と下の状態コードで、appconfig が理由を同じ名前のエラーコードにして端末に返す(下の「appconfig の口」)。

| 口 | 中身 |
|---|---|
| `POST /internal/ext/call` | `{provider, op, args, appId, creatorUid, creatorAnonymous, userUid, userAnonymous, requestId}` → `{result: {rows / item / resultId}, coinsCharged, coinsBalance}`(後の 2 つは `X-Coins-Charged` / `X-Coins-Balance` にする) |
| `POST /internal/ext/connections/start` | `{uid, provider, scopes?}` → `{authUrl}`(scope を書かなければ読む op の分だけ。書く op の scope は使う時に足す) |
| `POST /internal/ext/connections/revoke` / `revokeAll` / `list` | 切る / 全部切る(退会)/ 一覧(トークン無し。`list` は動かしている provider の id `enabled` も返す) |
| `POST /internal/ext/expire` | 90 日を過ぎた接続を最大 500 件消す(Scheduler から 1 日 1 回の想定) |
| `GET /oauth/callback/{provider}` | provider の同意から戻る唯一の認証なしの口。state を 1 回だけ確かめ、code をトークンに替えて保管し、`tailorit://connected?provider=…&status=ok\|error` に 302。URL にトークン・code は入らない |

| 理由 | HTTP |
|---|---|
| `bad_request` | 400 |
| `ext_not_connected` / `duplicate_request` | 409 |
| `ext_unavailable` / `ext_busy` | 503 |
| `rate_limited`(`scope`・`retryAfterSec` つき) | 429 |
| `pro_required` / `allowance_exhausted` / `overflow_cap_reached` / `coins_insufficient` | 402 |
| `upstream_timeout` / `upstream_unavailable` | 504 / 502 |

## appconfig の口(端末から)

端末は appconfig だけを呼ぶ。appconfig は人とアプリと引数を確かめ、`EXT_RUNNER_URL` の runner を ID トークン(audience = その URL)つきで呼ぶ。`EXT_RUNNER_URL` が無い時は 503 `ext_unavailable`(一覧は空)。契約は `openapi-appconfig.yaml` の tag `ext`。

| 口 | 中身 |
|---|---|
| `POST /v1/apps/{appId}/ext/{provider}/{op}` | `{"args": {...}}` → `{rows / item / resultId}`。有料の provider は `X-Coins-Charged` / `X-Coins-Balance` |
| `GET /v1/me/connections` | `connections`: つないだサービスの一覧(provider・状態・scope・日付。トークンは返さない)。`providers`: つなげるサービスの一覧(下) |
| `POST /v1/me/connections/{provider}/start` | `{"scopes"?}` → `{authUrl}`(OAuth の要る provider だけ。ほかは 404) |
| `DELETE /v1/me/connections/{provider}` | 切る(204) |

`GET /v1/me/connections` の `providers` は、端末の「つないだサービス」の画面がつなぐボタンを出すための一覧(#1566)。端末に provider を書き込まずに済む。

- 中身は `{id, nameJa, nameEn, enabled}`。OAuth の要る provider だけ・id の順。名前は provider のファイルの `NameJa` / `NameEn`(OAuth の provider では必須)
- `enabled` は runner が今その provider を動かしているか(runner の一覧の答えの `enabled`・`Service.EnabledProviders`)。`EXT_RUNNER_URL` が無い時はみな false
- 偽の provider(`fakeOauth`)は runner が動かしている時だけ出す。本番では動かないので出ない
- `?supportedFeatures=a,b`(generate と同じ値・くり返しも可)を付けると、`externalCall.<id>` が入っている provider だけにする。審査用の版は OAuth の provider のキーを送らない(`ReviewBuildFeatures`・決定 6)ので何も出ない。付けなければ絞らない
- 別の口にせず一覧に入れたのは、画面が 1 回の呼び出しで「つないだもの」と「つなげるもの」を出せるから

`POST /v1/apps/{appId}/ext/...` で appconfig が確かめること:

| 段 | 中身 | 失敗した時 |
|---|---|---|
| 1. 一覧 | provider と op が閉じた一覧にあるか | 404 `not_found` |
| 2. 引数 | op の名前だけ・4096 バイトまで・URL は受けない・op の `Check`(`Provider.CheckArgs`。runner でももう一度) | 400 `bad_request` |
| 3. 誰の枠か | 共有のアプリ(公開の DSL に public / shared の entity)は**作成者**。呼べるのは参加者(public の entity があれば入れた人も)。自分のアプリ・自分の写しの private のアプリは呼び出した人 | 403 `not_member` / 404(自分のツリーに無い)/ 410 `unpublished` |
| 4. 1 時間の上限 | 1 人 `EXT_USER_PER_HOUR` 回(既定 120) | 429 `rate_limited` |
| 5. runner | `X-Request-Id` を `requestId` に(コインの押さえの鍵)。Pro は runner が確かめる(`PRO_GATE_MODE` に関係なく)。作成者でない人の 402 `pro_required` には `details.reason: creator_needs_pro`。runner の回数の上限は 429 `rate_limited`(`Retry-After`・`details.scope` / `provider` / `retryAfterSec`) | 理由をそのままコードに。runner に届かない時は 502 `upstream_unavailable`・待ちきれない時は 504 `upstream_timeout` |

DSL に `externalCall` / `externalList` があるかの確かめは、DSL の子 Issue で部品と一緒に足す(今は provider が既定で off・Pro だけなので、一覧と引数だけを見る)。

## スプレッドシートの取り込み(自動の動き・#1712)

自動の動き(serverRules)の 1 回の中で `googleSheets` の `readRange` を呼び、表の行を entity に入れる(id の列で上書き)。実行する部分だけが入っている(`runner.SheetImporter`)。**DSL の書き方(serverRules の action)と runner の tick への組み込みはまだ**: 生成アプリから外の API を使う部品と AI の決まり(#1643)が決まってから、その形で足す。それまで DSL のスキーマ・`prompt.go`・`catalog.go` は変えない。

| 項目 | 中身 |
|---|---|
| 入力(`SheetImport`) | `SpreadsheetID`・`Range`(`readRange` と同じ)・`Entity`・`IDColumn`(見出しの名前)・`ImportKey`(任意。記録の id で `IDColumn` の代わりに使う名前)・`Fields`(見出し → field id。無ければ field の id か名前と同じ見出しの列)・`MaxRows` |
| 見出し | 範囲の最初の行(空の行は飛ばした後)。`IDColumn` が見出しに無い・範囲が空は `ErrImportNoIDColumn`(何も書かない・数えない) |
| 上書き | 記録の id は `store.IDFromClientID` に entity・`ImportKey`(無ければ `IDColumn`)・正規化した id の値を渡したもの(`importClientID`)。rule や action は入れない: rule の名前を変える・前に action を足すと全部の行が二重になるため。id の値は NFKC・前後の空白を外し、数に見えるものは 1 つの書き方にそろえる(`1` / `1.00` / `001` / `１` は同じ・`1,000` と `1000` は同じ)。表示の形式を変えても同じ記録になる。列の見出しの名前を変えるなら `ImportKey` を決めておく。何回走っても同じ id の行は同じ記録になり、二つにならない。新しい id は作る(`CreateRecord`・作った記録は `origin` に rule・版・event)、ある id は変わった field を書く(`ApplyRecordOnce`。印は event + rule + action + 記録の id なので、同じ event のやり直しで二度書かない) |
| 空のセル | 新しい記録には入れない。ある記録では消す(必須の field は消さず前の値のまま) |
| 飛ばす行 | id が空(`NoID`)・前の行と同じ id(`Duplicate`・最初の行が勝つ)・新しい行で必須の field が無い(`Required`)。型に合わないセル(`BadValues`)はその field だけ入れない |
| 型 | セルを型のある値にしてから、端末の書き込みと同じ確かめ(`appconfig.NormalizeValue`)を通す。integer / number / currency は数(`,` `¥` を外す)。integer / currency に小数があれば入れない(丸めない)・int64 の範囲を超えるものも入れない。boolean は true / false / 1 / 0 / yes / no / はい / いいえ / ○ / ×。date は `2026-11-12` か `2026/11/12`。datetime は RFC 3339・`2026-11-12 06:00`・日付だけ(その日の 0 時)。時差の無い日時は run の地域(`ImportRun.Location`・無ければ日本時間)で読む。enum は選択肢にある値だけ(選択肢の無い enum は何でも)。ほかの型・computed・encrypted の field は `ErrImportSpec`(呼ぶ前に止める) |
| 1 回の行数 | `MaxRows`(既定 `DefaultImportRows` = 50・上限 `MaxImportRows` = 99)。`readRange` の `limit` は見出しを足した数(最大 100)なので、それより先の行は読みもしない。列は `readRange` と同じ 10 列まで・1 セル 1000 文字まで |
| 数え方 | 呼び出しは `CallRequest.Run` に run の `quota.Decision` を渡す(自動の動きの外の API と同じ。`runs` 1・Pro だけ・回数の上限は作成者・トークンは作成者のもの)。request id は `import/<event>/<rule>/<action>`。記録と同じ値の行は書かず数えない(`Unchanged`)。書く行だけを `quota.Writes` で数え、1 回の上限・アプリの 1 日の上限で切った行は `Dropped`(書かない) |
| 列の受け渡し | `readRange` の答えの `Output.Cells`(列のまま・サーバーの中だけ。JSON には出さず端末に返らない)。端末に返す `rows` の形は変わらない |
| プライバシー | シートの中身は記録として保存する(今の記録と同じ扱い)。`readRange` の答え自体はキャッシュしない |

## 偽の provider(stg と試験)

決定 5 のとおり、stg は偽の provider で通す(本物の鍵は prod の runner だけ)。`APP_ENV=production` では `EXT_*_ENABLED` を入れても動かない。

| provider | 種類 | op | 入れ方 |
|---|---|---|---|
| `fakeFree` | 無料・鍵なし | `listItems(q)`: 決まった 2 行 | `EXT_FAKE_FREE_ENABLED=1` |
| `fakePaid` | 有料 | `echo(text, costUsd)`: `costUsd` を測った原価として返す | `EXT_FAKE_PAID_ENABLED=1`・`COIN_RESERVE_EXT_FAKE_PAID` |
| `fakeOauth` | 無料・OAuth | `listEvents`・`createEvent(title)` | `EXT_FAKE_OAUTH_ENABLED=1`・`EXT_OAUTH_CALLBACK_BASE`・`EXT_TOKEN_KMS_KEY`(stg の鍵) |

どの op も `fail=5xx | 4xx | timeout | grant` で失敗の形を試せる。`fakeOauth` の同意の URL は runner 自身の戻り先なので、本物の provider 無しで「つなぐ」まで通る。

## 人の作業

- Cloud KMS の鍵(対称・runner の SA にだけ `roles/cloudkms.cryptoKeyEncrypterDecrypter`)を作り、`EXT_TOKEN_KMS_KEY` を runner に入れる(`infra/runner.tf`)。
- appconfig に `EXT_RUNNER_URL`(runner の URL)を入れ、runner の invoker に appconfig の SA(`run-backend`)を足す(`infra/runner.tf`)。
- runner の `/oauth/callback/{provider}` を認証なしで受けられるようにする(今の runner は Cloud Run の IAM で全部の口が認証つき)。`/internal/*` は今のまま appconfig の SA だけにする(アプリの中で口ごとに確かめる形に変えるか、戻り先だけ別の入口にするかを決める)。
- Firestore の TTL: `connections` の `deleteAt` と `oauthStates` の `expireAt`(`gcloud firestore fields ttls update`)。`ExpireUnused` のコレクショングループの検索は `connections.deleteAt` の単一フィールドのインデックス(コレクショングループの範囲)が要る。
- Cloud Scheduler で `POST /internal/ext/expire` を 1 日 1 回。
- runner の最小 1 台(決定 4)・`#288` までは runner が prod だけなので、stg で偽の provider を通す時の runner の置き場所。
- 各社の鍵・OAuth のクライアント(Secret Manager・`EXT_<PROVIDER>_*`)と、原価・押さえの量(非公開の資料から env)。
- Google ToDo(#1677): カレンダーの OAuth クライアントに戻り先 `/oauth/callback/googleTasks` を足し、Google Cloud のプロジェクトで Tasks API を有効にし、同意の画面に scope `tasks.readonly`・`tasks`(センシティブ)を足して Google の確認を受け直す。
- GitHub App の登録(#1674): 権限は Issues・Pull requests・Metadata を読むだけ・「Expire user authorization tokens」を入れたまま・コールバックは runner の `/oauth/callback/github`・Webhook は切る。利用者は App を自分の repo / org に入れる(org は管理者の承認が要ることがある)。
- Linear の OAuth の App の登録(#1684): コールバックは runner の `/oauth/callback/linear`・公開にするか(ほかのワークスペースから使えるように)を決める。
- Google スプレッドシート(#1685): カレンダーの OAuth クライアントに戻り先 `/oauth/callback/googleSheets` を足し、Google Cloud のプロジェクトで Sheets API と Picker API を有効にし、同意の画面に scope `drive.file`(センシティブでない)を足す。iOS の Google Picker でファイルを選ばせる部分は別の Issue。
- 回数の上限の値(`EXT_LIMIT_*`・`EXT_GLOBAL_DAILY*`)を各社の割り当てに合わせて runner の env に入れる(非公開の資料から。入れなければコードの既定)。
- プライバシーポリシー・利用規約(`docs/legal`)に外の API への送信とトークンの保管・消し方を書く。
- YouTube(#1699): Google Cloud で YouTube Data API v3 を有効にし、鍵(使える API を YouTube Data API v3 だけに絞る)を Secret Manager の `EXT_YOUTUBE_API_KEY` に入れる。1 日の割り当て(Cloud Console の Quotas)を確かめて `EXT_GLOBAL_DAILY_YOUTUBE` を合わせる。上の「YouTube の規約」を読み直し、プライバシーポリシーに YouTube API サービスの利用と Google のプライバシーポリシーへのリンク、アプリに YouTube の利用規約へのリンクを足す。
- ODPT(#1691): [公共交通オープンデータセンターの開発者サイト](https://developer.odpt.org/)で開発者登録(運営のアカウント・審査に数日かかることがある)をし、発行された consumer key を Secret Manager の `EXT_ODPT_API_KEY` に入れる。上の「ODPT の事業者と規約」を読み直し、アプリの問い合わせ先(開発者のメールアドレスなど)を決める。路線 id は登録後に `odpt:Railway` で一度確かめる。
- ポッドキャスト(#1703): 上の「iTunes Search API の規約」を読み直し、「ストアの中身を広めるためだけ」の使い道に当たるか(番組を探して Apple Podcasts を開く使い方)を決める。`EXT_GLOBAL_DAILY_PODCASTS` を 1 分 20 回の目安に合わせて入れる。バッジ(Listen on Apple Podcasts)の出し方を iOS と決める。アートワークを端末に出すなら `ExtRow` に画像の欄を足すか(API の契約の変更)を決める。
