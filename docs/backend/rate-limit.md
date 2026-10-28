# ai の利用上限(#280)

端末(ProGate)で数える無料枠は、入れ直しや匿名アカウントの作り直しで戻る。ai サーバーでも利用者ごと・1 日ごとに数えて、Claude の費用が青天井にならないようにする。

- 実装: `backend/internal/ratelimit`(ミドルウェア)。`cmd/ai/main.go` で `ai.NewHandler` を包むだけで、handler は触っていない。
- 対象: `POST /v1/generate`・`POST /v1/chat`・`POST /v1/text`(#417)。ai 自身が Firebase ID トークンを確かめる(今までは appconfig に転送するだけで、chat の create は**トークン無しでも Claude を呼べた**)。
- 応答: 429 `rate_limited`。`details` は `openapi-ai.yaml` の `RateLimitDetails`、`Retry-After` も付く。

## 重さ(単位)

| 依頼 | 重さ | 理由 |
| --- | --- | --- |
| chat 1 往復 | 1 | 短い出力・軽いモデル設定 |
| text 1 回(aiButton generate のオンライン版・#417) | 1 | 出力 400 トークンまで・入力 4 KB まで・thinking なし |
| generate の修正(mode edit) | 2 | patch 中心 |
| generate の新規作成(create / auto) | 3 | 全体生成 + 検証のやり直し + logic pass |
| generate の undo | 0 | Claude を呼ばない |

auto はミドルウェアの時点で修正か新規かが分からないので、3 で数える(iOS は mode auto を送るので、修正も 3 になる。気になるなら iOS が edit を送るように変える)。

## 上限の既定値(1 日・0:00 JST で切り替え)

| 環境変数 | 既定 | 目安 |
| --- | --- | --- |
| `RATE_LIMIT_ANON_DAILY` | 30 | 新規 3 回(9)+ 修正・対話で 21。端末の無料枠(新規 3 回/日)に合わせた |
| `RATE_LIMIT_USER_DAILY` | 60 | サインイン済みは匿名の 2 倍 |
| `RATE_LIMIT_PRO_DAILY` | 300 | Pro でも上限はある(緩い)。サーバーでの Pro 判定はまだ無い |
| `RATE_LIMIT_IP_DAILY` | 300 | 匿名だけ。同じ IP からの匿名の合計(作り直し対策) |
| `RATE_LIMIT_GLOBAL_DAILY` | 2000 | 環境全体の費用ブレーカー。新規作成なら約 660 回 |
| `RATE_LIMIT_WEIGHT_CREATE` / `_EDIT` / `_CHAT` / `_TEXT` | 3 / 2 / 1 / 1 | |
| `RATE_LIMIT` | on(GOOGLE_CLOUD_PROJECT があれば) | `off` で無効。ローカルで素の起動なら既定 off |
| `RATE_LIMIT_STORE` | firestore | `memory` でインスタンスごとのメモリ |
| `RATE_LIMIT_COLLECTION` | rateLimits | |

0 を入れるとその上限は数えない。値は `--set-env-vars` で変える(deploy-cloudrun.yml は全置換なので、変えるならワークフローに書く)。

費用ブレーカーの目安: 1 回の新規作成を $0.03〜0.10 と置くと(入力 1〜2 万トークン + 出力数千トークン。実測で直す)、2000 単位 ≒ 最大 $20〜65/日/環境。stg と prod は別々に数える。

## 数える場所(Firestore・appconfig と同じ DB)

```
rateLimits/{env}_{yyyy-mm-dd}_uid_{uid}        {count, scope, expireAt, updatedAt}
rateLimits/{env}_{yyyy-mm-dd}_ip_{sha256 先頭 8 バイト}   匿名だけ。生の IP は残さない(IPv6 は /64 単位)
rateLimits/{env}_{yyyy-mm-dd}_global
```

- `env` は `APP_ENV`(stg / prod)。同じ Firestore でも環境ごとに分かれる。
- 1 回の依頼 = 1 トランザクション。全部の数を読み、どれか 1 つでも超えるなら**何も書かず** 429。超えなければ全部に足す。
- Firestore が使えない時(障害・権限)は、インスタンスのメモリで数え続ける(止めない・青天井にもしない)。
- `expireAt` = その日の終わり + 1 日。TTL ポリシーで自動で消える(下の手順)。

### 書き込みの見積もり

1 回の依頼で 読み 2〜3・書き 2〜3(匿名は IP の分が増える)。1000 人 × 10 回/日 = 1 万回なら 読み 3 万・書き 3 万/日。無料枠(読み 5 万・書き 2 万/日)を少し超えても数セント/日で、Claude の 1 回分より安い。

`global` は 1 つのドキュメントに全員が書く。Firestore は 1 ドキュメントあたり毎秒 1 回程度が目安で、Claude の 1 回が数十秒かかる今の規模なら足りる。毎秒数回を超えるようなら、global を数個に分ける(shard)。

## 抜け道への対策

| 対策 | 状態 |
| --- | --- |
| 匿名は上限が低い(30 vs 60) | 入れた |
| 匿名だけ IP ごとの合計上限(300) | 入れた。会場の Wi-Fi のように 1 つの IP に大勢がいる時は、サインイン済みは数えない。匿名の大勢がいるなら `RATE_LIMIT_IP_DAILY` を上げる |
| 全体の費用ブレーカー | 入れた |
| App Check(App Attest)で「本物のアプリからの依頼」だけ通す | **人の作業**(下)。いちばん効く |
| Pro をサーバーで確かめる | 差し込み口(`ratelimit.ProChecker`)だけ。App Store Server API で署名済みトランザクションを確かめる仕組みを足したら繋ぐ |

### 人の作業(この PR ではしていない)

1. **Firestore の TTL**(1 回だけ):
   `gcloud firestore fields ttls update expireAt --collection-group=rateLimits --enable-ttl --project=<PROJECT_ID>`
2. **App Check**:
   1. Firebase コンソール > App Check で iOS アプリを登録、プロバイダは App Attest(シミュレータ・開発ビルドは debug provider)。
   2. iOS: `FirebaseAppCheck` を入れ、`AppCheck.setAppCheckProviderFactory(AppAttestProviderFactory)` を `FirebaseApp.configure()` の前に。ai への依頼に `X-Firebase-AppCheck: <token>` を付ける。
   3. ai: Admin SDK の `appcheck.Client.VerifyToken` で確かめるミドルウェアを足す(まずはログだけ → 様子を見て必須にする)。
   4. Firebase Auth の匿名サインインにも App Check の強制(Identity Platform)をかけると、匿名アカウントを機械的に量産できなくなる。
3. **デプロイ後の確認**: ai のログに `ai: rate limit on (store=firestore ...)` が出ること、Firestore に `rateLimits` ができること。

## まだやっていないこと

- 失敗した依頼(Claude の 5xx など)の分は返さない。iOS の「もう一度」で 2 回分数える。
- `DELETE /v1/chat/{appId}` は今もトークン無しで呼べる(他人の対話状態を消せる)。別 Issue にする。
