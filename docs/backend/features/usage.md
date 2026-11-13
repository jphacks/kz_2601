# 今の使用量: /v1/me/usage の数え方・無料と Pro の上限・どこで数えているか(#317・#630)

> [バックエンド設計](../architecture.md) から分けた機能の文書。決まりの本体は [rate-limit.md の「今の使用量」](../rate-limit.md#今の使用量317630) と [「プランごとの上限」](../rate-limit.md#プランごとの上限317)、運用で変える値は [runtime-config.md の「運用で変える値」](../../infra/runtime-config.md#運用で変える値)。ここはその 2 つをつなぎ、端末が何をどこに出しているかを足す。API は [openapi-appconfig.yaml](../openapi-appconfig.yaml) の tag `usage`。コードは `backend/internal/appconfig/usage.go`・`backend/cmd/appconfig/usage.go`。

## 目次
- [1. 返す値](#1-返す値)
- [2. どこで数えているか](#2-どこで数えているか)
- [3. 無料と Pro の上限](#3-無料と-pro-の上限)
- [4. 端末の出し方](#4-端末の出し方)
- [5. 決めること](#5-決めること)

## 1. 返す値

`GET /v1/me/usage`(本人の分だけ・`Cache-Control: private, no-store`):

| 欄 | 中身 |
|---|---|
| `ai` | 今日の生成・チャット・text の合計(1 つの枠)。`used` / `limit` / `remaining` と `weights`(1 回で数える単位)。appconfig が ai の数を読めない時は null |
| `appSlots` | アプリの数の上限の内訳 `{base, purchased, limit, canBuySlot, slotCoins}`(アプリを数えない時は null) |
| `apps` | 持っているアプリ(自分で作ったもの + 参加した共有のアプリ)の数と上限 = プランの数 + 買った枠 |
| `perApp` | 自分のプランのアプリごとの上限 `{recordsPerApp, photosPerApp}`(数えない時は null) |
| `plan` | `free` / `pro`(サーバーで確かめた Pro の権利から) |
| `resetAt` | 次の 0:00 JST(UTC で返す) |
| `tier` | `anonymous` / `signed_in` / `pro`。`ai.limit` をどの上限で決めたか |

`GET /v1/apps/{appId}/usage`(アプリを開いた時):

| 欄 | 中身 |
|---|---|
| `payer` | `self`(自分のアプリ)/ `creator`(他の人が作ったアプリ。作成者の uid は返さない) |
| `photos` / `records` | 呼んだ人から見える分の写真・記録の数と、払う人のプランの上限 |
| `plan` | 払う人のプラン |
| `upgrade` | 払う人が無料で、Pro にすると記録か写真の上限が上がる |

- 上の `used` / `limit` / `remaining` の組は `UsageMeter`。`limit` と `remaining` が null = 数えない(環境変数が 0・`PLAN_LIMITS_MODE=off`)
- 集計に失敗した所だけ null にして、他の所は返す(ログ `appconfig: usage: ... failed`)
- 自分の写しが無いアプリは 404(ここでは写しを作らない)

## 2. どこで数えているか

| 数 | 書く所 | 読む所 | 遅れ |
|---|---|---|---|
| `ai`(今日の単位) | ai の `internal/ratelimit` が依頼のたびに `rateLimits/{APP_ENV}_{yyyy-mm-dd}_uid_{uid}` に足す([数える場所](../rate-limit.md#数える場所firestoreappconfig-と同じ-db)) | appconfig が同じ Firestore・同じ `RATE_LIMIT_COLLECTION`・同じ `APP_ENV` のその 1 文書を読む(`ratelimit.Peeker`) | 無し(キャッシュしない) |
| `apps` | appconfig の `users/{uid}/apps`(version が 1 より大きいもの) | count 集計 1 回 | インスタンスごとに 30 秒(`usageCountTTL`) |
| `records` / `photos` | appconfig のレコード | entity ごと + 写真の項目ごとの count 集計(見えない shared の entity は数えない) | (呼んだ人・アプリ・版)ごとに 30 秒 |

- `ai.limit` は ai と同じ式: tier の上限を `X-Client-Platform` の OS の上書きで替えたもの(`PlatformDailyLimit`・#2133)。だから `remaining` は ai の応答の `X-RateLimit-Remaining` と同じ数になる(違うのは、返した分(#548)がここにはすぐ入ること)
- appconfig は ai の `RATE_LIMIT_*` を自分でも読む(`ratelimit.ConfigFromEnv`)。値が壊れていると起動を止める。ai の値を運用で変えたら appconfig にも同じ値を入れる
- `RATE_LIMIT_STORE=memory` か、appconfig が Firestore でない(ローカルの memory store)時は ai がインスタンスごとに数えているので `ai` は null。起動ログ `usage` の `aiUnits` で分かる

## 3. 無料と Pro の上限

値はここに写さない(変わるたびにずれるので)。表は `backend/internal/plan`(`plan.Default`・`plan.FromEnv`)の 1 か所で、文書は [rate-limit.md の「プランごとの上限」](../rate-limit.md#プランごとの上限317) と [「上限の既定値」](../rate-limit.md#上限の既定値1-日000-jst-で切り替え)。

| 上限 | 環境変数 | 止める所 |
|---|---|---|
| アプリの数 / 人 | `PLAN_FREE_APPS` / `PLAN_PRO_APPS` + 買った枠([持てるアプリの数](../rate-limit.md#持てるアプリの数630)) | appconfig(402 `app_limit_reached`) |
| 生成・チャット・text / 日 | `RATE_LIMIT_ANON_DAILY` / `_USER_DAILY` / `_PRO_DAILY`(と OS ごとの `_<OS>`)・重みは `RATE_LIMIT_WEIGHT_*` | ai(429 `rate_limited`) |
| 写真 / アプリ | `PLAN_FREE_PHOTOS_PER_APP` / `PLAN_PRO_PHOTOS_PER_APP` | appconfig(403 `plan_limit_reached`) |
| 記録 / アプリ | `PLAN_FREE_RECORDS_PER_APP` / `PLAN_PRO_RECORDS_PER_APP` | appconfig(403 `plan_limit_reached`) |

- 強制の段階は `PLAN_LIMITS_MODE`(`enforce` / `log` / `off`)。`off` では `apps`・`perApp`・`records`・`photos` の `limit` が null になる
- どのプランで数えるか: 自分のアプリは呼んだ人、他の人が作ったアプリは作成者

## 4. 端末の出し方

| 端末 | 読む API | 出す所 | キャッシュ |
|---|---|---|---|
| Android | `/v1/me/usage`・`/v1/apps/{appId}/usage` | 音声の画面の「今日の残り」(`RemainingTodaySection`)・設定の「使っている量」(`UsageSection`・アプリの数と、作成者の時だけ今のアプリの記録・写真) | 5 分(`VoiceViewModel`・iOS と同じ) |
| iOS | `/v1/me/usage`・`/v1/apps/{appId}/usage` | 「今日の残り」・設定の「使っている量」(`UsageSettingsSection`)・上限に近い時(80% 以上)の 1 行のお知らせ(`UsageNoticeRules`・1 アカウント・1 リソースにつき 1 日 1 回) | 5 分(`AppRuntime+PlanUsage`) |
| Mac | `/v1/me/usage` だけ | 「今日の残り」・設定の「使っている量」(アプリの数だけ。設定のウインドウはアプリに属さないので記録・写真は出さない・`MacUsageSettings`) | 5 分 |
| Windows・Linux(`desktop/`) | 読まない | - | - |

- 参加者(`payer: creator`)には記録・写真の量も Pro の案内も出さない(iOS・Android)
- 「今日の残り」は単位で出し、「回」とは言わない(新しく作るのはあと何回 = `remaining / weights.create`)
- 古いサーバー(404)・モックでは出さない

## 5. 決めること

- Windows・Linux(`desktop/`)で使用量を出すか。今は既定がモックで、本物の API は手元の確かめにだけ使う(サインイン #1749 の前・`desktop/.../net/DesktopApi.kt`)。出すなら共有の `android/services` の `PlanUsage` をそのまま使える(推測)
