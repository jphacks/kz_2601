# フォローとタイムライン(#270)

「フォローした人の投稿だけが流れるタイムライン」を作れるようにする。洗い出しの D-22・方式は [#336 の決定](../../decisions/336-backend-autogen-approach.md)(種類「絞り込み」・filter の値 `{"myFollows": true}` を `$author` と組む)。DSL の書き方は [DSL README の「フォロー」](../../dsl/README.md#フォロー-myfollows270)。

## 目次
- [1. データ](#1-データ)
- [2. API](#2-api)
- [3. 見える範囲と安全](#3-見える範囲と安全)
- [4. 決めたこと](#4-決めたこと)
- [5. 分かっている制約](#5-分かっている制約)

## 1. データ

| 置き場 | 中身 |
|---|---|
| `apps_public/{appId}/follows/{uid}` | `{uids: [フォローしている人の uid(古い順)], updatedAt}`。1 人 1 文書・最大 500 人(`store.MaxFollows`) |

- フォローはアプリごと。同じ人でも別のアプリでは別。
- 書き込みはトランザクション(同時に押しても取りこぼさない・上限を超えない)。
- 退会の消去(#458)で、本人の文書と、他の人の一覧にある本人の uid を消す(ログの `follows`)。共有アプリごと消える時は `apps_public/{appId}` と一緒に消える。

## 2. API

| メソッド | パス | 返す |
|---|---|---|
| GET | `/v1/apps/{appId}/follows` | `{uids}` |
| PUT | `/v1/apps/{appId}/follows/{uid}` | `{following: true, count}`(冪等) |
| DELETE | `/v1/apps/{appId}/follows/{uid}` | `{following: false, count}`(冪等) |
| GET | `/v1/apps/{appId}/entities/{e}/records?filterField=author&filterOp=in&filterValue=myFollows` | フォローしている人の投稿(`nin` = それ以外) |

エラー: 公開済みの DSL に共有の entity が無い 404 `no_shared_data` / 自分 400 `follow_self` / 500 人を超える 403 `follows_full` / scope:shared だけのアプリで、呼んだ人が参加者でない 403 `not_member`・相手が参加者でない 404 `not_in_app` / 運営が非公開にしたアプリ 403 `unpublished`。契約は [openapi-appconfig.yaml](../openapi-appconfig.yaml)。

## 3. 見える範囲と安全

- フォローの一覧は本人だけが読める。フォローされた人への通知・フォロワーの一覧・人数は作らない。
- 相手の uid は投稿の `author` として既に見えているものだけ。フォローで新しく分かる情報は無い。
- ブロック(#321)した人の投稿は、フォローしていても一覧から除かれる(records の一覧の既存の処理)。
- 審査用の版(共有を外す・#451 G2)では AI のフォローのルールも外す。

## 4. 決めたこと

| 項目 | 決めたこと | 理由 |
|---|---|---|
| 置き場 | 1 人 1 文書の uid の配列 | 「自分のフォロー」を 1 回で読める。records の一覧はメモリで絞っているので、Firestore の `in`(30 件まで)を使わない |
| 上限 | 1 アプリ 500 人 | 文書の大きさ(1 MiB)に十分収まり、仲間内・小さな SNS に足りる |
| フォローできる相手 | public があるアプリは誰でも(投稿の author)、shared だけのアプリは参加者だけ | 参加者だけのアプリに外の人を持ち込まない |
| 自分の投稿 | `in` に入らない | 「フォローした人の投稿」の意味どおり。自分の投稿は「自分」タブ(`{"me":true}`) |
| プロフィール | 投稿者の表示名(`authorName`)だけ | 参加者の情報にアイコンが無い。足すなら別の Issue |

## 5. 分かっている制約

- records の一覧が 1 entity 2000 件のメモリの絞り込みのままであること前提。クエリで絞るように変える時は、30 人ずつの分割か、投稿時に配る(#341 の自動の動き)を選び直す。
- AI は、iOS がこの機能を持つかを見ずに使う(`supportedFeatures` での出し分けは入れていない)。古い iOS では一覧が空になる。
