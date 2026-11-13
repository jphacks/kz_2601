# アプリの一覧・並び・複製・版・戻す・削除(#2512)

> [バックエンド設計](../architecture.md) の機能の文書。形(リクエスト・応答)の正は [openapi-appconfig.yaml](../openapi-appconfig.yaml) の tag `apps` と `versions`。ここは「何ができて、いつ何で断られるか」をまとめる。

## 目次

- [何ができるか](#何ができるか)
- [上限](#上限)
- [断られる時(402・409 など)](#断られる時402409-など)
- [消した時に残るもの](#消した時に残るもの)
- [openapi のどこを見るか](#openapi-のどこを見るか)
- [コードの場所](#コードの場所)
- [決めること](#決めること)

## 何ができるか

アプリは `users/{uid}/apps/{appId}` に呼び出した人ごとに置く。他の人のアプリは自分の木に無いので、どの API も 404 になる。

| API | すること |
|---|---|
| `GET /v1/apps` | 自分のアプリの一覧(`updatedAt` の新しい順。`sortIndex` での並べ替えは端末がする)。`pageSize`(既定 50・最大 200)と `pageToken` でページを送る。各項目は `id`・`name`・`currentVersion`・`status`・`createdAt`・`updatedAt`・`sortIndex`・`glanceStat` |
| `POST /v1/apps` | アプリを作る。`dsl` を省くと初期の DSL(音声画面だけ)。`id` を渡すとその appId で作る(形が違えば 400)。`name` を省くと `dsl.app.name` |
| `GET /v1/apps/{appId}` | アプリの情報と今の DSL。`ETag` は今の版の番号 |
| `PATCH /v1/apps/{appId}` | 名前(`name`・前後の空白を除いて 1〜40 文字)・`status`(`active` / `archived`)・一覧のカードに出す数字(`glanceStat`)を変える。名前は `dsl.app.name` にも書いて新しい版を積む(`source.kind=manual`)。同じ名前なら版は進めない。`status` と `glanceStat` だけなら版は進めない |
| `PUT /v1/apps/order` | 一覧の並び順を保存する。`ids` の順に `sortIndex` を 1, 2, … にする。`ids` に無いアプリは今の `sortIndex` のまま(新しいアプリは持たず、iOS は先頭に出す)。自分の木に無い id は飛ばす。`updatedAt` は変えない |
| `POST /v1/apps/{appId}/duplicate` | 今の DSL だけを新しい appId に複製する。版 1(初期)→ 版 2(複製・`source.kind=import`)。**記録は複製しない**。共有の entity を持つアプリでも新しい appId になり、元の共有には参加しない。`name` を省くと元の名前 |
| `GET /v1/apps/{appId}/versions` | 版の履歴(新しい順・DSL の本体は含まない)。ページの送り方は一覧と同じ |
| `GET /v1/apps/{appId}/versions/{version}` | ある版の DSL |
| `POST /v1/apps/{appId}/versions/{version}/restore` | その版の中身で**新しい版を作る**(履歴は消さない・`source.kind=restore`・`source.restoredFrom` に元の版)。応答の `dsl` をそのまま今の DSL として使ってよい |
| `DELETE /v1/apps/{appId}` | アプリを物理削除する(元に戻せない)。204。消した後にもう一度呼ぶと 404 |

- 名前を DSL にも書く理由: DSL の保存は毎回 `dsl.app.name` をアプリの名前に写すので、一覧の名前だけ変えると次の音声の編集で元に戻る。
- 並び順と名前はサーバーに持つので、Apple ID に引き継いだ uid なら機種を変えても残る。
- `archived` のアプリをシェアリンクや「探す」から入れ直すと `active` に戻る(`share.go` の `reactivate`)。

## 上限

| 上限 | 値 | どこで決まるか |
|---|---|---|
| 持てるアプリの数 | プランの数(`PLAN_FREE_APPS` / `PLAN_PRO_APPS`)+ コインで買った枠 | [rate-limit.md「持てるアプリの数」](../rate-limit.md#持てるアプリの数630)。版 1 のままの空のアプリは数えない |
| 名前の長さ | 1〜40 文字(前後の空白を除いて) | `manage.go` の `maxAppNameRunes` |
| 並び順で一度に送れる id | 400 個まで・重ならないこと | `manage.go` の `maxReorderIDs`(Firestore の 1 トランザクションに収める) |
| 一覧・履歴の 1 ページ | 既定 50・最大 200 | `handler.go` の `defaultPage` / `maxPage` |
| 版の数 | 上限は無い(古い版を消す処理はコードに無い) | — |

## 断られる時(402・409 など)

| 状態 | code | いつ |
|---|---|---|
| 402 | `app_limit_reached` | 新しいアプリになる時(`POST /v1/apps`・複製)に持てる数を超える。空のアプリが最初の版になる保存(restore・複製の 2 版目)でも同じトランザクションの中で数える。`details` に置き換えの候補(`apps`)と枠を買えるか(`canBuySlot`)が入る。詳しくは [rate-limit.md](../rate-limit.md#持てるアプリの数630) |
| 402 | `pro_required` | 保存(`POST /v1/apps` に `dsl` 付き・複製・restore)で、有料の機能(共有のデータ・サーバーの自動の動きなど)を**新しく足す**のに Pro が無い。もう持っている機能は止めない。詳しくは [pro.md](pro.md) |
| 409 | `version_conflict` | `POST /v1/apps` で、渡した `id` のアプリがもうある(`details.currentVersion`)。`PATCH` の名前・restore で、書いている間に別の版が積まれた(取り直して送り直す) |
| 400 | `bad_request` | 名前の長さ・`status` の値・`glanceStat` の形・並び順の `ids`(多すぎる・重なる・形が違う)・`version` が正の整数でない・`pageToken` が壊れている |
| 422 | `invalid_dsl` | `POST /v1/apps` の `dsl` や戻す版の DSL が今の決まりで通らない(`details.problems`) |
| 404 | `not_found` | 自分の木に無いアプリ・無い版 |

## 消した時に残るもの

`DELETE /v1/apps/{appId}` が消すのは**呼び出した人が持ち主のもの**だけ。

- 消すもの: 自分のアプリ(`users/{uid}/apps/{appId}`)と配下の版・記録、自分が出したこのアプリのシェアリンク、自分の「探す」の掲載と配下のインストール、自分の共有の写し(`apps_public/{appId}`)と配下の共有の記録。
- 残すもの: 他の人が「探す」やシェアから入れたコピー(別の appId)、同じ appId に参加しているメンバーの写し、他の人が持ち主の掲載・写し・シェア、`app_owners/{appId}`(作成者の記録。appId の乗っ取りを防ぐため)、リリース申請。
- 参加しているメンバーが消した時は、自分の写しだけが消える。持ち主の掲載・シェア・共有の記録は残る。
- 自分の木は最後に消す。途中で失敗しても、もう一度呼べば続きから消せる。

## openapi のどこを見るか

[openapi-appconfig.yaml](../openapi-appconfig.yaml) の次の path。

- `/apps`・`/apps/{appId}`・`/apps/order`・`/apps/{appId}/duplicate`(tag `apps`)
- `/apps/{appId}/versions`・`/apps/{appId}/versions/{version}`・`/apps/{appId}/versions/{version}/restore`(tag `versions`)
- 応答の形: `components/schemas` の `AppSummary`・`AppWithDSL`・`VersionSummary`・`DSLEnvelope`・`RestoreResult`。402 は `components/responses/ProRequiredOrAppLimit`

## コードの場所

すべて `backend/internal/appconfig/` の下。

| ファイル | 中身 |
|---|---|
| `apps.go` | 一覧・作る・読む・`PATCH`・削除 |
| `manage.go` | 名前を変える(`rename`)・並び順(`reorderApps`)・複製(`duplicateApp` / `seedCopy`) |
| `dsl.go` | 版の履歴・ある版・戻す(`listVersions` / `getVersion` / `restoreVersion`) |
| `app_slots.go` | 持てるアプリの数(`checkAppLimit`・保存の中で数える `commit`・402 `app_limit_reached`) |
| `pro_gate.go` | 有料の機能の保存の確かめ(`checkPaidSave`・402 `pro_required`) |
| `handler.go` | 経路の登録・ページ送り・409 への読み替え(`mapStoreErr`) |

削除の中身は `backend/internal/store/firestore_purge.go`(`PurgeApp`)。

## 決めること

- 版の数の上限: 今は無い。古い版を消すか、何版まで持つか。
- `status: archived` の意味: サーバーは値を持つだけで、一覧からも持てるアプリの数からも外さない(推測: 端末の表示の切り替え用)。使い道を決めるか、使わないなら消すか。
