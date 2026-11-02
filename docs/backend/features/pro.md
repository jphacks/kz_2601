# 有料機能の判定(`requiresPro`・Issue #77・2026-10-25)

> [バックエンド設計](../architecture.md) から分けた機能の節(旧 §6.8)。

チャットの ready / proposal に「作る予定の内容が Tailor It Pro(¥500 / 月)を要するか」を付ける。**課金の強制はしない**(iOS が確認カードを出すための情報だけ)。

| 項目 | 決定 |
|---|---|
| 有料リスト(1 か所) | **`backend/internal/ai/paid_features.json`**(`//go:embed`)。初期は **`sharedData`(みんなで共有するデータ = entity.scope shared/public・apps_public の同期)だけ**。他の機能を有料にする時はここに 1 要素足す(`id`・`title`/`reason`/`withoutInstruction` の ja/en・`keywords`・`excludeKeywords`・`dslEntityScopes`)。一時的に外す時は `enabled: false`。id の重複は起動時に panic(テスト `TestPaidCatalog_ParseEditable`) |
| 判定の材料 | **generate に渡る文そのもの**(ready の `transcript` / 案の `instruction`)に `keywords` が含まれるか(`excludeKeywords` を先に取り除く。「共有ボタン」「共有しない」は有料にしない)。sharedData の語は生成プロンプトの `entity.scope` 規則(「みんなで」「共有」「投稿」「ランキング」「掲示板」「友達と」→ scope:public)と揃えてあるので、生成前に生成結果を予測できる |
| edit | 今のアプリが既に持つ有料機能(`dslEntityScopes` で現 DSL を判定)は除く(既に共有のアプリを直すたびに聞かない) |
| 応答(追加フィールド・すべて省略可) | ready: `requiresPro` / `paidFeatures[{id,title,reason}]` / `freeTranscript`。proposal: `options[].requiresPro` / `paidFeatures` / `freeInstruction`。無料なら一切出さない(旧 iOS は今と同じ JSON を受け取る) |
| 「この機能なしで作る」 | `freeTranscript` / `freeInstruction` = 元の文 + `withoutInstruction`(「無料版で作る: 他のユーザーとデータを共有する機能は入れない…scope は private」)。iOS はこれを generate に渡す |
| 実装 | `internal/ai/paid.go`(`PaidCatalog.Detect` / `InApp` / `Without` / `annotate`)。`Service.Chat`(旧 clarify)と `chatFirst` の戻り値に defer で付ける。テスト `paid_test.go` |

**iOS の生成後 DSL で判定しなかった理由**: ai の generate は生成した DSL を **その場で appconfig に保存する**(version が増え、共有アプリなら apps_public にも公開される)。生成後に判定すると、確認カードを出す前に有料機能入りのアプリがサーバーに出来てしまい(「元に戻して」やアプリ一覧から無料で開ける)、「この機能なしで作る」はもう 1 回の生成(LLM 費用・待ち時間)になる。チャットの ready / 案の時点なら生成前に止められ、判定に LLM 呼び出しも増えない。

**サーバー側の権利(#314・2026-11-02)**: iOS が購入・復元の後と起動時に StoreKit 2 の署名付き取引(JWS)を appconfig の `POST /v1/me/entitlements/apple` に送る。サーバーは Apple に問い合わせずに検証し(`internal/appstore`)、`users/{uid}/entitlements/pro` に保存する(`internal/entitlement`)。ai の回数制限はこれを読んで Pro の上限にする(#449)。402 で止めるのは #316、更新・解約・返金の通知は #315。

| 項目 | 決定 |
|---|---|
| 方式 | App Store の署名付き取引をサーバーで直接検証(RevenueCat 等は使わない。手数料なし・iOS の StoreKit 2 ラッパーと同じ前提) |
| 検証 | `x5c` を **Apple Root CA - G3**(`internal/appstore/roots.go` に埋め込み・SHA-256 `63343ABF…3E9179`)まで検証。システムの証明書ストアは使わない。中間・leaf の Apple のマーカー拡張・ES256 の署名・`bundleId` / `productId` / `environment` の許可リスト(`APPSTORE_*` 環境変数)・`expiresDate` の有無。検証時刻は取引の `signedDate`(Apple のライブラリでオンライン確認をしない時と同じ) |
| 環境 | 既定で Sandbox と Production の両方を受ける。審査と TestFlight は本番の版でも Sandbox で買うため(stg と prod は同じ Firestore を使っている)。`APPSTORE_ENVIRONMENTS=Production` で絞れる |
| 保存 | `users/{uid}/entitlements/pro` {productId, originalTransactionId, transactionId, environment, expiresAt, revokedAt?, updatedAt}。期限切れ・返金も保存(isPro=false)。順番が前後しても古い取引で上書きしない |
| 1 つの購読 = 1 つの uid | `appstore_transactions/{originalTransactionId}` {uid, productId, environment, boundAt, updatedAt}。別の有効な uid に結びついていれば 409 `transaction_owned_by_other`(1 つの購読を複数のアカウントで使い回せない)。退会済みの uid からは移す。購入時の `appAccountToken` は uid から作った UUIDv8(`entitlement.AccountToken`・iOS `ProAccountToken`)で、サーバーでは照合のログだけ |
| ai の判定 | `entitlement.ProChecker`(Pro は最大 5 分・期限まで、Pro でないは 15 秒キャッシュ)。読めない時は Pro でない扱い。検証済みの Pro は匿名アカウントでも Pro(購入にサインインは要らない) |
| 退会 | `cmd/purge` が `users/{uid}/entitlements/*` と uid の `appstore_transactions/*` を消す(同じ Apple ID で新しいアカウントに復元できる) |

まだ無いもの: `requiresPro` な内容の 402(#316)。それまでは iOS のカードだけのソフトなゲート(改造クライアントは生成できる。ただし回数の上限はサーバーの Pro の判定に従う)。

## 環境変数

| 変数 | appconfig | ai | 既定 / 備考 |
|---|---|---|---|
| `APPSTORE_BUNDLE_IDS` | o | o | 未設定 = `jp.kz2601.metamon`(`entitlement.DefaultBundleIDs`)。カンマ区切り |
| `APPSTORE_PRODUCT_IDS` | o | o | 未設定 = `jp.kz2601.metamon.pro.monthly`(`entitlement.DefaultProductIDs`) |
| `APPSTORE_ENVIRONMENTS` | o | o | 未設定 = `Sandbox,Production`。`Production` で絞る(上の「環境」) |
