# entity と記録の読み書き(query・unique・marks の入口)(#2514)

> [バックエンド設計](../architecture.md) の機能の文書。形の正は [openapi-appconfig.yaml](../openapi-appconfig.yaml) の tag `entities` と `records`。共有の記録(`scope: shared` / `public`)の置き場と権限は [shared-data.md](shared-data.md)、参加者は [members.md](members.md)、DSL の書き方は [docs/dsl/README.md](../../dsl/README.md)。

## 目次

- [entity と記録](#entity-と記録)
- [API](#api)
- [作る・直す・消す](#作る直す消す)
- [一覧と query の書き方](#一覧と-query-の書き方)
- [重複させない項目(unique)](#重複させない項目unique)
- [印(marks)と共有の記録](#印marksと共有の記録)
- [失敗のコード](#失敗のコード)
- [コードの場所](#コードの場所)
- [決めること](#決めること)

## entity と記録

- **entity の定義の正は DSL の `entities[]`**。entity のドキュメントは作らない。API の entity は今の DSL の読み出しで、変えるのは DSL の保存(`PUT` / `PATCH /dsl`)。
- 記録は `users/{uid}/apps/{appId}/entities/{entityId}/records/{recordId}`(private)。共有の entity は `apps_public/{appId}/entities/{entityId}/records/{recordId}`([shared-data.md](shared-data.md))。
- 書き込みのたびに、呼び出した人の今の DSL の項目の定義で `data` を検証して形をそろえる(`currency` / `integer` は整数、`date` は `YYYY-MM-DD`、`datetime` は RFC 3339)。
- DSL で項目や entity を消しても記録は消さない(スキーマレス。版を戻せば読める)。
- 今の DSL に無い entityId は 404(`entity "<id>" not found in current dsl`)。

## API

| API | すること |
|---|---|
| `GET /v1/apps/{appId}/entities` | 今の DSL の `entities[]` と `version` |
| `GET /v1/apps/{appId}/entities/{entityId}` | 1 つの entity の定義 |
| `GET /v1/apps/{appId}/entities/{entityId}/records` | 一覧。filter の無いページ送り・並べ替えに使う |
| `POST /v1/apps/{appId}/entities/{entityId}/records/query` | 一覧(GET と同じ)。filter・並べ替え・ページを本文で送る |
| `POST /v1/apps/{appId}/entities/{entityId}/records` | 作る(201) |
| `GET /v1/apps/{appId}/entities/{entityId}/records/{recordId}` | 1 件 |
| `PATCH /v1/apps/{appId}/entities/{entityId}/records/{recordId}` | 直す(`data` のキーごとに重ねる) |
| `DELETE /v1/apps/{appId}/entities/{entityId}/records/{recordId}` | 1 件消す(204) |
| `DELETE /v1/apps/{appId}/entities/{entityId}/records?confirm=true` | entity の記録を全部消す(デモのやり直し用・`confirm=true` が要る) |

## 作る・直す・消す

**作る**(`POST .../records`・本文 `{data, clientId?, authorName?}`)

- `data` は必須。今の DSL に無いキーは 400、`required` の項目が無い・型が違うと 422 `invalid_record`(`details.field`)。1 件の `data` は 300 KB まで(写真の data URL を含む・超えると 413)。
- `clientId`(256 文字まで)は冪等のキー。同じ `clientId` の送り直しは今ある記録を返す。共有の entity では `uid/clientId` から id を作るので、他の人の `clientId` とは重ならない。
- 共有の entity ではサーバーが `author`(uid)と `authorName`(本文の値・無ければ「匿名」・64 文字まで)を付ける。`data` に `author` は入れられない。
- `entity.single` の entity(設定値の置き場)は 1 人 1 件。もう一度作ると中身を上書きして 200 を返す。
- 権限(Pro・参加者・管理者だけの項目・宛先)を先に確かめ、言葉・大きさ・プランの上限・`unique` はその後。権限で断る時は 422 より先に 403 を返す。

**直す**(`PATCH .../records/{recordId}`・本文 `{data}`)

- `data` は 1 つ以上のキー。キーごとに今の記録に重ね、重ねた後の全体で `required` と大きさを確かめる。
- 共有の entity は作った人だけが直せる(他の人は 403)。管理者は他の人の記録でも `editableBy: admins` の項目だけ変えられる。

**消す**(`DELETE .../records/{recordId}`)

- 共有の entity は既定で作った人だけ。`deleteBy: admins` なら管理者だけ、`authorOrAdmins` なら作った人か管理者(それ以外は 403 `admins_only`)。
- 全部消す(`?confirm=true`)は、共有の entity では自分の記録だけ消す。`deleteBy` が `admins` / `authorOrAdmins` で管理者なら全員の分を消す。

## 一覧と query の書き方

GET と `POST .../records/query` は同じ一覧を返す。filter の値(uid など)を URL に載せないため、**filter は本文で送る**(GET の filter は古い iOS のために残している)。

```json
{ "filterField": "author", "filterOp": "neq", "filterValue": "<uid>", "sortField": "date", "sortDir": "desc", "pageSize": 100 }
```

| キー | 意味 |
|---|---|
| `filterField` | 項目の id。共有の entity では `author` も使える |
| `filterOp` | `eq`・`neq`・`gt`・`gte`・`lt`・`lte`・`thisMonth`・`today`(`thisMonth` / `today` は date / datetime の項目だけ)。`in` / `nin` は `filterField=author`・`filterValue=myFollows`(フォローしている人の記録・[follows.md](follows.md))だけ |
| `filterValue` | 比べる値(文字列) |
| `sortField` | `createdAt`(既定)・`updatedAt`・項目の id |
| `sortDir` | `asc` / `desc`(既定 `desc`) |
| `pageSize` / `pageToken` | 既定 50・最大 200(超えた値は 200 に丸める)。`pageToken` は前の応答の `nextPageToken` |

- 本文は 4 KiB まで(超えると 413)。知らないキー・型の違い・クエリ文字列付きは 400。`{}` は filter なし・既定の並び。
- 1 つの entity の記録を最大 2000 件まで読んでからメモリで絞って並べる。2000 件を超えると 413(`too many records to sort in memory`)。
- 暗号化する項目(`encrypted`)では絞れず並べられない(400)。
- private の entity でも `filterField=author` の `eq` / `neq` を受ける(記録は全部自分のものなので、`eq <自分>` は全件・`neq <自分>` は 0 件)。
- 共有の entity では、ブロックの関係にある人の記録・自分が宛先でない記録・年齢の申告が無い人への仲間探しの記録を外す。

## 重複させない項目(unique)

DSL の `entity.unique` に 1〜3 つの項目の id(または `$author` = 1 人 1 件)を書くと、その値の組が同じ記録を 2 つ作れない([docs/dsl/README.md §3.13](../../dsl/README.md#313-管理者の権限と重複の防止272-273m4))。

- 作る・直すで、同じ値の組の記録が他にあれば **409 `duplicate_record`**(`details.reason: duplicate`・`details.fields`: `unique` の項目の一覧)。端末は「もうあります」を出す。
- 値の組をハッシュにして、記録の書き込みと**同じトランザクション**で `entities/{e}/unique/{key}` を取る。同時に 2 つ作っても片方だけが通る。
- 値が空の項目がある記録は比べない(何も取らない)。`1` と `1.0` は同じとみなす。
- `unique` を付ける前の記録は鍵を持たないので、取る時に同じ値の記録も探す。
- 直す時に `unique` の値が変わらなければ、今の鍵のまま(前からある記録を毎回比べ直さない)。消すと鍵を放す。
- DSL の決まり: 使える型は string・integer・number・currency・boolean・date・datetime・enum・ref・member。`single` と仲間探し(`match`)の entity には付けられない。`$author` は共有の entity だけ。宛先つきの entity は `$author` を含める。

## 印(marks)と共有の記録

- 印(いいね・保存)は `PUT` / `DELETE .../records/{recordId}/marks/{markId}`。共有の entity で `entity.marks` に書いた印だけ・1 人 1 回。件数は記録の `markCounts`。詳しくは [shared-data.md](shared-data.md)。
- 共有の記録の置き場・作った人だけが直せること・作成者の公開した DSL の共有の決まりを使うことも [shared-data.md](shared-data.md)。参加者と管理者は [members.md](members.md)。

## 失敗のコード

| 状態 | code | いつ |
|---|---|---|
| 400 | `bad_request` | `data` が無い・知らないキー・`clientId` / `authorName` が長すぎる・filter / sort / ページが不正・暗号化した項目で絞る / 並べる・`confirm=true` が無い |
| 402 | `pro_required` | 共有の記録の書き込み(作る・直す・印)で、アプリの作成者が Pro でない([pro.md](pro.md)) |
| 403 | `forbidden` | 共有の entity で他の人の記録を直す・消す。`details.reason: admins_only` は管理者だけの操作、`details.reason: not_member` は shared の entity で参加者でない |
| 403 | `unpublished` | 運営が非公開にした共有のアプリ |
| 403 | `app_stopped` | 運営がサーバーの機能を止めている間の、自動の動きのきっかけになる作る・直す |
| 403 | `plan_limit_reached` | アプリごとの記録・写真の数の上限([rate-limit.md](../rate-limit.md#プランごとの上限317)) |
| 404 | `not_found` | アプリ・entity・記録が無い。ブロックの関係にある人の記録 |
| 409 | `duplicate_record` | `unique` の値が他の記録と同じ |
| 413 | `payload_too_large` | `data` が 300 KB を超える・query の本文が 4 KiB を超える・記録が 2000 件を超えて並べられない |
| 422 | `invalid_record` | `required` が無い・型が違う・`member` の値が参加者でない |
| 422 | `text_not_allowed` | 共有の記録の文字・投稿者の名前に表示できない言葉がある(`details.field`) |

## コードの場所

`backend/internal/appconfig/` の下。

| ファイル | 中身 |
|---|---|
| `handler.go` | 経路の登録 |
| `dsl.go` | entity の一覧・1 件(`listEntities` / `getEntity`)・entity の解決(`entityOf` / `resolveEntity`) |
| `records.go` | 一覧・query・作る・1 件・直す・消す・全部消す |
| `recorddata.go` | `data` の検証と形をそろえる(`normalizeData`) |
| `data_rules.go` | 管理者だけの項目・`deleteBy`・`unique`(`uniqueClaim` / `createRecordKept` / `updateRecordKept`) |
| `record_scope.go` | 共有の決まりを作成者の公開した DSL から読む |
| `marks.go` | 印 |
| `follows.go` | `myFollows` の絞り込み |
| `encrypted.go` | 暗号化した項目の絞り込みを断る |
| `textfilter.go` | 共有の記録の言葉の確かめ |
| `plan_limits.go` | 記録・写真の数の上限 |

`unique` の鍵の取り方は `backend/internal/store/unique.go`。DSL の決まりは `backend/pkg/dsl/data_rules.go`。

## 決めること

- openapi の `PUT /apps/{appId}/entities/{entityId}`(entity の定義を DSL の新しい版として保存する)は、コードに経路が無い(`GET` だけ)。openapi から消すか、作るか。
- openapi の GET `.../records` は「複合インデックスが無い組み合わせはメモリで並べる」と書くが、コードはいつも 2000 件まで読んでメモリで絞って並べる。インデックスを使う形に戻すか、書き方を合わせるか。
