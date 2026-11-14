# 公開されたアプリ「探す」: 公開・探す・検索・入れる・隠す(#50・#153・#154・#321)

> [バックエンド設計](../architecture.md) から分けた機能の文書。シェアリンクと共通の決まり(複製のしかた・作成者の決め方・Web の共有ページ)は [sharing.md](sharing.md)、通報・ブロック・運営による非公開化の手順は [moderation.md](../../infra/moderation.md)。API は [openapi-appconfig.yaml](../openapi-appconfig.yaml) の tag `sharing`(`/apps/{appId}/publish`・`/public-apps`・`/public-apps/search`・`/public-apps/{id}/install`)。コードは `backend/internal/appconfig/share.go`・`backend/internal/store/firestore_share.go`。

## 目次
- [1. API](#1-api)
- [2. 保存する物](#2-保存する物)
- [3. 公開の流れ](#3-公開の流れ)
- [4. 検索の仕組み](#4-検索の仕組み)
- [5. 入れる・入れた数](#5-入れる入れた数)
- [6. 隠す・消える時](#6-隠す消える時)
- [7. 端末](#7-端末)
- [8. 決めること](#8-決めること)

## 1. API

| API | 誰が | 中身 |
|---|---|---|
| `DELETE /v1/apps/{appId}/publish` | 掲載の持ち主(`ownerUid`) | 取り下げ → 204 |
| `GET /v1/public-apps?q=&sort=&pageSize=&pageToken=` | 認証済みなら誰でも | 古い iOS の版のために残している。新しい呼び出しは下の `search` |
| `POST /v1/apps/{appId}/publish` | その appId の作成者(`app_owners`・#153) | `{name?, description?, tags?, authorName?}` で掲載を作る・更新する → 200 で掲載 |
| `POST /v1/public-apps/search` | 認証済みなら誰でも | `{q, sort, pageSize, pageToken}` を本文で送る(検索語を URL に載せない)→ `{items, nextPageToken?}` |
| `POST /v1/public-apps/{id}/install` | 認証済みなら誰でも | 自分の木に入れる → 201 で `{...アプリ, dsl, joined, created, mine}` |

`id` は掲載者の appId。

## 2. 保存する物

| 置き場 | 中身 |
|---|---|
| `app_owners/{appId}` | `{uid, createdAt}`。appId の作成者(公開できる人)。決め方は [sharing.md](sharing.md) の「公開」 |
| `public_apps/{appId}` | `{ownerUid, name, description, tags[], authorName, components[], dslVersion, dslJson, installs, createdAt, updatedAt}`(+ 運営が隠した時の `hidden`・`hiddenReason`) |
| `public_apps/{appId}/installs/{uid}` | `{appId(その人の複製先), createdAt}`。入れた数を uid ごとに 1 回だけ数える印 |

`public_apps`(探すの掲載)と `apps_public`(共有の記録の置き場・[shared-data.md](shared-data.md))は別物。

## 3. 公開の流れ

1. 端末が `POST /v1/apps/{appId}/publish` を呼ぶ。作成者でなければ 403、中身が初期の DSL のまま(空のアプリ)は 400
2. 空の欄は DSL から埋める: `name` = アプリの名前・`description` = アプリの説明・`authorName` = 「匿名」
3. タグは前後の空白と先頭の `#` を落とし、空は捨てる
4. 長さを確かめる(超えたら 400)
5. 他の人が見る文字(名前・説明・作者名・タグ)を `moderation/textfilter` に通す(#321)
6. 今の DSL を**スナップショット**して保存する(`dslJson`・`dslVersion`)。後でアプリを変えても掲載は変わらない。もう一度 publish すると今の DSL で上書きする
7. 使っている部品の type を `components` に入れる(カードのアイコン用)
8. 指標 `app.published` を出す

| 欄 | 上限 |
|---|---|
| `authorName` | 64 文字 |
| `description` | 300 文字 |
| `name` | 60 文字 |
| `tags` | 8 個・1 個 20 文字 |

## 4. 検索の仕組み

- 掲載を `installs` の多い順に最大 500 件(`store.MaxCatalogApps`)読み、メモリの中で絞る。全文検索の索引も複合インデックスも使わない(掲載が少ない前提)
- `q` は小文字にして、名前・説明・作者名・タグのどれかに部分一致すれば当たり
- 除くもの: 運営が隠した掲載(本人の分は出す)・自分がブロックした人の掲載(#321)
- `sort`: `popular`(既定・入れた数の多い順 → 更新の新しい順)/ `new`(更新の新しい順)。それ以外は 400
- ページは `pageSize` と `pageToken`(他の一覧と同じ `pagingFrom`)
- 返す掲載: `id`・`name`・`description`・`tags`・`authorName`・`components`・`dslVersion`・`installs`・`mine`・`createdAt`・`updatedAt`。`hidden`・`hiddenReason` は本人の掲載にだけ付く。DSL は返さない

## 5. 入れる・入れた数

- 隠された掲載は 403 `unpublished`
- 2 回目からは `installs/{uid}` を見て、最初に作った複製を返す(`created: false`・アーカイブしていたら active に戻す)。複製を消していたら新しく作る
- 複製は [sharing.md](sharing.md) の `installCopy`: version 1(初期)→ version 2(掲載の DSL)で作り、**記録は複製しない**
- 共有のデータの扱いはシェアリンクと違う: 「探す」から入れる時は scope `public` の entity がある時だけ元の appId に参加する(`joined: true`)。scope `shared` だけのアプリは、招かれた人のためのデータなので新しい appId の独立した複製になり、入れた人がその作成者になる
- アプリの数の上限に当たる時は 402 `app_limit_reached`([rate-limit.md の「持てるアプリの数」](../rate-limit.md#持てるアプリの数630))
- 入れた数(`installs`)は uid ごとに 1 回だけ、印と同じトランザクションで +1。掲載者本人と、もう参加している人の入れ直しは数えない(#154)。数え損ねてもログ `appconfig: install not recorded` だけで、入れること自体は成功させる
- 指標 `share.imported`(`source=public`・`joined`・`created`)を出す

## 6. 隠す・消える時

| 時 | 何が起きるか |
|---|---|
| アカウントの削除(`DELETE /v1/me`・論理削除) | その人の掲載を取り下げる。30 日後の消去で掲載(`installs` ごと)と、他の掲載にあるその人の `installs/{uid}` を消す([account-deletion.md](account-deletion.md)) |
| 運営が隠す(`cmd/moderate`) | `hidden`・`hiddenReason`。本人以外の検索から消え、入れると 403。本人には理由を見せる。手順は [moderation.md](../../infra/moderation.md) |
| 掲載者が取り下げる | `DELETE /v1/apps/{appId}/publish`。もう入れた人の複製は残る |
| 利用者がブロックする | その人の掲載がブロックした人の検索から消える(`/v1/me/blocks`) |

## 7. 端末

| 端末 | 探す・入れる | 公開 |
|---|---|---|
| Android | `HttpDiscoverClient`(`POST /search`・`install`)(#902) | `publish` / `unpublish` |
| iOS | `HTTPSharingClient`(`POST /search`・`install`) | `publish`(説明・タグ・作者名) |
| Mac | `MacDiscoverModel` | `MacPublishSection` |
| Windows・Linux(`desktop/`) | 画面は無い | 無い |

- デモ用: `scripts/seed-public-apps.sh <appconfig URL>` が `docs/dsl/examples` の kakeibo / training / walk を `catalog-<name>` として公開する

## 8. 決めること

- 掲載が 500 件を超えた時の検索(今は入れた数の上位 500 件しか検索の対象にならない)。全文検索の索引を入れるか、件数で区切るか
- 古い `GET /v1/public-apps` をいつ外すか(推測: 古い iOS の版を最低の版 `MIN_IOS_BUILD` で止めた後)
