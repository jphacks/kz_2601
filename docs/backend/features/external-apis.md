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
8. [偽の provider(stg と試験)](#偽の-providerstg-と試験)
9. [人の作業](#人の作業)

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
| `frankfurter`(為替・欧州中央銀行・#1499) | 無料・鍵なし(`runs` だけ) | `latest(from?, to?, amount?)`・`historical(date, from?, to?, amount?)`。`from` / `to` は大文字 3 文字の通貨(`to` はカンマ区切りで 10 個まで・重ならない)・`amount` は正の数・`date` は 1999-01-04 から今日まで。行は通貨ごと(見出し = 通貨・値 = レート・補足 = 「日付・量 基準」) | `https://api.frankfurter.app` だけ・本文 64 KiB まで | latest 1 時間・historical 1 日 | `EXT_FRANKFURTER_ENABLED=1` |

provider が 404 / 422(持っていない通貨・日)を返した時は 400 `bad_request`(`runs` は戻す)。429 / 5xx は 502 `upstream_unavailable`。

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
- 回数の上限の値(`EXT_LIMIT_*`・`EXT_GLOBAL_DAILY*`)を各社の割り当てに合わせて runner の env に入れる(非公開の資料から。入れなければコードの既定)。
- プライバシーポリシー・利用規約(`docs/legal`)に外の API への送信とトークンの保管・消し方を書く。
