# シェアリンクと公開カタログ「探す」(#49 #50)

> [バックエンド設計](../architecture.md) から分けた機能の節(旧 §3.2)。

```
shares/{token}        # {appId, ownerUid, name, dslVersion, dslJson, revoked, createdAt}  token = 192bit base64url
public_apps/{appId}   # {ownerUid, name, description, tags[], authorName, components[], dslVersion, dslJson, installs, createdAt, updatedAt}
public_apps/{appId}/installs/{uid}  # {appId(その人の複製先), createdAt}  installs を uid ごとに 1 回だけ数える(#154)
app_owners/{appId}    # {uid, createdAt}  appId の作成者(publish できる人・#153)
```

`public_apps`(探すの掲載)は [§3.1](shared-data.md) の `apps_public`(共有レコードの置き場)とは**別物**。名前が近いので注意。

| 項目 | 決定 |
|---|---|
| シェア | `POST /apps/{appId}/share` が現行 DSL を**スナップショット**して token を返す(後から作者が変えてもリンク先は変わらない)。`DELETE /shares/{token}` で取り消し(発行者のみ)。取り消し後の import / プレビュー / Web ページは 404 |
| 複製(import / install 共通 `installCopy`) | 受け取った人の木に version 1(初期)→ version 2(複製 DSL・`source.kind=import`)で作る(iOS の変身一覧は version 1 を「未生成」として隠すため)。**レコードは複製しない**。応答の `mine` は「呼び出した人が元のアプリの作成者(`app_owners`)か、リンク / 掲載の発行者か」(#565)。プレビュー(`GET /shares/{token}` の `mine`)が取れなくても、iOS はこれで自分のアプリの課金画面を飛ばす |
| 共有データ(scope: shared/public)の扱い | 共有 entity を含むアプリは**新しい appId を振らず元の appId に参加**する。理由: #93 の共有レコードは `apps_public/{appId}` で appId 単位、かつ「他人が同じ appId を開く = インストール」という設計なので、新 appId だと受け取った人は空の掲示板を見るだけになり共有の意味が無い。既に持っていれば何もしない(`created:false`)。private だけのアプリは新しい UUID の appId に独立コピー |
| 公開 | `POST /apps/{appId}/publish`(name/description/tags/authorName)で作成・更新、`DELETE` で取り下げ。**その appId の作成者だけ**が掲載できる(#153)。作成者 = 最初に実コミット(生成 / PUT・PATCH /dsl / restore / `POST /apps` に dsl 付き / 新規 appId への複製)した uid を `app_owners/{appId}` に Create(既にあれば上書きしない)で記録。GET /dsl の seed(参加)と共有アプリへの参加 install は記録しないので、他人の appId を開いた人・参加者は 403。記録の無い既存アプリは「探す」の掲載者 → `apps_public` の持ち主の順で作成者とみなして記録し、どちらも無ければ最初に publish した人(空のアプリでは不可)を記録する。中身が初期 DSL のまま(components / entities 空)は 400 |
| 検索 | `GET /public-apps?q=&sort=popular`(または `new`): installs 降順で最大 500 件を読み、名前・説明・タグ・作者名の部分一致をメモリ内で絞る(全文検索なし・複合インデックス不要) |
| installs | `POST /public-apps/{id}/install` は `installs/{uid}` を見て、2 回目以降は最初の複製を返す(`created:false`・アーカイブ済みなら active に戻す。複製が削除済み(#180)なら新しく複製する)。installs は uid ごとに 1 回だけ、マーカーと同じトランザクションで +1。掲載者本人・参加済みの再インストールは数えない(#154) |
| 認可 | publish = appId の作成者のみ(`app_owners`)。unpublish = 掲載の ownerUid。revoke = 発行者。share = 自分の木にあるアプリなら誰でも(参加者が友だちを招ける・#118。空のアプリは 400)。import / install / 検索 = 認証済みなら誰でも |
| Web の共有ページ(#362) | 認証不要の `GET /s/{token}`(appconfig 直下・`/v1` の外)。スナップショット DSL の画面を**読むだけ**で描く(`sharepage.go` + `shareview.go`・`html/template`・JS なし・部品の対応は `docs/dsl/README.md` §3.8)。**レコードは出さない**(#253。公開レコード用の口 `RecordPeek` は nil 固定)。導線は「アプリで開く」(`tailorit://import/<token>`)と App Store(`AppStoreURL`)。og:title / og:description 付き・`noindex`。セキュリティ: CSP `default-src 'none'` + `<style>` の nonce だけ(script・img・frame・form は不可)、DSL の文字は全部エスケープ、リンクは `https://` の `link` だけ、`Referrer-Policy: no-referrer`(token を外へ漏らさない)。描画の上限は 12 画面・400 部品・200ms・512KB。キャッシュ: 有効なら `private, max-age=60`(取り消しは最大 1 分で反映)、無効は 404 + `no-store`。取り消し・存在しない token は従来どおり「リンクが無効です」。運営の非公開化(#321)は `shareHidden` で同じ 404 にする。ベース URL は `SHARE_WEB_BASE_URL`、無ければリクエストのホスト |
| モデレーション(#321・審査 1.2) | **通報** `POST /reports` → `reports/{id}`(通報者 uid・対象の種類と ID・持ち主 uid・理由・日時のみ。1 人 1 日 30 件・同じ対象は 1 件)+ ログ `moderation: report` を運営のアラートが拾う。**ブロック** `/me/blocks`(`users/{uid}/blocks/{blockedUid}`): ブロックした人の投稿(GET records)と掲載(GET /public-apps)をサーバーで除く。**運営の非公開化** `cmd/moderate`: `public_apps/{id}` / `apps_public/{appId}` に `hidden`・`hiddenReason`(作者に見せる理由)。掲載は本人以外に出さず install は 403 `unpublished`、共有アプリは records・シェアリンク・参加・GET /dsl の seed を止める。投稿は `delete-record` で消す。手順: `docs/infra/moderation.md` |

デモ用シード: `scripts/seed-public-apps.sh <appconfig URL>` が `docs/dsl/examples` の kakeibo / training / walk を `catalog-<name>` として公開する(再実行は上書き)。

## 環境変数

| 変数 | appconfig | ai | 既定 / 備考 |
|---|---|---|---|
| `SHARE_WEB_BASE_URL` | o | - | 未設定 = リクエストのホスト。Web の共有ページ(`GET /s/{token}`)のベース URL。prod は tailor-it.app の `/s/*` を appconfig に中継できてから `https://tailor-it.app` にする(Universal Links・#331・`docs/infra/universal-links/README.md`) |
