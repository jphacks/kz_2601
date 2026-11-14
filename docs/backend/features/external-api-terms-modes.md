# 外の API の規約を作りで守る(Terms・SunsetAt)

> 2026-11-15 のオーナーの方針「外の API はできるだけ入れる。規約に合わせて作りを変え、外さない」の実装メモ。規約を読んだ記録は [research/extapi-terms-2026-11.md](../../research/extapi-terms-2026-11.md)(PR #3732)。入れる・入れないの決定は [decisions/external-apis.md](../../decisions/external-apis.md)。値段はここに書かない。

## 目次

1. [registry の項目](#registry-の項目)
2. [どこで効くか](#どこで効くか)
3. [AI に渡さない(NoAI)の決まり](#ai-に渡さないnoaiの決まり)
4. [provider ごとの設定](#provider-ごとの設定)
5. [connpass(有料のプラン)](#connpass有料のプラン)
6. [Spotify(試験の間だけ)](#spotify試験の間だけ)
7. [Zaim(終わるまで)](#zaim終わるまで)

## registry の項目

`backend/internal/extapi/terms.go` の `Terms` を `Provider.Terms` に書く。

| 項目 | 意味 | 作りでの効き方 |
|---|---|---|
| `NoCharge` | 規約が課金を禁じる(直接・間接)・非商用だけの API | Pro だけにしない・`runs` を数えない・コインを引かない(無料の版でも使える)。回数の上限(`limits.go`)は効く。有料の provider には付けられない |
| `NoAI` | 結果を LLM に渡してはいけない | 答えに `noAI: true` を付ける(端末は AI の材料から外す)。DSL の決まり `dsl.CheckExternalUse` が AI への配線を断る |
| `ShowOwnDataOnly` | OAuth のデータはつないだ本人にだけ見せる | 自動の動き(作成者のトークンで全員のために呼ぶ)は 400。共有・公開の entity があるアプリ・他の人のアプリからは 403 `forbidden`(`details.reason: provider_own_data_only`)。OAuth の provider だけ |
| `Attribution` | 規約が求める表示(1 行・300 文字まで) | 答えの `attribution` に入れる(op が自分で入れた時はそちらが勝つ)。端末は行の下に出す |

ほかに `Provider` の 2 つ:

| 項目 | 意味 |
|---|---|
| `SunsetAt` | API が終わる時刻。過ぎたら off(503 `ext_unavailable`)・「つないだサービス」の一覧に出さない・審査用の版の機能のキーから外す・DSL の決まりが断る |
| `UserMinute` | 1 人の 1 分の既定の回数(0 = なし)。`EXT_LIMIT_USER_MINUTE` / `EXT_LIMIT_<PROVIDER>_USER_MINUTE` が勝つ。超えると 429 `rate_limited`(`scope: user_minute`) |

## どこで効くか

| 所 | ファイル |
|---|---|
| Pro・`runs`・コイン(NoCharge)・自動の動きを断る(ShowOwnDataOnly)・SunsetAt | `internal/extapi/service.go`(`Call`・`check`・`Enabled`) |
| 共有のアプリを断る(ShowOwnDataOnly)・答えに `attribution` / `noAI` | `internal/appconfig/ext.go`(`extOwnDataOnly`・`Provider.WithTerms`) |
| 一覧から外す(SunsetAt) | `internal/appconfig/ext.go`(`connectable`)・`internal/extapi/registry.go`(`ReviewBuildFeatures`) |
| DSL の決まり(NoAI・ShowOwnDataOnly・SunsetAt) | `pkg/dsl/external_terms.go`(`CheckExternalUse`・`ExternalTerms` は extapi が `init()` で入れる) |
| 1 分の上限 | `internal/extapi/limits.go`(`UserMinute`) |

契約の変更(`openapi-appconfig.yaml`): `ExtResult` に `attribution`・`noAI` を足した。403 に `provider_own_data_only`、429 の scope に `user_minute` を足した。

## AI に渡さない(NoAI)の決まり

- 今の DSL では、外の API の結果は AI に届かない(`aiButton.inputFeeds` が指せるのは `feed` だけ・`externalCall` / `externalList` はまだ DSL に無い・#1643)。
- `externalCall` / `externalList` を DSL に足す時は、部品ごとに `dsl.CheckExternalUse(app, where, provider, toAI)` を呼ぶ。`toAI` は、その結果が `aiButton.inputFeeds`・`aiChat` の材料・AI の入力の欄に流れる時に true。
- 端末は答えの `noAI: true` を見て、その行を AI の材料(「外部データ」の節)に入れない。
- 学習には、どの provider の結果も使わない。

## provider ごとの設定

| provider | 場所 | NoCharge | NoAI | ShowOwnDataOnly | Attribution | ほか |
|---|---|---|---|---|---|---|
| `jira` | #3712 | ○(Atlassian Developer Terms 8.3) | | ○ | | 原価は runner の呼び出しだけ(オーナー OK) |
| `dropbox` | #3713 | | | ○ | | 50 人をつなぐ前に本番の承認 |
| `backlog` | #3718 | | | ○ | | 求められたら表示を足す |
| `strava` | #3715 | | ○(Strava に確かめるまで) | ○ | Powered by Strava | |
| `spotify` | epic/god | ○ | ○ | ○ | ○ | 試験の間だけ(下) |
| `tmdb` | #3726 | ○ | ○ | | TMDB の決まった文 | 既定 off・商用の契約まで |
| `lastfm` | #3724 | ○ | | | Last.fm | 既定 off・商用の契約まで |
| `yolp` | #3725 | ○ | | | Web Services by Yahoo! JAPAN | 既定 off・個別の許可まで |
| `nhk` | #3723 | ○(規約を読むまで) | | | NHK | 既定 off・規約の本文は人がブラウザで読む |
| `unsplash` | #3730 | | | | Unsplash | 撮影者の表示は行ごと |
| `openMeteo` | #3716 | ○(無料の API の間) | | | Open-Meteo(CC BY 4.0) | 有料の API の契約で外せる |
| `connpass` | epic/god | | | | connpass | 有料のプラン・控えめの上限(下) |
| `zaim` | epic/god | | | ○ | | `SunsetAt` 2026-12-07 00:00 JST |
| `estat`・`wikipedia`・`openWeather`・`podcasts`・`googlePlaces`・`odpt`・`gtfsRealtime` | epic/god | | | | 今までの文言 | |

## connpass(有料のプラン)

- 2026-11-15 オーナー: 有料のプランで使う。鍵(`EXT_CONNPASS_API_KEY`)が無ければ provider は off のまま(`newConnpass` が断る)。
- 控えめの上限(env で変えられる): 環境全体 1 日 80 回(`EXT_GLOBAL_DAILY_CONNPASS`)・1 人 1 分 3 回(`EXT_LIMIT_CONNPASS_USER_MINUTE`)・既定の 1 時間 / 1 日の上限。同じ引数の答えは 10 分持つ。connpass が 429 を返したら 1 分止まる。
- 契約の手順と、第三者向けに出すための事前の承諾は人の作業(オーナーの計画のファイル)。

## Spotify(試験の間だけ)

- 開発モード(Development mode)のまま、チームの試験に使う。
- **5 人の上限は、自分の Spotify アカウントをつなぐ利用者の数**(Spotify のダッシュボードの許可リストに入れた人)。サーバーの台数ではない。チームの試験には足りる。
- 本番に出すには拡張の枠(Extended Quota)が要る。今の Spotify の条件(法人・25 万 MAU 以上)は満たせないので、本番では off のまま。

## Zaim(終わるまで)

- Zaim API は 2026-12-07 に終わる([お知らせ](https://content.zaim.net/notices/show/2280))。オーナー: 終わるまで残す。
- `SunsetAt` を 2026-12-07 00:00 JST にしたので、その時から自動で off になる(デプロイは要らない)。後で provider のファイルを消すかはオーナーが決める。
