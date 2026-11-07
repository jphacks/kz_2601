# 外部の公開データの読み取り(#348)

> 状態: 実装(2026-11-06)。オーナーの決定(2026-11-06): **鍵の要らない公開の API の許可リストから始める。任意の URL は許さない。**
> 対象 Issue: [#348](https://github.com/jphacks/kz_2601/issues/348)。前提: [#339 悪用と脅威のモデル](../../decisions/339-abuse-threat-model.md) の「3. 外部への送信」の条件。
> コード: `backend/pkg/dsl/feed.go`(DSL の検証)・`backend/internal/feeds`(取得・許可リスト・キャッシュ・警報 `parse_jma_warning.go`・気圧 `parse_pressure.go`)・`backend/internal/appconfig/feeds.go`(API)・`backend/internal/ai/feed_rules.go`(AI の規則)・iOS `FeedLeaf` / `FeedHost` / `HTTPFeedClient`。

## 目次

1. [できること](#できること)
2. [DSL の形](#dsl-の形)
3. [出どころと規約](#出どころと規約)
4. [安全の作り(#339 の条件との対応)](#安全の作り339-の条件との対応)
5. [API](#api)
6. [環境変数](#環境変数)
7. [審査用の版](#審査用の版)
8. [試験](#試験)
9. [まだ決めていないこと・残る危険](#まだ決めていないこと残る危険)

## できること

- 作ったアプリに、**天気予報(府県の単位)・今出ている気象の警報と注意報(府県の単位)・48 時間の気圧の予報(府県の代表地点)・これから来る祝日・運営が許可したお知らせ(RSS)** の一覧を出す。
- **読むだけ**。外へ送る・書き込む連携は無い。読んだ内容を記録に保存する・計算やグラフに使うこともできない(表示だけ)。
- 任意の URL・任意のサイト・任意の RSS は書けない。出どころは閉じた一覧(provider)と、その中の決まった値(府県予報区のコード・フィードの id)だけ。
- 警報: 解除された・出ていないものは出さない。警報が出たら知らせる(通知)・市町村の単位・地震や津波はできない。
- 気圧: 6 時間ごとの行に最低 / 最高(hPa)・1 時間ごとの値の小さな折れ線・急な低下の印(24 時間以内に 6 hPa 以上下がる)を出す。値を記録・グラフ・通知の条件に使うことはできない。

## DSL の形

```json
{"type": "feed", "provider": "weather", "region": "170000", "title": "金沢の天気", "limit": 7}
{"type": "feed", "provider": "holidays", "title": "次の祝日", "limit": 5}
{"type": "feed", "provider": "rss", "feedId": "fdma-disaster", "limit": 10}
{"type": "feed", "provider": "warnings", "region": "170000", "title": "石川県の警報・注意報", "emptyText": "今出ている警報・注意報はありません"}
{"type": "feed", "provider": "pressure", "region": "016000", "title": "札幌の気圧", "limit": 8}
```

| キー | 中身 |
|---|---|
| `provider` | 必須。`weather` / `warnings` / `pressure` / `holidays` / `rss`(スキーマの閉じた enum) |
| `region` | weather / warnings / pressure だけ・必須。気象庁の府県予報区のコード(`dsl.WeatherRegions` の 56 個。3 つで同じ一覧) |
| `feedId` | rss だけ・必須。許可したフィードの id(`dsl.RSSFeeds`) |
| `title` / `limit`(1〜20・既定 5)/ `emptyText` | 任意 |

「データの出どころ」を一覧・数字・グラフの部品の `source` として宣言する形も考えたが、最初は**専用の部品 1 つ**にした。外部のデータを記録の集計(iOS のクライアント集計)と混ぜないため、描き方を 1 か所に閉じて審査用の版から部品ごと外せるようにするため。数字・グラフに使う形は、需要を見てから足す。

## 出どころと規約

2026-11-06 に規約を確かめた(warnings・pressure は 2026-11-08)。どれも鍵は要らない。

| provider | 出どころ(サーバーが取りに行く URL) | 規約 | 出典の表示 |
|---|---|---|---|
| weather | 気象庁 `https://www.jma.go.jp/bosai/forecast/data/forecast/{府県予報区}.json` | [気象庁ホームページについて](https://www.jma.go.jp/jma/kishou/info/coment.html): 権利表記の無いものは [公共データ利用規約(第1.0版)](https://www.digital.go.jp/resources/open_data/public_data_license_v1.0)(商用利用可・出典の記載が要る・CC BY 4.0 と互換) | 気象庁ホームページ |
| warnings | 気象庁 `https://www.jma.go.jp/bosai/warning/data/r8/{府県予報区}.json`(2026 年の新しい警報の形。前の `bosai/warning/data/warning/` は 2026-05 から更新が止まっている) | weather と同じ(公共データ利用規約(第1.0版)) | 気象庁ホームページ |
| pressure | MET Norway(ノルウェー気象研究所)`https://api.met.no/weatherapi/locationforecast/2.0/compact?lat=..&lon=..`(海面気圧 `air_pressure_at_sea_level`。緯度経度は府県予報区ごとにサーバーが持つ代表地点・小数 2 桁) | [Terms of Service](https://api.met.no/doc/TermsOfService)・[License](https://api.met.no/doc/License): NLOD 2.0 と CC BY 4.0。鍵・登録は無し。名乗る User-Agent が要る(無いと 403)。`Expires` より前に同じ問い合わせを繰り返さない・1 秒 20 回まで・座標は小数 4 桁まで・アプリは直接でなく自前のサーバーを通してキャッシュするよう勧めている。商用を禁じる文は無い(読んだ範囲) | MET Norway(ノルウェー気象研究所)・CC BY 4.0・加工したこと |
| holidays | 内閣府 `https://www8.cao.go.jp/chosei/shukujitsu/syukujitsu.csv`(Shift_JIS・[案内のページ](https://www8.cao.go.jp/chosei/shukujitsu/gaiyou.html)) | [内閣府ホームページ利用規約](https://www.cao.go.jp/notice/rule.html): 権利表記の無いものは公共データ利用規約(第1.0版) | 内閣府ホームページ |
| rss `digital-news` | デジタル庁 新着・更新 `https://www.digital.go.jp/rss/news.xml`([RSS の案内](https://www.digital.go.jp/rss)) | [デジタル庁 著作権について](https://www.digital.go.jp/copyright-policy): 公共データ利用規約(第1.0版) | デジタル庁ホームページ |
| rss `fdma-news` | 消防庁 新着情報 `https://www.fdma.go.jp/index.xml`([RSS の案内](https://www.fdma.go.jp/about/rss.html)) | [消防庁 このサイトについて](https://www.fdma.go.jp/about/others/post3.html): 公共データ利用規約(第1.0版) | 消防庁ホームページ |
| rss `fdma-disaster` | 消防庁 災害情報 `https://www.fdma.go.jp/disaster/info/index.xml` | 同上 | 消防庁ホームページ |

外したもの:

| 候補 | 外した理由 |
|---|---|
| Open-Meteo(緯度経度の天気) | [利用規約](https://open-meteo.com/en/terms): 無料の API は**非商用だけ**(サブスクや広告のあるアプリは商用に当たる)。Tailor It は Pro の課金があるので使えない。商用は有料の契約(鍵が要る)。代わりに気象庁にした。気圧の予報も同じ理由で使わず、MET Norway にした(気象庁の公開の JSON に気圧の予報は無い) |
| 内閣府の報道発表の RSS(`https://www.cao.go.jp/rss/news.rdf`) | [RSS の案内](https://www.cao.go.jp/rss/index.html): フィードの内容を元にサイトを作る・再配布することを、商用・非商用を問わず禁じている |

加工の扱い: 公共データ利用規約は、編集・加工したら加工したことを書くよう求める。警報は気象庁のコードを名前に置き換え、種類と状態ごとに地域をまとめて出す(加工)。気圧は 1 時間ごとの値を 6 時間の行にまとめ、低下の印を足す(加工)ので、出典の license に「CC BY 4.0・6 時間ごとにまとめて加工」と書いた。天気は気象庁の天気のコードを「晴れ / くもり / 雨 / 雪」と絵に置き換えて出すので、出典の行は「出典: 気象庁ホームページ(公共データ利用規約(第1.0版))」とし、加工の表記の要否と書き方は**人が確かめる**(下の「まだ決めていないこと」)。

許可リストを増やす時: `backend/internal/feeds/client.go` の `DefaultEndpoints` と `backend/pkg/dsl/feed.go` の `RSSFeeds` に 1 行ずつ足す(テストが両方の一致を見る)。**規約(再配布・商用・出典)を確かめて、この表に URL を書いてから**。追加は人の判断(#339)。

ISBN から本の情報を引く `lookupBook` も同じ作り(専用のクライアント・固定の URL・上限・キャッシュ・止める)で、出どころは openBD だけ: [book-lookup.md](book-lookup.md)。

## 安全の作り(#339 の条件との対応)

| #339 の条件 | この実装 |
|---|---|
| 宛先は運営の許可リストだけ | provider は閉じた enum。取りに行く URL はサーバーのコードで固定。DSL・リクエストから受け取るのは provider と型の決まった値(6 桁の府県予報区・フィードの id・件数)だけで、どれも許可リストと照合してから使う。許可リストに無いものは、通信の前に 400 で断る |
| 仲介を通す | 端末は appconfig にだけ聞き、外部へは直接つながらない。appconfig の外への通信は `internal/feeds` の専用のクライアントだけ |
| 内部のアドレスに届かない | 専用のクライアントは、接続の直前に**実際の接続先のアドレス**を確かめ、公開のユニキャスト以外(ループバック・プライベート・リンクローカル(メタデータのサーバーを含む)・CGNAT・マルチキャスト・文書用・IPv4 を写した IPv6 など)には接続しない。名前を引き直して内部を指すようになっても同じ。環境のプロキシ設定は使わない。リダイレクトは許可リストのホストの同じ scheme の中だけ(3 回まで) |
| 送る項目を最小にする | 送るのは府県予報区のコードとフィードの URL だけ。利用者の uid・位置・記録の値は送らない。**緯度経度は受け取らない**(天気・警報・気圧は府県の単位)ので、ログにも座標は残らない。気圧の出どころには府県ごとに決まった代表地点(県庁などの所在地・`internal/feeds/pressure_points.go`)の座標を送るだけで、利用者の位置ではない |
| 秘密は DSL に置かない | 鍵の要らない出どころだけなので、秘密は無い |
| 量の上限 | 1 回 8 秒まで・応答の大きさの上限(天気 256 KiB・警報 512 KiB・気圧 256 KiB・祝日 128 KiB・RSS 512 KiB)・保存する行は 60 まで。キャッシュ(天気 30 分・警報 10 分・気圧 30 分・祝日 12 時間・RSS 15 分。気圧の問い合わせは府県の 56 地点にまとまる)で同じ問い合わせはまとめ、同時の問い合わせも 1 回の取得にまとめる。出どころへの取得はインスタンスあたり 1 分 30 回まで。利用者ごとに 1 時間 120 回まで。失敗は 1 分覚えて、すぐに取り直さない |
| 同意と開示 | 利用者の情報を外部へ送らないので、第三者への提供には当たらない(と考える)。ただしプライバシーポリシー・App Privacy の書き方は人が確かめる(下) |
| 止められる | `FEEDS_DISABLED` で provider ごとに止められる(`all` で全部)。止めた provider は 503 で、部品は「読み込めませんでした」 |
| 試験 | 許可リストの外・内部のアドレスに届かないこと・応答の上限・キャッシュを Go のテストで確かめる(下)。テストは偽の出どころ(httptest)だけを使い、実際の外部には出ない |

表示の側: 取った文字は制御文字・見えない文字を除いて長さで切る。RSS の記事のリンクは https だけ(それ以外は捨てる)。リンクは Safari で開く(アプリの中で Web は表示しない)。

## API

`GET /v1/feeds/{provider}?region=&feedId=&limit=`(認証が要る)。形は [openapi-appconfig.yaml](../openapi-appconfig.yaml) の `/feeds/{provider}`。

```json
{"provider": "weather", "title": "石川県の天気",
 "items": [{"date": "2026-11-07", "title": "雨", "subtitle": "雨 時々 くもり", "icon": "cloud.rain", "max": 13, "min": 9, "pop": 80}],
 "source": {"name": "気象庁ホームページ", "url": "https://www.jma.go.jp/", "license": "公共データ利用規約(第1.0版)"},
 "fetchedAt": "2026-11-06T03:00:00Z"}
```

出どころが落ちている時は、24 時間以内に取れた最後の結果を `stale: true` で返す(部品は「前に取った内容です」と添える)。

## 環境変数

| 変数 | 何をするか | 既定 |
|---|---|---|
| `FEEDS_DISABLED` | 止める provider(カンマ区切り。`all` = 全部) | 無し(止めない) |
| `FEEDS_USER_PER_HOUR` | 1 人 1 時間の `GET /v1/feeds` の回数(`0` で数えない) | 120 |
| `FEEDS_UPSTREAM_PER_MINUTE` | インスタンスあたり 1 分の出どころへの取得 | 30 |

## 審査用の版

外部の Web の内容(他のサイトのお知らせ)を出すので、**審査用の版(`APP_STORE_REVIEW`)では部品ごと外す**。既存の外し方(`webView` と同じ)にそろえた: `FeedLeaf` / `FeedHost` / `HTTPFeedClient` / `FeedCache` / `AppRuntime+Feeds` を `#if !APP_STORE_REVIEW` で囲み、Renderer に登録しない。`ClientCapabilities.reviewExcludedComponents` に `feed` を入れたので、AI は審査用の版に feed を作らない(`restrict.go` の仕組み・feed の規則の行も落ちる)。古い iOS も `allowedComponents` に feed を送らないので作られない。

戻す時に見ること(審査のガイドライン):
- **2.5.2**: 取ってくるのはデータ(JSON・CSV・RSS)で、コードは落とさない・実行しない。
- **4.7**: アプリの中の Web の内容・ミニアプリの扱い。記事はアプリの中で表示せず Safari で開く。
- **1.2(利用者の作るコンテンツ)**: 出どころは運営が選んだ公的機関だけで、利用者が出どころを足せない。
- **5.1.1 / 5.1.2**: 利用者の情報を外部へ送らない。App Privacy の表示は変わらない(と考える)。

## 試験

- `backend/internal/feeds`: provider ごとの URL の組み立て・解析(気象庁の JSON・Shift_JIS の CSV・RSS 2.0 / RSS 1.0 / Atom・気象庁の警報(r8)と MET Norway の予報は 2026-11-08 に実際に取った応答を `testdata/` に置いて使う)・許可リストの外を通信の前に断る・本番のクライアントがループバックに接続しない・公開アドレスの判定・別のホストへのリダイレクトを断る・応答の大きさの上限・キャッシュ・同時の問い合わせを 1 回にまとめる・失敗時の stale・回数の上限・止めるスイッチ。
- `backend/internal/appconfig/feeds_test.go`: パラメータの検証(未知・重複・範囲)・認証・利用者ごとの上限・止めるスイッチ・キャッシュ。
- `backend/pkg/dsl/feed_test.go`: スキーマ(閉じた provider・url を書けない)と `Validate()`。
- AI: `backend/internal/ai/feed_rules_test.go`・ai-eval `feed_cases.json` / `feed_chat_cases.json`(`-check` だけ確かめた。実際に Claude で回すのは別)。
- AI: warnings・pressure の規則は `feed_warnings_pressure_rules.go`(`TestFeedWarningsPressureRules`)・ai-eval `feed_cases.json` に 2 件・`jphacks_coverage_cases.json` の hs_2503・NG_2301。
- iOS: `FeedTests`(DSL から問い合わせ)・`FeedLeafTests`(表示の組み立て)・`FeedClientTests`(appconfig だけに聞く・キャッシュ)。

## まだ決めていないこと・残る危険

- **課金**: 外部の読み取りの原価は小さい(キャッシュで出どころへの取得は少ない・Cloud Run の通信だけ)。今は無料・回数の上限だけにした。Pro の機能に入れるかは決めていない。
- **気象庁の JSON は公式の API として案内されていない**(ホームページが使うデータ)。形が予告なく変わることがある。変わったら取得は失敗し、部品は「読み込めませんでした」(stale があればそれ)になる。実際の形は stg で確かめる。
- **加工の表記**: 天気のコードを言葉と絵に置き換えている。公共データ利用規約の「加工したことの記載」をどう書くかは人が確かめる。
- **プライバシーポリシー・規約への追記の要否**: 利用者の情報は送らないが、「外部の公開データを表示する」「出典」をポリシーや規約に書くかは人の判断(`docs/legal/*` は変えていない)。
- **Cloud Run の外への通信**: infra・デプロイの workflow に VPC の egress の設定は無いので、appconfig から外部の HTTPS に出られる見込み。stg で 1 回確かめる。
- **警報の表示と気象業務法**: 気象庁の警報を出典つきでそのまま見せるだけで、独自の警報は出さない(気象業務法 23 条は気象庁以外の警報を禁じる)。解除の遅れ(キャッシュ 10 分・端末 5 分)があるので、命に関わる判断に使う表示として足りるか、注意の文を部品に出すかは人が確かめる。
- **警報の JSON(r8)も公式の API ではない**。コードと名前の対応は気象庁の警報のページのものを写した(`warningKinds`)。知らないコードは出さない。氾濫(別のファイル)は読んでいない。
- **MET Norway の規約**: 商用の可否は読んだページに書かれていない(禁じる文は無い)。出典の書き方(「MET Norway」・CC BY 4.0 の表示と加工の表記)と、`Expires` を守る作り(今はキャッシュ 30 分で、`If-Modified-Since` は送っていない)で足りるかは人が確かめる。User-Agent は既存の `TailorIt-feeds/1 (+https://github.com/jphacks/kz_2601)`。
- **気圧は海面気圧**: 天気図と同じ海面に直した値で、その場の気圧(高い土地では低い)とは違う。変化(下がり方)を見る用途には差が無い。
- warnings・pressure の実際の取得は stg で 1 回確かめる。
- 府県予報区のコードの一覧は、気象庁の公開の情報と開発者の資料から作った。十勝・奄美は別のファイルが無い(釧路・根室、鹿児島のファイルに入る)ので一覧に入れていない。
