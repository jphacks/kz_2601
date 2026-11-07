# ISBN から本の情報を入れる(lookupBook)

> 状態: 実装(2026-11-08)。外部の公開データ(#348)と同じ安全の作りで、**鍵の要らない決まった出どころ 1 つ(openBD)だけ**を使う。出どころを増やすのは人の判断(#339)。
> 目的: 過去の JPHACKS 作品 NG_2312 LenBow(本の貸し借り)の「書籍情報の自動取得」を作れるようにする([jphacks-coverage.md](../../research/jphacks-coverage.md))。
> コード: `backend/pkg/dsl/book_lookup.go`(DSL の検証)・`backend/internal/books`(ISBN の確かめ・取得・キャッシュ)・`backend/internal/appconfig/books.go`(API)・`backend/internal/ai/book_rules.go`(AI の規則)・iOS `BookLookupPlan`(MetamonDSL)/ `HTTPBookLookupClient`(MetamonServices)/ `AppRuntime+Books`。

## 目次

1. [できること](#できること)
2. [DSL の形](#dsl-の形)
3. [出どころと規約](#出どころと規約)
4. [安全の作り](#安全の作り)
5. [API と環境変数](#api-と環境変数)
6. [iOS](#ios)
7. [試験](#試験)
8. [人が確かめること](#人が確かめること)

## できること

- 本の記録フォームで ISBN(10 桁・13 桁。本のバーコードを `qrScanner` で読むか手で入れる)を入れてボタンを押すと、**書名・著者・出版社・出版日**がフォームに入る。保存はしない(保存は `saveRecord`)。
- 入るのは文字だけ。書影(表紙の画像)・あらすじ・値段は入れない(API は書影の URL を返すが、iOS は今は使わない)。
- 見つからない本・つながらない時はメッセージだけ出して、手入力に戻る。

## DSL の形

```json
{"type": "button", "label": "ISBN から入力", "action": {"type": "lookupBook",
  "bookLookup": {"isbnField": "isbn", "titleField": "title", "authorsField": "author",
                 "publisherField": "publisher", "publishedDateField": "published_on"}}}
```

| キー | 中身 |
|---|---|
| `isbnField` | 必須。フォームの entity(`screen.entity`)の string field |
| `titleField` / `authorsField` / `publisherField` | string / text の field。entity にある分だけ |
| `publishedDateField` | string / text(「2014-04-08」「2014-04」「2014」の形)か date(足りない月日は 1 日にする) |

- 置けるのは `screen.entity` のあるフォーム画面の button / toolbar だけ(入力中のフォームを埋めるため)。`list.onItemTap` などのタップには置けない。
- 埋める field は最低 1 つ。同じ field を 2 回使えない・ISBN の field を埋め先にできない・computed は不可。
- 新しい部品ではなく **Action 1 つ + 中身のオブジェクト 1 つ**にした(閉じた語彙で一番小さい)。ボタンの見た目・`confirmMessage`・toolbar をそのまま使え、古い iOS は「未対応のアクション」の toast で済む。DSL に出どころの URL・名前は書かない。

## 出どころと規約

2026-11-08 に確かめた。鍵は要らない。

| 出どころ | URL(サーバーが取りに行く) | 規約 | 出典の表示 |
|---|---|---|---|
| openBD | `https://api.openbd.jp/v1/get?isbn={13 桁}` | [openBD](https://openbd.jp/) の利用条件: 書誌・書影などすべての情報は**本の販促・紹介の目的に限り**使える(個人・団体・企業を問わない)。データを勝手に改変しない。キャッシュする時は変更をできるだけ早く反映する。削除の要請には速やかに従う。2023-08 から書誌は国立国会図書館の書誌データ(CC BY 4.0)に切り替えている([お知らせ](https://openbd.jp/news/20230829.html))。v1 は「技術的な調整が済み次第終える」と告知されている(日付は未定・[お知らせ](https://openbd.jp/news/20230725.html)。代わりの API は仕様を変えずに 60 か月以上続ける予定) | 「出典: openBD」を toast に出す。API の `source.license` に「openBD の利用条件(本の紹介のため)・書誌は国立国会図書館の書誌データ(CC BY 4.0)を含む」 |

改変の扱い: 値は出どころのまま(制御文字・見えない文字を除き、長さで切るだけ)。出版日だけ `20140408` を `2014-04-08` の形に書き直す。

外したもの:

| 候補 | 外した理由 |
|---|---|
| Google Books API | 日本の本の収録と、鍵なしで使う時の上限・規約([Google APIs Terms of Service](https://developers.google.com/terms))をまだ確かめていない。最初は 1 つに絞った |
| 国立国会図書館サーチの API | 書誌は CC BY 4.0 で使えるが、応答が XML(OpenSearch / SRU)で形が大きい。openBD が同じ書誌を JSON で返すので先に openBD にした。openBD v1 が終わる時の第一の代わり |
| 書店の API(楽天ブックスなど) | 鍵(アプリ ID)が要る |

出どころを増やす・替える時: `backend/internal/books/books.go` の `DefaultEndpoint` と解析を足し、**規約(目的の制限・再配布・出典)を確かめてこの表に URL を書いてから**。追加・切り替えは人の判断(#339)。

## 安全の作り

[external-feeds.md の「安全の作り」](external-feeds.md#安全の作り339-の条件との対応) と同じ条件を、次のように満たす。

| 条件 | この実装 |
|---|---|
| 宛先は許可リストだけ | 取りに行く URL はコードで固定。リクエストから受け取るのは `isbn` だけで、**桁と検査数字を確かめてから**(ISBN-10 は 13 桁に直す・ISBN-13 は 978 / 979 始まり)13 桁の数字だけを URL に入れる。正しくない ISBN・他のパラメータ・重複は通信の前に 400 |
| 内部のアドレスに届かない | feeds の専用のクライアント(`feeds.NewClient`)を使う。接続の直前に実際の接続先のアドレスを確かめ、公開のユニキャスト以外には接続しない。リダイレクトは同じ scheme + ホストだけ・3 回まで |
| 送る項目を最小にする | 送るのは ISBN だけ。uid・記録の値は送らない |
| 量の上限 | 8 秒で打ち切り・本文 256 KiB まで。書名 300・著者 300・出版社 120 文字で切る。キャッシュは ISBN ごと(見つかった 1 日・無かった 6 時間・失敗 1 分)で最大 1 万件。同じ ISBN の同時の取得は 1 回にまとめる。出どころへはインスタンスあたり 1 分 30 回まで・利用者ごとに 1 時間 60 回まで |
| 止められる | `BOOK_LOOKUP_DISABLED=1`(または `FEEDS_DISABLED=all`)で 503。iOS は「本の情報を読み込めません。手で入力してください」 |
| 応答の確かめ | 出どころの記録の ISBN が問い合わせと違えば捨てる。書影の URL は `https://cover.openbd.jp/` だけ残す |

## API と環境変数

`GET /v1/lookup/book?isbn=978-4-7808-0204-7` → `{isbn, title, authors?, publisher?, publishedDate?, coverUrl?, source{name,url,license}, fetchedAt}`(`docs/backend/openapi-appconfig.yaml` の `/lookup/book`)。404 = 見つからない・429 = 利用者の上限・503 = 止めている / つながらない。

| 環境変数 | 中身 | 既定 |
|---|---|---|
| `BOOK_LOOKUP_DISABLED` | `1` / `true` で止める | 止めない |
| `BOOK_LOOKUP_USER_PER_HOUR` | 利用者ごとの 1 時間の回数 | 60 |
| `BOOK_LOOKUP_UPSTREAM_PER_MINUTE` | インスタンスあたりの出どころへの 1 分の回数 | 30 |

## iOS

- ボタンを押すと入力中の文字を確定させ、ISBN を端末でも確かめてから appconfig に聞く(端末は外部へ直接つながらない)。待つ間はボタンにくるくるを出し、二度押しを止める。
- 見つかった値は、entity にある・型の合う field だけを上書きする(`BookLookupPlan.values`・純関数)。ISBN の field は 13 桁に揃える。
- 本のバーコード(EAN-13)は今の `qrScanner` で読める(読んだ文字が ISBN の field に入る)。読んだ後に自動で引く動きは入れていない(ボタンを押す)。
- 審査用の版にも残す。書誌の文字をフォームに入れるだけで、権限も利用者の情報の送信も無い。

## 試験

- Go: `internal/books`(ISBN の検査数字・記録した openBD の応答(`testdata/`)の解析・通信の前に断る・キャッシュ・同時の取得のまとめ・上限・止める・内部のアドレス・リダイレクト)、`internal/appconfig/books_test.go`(API の 200 / 400 / 401 / 404 / 429 / 503)、`pkg/dsl/book_lookup_test.go`(置き場所・field の型)。テストは偽の出どころ(httptest)と記録した応答だけで、実際の openBD には出ない。
- iOS: `BookLookupPlanTests`(ISBN・埋める値・出版日)、`BookLookupClientTests`(`v1/lookup/book` だけに聞く)。
- AI: `book_rules_test.go`、ai-eval の `lookupBook`(`internal/ai/eval/book_cases.json`・`jphacks_coverage_cases.json` の `cov_book_lending`)。

## 人が確かめること

- openBD の「本の販促・紹介の目的に限る」に、蔵書・貸し借り・読書記録のアプリでフォームに書名を入れる使い方が当たるか(当たらないなら止める: `BOOK_LOOKUP_DISABLED=1`)。
- 出典の出し方(今は toast の「出典: openBD」だけ)。国立国会図書館の CC BY 4.0 の表示を記録の画面にも出すか。
- openBD v1 が終わる時の切り替え先(国立国会図書館サーチなど)。
- プライバシーポリシー・App Privacy の書き方(ISBN だけを外部へ送る)。
