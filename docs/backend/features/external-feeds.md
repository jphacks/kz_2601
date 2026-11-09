# 外部の公開データの読み取り(#348)

> 状態: 実装(2026-11-06)。オーナーの決定(2026-11-06): **鍵の要らない公開の API の許可リストから始める。任意の URL は許さない。**
> 対象 Issue: [#348](https://github.com/jphacks/kz_2601/issues/348)。前提: [#339 悪用と脅威のモデル](../../decisions/339-abuse-threat-model.md) の「3. 外部への送信」の条件。
> コード: `backend/pkg/dsl/feed.go`・`feed_arxiv_diet.go`(DSL の検証)・`backend/internal/feeds`(取得・許可リスト・キャッシュ・警報 `parse_jma_warning.go`・気圧 `parse_pressure.go`・arXiv と国会会議録 `parse_arxiv_diet.go`)・`backend/internal/appconfig/feeds.go`(API)・`backend/internal/ai/feed_rules.go` ほか `feed_*_rules.go`(AI の規則)・iOS `FeedLeaf` / `FeedHost` / `HTTPFeedClient`。

## 目次

1. [できること](#できること)
2. [DSL の形](#dsl-の形)
3. [出どころと規約](#出どころと規約)
4. [安全の作り(#339 の条件との対応)](#安全の作り339-の条件との対応)
5. [API](#api)
6. [環境変数](#環境変数)
7. [審査用の版](#審査用の版)
8. [試験](#試験)
9. [記録に残す(サーバーの自動の動き saveFeed)](#記録に残すサーバーの自動の動き-savefeed)
10. [まだ決めていないこと・残る危険](#まだ決めていないこと残る危険)

## できること

- 作ったアプリに、**天気予報(府県の単位)・今出ている気象の警報と注意報(府県の単位)・48 時間の気圧の予報(府県の代表地点)・これから来る祝日・運営が許可したお知らせ(RSS)・arXiv の分野ごとの新着論文・国会会議録の発言の検索** の一覧を出す。
- **読むだけ**。外へ送る・書き込む連携は無い。部品(feed)が読んだ内容は記録に保存できない(表示だけ)。天気・気圧・警報を毎日の記録として残すのは、サーバーの自動の動きの `saveFeed`(下)だけ。
- 任意の URL・任意のサイト・任意の RSS は書けない。出どころは閉じた一覧(provider)と、その中の決まった値(府県予報区のコード・フィードの id)だけ。
- 警報: 解除された・出ていないものは出さない。警報が出たら知らせる(通知)・市町村の単位・地震や津波はできない。
- 気圧: 6 時間ごとの行に最低 / 最高(hPa)・1 時間ごとの値の小さな折れ線・急な低下の印(24 時間以内に 6 hPa 以上下がる)を出す。値を記録・グラフ・通知の条件に使うことはできない。
- arXiv(`arxiv`): 許可した分野(`dsl.ArxivCategories` の 9 つ)の新着を投稿日の新しい順に。行は題名・著者(3 人まで、それ以上は「ほか」)・投稿日。タップで論文のページ(arxiv.org/abs/…)を Safari で開く。要約(abstract)は出さない。キーワード検索・分野の組み合わせはできない。
- 国会会議録(`diet`): アプリに決めて書いた語(`query`・40 字まで)で発言を探し、新しい会議から。行は発言者(会派)・院と会議名と号・会議の日付。タップで会議録の発言のページを開く。発言の本文は出さない。利用者がその場で語を打って検索する形は無い(DSL に書いた語だけ)。
- NHK のニュースは**入れていない**(下の「外したもの」: RSS の規約が個人の利用だけで、プログラムでの再提供を許していない)。

## DSL の形

```json
{"type": "feed", "provider": "weather", "region": "170000", "title": "金沢の天気", "limit": 7}
{"type": "feed", "provider": "holidays", "title": "次の祝日", "limit": 5}
{"type": "feed", "provider": "rss", "feedId": "fdma-disaster", "limit": 10}
{"type": "feed", "provider": "warnings", "region": "170000", "title": "石川県の警報・注意報", "emptyText": "今出ている警報・注意報はありません"}
{"type": "feed", "provider": "pressure", "region": "016000", "title": "札幌の気圧", "limit": 8}
{"type": "feed", "provider": "arxiv", "feedId": "cs.CL", "title": "自然言語処理の新着論文", "limit": 10}
{"type": "feed", "provider": "diet", "query": "こども家庭庁", "title": "国会で「こども家庭庁」", "limit": 10}
```

| キー | 中身 |
|---|---|
| `provider` | 必須。`weather` / `warnings` / `pressure` / `holidays` / `rss` / `arxiv` / `diet`(スキーマの閉じた enum) |
| `region` | weather / warnings / pressure だけ・必須。気象庁の府県予報区のコード(`dsl.WeatherRegions` の 56 個。3 つで同じ一覧) |
| `feedId` | rss・arxiv だけ・必須。rss は許可したフィードの id(`dsl.RSSFeeds`)、arxiv は分野(`dsl.ArxivCategories`: cs.AI / cs.CL / cs.CR / cs.CV / cs.HC / cs.LG / cs.RO / cs.SE / stat.ML) |
| `query` | diet だけ・必須。国会会議録で探す語。空白(全角も)をまとめて 1〜40 字・改行やタブ・制御文字・見えない文字(Cf)は不可。空白区切りは AND。API には `q` で渡す |
| `title` / `limit`(1〜20・既定 5)/ `emptyText` | 任意 |

「データの出どころ」を一覧・数字・グラフの部品の `source` として宣言する形も考えたが、最初は**専用の部品 1 つ**にした。外部のデータを記録の集計(iOS のクライアント集計)と混ぜないため、描き方を 1 か所に閉じて審査用の版から部品ごと外せるようにするため。数字・グラフに使う形は、需要を見てから足す。

## 出どころと規約

2026-11-06 に規約を確かめた(warnings・pressure・arxiv・diet・NHK は 2026-11-08)。どれも鍵は要らない。

| provider | 出どころ(サーバーが取りに行く URL) | 規約 | 出典の表示 |
|---|---|---|---|
| weather | 気象庁 `https://www.jma.go.jp/bosai/forecast/data/forecast/{府県予報区}.json` | [気象庁ホームページについて](https://www.jma.go.jp/jma/kishou/info/coment.html): 権利表記の無いものは [公共データ利用規約(第1.0版)](https://www.digital.go.jp/resources/open_data/public_data_license_v1.0)(商用利用可・出典の記載が要る・CC BY 4.0 と互換) | 気象庁ホームページ |
| warnings | 気象庁 `https://www.jma.go.jp/bosai/warning/data/r8/{府県予報区}.json`(2026 年の新しい警報の形。前の `bosai/warning/data/warning/` は 2026-05 から更新が止まっている) | weather と同じ(公共データ利用規約(第1.0版)) | 気象庁ホームページ |
| pressure | MET Norway(ノルウェー気象研究所)`https://api.met.no/weatherapi/locationforecast/2.0/compact?lat=..&lon=..`(海面気圧 `air_pressure_at_sea_level`。緯度経度は府県予報区ごとにサーバーが持つ代表地点・小数 2 桁) | [Terms of Service](https://api.met.no/doc/TermsOfService)・[License](https://api.met.no/doc/License): NLOD 2.0 と CC BY 4.0。鍵・登録は無し。名乗る User-Agent が要る(無いと 403)。`Expires` より前に同じ問い合わせを繰り返さない・1 秒 20 回まで・座標は小数 4 桁まで・アプリは直接でなく自前のサーバーを通してキャッシュするよう勧めている。商用を禁じる文は無い(読んだ範囲) | MET Norway(ノルウェー気象研究所)・CC BY 4.0・加工したこと |
| holidays | 内閣府 `https://www8.cao.go.jp/chosei/shukujitsu/syukujitsu.csv`(Shift_JIS・[案内のページ](https://www8.cao.go.jp/chosei/shukujitsu/gaiyou.html)) | [内閣府ホームページ利用規約](https://www.cao.go.jp/notice/rule.html): 権利表記の無いものは公共データ利用規約(第1.0版) | 内閣府ホームページ |
| rss `digital-news` | デジタル庁 新着・更新 `https://www.digital.go.jp/rss/news.xml`([RSS の案内](https://www.digital.go.jp/rss)) | [デジタル庁 著作権について](https://www.digital.go.jp/copyright-policy): 公共データ利用規約(第1.0版) | デジタル庁ホームページ |
| rss `fdma-news` | 消防庁 新着情報 `https://www.fdma.go.jp/index.xml`([RSS の案内](https://www.fdma.go.jp/about/rss.html)) | [消防庁 このサイトについて](https://www.fdma.go.jp/about/others/post3.html): 公共データ利用規約(第1.0版) | 消防庁ホームページ |
| rss `fdma-disaster` | 消防庁 災害情報 `https://www.fdma.go.jp/disaster/info/index.xml` | 同上 | 消防庁ホームページ |
| arxiv `{分野}` | arXiv API `https://export.arxiv.org/api/query?search_query=cat:{分野}&sortBy=submittedDate&sortOrder=descending&max_results=20`(Atom)。[RSS](https://info.arxiv.org/help/rss.html)(`rss.arxiv.org/rss/{分野}`)は**土日に空になる**(毎日 1 回の告知分だけ)ので、いつでも新着が出る API にした。規約は同じ | [Terms of Use for arXiv APIs](https://info.arxiv.org/help/api/tou.html)(RSS も対象と明記): 論文の**メタデータ**(題名・著者など)は CC0 1.0 で、取得・保存・加工・共有してよい。PDF などの本文は arXiv が権利者でないので、サーバーに置いて配らない(取らない・リンクだけ)。**3 秒に 1 回まで・同時に 1 本まで**(複数の機械でも合計)。arXiv が承認・提携しているように見せない。決まった謝辞の文は求めていない。商用の扱いは書かれていない | arXiv.org・「論文の情報は CC0 1.0・arXiv の承認や提携はありません」。出典の URL はその分野の一覧(`arxiv.org/list/{分野}/recent`) |
| diet | 国立国会図書館 国会会議録検索システムの検索用 API `https://kokkai.ndl.go.jp/api/speech?any={語}&maximumRecords=20&recordPacking=json`([API の仕様](https://kokkai.ndl.go.jp/api.html)) | API の仕様のページ: 手続きは要らない。**短い時間の大量のアクセスを控え、自動の取得は並列にせず数秒あけて次を送る**。運用に障ると判断したら予告なく止められる。利用は「[国立国会図書館ウェブサイトのコンテンツ利用規約](https://www.ndl.go.jp/sitepolicy/terms)」(公共データ利用規約(第1.0版)。第三者に権利があるもの・別の規則があるものは除く)に従う。**発言の著作権は各発言者**(国会図書館の職員の発言とデータベースは国会図書館)で、転載は原則として権利者の許諾が要る(引用などの例外を除く)。商用・出典の書き方・キャッシュは書かれていない | 国立国会図書館 国会会議録検索システム・「検索用 API・発言の著作権は各発言者」 |

外したもの:

| 候補 | 外した理由 |
|---|---|
| Open-Meteo(緯度経度の天気) | [利用規約](https://open-meteo.com/en/terms): 無料の API は**非商用だけ**(サブスクや広告のあるアプリは商用に当たる)。Tailor It は Pro の課金があるので使えない。商用は有料の契約(鍵が要る)。代わりに気象庁にした。気圧の予報も同じ理由で使わず、MET Norway にした(気象庁の公開の JSON に気圧の予報は無い) |
| 内閣府の報道発表の RSS(`https://www.cao.go.jp/rss/news.rdf`) | [RSS の案内](https://www.cao.go.jp/rss/index.html): フィードの内容を元にサイトを作る・再配布することを、商用・非商用を問わず禁じている |
| NHK ニュースの RSS(`https://news.web.nhk/n-data/conf/na/rss/cat0.xml` 〜 `cat7.xml`: 主要・社会・文化・エンタメ・科学・医療・政治・経済・国際・スポーツ。旧 `www3.nhk.or.jp/rss/news/cat*.xml` は 2025-10 から転送) | [NHK ONE ニュース RSS](https://www.nhk.or.jp/toppage/rss/index.html)(2026-11-08 に読んだ): 「提供するRSSは個人の方の利用のためのみに使用することができます。ブログやプログラム等によって、商業目的での利用を含め再配信や再提供を許可するものではありません」。サーバーが取って利用者のアプリに出すのは「プログラムによる再提供」に当たるので入れない。使うなら NHK の許諾が要る(人の判断) |

arxiv・diet の表示: どちらも**一覧(メタデータ)とリンクだけ**を出し、本文(論文の PDF・要約、発言の本文)は出さない・保存しない。国会会議録の API は本文ごと返すが、サーバーは解析で捨てる(キャッシュにも残らない)。発言の一覧(発言者・会議名・日付)の表示と、本文の扱い(著作権法 40 条 1 項は、公開の政治上の演説・陳述を、同じ人のものを編集する場合を除いて利用できるとする)で足りるかは**人が確かめる**。

加工の扱い: 公共データ利用規約は、編集・加工したら加工したことを書くよう求める。警報は気象庁のコードを名前に置き換え、種類と状態ごとに地域をまとめて出す(加工)。気圧は 1 時間ごとの値を 6 時間の行にまとめ、低下の印を足す(加工)ので、出典の license に「CC BY 4.0・6 時間ごとにまとめて加工」と書いた。天気は気象庁の天気のコードを「晴れ / くもり / 雨 / 雪」と絵に置き換えて出すので、出典の行は「出典: 気象庁ホームページ(公共データ利用規約(第1.0版))」とし、加工の表記の要否と書き方は**人が確かめる**(下の「まだ決めていないこと」)。

許可リストを増やす時: `backend/internal/feeds/client.go` の `DefaultEndpoints` と `backend/pkg/dsl/feed.go` の `RSSFeeds` に 1 行ずつ足す(テストが両方の一致を見る)。arXiv の分野は `backend/pkg/dsl/feed_arxiv_diet.go` の `ArxivCategories`(スキーマの `feedId` の説明と AI の規則 `feed_arxiv_diet_rules.go` にも書く。テストが規則との一致を見る)。**規約(再配布・商用・出典)を確かめて、この表に URL を書いてから**。表に無い URL・https でない URL・「外したもの」の URL がコードの許可リストにあると、`TestAllowlistMatchesDoc`(`internal/feeds/allowlist_doc_test.go`)が落ちる。追加は人の判断(#339)。

ISBN から本の情報を引く `lookupBook` も同じ作り(専用のクライアント・固定の URL・上限・キャッシュ・止める)で、出どころは openBD だけ: [book-lookup.md](book-lookup.md)。

## 安全の作り(#339 の条件との対応)

| #339 の条件 | この実装 |
|---|---|
| 宛先は運営の許可リストだけ | provider は閉じた enum。取りに行く URL はサーバーのコードで固定。DSL・リクエストから受け取るのは provider と型の決まった値(6 桁の府県予報区・フィードの id・件数)だけで、どれも許可リストと照合してから使う。許可リストに無いものは、通信の前に 400 で断る |
| 仲介を通す | 端末は appconfig にだけ聞き、外部へは直接つながらない。appconfig の外への通信は `internal/feeds` の専用のクライアントだけ |
| 内部のアドレスに届かない | 専用のクライアントは、接続の直前に**実際の接続先のアドレス**を確かめ、公開のユニキャスト以外(ループバック・プライベート・リンクローカル(メタデータのサーバーを含む)・CGNAT・マルチキャスト・文書用・IPv4 を写した IPv6 など)には接続しない。名前を引き直して内部を指すようになっても同じ。環境のプロキシ設定は使わない。リダイレクトは許可リストのホストの同じ scheme の中だけ(3 回まで) |
| 送る項目を最小にする | 送るのは府県予報区のコード・フィードの URL・arXiv の分野・国会会議録で探す語(diet の `query`)だけ。diet の語は**アプリを作った時に DSL に書いた決まった語**で、利用者がその場で打った文字や記録の値ではない(DSL から値を差し込む形は無い)。ただし API は認証した利用者なら任意の語(40 字まで)を送れるので、語は国会図書館に届く。アプリのログには語を残さない(失敗のログは文字数だけ・アクセスのログはパスだけ)。ただし Cloud Run の要求ログは URL を問い合わせごと残すので、語はそこに残る。利用者の uid・位置・記録の値は送らない。**緯度経度は受け取らない**(天気・警報・気圧は府県の単位)ので、ログにも座標は残らない。気圧の出どころには府県ごとに決まった代表地点(県庁などの所在地・`internal/feeds/pressure_points.go`)の座標を送るだけで、利用者の位置ではない |
| 秘密は DSL に置かない | 鍵の要らない出どころだけなので、秘密は無い |
| 量の上限 | 1 回 8 秒まで・応答の大きさの上限(天気 256 KiB・警報 512 KiB・気圧 256 KiB・祝日 128 KiB・RSS 512 KiB・arxiv 1 MiB・diet 3 MiB(発言の本文ごと返るため。20 件の実測は 84 KB))・保存する行は 60 まで・キャッシュは 2000 件まで(diet の語で増えないよう、古いものから捨てる)。arxiv と diet は出どころの求めに合わせ、インスタンスあたり**同時に 1 本**で、前の取得から間をあけて取りに行く(diet は 3 秒。arxiv は 3 秒 × appconfig の最大インスタンス数(`deploy-cloudrun.yml` の `--max-instances=3`)= 9 秒にして、全インスタンスが同時に取っても合計で 3 秒に 1 回を超えないようにした。最大インスタンス数を変える時は `backend/internal/feeds/feeds.go` の `appconfigMaxInstances` も変える)。順番待ちは来た順で 10 秒まで。超えたら 503。取得は待っていた要求の期限と切り離して 8 秒まで続け、結果はキャッシュに入る(要求の側の打ち切りは失敗として覚えない)。キャッシュ(天気 30 分・警報 10 分・気圧 30 分・祝日 12 時間・RSS 15 分・arxiv 3 時間・diet 3 時間。気圧の問い合わせは府県の 56 地点にまとまる)で同じ問い合わせはまとめ、同時の問い合わせも 1 回の取得にまとめる。出どころへの取得はインスタンスあたり 1 分 30 回まで(diet は別枠で 1 分 10 回。検索語はキャッシュに当たりにくいので、ほかの部品の枠を使わない)。利用者ごとに 1 時間 120 回まで。検索語のある問い合わせ(diet)で出どころまで取りに行くのは、利用者ごと・インスタンスごとに 1 時間 20 回まで(キャッシュにある語はこの数に入らない。超えたら 429)。失敗は 1 分覚えて、すぐに取り直さない |
| 同意と開示 | 利用者の情報を外部へ送らないので、第三者への提供には当たらない(と考える)。ただしプライバシーポリシー・App Privacy の書き方は人が確かめる(下) |
| 止められる | `FEEDS_DISABLED` で provider ごとに止められる(`all` で全部。今回足した出どころだけ止めるなら `FEEDS_DISABLED=arxiv,diet`)。止めた provider は 503 で、部品は「読み込めませんでした」 |
| 試験 | 許可リストの外・内部のアドレスに届かないこと・応答の上限・キャッシュを Go のテストで確かめる(下)。テストは偽の出どころ(httptest)だけを使い、実際の外部には出ない |

表示の側: 取った文字は制御文字・見えない文字を除いて長さで切る。RSS の記事のリンクは https だけ(それ以外は捨てる)。リンクは Safari で開く(アプリの中で Web は表示しない)。

## API

`GET /v1/feeds/{provider}?region=&feedId=&q=&limit=`(認証が要る。`q` は diet だけ)。形は [openapi-appconfig.yaml](../openapi-appconfig.yaml) の `/feeds/{provider}`。

```json
{"provider": "weather", "title": "石川県の天気",
 "items": [{"date": "2026-11-07", "title": "雨", "subtitle": "雨 時々 くもり", "icon": "cloud.rain", "max": 13, "min": 9, "pop": 80}],
 "source": {"name": "気象庁ホームページ", "url": "https://www.jma.go.jp/", "license": "公共データ利用規約(第1.0版)"},
 "fetchedAt": "2026-11-06T03:00:00Z"}
```

出どころが落ちている時は、24 時間以内に取れた最後の結果を `stale: true` で返す(部品は「前に取った内容です」と添える)。

arxiv と diet(同時に 1 本の出どころ)は、キャッシュの期限が切れても 24 時間以内の結果があれば順番待ちをせずに `stale: true` ですぐ返す。その時、その出どころの取得が空いていれば、取り直しを裏で 1 本だけ始める(同じ問い合わせの取り直しは 1 本にまとめる。空いていない時は次の要求が始める)。次の要求からは取り直した結果を返す。

diet の検索語で出どころまで取りに行く回数は、利用者ごとに加えて IP ごと(ハッシュしたものをインスタンスのメモリで数える)にも 1 時間 60 回までにする(匿名のアカウントを作り直しても同じ IP で数える)。数えるのは取得を始める時に、確かめるのと同じ 1 回のロックの中で行う(同時に並んだ要求がまとめて上限を越えない)。

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
- arxiv・diet: `backend/internal/feeds/arxiv_diet_test.go`(2026-11-08 に 1 回だけ実際に取った応答を 4 件に切って `testdata/arxiv_api_cs_AI.xml`・`testdata/kokkai_speech_kodomo.json` に置いた。解析・送る問い合わせ・通信の前に断る値・3 秒あける・1 つずつ止める・キャッシュの上限)・`backend/pkg/dsl/feed_arxiv_diet_test.go`・`backend/internal/appconfig/feeds_arxiv_diet_test.go`(`q` の検証)・AI の規則 `feed_arxiv_diet_rules.go`(`TestFeedArxivDietRules`)・ai-eval `feed_arxiv_diet_cases.json`(4 件・NHK は作らない)・iOS `FeedTests` / `FeedLeafTests`。
- iOS: `FeedTests`(DSL から問い合わせ)・`FeedLeafTests`(表示の組み立て)・`FeedClientTests`(appconfig だけに聞く・キャッシュ)。

## 記録に残す(サーバーの自動の動き saveFeed)

予報は「今から先」しか見られないので、体調と気圧の関係のように後で比べたいアプリ(jphacks/tk_a_2518)のために、時刻のきっかけのサーバーのルール(`serverRules`・Pro)がその時の値を記録 1 件として残せる。

```json
{"id": "daily_feed", "on": {"every": "daily", "time": "07:00"},
 "do": [{"saveFeed": {"provider": "pressure", "region": "130000", "entity": "pressure_log",
   "fields": {"date": "day", "summary": "note", "min": "low", "max": "high", "drop": "falling"}}}]}
```

| provider | 残す値 |
|---|---|
| `weather` | summary = 今日の天気の言葉・min / max = 今日の最低 / 最高気温(°C。気象庁の発表に無い時は書かない) |
| `pressure` | これから 24 時間(6 時間の行 4 つ): min / max(hPa)・drop = 6 hPa 以上の急な低下・summary = 低下の文か「1003〜1012 hPa」 |
| `warnings` | summary = 今出ている警報・注意報の名前(「大雨警報・雷注意報」・無ければ「発表なし」) |

- 語彙は閉じている: provider は上の 3 つ・region は feed と同じ府県予報区のコード(DSL に書いた決まった値。記録の値は差し込めない)。each の無い時刻のきっかけだけ(1 回の実行で 1 回取る)・アプリに 3 つまで。
- 書く先は `createRecord` と同じ: ルールと同じ側の entity(private は作成者の記録・shared / public はアプリの記録)・single と宛先付きは不可・連鎖の検出に入る。fields は残す値 → field の id(date は date / datetime・summary は string / text・min / max は number / integer・drop は boolean)。required の field は値が必ずある date か summary だけ。
- 取りに行くのは runner(`backend/internal/runner/feed.go`)で、appconfig と同じ `internal/feeds` の `Service` を使う(許可リスト・接続先の確認・応答の上限・キャッシュ・1 分あたりの取得・`FEEDS_DISABLED`)。止めた provider の回は何も書かずに ok。出どころが落ちて古い写ししか無い時は保存せず、その回をやり直す(やり直しの上限・遅れの上限は他のルールと同じ)。
- 書き込みは他のルールと同じ実行・書き込みの上限・止めるスイッチ・Pro の判定を通る。
- AI: `server_rules_vocab_savefeed.go`(自動の動きの工程の語彙)・`weather_log_rules.go`(生成の本体に「毎日の気圧・天気を残すなら記録の entity と list / chart を作る」の 1 行。feed の部品が使えない版でも残る)。ai-eval `server_rules_cases.json` の `sr_save_pressure`。
- 試験: `pkg/dsl/server_rules_feed_test.go`・`internal/runner/feed_test.go` / `feed_snapshot_test.go`・`internal/ai/server_rules_vocab_savefeed_test.go`。
- runner の Cloud Run から外部の HTTPS に出られるかは、runner をデプロイする時に確かめる(まだデプロイしていない)。

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
- **arxiv・diet・(外した)NHK の追加は人の判断(#339)**。arxiv と diet は既定で有効で、`FEEDS_DISABLED=arxiv,diet` で 1 つずつ止められる。マージの前に、下の点を人が確かめる:
  - arXiv: 商用の扱いが規約に無い。「arXiv の承認や提携はない」の書き方。間隔はインスタンスごとなので、3 秒 × 最大インスタンス数(今は 9 秒)にして合計が 3 秒に 1 回を超えないようにした(キャッシュ 3 時間・9 分野なので実際はもっと少ない)。
  - 国会会議録: 発言の著作権は各発言者にある。発言者・会議名・日付の一覧とリンクだけを出す今の形で足りるか。出典の書き方(国立国会図書館ウェブサイトのコンテンツ利用規約は公共データ利用規約(第1.0版)を指すが、会議録の発言は「第三者が著作権を有するコンテンツ」に当たる)。商用・キャッシュは API の仕様のページに書かれていない。
  - NHK: 入れていない。os_2401・sp_2512 のようなニュースのアプリを作るなら、NHK の許諾を取るか、再提供を許す別の出どころを探す。
- 府県予報区のコードの一覧は、気象庁の公開の情報と開発者の資料から作った。十勝・奄美は別のファイルが無い(釧路・根室、鹿児島のファイルに入る)ので一覧に入れていない。
