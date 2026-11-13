# コイン(残高・履歴・設定・枠・Apple の購入・config)(#2513)

> [バックエンド設計](../architecture.md) の機能の文書。設計と決定は [630-token-billing.md](../../decisions/630-token-billing.md)(正典)、形の正は [openapi-appconfig.yaml](../openapi-appconfig.yaml) の tag `coins`、環境変数は [runtime-config.md](../../infra/runtime-config.md#運用で変える値)。ここは「何で増えて何で減るか」と API の振る舞いをまとめる。値段・率はこの repo に書かない(環境変数の名前だけ)。

## 目次

- [全体の動き](#全体の動き)
- [API](#api)
- [何で増えて何で減るか(履歴の種類)](#何で増えて何で減るか履歴の種類)
- [超えた後のコイン払いの設定](#超えた後のコイン払いの設定)
- [枠(slot)](#枠slot)
- [Apple の購入の確かめ方](#apple-の購入の確かめ方)
- [失敗のコード](#失敗のコード)
- [コードの場所](#コードの場所)
- [決めること](#決めること)

## 全体の動き

- 財布は `users/{uid}/coins/wallet`。出どころ別に `grant`(Pro の毎月の分・`grantPeriodEnd` で消える)・`initial`(最初の分・1 人 1 回)・`purchased`(買った分・期限なし)を持つ。引く順番は **grant → initial → purchased**。
- `available` = 新しい呼び出しに使える量、`reserved` = 実行中の呼び出しが押さえている量、`total` = 両方の合計(履歴の `amount` の合計と一致する)。
- AI の呼び出し(generate・edit・chat・text)・通話・有料の外の API は、実行の前に上の方の量を**押さえ**(`coinReservations/{holdId}`)、終わったら実費(押さえた量まで)を**確定**し、失敗したら全部**戻す**。確定も戻しも残高と履歴を同じトランザクションで書く。
- 確定されないまま `COIN_HOLD_TTL` を過ぎた押さえは、その人の残高を次に読んだ時(`GET /v1/me/coins`・次の押さえ)に戻す。定期の掃除のジョブはまだ無い。
- すべて `COIN_BILLING_MODE` で切り替える。`off`(既定)= 何も読み書きしない。`log` = 財布と履歴は作るが、AI の呼び出しは引かずにログ(`coins.would_spend` / `coins.would_block`)だけ。`enforce` = 押さえて引く。
- 引いた呼び出しの応答には `X-Coins-Charged`(引いた量)と `X-Coins-Balance`(残り)が付く(ai・外の API)。

## API

| API | 認証 | すること |
|---|---|---|
| `GET /v1/me/coins` | 要る | 残高(`mode`・`total`・`available`・`purchased`・`grant`・`grantPeriodEnd`・`initial`・`reserved`・`overflow`・`allowance`・`appSlots`)。読むたびに、最初の分・Pro の毎月の分を足し、期間の過ぎた毎月の分を消し、期限の過ぎた押さえを戻す。`off` は `mode: off` とすべて 0 |
| `GET /v1/me/coins/ledger` | 要る | 履歴(新しい順)。`limit` は 1〜100(既定 20)、`cursor` は前の応答の `nextCursor`。応答は `{entries, nextCursor}`。`off` は空 |
| `PUT /v1/me/coins/settings` | 要る | 超えた後のコイン払いの設定(`overflow.enabled`・`overflow.monthlyCap`)。応答は保存した後の残高 |
| `POST /v1/me/coins/slot` | 要る | 持てるアプリの枠を 1 つ買う(`requestId` ごとに 1 回) |
| `POST /v1/me/coins/apple` | 要る | App Store のコインのパック(消耗型)の署名付き取引(`signedTransaction`)を送って足す |
| `GET /v1/coins/config` | 要らない | 端末が画面に出す数(`mode`・`packs`・`estimates`・`initialGrant`・`proMonthly`・`allowance`・`overflow`・`appSlotCoins`)。環境変数を起動時に 1 回読む。1 コインの率(`COIN_PER_USD`)は出さない。`Cache-Control: public, max-age=300` |

使える量の月の内訳(`allowance`)と、持てるアプリの数とその上限は [rate-limit.md](../rate-limit.md) と `GET /v1/me/usage` を見る。

## 何で増えて何で減るか(履歴の種類)

履歴は `users/{uid}/coinLedger/{entryId}` に、残高を変えるのと同じトランザクションで 1 行ずつ書く。`amount` は符号つき。行の id は決まった形なので、送り直しても同じ行になる。種類は `backend/internal/coins/coins.go` の `Kind*` の定数。

| `kind` | 増減 | いつ書くか | 今書いているか |
|---|---|---|---|
| `grant_initial` | + | サインイン済み(匿名でない)の人の財布を初めて読んだ時に `COIN_INITIAL_GRANT` を 1 回。サインインの相手ごとに 1 回で、アカウントを作り直しても 2 回目は無い(`coin_initial_grants`) | 書く |
| `grant_monthly` | + | Pro の請求期間が新しくなった時に `COIN_PRO_MONTHLY` に戻す | 書く |
| `grant_expire` | − | 期間の過ぎた毎月の分を消す。期間をまたいだ押さえから戻る毎月の分も消す | 書く |
| `purchase` | + | `POST /v1/me/coins/apple` でパックを足した | 書く |
| `spend_ai` | − | AI の呼び出し(generate・edit・chat・text)の確定 | 書く |
| `spend_call` | − | 通話の分数([calls.md](calls.md)) | 書く |
| `spend_ext` | − | 有料の外の API([external-apis.md](external-apis.md)) | 書く |
| `spend_overflow` | − | runner が月の枠を超えた分を 1 まとまり買った(下の「超えた後のコイン払いの設定」) | 書く |
| `spend_slot` | − | 枠を 1 つ買った | 書く |
| `refund_failed` | + | 確定した分を返した | 定数だけ。失敗した呼び出しは確定の前に押さえを戻す(履歴の行は出ない) |
| `refund_apple` | − | App Store でパックが返金された | 定数だけ(返金の通知でまだ引いていない) |
| `adjust` | ± | サポートの手での調整 | 定数だけ(書く口が無い) |

## 超えた後のコイン払いの設定

`PUT /v1/me/coins/settings` に `{"overflow": {"enabled": true, "monthlyCap": N}}`。

- `enabled` の既定は OFF。`monthlyCap` は本人が決める月(JST)のコインの上限で、0〜1000000000(打ち間違いの歯止め)。
- `COIN_BILLING_MODE=enforce` の時、runner は Pro の作成者の月の枠(`ALLOWANCE_PRO_*`)を超えた分を、ON なら 1 まとまりずつコインで買う(`spend_overflow`・同じまとまりを二重に買わない)。今月の `spent` + 1 まとまりが `monthlyCap` を超えるなら買わずに止める。
- この口は今月払った量(`spent`)を変えない。月が変わると `spent` は 0 から数える。
- 共有のアプリの自動の動きは作成者の枠と作成者のコインで払う。

## 枠(slot)

`POST /v1/me/coins/slot` に `{"requestId": "<端末が押すたびに作る UUID>"}`(128 文字まで)。

- `COIN_PRICE_APP_SLOT` コインで、持てるアプリの数を 1 つ増やす。期限なし・プランを変えても残る・返金しない。
- **冪等**: 同じ `requestId` の 2 回目は `bought: false` で今の枠を返し、コインを引かない。財布の `appSlots`・`spend_slot` の行・冪等の記録(`coinReservations`)を 1 つのトランザクションで書く。
- 買えるのは `COIN_BILLING_MODE=enforce` かつ `COIN_PRICE_APP_SLOT` > 0 の時だけ(それ以外は 404)。`log` は前に買った枠を数えるだけ、`off` は財布を読まない。
- 上限に達していなくても買える。持てるアプリの数の上限は プランの数 + `appSlots`([rate-limit.md](../rate-limit.md#持てるアプリの数630))。

## Apple の購入の確かめ方

1. 端末がパック(消耗型)を買う。端末は `appAccountToken` に自分の uid から作った値を入れる。
2. 端末が `POST /v1/me/coins/apple` に StoreKit 2 の JWS を送る(32 KB まで)。
3. サーバーが署名を確かめる(Apple の証明書の鎖・署名・`bundleId`・環境。Pro の購入の確かめ `POST /v1/me/entitlements/apple` と同じ)。加えて `type` が消耗型であること・返金済みでないこと・`productId` が `COIN_PACKS` にあること・`appAccountToken` が呼び出した人のものであることを見る。
4. `coin_transactions/{transactionId}` と財布を同じトランザクションで書き、`purchase` の行を足す。同じ取引の 2 回目は `credited: false` で今の残高を返す(二重に足さない)。
5. 端末はサーバーの 200 を受けてから `Transaction.finish()` を呼ぶ。消耗型は StoreKit で戻せないので、届くまで送り直す。

匿名のアカウントは買えない(サインインしてから)。

## 失敗のコード

| 状態 | code | いつ |
|---|---|---|
| 402 | `coins_insufficient` | 押さえる量・枠の値段に残高が足りない。`details`: `required`・`balance`・`payer`(`self` / `creator`)。`enforce` の時だけ |
| 402 | `allowance_exhausted` | 月の枠を使い切り、超えた後のコイン払いが OFF(runner) |
| 402 | `overflow_cap_reached` | 超えた後のコイン払いが本人の `monthlyCap` に届いた。`details`: `cap`・`spent` |
| 400 | `bad_request` | `limit` が 1〜100 でない・`cursor` が壊れている・`overflow` が無い・`monthlyCap` が範囲の外・`requestId` が無いか長すぎる・`signedTransaction` が無い |
| 403 | `forbidden` | 匿名のアカウントがパックを送った(`details.reason: anonymous`) |
| 404 | `not_found` | `COIN_BILLING_MODE=off` で設定・購入を呼んだ。枠を買えない時の `slot` |
| 409 | `transaction_owned_by_other` | その取引はもう別のアカウントに足した |
| 409 | `duplicate_request` | AI の呼び出しで同じ `X-Request-Id` を送り直した(`enforce`) |
| 410 | `account_deleted` | 退会済み |
| 422 | `invalid_transaction` | 取引を確かめられない。`details.reason`: `malformed`・`chain`・`signature`・`bundle_id`・`product_id`・`environment`・`not_consumable`・`revoked`・`account` |

## コードの場所

`backend/internal/coins/` の下。

| ファイル | 中身 |
|---|---|
| `coins.go` | 財布・履歴の行・押さえの形、履歴の種類の定数(`Kind*`)、引く順番 |
| `service.go` | 残高の読み取り・履歴・押さえ / 確定 / 戻し(`Reserve` / `Commit` / `Release`)・パックを足す(`CreditApple`) |
| `settings.go` | 超えた後のコイン払いの設定(`SetOverflow`) |
| `overflow.go` | runner が枠を超えた分を買う(`BuyOverflow`) |
| `slots.go` | 枠を買う(`BuySlot`) |
| `config.go` | 環境変数の読み取りと `GET /v1/coins/config` の中身(`PublicConfig`) |
| `http.go` | 402 の書き方(`WriteError`)・`X-Coins-*` の名前 |
| `ops.go`・`firestore.go`・`memory.go`・`store.go` | トランザクションの中身と置き場 |
| `coinsapi/handler.go` | HTTP の口(`/v1/me/coins*`・`/v1/coins/config`) |

使う側: `backend/internal/ai/coins.go`(AI)・`backend/internal/calls`(通話)・`backend/internal/extapi`(外の API)・`backend/internal/appconfig/app_slots.go`(枠を持てるアプリの数に足す)。最初の分の古い記録の作り直しは [coin-initial-backfill.md](../../infra/coin-initial-backfill.md)。

## 決めること

- パックの返金(`refund_apple`): 設計は App Store の返金の通知で引くとしているが、まだ引いていない。残高が負になった時の止め方と合わせて作る。
- `refund_failed`・`adjust`: 定数はあるが書く所が無い。サポートの調整の口を作るか、使わない定数を消すか。
- 押さえの定期の掃除(`Service.Sweep` を呼ぶジョブ): 今は次に読んだ時に戻すだけ。読まない人の押さえは `reserved` に残り続ける(なくならないが使えない)。
