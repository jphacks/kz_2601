# #630 コイン課金(作りの設計)

> 状態: **判断待ち(下書き)**。2026-11-06 作成、2026-11-10 にオーナーが選んだ方向(下の「方向」)で書き直した。決めるのはオーナー。
> 対象 Issue: [#630](https://github.com/jphacks/kz_2601/issues/630)。前提は正典 [`docs/00_decisions.md`](../00_decisions.md) の「課金・Pro」、費用の作り [#338](338-server-feature-cost-model.md)、有料の機能 [features/pro.md](../backend/features/pro.md)、1 日の回数 [rate-limit.md](../backend/rate-limit.md)。
> **値段は非公開の資料で決める。** この文書は作るのに要る技術の設計だけを書く(1 コインの値段・処理ごとのコイン数・枠の大きさ・原価・粗利は書かない)。値段に関わる数は環境変数で持ち、この repo には既定値 0(= 数えない)か仮の値だけを置く。

## 目次

1. [方向(2026-11-10)](#方向2026-11-10)
2. [何で払うか](#何で払うか)
3. [商品(App 内課金の種類)](#商品app-内課金の種類)
4. [残高と台帳(Firestore の形)](#残高と台帳firestore-の形)
5. [AI で引く(予約 → 確定・失敗の返金)](#ai-で引く予約--確定失敗の返金)
6. [サーバーの枠と、超えた後のコイン払い](#サーバーの枠と超えた後のコイン払い)
7. [暴走の上限(コインと別)](#暴走の上限コインと別)
8. [共有アプリの払う人](#共有アプリの払う人)
9. [アプリの中の AI](#アプリの中の-ai)
10. [API とエラーコード](#api-とエラーコード)
11. [App Store・返金・審査](#app-store返金審査)
12. [移り方(フラグ)](#移り方フラグ)
13. [PR の分け方](#pr-の分け方)
14. [決めてほしいこと(作り)](#決めてほしいこと作り)
15. [参考](#参考)

## 方向(2026-11-10)

| 項目 | 方向 |
|---|---|
| 通貨 | アプリの中の通貨 **1 種類**(仮の名前「コイン」)。App Store の**消耗型**の App 内課金で売る |
| 期限 | **買ったコインは期限なし**(ガイドライン 3.1.1) |
| ランダム | ガチャ・ランダムの品は無い |
| AI の費用 | 作る・直す・アプリの中の AI(aiButton・aiChat)・レシピの選び(recipe selector)など **LLM を呼ぶものは全部、使うたびにコインから直接引く**。引く量は 1 回の実際の費用(`ai.usage` の `est_cost_usd`)から決める |
| サーバーの費用 | 自動の動き(runner の実行)・通知・レコードの保存と書き込み・外部データの取得など。まず **Pro の枠**(月 / 日)で払う。枠を使い切ったら、**本人が「超えた後のコイン払い」を ON にしていれば**(既定 OFF)コインで続ける。本人が決めた**月のコインの上限**まで |
| 通知 | 枠とコインの両方で 80% / 100% の通知(#345 の runner の通知を使う) |
| Pro | 有料の機能の解放 + サーバーの枠 + **毎月の少しのコイン**(繰り越さない) |
| 暴走の歯止め | コインと別に残す(アプリ・日・1 回の大きさごとの上限、全体の予算 #317) |
| アプリの中の AI | アプリを作れない(文字と項目だけ)。**必ず誰かのコインから引く**(無料の道を作らない) |
| 共有アプリ | サーバーの費用と参加者の AI は**作成者**の枠・コインで払う。作成者が止められる |

Claude の Pro プラン + 上限つきの追加利用と同じ形。

## 何で払うか

| 使うもの | 何で払うか | 数える単位(ログ) | 実装の場所 |
|---|---|---|---|
| 新規作成・修正(`/v1/generate`) | コイン(実費) | `ai.generate` の `est_cost_usd`(再試行・計算パス・レシピの選びを含む合計。`cost_tally.go`) | `internal/ai` |
| 相談(`/v1/chat`) | コイン(実費) | `ai.usage`(`chat_first` など) | `internal/ai/chat.go` |
| アプリの中の AI(`/v1/text`: aiButton・aiChat・長文・写真) | コイン(実費) | `ai.usage` | `internal/ai/text*.go` |
| レシピの選び(Haiku) | コイン(作成の合計に入る) | `ai.usage` の `recipes` | `internal/ai/recipe_selector.go` |
| 元に戻す(undo) | 引かない | Claude を呼ばない | — |
| 自動の動きの実行・通知の宛先・書き込み・読み取り | Pro の枠 → (ON なら)コイン | `runner.usage` | `internal/runner/quota` |
| レコード・写真の保存 | Pro の枠(件数 #317)→ (ON なら)コイン | `plan` の件数 | `internal/plan` |
| 外部データの取得(feeds) | Pro の枠に数えるかは要確認(今はキャッシュつきの読み取りだけ) | — | `internal/feeds` |

- **コインに直す式**: `coins = ceil(est_cost_usd × COIN_PER_USD)`、最小 `COIN_MIN_PER_CALL`。`COIN_PER_USD` は環境変数(値は非公開の資料)。1 回の費用が分かるので「処理ごとの固定の値段表」は持たない。
- **料金表に無いモデル**(`EstimateCostUSD` が ok=false)は、その処理の予約の量(下)をそのまま確定する。ログ `coins.unpriced_model` を出し、料金表を足すまで見張る。
- **表示**: 押す前に「約 N コイン」(その処理の最近の中央値・`GET /v1/me/coins` の `estimates`)を出し、終わったら実際に引いた量を履歴に出す。

## 商品(App 内課金の種類)

| 商品 | 種類 | 中身 | 作り |
|---|---|---|---|
| Pro(今の `jp.kz2601.metamon.pro.monthly`) | 自動更新の購読 | 有料の機能・サーバーの枠・毎月のコイン | 今の `internal/entitlement`(#314 #315)のまま。毎月のコインは購読の期間で付ける |
| コインのパック(2〜3 個・id は App Store Connect で決める) | **消耗型** | コイン N 枚 | 新しい受け口 `POST /v1/me/coins/apple`。`APPSTORE_PRODUCT_IDS` に足し、**商品 id → 枚数**は環境変数 `COIN_PACKS`(値は非公開) |

- 匿名のアカウントでは**買えない**(消耗型は StoreKit で復元されない。残高はサーバーで uid に持つので、端末を失うと戻せない)。買う前にサインインを求める。
- 審査用の版(`APP_STORE_REVIEW`・`ProProducts.swift`)では今と同じく App 内課金を出さない。

## 残高と台帳(Firestore の形)

```
users/{uid}/coins/wallet          {purchased, grant, grantPeriodEnd, reserved, overflow: {enabled, monthlyCap, month, spent}, updatedAt}
users/{uid}/coinLedger/{entryId}  {kind, amount, op, appId, requestId, transactionId, usd, at}
coinReservations/{requestId}      {uid, amount, op, appId, expireAt}
coin_transactions/{transactionId} {uid, productId, amount, environment, at}
usage/{uid}_{yyyymm}              {runs, recipients, writes, reads, warned80, warned100}
```

- `kind`: `grant_monthly` / `purchase` / `spend_ai` / `spend_overflow` / `refund_failed` / `refund_apple` / `adjust`。`amount` は符号つき。
- **二つの財布**: `grant`(Pro の毎月の分。期間が変わったら Pro の量に戻す・繰り越さない)と `purchased`(買った分・期限なし)。**引く順番は `grant` → `purchased`**(期限のある方を先に使う)。
- 毎月の分の付与は**遅延評価**(次に使う時に `entitlements/pro` の `expiresAt` と `grantPeriodEnd` を比べて戻す)。定期処理は要らない。
- **台帳の 1 行は残高の変更と同じトランザクション**で書く(後から合計を作り直せる・基準日の未使用残高も `purchased` の合計で出せる)。
- 退会(`cmd/purge`)で `coins`・`coinLedger` を消す。`coin_transactions` は返金の照合のため uid を外して残すか要確認。
- 残高・台帳・予約は**サーバーだけが書く**(Firestore のルールでクライアントは読み取りだけ)。

## AI で引く(予約 → 確定・失敗の返金)

| 段 | 中身 |
|---|---|
| 1. 受付 | 今の `ratelimit` ミドルウェアの位置。払う人(本人 or 共有アプリの作成者)の財布から、その処理の**予約の量**(`COIN_RESERVE_<OP>`。処理ごとの上の方の費用)を 1 トランザクションで `reserved` に移す。足りなければ何も書かず **402 `coins_insufficient`**(`details.required`・`details.balance`) |
| 2. 冪等 | 予約の鍵は `X-Request-Id`(`httpx/middleware.go`)。同じ uid と id の 2 回目は予約を作らず 1 回目の結果に従う(再送で二重に引かない) |
| 3. 確定 | 終わったら `cost_tally` の合計(`est_cost_usd`)をコインに直し、予約との差を戻す。台帳に `spend_ai` を 1 行(`usd` も残す)。実費が予約を超えたら**予約の量で止める**(利用者に予約以上は請求しない・差はログ `coins.over_reserve` で見張る) |
| 4. 失敗の返金 | アプリが渡らなかった失敗(5xx・タイムアウト・検証が最後まで通らない)は**全部戻す**(`refund_failed`)。#548 の `refund.go` の表を「回数」から「コイン」に置き換える。切断(クライアントが閉じた)の扱いは決めてほしいこと |
| 5. 取り残し | 確定されない予約(インスタンスが落ちた)は `expireAt`(例 10 分)の後に戻す(TTL + 掃除)。利用者に損をさせない側に倒す |
| 6. 同時 | 予約は 1 トランザクションなので、同時に送っても残高より多くは引けない |

- 確定は応答を返した後でもよい(ストリームの `chat`・`generate`)。確定に失敗したら予約のまま残り、5 の掃除で戻る(取りっぱぐれる側に倒す)。
- 応答に `X-Coins-Charged`(確定した量)と `X-Coins-Balance` を付ける(iOS の残りの表示。M1 の `X-RateLimit-Remaining` の代わり)。

## サーバーの枠と、超えた後のコイン払い

| 項目 | 設計 |
|---|---|
| 枠 | Pro の人(作成者)ごとの**月**の枠(実行・通知の宛先・書き込み・読み取り)を `usage/{uid}_{yyyymm}` に数える。量は環境変数 `ALLOWANCE_PRO_*`(値は非公開)。今の**日**の上限(`RUNNER_LIMIT_*`)は暴走の歯止めとして別に残す |
| 枠を超えた時(既定) | 止める。runner は `skipped_allowance` で実行しない。作成者に通知(100%) |
| 超えた後のコイン払い | 設定 `overflow.enabled`(**既定 OFF**)と `overflow.monthlyCap`(本人が決める月のコインの上限)。ON なら、超えた分を**まとまり(ブロック)ごと**にコインで買う: 1 ブロック = 実行 `OVERFLOW_BLOCK_RUNS` 回分など、ブロックの値段は `OVERFLOW_BLOCK_COINS`(値は非公開) |
| ブロックで引く理由 | 1 実行ごとに台帳を書くと、書き込みの費用が実行の費用を超える。ブロックを買う時だけ 1 トランザクション(財布・台帳・`usage` の枠の追加) |
| 上限に着いた時 | `overflow.spent + ブロック > monthlyCap` なら買わずに止める(`skipped_overflow_cap`)。残高が足りない時も止める(`skipped_coins`) |
| 通知(#345 を使う) | `runner/quota/warn.go` の `WarnNear` 80 / `WarnFull` 100 と `ClaimWarning`(1 日 1 回)を、**枠(月)**と**コインの月の上限**にも使う。通知の種類は 4 つ(枠 80%・枠 100%・上限 80%・上限 100%)。それぞれ月に 1 回 |
| 保存(件数) | `plan` の件数(#317)を枠にする。超えたら今の 402(件数)。保存をコインで足すか(月ごとに件数のブロックを買う)は決めてほしいこと |

- 枠とコインの順番: **枠 → (ON なら)コイン**。毎月のコイン(`grant`)もコインなので、ON なら超えた分に使われる。
- 枠は作成者のもの。共有アプリの参加者の操作で動いた実行も作成者の枠で数える(今の runner の数え方と同じ)。

## 暴走の上限(コインと別)

コインが十分あっても止める歯止め。全部今あるものを残す。

| 歯止め | 場所 |
|---|---|
| AI の 1 日の回数(利用者・IP・全体) | `internal/ratelimit`(`RATE_LIMIT_*`)→ 429 `rate_limited` |
| AI の全体の 1 日の費用 | `internal/budget`(#317・`AI_FREE_DAILY_BUDGET_USD`)→ 503 `free_plan_paused`。コインで払う呼び出しにも全体の予算(別の値)を置くかは決めてほしいこと |
| AI の止め弁 | `AI_PAUSED`(`internal/ai/pause.go`) |
| 1 回の大きさ | `/v1/text` の入力 4 KB・出力の上限・aiChat の 20 往復 / 16 KB(`text.go`・`text_chat.go`・`pkg/dsl/ai_chat.go`) |
| runner のアプリ・作成者・受け取る人・全体の 1 日の上限 | `internal/runner/quota`(`RUNNER_LIMIT_*`・`RUNNER_GLOBAL_DAILY`・`RUNNER_PAUSED`) |
| 共有アプリの AI(アプリごと・参加者ごとの 1 日) | `text_payer_scope.go`(`AI_CREATOR_PAID_APP_DAILY`・`AI_CREATOR_PAID_MEMBER_DAILY`) |

## 共有アプリの払う人

| 使う人 | 使うもの | 払う人 |
|---|---|---|
| 作成者 | 作る・直す・相談・アプリの中の AI | 作成者 |
| 参加者 | 共有アプリの中の AI(aiButton・aiChat) | **作成者**のコイン |
| 参加者 | 自分のアプリを作る・直す | 参加者 |
| 誰でも | 共有アプリの自動の動き・通知・保存 | **作成者**の枠 → (ON なら)作成者のコイン |

作成者が決められること(アプリごと・`apps/{appId}` の設定):

| 設定 | 既定 |
|---|---|
| 参加者がアプリの中の AI を使えるか | ON(作成者が OFF にできる) |
| このアプリで参加者の AI に使ってよい月のコイン | 作成者が決める(0 = 参加者は使えない) |
| 参加者 1 人の 1 日の回数 | 今の `AI_CREATOR_PAID_MEMBER_DAILY` |

- 払う人の判定は今の `text_payer.go` / `text_payer_scope.go`(公開されたアプリの作成者を読む)を使う。予約は**作成者の財布**に対して作る。
- 作成者の残高が尽きたら参加者には 402 `coins_insufficient`(`details.payer = "creator"`)。参加者には「作成者のコインが足りません」と出し、作成者には通知。

## アプリの中の AI

- **アプリを作れない**: `/v1/text` が返すのは `shape` = `line` / `list` / `fields`(文字・項目)だけで、DSL を返す口が無い(`text.go`)。新しい形を足す時もアプリ(DSL)を返さない(「Tailor It の中の Tailor It」で作成の費用を逃がさない)。
- **無料の道が無い**: `/v1/text` は必ず払う人(本人 or 作成者)の財布から予約する。払う人が分からない・匿名の時は 402。今の「Pro が要る」(402 `pro_required`)は残す。

## API とエラーコード

**契約(`openapi-ai.yaml`・`openapi-appconfig.yaml`)の変更なので、先に契約の PR で合意する。**

| 口 | 中身 |
|---|---|
| `GET /v1/me/coins` | 残高(`purchased`・`grant`・`grantPeriodEnd`)・枠の使った量と上限・`overflow`(ON/OFF・月の上限・今月使った量)・処理ごとの目安(`estimates`) |
| `GET /v1/me/coins/ledger?cursor=` | 履歴(日時・種類・処理・アプリ名・増減) |
| `POST /v1/me/coins/apple` | 消耗型の JWS を検証して足す。`transactionId` が既にあれば何もせず今の残高を返す(冪等) |
| `PUT /v1/me/coins/settings` | `overflow.enabled`・`overflow.monthlyCap` |
| `PUT /v1/apps/{appId}/ai-payer` | 共有アプリの作成者の設定(上の表) |

| コード | HTTP | 意味 |
|---|---|---|
| `coins_insufficient` | 402 | 払う人のコインが予約の量より少ない(`details.required`・`balance`・`payer`) |
| `allowance_exhausted` | 402 | サーバーの枠を使い切り、超えた後のコイン払いが OFF(appconfig の書き込み・件数) |
| `overflow_cap_reached` | 402 | 超えた後のコイン払いが月の上限に着いた |
| `pro_required` | 402 | 今のまま(有料の機能) |
| `rate_limited` | 429 | 今のまま(暴走の歯止め) |
| `free_plan_paused` / `ai_paused` | 503 | 今のまま(全体の予算・止め弁) |

runner は HTTP を返さないので、`runner.usage` の `outcome` に `skipped_allowance` / `skipped_overflow_cap` / `skipped_coins` を足す(`scripts/runner-usage.sh` の「止めた理由」に出る)。

## App Store・返金・審査

| 論点 | 扱い |
|---|---|
| 3.1.1 期限 | 買ったコインは期限なし。Pro の毎月の分は「その月の分(繰り越さない)」と表示し、買った分と画面で分ける。購読に含む分が「買ったクレジット」に当たるかは要確認 |
| 3.1.1 ランダム | ランダムの品を売らない(該当なし) |
| 3.1.2(c) | Pro の購入画面に「有料の機能・サーバーの枠・毎月のコイン」を書く |
| 二重の付与 | `coin_transactions/{transactionId}` を足すのと同じトランザクションで財布に足す。iOS はサーバーが受けてから `finish()` |
| 返金 | 通知 v2 の `REFUND`(`appstore_notify.go`)で該当の取引の分を引く(`refund_apple`)。残高が負になれば AI とコイン払いを止める。照合ジョブ(`cmd/appstore-reconcile`)に消耗型を足す。`CONSUMPTION_REQUEST` の答え方は要確認 |
| 外の決済 | 作らない(#413 と同じ・App 内課金だけ) |
| 法務 | 前払式支払手段(資金決済法)に当たるか・利用規約 §4・特定商取引法の表示は人が確かめる(`docs/legal` はこの PR で変えない) |

## 移り方(フラグ)

`PRO_GATE_MODE` と同じ 3 段のフラグ `COIN_BILLING_MODE` = off / log / enforce。

| 段 | 中身 |
|---|---|
| off(今) | 今の回数の上限・端末の「1 日 3 回」 |
| log | 財布と台帳を作り、引くはずの量をログだけ出す(`coins.would_spend`・`coins.would_overflow`)。止めるのは今の上限 |
| enforce | コインで止める(402 `coins_insufficient`)。パックを売る。端末の「1 日 3 回」(`ProGate.swift`・`CreationQuota.swift`)をやめ、サーバーの財布だけを正にする |

- 戻す時は off に戻せば今の上限に戻る(台帳は残す)。
- 今の Pro の購読者は何もしない(商品 id は同じ)。毎月のコインは次の更新日から。

## PR の分け方

1. docs: この設計(この PR)→ オーナーの判断 → 正典の更新(人)
2. contract: `GET/PUT /v1/me/coins*`・`POST /v1/me/coins/apple`・402 の 3 コード
3. backend: `internal/coins`(財布・台帳・予約 → 確定・失敗の返金)と `ratelimit` の隣の口(フラグ log)
4. backend: 消耗型の検証と付与・`REFUND`・照合ジョブ
5. backend: 月の枠(`usage/{uid}_{yyyymm}`)・超えた後のコイン払い(ブロック)・80 / 100% の通知(runner)
6. backend: 共有アプリの作成者の設定(`ai-payer`)
7. iOS: 残高・履歴・足りない時のシート・パックの購入・超えた後のコイン払いの設定・押す前の「約 N コイン」
8. 人: App Store Connect の商品・審査メモ・利用規約・法務

## 決めてほしいこと(作り)

| # | 決めること | 推奨 |
|---|---|---|
| 1 | 予約より実費が大きい時 | 予約の量で止める(利用者に予約以上は請求しない) |
| 2 | クライアントが途中で切った時(費用はかかっている) | 結果が保存されたら確定、されなければ戻す |
| 3 | 超えた後のコイン払いの単位 | ブロックで買う(1 実行ごとに台帳を書かない) |
| 4 | 保存(件数)もコインで足せるようにするか | 最初は足さない(件数の 402 のまま)。runner の分だけ先に作る |
| 5 | コインで払う AI にも全体の 1 日の予算を置くか | 置く(別の環境変数。暴走の歯止め) |
| 6 | 返金で残高が負になった時 | AI とコイン払いを止める |
| 7 | 退会した時の `coin_transactions` | uid を外して残す(返金の照合) |
| 8 | 共有アプリで参加者の AI の既定 | ON・月の上限は作成者が決める(未設定 = 0 で使えない、も候補) |

値段(1 コインの値段・パック・処理ごとの目安・枠の大きさ・毎月のコイン)は非公開の資料で決める。

## 参考

- [App Store Review Guidelines](https://developer.apple.com/app-store/review/guidelines/)(3.1.1・3.1.2)
- リポジトリ内: [#338 費用の作り](338-server-feature-cost-model.md)・[#413 生成アプリの決済](413-generated-app-payments.md)・[features/pro.md](../backend/features/pro.md)・[rate-limit.md](../backend/rate-limit.md)・[ai-cost.md](../infra/ai-cost.md)・[runner-usage.md](../infra/runner-usage.md)・[app-store-notifications.md](../infra/app-store-notifications.md)
- コード: `backend/internal/ratelimit/`(`refund.go` #548)・`backend/internal/budget/`・`backend/internal/ai/usage_log.go`・`cost_tally.go`・`text_payer_scope.go`・`backend/internal/runner/quota/`(`warn.go` #345)・`backend/internal/entitlement/`・`backend/internal/plan/`
