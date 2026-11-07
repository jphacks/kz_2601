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

## 上限の既定値(1 日・0:00 JST で切り替え)

| 環境変数 | 既定 | 目安 |
| --- | --- | --- |
| `RATE_LIMIT_ANON_DAILY` | 30 | 新規 3 回(9)+ 修正・対話で 21。端末の無料枠(新規 3 回/日)に合わせた |
| `RATE_LIMIT_USER_DAILY` | 60 | サインイン済みは匿名の 2 倍 |
| `RATE_LIMIT_PRO_DAILY` | 300 | Pro でも上限はある(緩い)。Pro はサーバーで検証した権利(`users/{uid}/entitlements/pro`・#314 #449)で判定 |
| `RATE_LIMIT_IP_DAILY` | 300 | 匿名だけ。同じ IP からの匿名の合計(作り直し対策) |
| `RATE_LIMIT_GLOBAL_DAILY` | 2000 | 環境全体の費用ブレーカー。新規作成なら約 660 回 |
| `RATE_LIMIT_WEIGHT_CREATE` / `_EDIT` / `_CHAT` / `_TEXT` / `_TEXT_TASK` | 3 / 2 / 1 / 1 / 2 | `_TEXT_TASK` は `/v1/text` の本文に generate 以外の `task` がある時 |
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
