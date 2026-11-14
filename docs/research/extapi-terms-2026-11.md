# 外の API 21 個の規約を開いて確かめたメモ(2026-11)

> 2026-11 に足した・足そうとしている外の API(`feat(extapi):` の PR)の規約を、公式のページを開いて読んだ記録。確認日 2026-11-15(読んだのは 2026-11-14 の夜から)。**推奨は調べた人の意見で、決定ではない。** 入れるかどうかと #1954(機能に課金しない趣旨の条項)の扱いは、オーナーが決めて [decisions/external-apis.md](../decisions/external-apis.md) に書く。値段はここには書かない。開けなかった・見つからなかったものは **未確認**。引用は原文のまま(英語は英語のまま)。

## 目次

1. [前提(Tailor It の形)](#前提tailor-it-の形)
2. [推奨の一覧](#推奨の一覧)
3. [決定と食い違うもの](#決定と食い違うもの)
4. [provider ごとの中身](#provider-ごとの中身)
5. [全体で効くこと](#全体で効くこと)
6. [未確認の所](#未確認の所)

## 前提(Tailor It の形)

- 有料のアプリ(Tailor It Pro の月額とコイン)。多くの規約で「商用」に当たる。
- 生成アプリは runner(サーバー)を通して API を呼ぶ。鍵のある API は運営の鍵を 1 本だけ使うので、**回数の上限は全利用者の合計**にかかる。
- OAuth の API は、利用者が自分のアカウントをつなぎ、本人のデータを本人だけが見る。
- アプリの定義は LLM(Claude)で作る。API のデータを LLM に渡すかどうかは provider ごとに縛りがある。
- 読む op は分〜1 日キャッシュする。

## 推奨の一覧

| provider | PR | 推奨 | 一言 |
|---|---|---|---|
| `zipcloud` | #3703(マージ済み) | 入れてよい | 課金の条項なし。郵便番号だけのアプリは 2 条 2 号に当たるおそれ |
| `qiita` | #3705(マージ済み) | 条件付き | 題・リンク・いいねだけ・本文は出さず LLM にも渡さない。Guideline は未読 |
| `googleDrive` | #3707(マージ済み) | 入れてよい(条件あり) | `drive.file` は非センシティブ。Limited Use・学習に使わない・キャッシュは header に従う |
| `wikipedia` | #3702(マージ済み) | 入れてよい | CC BY-SA の表示とリンク・UA に連絡先・200 回 / 分 |
| `microsoftTodo` | #3704(マージ済み) | 条件付き | データの再販の禁止・退会で消す・プライバシーポリシーと同意の取り消しのリンク |
| `zaim` | #3710 | **入れない** | **Zaim API は 2026-12-07 に終わる**・新しい登録は 2026-11-09 で締め切り・後継なし |
| `connpass` | #3711 | **入れない**(契約と承諾があれば可) | 無料の版は業務に使えない・第三者向けに出すには事前の承諾 |
| `jira` | #3712 | 条件付き | Developer Terms 8.3「間接にも課金しない」。Pro 限定・コインを引く形にしない |
| `dropbox` | #3713 | 条件付き | 50 人をつないだら 2 週間で本番の承認が要る |
| `spotify` | #3714 | **入れない** | 開発モードは 5 人まで・拡張は法人と 25 万 MAU・AI と分析の禁止 |
| `strava` | #3715 | 条件付き(決定は「入れない」) | 本人のデータは本人だけ・1,000 回 / 日。AI の禁止が消えたかは未確認 |
| `openMeteo` | #3716 | 条件付き(有料の API の契約が要る) | 無料の API は非商用だけ |
| `backlog` | #3718 | 条件付き | 無償の許諾・本人のデータを本人に。消す求めに応える・求められたら表示 |
| `estat` | #3719 | 入れてよい | 商用可。指定の文言と出典を必ず出す |
| `nominatim` | #3720 | 条件付き(できれば別のもの) | 全体で 1 回 / 秒・1 本ずつ・自動補完の禁止・止める仕組みが要る |
| `nhk` | #3723 | **入れない**(未確認のため保留) | 規約の本文を開けなかった。営利の禁止があれば合わない |
| `lastfm` | #3724 | **入れない**(商用の契約があれば可) | 3.1 非商用だけ・3.2 商用は契約 |
| `yolp` | #3725 | **入れない**(個別の許可があれば可) | 対価を受ける目的での利用の禁止 |
| `tmdb` | #3726 | **入れない**(商用の契約があれば可) | 有料のアプリも LLM での利用も「商用」と定義 |
| `unsplash` | #3730 | 条件付き | 本番の承認・画像は返った URL のまま・撮影者の表示・壁紙や写真の閲覧のアプリは作らせない |
| `gitlab` | #3706 | 入れてよい | 連携の配布は許されている。連携の EULA を用意・大量の取得をしない |

`foursquare`(#3729)・`googleTranslate`(#3727)はこの調べに入れていない。

## 決定と食い違うもの

- [decisions/external-apis.md「規約で課金できない・商用が縛られるもの」](../decisions/external-apis.md#規約で課金できない商用が縛られるもの) は **Spotify・Open-Meteo・Strava を「入れない」** としている。今回、この 3 つの PR(#3714・#3716・#3715)が開いている。今回読んだ規約は、Spotify と Open-Meteo については決定と同じ向きだった。Strava については次のとおり。
  - 2026-06-01 施行の今の API Agreement で AI / ML の語は見つからなかった。
  - 「本人のデータは本人にだけ見せる」は残っている。
  - 決定を見直すかどうかはオーナーが決める。
- `zaim` は API そのものが 2026-12-07 に終わるので、PR #3710 をどうするか(閉じるなど)を決める必要がある。

## provider ごとの中身

各項目の順: 規約の URL / 商用 / 表示 / 回数の上限 / キャッシュ・保存 / AI / Tailor It と合わない所 / 推奨。

### zipcloud

- **URL**: [利用規約](https://zipcloud.ibsnet.co.jp/rule/api)・[API の説明](https://zipcloud.ibsnet.co.jp/doc/api)
- **商用**: 禁止の条項は無い。次の 2 つだけある。
  - 2 条 7 号「当社が、過度若しくは不適切と判断する商用目的の宣伝・広告行為」
  - 2 条 2 号「本APIにより提供される機能のみを提供することを目的とした対象サイトにおいて本APIを利用すること」
  - 無料かどうかはページに書いていない(**未確認**)。
- **表示**: 条項なし。
- **上限**: 数は無い。4 条 2 号「アクセス回数、アクセス時間の制限など」。
- **キャッシュ**: 禁止は無い。5 条 1 号「常に最新の本ウェブサイト内の情報を…表示することとします」。
- **AI**: なし。
- **合わない所**: 郵便番号を引くだけのアプリは 2 条 2 号に当たるおそれがある。1 条 2 号(いつでも取り消せる)・6 条(保証しない)。
- **推奨**: 入れてよい。郵便番号を引くのは機能の一部にとどめ、止まった時の代わりを用意する。

### qiita

- **URL**: [利用規約](https://qiita.com/terms)・[API v2](https://qiita.com/api/v2/docs)。API 向けの規約は見つからない。
- **商用**: API の条項は無い。
  - 7 条 3 項: 投稿された**コード**は「商用私用問わず無償で」使える。
  - 8 条 1 項 3 号: 記事の「著作権法第21条から第28条までにおいて定義される権利については、ユーザー又は第三者に留保されます」。
  - 18 条 2 項: 「私的使用の範囲を越えて」使うことは禁止。
- **表示**: 条項なし(Guideline・Design Guideline は**未読**)。
- **上限**: 認証なしは IP ごとに 60 回 / 時、認証ありは 1000 回 / 時。
- **キャッシュ・AI**: なし。
- **合わない所**: 本文を有料のアプリで出す・LLM に渡すと、書いた人の著作物を使うことになる。
- **推奨**: 条件付き。題・URL・タグ・いいねだけを出し、記事へリンクする。本文は LLM に渡さない。出す前に Guideline を読むか、Qiita に聞く。

### googleDrive

- **URL**:
  - [Google APIs Terms](https://developers.google.com/terms)
  - [API Services User Data Policy](https://developers.google.com/terms/api-services-user-data-policy)
  - [Workspace API User Data and Developer Policy](https://developers.google.com/workspace/workspace-api-user-data-developer-policy)
  - [Drive の scope](https://developers.google.com/workspace/drive/api/guides/api-specific-auth)
- **商用**: 全体の禁止は無い。
  - ToS 4(a)(1)「Sublicense an API for use by a third party」
  - 5(e)(2): 返った中身を売らない。
- **表示**: ToS 6(b)「display any attribution(s) required by Google as described in the documentation for the API」。Drive に決まった表示は見当たらない。
- **上限**: **未確認**。
- **キャッシュ**: ToS 5(e)(1) で、キャッシュの header より長く持たない・永続の写しを持たない。
- **利用者のデータ**:
  - Limited Use: 使うのは「providing or improving user-facing features that are prominent in the requesting application's user interface」のためだけ。
  - 移すのは「only with the user's consent」の時だけ。
  - ToS 5(c): 他の人に見せるのは「without explicit opt-in consent from that user」でない時だけ。
- **AI**: Workspace ポリシーが「Transferring, selling, or using user data to create, train, or improve a machine learning or artificial intelligence model」を禁止している。
- **scope の確認**: `drive.file` は「Non-sensitive scopes」で、「only require basic OAuth App Verification」。
- **推奨**: 入れてよい。条件:
  - 本人のデータは本人だけに見せる。
  - キャッシュは header に従う(今はキャッシュしない)。
  - 学習に使わない。LLM に渡すなら本人の機能のためだけにして、そのことを書いておく。
  - 基本の OAuth の確認を済ませる。

### wikipedia

- **URL**:
  - [API Usage Guidelines](https://foundation.wikimedia.org/wiki/Policy:Wikimedia_Foundation_API_Usage_Guidelines)
  - [Rate limits](https://www.mediawiki.org/wiki/Wikimedia_APIs/Rate_limits)
  - [Reusing Wikipedia content](https://en.wikipedia.org/wiki/Wikipedia:Reusing_Wikipedia_content)
- **商用**: 本文は「can be used free of charge for any purpose so long as licensing terms are met」。API そのものは「may not sublicense, lease, assign, or guarantee the availability or functionality」。
- **表示**: CC BY-SA 4.0。ページへのリンク(か URL)と、CC BY-SA で出ている旨の文とライセンスへのリンクが要る。要約した文にも share-alike がかかるおそれがある。
- **上限**: 2026 年の新しい上限で、試行中なので変わる。
  - 識別できない呼び出しは 10 回 / 分。
  - 決まりに合う UA の bot は 200 回 / 分。
  - 同時は「3 or fewer」。
  - UA に連絡先が要る。
- **キャッシュ**: 「follow the requirements of the content licenses when republishing downloaded or cached data」。
- **AI**: なし。大量の商用には Wikimedia Enterprise がある。
- **推奨**: 入れてよい。連絡先入りの UA を付け、生成アプリに記事へのリンクと CC BY-SA の表示を出し、キャッシュで 200 回 / 分の下に収める。

### microsoftTodo

- **URL**: [Microsoft APIs Terms of Use](https://learn.microsoft.com/en-us/legal/microsoft-apis/terms-of-use)(2025-10 更新)・[Graph throttling](https://learn.microsoft.com/en-us/graph/throttling-limits)
- **商用**: 3(a) で、連携を自分のアプリの中で使わせることが許されている。禁止:
  - 3(b)(14)「Redistribute or resell, or sublicense access to, the Microsoft APIs, any data obtained using the Microsoft APIs」
  - (12) 広告に使わない。
  - (11) 利用者が許した範囲を超えない。
  - (4) 要る分だけ写す。
- **表示**: なし。
- **上限**: アプリ全体で「130,000 requests per 10 seconds」。Outlook はメールボックスごとに 10,000 回 / 10 分・同時 4 本。今の todoTask の上限は**未確認**。
- **保存**: 5(b)〜(e)。
  - データを新しく保つ。
  - アプリを消す・退会した時に消す。
  - Microsoft と同じくらい守るプライバシーポリシーを持つ。
  - https://account.live.com/consent/Manage へのリンクを置く。
- **AI**: AI の条項は無い。5(a)「obtain additional consent if the processing changes」。
- **推奨**: 条件付き。プライバシーポリシーと退会の時に消す仕組みを先に用意する。LLM に渡すことを書いておく。データを売らない。

### zaim

- **URL**: [dev.zaim.net](https://dev.zaim.net/)・[終了のお知らせ](https://content.zaim.net/notices/show/2280)(このページはこの PR で別に開いて確かめた)
- **終了**: 「2026年12月7日（月）をもちまして、外部開発者向け機能「Zaim API」および開発者向け管理画面「Zaim Developers Center」の提供を終了することとなりました。」
  - 新しいアプリの登録は 2026-11-09 で締め切り。
  - 「後継のAPI や代替手段のご用意はございません。」
- **規約の本文**: 終わるので読んでいない。
- **推奨**: **入れない**。新しく登録できず、3 週間ほどで止まる。

### connpass

- **URL**: [API v2](https://connpass.com/about/api/v2/)(`/about/api/` から 302)・[API のヘルプ](https://help.connpass.com/api/)・[API 利用規約](https://help.connpass.com/api/api-term.html)
- **商用**: 無料の版では不可。
  - ヘルプ: 「業務目的でフリープラン用APIを利用することはできません。」
  - 規約 5 条の 2(2): 有料の版でも使い道は自分のイベントの運営・参加・学習などに限られる。
  - 5 条の 2(3): 「APIを利用して取得した情報を集約又は公開するなど、第三者向けのサービス又は機能として利用する場合、API利用者は事前に当社の承諾を得るものとします。」
- **表示**: なし。
- **上限**(openapi.json): フリーは 1 回 / 12 秒・100 回 / 日。有料の版はもっと多い。
- **保存**: 9 条(3) 求められたら消す。9 条(5) 目的の外に使わない。
- **AI**: なし(6 条の目的外の利用の禁止はある)。
- **推奨**: **入れない**。入れるなら、connpass の事前の承諾と業務向けの版の契約が先。

### jira

- **URL**: [Atlassian Developer Terms](https://developer.atlassian.com/platform/marketplace/atlassian-developer-terms/)(2025-12-01 施行)・[Rate limiting](https://developer.atlassian.com/cloud/jira/platform/rate-limiting/)。Developer Guidelines は 404。
- **商用・課金**: 8.3「you may not directly or indirectly charge end users for use of, or access to, the functionality of the Atlassian Apps」。続く「(but this does not limit you from charging a standard overall fee for your Marketplace Apps)」は Marketplace のアプリの話。**#1954 が言う条項はこれ**(Trello も同じ Atlassian の規約)。
- **表示**: 義務は無い。8.4「You may not suggest any affiliation with Atlassian」。
- **上限**: OAuth 3LO は点数で数える。
  - 既定は 65,000 点 / 時。
  - 瞬間は GET / POST 100 RPS。
  - OAuth 3LO にかかるのは 2026-03-02 から。
- **保存**: 規約に決まりは無い。9.2 で、エンドユーザーの同意を取り、本人が許した範囲だけを扱う。「You may not sell any End User Data」。
- **AI**: 語は無い。
- **合わない所**: Jira を Pro 限定にする・使うたびにコインを引くと、8.3 の「間接の課金」に当たるおそれ。6(d) で、利用者の操作から起きない呼び出しは禁止。
- **推奨**: 条件付き。Jira の op は全員が使える・コインを引かない形にする(#1954 の扱いはオーナーが決める)。LLM に渡すことはプライバシーポリシーに書いて同意を取る。

### dropbox

- **URL**: [Developer Terms](https://www.dropbox.com/developers/terms)・[Branding guide](https://docs.dropboxapi.com/dropbox-api/docs/developer-resources/branding-guide)・[Developer guide](https://docs.dropboxapi.com/dropbox-api/docs/developer-resources/developer-guide)
- **商用**: 禁止は無い。1.1 で、本番にするには承認が要り、「Dropbox may, in its sole discretion, deny these requests」。
- **開発中の上限**: 「once your app links 50 Dropbox users, you will have two weeks to apply for and receive production status approval」。開発中は 500 人まで。
- **表示**: 決まった文は無い。名前とロゴの決まり:
  - アプリの名前に「Dropbox」を入れない。
  - 名前を「drop」で始めない。
  - 5.1 で、Dropbox が作った・推していると思わせない。
- **上限**: 数は公開されていない。
- **保存**: 3.4「only access, store, transfer and process User Data in accordance with Users' instructions」。
- **AI**: なし。
- **推奨**: 条件付き。出す前に本番の承認を取る。プライバシーポリシーに LLM に渡すことを書く。

### spotify

- **URL**: [Developer Policy](https://developer.spotify.com/policy)・[Developer Terms](https://developer.spotify.com/terms)・[Quota modes](https://developer.spotify.com/documentation/web-api/concepts/quota-modes)
- **商用**: Policy IV.2「Except for the limited commercial uses for Non-Streaming SDAs (set out below) commercial uses are not permitted for SDAs」。Terms V.5「You will not sell any Spotify Content or other data obtained from Spotify」。
- **上限**:
  - 開発モードは「Up to 5 authenticated Spotify users」。
  - 拡張は「Spotify only accepts applications from organizations (not individuals)」で、「at least 250k MAUs」が要る。
- **キャッシュ**: Terms IV.3.1(a) で、Spotify の中身のデータベースを作らない。IV.3.2「Do not locally cache any Spotify Content, except as strictly necessary」。
- **AI**:
  - Policy III.14「Do not use the Spotify Platform or any Spotify Content to train a machine learning or AI model」。
  - III.13「Do not analyze the Spotify Content or the Spotify Service for any purpose」。
- **表示**: II.4.1(Spotify のマークで出所を示す)・II.4.2(Spotify へのリンクを付ける)。
- **推奨**: **入れない**。5 人しかつなげず、拡張の条件も満たせない。AI と課金の条項もぶつかる。決定とも同じ。

### strava

- **URL**: [API Agreement](https://www.strava.com/legal/api)(2026-06-01 施行)・[Rate limits](https://developers.strava.com/docs/rate-limits/)・[Guidelines](https://developers.strava.com/guidelines/)
- **商用**: 全体の禁止は無い。
  - 3.2: Strava は料金を取ることがある。
  - Strava の機能をまねる・競うアプリは禁止。
- **見せる相手**: 「Strava Data provided by a specific user can only be displayed or disclosed in your Developer Application to that user.」
- **表示**: 「Powered by Strava」か「Compatible with Strava」。リンクは「View on Strava」。アプリの名前に Strava を入れない。
- **上限**: 全体で 200 回 / 15 分・2,000 回 / 日。読む呼び出しは 100 回 / 15 分・1,000 回 / 日。
- **保存**: 4.4 で、終わった時に全部消す。
- **AI**: 今の規約には AI / ML / model / train の語が見つからなかった。2024 年の版は AI に使うことを禁止していた(決定 #1670 の理由)。**本当に外れたかは未確認**なので、Strava に書面で聞く。
- **推奨**: 条件付き。決定は「入れない」なので、見直すかはオーナーが決める。本人のデータを本人だけに見せる・表示を守る・AI を Strava に確かめる・上限の引き上げを頼む。

### openMeteo

- **URL**: [Terms](https://open-meteo.com/en/terms)・[Pricing](https://open-meteo.com/en/pricing)
- **商用**: 「You may only use the free API services for non-commercial purposes.」購読や広告のあるアプリは商用で、有料の API の版が要る。
- **表示**: CC BY 4.0(「giving appropriate credit and indicating any modifications」)。
- **上限**(無料): 1 日 10,000 回・1 時間 5,000 回・1 分 600 回・1 か月 300,000 回。
- **キャッシュ・AI**: なし。
- **推奨**: 条件付き。有料の API の版を契約し、CC BY 4.0 の表示を出すなら入れられる。無料のままでは入れない(決定と同じ)。

### backlog

- **URL**: [ヌーラボ API 利用規約](https://nulab.com/ja/terms/api/)・[ヌーラボの利用規約](https://nulab.com/ja/terms/)・[Rate limit](https://developer.nulab.com/docs/backlog/rate-limit/)。`developer.nulab.com/ja/terms/` は 404。
- **商用**:
  - 5 条 1 項「非独占的かつ無償で利用することを許諾します。」
  - 5 条 2 項: アプリやサービスの開発・提供に使ってよいが、「ヌーラボの提供するサービスと同一性のないものに限る」。
  - 有料のアプリの禁止は見当たらない。
- **表示**: 9 条 3 項で、ヌーラボに求められたら「ヌーラボAPIにより提供されたものであることを表示するもの」。商標はロゴガイドラインに従う。
- **上限**: 数はページに無い。ユーザーごとに数え、Get Rate Limit で取る。超えると 429。
- **保存**: 11 条 3 項 求められたら消す。11 条 5 項 目的の外に使わない。
- **AI**: なし(10 条 1 項 5 号 目的外の利用の禁止)。
- **推奨**: 条件付き。本人の OAuth で本人に見せる。消す求めに応え、429 に対処する。

### estat

- **URL**: [API 利用規約](https://www.e-stat.go.jp/api/agreement/)・[利用条件](https://www.e-stat.go.jp/terms-of-use)・[クレジット](https://www.e-stat.go.jp/api/api-info/credit)
- **商用**: 「商用利用も可能です。」CC BY 4.0 と互換。
- **表示**: API 利用規約 7 条で、出所を示すことが義務。
  - 文言: 「このサービスは、政府統計総合窓口(e-Stat)のAPI機能を使用していますが、サービスの内容は国によって保証されたものではありません。」
  - 出典: 「出典：政府統計の総合窓口(e-Stat)（https://www.e-stat.go.jp/）」
  - 加工した時はその旨を書く。
- **上限**: 数は無い。8 条 1 項(2)「短時間における大量のアクセス」の禁止。
- **キャッシュ・AI**: なし。
- **推奨**: 入れてよい。文言と出典を生成アプリに必ず出す。

### nominatim

- **URL**: [Nominatim Usage Policy](https://operations.osmfoundation.org/policies/nominatim/)
- **商用**: 禁止は無いが、「Commercial applications should keep that in mind when relying on this API for serving paying customers」。
- **表示**: 「Clearly display [attribution] … as suitable for your medium」。データは ODbL。正確な文は ODbL 側(**未確認**)。
- **上限**:
  - 「an absolute maximum of 1 request per second」。
  - 1 本ずつ(single thread)。
  - アプリが分かる User-Agent か Referer が要る。
  - 自動補完は禁止。
- **キャッシュ**: 「Results must be cached on your side」。
- **合わない所**:
  - runner 1 台から全利用者が呼ぶので、合計で 1 回 / 秒になる。
  - 頼まれたら、アプリを更新しなくても止められる必要がある。
  - 安定は保証されない。
- **推奨**: 条件付き(できれば使わない)。全体で 1 回 / 秒の列・キャッシュ・止める仕組み・表示を入れる。増えたら商用の geocoder か自前の Nominatim に移る前提。

### nhk

- **URL**: [NHK API ポータル](https://api-portal.nhk.or.jp/)。JavaScript で描くページで、規約の本文を取れなかった。
- **わかったこと**: 2013 年の報道([AV Watch](https://av.watch.impress.co.jp/docs/news/602497.html))の「利用は無料」「NHKの指定するクレジットを表示するなどの利用条件」だけ。
- **未確認**: 営利の禁止・上限・キャッシュ・AI。
- **推奨**: **入れない**(保留)。人がブラウザで規約の本文を読むまで。営利の禁止があれば Tailor It の課金と合わない。

### lastfm

- **URL**: [API Terms of Service](https://www.last.fm/api/tos)
- **商用**:
  - 3.1「You are permitted to use the Last.fm Data solely for non-commercial purposes」。
  - 3.2「If at any time you wish to use the Last.fm Data for commercial purposes, you must apply for a commercial use agreement.」
  - 「Last.fm reserves the right to share in revenue generated from your use of Last.fm Data」。
- **表示**: Last.fm の表示とリンク。2.7「powered by AudioScrobbler」のボタン。置き場所は Last.fm の承認が要る。
- **上限**: 4.4 に上限があるとだけ書いてある(数は**未確認**)。
- **キャッシュ**: 4.3.4 で、HTTP header に従ってキャッシュすることが**義務**。100 MB まで。
- **AI**: なし。
- **推奨**: **入れない**。商用の契約を結んだら入れられる。

### yolp

- **URL**: [ローカルサーチ API](https://developer.yahoo.co.jp/webapi/map/openlocalplatform/v1/localsearch.html)・[ガイドライン](https://developer.yahoo.co.jp/guideline/)・[クレジット](https://developer.yahoo.co.jp/attribution/)・[サポート記事](https://support.yahoo-net.jp/PccDeveloper/s/article/H000011080)・[LINEヤフーの規約](https://www.lycorp.co.jp/ja/company/terms/)
- **商用**: 原則は不可。
  - サポート記事: 「利用者自身の便宜をはかる非商用目的のみに使用することが認められています」。法人は窓口に相談。
  - 規約 第 2 章 3(3): 「対価を受ける目的でソフトウェアまたは開発ソフトウェアを自ら利用し、または第三者に利用させること」の禁止。
- **表示**: 「Web Services by Yahoo! JAPAN」などのクレジットを、アプリ(か、ストアのページ)の下部に置く。
- **上限**: 「24時間中1アプリケーションにつき50000件」。
- **キャッシュ**: 決まった条文は無い(第 1 章 14 条 提供目的を超えない)。
- **AI**: なし。
- **推奨**: **入れない**。LINEヤフーから個別の許可を取れば入れられる。

### tmdb

- **URL**: [API Terms of Use](https://www.themoviedb.org/api-terms-of-use)・[FAQ](https://developer.themoviedb.org/docs/faq)・[Rate limiting](https://developer.themoviedb.org/docs/rate-limiting)
- **商用**: 2.A「does not permit any commercial use of TMDB… only permitted under a separate written agreement」。商用の例に次の 2 つがある。
  - 「Charging users a fee for Your Application… that includes some form of integration」
  - 「large language model (LLM), artificial intelligence, or any other machine learning based interactive query-response systems or chatbots」
- **表示**: TMDB のロゴと「This product uses the TMDB API but is not endorsed or certified by TMDB.」。
- **上限**: およそ 40 回 / 秒(変わる)。
- **キャッシュ**: 1.C で 6 か月まで。1.D で、終わる時に消す。
- **AI**: 1.C で、ML / AI のアプリの学習に使うことを禁止。
- **推奨**: **入れない**。TMDB と商用の契約を結んだら入れられる。Tailor It は有料のアプリで、LLM も使うので、2 つの理由で商用に当たる。

### unsplash

- **URL**: [API Guidelines](https://help.unsplash.com/en/articles/2511245-unsplash-api-guidelines)・[API Terms](https://unsplash.com/api-terms)・[Documentation](https://unsplash.com/documentation)
- **商用**: 3 で、使えるのは「solely to develop and operate your Developer Apps」。Guidelines「You cannot use the API to sell unaltered Unsplash photos directly or indirectly」。
- **合わない所**: 「You cannot replicate the core user experience of Unsplash (unofficial clients, wallpaper applications, etc.)」。
- **表示**: 9 で、写真ごとに Unsplash と撮影者を表示し、撮影者のプロフィールにリンクする。
- **上限**: デモは 50 回 / 時、本番の承認の後は 1000 回 / 時。
- **キャッシュ**:
  - 「All API uses must use the hotlinked image URLs」。
  - ダウンロードに当たる操作では `download_location` を呼ぶ。
  - 画像を自分で保存しない。
- **AI**: 12 は unsplash.com/data(データセット)を案内しているだけ(**未確定**)。
- **推奨**: 条件付き。
  - 本番の承認を取る。
  - 画像は返った URL のまま出す。
  - 撮影者の表示を生成アプリに必ず入れる。
  - 壁紙・写真の閲覧が中心のアプリは作らせない。

### gitlab

- **URL**: [GitLab API Terms of Use](https://handbook.gitlab.com/handbook/legal/api-terms/)・[GitLab.com rate limits](https://docs.gitlab.com/user/gitlab_com/rate_limits/)・[GitLab terms](https://about.gitlab.com/terms/)
- **商用**: 1.1.1「Develop, test and support any application or integration」と 1.1.2「Distribute, or allow third parties to access, your Integration」が許されている。商用の縛りは無い。
- **表示**: 義務は無い。1.3.4 で表示を消さない。商標は Trademark Guidelines に従う。
- **上限**:
  - 今は、認証ありがユーザーごとに 2,000 回 / 分、認証なしが IP ごとに 500 回 / 分。
  - Free の版について、もっと低い上限が予告されている。
- **保存**: 1.3.9 で、大量の取得・scraping の禁止。
- **AI**: なし。
- **その他**: 1.3.10 で、連携のための EULA が要る。
- **推奨**: 入れてよい。連携の EULA(Tailor It の利用規約で足りるかは人が確かめる)を用意し、大量の取得をしない。

## 全体で効くこと

- **LLM に渡す**: はっきり縛るのは次の 3 つ。どの規約でも、学習には使わない。
  - Spotify(分析の禁止)
  - TMDB(LLM の利用は商用)
  - Google Workspace(学習の禁止)
- **目的の外に使わない**: connpass・backlog・yolp・Microsoft は「目的の外」「許した範囲」を書いている。LLM に渡すことをプライバシーポリシーに書き、同意を取る形にそろえるとよい。
- **鍵が 1 本**: 運営の鍵 1 本で全員が呼ぶので、次の上限は利用者が増えると足りなくなる。
  - Nominatim 1 回 / 秒
  - connpass フリー 100 回 / 日
  - Strava 1,000 回 / 日
  - Unsplash 1000 回 / 時
  - yolp 50,000 件 / 日
- **本人のデータは本人に**: Strava・Google・Microsoft・Atlassian・Dropbox・ヌーラボは、OAuth のデータを他の人に見せる・共有するアプリの形とぶつかる。生成アプリの共有の時に OAuth のデータが混ざらないか、作りで確かめる。
- **#1954 に当たる条項**: 今回はっきりあったのは Atlassian(Jira)8.3。Spotify は商用の利用を限っている。

## 未確認の所

- NHK の規約の本文(ページを開けない。人がブラウザで読む)
- Qiita の Guideline・Design Guideline
- Google Drive の上限の数・Microsoft To Do(todoTask)の上限
- Dropbox の上限の数・Last.fm の上限の数
- Strava の AI の禁止が今の規約で本当に無くなったか
- Unsplash の API を AI に使うことの決まり
- Nominatim(ODbL)の正確な表示の文
- Atlassian Developer Guidelines の本文(404)
- zipcloud が無料かどうか
