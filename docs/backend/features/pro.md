# 有料機能(`requiresPro`・Issue #77・2026-10-25 / サーバーでの強制・#316・2026-11-06)

> [バックエンド設計](../architecture.md) から分けた機能の節(旧 §6.8)。

## 有料の機能とサーバーでの強制(方針・2026-11-06 オーナーの決定)

正典は [`docs/00_decisions.md`](../../00_decisions.md) の「課金・Pro」。ここは方針だけで、402 の実装の詳細は #316 が書く。

- アプリを作る・直すだけなら無料(今の 1 日の回数制限つき・[rate-limit.md](../rate-limit.md))。
- 次の有料の機能を持つアプリを作る・使う時は Tailor It Pro が要る:
  1. みんなで共有するデータ(`sharedData`)
  2. サーバーの自動の動き(`serverRules`: 通知・定期処理)。無料の枠は持たない([#338](../../decisions/338-server-feature-cost-model.md) 2026-11-06 改定)
  3. アプリの中から使う AI(`aiButton` のオンライン版など・[text.md](text.md))
- **サーバー側で強制する**: Pro でなければ 402 `pro_required`(#316)。下の `requiresPro` の確認カードは案内で、それだけには頼らない。
- 共有のアプリの費用は作成者の Pro で持ち、参加者は無料で使える。
- 将来のトークン制(使った分だけ)は #630 で検討中。決まっていない。

チャットの ready / proposal に「作る予定の内容が Tailor It Pro(¥500 / 月)を要するか」を付け(iOS の確認カード)、**保存と実行の時にサーバーで止める**(402 `pro_required`・下の「サーバーでの強制」)。

**何が有料か(オーナーの決定・2026-11-06)**: 話してアプリを作る・直すのは無料のまま(1 日の回数の上限つき)。**有料の機能を持つアプリを作る・その機能を使う**のに Pro が要る。将来はトークン制(#630)。

| id | 機能 | 止める所(gate) | DSL での判定 |
|---|---|---|---|
| `sharedData` | みんなで共有するデータ(entity.scope shared / public・apps_public の同期) | 保存(`save`)・共有のレコードの書き込み | `dslEntityScopes: [shared, public]` |
| `serverRules` | サーバーの自動の動き(serverRules・通知・定期処理・数える / 記録する) | 保存(`save`)・runner の実行 | `dslServerRules: true`(serverRules が 1 本以上) |
| `onlineAI` | アプリの中のオンラインの AI(aiButton のオンライン版 `POST /v1/text`) | 使う時(`use`)。DSL には入れてよい(端末内の AI は無料) | `dslComponents: [aiButton]`(表示用。保存では見ない) |
| `encryptedFields` | 暗号化して保存する項目(Field.encrypted・#233。鍵は iCloud キーチェーン・private のアプリだけ) | 保存(`save`) | `dslEncryptedFields: true`(encrypted の field が 1 つ以上) |

| 項目 | 決定 |
|---|---|
| 有料リスト(1 か所) | **`backend/internal/paid/paid_features.json`**(`//go:embed`・`internal/paid`。ai と appconfig が同じものを読む)。足す時は 1 要素(`id`・`gate`(save 既定 / use)・`title`/`reason`/`withoutInstruction` の ja/en・`keywords`・`excludeKeywords`・DSL での判定 `dslEntityScopes` / `dslServerRules` / `dslComponents`)。一時的に外す時は `enabled: false`。id の重複・不明な gate は起動時に panic(テスト `internal/paid/paid_test.go`) |
| チャットの判定(`requiresPro`) | gate が save の機能だけ。gate が use の機能(onlineAI)は生成前に聞かない(端末内の AI なら無料で使えるため) |
| 判定の材料(チャット) | **generate に渡る文そのもの**(ready の `transcript` / 案の `instruction`)に `keywords` が含まれるか(`excludeKeywords` を先に取り除く。「共有ボタン」「共有しない」は有料にしない)。sharedData の語は生成プロンプトの `entity.scope` 規則(「みんなで」「共有」「投稿」「ランキング」「掲示板」「友達と」→ scope:public)と揃えてあるので、生成前に生成結果を予測できる |
| edit | 今のアプリが既に持つ有料機能(`dslEntityScopes` で現 DSL を判定)は除く(既に共有のアプリを直すたびに聞かない) |
| 応答(追加フィールド・すべて省略可) | ready: `requiresPro` / `paidFeatures[{id,title,reason}]` / `freeTranscript`。proposal: `options[].requiresPro` / `paidFeatures` / `freeInstruction`。無料なら一切出さない(旧 iOS は今と同じ JSON を受け取る) |
| 「この機能なしで作る」 | `freeTranscript` / `freeInstruction` = 元の文 + `withoutInstruction`(「無料版で作る: 他のユーザーとデータを共有する機能は入れない…scope は private」)。iOS はこれを generate に渡す |
| serverRules の語 | 「参加者に通知」「メンバーに通知」「作った人に通知」「自動で数え」「アプリを開いていなくても」など、サーバーでしかできない動きの語だけ。「毎朝 7 時にリマインド」は端末の通知(app.notifications)で作れるので有料にしない。生成前の予測は目安で、確実な判定は保存の前の DSL(下) |
| 実装 | `internal/paid`(リストと DSL での判定 `Catalog.InApp` / `Added`)・`internal/ai/paid.go`(`PaidCatalog.Detect` / `Without` / `annotate` / `gatePaid`)。`Service.Chat`(旧 clarify)と `chatFirst` の戻り値に defer で付ける。テスト `internal/ai/paid_test.go`・`pro_gate_test.go` |

**チャットの判定を生成前にする理由**(#77。#316 で保存の前の判定を足した後も、確認カードはこちら): ai の generate は生成した DSL を **その場で appconfig に保存する**(version が増え、共有アプリなら apps_public にも公開される)。生成後に判定すると、確認カードを出す前に有料機能入りのアプリがサーバーに出来てしまい(「元に戻して」やアプリ一覧から無料で開ける)、「この機能なしで作る」はもう 1 回の生成(LLM 費用・待ち時間)になる。チャットの ready / 案の時点なら生成前に止められ、判定に LLM 呼び出しも増えない。

**サーバー側の権利(#314・2026-11-02)**: iOS が購入・復元の後と起動時に StoreKit 2 の署名付き取引(JWS)を appconfig の `POST /v1/me/entitlements/apple` に送る。サーバーは Apple に問い合わせずに検証し(`internal/appstore`)、`users/{uid}/entitlements/pro` に保存する(`internal/entitlement`)。ai の回数制限はこれを読んで Pro の上限にする(#449)。402 で止めるのは #316。更新・解約・返金は App Store Server Notifications v2 の `POST /v1/appstore/notifications` と照合ジョブ `cmd/appstore-reconcile` で反映する(#315・[app-store-notifications.md](../../infra/app-store-notifications.md))。

| 項目 | 決定 |
|---|---|
| 方式 | App Store の署名付き取引をサーバーで直接検証(RevenueCat 等は使わない。手数料なし・iOS の StoreKit 2 ラッパーと同じ前提) |
| 検証 | `x5c` を **Apple Root CA - G3**(`internal/appstore/roots.go` に埋め込み・SHA-256 `63343ABF…3E9179`)まで検証。システムの証明書ストアは使わない。中間・leaf の Apple のマーカー拡張・ES256 の署名・`bundleId` / `productId` / `environment` の許可リスト(`APPSTORE_*` 環境変数)・`expiresDate` の有無。検証時刻は取引の `signedDate`(Apple のライブラリでオンライン確認をしない時と同じ) |
| 環境 | 既定で Sandbox と Production の両方を受ける。審査と TestFlight は本番の版でも Sandbox で買うため(stg と prod は同じ Firestore を使っている)。`APPSTORE_ENVIRONMENTS=Production` で絞れる |
| 保存 | `users/{uid}/entitlements/pro` {productId, originalTransactionId, transactionId, environment, expiresAt, revokedAt?, graceExpiresAt?, willRenew?, signedAt, updatedAt}。期限切れ・返金も保存(isPro=false)。順番が前後しても古い取引で上書きしない(同じ取引は `signedAt` が新しい時だけ)。猶予期間中は `graceExpiresAt` まで Pro(#315) |
| 1 つの購読 = 1 つの uid | `appstore_transactions/{originalTransactionId}` {uid, productId, environment, boundAt, updatedAt}。別の有効な uid に結びついていれば 409 `transaction_owned_by_other`(1 つの購読を複数のアカウントで使い回せない)。退会済みの uid からは移す。購入時の `appAccountToken` は uid から作った UUIDv8(`entitlement.AccountToken`・iOS `ProAccountToken`)で、サーバーでは照合のログだけ |
| ai の判定 | `entitlement.ProChecker`(Pro は最大 5 分・期限まで、Pro でないは 15 秒キャッシュ)。読めない時は Pro でない扱い。検証済みの Pro は匿名アカウントでも Pro(購入にサインインは要らない) |
| 退会 | `cmd/purge` が `users/{uid}/entitlements/*` と uid の `appstore_transactions/*` を消す(同じ Apple ID で新しいアカウントに復元できる) |

## サーバーでの強制(#316・2026-11-06)

改造したクライアントでも、Pro でない uid では有料の機能を作れない・使えない。402 の本文は `{"error":{"code":"pro_required","message":…,"details":{"features":[{id,title,reason}], "reason"?}}}`(openapi の `ProRequired`)。

| 所 | 止めるもの | Pro を見る相手 |
|---|---|---|
| ai `POST /v1/generate`(create / edit) | 生成した DSL が今のアプリに無い save の有料機能を持つ時、**保存せずに** 402(生成した後・保存の前に DSL で判定。Claude の費用は掛かるが、生成前の語の予測より確実)。ストリーミングは `event: error` で同じ本文 | 呼び出した人(`Service.Pro`。ID トークンの uid が無い時は appconfig に任せる) |
| ai `POST /v1/text` | Pro でなければ 402(features = onlineAI)。Claude は呼ばない。`appId` の共有のアプリの参加者(作成者でないメンバー)は作成者の Pro で通る。作成者が Pro でなければ `details.reason: creator_needs_pro`(iOS は課金画面ではなく案内)。参加はサーバーが app_owners / members で確かめる(他人の appId を送っても通らない)。作成者が払うのは、依頼がそのアプリの公開中の DSL の aiButton / aiChat と合う時だけ(`text_payer_scope.go`)で、アプリごとに 1 日 `AI_CREATOR_PAID_APP_DAILY` 回まで([rate-limit.md](../rate-limit.md#作成者が払う呼び出しの上限))。利用上限は呼び出した人のまま、ai.usage に `payer_uid_hash`(作成者)を付ける | 呼び出した人。共有のアプリの参加者はアプリの作成者(app_owners) |
| appconfig `PUT` / `PATCH /v1/apps/{appId}/dsl`・`POST …/versions/{n}/restore`・`POST /v1/apps`(dsl つき)・`POST …/duplicate` | 今の DSL(新しいアプリは無し)に無い save の有料機能を足す保存は 402。ai の生成も最後はここを通る(ai が 402 を中継する。「元に戻して」も) | 呼び出した人 = アプリの作成者。**参加者の写し**(作成者が別にいる appId)の保存は見ない(写しは公開されず、写しの private のルールは参加者を作成者として runner が止める) |
| appconfig 共有のレコードの作成・更新・印を付ける | アプリの作成者が Pro でなければ 402(features = sharedData)。呼び出した人が作成者でない時は `details.reason: creator_needs_pro`(参加者は払えないので、iOS は課金画面ではなく案内を出す) | アプリの作成者(app_owners) |
| runner(自動の動き) | 作成者が Pro でない時は実行しない(`skipped_plan`・`limit: needs_pro`。共有のアプリは今まで通り `shared_needs_pro`、匿名は `anonymous`)。**private のアプリのルールも含む**(#338 の「無料にも小さな枠」は無くした・2026-11-06 の決定)。無料の枠は 0(`quota.Plan` が無料のプランを返さない。`RUNNER_LIMIT_*_FREE` は使われない) | 作成者(共有 = app_owners・private = 写しの持ち主) |

| 項目 | 決定 |
|---|---|
| Pro が切れた人のアプリ | 開く・読む(DSL・記録・共有の記録)・有料機能を足さない直し・自分の記録を消す・印を外すはできる。有料機能を足す保存・有料機能の版への restore・複製・共有の書き込み・自動の動きの実行・オンラインの AI は止まる。更新すれば元に戻る(データは消さない) |
| 判定の単位 | 機能ごと。既に共有のアプリにルールを足すのは serverRules が「新しい」ので 402、既にルールがあるアプリのルールを直すのは通る(実行は runner が止める) |
| 取り込み | シェアの取り込み・「探す」からのインストール(`installCopy`)は止めない(参加は無料。独立した写しになった共有アプリは、書き込みの時に作成者 = 取り込んだ人の Pro を見る) |
| 段階的に出すスイッチ | 環境変数 `PRO_GATE_MODE`(ai・appconfig・runner が同じ値を読む・`paid.ModeFromEnv`)。`enforce`(既定)/ `log`(止めずに `pro_gate.would_block` をログに出す。runner は private のアプリを #316 の前の無料の枠で動かす)/ `off`(見ない)。enforce でも止めるたびに同じログを出す(`blocked:true`)。不明な値は起動しない。[runtime-config.md](../../infra/runtime-config.md) |
| キャッシュ | `entitlement.ProChecker`(Pro は最大 5 分、Pro でないは 15 秒)。appconfig は購入・復元の `POST /v1/me/entitlements/apple` の後にその uid のキャッシュを捨てる(`Service.Changed` → `ProChecker.Forget`)ので、買った直後の保存は通る。ai のキャッシュは別のプロセスなので最大 15 秒は古い(その間の generate は appconfig ではなく ai が 402 を返しうる。iOS は購入の後に 1 回やり直す) |
| checker が無い時 | appconfig は `appconfig.WithPro` を渡さなければ見ない(テスト・ツール)。cmd/appconfig は常に渡す(エミュレータ・メモリの権利の保存でも。ローカルで共有を試すには `users/{uid}/entitlements/pro` を書く)。ai は `GOOGLE_CLOUD_PROJECT` が無いローカルでは見ない、Firestore に繋がらない時は誰も Pro でない扱い |
| 審査用の版 | App 内課金が無く、共有・serverRules・オンラインの AI をバイナリから外している。backend は `disabledFeatures: sharedData` の依頼で共有を private に直し(既存)、**serverRules を作らない**(#316 で server-rules pass を飛ばすようにした)ので、審査用の版の生成は 402 に当たらない。`/v1/text` は呼ばない。共有のアプリに参加する入口(シェアのリンク・探す)も無い |

## 環境変数

| 変数 | appconfig | ai | 既定 / 備考 |
|---|---|---|---|
| `APPSTORE_BUNDLE_IDS` | o | o | 未設定 = `jp.kz2601.metamon`(`entitlement.DefaultBundleIDs`)。カンマ区切り |
| `APPSTORE_PRODUCT_IDS` | o | o | 未設定 = `jp.kz2601.metamon.pro.monthly`(`entitlement.DefaultProductIDs`) |
| `APPSTORE_ENVIRONMENTS` | o | o | 未設定 = `Sandbox,Production`。`Production` で絞る(上の「環境」) |
