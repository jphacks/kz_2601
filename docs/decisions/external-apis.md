# 生成したアプリから外の API を使う(設計)

> 状態: **判断待ち(下書き)**。2026-11-11 作成。方向(無料の API は枠・有料の API はコイン・サーバーから呼ぶ)は 2026-11-11 にオーナーが決めた(正典 [`docs/00_decisions.md`](../00_decisions.md) の「課金・Pro」)。この文書は作りの案で、「決めてほしいこと」はオーナーが決める。親 Issue ができたら `<番号>-external-apis.md` に名前を変える。
> 前提: コイン課金 [#630](630-token-billing.md)(押さえ → 確定・サーバーの枠と超えた後のコイン払い)・費用の作り [#338](338-server-feature-cost-model.md)・悪用と脅威 [#339](339-abuse-threat-model.md) の「3. 外部への送信」・runner([infra/runner.md](../infra/runner.md))・外部の公開データ [features/external-feeds.md](../backend/features/external-feeds.md)・本の情報 [features/book-lookup.md](../backend/features/book-lookup.md)・通話 [features/calls.md](../backend/features/calls.md)。
> **値段は書かない。** 1 コインの値段・API ごとの原価・枠の大きさ・粗利は非公開の資料で決める。この文書は「原価(USD)に `COIN_PER_USD` を掛ける」とだけ書く。

## 目次

1. [方向(2026-11-11)](#方向2026-11-11)
2. [今あるもの](#今あるもの)
3. [どこに足すか](#どこに足すか)
4. [呼び出しの流れ](#呼び出しの流れ)
5. [数え方(無料の API・有料の API)](#数え方無料の-api有料の-api)
6. [DSL の形](#dsl-の形)
7. [利用者ごとの OAuth](#利用者ごとの-oauth)
8. [API キーの保管](#api-キーの保管)
9. [失敗・回数の上限・キャッシュ](#失敗回数の上限キャッシュ)
10. [審査・規約](#審査規約)
11. [入れる API の候補](#入れる-api-の候補)
12. [Issue の分け方](#issue-の分け方)
13. [決めてほしいこと](#決めてほしいこと)
14. [参考](#参考)

## 方向(2026-11-11)

| 項目 | 方向 |
|---|---|
| 呼ぶ所 | **サーバー(runner)**。端末から外の API を直接呼ばない。API キーと利用者ごとの OAuth のトークンは端末に置かない |
| API 自体が無料のもの | 外の API の 1 回の呼び出しを**サーバーの枠(`runs`)の 1 回**として数える。Pro の月の枠(`ALLOWANCE_PRO_RUNS`)を超えた分だけ、今の「超えた後のコイン払い」(`COIN_OVERFLOW_*`)でコインを引く。**API の代金としては引かない** |
| API 自体が有料のもの | 1 回の原価(USD)に `COIN_PER_USD` を掛けた分のコインを引く。AI と同じ **押さえる → 実費で引く → 差額を戻す**(`coins.Service` の Reserve / Commit / Release) |
| 有料の機能か | サーバーの機能なので **Pro が要る**(00_decisions の「有料の機能」②と同じ線。下の決めてほしいこと 1) |
| 任意の URL | 作らない。#348 と同じく**閉じた一覧(provider)と決まった操作だけ**。DSL に URL・ヘッダー・本文の形は書けない |

## 今あるもの

| もの | 場所 | 外の API との関係 |
|---|---|---|
| runner | `backend/cmd/runner`・`internal/runner` | Cloud Scheduler の毎分の `POST /internal/tick` で outbox ときっかけを処理する背景のサービス。専用の SA・APNs の鍵を持つのは runner だけ(`infra/runner.tf` の `runner_secret_ids`)。利用者に開いた HTTP の口は無い |
| runner の枠 | `internal/runner/quota`(`allowance.go`) | Pro の作成者ごと・JST の月ごとに `runs` / `recipients` / `writes` / `reads` を数える。超えたら `coins.Service.BuyOverflow` でブロックを買う(`COIN_OVERFLOW_BLOCK_<KIND>` / `COIN_OVERFLOW_PRICE_<KIND>`)。止めた理由は `skipped_allowance` / `skipped_overflow_cap` / `skipped_coins` |
| コイン | `internal/coins` | `Reserve`(op ごとの `COIN_RESERVE_<OP>` か呼び出し側の `Max`)→ `Commit`(`est_cost_usd × COIN_PER_USD`・押さえの量まで)/ `CommitCoins` / `Release`。押さえは `HoldTTL` で期限切れ → 戻る。`COIN_BILLING_MODE` = off / log / enforce |
| 外部の公開データ `feed` | `internal/feeds`・appconfig `GET /v1/feeds` | 鍵の要らない許可リスト(気象庁・MET Norway・内閣府・arXiv・国会会議録など)。出口の検査・本文の上限・キャッシュ・1 分の取得の予算・`FEEDS_DISABLED`。runner の `saveFeed` も同じ `feeds.Service` を通る |
| 本の情報 `lookupBook` | `internal/books`・appconfig `GET /v1/lookup/book` | 鍵の要らない出どころ 1 つ(openBD)。既定で止めている |
| 開く・送る `openUrlButton.app` | iOS `OpenApp` | 他のアプリを開くだけ。サーバーは何もしない |
| 端末の仕組み | iOS | `addToCalendar`(EventKit・端末に設定した Google カレンダーにも入る)・`musicPlayer`(MusicKit)・`placeSearch`(MapKit)は**端末の OS の仕組み**で、鍵もサーバーも要らない |

## どこに足すか

**runner に「外の API の口」を足し、appconfig は認証と Pro の確認だけして runner に渡す。**

| 案 | 中身 | 判断 |
|---|---|---|
| A. appconfig に足す(feed・lookupBook と同じ) | 速い・作りが小さい | **採らない**。appconfig の SA(`run-backend`)が外の API キーと利用者のトークンを読めるようになる。鍵を読める所を広げない(#337 の「鍵を持つのは runner だけ」と同じ考え) |
| B. runner に足す(推奨) | runner に内部の口 `POST /internal/ext/call` を足す。appconfig が利用者の要求を受けて、サービス間の認証(OIDC・invoker は appconfig の SA だけ)で runner を呼ぶ。自動の動き(serverRules)からは runner の中で同じ関数を呼ぶ | **推奨**。鍵とトークンを読めるのは runner の SA だけ。枠(`quota`)とコインの数え方が 1 か所になる |
| C. 外の API ごとに新しいサービス | 鍵の範囲を最も狭くできる | 採らない。API の数だけデプロイが増える(00_decisions「サービスを増やすのは担当かスケール特性が違う時」) |

B の注意:

- runner は今 `最大 2 台・--timeout=90・認証なし不可`。利用者が押して待つ呼び出しが入るので、**最小 1 台**(冷えた起動を避ける)と最大の台数を上げるかを決める(決めてほしいこと 4)。
- 外の API の呼び出しはきっかけの処理(tick)と同じ台で動くが、**別の同時実行の上限**(セマフォ)を持たせ、tick を詰まらせない。
- `#288 の分離までは runner を prod だけ` の決まりがあるので、stg で試す時は外の API の口だけを stg の runner に出すか、stg は**偽の provider**(固定の答え)で動かす(決めてほしいこと 5)。

## 呼び出しの流れ

### 利用者が押す(同期)

```
iOS(DSL のアクション externalCall)
  → appconfig POST /v1/apps/{appId}/ext/{provider}/{op}   ← Firebase の ID トークン・App Check
      ・DSL にそのアクションがあるか(公開の DSL の版・provider と op が一覧にあるか)
      ・Pro か(作成者の Pro・共有アプリなら作成者)・止め弁
      ・引数を op の型で検証(長さ・値の範囲。URL は受けない)
  → runner POST /internal/ext/call(OIDC)
      ・数える(下の「数え方」)
      ・利用者の OAuth のトークンを読み、要れば更新
      ・外の API を呼ぶ(出口の許可リスト・時間切れ・本文の上限)
      ・答えを op の決まった形に直す(余計な項目は捨てる)
      ・確定(実費で引く・差額を戻す)/ 失敗なら戻す
  ← 決まった形の JSON(行の一覧・1 件・成功の印)
```

- 端末は `provider` と `op` と引数だけを送る。URL・ヘッダー・トークンは端末に来ない・端末から送れない。
- 答えは `feed` と同じく**表示の形に直した値だけ**を返す(外の API の生の JSON を返さない)。保存は利用者が押した時だけ `saveRecord` などの今の経路で(勝手に保存しない)。

### 自動の動き(非同期)

serverRules の処理に `externalCall` を足す(`saveFeed` と同じ位置)。runner の中で同じ関数を呼ぶ。トークンを使う op(カレンダーに書くなど)は**作成者本人のトークンだけ**を使う(参加者のトークンを作成者のルールで使わない)。失敗の扱いは今の runner(冪等・回数の上限つきの再試行・止めた理由を作成者に出す)のまま。

## 数え方(無料の API・有料の API)

provider ごとに「無料 / 有料」と原価の単位を**サーバーの表**(`internal/extapi` の provider の定義)に持つ。原価の数(USD)は環境変数 `EXT_COST_USD_<PROVIDER>_<OP>`(値は非公開の資料・既定 0)。

| 種類 | 何で払うか | 数える単位 | 実装 |
|---|---|---|---|
| **API 自体が無料**(無料の枠の中で使う・鍵だけ要る・OAuth だけ要る) | **サーバーの枠 `runs` の 1 回**。Pro の月の枠を超えたら、本人が「超えた後のコイン払い」を ON にしていればブロックを買う(`COIN_OVERFLOW_*_RUNS`)。OFF なら止める(`skipped_allowance`) | 外の API への 1 回の要求 = `runs` 1 | `quota.allow(ctx, dec, coins.KindRuns, 1)` をそのまま使う。新しい種類(kind)は足さない |
| **API 自体が有料**(呼ぶたびに原価がかかる) | **コイン**。`coins = ceil(原価 USD × COIN_PER_USD)`(最小 `COIN_MIN_PER_CALL`)。押さえる → 実費で引く → 差額を戻す | 外の API の課金の単位(1 回・1,000 文字・1 件など)を USD に直した 1 回の原価 | `coins.Service.Reserve`(新しい op `ext`・押さえの量は `COIN_RESERVE_EXT_<PROVIDER>`)→ `Commit(Usage{USD, Priced})` / 失敗は `Release`。履歴の種類 `spend_ext`(下の決めてほしいこと 2) |

- **有料の API も `runs` の 1 回として数えるか**: 数える(サーバーの処理の費用は同じようにかかるため)。ただし有料の API の代金は枠に含めない(決めてほしいこと 3)。
- **誰の枠・誰のコインか**: #630 と同じ。
  - 利用者が押す呼び出し: `runs` は**作成者**の枠(サーバーの費用は作成者・00_decisions「共有のアプリの費用は作成者の Pro」)、有料の API のコインは**押した人**(アプリの中の AI と同じ「使う人が払う」)。
  - 自動の動き: どちらも**作成者**。
- **料金が分からない時**(原価の表に無い・API が使った量を返さない): 押さえた量をそのまま確定する(`coins.unpriced_model` と同じ扱い・ログ `coins.ext_unpriced`)。
- **API の無料の枠を超えると有料になるもの**(例: 月 N 件まで無料): 運営のアカウントでの合計なので、利用者ごとには「無料」として扱えない。**有料の API として数え**、原価は超えた後の単価で見積もる(運営の無料の枠は粗利の側に入る)。
- **応答の見出し**: 有料の API は AI と同じく `X-Coins-Charged` / `X-Coins-Balance` を返す。押す前の「約 N コイン」は `GET /v1/me/coins` の `estimates` に `ext_<provider>` を足す。
- **`COIN_BILLING_MODE`**: off は数えない(今の runner の日の上限だけ)・log は `coins.would_spend` を出すだけ・enforce で止める。今のフラグに乗る。

## DSL の形

新しいアクション `externalCall` を 1 つ足し、provider と op は閉じた enum にする(feed の `provider` と同じ考え)。

```json
{"type": "button", "label": "Google カレンダーに入れる",
 "action": {"type": "externalCall", "externalCall": {
   "provider": "googleCalendar", "op": "createEvent",
   "args": {"titleField": "title", "startField": "start_at", "endField": "end_at"}}}}
```

```json
{"type": "externalList", "provider": "spotify", "op": "searchTracks",
 "queryField": "song", "limit": 10, "title": "曲を探す"}
```

| 決まり | 中身 |
|---|---|
| 読む op(検索・一覧) | 部品 `externalList`(`feed` と同じ描き方・行は見出し / 補足 / 値 / リンク)。保存はしない。「記録に追加」は `placeSearch` と同じく入力中のフォームの field を上書きするだけ |
| 書く op(予定を作る・タスクを足す・曲をプレイリストに入れる) | アクション `externalCall`。必ず確認のダイアログ(`confirmMessage` 必須)。書いた結果の id を field に残せる(`resultIdField`) |
| 引数 | op ごとに決まった名前だけ(`args` は `additionalProperties:false`)。値は field の参照か固定の値。URL・自由なヘッダーは書けない |
| 機能のキー | `supportedFeatures` に `externalCall.<provider>` を provider ごとに足す。送らない版の iOS には生成しない |
| 審査用の版 | OAuth の要る provider は最初は送らない(審査で「つなぐ」をどう見せるかが決まるまで)。鍵だけの読む provider は残してよい(決めてほしいこと 6) |
| 端末の仕組みで足りるもの | **端末の仕組みを先に使う**。端末のカレンダーへの追加は `addToCalendar`(EventKit)、Apple Music は `musicPlayer`、近くの場所は `placeSearch`。外の API は「サーバーで動く(自動の動き・共有)」「端末の仕組みに無い」時だけ AI に選ばせる |

`ActionType` に case を足すとホスト(`AppRuntime.run` の網羅 switch)の改修が要る(DSL README の注意)。`externalCall` 1 つにまとめ、provider と op の増減は iOS の改修無しで描ける形にする(行の形・成功の toast は共通)。

## 利用者ごとの OAuth

### つなぐ

| 段 | 中身 |
|---|---|
| 1. 始める | iOS の「つなぐ」(アプリの中のボタンか設定の「つないだサービス」)→ appconfig `POST /v1/me/connections/{provider}/start` → runner が `state`(ランダム・uid と provider と期限 10 分に結ぶ・1 回だけ)と PKCE の `code_verifier` を作り Firestore に置く → 認可の URL を返す |
| 2. 同意 | iOS は `ASWebAuthenticationSession` で開く(アプリの中の WebView は使わない。Google は埋め込みの WebView を禁じている)。scope は **op に要る最小**だけ(下) |
| 3. 戻る | 戻り先は **runner の HTTPS の口**(`/oauth/callback/{provider}`・認証なしで受ける唯一の口)。`state` を確かめ、code をトークンに替え、保管して `tailorit://connected?provider=…` に返す。戻り先にアプリのスキームを使わない(他のアプリに横取りされない) |
| 4. 表示 | 設定の「つないだサービス」に provider・つないだ日・使っているアプリ・**切る**を出す |

- **追加の scope**: 最初は op の最小の scope でつなぎ、書く op を初めて使う時に追加で同意を取る(段階的な許可)。
- **共有アプリ**: つなぐのはいつも**本人のアカウントだけ**。他人のトークンで他人のカレンダーに書く道を作らない。参加者が書く op を使う時は参加者がつなぐ。

### トークンの保管

| もの | 場所 | 中身 |
|---|---|---|
| リフレッシュトークン・アクセストークン | `users/{uid}/connections/{provider}` | **Cloud KMS で暗号化した値**(鍵は runner の SA だけが `encrypt/decrypt`)・scope・つないだ日・最後に使った日・状態(`active` / `revoked` / `error`) |
| `state` / PKCE | `oauthStates/{state}` | `expireAt`(10 分・TTL) |

- Firestore のルールでクライアントは**読めない・書けない**(サーバーだけ)。iOS には「つないでいるか・scope の名前」だけを `GET /v1/me/connections` で返す。
- アクセストークンは使う直前に期限を見て更新。更新に失敗(`invalid_grant`)したら `error` にして、次に押した時に「もう一度つないでください」。
- ログにトークン・認可コード・外の API の個人の中身(予定の題名など)を出さない。出すのは provider・op・結果・時間・uid のハッシュだけ。

### 切る・取り消し

| きっかけ | すること |
|---|---|
| 利用者が「切る」 | provider の取り消しの口を呼ぶ(Google `oauth2.googleapis.com/revoke`・Microsoft はアプリ側で消す・Spotify は取り消しの口が無いので消すだけで、利用者に Spotify の「アプリの管理」で外せることを案内)→ 保管した値を**消す**(`revoked` の印だけ残すかは決めてほしいこと 7) |
| provider の側で取り消された | 次の呼び出しの 401 / `invalid_grant` で `error` にして使わない。Google の「アカウントの保護」(RISC)の知らせを受けるかは後で決める |
| アカウント削除(`DELETE /v1/me`) | 論理削除の時に**すぐ**全部の connections を取り消して消す([account-deletion.md](../backend/features/account-deletion.md) の表に 1 行足す・端末のプッシュと同じ扱い)。30 日後の消去(`cmd/purge`)でも残りを消す |
| Pro が切れた | トークンは残し、使わない(戻った時につなぎ直さなくてよい)。90 日使わなければ消す(決めてほしいこと 7) |

## API キーの保管

- 運営の API キー・OAuth のクライアントの秘密は **Secret Manager**。`infra/runner.tf` の `runner_secret_ids` に足し、読めるのは runner の SA だけ(`run-backend` は読めない・今の APNs の鍵と同じ)。
- 名前は `EXT_<PROVIDER>_API_KEY` / `EXT_<PROVIDER>_CLIENT_SECRET`。Cloud Run の `--set-secrets`(全置換)で入れる。値はこの repo・Issue・PR・ログに書かない(public repo)。
- provider ごとの止め弁 `EXT_<PROVIDER>_ENABLED`(既定 off)と全体の `EXT_DISABLED`。鍵が無い provider は自動で off(起動のログに出す)。
- 鍵の入れ替えは Secret Manager の版を足して runner を入れ直すだけ。鍵の一覧は `docs/infra/credentials-inventory.md` に足す(人の作業・値は書かない)。

## 失敗・回数の上限・キャッシュ

| 項目 | 設計 |
|---|---|
| 時間切れ | 1 回 8 秒(利用者が押す呼び出し)。自動の動きは runner の今の再試行に任せる |
| 再試行 | 読む op だけ、429 / 5xx で 1 回(`Retry-After` を守る)。書く op は再試行しない(二重に作らない)。書く op は provider の冪等の鍵があれば `X-Request-Id` から作って渡す |
| 失敗とコイン | 外の API が失敗した・こちらの時間切れ → 有料のコインは**戻す**(`Release`)。`runs` の 1 回は戻す(`quota.Refund`)。外の API が 4xx(引数の誤り・見つからない)で費用がかかった時だけ確定する |
| 止まっている provider | 5xx が続いたら provider ごとに 1 分止める(`feeds` の `FailTTL` と同じ考え)。押すと「今は使えません」 |
| 回数の上限(暴走の歯止め・コインと別) | 利用者ごと・アプリごと・provider ごとの日の上限(`EXT_LIMIT_<SCOPE>`)と、運営の全体の日の上限(`EXT_GLOBAL_DAILY_<PROVIDER>`・provider の割り当ての手前で止める)。今の `ratelimit` の数え方を使う |
| provider の割り当て | 運営のアカウントの割り当て(1 日・1 秒)を表に持ち、全体の上限はその手前に置く。超えたら 503 `ext_busy` |
| キャッシュ | 鍵だけの読む op(天気・地図の検索・為替・本)は**同じ引数の答えを provider の規約が許す時間だけ**キャッシュ(Firestore か台の中のメモリ)。キャッシュから返した時は `runs` も コインも数えない。規約がキャッシュを禁じる provider(Google Maps の場所の詳細など)は場所の id だけ持つ。OAuth の op(本人のカレンダーなど)はキャッシュしない |
| 出口 | 外への接続は provider の決まったホストだけ(`feeds` の出口の検査と同じ・リダイレクトでホストが変わったら止める)。本文の上限(1 MB) |
| ログ | `ext.call`(provider・op・結果・時間・`runs` か coins か・キャッシュか)。`scripts/runner-usage.sh` の「止めた理由」に `ext_*` を足す |

エラーコード(appconfig が返す):

| コード | HTTP | 意味 |
|---|---|---|
| `ext_not_connected` | 409 | OAuth の要る op で、本人がつないでいない・取り消された |
| `ext_unavailable` | 503 | provider が止まっている・止め弁・鍵が無い |
| `ext_busy` | 503 | 運営の割り当ての手前の全体の上限 |
| `coins_insufficient` / `allowance_exhausted` / `overflow_cap_reached` / `pro_required` / `rate_limited` | 402 / 429 | 今のまま(#630) |

## 審査・規約

| 論点 | 扱い |
|---|---|
| Google の OAuth のアプリの審査 | `calendar.events` などの**センシティブな scope** は Google の確認(ブランドの確認 + scope の確認・デモ動画・プライバシーポリシーの URL)が要る。確認前は「未確認のアプリ」の画面が出て、利用者は 100 人まで。Gmail・Drive の全体などの**制限付きの scope** は年 1 回の第三者のセキュリティ評価(CASA)が要るので**使わない**。最初はカレンダーとタスクの scope だけ |
| Spotify の開発中のモード | 開発中のモードは**つなげる利用者が 5 人まで**(以前の 25 人から減った・2026-11-11 に [Quota modes](https://developer.spotify.com/documentation/web-api/concepts/quota-modes) で確かめた)で、管理画面で登録した Premium の人だけ。広く出す「拡張の割り当て」は 2025-05-15 から**法人だけ**が申し込め、登録した事業・公開済みのサービス・月 25 万人以上の利用者などが条件。つまり**個人や小さなチームでは一般公開できない**前提で、Spotify は「つなぐ」を後回しにし、今の「開くだけ」を続ける |
| 課金が許されない API | 規約が「この API の利用に料金を取る」「API のデータを売る」を禁じる provider は、**コインを引かず枠だけで数える**か、**入れない**。下の候補の表の「規約の注意」で印を付けた。Spotify(再生を含むアプリの販売・アプリ内課金の禁止)・Google Books(アプリの利用に料金を取ることの禁止)・Google Maps(内容を別のサービスとして売ることの禁止・キャッシュの制限)・NewsAPI(無料の版は開発だけ)・Open-Meteo(無料の版は非商用だけ)など |
| 「API の代金としては引かない」との関係 | 無料の API は枠(`runs`)= **サーバーの処理の費用**として数え、API のデータに値段を付けない。有料の API は**運営が払った原価を利用者が払い戻す**形で、データを売る形ではない。それでも規約が「利用者への再販」を禁じる provider は、法務の確認まで有料で入れない(人が確かめる) |
| Apple のガイドライン | 外のサービスのアカウントをつなぐのは任意(つながなくてもアプリは使える)。コインは App 内課金だけ(#630)。外のサービスのログインを Tailor It のアカウントの代わりにしない |
| プライバシーポリシー・利用規約 | 外の API に送るもの(検索語・予定の中身・位置のおおよそ)と、トークンの保管・消し方を書く必要がある。**`docs/legal` はこの文書では変えない**。入れる前に人が書く(Issue の「人がすること」) |
| 出典の表示 | provider の求める表示(Google の「Powered by Google」・Spotify のロゴと曲へのリンク・OpenWeather など)を `externalList` の下に出す。表示の決まりは provider の定義に持つ |

## 入れる API の候補

2026-11-11 に各社の公開の資料で確かめた(出典の列・候補は 27 個)。**値段は書かない**(有料のものは課金の単位だけ)。「OAuth」は利用者ごとのトークンが要るか。「規約の注意」の **[×課金]** は、規約が API の利用・データへの課金や商用を禁じる・強く縛るもの(上の「課金が許されない API」)。

「確かめ」の列: **済** = 各社の公開のページを読んだ・**一部** = 一部の項目は記憶や二次の情報・**未** = ページを読めなかった(入れる前に人が読む)。日本で使えるかは、読んだページに日本を外す決まりが無いことだけを確かめた。

| # | API | 分野 | 無料 / 有料(有料の単位) | OAuth・審査 | 規約の注意 | 確かめ | 出典 |
|---|---|---|---|---|---|---|---|
| 1 | Google Calendar API | カレンダー | 無料(割り当てあり) | 要る。`calendar.events` は**センシティブな scope** で Google の確認(理由・デモ動画)が要る | Google API サービスの利用者データのポリシー(使う目的を絞る・広告に使わない) | 一部 | [センシティブな scope の確認](https://developers.google.com/identity/protocols/oauth2/production-readiness/sensitive-scope-verification) |
| 2 | Microsoft Graph(Outlook の予定) | カレンダー | 無料(スロットリングあり) | 要る。職場・学校のアカウントには発行元の確認(publisher verification)が要る。個人の outlook.com は不要 | Microsoft APIs の利用規約 | 未 | [Microsoft Graph](https://learn.microsoft.com/graph/overview) |
| 3 | Google Tasks API | タスク | 無料 | 要る。scope はセンシティブの見込み(未確認) | Calendar と同じ | 一部 | 同上 |
| 4 | Todoist API | タスク | 無料(使える機能は利用者の Todoist のプラン次第) | 要る。審査は Todoist の一覧に載せる時だけ | 名前は「x for Todoist」・「Todoist と提携していない」の断り書き | 済 | [Todoist API v1](https://developer.todoist.com/api/v1/) |
| 5 | Notion API | タスク・メモ | 無料 | 要る(public integration)。つなぐのに審査は要らない | 回数の上限(1 秒あたり数回)を守る | 済 | [Authorization](https://developers.notion.com/docs/authorization) |
| 6 | Spotify Web API | 音楽 | 無料 | 要る。**開発中のモードは 5 人まで**・Premium が要る。広く出す「拡張の割り当て」は 2025-05-15 から**法人だけ**・登録した事業・公開済みのサービス・**月 25 万人以上の利用者**などが条件 | **[×課金]** 再生を含むアプリを売る・アプリ内課金を取ることを禁じる。Spotify のデータを売る・広告に渡す・機械学習に使うことを禁じる | 済 | [Quota modes](https://developer.spotify.com/documentation/web-api/concepts/quota-modes)・[Developer Policy](https://developer.spotify.com/policy) |
| 7 | OpenWeatherMap One Call API 3.0 | 天気 | 1 日の無料の回数を超えると有料(**1 回ごと**) | 鍵だけ | ODbL・出典の表示が要る | 済 | [One Call 3.0](https://openweathermap.org/api/one-call-3)・[Price](https://openweathermap.org/price) |
| 8 | Open-Meteo | 天気 | 無料の版は**非商用だけ**。商用は月の回数の定額 | 鍵(商用) | **[×課金]**(無料の版)CC BY 4.0。#348 で外した理由と同じ | 済 | [Terms](https://open-meteo.com/en/terms) |
| 9 | Google Places API (New) | 地図・場所 | 有料(**SKU ごとに 1,000 回ごと**・月の無料の分あり) | 鍵だけ | 場所の id は保存してよい・緯度経度は 30 日まで・名前や住所や口コミは保存しない・Google の表示とロゴの決まり。地図のデータを別のサービスとして売らない | 一部 | [Places API の決まり](https://developers.google.com/maps/documentation/places/web-service/policies) |
| 10 | Foursquare Places API | 地図・場所 | 有料(**1,000 回ごと**・無料の分あり・上位の項目は高い) | 鍵だけ | 表示・商用の決まりは料金のページに無い(人が規約を読む) | 一部 | [Pricing](https://foursquare.com/pricing/) |
| 11 | Yahoo! JAPAN YOLP(ローカルサーチ) | 地図・場所 | 無料(回数の上限あり) | アプリ ID だけ(利用者のデータを使う時だけ Yahoo! ID 連携) | クレジットの表示の決まり。商用は別のページ(「WebAPI の商用利用について」)を人が読む | 一部 | [ガイドライン](https://developer.yahoo.co.jp/guideline/) |
| 12 | 駅すぱあと API / NAVITIME API | 乗換 | 無料の版はあるが、商用は契約(未確認) | 鍵 | 商用の条件は各社に直接聞く | 未 | [駅すぱあと API](https://api-info.ekispert.jp/)・[NAVITIME API](https://api-sec.navitime.biz/) |
| 13 | DeepL API Pro | 翻訳 | 有料(**基本料 + 文字数**)。無料の版は月の文字数まで | 鍵だけ | Pro は送った文を保存・学習に使わない。**無料の版はデータの扱いと商用が曖昧なので Pro を使う** | 一部 | [Usage limits](https://developers.deepl.com/docs/resources/usage-limits) |
| 14 | Google Cloud Translation | 翻訳 | 有料(**文字数ごと**・月の無料の分あり) | サービスアカウント | Google Cloud の規約 | 一部 | [Pricing](https://cloud.google.com/translate/pricing) |
| 15 | NewsAPI.org | ニュース | 無料の版は**開発だけ**(本番・stg で使えない)。本番は有料の定額 | 鍵だけ | **[×課金]**(無料の版)。記事の本文は各社の著作物 | 済 | [Pricing](https://newsapi.org/pricing) |
| 16 | Google Books API | 本 | 無料 | 鍵だけ(「マイ ライブラリ」だけ OAuth) | **[×課金]**「アプリの利用に料金を取ってはいけない」(Google と別の契約が無い限り) | 済 | [Terms](https://developers.google.com/books/terms) |
| 17 | openBD | 本 | 無料 | 不要 | 本の宣伝・紹介の目的に限る・データを変えない・削除の求めに従う(今の lookupBook。既定で止めている) | 済 | [openBD](https://openbd.jp/) |
| 18 | 国立国会図書館サーチ API | 本 | 無料 | 不要 | **[×課金]** 収益のある利用は商用で、**先に申請が要る**。出典の表示・一部は CC BY | 済 | [API のヘルプ](https://ndlsearch.ndl.go.jp/help/api) |
| 19 | 楽天レシピ API(楽天ウェブサービス) | レシピ | 無料 | アプリ ID | クレジットの表示・1 秒 1 回ほど・商用可(未確認) | 未 | [楽天ウェブサービス](https://webservice.rakuten.co.jp/guide/) |
| 20 | Edamam Recipe API | レシピ | 有料(**月の定額・回数込み**・無料の版なし) | 鍵だけ | 出典の表示が全プランで必須・キャッシュはレシピの id と名前だけ | 済 | [Recipe API](https://developer.edamam.com/edamam-recipe-api) |
| 21 | Spoonacular | レシピ | 有料(**1 日の回数の定額・超えた分は追加**) | 鍵だけ | 既定でキャッシュ不可(許可で 1 時間)・保存は id と題名と画像の URL だけ・元のレシピの出典 | 済 | [Terms](https://spoonacular.com/food-api/terms) |
| 22 | Frankfurter(欧州中央銀行の為替) | 為替 | 無料・鍵なし(回数の上限あり) | 不要 | 合わせたレートは商用可。1 つの出どころのレートはその出どころの規約 | 済 | [frankfurter.dev](https://frankfurter.dev/) |
| 23 | ExchangeRate-API | 為替 | 無料と有料(どちらも商用可) | 鍵だけ | **データを再配布しない・API として出さない**(アプリの中で表示するだけ)・取引に使わない | 済 | [Terms](https://www.exchangerate-api.com/terms) |
| 24 | Google Photos Picker API | 写真 | 無料 | 要る。**2025-03-31 に Library API の読む scope が無くなり**、利用者の写真は Picker で選んだものだけ | Photos API の利用者データのポリシー | 一部 | [Updates](https://developers.google.com/photos/support/updates) |
| 25 | Unsplash API | 写真 | 無料(本番は承認の後に 1 時間 1,000 回) | 鍵だけ | 画像は返った URL を直接使う・撮影者と Unsplash の表示・ダウンロードの報告 | 済 | [API Terms](https://unsplash.com/api-terms) |
| 26 | Strava API | 運動の記録 | 無料 | 要る | **[×課金]** 寄り: データは本人にだけ見せる・**AI / 機械学習に使うことを禁じる**(アプリの中の AI と組めない)・Strava の機能を真似ない | 一部 | [API Agreement](https://www.strava.com/legal/api) |
| 27 | Google Fit / Fitbit Web API | 運動の記録 | — | — | **入れない**。Google Fit の API は 2026 年末で終わり、古い Fitbit Web API は 2026-10 末に止まる予定(後継は Google Health API) | 一部 | [Fitbit の告知](https://dev.fitbit.com/blog/)・[Fit の移行 FAQ](https://developer.android.com/health-and-fitness/health-connect/migration/fit/faq) |

### 規約で課金できない・商用が縛られるもの

| API | 扱い |
|---|---|
| Spotify | **コインを引かない・再生をつなげない**。5 人の上限と拡張の条件(法人・25 万人)があるので、つなぐ形は入れず、今の「開くだけ」を続ける |
| Google Books | 入れない(「アプリの利用に料金を取らない」に Tailor It Pro・コインが当たるおそれ)。本は openBD のまま |
| NewsAPI.org | 無料の版は本番で使えない。有料にしても記事の本文の権利があるので、最初は入れない(ニュースは #348 の RSS の許可リストで) |
| Open-Meteo | 無料の版は使えない(#348 と同じ)。天気は気象庁と OpenWeatherMap |
| 国立国会図書館サーチ | 申請が通るまで入れない |
| Strava | AI に使えない・本人にだけ見せる決まりが共有アプリと合わない。入れない |
| ExchangeRate-API・Edamam・Spoonacular | 商用は可だが、再配布・キャッシュの縛りが強い。為替は Frankfurter を先に |
| DeepL | 無料の版は使わない(Pro だけ) |

## Issue の分け方

1. 親: 外の API の土台(runner の外の API の口・appconfig の受け口・数え方・OAuth のつなぎ・止め弁・ログ)
2. 契約: `openapi-appconfig.yaml` に `/v1/apps/{appId}/ext/...`・`/v1/me/connections/...`・エラーコード。DSL スキーマに `externalCall` / `externalList`(正典 `docs/dsl/dsl.schema.json` と `backend/pkg/dsl/schema.json` をそろえる)
3. 子: provider ごと(上の上位から)。1 provider = 1 PR(定義・引数の検証・答えの形・AI の規則のファイル・iOS の行の形の確認・試験)
4. 人: Google の OAuth の確認・各社の鍵の発行・Secret Manager・プライバシーポリシー・利用規約

Issue の下書きは手元にある(公開する時に Issue にする)。

## 決めてほしいこと

| # | 決めること | 推奨 |
|---|---|---|
| 1 | 外の API は Pro だけか | Pro だけ(有料の機能の②と同じ線。無料の人は使えない) |
| 2 | 有料の API の履歴の種類 | 新しい `spend_ext`(AI の `spend_ai` と分けて、何に使ったかを履歴で分かるように) |
| 3 | 有料の API も `runs` の 1 回として数えるか | 数える(サーバーの処理は同じ。API の代金は枠に入れない) |
| 4 | runner の台数 | 最小 1 台・最大は外の API の口を入れる時に実測で決める |
| 5 | stg での試し方 | 偽の provider(固定の答え)で stg の appconfig から通す。本物の鍵は prod の runner だけ |
| 6 | 審査用の版 | OAuth の要る provider は送らない。鍵だけの読む provider は残す |
| 7 | 切った・使っていないトークン | 切ったらすぐ消す(取り消した印だけ残す)。90 日使わなければ消す |
| 8 | 最初に入れる provider | Google カレンダー・Outlook の予定・Todoist・Notion・天気(OpenWeatherMap)・為替(Frankfurter)・翻訳(DeepL Pro)・場所の検索(Google Places)の 8 つ |
| 9 | 規約で課金できない provider | コインを引かず枠だけで数える。それも許されない(再配布・商用の禁止)ものは入れない |

## 参考

- リポジトリ内: [#630 コイン課金](630-token-billing.md)・[#338 費用の作り](338-server-feature-cost-model.md)・[#339 悪用と脅威](339-abuse-threat-model.md)・[features/external-feeds.md](../backend/features/external-feeds.md)・[features/book-lookup.md](../backend/features/book-lookup.md)・[features/calls.md](../backend/features/calls.md)・[features/account-deletion.md](../backend/features/account-deletion.md)・[infra/runner.md](../infra/runner.md)・[infra/runtime-config.md](../infra/runtime-config.md)
- コード: `backend/internal/runner/quota/allowance.go`(`runs` の月の枠と超えた後のコイン払い)・`backend/internal/coins/service.go`(Reserve / Commit / Release)・`backend/internal/feeds`(出口の検査・キャッシュ・取得の予算)・`backend/internal/runner/feed.go`(saveFeed)
