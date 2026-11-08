# ai の利用上限(#280)

端末(ProGate)で数える無料枠は、入れ直しや匿名アカウントの作り直しで戻る。ai サーバーでも利用者ごと・1 日ごとに数えて、Claude の費用が青天井にならないようにする。

- 実装: `backend/internal/ratelimit`(ミドルウェア)。`cmd/ai/main.go` で `ai.NewHandler` を包むだけで、handler は触っていない。
- 対象: `POST /v1/generate`・`POST /v1/chat`・`POST /v1/text`(#417)。ai 自身が Firebase ID トークンを確かめる(今までは appconfig に転送するだけで、chat の create は**トークン無しでも Claude を呼べた**)。
- 対象外: `DELETE /v1/chat/{appId}`(対話状態を消すだけで Claude を呼ばない)。トークンが要り、無ければ 401、他人の対話なら 403、それ以外は 204(#286・`backend/internal/ai/handler.go`)。
- 応答: 429 `rate_limited`。`details` は `openapi-ai.yaml` の `RateLimitDetails`、`Retry-After` も付く。
- AI だけを止めたい時は上限を 1 にせず、一時停止のスイッチ(`AI_PAUSED=1`・503 `ai_paused`)を使う: [ai-pause.md](../infra/ai-pause.md)(#511)。上限を 1 にするのは `AI_PAUSED` の入っていない版が動いている時の予備だけ([incident-runbook.md §5](../ops/incident-runbook.md#5-手順-a-claude-api-の障害上限))

## 重さ(単位)

| 依頼 | 重さ | 理由 |
| --- | --- | --- |
| chat 1 往復 | 1 | 短い出力・軽いモデル設定 |
| text 1 回(aiButton generate のオンライン版・#417) | 1 | 出力 400 トークンまで・入力 4 KB まで・thinking なし |
| text 1 回(aiButton の要約・分類・書き直し・抜き出し・提案のオンライン版・`task` 付き) | 2 | 出力 1000 トークンまで(書き直しは入力と同じくらい長い)・入力 4 KB まで・thinking なし |
| text 1 回(aiButton の長文・`length: long`・task を問わない) | 3 | 出力 1500 トークンまで・入力 16 KB まで(約 4000 トークン)・thinking なし。短い text(入力 4 KB・出力 400〜1000)の約 3 回分の費用。オンライン AI だけで動く(端末内 AI を使わない) |
| text 1 回(aiButton の写真つき・`inputPhoto`・本文に `image`) | 3 | 長い辺 1024px の写真は入力 約 1500 トークン。task / length の重さと大きい方を数える。オンライン AI だけで動く |
| generate の修正(mode edit) | 2 | patch 中心 |
| generate の新規作成(create / auto) | 3 | 全体生成 + 検証のやり直し + logic pass |
| generate の undo | 0 | Claude を呼ばない |

auto はミドルウェアの時点で修正か新規かが分からないので、いったん 3 で数え、終わった後で実際の重さとの差を返す(下の「返す時」)。

## 返す時(#548)

依頼の受付時に数え、依頼が終わった時に次の分を返す。返すのは数えた全部の数(利用者・IP・global)で、0 より下にはしない。

| 終わり方 | 返す分 | 理由 |
| --- | --- | --- |
| 502 / 503 / 504(Claude の 5xx・タイムアウト・Claude や appconfig に届かない) | 数えた分の全部 | サーバー側の失敗。iOS の「もう一度」で 2 回分にならない |
| 200 で、mode auto が修正(edit)になった | 3 − 2 = 1(新規 − 修正の重さ) | 修正の重さは表の 2。auto だけ安くも高くもしない |
| 200 で、mode auto が「元に戻す」(undo)になった | 3 全部 | Claude を呼ばない |
| 4xx(入力の誤り・422・Claude の 429 も含む)と 500 | 返さない | 利用者が起こせる失敗、または原因が分からない失敗 |
| `/v1/text` の 402 `pro_required`(Pro が無い・`creator_needs_pro`)と 429 `scope: app`(作成者が払う呼び出しの 1 日の上限) | 数えた分の全部(応答の前)。ただし 1 人 1 時間 30 回まで(下の「返した依頼の上限」) | Claude を呼ばない。handler が `ratelimit.Waive` で応答の前に返し、`X-RateLimit-Remaining` もこの依頼を数えない値に書き直す。400 / 403(写真のスイッチ off)/ 413 は上の 4xx と同じく返さない |
| 503 `ai_paused` | (数えていない) | 一時停止はレートリミットの手前で返す(`cmd/ai/main_test.go` で順番を固定) |
| 途中で端末が切断した(ストリームの途中など) | 返さない | Claude の 1 回分の費用はもうかかっている。控えめな方を選んだ |

- handler は終わり方を**リクエストの context** で伝える(`ratelimit.ReportStatus` / `ratelimit.ReportMode`)。応答ヘッダーは使わないので、内部の値が端末に漏れない。
- 通常の JSON 応答は HTTP の status で判断する。SSE(`/v1/generate?stream=1`)は最初に 200 を返すので、`event: error` の status を handler が `ReportStatus` で伝え、ストリームの終わりに返す。
- mode は generate が決めた実際の mode(create / edit / undo)。保存の形(full / patch)ではない(修正の検証が通らず全体で保存しても、修正の重さ)。
- 返すのは数えたのと同じストア・同じドキュメント(その日の id)。Firestore は 1 トランザクションで全部を読んで引き、`expireAt` も数える時と同じ値で書く。もう無いドキュメント(TTL で消えた)は作り直さない。数える時に Firestore が落ちてメモリで数えた依頼は、メモリに返す。
- 応答の `X-RateLimit-Limit` / `-Remaining` / `-Reset` は利用者の上限の数(iOS の「今日の残り」#630 が読む)。依頼の前に書くので、返した分は入らない(次の依頼から反映)。
  - 重み 0 の依頼(generate の undo)は数えないが、今日の消費を読み取り 1 回(トランザクション・書き込みなし)で読み、正しい残りを返す。上限のまま返すと「元に戻す」の後で残りが満タンに見えるため。読めなかった時はヘッダーを付けない。
  - 利用者の上限が 0(数えない)の時はヘッダーを付けない(「残り 0」と出さないため)。iOS はヘッダーの無い応答では前の値を変えない(日付が変わると隠す)。429 は今までどおり付ける。
- 返した時はログに `ratelimit.refund`(units・status・mode・path)が出る。

### 返した依頼の上限

返した依頼(`ratelimit.Waive`)も、サーバーでは Firestore の読み書き(数える・返す・作成者や参加の確認)を使う。そこで返した回数を利用者ごと・1 時間ごとに別に数える(`RATE_LIMIT_WAIVED_HOURLY`・既定 30)。

```
rateLimits/{env}_{yyyy-mm-ddTHH}_waived_{uid}   {count, scope: waived, expireAt, updatedAt}
```

- 30 回までは今までどおり、1 日の利用上限を数えない(`X-RateLimit-Remaining` も変わらない)。
- 超えた分は返さず、その依頼の重さを 1 日の利用上限で数える。上限に届くと、それ以降はミドルウェアの 429 で止まり、handler まで届かない。
- 数える時に Firestore が失敗したら返す(今までと同じ)。
- 実装: `backend/internal/ratelimit/waive_cap.go`。

### 作成者が払う呼び出しの上限

共有のアプリの参加者の `/v1/text` は作成者の Pro で通る([pro.md](features/pro.md))。作成者が払う呼び出しを、アプリの部品の設定と照らし合わせ、アプリごとの 1 日の回数で数える。

- 照らし合わせ: 依頼がアプリの公開中の DSL(`apps_public/{appId}`)の部品と合う時だけ作成者が払う。aiButton は同じ task(無ければ generate)で、instruction が prompt の `{{input}}`(端末が書く「下の材料」/「the data below」)と `{{項目}}`(どんな文字でもよい)を埋めたもの。aiChat は instruction が persona と同じ。`length: long` は部品も long の時、写真つきは部品に inputPhoto がある時だけ。端末が 500 字で切った instruction は、途中までが合えばよい。合わなければ呼び出した人の Pro で判定する(Pro でなければ理由なしの 402)。
- 回数: アプリごと・1 日(0:00 JST)ごとに `AI_CREATOR_PAID_APP_DAILY` 回まで(既定 300・`0` で数えない)。超えると 429 `rate_limited`(`details.scope: app`)で、Claude は呼ばず、利用上限も数えない(上の「返す時」)。Firestore が失敗した時はインスタンスのメモリで数える。

```
rateLimits/{env}_{yyyy-mm-dd}_creator_paid_{appId}   {count, scope: app, expireAt, updatedAt}
```

- 実装: `backend/internal/ai/text_payer_scope.go`。

## 上限の既定値(1 日・0:00 JST で切り替え)

| 環境変数 | 既定 | 目安 |
| --- | --- | --- |
| `RATE_LIMIT_ANON_DAILY` | 30 | 新規 3 回(9)+ 修正・対話で 21。端末の無料枠(新規 3 回/日)に合わせた |
| `RATE_LIMIT_USER_DAILY` | 60 | サインイン済みは匿名の 2 倍 |
| `RATE_LIMIT_PRO_DAILY` | 300 | Pro でも上限はある(緩い)。Pro はサーバーで検証した権利(`users/{uid}/entitlements/pro`・#314 #449)で判定 |
| `RATE_LIMIT_IP_DAILY` | 300 | 匿名だけ。同じ IP からの匿名の合計(作り直し対策) |
| `RATE_LIMIT_GLOBAL_DAILY` | 2000 | 環境全体の費用ブレーカー。新規作成なら約 660 回 |
| `RATE_LIMIT_WAIVED_HOURLY` | 30 | 1 人 1 時間に数えずに返す依頼(上の「返した依頼の上限」)。`0` で数えない |
| `RATE_LIMIT_WEIGHT_CREATE` / `_EDIT` / `_CHAT` / `_TEXT` / `_TEXT_TASK` / `_TEXT_LONG` / `_TEXT_IMAGE` | 3 / 2 / 1 / 1 / 2 / 3 / 3 | `_TEXT_TASK` は `/v1/text` の本文に generate 以外の `task` がある時。`_TEXT_LONG` は本文に `length: "long"` がある時と `task: "chat"`(aiChat)の時。`_TEXT_IMAGE` は本文に `image` がある時。重なった時は大きい方(max) |
| `RATE_LIMIT` | on(GOOGLE_CLOUD_PROJECT があれば) | `off` で無効。ローカルで素の起動なら既定 off |
| `RATE_LIMIT_STORE` | firestore | `memory` でインスタンスごとのメモリ |
| `RATE_LIMIT_COLLECTION` | rateLimits | |

0 を入れるとその上限は数えない。値は運用で gcloud の `--update-env-vars` で変える。デプロイ(deploy-cloudrun.yml)はこれらを書かないので、次のデプロイの後も残る(#509・[`docs/infra/runtime-config.md`](../infra/runtime-config.md))。

費用ブレーカーの目安: 1 回の新規作成を $0.03〜0.10 と置くと(入力 1〜2 万トークン + 出力数千トークン。実測で直す)、2000 単位 ≒ 最大 $20〜65/日/環境。stg と prod は別々に数える。

## 数える場所(Firestore・appconfig と同じ DB)

```
rateLimits/{env}_{yyyy-mm-dd}_uid_{uid}        {count, scope, expireAt, updatedAt}
rateLimits/{env}_{yyyy-mm-dd}_ip_{sha256 先頭 8 バイト}   匿名だけ。生の IP は残さない(IPv6 は /64 単位)
rateLimits/{env}_{yyyy-mm-dd}_global
```

- `env` は `APP_ENV`(stg / prod)。同じ Firestore でも環境ごとに分かれる。
- 1 回の依頼 = 1 トランザクション。全部の数を読み、どれか 1 つでも超えるなら**何も書かず** 429。超えなければ全部に足す。返す時(上)ももう 1 トランザクション。
- Firestore が使えない時(障害・権限)は、インスタンスのメモリで数え続ける(止めない・青天井にもしない)。
- `expireAt` = その日の終わり + 1 日。TTL ポリシーで自動で消える(下の手順)。

### 書き込みの見積もり

1 回の依頼で 読み 2〜3・書き 2〜3(匿名は IP の分が増える)。1000 人 × 10 回/日 = 1 万回なら 読み 3 万・書き 3 万/日。無料枠(読み 5 万・書き 2 万/日)を少し超えても数セント/日で、Claude の 1 回分より安い。返す時(サーバーの失敗と、mode auto が修正になった時。iOS は auto を送るので修正のたび)は、同じ数の読み書きがもう 1 回増える。

`global` は 1 つのドキュメントに全員が書く。Firestore は 1 ドキュメントあたり毎秒 1 回程度が目安で、Claude の 1 回が数十秒かかる今の規模なら足りる。毎秒数回を超えるようなら、global を数個に分ける(shard)。

## プランごとの上限(#317)

無料と Tailor It Pro で使える量の表。値は環境変数で変えられる(`0` = 数えない)。コードの表は `backend/internal/plan`(`plan.Default`・`plan.FromEnv`)の 1 か所。生成・チャットの回数は上の ratelimit がそのまま数え、ここには同じ環境変数を読んで載せているだけ。

| 上限 | 無料 | Pro | 環境変数 | 数える所 |
| --- | --- | --- | --- | --- |
| 生成・チャット / 日(単位・上の「重さ」) | 60(匿名は 30) | 300 | `RATE_LIMIT_USER_DAILY`(`_ANON_DAILY`)/ `RATE_LIMIT_PRO_DAILY` | ai(429 `rate_limited`) |
| アプリの数 / 人 | 100 | 1000 | `PLAN_FREE_APPS` / `PLAN_PRO_APPS` | appconfig: 自分の新しいアプリ(`POST /apps`・新しい appId の最初の読み込み・複製)。共有のアプリを入れるのは数えない |
| レコードの件数 / アプリ | 5000 | 50000 | `PLAN_FREE_RECORDS_PER_APP` / `PLAN_PRO_RECORDS_PER_APP` | appconfig: レコードの作成。その人が見える分(自分の分 + 共有の分)の全 entity の合計。single の entity は数えない |
| 写真の枚数 / アプリ(= 容量) | 500(約 150 MB) | 5000(約 1.5 GB) | `PLAN_FREE_PHOTOS_PER_APP` / `PLAN_PRO_PHOTOS_PER_APP` | appconfig: 写真の項目が空から入る作成・更新(差し替えは数えない)。1 枚は 300 KB まで(レコード 1 件の上限)なので、枚数で容量を抑える |

- **既定は緩い**(今の使い方では当たらない)。暴走だけを止める値で、本当の値は #298 の 1 人あたりの費用と #325 の価格から人が決める。
- **誰のプランで数えるか**: 自分のアプリは呼び出した人、他の人が作ったアプリ(共有のアプリの参加者)は作成者。作った人が払い、参加者は無料(`docs/00_decisions.md`「課金・Pro」)。Pro は ai と同じ検証済みの権利(`users/{uid}/entitlements/pro`)。
- **強制の段階**: `PLAN_LIMITS_MODE` = `enforce`(既定)/ `log`(止めずに `plan_limit.would_block` を出すだけ)/ `off`。`PRO_GATE_MODE` と同じ考えで、出す時は `log` で量を見てから `enforce`。`plan_limit.would_block` は止めた時(enforce)も出る(`blocked`・`plan`・`resource`・`uid_hash`・`used`・`limit`)。
- **数え方**: Firestore の count 集計(1000 件で読み 1 回)。レコードの作成のたびに entity の数だけ、写真が入る時は写真の項目の数だけ増える。数えるのに失敗した時は通す(保存を止めるほどのものではない)。
- **超えた時**: 403 `plan_limit_reached`。`details` は `resource`(`apps` / `recordsPerApp` / `photosPerApp`)・`plan`(`free` / `pro`)・`limit`・`used`・`upgrade`(Pro にすると上がる)・`reason: creator_plan_limit`(作成者のプランの上限。参加者は払えないので課金画面ではなく案内を出す。`pro_required` の `creator_needs_pro` と同じ考え)。ai の generate が新しいアプリで当たった時もそのまま中継する。
- 消すと空きができる(数は今の件数で、累計ではない)。上限を下げても、もうあるものは消さない(新しく足すのが止まるだけ)。

## 今の使用量(#317・#630)

iOS が「今日の残り N」「上限に近い」を、断られる前に出すための読み取り。appconfig が答える(認証・App Check は他の `/v1/me` と同じ・本人の分だけ)。形は `openapi-appconfig.yaml` の tag `usage`。

| API | 返すもの | 読み取りの費用 |
| --- | --- | --- |
| `GET /v1/me/usage` | `plan`・`tier`・`resetAt`(次の 0:00 JST)・`ai`(生成・チャット・text の今日の合計: `used` / `limit` / `remaining` と `weights`)・`apps`(自分のアプリの数と上限)・`perApp`(自分のプランのアプリごとの上限) | `ai` は上の「数える場所」の `uid` のドキュメントを 1 回読む(キャッシュしない)。`apps` は count 集計 1 回を、インスタンスごとに 30 秒キャッシュ |
| `GET /v1/apps/{appId}/usage` | このアプリの `records` / `photos`(`used` / `limit` / `remaining`)・払う人の `plan`・`payer`(`self` / `creator`)・`upgrade` | entity ごと(+ 写真の項目ごと)の count 集計。(呼び出した人・アプリ・版)ごとに 30 秒キャッシュ |

- **2 本に分けた理由**: 記録・写真はアプリごとで、数えるのに entity の数だけ集計が要る。既存の API もアプリのものは `/v1/apps/{appId}/...`、自分のものは `/v1/me/...` に置いている。一覧や設定で `/v1/me/usage` を、アプリを開いた時に `/v1/apps/{appId}/usage` を呼ぶ想定。
- **今日の残り(`ai`)は iOS の表示と同じ数**: iOS(#653・#669 の `RemainingToday`)は ai の応答の `X-RateLimit-Remaining` / `-Limit` / `-Reset` を出している。`ai` は同じドキュメント・同じ上限(匿名 30・サインイン 60・Pro 300)・同じ 0:00 JST から出すので、`remaining` はヘッダーと同じ。違うのは、返した分(#548)がすぐ入ること(ヘッダーは次の依頼から)。生成とチャットは 1 つの枠(単位)なので分けない。「新しく作るのはあと何回」は `remaining / weights.create`。
- **上限の値は ai の環境変数**: appconfig も `RATE_LIMIT_ANON_DAILY` / `_USER_DAILY` / `_PRO_DAILY` / `RATE_LIMIT_WEIGHT_*` / `RATE_LIMIT_COLLECTION` を読む。運用で ai の値を変えた時は appconfig にも同じ値を入れる([runtime-config.md](../infra/runtime-config.md))。`RATE_LIMIT_STORE=memory`(ai がインスタンスのメモリで数える)では `ai` は null。
- **参加者が見る範囲**: 他の人が作ったアプリでは、作成者のプランの名前と上限・自分から見える分の数だけ。作成者の uid は返さない。scope:"shared" の entity は参加していなければ数えない。
- `limit` / `remaining` が null = 数えない(環境変数が 0・`PLAN_LIMITS_MODE=off`)。集計に失敗した所は null で、他の所は返す。
- 自分の写しが無いアプリは 404(`GET /dsl` と違い、ここでは写しを作らない・アプリの数も増えない)。

## 全体の遮断(#317)

1 日の AI の費用(#298 の `ai.usage` の `est_cost_usd`)の合計が閾値を超えたら、**無料プランの生成を一時停止**する。Pro は止めない(上の上限まで使える)。既定は無効。

| 環境変数 | 既定 | 中身 |
| --- | --- | --- |
| `AI_FREE_DAILY_BUDGET_USD` | 無し(無効) | その日(0:00 JST から)の費用の閾値(USD)。`0` / 無しで無効 |
| `AI_FREE_BUDGET_RESUME_DAY` | 無し | 手で戻す: `2026-11-07` のように JST の日付を入れると、その日だけ止めない。翌日からはまた効く(戻し忘れても次の日に遮断が死なない) |

- 止めるもの: Pro でない人の `POST /v1/generate`(`mode=undo` を除く)と `POST /v1/chat`。`POST /v1/text` は Pro の機能(onlineAI)なので止めない。
- 応答: 503 `free_plan_paused`。`details` は `reason: daily_budget`・`plan: free`・`resumeAt`(次の 0:00 JST)・`retryAfterSec`。`Retry-After` も付く。ログは `budget.free_paused`。
- 順番: 上限(ratelimit)の内側で判定する(呼び出した人が確かめられていて Pro が分かる)。503 なので数えた分は返す(上の「返す時」)。
- 費用の数え方: ai のログの出口(`ai.usage` の行)で `est_cost_usd` を足す(`backend/internal/budget`)。料金表に無いモデル(`est_cost_usd` が無い行)は数えない。合計は上限と同じコレクションの `rateLimits/{env}_{yyyy-mm-dd}_ai_cost {usd, expireAt, updatedAt}`(足すのは `Increment`・TTL も同じ)。各インスタンスは 30 秒ごとに読み直し、自分が足した分はすぐ数える。Firestore が読めない時は最後に分かっている合計のまま(止めも解きもしない)。
- 戻り方: 0:00 JST に自動で戻る。その日のうちに戻すのは `AI_FREE_BUDGET_RESUME_DAY` を今日にする、または `AI_FREE_DAILY_BUDGET_USD` を消す / 上げる(どちらも新しいリビジョン・[runtime-config.md](../infra/runtime-config.md#変え方))。
- 他の止め方との違い: `RATE_LIMIT_GLOBAL_DAILY` は全員(Pro も)を単位で止める。`AI_PAUSED` は全員を今すぐ止める(障害用)。これは費用で無料だけを止める。
- 閾値は人が決める(下の「人の判断」)。目安は `scripts/ai-cost.sh prod --days 30` の 1 日の合計の最大より上(普段は当たらず、暴れた日だけ当たる値)。
- **stg で試す(人の作業)**: ai-stg に `AI_FREE_DAILY_BUDGET_USD=0.01` を入れる → 無料のアカウントで 1 回生成(費用が付く)→ 30 秒以内にもう 1 回生成して 503 `free_plan_paused` → Pro のアカウントは通る → `AI_FREE_BUDGET_RESUME_DAY` を今日にして通る → 両方消す。

## 上限に近い・超えた時の文言(案・#317)

iOS の表示は別の PR(#669 が `ErrorCopy` を変えているため、その後)。サーバーの `message` も同じ考えで書いてある(`plan.Message`・`budget.Message`)。「何が起きたか + 次にすること」の 2 文にする。

| 場面 | 見分け方 | 文言(案) |
| --- | --- | --- |
| 生成の残りが少ない(残り 2 割以下) | `X-RateLimit-Remaining` / `Limit` | 今日の生成はあと {n} 回分です。0:00 にまた増えます。 |
| 生成の上限(無料) | 429 `rate_limited`・`tier` が `signed_in` | 今日の生成の上限に達しました。0:00 からまた使えます。Tailor It Pro にすると 1 日に使える回数が増えます。 |
| アプリの数の上限 | 403 `plan_limit_reached`・`resource: apps`・`upgrade: true` | 無料プランで作れるアプリ({limit} 個)がいっぱいです。使わないアプリを消すか、Tailor It Pro にすると作れる数が増えます。 |
| レコードの件数の上限 | `resource: recordsPerApp` | このアプリに保存できるデータ({limit} 件)がいっぱいです。古いデータを消すか、Tailor It Pro にすると上限が上がります。 |
| 写真の上限 | `resource: photosPerApp` | このアプリに保存できる写真({limit} 枚)がいっぱいです。古い写真を消すか、Tailor It Pro にすると上限が上がります。 |
| Pro でも上限 | `upgrade: false`・`plan: pro` | {件数 / 枚数}の上限に達しました。古いものを消すと続けて保存できます。 |
| 参加者(作った人の上限) | `reason: creator_plan_limit` | このアプリを作った人のプランの上限に達しました。作った人に知らせてください(作った人が Tailor It Pro にすると上限が上がります)。課金画面は出さない |
| 全体の遮断 | 503 `free_plan_paused` | 今日は利用が集中しているため、無料プランの新しい生成を止めています。{resumeAt}(0:00)に再開します。Tailor It Pro なら続けて使えます。「もう一度」は出さない(`Retry-After` が翌日まで) |

- 「近い」は、生成の回数なら応答のヘッダー、アプリの数・件数・写真は下の「今の使用量」の API で分かる。

## 抜け道への対策

| 対策 | 状態 |
| --- | --- |
| 匿名は上限が低い(30 vs 60) | 入れた |
| 匿名だけ IP ごとの合計上限(300) | 入れた。会場の Wi-Fi のように 1 つの IP に大勢がいる時は、サインイン済みは数えない。匿名の大勢がいるなら `RATE_LIMIT_IP_DAILY` を上げる |
| 全体の費用ブレーカー | 入れた |
| App Check(App Attest)で「本物のアプリからの依頼」だけ通す | **人の作業**(下)。いちばん効く |
| Pro をサーバーで確かめる | 入れた(#449)。appconfig が StoreKit 2 の署名付き取引を検証して保存した権利を `entitlement.ProChecker` で読む([features/pro.md](features/pro.md))。検証済みの Pro は匿名でも Pro で、IP ごとの上限も数えない |

### 人の作業(この PR ではしていない)

1. **Firestore の TTL**(1 回だけ):
   `gcloud firestore fields ttls update expireAt --collection-group=rateLimits --enable-ttl --project=<PROJECT_ID>`
2. **App Check**:
   1. Firebase コンソール > App Check で iOS アプリを登録、プロバイダは App Attest(シミュレータ・開発ビルドは debug provider)。
   2. iOS: `FirebaseAppCheck` を入れ、`AppCheck.setAppCheckProviderFactory(AppAttestProviderFactory)` を `FirebaseApp.configure()` の前に。ai への依頼に `X-Firebase-AppCheck: <token>` を付ける。
   3. ai: Admin SDK の `appcheck.Client.VerifyToken` で確かめるミドルウェアを足す(まずはログだけ → 様子を見て必須にする)。
   4. Firebase Auth の匿名サインインにも App Check の強制(Identity Platform)をかけると、匿名アカウントを機械的に量産できなくなる。
3. **デプロイ後の確認**: ai のログに `ai: rate limit on (store=firestore ...)` が出ること、Firestore に `rateLimits` ができること。
