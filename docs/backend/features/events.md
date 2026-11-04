# ホットリロードの変化通知(`GET /v1/apps/{appId}/events`・SSE・#82)

> [バックエンド設計](../architecture.md) から分けた機能の節(旧 §5.1)。

| 論点 | 決定 | 理由 |
|---|---|---|
| 方式 | **backend が Firestore snapshot listener を持ち SSE で通知**(案 B)。iOS が Firestore SDK で直接購読する案 A は不採用 | ルールは全面拒否のまま(クライアント直読みを開けない=認可は API の 1 箇所)。iOS に FirebaseFirestore(gRPC/abseil/leveldb)を足さない=ビルド時間・バイナリ増を避ける。エミュレータ + curl でローカル完結して検証できる |
| 流すもの | `hello{version,entities}` / `dsl{version}` / `records{entity}` / `bye` のみ。本体は既存 API で再取得 | データの正は API のまま。SSE の認可漏れが即データ漏れにならない |
| 購読対象 | `users/{uid}/apps/{appId}`(自分の DSL)+ 現行 DSL の各 entity の records(private は自分の下、public/shared は `apps_public/{appId}/entities/{e}/records`) | 共有掲示板(public)で他ユーザーの投稿が数百 ms で出る |
| records の窓 | `orderBy(updatedAt desc).limit(100)`。**意図した制限**: 窓外(101 件目以降)の古いレコードへの印(いいね)の変化と削除は通知しない(次の pull で反映)。Memory の store も同じ窓で通知を絞り、両方の試験で確かめる(#553) | 接続ごとに一致件数ぶん読み取り課金されるため。作成/更新は必ず窓の先頭に入る。印で `updatedAt` を進める案は不採用: records は `sort=updatedAt` で並べられるので、いいねで一覧の順番が変わってしまう |
| 寿命 | 1 本 **50 秒**(`EVENTS_MAX_STREAM`)で `bye` → クライアント即再接続。心拍 15 秒 | appconfig の Cloud Run `--timeout=60` 内に収める(デプロイ設定は変更不要)。DSL が変わったら entity 構成が変わり得るので `dsl` の直後にも `bye` |
| 多インスタンス | 各インスタンスが自分で Firestore を listen するので、別インスタンスへの書き込みも届く | in-process pub/sub だと max-instances=3 で取りこぼす |
| store | `store.Watcher`(`WatchApp` / `WatchRecords`)。Firestore = snapshot listener、Memory = in-process hub(ローカル・テスト) | ハンドラは interface 依存のまま |
| 実測 | Firestore エミュレータ: 投稿 → SSE `records` まで **38〜39 ms**。シミュレータ表示まで(250 ms デバウンス + pull 込み)**442 ms** | `scripts/realtime-smoke.sh` |

コスト目安: 1 クライアント 1 entity あたり再接続ごとに最大 100 読み取り(50 秒周期で ≒7,200/時)。無料枠 5 万/日に対し、デモ数台・数時間なら収まるが、常時多数接続は不可。必要なら `EVENTS_MAX_STREAM` と Cloud Run `--timeout` を伸ばす(例 3600s / 55m)と読み取りが約 1/60 になる(Cloud Run は接続中ずっと課金対象になる点とのトレードオフ)。

## 環境変数

| 変数 | appconfig | ai | 既定 / 備考 |
|---|---|---|---|
| `EVENTS_MAX_STREAM` | o | - | `50s`。`GET /events` 1 本の寿命(Cloud Run `--timeout` 未満にする) |
