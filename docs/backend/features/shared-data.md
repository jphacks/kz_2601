# 共有データ(entity.scope・#47)

> [バックエンド設計](../architecture.md) から分けた機能の節(旧 §3.1)。

DSL の `Entity.scope`(`private` 既定 / `shared` / `public`)でレコードの置き場と権限が変わる。`shared` の置き場は `public` と同じで、読み書きできるのは作成者と参加者だけ([§3.4](members.md)・#254)。下の表の public / shared 列の「認証ユーザー全員」は shared では「参加者全員」と読む。

```
apps_public/{appId}                                   # 公開 DSL: {ownerUid, name, version, dsl(map), dslJson, createdAt, updatedAt}
└─ entities/{entityId}/records/{recordId}             # 共有レコード: {entity, data, clientId, author, authorName, createdAt, updatedAt}
```

| 項目 | private(従来) | public / shared |
|---|---|---|
| レコードの置き場 | `users/{uid}/apps/{appId}/entities/{e}/records` | `apps_public/{appId}/entities/{e}/records`(uid 非依存。appId は iOS の UUID なので単独で一意) |
| 読み取り | 本人のみ | 認証ユーザー全員 |
| 印(いいね・保存 #255) | 無し(DSL がエラー) | `entity.marks` に宣言した印を 1 人 1 回。件数はレコードの `markCounts.{markId}`、誰が付けたかは `.../records/{id}/marks/{markId}/by/{uid}`(トランザクションで両方を更新)。`PUT` / `DELETE .../records/{id}/marks/{markId}`。レコード削除で一緒に消える |
| create | 本人 | 認証ユーザー全員。`author`=uid・`authorName`=リクエストの `authorName`(無ければ "匿名")を自動付与。`clientId` の冪等 id は `uid/clientId` から導出(他人と衝突しない) |
| update / delete | 本人 | `author == uid` のみ(他人のレコードは **403 forbidden**) |
| delete all(`?confirm=true`) | 全件 | **自分の投稿だけ**削除 |
| `GET /records` | 従来どおり | 各レコードに `author`/`authorName` を含む。`filterField=author&filterOp=eq&filterValue=<uid>` で「自分の投稿」。uid を URL に載せないため、絞り込みは `POST .../records/query` の本文で送る(GET の filter は古い iOS 用に残す) |
| DSL | `users/{uid}/apps/{appId}` | 作成者の `PUT/PATCH /dsl`・`POST /apps`・restore 時に public entity を 1 つでも含めば `apps_public/{appId}` に複製(所有者=最初に公開した uid・他人は上書き不可)。既存のアプリへのコミット(`PUT/PATCH /dsl`・restore・名前の変更)は**コミットと同じトランザクションで**複製を更新し、複製の版はいつも作成者の最新の版と同じ(複製に失敗したらコミットごと失敗する)。複製が既にあれば、共有の entity が無くなったコミットでも更新する(取り下げた共有の決まりと自動の動きが残らない。共有レコードは消さず、参加者からは見えなくなる。共有に戻すとまた見える)。他ユーザーの `GET /dsl` は自分の下に無ければ公開 DSL を複製して version 1 を作る(=インストール) |

実装: `store.RecordSet{UID, AppID, Entity, Public}` でコレクションを選ぶ(`store.PrivateRecords` / `store.PublicRecords`)。`store.PublishApp` / `GetPublicApp` が公開 DSL。handler は `appconfig/records.go` の `recordSet()` と `ownRecord()`(403 判定)。レコード検証は従来どおり呼び出し側ユーザーの現行 DSL で行うので、インストール後に各自が entity を変えても他人のレコードは消えない(スキーマレス)。entity の共有の設定(`scope`・`edit`・`marks`)だけは、作成者(`app_owners`)が公開した DSL(`apps_public/{appId}`)のものを使う。作成者は自分の DSL のまま。自分の DSL では shared / public なのに作成者の公開 DSL に無い entity は **403 `forbidden` / `details.reason: not_published`**。シェアの作成と events の参加者の確かめも同じ判定(`appconfig/record_scope.go`)。
