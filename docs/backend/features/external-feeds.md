# 外部の公開データの読み取り(#348)

> 状態: 実装(2026-11-06)。オーナーの決定(2026-11-06): **鍵の要らない公開の API の許可リストから始める。任意の URL は許さない。**
> 対象 Issue: [#348](https://github.com/jphacks/kz_2601/issues/348)。前提: [#339 悪用と脅威のモデル](../../decisions/339-abuse-threat-model.md) の「3. 外部への送信」の条件。
> コード: `backend/pkg/dsl/feed.go`・`feed_arxiv_diet.go`・`feed_bikeshare.go`(DSL の検証)・`backend/internal/feeds`(取得・許可リスト・キャッシュ・警報 `parse_jma_warning.go`・気圧 `parse_pressure.go`・arXiv と国会会議録 `parse_arxiv_diet.go`・シェアサイクル `parse_gbfs.go`)・`backend/internal/appconfig/feeds.go`(API)・`backend/internal/ai/feed_rules.go` ほか `feed_*_rules.go`(AI の規則)・iOS `FeedLeaf` / `FeedHost` / `HTTPFeedClient`。

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
- 使っているサービスの稼働状況(`status`・#1675): 許可したサービス(`dsl.StatusPages` の GitHub・Cloudflare・Vercel・Claude(Anthropic))の公開の status ページから、全体の状態・対応中の障害・実施中のメンテナンス・止まっている部分(コンポーネント)・予定のメンテナンスを出す。障害の説明の本文(更新の文)は出さない。タップでその障害のページ(status ページ)を開く。OpenAI・Slack は入れていない(下の「外したもの」)。**オーナーの規約の確認と正典のスキーマの変更の OK を待っている**(#339。develop へ入れる前に)。AI の規則(`feed_rules.go` など)は親の決定を待つので、まだ AI は `status` を作らない。iOS / Android は `status` をまだ知らない(プレースホルダ)ので、描くのは別の作業。
- NHK のニュースは**入れていない**(下の「外したもの」: RSS の規約が個人の利用だけで、プログラムでの再提供を許していない)。
- シェアサイクル(`bikeshare`・#1694): アプリに決めて書いた地点(`query` に `"緯度,経度"`。例: いつもの駅・家の近く)から **2 km 以内の近いポート** を近い順に。行はポートの名前・「自転車 3 台・返せる空き 5・約 250 m」・事業者の最終更新の時刻。貸出を止めているポートは「貸出停止中」。事業者は許可した GBFS(`dsl.BikeshareSystems`: ドコモ・バイクシェア(東京)/ ドコモ・バイクシェア(東京以外)/ HELLO CYCLING)。今いる場所(端末の位置)で探す形・予約・ポートを名前で選ぶ形は無い。

## DSL の形

```json
{"type": "feed", "provider": "weather", "region": "170000", "title": "金沢の天気", "limit": 7}
{"type": "feed", "provider": "holidays", "title": "次の祝日", "limit": 5}
{"type": "feed", "provider": "rss", "feedId": "fdma-disaster", "limit": 10}
{"type": "feed", "provider": "warnings", "region": "170000", "title": "石川県の警報・注意報", "emptyText": "今出ている警報・注意報はありません"}
{"type": "feed", "provider": "pressure", "region": "016000", "title": "札幌の気圧", "limit": 8}
{"type": "feed", "provider": "arxiv", "feedId": "cs.CL", "title": "自然言語処理の新着論文", "limit": 10}
{"type": "feed", "provider": "diet", "query": "こども家庭庁", "title": "国会で「こども家庭庁」", "limit": 10}
{"type": "feed", "provider": "bikeshare", "feedId": "docomo-cycle-tokyo", "query": "35.681,139.767", "title": "東京駅の近くのポート", "limit": 5}
{"type": "feed", "provider": "status", "feedId": "github", "title": "GitHub の稼働状況", "limit": 10}
```

| キー | 中身 |
|---|---|
| `provider` | 必須。`weather` / `warnings` / `pressure` / `holidays` / `rss` / `arxiv` / `diet` / `bikeshare` / `status`(スキーマの閉じた enum) |
| `region` | weather / warnings / pressure だけ・必須。気象庁の府県予報区のコード(`dsl.WeatherRegions` の 56 個。3 つで同じ一覧) |
| `feedId` | rss・arxiv・bikeshare・status だけ・必須。rss は許可したフィードの id(`dsl.RSSFeeds`)、arxiv は分野(`dsl.ArxivCategories`: cs.AI / cs.CL / cs.CR / cs.CV / cs.HC / cs.LG / cs.RO / cs.SE / stat.ML)、bikeshare は事業者(`dsl.BikeshareSystems`: docomo-cycle-tokyo / docomo-cycle / hellocycling)、status はサービス(`dsl.StatusPages`: claude / cloudflare / github / vercel) |
| `query` | diet だけ・必須。国会会議録で探す語。空白(全角も)をまとめて 1〜40 字・改行やタブ・制御文字・見えない文字(Cf)は不可。空白区切りは AND。API には `q` で渡す |
| bikeshare の `feedId` / `query` | bikeshare だけ・どちらも必須。`feedId` は GBFS の system id(`dsl.BikeshareSystems`: `docomo-cycle-tokyo` / `docomo-cycle` / `hellocycling`)。`query` は探す地点 `"緯度,経度"`(10 進の数だけ・日本の範囲(緯度 20〜46・経度 122〜154))。サーバーが小数 3 桁(約 100 m)に丸めて使い、**出どころには送らない** |
| `title` / `limit`(1〜20・既定 5)/ `emptyText` | 任意 |

「データの出どころ」を一覧・数字・グラフの部品の `source` として宣言する形も考えたが、最初は**専用の部品 1 つ**にした。外部のデータを記録の集計(iOS のクライアント集計)と混ぜないため、描き方を 1 か所に閉じて審査用の版から部品ごと外せるようにするため。数字・グラフに使う形は、需要を見てから足す。

## 出どころと規約

2026-11-06 に規約を確かめた(warnings・pressure・arxiv・diet・NHK は 2026-11-08、rss の `soumu-news`・`osaka-city-news`・`go-blog` と外したものの厚生労働省以下は 2026-11-12・#1701、status は 2026-11-12)。どれも鍵は要らない。

| provider | 出どころ(サーバーが取りに行く URL) | 規約 | 出典の表示 |
|---|---|---|---|
| weather | 気象庁 `https://www.jma.go.jp/bosai/forecast/data/forecast/{府県予報区}.json` | [気象庁ホームページについて](https://www.jma.go.jp/jma/kishou/info/coment.html): 権利表記の無いものは [公共データ利用規約(第1.0版)](https://www.digital.go.jp/resources/open_data/public_data_license_v1.0)(商用利用可・出典の記載が要る・CC BY 4.0 と互換) | 気象庁ホームページ |
| warnings | 気象庁 `https://www.jma.go.jp/bosai/warning/data/r8/{府県予報区}.json`(2026 年の新しい警報の形。前の `bosai/warning/data/warning/` は 2026-05 から更新が止まっている) | weather と同じ(公共データ利用規約(第1.0版)) | 気象庁ホームページ |
| pressure | MET Norway(ノルウェー気象研究所)`https://api.met.no/weatherapi/locationforecast/2.0/compact?lat=..&lon=..`(海面気圧 `air_pressure_at_sea_level`。緯度経度は府県予報区ごとにサーバーが持つ代表地点・小数 2 桁) | [Terms of Service](https://api.met.no/doc/TermsOfService)・[License](https://api.met.no/doc/License): NLOD 2.0 と CC BY 4.0。鍵・登録は無し。名乗る User-Agent が要る(無いと 403)。`Expires` より前に同じ問い合わせを繰り返さない・1 秒 20 回まで・座標は小数 4 桁まで・アプリは直接でなく自前のサーバーを通してキャッシュするよう勧めている。商用を禁じる文は無い(読んだ範囲) | MET Norway(ノルウェー気象研究所)・CC BY 4.0・加工したこと |
| holidays | 内閣府 `https://www8.cao.go.jp/chosei/shukujitsu/syukujitsu.csv`(Shift_JIS・[案内のページ](https://www8.cao.go.jp/chosei/shukujitsu/gaiyou.html)) | [内閣府ホームページ利用規約](https://www.cao.go.jp/notice/rule.html): 権利表記の無いものは公共データ利用規約(第1.0版) | 内閣府ホームページ |
| rss `digital-news` | デジタル庁 新着・更新 `https://www.digital.go.jp/rss/news.xml`([RSS の案内](https://www.digital.go.jp/rss)) | [デジタル庁 著作権について](https://www.digital.go.jp/copyright-policy): 公共データ利用規約(第1.0版) | デジタル庁ホームページ |
| rss `fdma-news` | 消防庁 新着情報 `https://www.fdma.go.jp/index.xml`([RSS の案内](https://www.fdma.go.jp/about/rss.html)) | [消防庁 このサイトについて](https://www.fdma.go.jp/about/others/post3.html): 公共データ利用規約(第1.0版) | 消防庁ホームページ |
| rss `fdma-disaster` | 消防庁 災害情報 `https://www.fdma.go.jp/disaster/info/index.xml` | 同上 | 消防庁ホームページ |
| rss `soumu-news` | 総務省 新着情報 `https://www.soumu.go.jp/news.rdf`(RSS 1.0・Shift_JIS・[RSS の案内](https://www.soumu.go.jp/menu_kyotsuu/rss_information.html)) | [総務省 著作権について](https://www.soumu.go.jp/menu_kyotsuu/policy/tyosaku.html): 権利表記の無いものは公共データ利用規約(第1.0版)。出典(例「出典：総務省ホームページ」)を書く・加工したらそう書く。ロゴ・シンボルは対象外。RSS の案内に再配布を禁じる文は無い(読んだ範囲) | PDL1.0 の例にならい「出典：総務省ホームページ(https://www.soumu.go.jp/)、PDL1.0(https://www.digital.go.jp/resources/open_data/public_data_license_v1.0)」・「題名と日付を抜き出して表示」(加工した旨) |
| rss `osaka-city-news` | 大阪市 新着情報 `https://www.city.osaka.lg.jp/main/rss/rss_new.xml`(RSS 2.0・UTF-8・100 件) | [大阪市 RSS配信](https://www.city.osaka.lg.jp/main/site_policy/0000000148.html): 「当サイトから配信しているRSSデータについては、…CC-BYで提供いたします」(リンク先は CC BY 2.1 JP)。リンク先のページの中身は [著作権・免責](https://www.city.osaka.lg.jp/main/site_policy/0000000124.html)(CC BY 4.0 と互換・商用可・出典を書く・政府標準利用規約(第2.0版)に準拠)。予告なく止める・変えることがある | 大阪市ホームページ・「CC BY 2.1 JP (https://creativecommons.org/licenses/by/2.1/jp/)・題名と日付を抜き出して表示」 |
| rss `go-blog` | The Go Blog `https://go.dev/blog/feed.atom`(Atom・英語・記事の本文ごと返る。10 件で約 360 KB) | [go.dev Copyright](https://go.dev/copyright): 「Except as noted, the contents of this site are licensed under the Creative Commons Attribution 4.0 License」(コードは BSD)。CC BY 4.0 は商用も再配布も可・出典の表示が要る。サーバーは題名・リンク・日付だけ使い、本文(`content`)は捨てる | The Go Blog(go.dev)・「CC BY 4.0 (https://creativecommons.org/licenses/by/4.0/)・題名と日付を抜き出して表示」 |
| arxiv `{分野}` | arXiv API `https://export.arxiv.org/api/query?search_query=cat:{分野}&sortBy=submittedDate&sortOrder=descending&max_results=20`(Atom)。[RSS](https://info.arxiv.org/help/rss.html)(`rss.arxiv.org/rss/{分野}`)は**土日に空になる**(毎日 1 回の告知分だけ)ので、いつでも新着が出る API にした。規約は同じ | [Terms of Use for arXiv APIs](https://info.arxiv.org/help/api/tou.html)(RSS も対象と明記): 論文の**メタデータ**(題名・著者など)は CC0 1.0 で、取得・保存・加工・共有してよい。PDF などの本文は arXiv が権利者でないので、サーバーに置いて配らない(取らない・リンクだけ)。**3 秒に 1 回まで・同時に 1 本まで**(複数の機械でも合計)。arXiv が承認・提携しているように見せない。決まった謝辞の文は求めていない。商用の扱いは書かれていない | arXiv.org・「論文の情報は CC0 1.0・arXiv の承認や提携はありません」。出典の URL はその分野の一覧(`arxiv.org/list/{分野}/recent`) |
| diet | 国立国会図書館 国会会議録検索システムの検索用 API `https://kokkai.ndl.go.jp/api/speech?any={語}&maximumRecords=20&recordPacking=json`([API の仕様](https://kokkai.ndl.go.jp/api.html)) | API の仕様のページ: 手続きは要らない。**短い時間の大量のアクセスを控え、自動の取得は並列にせず数秒あけて次を送る**。運用に障ると判断したら予告なく止められる。利用は「[国立国会図書館ウェブサイトのコンテンツ利用規約](https://www.ndl.go.jp/sitepolicy/terms)」(公共データ利用規約(第1.0版)。第三者に権利があるもの・別の規則があるものは除く)に従う。**発言の著作権は各発言者**(国会図書館の職員の発言とデータベースは国会図書館)で、転載は原則として権利者の許諾が要る(引用などの例外を除く)。商用・出典の書き方・キャッシュは書かれていない | 国立国会図書館 国会会議録検索システム・「検索用 API・発言の著作権は各発言者」 |
| status `github` | GitHub の status ページ `https://www.githubstatus.com/api/v2/summary.json`(Atlassian Statuspage・[API の案内](https://www.githubstatus.com/api)) | API の案内のページ: 「status ページの利用者がページの要素にプログラムで触れられるように」公開している。鍵・回数の上限・再配布の制限は書かれていない。ページの下は [GitHub Terms of Service](https://docs.github.com/site-policy/github-terms/github-terms-of-service) を指す: API の乱用・過度に頻繁な要求は止められることがある(H)・スパムのためにデータを取らない。公開の情報を出し直すことを禁じる文は無い(読んだ範囲) | 「GitHub の status ページ」・「各社が公開する status の API・各社の承認や提携はありません」。リンクは status ページと障害のページ |
| status `cloudflare` | Cloudflare の status ページ `https://www.cloudflarestatus.com/api/v2/summary.json`([API の案内](https://www.cloudflarestatus.com/api)) | API の案内のページ: **自動で定期的に見るなら API を使い、HTML を取らない**・連絡先の URL かメールを入れた User-Agent を名乗る(無いと強く絞る)。数の上限・再配布は書かれていない。[Website Terms of Use](https://www.cloudflare.com/website-terms/) は「Cloudflare が明示に許した場合を除き Materials を使わない」とし、AI の学習のための自動の取得を禁じる。status の API は案内のページが自動の取得を明示に許していると読んだ(人が確かめる) | 「Cloudflare の status ページ」・同上 |
| status `vercel` | Vercel の status ページ `https://www.vercel-status.com/api/v2/summary.json`([API の案内](https://www.vercel-status.com/api)) | API の案内のページ: GitHub と同じ文(プログラムで触れられるように公開)。規約のリンクは無い。[Vercel Terms of Service](https://vercel.com/legal/terms) は Vercel の顧客向けの契約で、公開の status ページを読むことには触れていない | 「Vercel の status ページ」・同上 |
| status `claude` | Claude(Anthropic)の status ページ `https://status.claude.com/api/v2/summary.json`(`status.anthropic.com` から 301 で移った・[API の案内](https://status.claude.com/api)) | API の案内のページ: GitHub と同じ文(プログラムで触れられるように公開)。規約のリンクは無い。[Consumer Terms](https://www.anthropic.com/legal/consumer-terms) は Claude.ai などとその Web サイトへの bot・スクリプトでの自動の利用を、**Anthropic が明示に許した場合を除いて**禁じる。status の API の案内を明示の許しと読んだ(人が確かめる) | 「Claude(Anthropic)の status ページ」・同上 |

シェアサイクル(bikeshare・#1694。2026-11-12 に規約を確かめた): どれも公共交通オープンデータ協議会(ODPT)の**登録の要らない公開の API**(`api-public.odpt.org`)の GBFS 2.3。取りに行くのは system ごとに `station_information.json`(ポートの名前と位置。6 時間に 1 回)と `station_status.json`(台数。2 分に 1 回)の 2 つだけ。ODPT のカタログ(CKAN)の各データセットのページにライセンスと出典の書き方(「コンテンツ等の提供者名: 〇〇 / 公共交通オープンデータ協議会」・詳しくは [ODPT の FAQ](https://developer.odpt.org/ja/faq-info#cc-by-credit))が書いてある。各 system の `system_information.json` の `license_url` も同じライセンスを指す。

| provider | 出どころ(サーバーが取りに行く URL) | 規約 | 出典の表示 |
|---|---|---|---|
| bikeshare `docomo-cycle-tokyo` | ドコモ・バイクシェア(東京エリア)`https://api-public.odpt.org/api/v4/gbfs/docomo-cycle-tokyo/station_information.json`・`https://api-public.odpt.org/api/v4/gbfs/docomo-cycle-tokyo/station_status.json`(1,937 ポート) | [ODPT のデータセット](https://ckan.odpt.org/dataset/c_bikeshare_gbfs-d-bikeshare): [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/deed.ja)(商用可・改変可・出典と変更の表示が要る)。鍵・登録は無し(鍵つきの `api.odpt.org` は使わない) | 株式会社ドコモ・バイクシェア / 公共交通オープンデータ協議会・「CC BY 4.0・近い順に並べ替えて表示」 |
| bikeshare `docomo-cycle` | ドコモ・バイクシェア(東京以外の全国)`https://api-public.odpt.org/api/v4/gbfs/docomo-cycle/station_information.json`・`https://api-public.odpt.org/api/v4/gbfs/docomo-cycle/station_status.json`(6,295 ポート) | [ODPT のデータセット](https://ckan.odpt.org/dataset/c_bikeshare_gbfs-d-nationwide-bikeshare): CC BY 4.0(同上) | 同上 |
| bikeshare `hellocycling` | HELLO CYCLING(OpenStreet 株式会社)`https://api-public.odpt.org/api/v4/gbfs/hellocycling/station_information.json`(約 8 MB)・`https://api-public.odpt.org/api/v4/gbfs/hellocycling/station_status.json`(15,708 ステーション) | [ODPT のデータセット](https://ckan.odpt.org/dataset/c_bikeshare_gbfs-openstreet): CC BY 4.0・ODC BY 1.0・ODbL 1.0 から選べる([ライセンスの文](https://d1yl7kw204zjxn.cloudfront.net/gbfs/v2/public/hellocycling_gbfs_licence.txt)は 3 つから選ぶことだけを書く)。**CC BY 4.0 を選んで使う**(ODbL の継承の条件を避ける) | OpenStreet株式会社 / 公共交通オープンデータ協議会・「CC BY 4.0(3 つのライセンスから選択)・近い順に並べ替えて表示」 |

外したもの:

| 候補 | 外した理由 |
|---|---|
| Open-Meteo(緯度経度の天気) | [利用規約](https://open-meteo.com/en/terms): 無料の API は**非商用だけ**(サブスクや広告のあるアプリは商用に当たる)。Tailor It は Pro の課金があるので使えない。商用は有料の契約(鍵が要る)。代わりに気象庁にした。気圧の予報も同じ理由で使わず、MET Norway にした(気象庁の公開の JSON に気圧の予報は無い) |
| 内閣府の報道発表の RSS(`https://www.cao.go.jp/rss/news.rdf`) | [RSS の案内](https://www.cao.go.jp/rss/index.html): フィードの内容を元にサイトを作る・再配布することを、商用・非商用を問わず禁じている |
| OpenAI の status ページ(`https://status.openai.com/api/v2/summary.json`) | [status ページ](https://status.openai.com/) は incident.io で、API の案内も規約のリンクも無い。`/api/v2/summary.json` は返るが Statuspage の形の一部(障害の一覧が無い)で、文書の無い作りに頼ることになる。[Terms of Use](https://openai.com/policies/terms-of-use/) は 2026-11-12 に読めなかった(403)。OpenAI のサービスからデータをプログラムで取り出すことを禁じる条項があるとされるので、人が読んで許されるまで入れない |
| Slack の status(`https://slack-status.com/api/v2.0.0/current`) | [Slack Status API](https://docs.slack.dev/reference/slack-status-api) は鍵なしの JSON で 1 分に 1 回までを勧めるが、Slack の API の文書の 1 つとして [API Terms of Service](https://slack.com/terms-of-service/api) が掛かると読める: **別の許可なしに Slack の API とつながるアプリを商用で配ること**(有料の製品とつながる無料のアプリも含む)と、API へのアクセスの**再配信・シンジケート**を禁じる。Tailor It は Pro の課金があり、サーバーが取って利用者に出すので当たる |
| 厚生労働省の新着情報の RSS(`https://www.mhlw.go.jp/stf/news.rdf`) | [RSS の案内](https://www.mhlw.go.jp/rss/index.html): 「営利・非営利を問わず、その情報に基づいたホームページ・メールマガジンの作成、情報の再配布は固くお断りします」。サイト全体は公共データ利用規約だが、RSS はこの決まりが優先 |
| 東京都の新着情報の RSS(`https://www.metro.tokyo.lg.jp/index.html/-/asset_publisher/knkg/rss`) | [RSS の案内](https://www.metro.tokyo.lg.jp/rss/index.html): RSS での再掲載・サイトの作成には**事前の申請**が要り、断ることもあるとある。申請していないので入れない(使うなら申請が要る・人の判断) |
| 広島市の新着情報の RSS(`https://www.city.hiroshima.lg.jp/news.rss`) | RSS の案内に決まりは無いが、[リンク・著作権・免責事項](https://www.city.hiroshima.lg.jp/about/1010662.html): 私的使用・引用などを除き、無断の転用・複製・転載は禁止(事前に担当課へ相談) |
| 千葉市の RSS(`https://www.city.chiba.jp/kinkyu.xml` ほか) | [リンク・著作権・免責事項](https://www.city.chiba.jp/front/link_copyright.html): 私的使用・引用などを除き、無断で複製・転用できない |
| Qiita の記事のフィード(`https://qiita.com/popular-items/feed`) | [利用規約](https://qiita.com/terms): 記事の著作権は書いた人に残り(8.1.3)、ほかの利用者に許すのはコードとスニペットだけ(7.3)。第三者がプログラムで配り直すことを許す文は無い |
| Zenn の記事のフィード(`https://zenn.dev/feed`) | [利用規約](https://zenn.dev/terms) 6 条 3 項: 「権利者の許可を得ることなく、無断で転載または二次配布等を行うことはできません」 |
| 経済産業省の RSS(`https://www.meti.go.jp/`) | 規約のページ(https://www.meti.go.jp/main/rules.html)が自動の取得を断り(403)、規約を確かめられなかった。確かめるまで入れない |
| NHK ニュースの RSS(`https://news.web.nhk/n-data/conf/na/rss/cat0.xml` 〜 `cat7.xml`: 主要・社会・文化・エンタメ・科学・医療・政治・経済・国際・スポーツ。旧 `www3.nhk.or.jp/rss/news/cat*.xml` は 2025-10 から転送) | [NHK ONE ニュース RSS](https://www.nhk.or.jp/toppage/rss/index.html)(2026-11-08 に読んだ): 「提供するRSSは個人の方の利用のためのみに使用することができます。ブログやプログラム等によって、商業目的での利用を含め再配信や再提供を許可するものではありません」。サーバーが取って利用者のアプリに出すのは「プログラムによる再提供」に当たるので入れない。使うなら NHK の許諾が要る(人の判断) |

status の表示: 社名・サービス名は文字だけで、ロゴは使わない。全体の状態は indicator を日本語(すべて正常 / 一部で低下 / 大きな障害 / 重大な障害 / メンテナンス中)にして、元の英語の説明を下に添える。障害とメンテナンスは名前・状態・影響・時刻だけで、更新の本文(`incident_updates`)は読まない。障害のページのリンクは、応答の URL を使わず、許可リストの status ページの URL と障害の id(英小文字と数字だけ)から作る。Cloudflare はふだんでも約 230 KB(コンポーネントが 480 ほど)あるので、上限を 2 MiB にした。

arxiv・diet の表示: どちらも**一覧(メタデータ)とリンクだけ**を出し、本文(論文の PDF・要約、発言の本文)は出さない・保存しない。国会会議録の API は本文ごと返すが、サーバーは解析で捨てる(キャッシュにも残らない)。発言の一覧(発言者・会議名・日付)の表示と、本文の扱い(著作権法 40 条 1 項は、公開の政治上の演説・陳述を、同じ人のものを編集する場合を除いて利用できるとする)で足りるかは**人が確かめる**。

加工の扱い: 公共データ利用規約は、編集・加工したら加工したことを書くよう求める。警報は気象庁のコードを名前に置き換え、種類と状態ごとに地域をまとめて出す(加工)。気圧は 1 時間ごとの値を 6 時間の行にまとめ、低下の印を足す(加工)ので、出典の license に「CC BY 4.0・6 時間ごとにまとめて加工」と書いた。天気は気象庁の天気のコードを「晴れ / くもり / 雨 / 雪」と絵に置き換えて出すので、出典の行は「出典: 気象庁ホームページ(公共データ利用規約(第1.0版))」とし、加工の表記の要否と書き方は**人が確かめる**(下の「まだ決めていないこと」)。

シェアサイクルで外した・まだ入れていない候補(#1694。2026-11-12 に調べた。日本の GBFS は [MobilityData の systems.csv](https://github.com/MobilityData/gbfs/blob/master/systems.csv) の JP の 4 件と [ODPT のカタログの GBFS の 3 件](https://ckan.odpt.org/dataset/?q=GBFS) がすべてで、入れた 3 つ以外は下の 2 つ):

| 候補 | 外した理由 |
|---|---|
| シクロシティ富山(CyclOcity・JCDecaux)`https://api.cyclocity.fr/contracts/toyama/gbfs/v3/gbfs.json` | [Mobility Database の掲載](https://mobilitydatabase.org/feeds/gbfs/gbfs-toyama) のライセンスは [JCDecaux の Open Licence](https://developer.jcdecaux.com/files/Open-Licence-fr.pdf)(Etalab の Licence Ouverte 1.0 と同じ文。商用・再配布可・出典(提供者の名前と最終更新日)が要る)。ただしこのライセンスは**集約サイトの掲載から辿ったもの**で、富山の事業者・富山市のページで確かめられていない。形も GBFS 3.0(ほかの 3 つは 2.3)。事業者のページでライセンスを確かめてから足す(人の判断) |
| ODPT の鍵つきの API(`https://api.odpt.org/api/v4/gbfs/{system}/...?acl:consumerKey=...`) | 中身は公開の API と同じで、開発者サイトの登録と鍵(秘密)が要る。公開の API で足りるので使わない |

許可リストを増やす時: `backend/internal/feeds/client.go` の `DefaultEndpoints` と `backend/pkg/dsl/feed.go` の `RSSFeeds` に 1 行ずつ足す(テストが両方の一致を見る)。status ページは `DefaultEndpoints` の `Status` と `backend/pkg/dsl/feed.go` の `StatusPages`(`TestStatusAllowlistMatchesDSL` が一致を見る。Atlassian Statuspage の `/api/v2/summary.json` の形だけ読める)。arXiv の分野は `backend/pkg/dsl/feed_arxiv_diet.go` の `ArxivCategories`(スキーマの `feedId` の説明と AI の規則 `feed_arxiv_diet_rules.go` にも書く。テストが規則との一致を見る)。**規約(再配布・商用・出典)を確かめて、この表に URL を書いてから**。表に無い URL・https でない URL・「外したもの」の URL がコードの許可リストにあると、`TestAllowlistMatchesDoc`(`internal/feeds/allowlist_doc_test.go`)が落ちる。追加は人の判断(#339)。シェアサイクルの system は `client.go` の `Bikeshare` と `backend/pkg/dsl/feed_bikeshare.go` の `BikeshareSystems`(`TestBikeshareAllowlistMatchesDSL` が一致を見る)とスキーマの `feedId` の説明。

#1701 で足した `soumu-news`・`osaka-city-news`・`go-blog` は、AI の規則(`backend/internal/ai/feed_rules.go`)にはまだ書いていない(生成アプリから外の API を使う決まりは親の Issue の決定待ち)。DSL に書けば検証は通り、表示もできる。出典の license はフィードごと(`RSSFeed.License`。空なら公共データ利用規約(第1.0版))。3 本はどれも本文を使わず題名と日付だけを抜き出すので、license にその旨(変更の表示)とライセンスの URL を入れる。

ISBN から本の情報を引く `lookupBook` も同じ作り(専用のクライアント・固定の URL・上限・キャッシュ・止める)で、出どころは openBD だけ: [book-lookup.md](book-lookup.md)。

## 安全の作り(#339 の条件との対応)

| #339 の条件 | この実装 |
|---|---|
| 宛先は運営の許可リストだけ | provider は閉じた enum。取りに行く URL はサーバーのコードで固定。DSL・リクエストから受け取るのは provider と型の決まった値(6 桁の府県予報区・フィードの id・件数)だけで、どれも許可リストと照合してから使う。許可リストに無いものは、通信の前に 400 で断る |
| 仲介を通す | 端末は appconfig にだけ聞き、外部へは直接つながらない。appconfig の外への通信は `internal/feeds` の専用のクライアントだけ |
| 内部のアドレスに届かない | 専用のクライアントは、接続の直前に**実際の接続先のアドレス**を確かめ、公開のユニキャスト以外(ループバック・プライベート・リンクローカル(メタデータのサーバーを含む)・CGNAT・マルチキャスト・文書用・IPv4 を写した IPv6 など)には接続しない。名前を引き直して内部を指すようになっても同じ。環境のプロキシ設定は使わない。リダイレクトは許可リストのホストの同じ scheme の中だけ(3 回まで) |
| 送る項目を最小にする | 送るのは府県予報区のコード・フィードの URL・arXiv の分野・国会会議録で探す語(diet の `query`)だけ。diet の語は**アプリを作った時に DSL に書いた決まった語**で、利用者がその場で打った文字や記録の値ではない(DSL から値を差し込む形は無い)。ただし API は認証した利用者なら任意の語(40 字まで)を送れるので、語は国会図書館に届く。アプリのログには語を残さない(取りに行くたびのログ `feeds.fetch` は語の文字数だけ・アクセスのログはパスだけ)。ただし Cloud Run の要求ログは URL を問い合わせごと残すので、語はそこに残る。利用者の uid・位置・記録の値は送らない。**緯度経度は受け取らない**(天気・警報・気圧は府県の単位)ので、ログにも座標は残らない。気圧の出どころには府県ごとに決まった代表地点(県庁などの所在地・`internal/feeds/pressure_points.go`)の座標を送るだけで、利用者の位置ではない。bikeshare は例外で地点(`query` の緯度経度)を受け取るが、サーバーは小数 3 桁(約 100 m)に丸め、**出どころには送らない**(system の全ポートを決まった URL で取り、近い順はサーバーで選ぶ)。キャッシュは system ごとに 1 つで地点を鍵にしない。`feeds.fetch` のログにも地点は書かない。地点は DSL に書いた決まった値(いつもの駅など)で、端末の今の位置ではない |
| 秘密は DSL に置かない | 鍵の要らない出どころだけなので、秘密は無い |
| 量の上限 | 1 回 8 秒まで・応答の大きさの上限(天気 256 KiB・警報 512 KiB・気圧 256 KiB・祝日 128 KiB・RSS 1 MiB(`go-blog` は記事の本文ごと返るため。2026-11-12 の実測は 10 件で約 360 KB)・arxiv 1 MiB・diet 3 MiB(発言の本文ごと返るため。20 件の実測は 84 KB)・status 2 MiB(Cloudflare はふだんで約 230 KB))・保存する行は 60 まで・キャッシュは 2000 件まで(diet の語で増えないよう、古いものから捨てる)。arxiv と diet は出どころの求めに合わせ、インスタンスあたり**同時に 1 本**で、前の取得から間をあけて取りに行く(diet は 3 秒。arxiv は 3 秒 × appconfig の最大インスタンス数(`deploy-cloudrun.yml` の `--max-instances=3`)= 9 秒にして、全インスタンスが同時に取っても合計で 3 秒に 1 回を超えないようにした。最大インスタンス数を変える時は `backend/internal/feeds/feeds.go` の `appconfigMaxInstances` も変える)。順番待ちは来た順で 10 秒まで。超えたら 503。取得は待っていた要求の期限と切り離して 8 秒まで続け、結果はキャッシュに入る(要求の側の打ち切りは失敗として覚えない)。キャッシュ(天気 30 分・警報 10 分・気圧 30 分・祝日 12 時間・RSS 15 分・arxiv 3 時間・diet 3 時間・status 5 分。気圧の問い合わせは府県の 56 地点にまとまる)で同じ問い合わせはまとめ、同時の問い合わせも 1 回の取得にまとめる。出どころへの取得はインスタンスあたり 1 分 30 回まで(diet は別枠で 1 分 10 回。検索語はキャッシュに当たりにくいので、ほかの部品の枠を使わない)。利用者ごとに 1 時間 120 回まで。検索語のある問い合わせ(diet)で出どころまで取りに行くのは、利用者ごと・インスタンスごとに 1 時間 20 回まで(キャッシュにある語はこの数に入らない。超えたら 429)。失敗は 1 分覚えて、すぐに取り直さない。bikeshare: 応答の上限は 16 MiB(HELLO CYCLING の `station_information.json` が 2026-11-12 に約 8 MB・`station_status.json` が約 4.5 MB)・キャッシュは台数 2 分(出どころの `ttl` は 60 秒)・ポートの名前と位置 6 時間。1 回の取得は出どころへ 1〜2 本で、1 分の枠は 1 回と数える。全ポートを持つので保存する行の 60 の上限は当てない(HELLO CYCLING で約 1.6 万行)。地点は出どころに届かないので、利用者ごとの検索語の数にも入れない。端末のキャッシュは 60 秒(ほかは 300 秒) |
| ログ | 外へ取りに行くたびに 1 行(`event=feeds.fetch`・成功は INFO・失敗は WARN): provider・status(`ok` / HTTP の番号 / `timeout`・`network`・`too_large`・`bad_response` などの種類)・bytes・duration_ms と、許可リストの region・feed_id・語の文字数(q_runes)。キャッシュから返した時は外へ出ないので書かない。語・URL・応答の本文・uid は書かない(通信の失敗の中身は URL を含むので、種類だけを書く) |
| 同意と開示 | 利用者の情報を外部へ送らないので、第三者への提供には当たらない(と考える)。ただしプライバシーポリシー・App Privacy の書き方は人が確かめる(下) |
| 止められる | `FEEDS_DISABLED` で provider ごとに止められる(`all` で全部。今回足した出どころだけ止めるなら `FEEDS_DISABLED=arxiv,diet`)。止めた provider は 503 で、部品は「読み込めませんでした」 |
| 試験 | 許可リストの外・内部のアドレスに届かないこと・応答の上限・キャッシュを Go のテストで確かめる(下)。テストは偽の出どころ(httptest)だけを使い、実際の外部には出ない |

表示の側: 取った文字は制御文字・見えない文字を除いて長さで切る。RSS の記事のリンクは https だけ(それ以外は捨てる)。リンクは Safari で開く(アプリの中で Web は表示しない)。

## API

`POST /v1/feeds/{provider}` に `{"region","feedId","q","limit"}`(認証が要る。`q` は diet(検索語)と bikeshare(地点 `"緯度,経度"`)だけ。利用者が入力した値を URL に載せないため本文で送る・`GET ?region=&feedId=&q=&limit=` は古い iOS ビルドのために残している)。形は [openapi-appconfig.yaml](../openapi-appconfig.yaml) の `/feeds/{provider}`。

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
| `FEEDS_USER_PER_HOUR` | 1 人 1 時間の `/v1/feeds` の回数(GET と POST の合計・`0` で数えない) | 120 |
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
- status: `backend/internal/feeds/status_test.go`(2026-11-12 に 1 回だけ実際に取った応答を `testdata/status_cloudflare_summary.json`(切ったもの)・`testdata/status_claude_summary.json` に置いた。行の順・本文を出さない・リンクを id から作る・知らない値・許可リストの外を通信の前に断る・壊れた応答・失敗を覚える・大きさの上限・止めるスイッチ・コードと DSL の一覧の一致)・`backend/pkg/dsl/feed_status_test.go`。
- iOS: `FeedTests`(DSL から問い合わせ)・`FeedLeafTests`(表示の組み立て)・`FeedClientTests`(appconfig だけに聞く・キャッシュ)。
- bikeshare: `backend/internal/feeds/bikeshare_test.go`(2026-11-12 に 1 回だけ実際に取った応答を、東京駅の近くの 6 ポート + 2.8 km 先の 1 つ(ドコモ・東京)・新御徒町の近くの 4 ステーション + 2.8 km 先の 1 つ(HELLO CYCLING)に切って `testdata/gbfs_*_station_{information,status}.json` に置いた。近い順・2 km の外を出さない・貸出停止・数の station_id・撤去済み・地点を出どころに送らない・地点が違ってもキャッシュは 1 つ・台数は 2 分 / 位置は 6 時間で取り直す・通信の前に断る値・応答の上限・止めるスイッチ)・`backend/pkg/dsl/feed_bikeshare_test.go`(`Validate()` と地点の読み取り・丸め)。

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
- **bikeshare(#1694)の追加は人の判断(#339)**。develop に入れる前にオーナーが下を確かめる(`FEEDS_DISABLED=bikeshare` で止められる):
  - 規約: ライセンス(CC BY 4.0・HELLO CYCLING は 3 つから CC BY 4.0 を選ぶ)は ODPT のカタログで読んだ。`api-public.odpt.org` そのものの利用条件(回数・商用・キャッシュ)は、ODPT の開発者サイトが JavaScript で描くページで 2026-11-12 に機械で読めず、確かめられていない。出典の書き方(FAQ の「コンテンツ等の提供者名」の形・変更の表示「近い順に並べ替えて表示」)で足りるか。
  - 正典の変更: DSL のスキーマ(`feed.provider` の enum に `bikeshare`・`feedId` / `query` の説明)を変えた。
  - 出どころの量: HELLO CYCLING は 1 回に約 4.5 MB(6 時間に 1 回は +8 MB)を取る。使われる間はインスタンスごとに 2 分に 1 回。
  - 端末: iOS(`Feed.providers`)と Android は `bikeshare` をまだ知らない(この PR は backend だけ)。AI に選ばせる規則(`feed_rules.go` ほか)も足していない(#1694 の親の「部品と AI の決まり」の後)。
  - 地点: DSL に書いた決まった地点だけ。端末の今の位置で探す形は、位置を送ることになるので別に決める。
  - ドコモのポート名は事業者の番号つき(「B2-33.新槇町ビル」)をそのまま出す。
- 府県予報区のコードの一覧は、気象庁の公開の情報と開発者の資料から作った。十勝・奄美は別のファイルが無い(釧路・根室、鹿児島のファイルに入る)ので一覧に入れていない。
- **status の追加(#339・#1675)は 2026-11-12 にオーナーが OK した**(4 社(GitHub・Cloudflare・Vercel・Claude)と、正典のスキーマの `provider` の enum に `status`)。`FEEDS_DISABLED=status` で止められる。読んだ時の前提(規約が変わったら見直す):
  - どの status ページにも「出し直してよい」とはっきり書いた規約は無い。API の案内の「プログラムで触れられるように公開」(Cloudflare は「自動の取得は API で」)を、サーバーが取って利用者のアプリに出すことの許しと読んだ。Cloudflare の Website Terms と Anthropic の Consumer Terms は、明示の許しが無い自動の利用を禁じる形なので、その読みで足りるか。
  - OpenAI(incident.io・規約が読めなかった)と Slack(API Terms of Service の商用・再配信の制限)は外した。入れるなら許諾か、規約の読み直しが要る。
  - 社名とサービス名は名前だけを出し、ロゴは出さない(アイコンは状態を表す汎用のものだけ)。出典に「各社の承認や提携はありません」と書く。
