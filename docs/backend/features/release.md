# リリース申請(#76)

> [バックエンド設計](../architecture.md) から分けた機能の節(旧 §3.3)。

```
release_requests/{id}  # {appId, ownerUid, name, dslVersion, dslJson, purpose, contact, status, createdAt, updatedAt}  id = rr_<ULID 風>
```

| 項目 | 決定 |
|---|---|
| 申請 | `POST /apps/{appId}/release-requests`(`purpose` 必須 200 字・`contact` 任意 100 字)。所有者のみ(自分の木に無いアプリは 404)。現行 DSL をスナップショットして `status=received` で保存 |
| 冪等 | 同じ (ownerUid, appId) に `declined` 以外の申請があれば何も書かず既存を 200 で返す(409 にしない)。Firestore はクエリ + 作成を 1 トランザクションで行うので連打でも 1 件 |
| 取得 | `GET /apps/{appId}/release-requests`(そのアプリ)/ `GET /release-requests`(全アプリ・変身一覧のバッジ用)。DSL 本体は返さない |
| status | `received`(受付)→ `reviewing`(検討中)→ `planned`(リリース予定)/ `declined`(見送り)。**API では変えない**。管理者が Firestore コンソールで書き換える(下記) |
| インデックス | `ownerUid ==`(+ `appId ==`)の等価フィルタのみ・並びはメモリ。単一フィールドの自動インデックスで足りるので複合インデックス不要 |

**管理者の運用手順(Firestore コンソール)**

1. GCP コンソール → Firestore → データ → コレクション `release_requests` を開く(新しい申請は `status = received`)。
2. 申請のドキュメントで `purpose` / `contact` / `name` / `dslVersion` を確認する。DSL 本体は `dslJson`(文字列)。同種の申請を探すときはコレクションのフィルタで `appId` や `name` を指定する。
3. `status` フィールドを編集して `reviewing` / `planned` / `declined` のいずれかに書き換える(小文字・この 4 値以外は iOS で「受付」扱い)。`updatedAt` も手で今の時刻にしておくと履歴が追いやすい(任意)。
4. 申請者の iOS では次に変身一覧を開いたときカードのバッジが変わる。`declined` にすると、申請者は同じアプリで再申請できる。
5. 連絡が必要なら `contact` へ運営から直接連絡する(アプリ内通知は未実装)。

ローカルではエミュレータの REST で同じ操作ができる: `curl -X PATCH -H 'Authorization: Bearer owner' "http://localhost:8089/v1/projects/demo-kz2601/databases/(default)/documents/release_requests/<id>?updateMask.fieldPaths=status" -d '{"fields":{"status":{"stringValue":"planned"}}}'`。

**セキュリティルール**: クライアントは Firestore を直接読まず API 経由のみ。`release_requests` には連絡先(個人情報)が入るので、ルールでクライアントからの読み書きを許可しないこと(既存の `shares` / `public_apps` と同じ扱い)。
