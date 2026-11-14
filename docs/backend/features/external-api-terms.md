# 外の API の provider ごとの規約の条項の一覧

#2313(親 #1954)。runner の registry(`backend/internal/extapi` の各ファイルの `register`)にある provider を 1 行ずつ並べ、規約の名前と URL・課金に関する条項の場所・出典の表示・データの保存の決まりを書く。#1954(「サービスの機能に課金しない」趣旨の条項がある provider で、`runs` に数える・Pro だけにする扱いでよいか)を人が決めるための材料。

- **読みの結論は書かない。** 条項が「当たる / 当たらない」は人が規約を読んで決める(#1954)。この文書は場所と要約だけ。条項の文は引用しない。
- **値段は書かない。**
- 設計と決定は [decisions/external-apis.md](../../decisions/external-apis.md)(「審査・規約」「入れる API の候補」)、provider の中身は [external-apis.md](external-apis.md)。

## 目次

1. [書き方](#書き方)
2. [一覧](#一覧)
3. [登録していない・入れていないもの](#登録していない入れていないもの)
4. [人の作業](#人の作業)

## 書き方

| 列 | 中身 |
|---|---|
| provider | registry の `ID`(DSL の名前)。既定は全部 off(`EXT_<PROVIDER>_ENABLED`) |
| 種類 | `Billing`(無料 / 有料)と `NeedsOAuth`(利用者ごとのトークン)・利用者の鍵 |
| 規約 | 規約の名前と URL。**「repo の文書」** はこの repo の文書に既に書いてあり、その日付に人か調べた人が開いたもの。**「推測」** はこの文書を書いた時に開いて確かめていない URL(名前と場所は人が確かめる) |
| 課金に関する条項 | 料金・商用・再販に触れる条項の**場所**(節の番号)。分からない所は「要確認」。#1954 が「ある」と言っている provider はそう書く |
| 出典の表示 | 規約が求める表示と、コードの定数(あれば) |
| データの保存 | 規約が書く保存・キャッシュの決まり(分かる所)と、今の作り(コードのキャッシュ・保存) |
| 人が確かめた | 空欄。人が規約を開いて上の列を確かめたら、日付と名前を書く |

## 一覧

registry の順(名前順)。偽の provider(`fakeFree`・`fakePaid`・`fakeOauth`・`fake.go`)は stg と試験だけで本番では on にならないので載せない。

| provider | 種類 | 規約 | 課金に関する条項 | 出典の表示 | データの保存 | 人が確かめた |
|---|---|---|---|---|---|---|
| `asana` | 無料・OAuth | Asana API Terms(推測: Asana の規約のページ `https://asana.com/terms`) | **#1954 が「機能に課金しない趣旨の条項がある」と言う**。節は要確認 | 規約の決まりは要確認([external-apis.md](external-apis.md) の表に「出典の表示」とある) | 要確認(「データを売らない」とある)。今の作り: OAuth の op はキャッシュしない | |
| `connpass` | 無料・運営の API キー(フリープラン) | connpass API 利用規約(推測: `https://connpass.com/api/api-term`。2026-11-14 は 404 で開けなかった)・[API のご利用について(help)](https://help.connpass.com/api/)・[API v2 リファレンス](https://connpass.com/about/api/v2/)(どちらも 2026-11-14 に開いた。リファレンスは JS で描くので中身は未確認) | **help に「業務目的でフリープラン用 API を利用することはできません」とある**(フリープランは 12 秒に 1 回・1 日 100 回・2026-11-04 から)。有料プラン(コミュニティ・スタンダード)の条件と、Tailor It での利用が業務目的に当たるかは要確認 | 要確認(help に出典の決まりは無い。コードに出典の定数は無い) | 要確認(規約を開けていない)。今の作り: 10 分キャッシュ・説明の本文は読まない・キーはサーバーの env だけ(help「キーを公開の場所に置かない」) | |
| `deepl` | 有料・運営の鍵 | DeepL API Pro の利用規約(推測: `https://www.deepl.com/pro-license`)・使える量 [Usage limits](https://developers.deepl.com/docs/resources/usage-limits)(repo の文書) | 要確認。Pro だけを使う(無料の版はデータの扱いと商用が曖昧・[decisions/external-apis.md](../../decisions/external-apis.md) の候補 13) | 要確認 | 規約: Pro は送った文を保存・学習に使わない(repo の文書)。今の作り: 文はキャッシュもログもしない・無料の鍵(`:fx`)では on にしない(`deepl.go`) | |
| `estat` | 無料・運営の鍵(appId) | [e-Stat API 利用規約](https://www.e-stat.go.jp/api/api-info/api-terms)(推測)・[クレジット表示](https://www.e-stat.go.jp/api/api-info/credit)(repo の文書) | 要確認(規約の URL も推測) | クレジット表示のページの文(2026-11-14 に確認)。コード `EStatAttribution` = 「このサービスは、政府統計総合窓口(e-Stat)のAPI機能を使用していますが、サービスの内容は国によって保証されたものではありません。」・出典 `EStatSource` = `政府統計の総合窓口(e-Stat)` | 要確認。今の作り: searchStats 1 時間・statsData 1 日キャッシュ | |
| `foursquare` | 有料・運営の鍵 | Foursquare の API の規約(推測: `https://foursquare.com/legal/api/`)・[Pricing](https://foursquare.com/pricing/)(repo の文書・decisions の候補 10) | 要確認(料金のページに表示・商用の決まりが無い) | 要確認。今の作り: `FoursquareAttribution` = `Powered by Foursquare` | 要確認。今の作り: キャッシュしない・場所の id(`fsq_place_id`)を値に返す(`foursquare.go`) | |
| `frankfurter` | 無料・鍵なし | [frankfurter.dev](https://frankfurter.dev/)(repo の文書) | 合わせたレートは商用可・1 つの出どころのレートはその出どころの規約(repo の文書)。節の番号は無い見込み(推測) | 要確認(コードに出典の定数は無い) | 今の作り: latest 1 時間・historical 1 日キャッシュ | |
| `github` | 無料・OAuth(GitHub App) | GitHub Terms of Service の API の節(推測: `https://docs.github.com/site-policy/github-terms/github-terms-of-service` の「H. API Terms」) | 要確認 | 要確認 | 要確認。今の作り: キャッシュしない・同じトークンの呼び出しは 1 本ずつ([external-apis.md「GitHub と Linear の注」](external-apis.md#github-と-linear-の注16741684)) | |
| `gitlab` | 無料・OAuth(scope `read_api`) | GitLab の Terms of Service(推測: `https://about.gitlab.com/terms/`)・[API v4](https://docs.gitlab.com/api/rest/)・[OAuth 2.0](https://docs.gitlab.com/api/oauth2/)(推測: この文書を書いた時に開いて確かめていない) | 要確認 | 要確認 | 要確認。今の作り: キャッシュしない・gitlab.com だけ(`gitlab.go`) | |
| `googleCalendar` | 無料・OAuth | Google API サービスの利用者データのポリシー(推測: `https://developers.google.com/terms/api-services-user-data-policy`)・Google APIs の利用規約(推測: `https://developers.google.com/terms`)。scope の確認は [センシティブな scope の確認](https://developers.google.com/identity/protocols/oauth2/production-readiness/sensitive-scope-verification)(repo の文書) | 要確認 | 要確認 | 規約: 使う目的を絞る・広告に使わない(repo の文書)。今の作り: キャッシュしない | |
| `googleDrive` | 無料・OAuth(scope `drive.file`) | Google API サービスの利用者データのポリシー(推測: `https://developers.google.com/terms/api-services-user-data-policy`)・[制限付きの scope の確認](https://developers.google.com/identity/protocols/oauth2/production-readiness/restricted-scope-verification)(推測。`drive.readonly` / `drive.metadata.readonly` は制限付きなので使わない) | 要確認 | 要確認 | 今の作り: ファイルの中身は読まない・キャッシュしない(`google_drive.go`) | |
| `googlePhotos` | 無料・OAuth | Google API サービスの利用者データのポリシー(推測: `https://developers.google.com/terms/api-services-user-data-policy`)・Photos API の [Updates](https://developers.google.com/photos/support/updates)(repo の文書・decisions の候補 24) | 要確認 | 要確認 | 要確認(Photos API の利用者データのポリシー)。今の作り: OAuth の op はキャッシュしない・`baseUrl` は端末に返さない(`google_photos.go`) | |
| `googlePlaces` | 有料・運営の鍵 | Google Maps Platform の規約(推測: `https://cloud.google.com/maps-platform/terms`)・[Places API の決まり](https://developers.google.com/maps/documentation/places/web-service/policies)(repo の文書) | 地図のデータを別のサービスとして売らない(repo の文書)。節は要確認 | 「Google Maps」の表示とロゴの決まり。コード `GooglePlacesAttribution` = `Google Maps` | 規約: 場所の id は保存してよい・緯度経度は 30 日まで・名前・住所・口コミは保存しない(repo の文書)。今の作り: キャッシュしない・値は場所の id(`googleplaces.go`) | |
| `googleSheets` | 無料・OAuth(scope `drive.file`) | `googleCalendar` と同じ(推測) | 要確認 | 要確認 | 今の作り: シートの中身をキャッシュしない(`google_sheets.go`) | |
| `googleTasks` | 無料・OAuth | `googleCalendar` と同じ(推測) | 要確認 | 要確認 | 今の作り: キャッシュしない | |
| `gtfsRealtime` | 無料・鍵なし | フィードごとのライセンス(今は都営バス・CC BY 4.0)と、公共交通オープンデータセンターの [利用規約](https://developer.odpt.org/terms/center_use_rules.html)・[基本ライセンス](https://developer.odpt.org/terms/data_basic_license.html)(repo の文書)。[external-apis.md「GTFS リアルタイムのフィードと規約」](external-apis.md#gtfs-リアルタイムのフィードと規約1692) | 要確認(フィードごと) | CC BY のクレジット(フィードの `credit`)と `GTFSAttribution`・センターのフィードは `ODPTAttribution` も | 規約: 古いデータを使わない・再配布の決まり(同上)。今の作り: 30 秒キャッシュ・生のデータは端末に返さない | |
| `jira` | 無料・OAuth(Atlassian 3LO) | Atlassian Developer Terms(推測: `https://developer.atlassian.com/platform/marketplace/atlassian-developer-terms/`) | **#1954 が Atlassian を名指しする**。節は要確認 | 要確認 | 要確認。今の作り: キャッシュしない・cloud id もトークンに持たない(`jira.go`) | |
| `linear` | 無料・OAuth | Linear の API の規約(要確認。名前と URL も分からない) | 要確認 | 要確認 | 要確認。今の作り: キャッシュしない | |
| `microsoftTodo` | 無料・OAuth | Microsoft APIs Terms of Use(推測: `https://learn.microsoft.com/legal/microsoft-apis/terms-of-use`)・[Microsoft Graph](https://learn.microsoft.com/graph/overview)(repo の文書) | 要確認 | 要確認 | 要確認。今の作り: キャッシュしない | |
| `natureRemo` | 無料・利用者のトークン(#1671 まで off) | Nature Remo Cloud API の規約(要確認。開発者のページ `https://developer.nature.global/` は推測) | 要確認 | 要確認 | 要確認。今の作り: キャッシュしない・在宅が分かる値は返さない(`nature_remo.go`) | |
| `nominatim` | 無料・鍵なし | [Nominatim Usage Policy](https://operations.osmfoundation.org/policies/nominatim/)(2026-11-14 に読んだ)・データは [ODbL](https://www.openstreetmap.org/copyright) | 料金の条項は無い。方針の「重い利用」(1 秒 1 回を超える・一括の変換)の禁止と、商用の利用には自前の Nominatim を勧める記述が生成アプリに当たるかは要確認 | ODbL の出典の表示。コード `NominatimAttribution` = `© OpenStreetMap contributors` | 方針: 同じ場所の繰り返しの問い合わせを避ける(結果のキャッシュ)・結果の大量の保存はデータベースの作成に当たる(ODbL の share-alike)。今の作り: 1 日キャッシュ・台の中で 1 秒の間を空ける(`nominatim.go`) | |
| `notion` | 無料・OAuth | Notion の開発者向けの規約(要確認。名前と URL も分からない)・[Authorization](https://developers.notion.com/docs/authorization)(repo の文書) | 要確認 | 要確認 | 要確認。今の作り: キャッシュしない | |
| `odpt` | 無料・運営の鍵 | [公共交通オープンデータセンター利用規約](https://developer.odpt.org/terms/center_use_rules.html)・[基本ライセンス](https://developer.odpt.org/terms/data_basic_license.html)・[開発者ガイドライン](https://developer.odpt.org/terms/data_basic_use_guideline.html)(repo の文書)。[external-apis.md「ODPT の事業者と規約」](external-apis.md#odpt-の事業者と規約1691) | 基本ライセンス 4 条 7 項(営利・非営利を問わない)・8 条 4 項(再配布)(repo の文書) | ガイドライン「利用者への通知」・都営は CC BY のクレジット。コード `ODPTAttribution`・`ODPTToeiCredit` | ガイドライン 2.1.2(`dct:valid` を過ぎたデータは使わない)。今の作り: 1 分キャッシュ・`dct:valid` で捨てる | |
| `openMeteo` | 無料・鍵なし・**既定 off・入れるかは判断待ち**(#348 と同じ理由) | [Terms](https://open-meteo.com/en/terms)・[Licence](https://open-meteo.com/en/licence)(2026-11-14 に開いた) | Terms の「Terms of Use」→「Non-Commercial Use」: **無料の API は非商用だけ**。商用の例に「サブスクリプションや広告のあるアプリ」「商用の製品」がある(Tailor It Pro のあるアプリが当たるかは人が読んで決める)。商用は「API Subscription」(有料・鍵・ホストが `customer-api.open-meteo.com`)。無料の回数は 1 分 600・1 時間 5,000・1 日 10,000・1 月 300,000(IP ごと) | Licence: CC BY 4.0。表示する場所の横に `Weather data by Open-Meteo.com` と `https://open-meteo.com/` へのリンク。一部の元データ(英国気象庁など)は CC BY-SA。コード `OpenMeteoAttribution`・`OpenMeteoAttributionURL` | 規約にキャッシュの決まりは無い(CC BY 4.0 の範囲)。今の作り: 10 分キャッシュ | |
| `openWeather` | 無料・運営の鍵(One Call 3.0 の UV は回数で原価) | OpenWeather の利用規約(推測: `https://openweathermap.org/terms`)・[One Call 3.0](https://openweathermap.org/api/one-call-3)(repo の文書) | 要確認 | ODbL・出典の表示。コード `OpenWeatherAttribution` = `Weather data © OpenWeather (ODbL)` | 規約: キャッシュしてよい(`openweather.go` のコメント。節は要確認)。今の作り: キャッシュしない | |
| `outlookCalendar` | 無料・OAuth | Microsoft APIs の利用規約(推測: `https://learn.microsoft.com/legal/microsoft-apis/terms-of-use`)。[Microsoft Graph](https://learn.microsoft.com/graph/overview)(repo の文書) | 要確認 | 要確認 | 要確認。今の作り: キャッシュしない | |
| `podcasts` | 無料・鍵なし | [iTunes Search API](https://performance-partners.apple.com/search-api)(repo の文書)。[external-apis.md「iTunes Search API の規約」](external-apis.md#itunes-search-api-の規約1703) | 使い道(ストアの中身を広めるためだけ)の決まり(repo の文書)。節の番号は無い見込み(推測) | バッジと「provided courtesy of iTunes」。コード `PodcastsAttribution` | 規約: 試聴は保存・キャッシュしない・求めに応じて消す。今の作り: 6 時間キャッシュ・音声の URL は読まない | |
| `qiita` | 無料・鍵なし | [Qiita 利用規約](https://qiita.com/terms)・[Qiita API v2 のドキュメント](https://qiita.com/api/v2/docs)(2026-11-14 に読んだ。API 向けの条項は規約に無い) | 料金の条項は無い。11 条 5 項 3 号(サービスの転用・売却・再販の禁止)・同 4 号(商用の宣伝・勧誘の禁止)が当たるかは要確認 | 規約に出典の表示の決まりは無い(コードに出典の定数は無い)。行は題・user id・いいね・記事へのリンクだけ | 規約: 記事の権利は書いた人(8 条)・キャッシュの決まりは無い。今の作り: 10 分キャッシュ・本文は読まない(`qiita.go`) | |
| `slack` | 無料・OAuth(ユーザーのトークン) | Slack API Terms of Service(推測: `https://slack.com/terms-of-service/api`) | **#1954 が「機能に課金しない趣旨の条項がある」と言う**。商用の配布は Marketplace か別の契約(`slack.go` のコメント)。節は要確認 | 要確認 | 規約: 機能に要る間だけ持つ・モデルの学習に使わない・プライバシーポリシーに読むものを書く(`slack.go`)。今の作り: 本文を読む op・ためる op は無い | |
| `spotify` | 無料・OAuth(PKCE)・**既定 off・入れるかは判断待ち** | [Spotify Developer Terms](https://developer.spotify.com/terms)・[Developer Policy](https://developer.spotify.com/policy)・[Quota modes](https://developer.spotify.com/documentation/web-api/concepts/quota-modes)(repo の文書: [decisions/external-apis.md](../../decisions/external-apis.md) の候補 6。規約と Policy のページはこの文書を書いた時に開いていない = 推測) | **[decisions/external-apis.md](../../decisions/external-apis.md) が「再生を含むアプリの販売・アプリ内課金の禁止」「データを売らない・広告・機械学習に使わない」と言う**(節は要確認)。決定の文書の「規約で課金できない・商用が縛られるもの」は今も「つなぐ形は入れない」。開発中のモードは登録した 5 人まで・拡張の割り当ては法人だけ(2025-05-15〜) | Spotify の内容はロゴと Spotify へのリンクとともに出す(決定の文書「出典の表示」)。コードは行ごとに `open.spotify.com` のリンクを持つ。出典の定数はまだ無い(文言は判断待ち) | Policy はメタデータのキャッシュに期限を置く(要確認)。今の作り: OAuth の op はキャッシュしない・ログに曲名を出さない | |
| `strava` | 無料・OAuth・**既定 off・入れるかは判断待ち**(#1670) | [Strava API Agreement](https://www.strava.com/legal/api)(repo の文書: [decisions/external-apis.md](../../decisions/external-apis.md) の候補 26)・[Brand Guidelines](https://developers.strava.com/guidelines/)(推測) | **[decisions/external-apis.md](../../decisions/external-apis.md) が「データは本人にだけ見せる」「AI / 機械学習に使うことを禁じる」「Strava の機能を真似ない」と言う**(節は要確認)。決定の文書の「規約で課金できない・商用が縛られるもの」は今も「入れない」。回数: アプリ全体で 15 分 100 回・1 日 1,000 回(登録時の既定・読むだけの口は別枠のことがある。要確認) | 「Powered by Strava」の表示(API Agreement・Brand Guidelines。節は要確認)。コード `StravaAttribution` = `Powered by Strava` | 規約: 本人にだけ見せる・AI に渡さない・保存の期間は要確認。今の作り: OAuth の op はキャッシュしない・記録(`Cells`)に書かない・AI に渡す口は無い | |
| `switchbot` | 無料・利用者のトークン(#1671 まで off) | SwitchBot API の規約(要確認。名前と URL も分からない) | 要確認 | 要確認 | 要確認。今の作り: キャッシュしない・在宅が分かる値は返さない(`switchbot.go`) | |
| `todoist` | 無料・OAuth | Todoist API の規約(要確認)。[Todoist API v1](https://developer.todoist.com/api/v1/)(repo の文書) | 要確認 | 連携の名前は「… for Todoist」・Todoist と提携していない旨の断り書き(`todoist.go`・repo の文書) | 要確認。今の作り: キャッシュしない | |
| `vercel` | 無料・OAuth(integration) | Vercel の integration の規約(要確認。名前と URL も分からない) | 要確認 | 要確認 | 要確認。今の作り: キャッシュしない | |
| `wikipedia` | 無料・鍵なし | [Wikimedia Foundation の利用規約](https://foundation.wikimedia.org/wiki/Policy:Terms_of_Use)・[User-Agent policy](https://foundation.wikimedia.org/wiki/Policy:Wikimedia_Foundation_User-Agent_Policy)・[REST API](https://ja.wikipedia.org/api/rest_v1/)(推測: この文書を書いた時に開いて確かめていない) | 本文は CC BY-SA 4.0(再利用・商用は可、出典と同じライセンスの表示が要る)。節は要確認 | CC BY-SA 4.0 の表示。コード `WikipediaAttribution` = `Wikipedia(CC BY-SA 4.0)`・UA `TailorIt/1.0 (https://tailor-it.app)` | 規約: 上限 200 回 / 秒・連絡先入りの UA。今の作り: 1 時間キャッシュ・本文は先頭 300 文字だけ | |
| `withings` | 無料・OAuth | Withings の公開 API の規約(要確認。名前と URL も分からない) | 要確認 | 要確認 | 要確認。今の作り: 健康のデータはキャッシュもログもしない・記録に書かない(`withings.go`) | |
| `yolp` | 無料・運営のアプリ ID | [Yahoo!デベロッパーネットワーク ガイドライン](https://developer.yahoo.co.jp/guideline/)(repo の文書)・[YOLP ローカルサーチ API](https://developer.yahoo.co.jp/webapi/map/openlocalplatform/v1/localsearch.html)(推測: この文書を書いた時に開いて確かめていない) | 商用は別のページ「WebAPI の商用利用について」(repo の文書)。節は要確認 | クレジットの表示(ガイドライン)。コード `YOLPAttribution` = `Web Services by Yahoo! JAPAN` | 要確認(保存・キャッシュの決まりはガイドラインに要確認)。今の作り: 10 分キャッシュ・電話番号は出さない(`yolp.go`) | |
| `youtube` | 無料・運営の鍵(1 日の割り当て) | [YouTube API サービス利用規約](https://developers.google.com/youtube/terms/api-services-terms-of-service)・[開発者ポリシー](https://developers.google.com/youtube/terms/developer-policies)(repo の文書)。[external-apis.md「YouTube の規約」](external-apis.md#youtube-の規約1699) | 要確認(割り当ては規約 15・ポリシー III.D.3) | 規約 10.3・ポリシー III.F.2。コード `YouTubeAttribution` = `YouTube` | ポリシー III.E.4.b〜e(30 日まで)。今の作り: 1 時間 / 30 分キャッシュ | |
| `zaim` | 無料・OAuth 1.0a | Zaim API 利用規約(推測: 開発者のページ `https://dev.zaim.net/` の「利用規約」。名前と場所は人が確かめる)。[Zaim REST API](https://dev.zaim.net/home/api)(推測) | 要確認(個人の家計のデータを読み書きするので、商用・再販・利用者の同意の節を最初に確かめる) | 要確認 | 要確認。今の作り: 記録(金額・品目・場所・メモ)はキャッシュもログもしない・`Output.Cells` はサーバーの中だけ(`zaim.go`) | |
| `zipcloud` | 無料・鍵なし | [郵便番号検索 API 利用規約](https://zipcloud.ibsnet.co.jp/rule/api)・[API の説明](https://zipcloud.ibsnet.co.jp/doc/api)(2026-11-14 に読んだ) | 料金・課金の条項は無い。2.2(API の機能の提供を主目的とするサイトでの利用の禁止)・2.7(過度な商用の宣伝・広告の禁止)が当たるかは要確認 | 規約に出典の表示の決まりは無い(コードに出典の定数は無い)。データは日本郵便の全国一括データを運営が加工したもの(説明のページ) | 規約: 1.1(データの権利は運営)・4.2(回数の制限は運営の判断)・4.3(求めがあれば表示をやめる)・5.1(最新の情報を表示する)・3(予告なく止まる)。今の作り: 1 日キャッシュ(データは月ごとの更新) | |

#1954 の Atlassian(Trello)は registry に無い(認可の形が合わず provider を作っていない・[external-apis.md「Trello を入れていない理由」](external-apis.md#trello-を入れていない理由1682設計のメモ))。入れる時はこの表に足す。

## 登録していない・入れていないもの

| provider / API | 状態 | 規約 |
|---|---|---|
| `cloudflare` | 定義だけで登録しない(利用者の鍵を預かる土台 #1671 の後・`cloudflare.go` の `cloudflareProvider`) | Cloudflare の利用規約(推測: `https://www.cloudflare.com/terms/`)。課金に関する条項・出典・保存は要確認。登録する時に上の表へ移す |
| Spotify・Google Books・NewsAPI・Open-Meteo・国立国会図書館サーチ・Strava | 入れない(規約で課金・商用が縛られる) | [decisions/external-apis.md「規約で課金できない・商用が縛られるもの」](../../decisions/external-apis.md#規約で課金できない商用が縛られるもの) |

## 人の作業

- [ ] 「推測」「要確認」の行の規約を開き、名前・URL・課金に関する条項の節の番号・出典・保存の決まりを確かめて埋める。「人が確かめた」に日付と名前を書く
- [ ] #1954 が名指しする Asana・Slack(と、入れる時の Atlassian)は、課金に関する条項の場所を最初に確かめる
- [ ] 読みの結論(当たる / 当たらない)と扱い(#1954 の A〜E)は、オーナーが決めて [decisions/external-apis.md](../../decisions/external-apis.md) に書く(この文書には書かない)
- [ ] provider を registry に足した時は、この表にも 1 行足す
