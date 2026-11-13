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
| `deepl` | 有料・運営の鍵 | DeepL API Pro の利用規約(推測: `https://www.deepl.com/pro-license`)・使える量 [Usage limits](https://developers.deepl.com/docs/resources/usage-limits)(repo の文書) | 要確認。Pro だけを使う(無料の版はデータの扱いと商用が曖昧・[decisions/external-apis.md](../../decisions/external-apis.md) の候補 13) | 要確認 | 規約: Pro は送った文を保存・学習に使わない(repo の文書)。今の作り: 文はキャッシュもログもしない・無料の鍵(`:fx`)では on にしない(`deepl.go`) | |
| `frankfurter` | 無料・鍵なし | [frankfurter.dev](https://frankfurter.dev/)(repo の文書) | 合わせたレートは商用可・1 つの出どころのレートはその出どころの規約(repo の文書)。節の番号は無い見込み(推測) | 要確認(コードに出典の定数は無い) | 今の作り: latest 1 時間・historical 1 日キャッシュ | |
| `github` | 無料・OAuth(GitHub App) | GitHub Terms of Service の API の節(推測: `https://docs.github.com/site-policy/github-terms/github-terms-of-service` の「H. API Terms」) | 要確認 | 要確認 | 要確認。今の作り: キャッシュしない・同じトークンの呼び出しは 1 本ずつ([external-apis.md「GitHub と Linear の注」](external-apis.md#github-と-linear-の注16741684)) | |
| `googleCalendar` | 無料・OAuth | Google API サービスの利用者データのポリシー(推測: `https://developers.google.com/terms/api-services-user-data-policy`)・Google APIs の利用規約(推測: `https://developers.google.com/terms`)。scope の確認は [センシティブな scope の確認](https://developers.google.com/identity/protocols/oauth2/production-readiness/sensitive-scope-verification)(repo の文書) | 要確認 | 要確認 | 規約: 使う目的を絞る・広告に使わない(repo の文書)。今の作り: キャッシュしない | |
| `googlePlaces` | 有料・運営の鍵 | Google Maps Platform の規約(推測: `https://cloud.google.com/maps-platform/terms`)・[Places API の決まり](https://developers.google.com/maps/documentation/places/web-service/policies)(repo の文書) | 地図のデータを別のサービスとして売らない(repo の文書)。節は要確認 | 「Google Maps」の表示とロゴの決まり。コード `GooglePlacesAttribution` = `Google Maps` | 規約: 場所の id は保存してよい・緯度経度は 30 日まで・名前・住所・口コミは保存しない(repo の文書)。今の作り: キャッシュしない・値は場所の id(`googleplaces.go`) | |
| `googleSheets` | 無料・OAuth(scope `drive.file`) | `googleCalendar` と同じ(推測) | 要確認 | 要確認 | 今の作り: シートの中身をキャッシュしない(`google_sheets.go`) | |
| `googleTasks` | 無料・OAuth | `googleCalendar` と同じ(推測) | 要確認 | 要確認 | 今の作り: キャッシュしない | |
| `gtfsRealtime` | 無料・鍵なし | フィードごとのライセンス(今は都営バス・CC BY 4.0)と、公共交通オープンデータセンターの [利用規約](https://developer.odpt.org/terms/center_use_rules.html)・[基本ライセンス](https://developer.odpt.org/terms/data_basic_license.html)(repo の文書)。[external-apis.md「GTFS リアルタイムのフィードと規約」](external-apis.md#gtfs-リアルタイムのフィードと規約1692) | 要確認(フィードごと) | CC BY のクレジット(フィードの `credit`)と `GTFSAttribution`・センターのフィードは `ODPTAttribution` も | 規約: 古いデータを使わない・再配布の決まり(同上)。今の作り: 30 秒キャッシュ・生のデータは端末に返さない | |
| `linear` | 無料・OAuth | Linear の API の規約(要確認。名前と URL も分からない) | 要確認 | 要確認 | 要確認。今の作り: キャッシュしない | |
| `natureRemo` | 無料・利用者のトークン(#1671 まで off) | Nature Remo Cloud API の規約(要確認。開発者のページ `https://developer.nature.global/` は推測) | 要確認 | 要確認 | 要確認。今の作り: キャッシュしない・在宅が分かる値は返さない(`nature_remo.go`) | |
| `notion` | 無料・OAuth | Notion の開発者向けの規約(要確認。名前と URL も分からない)・[Authorization](https://developers.notion.com/docs/authorization)(repo の文書) | 要確認 | 要確認 | 要確認。今の作り: キャッシュしない | |
| `odpt` | 無料・運営の鍵 | [公共交通オープンデータセンター利用規約](https://developer.odpt.org/terms/center_use_rules.html)・[基本ライセンス](https://developer.odpt.org/terms/data_basic_license.html)・[開発者ガイドライン](https://developer.odpt.org/terms/data_basic_use_guideline.html)(repo の文書)。[external-apis.md「ODPT の事業者と規約」](external-apis.md#odpt-の事業者と規約1691) | 基本ライセンス 4 条 7 項(営利・非営利を問わない)・8 条 4 項(再配布)(repo の文書) | ガイドライン「利用者への通知」・都営は CC BY のクレジット。コード `ODPTAttribution`・`ODPTToeiCredit` | ガイドライン 2.1.2(`dct:valid` を過ぎたデータは使わない)。今の作り: 1 分キャッシュ・`dct:valid` で捨てる | |
| `openWeather` | 無料・運営の鍵(One Call 3.0 の UV は回数で原価) | OpenWeather の利用規約(推測: `https://openweathermap.org/terms`)・[One Call 3.0](https://openweathermap.org/api/one-call-3)(repo の文書) | 要確認 | ODbL・出典の表示。コード `OpenWeatherAttribution` = `Weather data © OpenWeather (ODbL)` | 規約: キャッシュしてよい(`openweather.go` のコメント。節は要確認)。今の作り: キャッシュしない | |
| `outlookCalendar` | 無料・OAuth | Microsoft APIs の利用規約(推測: `https://learn.microsoft.com/legal/microsoft-apis/terms-of-use`)。[Microsoft Graph](https://learn.microsoft.com/graph/overview)(repo の文書) | 要確認 | 要確認 | 要確認。今の作り: キャッシュしない | |
| `podcasts` | 無料・鍵なし | [iTunes Search API](https://performance-partners.apple.com/search-api)(repo の文書)。[external-apis.md「iTunes Search API の規約」](external-apis.md#itunes-search-api-の規約1703) | 使い道(ストアの中身を広めるためだけ)の決まり(repo の文書)。節の番号は無い見込み(推測) | バッジと「provided courtesy of iTunes」。コード `PodcastsAttribution` | 規約: 試聴は保存・キャッシュしない・求めに応じて消す。今の作り: 6 時間キャッシュ・音声の URL は読まない | |
| `slack` | 無料・OAuth(ユーザーのトークン) | Slack API Terms of Service(推測: `https://slack.com/terms-of-service/api`) | **#1954 が「機能に課金しない趣旨の条項がある」と言う**。商用の配布は Marketplace か別の契約(`slack.go` のコメント)。節は要確認 | 要確認 | 規約: 機能に要る間だけ持つ・モデルの学習に使わない・プライバシーポリシーに読むものを書く(`slack.go`)。今の作り: 本文を読む op・ためる op は無い | |
| `switchbot` | 無料・利用者のトークン(#1671 まで off) | SwitchBot API の規約(要確認。名前と URL も分からない) | 要確認 | 要確認 | 要確認。今の作り: キャッシュしない・在宅が分かる値は返さない(`switchbot.go`) | |
| `todoist` | 無料・OAuth | Todoist API の規約(要確認)。[Todoist API v1](https://developer.todoist.com/api/v1/)(repo の文書) | 要確認 | 連携の名前は「… for Todoist」・Todoist と提携していない旨の断り書き(`todoist.go`・repo の文書) | 要確認。今の作り: キャッシュしない | |
| `vercel` | 無料・OAuth(integration) | Vercel の integration の規約(要確認。名前と URL も分からない) | 要確認 | 要確認 | 要確認。今の作り: キャッシュしない | |
| `withings` | 無料・OAuth | Withings の公開 API の規約(要確認。名前と URL も分からない) | 要確認 | 要確認 | 要確認。今の作り: 健康のデータはキャッシュもログもしない・記録に書かない(`withings.go`) | |
| `youtube` | 無料・運営の鍵(1 日の割り当て) | [YouTube API サービス利用規約](https://developers.google.com/youtube/terms/api-services-terms-of-service)・[開発者ポリシー](https://developers.google.com/youtube/terms/developer-policies)(repo の文書)。[external-apis.md「YouTube の規約」](external-apis.md#youtube-の規約1699) | 要確認(割り当ては規約 15・ポリシー III.D.3) | 規約 10.3・ポリシー III.F.2。コード `YouTubeAttribution` = `YouTube` | ポリシー III.E.4.b〜e(30 日まで)。今の作り: 1 時間 / 30 分キャッシュ | |

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
