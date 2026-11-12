# Cloud Run の環境変数(デプロイが決める値・運用で変える値)(#509)

`appconfig-<env>` / `ai-<env>`(`<env>` = `stg` / `prod`)と `runner-prod`(#288 の分離まで prod だけ・[runner.md](runner.md))の環境変数を、**デプロイ(`.github/workflows/deploy-cloudrun.yml`)が毎回決める値**と、**運用中に人が gcloud で変える値**に分けて書きます。

デプロイは `--update-env-vars` で前者だけを上書きします。後者は次のデプロイの後も残ります。

> 本書のコマンドはどれも Cloud Run の設定を変えます(新しいリビジョンができ、トラフィックが移ります)。prod は承認を得てから人が行います。

## 目次
- [しくみ](#しくみ)
- [デプロイが決める値](#デプロイが決める値)
- [運用で変える値](#運用で変える値)
- [変え方](#変え方)
- [FIRESTORE_DATABASE(復元時)](#firestore_database復元時)
- [シークレット](#シークレット)
- [ワークフローから変数を外す時](#ワークフローから変数を外す時)
- [今の値を見る](#今の値を見る)

## しくみ

| フラグ | 動き | 使う所 |
|---|---|---|
| `--update-env-vars` | 書いたキーだけを足す・上書きする。他は残す | デプロイの env(#509 から) |
| `--set-env-vars` | 既存の env を全部消してから入れる | 使わない(#509 まではこれで、手で入れた値が消えていた) |
| `--remove-env-vars` | 書いたキーを消す | 外した変数の後片付け |
| `--set-secrets` | 既存のシークレット参照を全部消してから入れる | デプロイのシークレット(ai) |

`gcloud run deploy` は作成と更新の両方を行うので、サービスがまだ無い初回デプロイでも `--update-env-vars` で作れます(`gcloud run deploy --help`)。

## デプロイが決める値

デプロイのたびにワークフローの値に戻ります。gcloud で変えても次のデプロイで戻るので、ずっと変えたい時はワークフローか GitHub の Environment 変数を変えます。

| 変数 | サービス | 値 |
|---|---|---|
| `GOOGLE_CLOUD_PROJECT` | 3 つ(appconfig・ai・runner) | repo Variable `GCP_PROJECT_ID` |
| `FIRESTORE_DATABASE` | 3 つ | Environment 変数 `FIRESTORE_DATABASE`。未設定なら `(default)`。[下を参照](#firestore_database復元時) |
| `APP_ENV` | 3 つ | `stg` / `prod` |
| `ALLOW_SHARED_PROD_DB` | 3 つ | stg は `1`、prod は `0`(#290) |
| `APP_CHECK_MODE` | appconfig・ai | Environment 変数 `APP_CHECK_MODE`。未設定なら `off`([app-check.md](app-check.md#モードの切り替え)) |
| `GIT_SHA` | 3 つ | デプロイしたコミット |
| `APPCONFIG_BASE_URL` | ai | 同じ環境の appconfig の URL |
| `RUNNER_ACCOUNT_AGE` | runner(prod だけ) | `firestore`。送り手のアカウントの古さを `users/{uid}.firstSeenAt` から読む(#339「作成から 24 時間は他人宛ての通知を出せない」・[runner.md](runner.md#アカウントの古さ作成から-24-時間))。`firebase`(Firebase Auth の作成時刻・SA に `roles/firebaseauth.viewer` が要る)はコードに残すが使わない |

runner は prod で GitHub の変数 `RUNNER_DEPLOY=1` の時だけデプロイする(既定は飛ばす・[runner.md](runner.md))。

`GET /health`(appconfig・ai・runner)はこのうち `APP_ENV`・`FIRESTORE_DATABASE`・`GIT_SHA`(先頭 7 文字)だけを返す(#290)。例: `{"status":"ok","env":"stg","firestoreDatabase":"(default)","version":"89061ea"}`。デプロイ後に stg / prod の取り違えと版をここで確かめる。誰でも叩ける口なので、ほかの値は足さない。

## 運用で変える値

デプロイでは触りません。未設定なら backend の既定値で動きます。

| 変数 | サービス | 何をするか | 既定 |
|---|---|---|---|
| `NOTICE_JSON` | appconfig | アプリ内のお知らせ(#333)。`GET /v1/client-config` の `notice` として 1 行のバナーを出す。形は `openapi-appconfig.yaml` の `Notice`(PR #500 で入る) | 無し(出さない) |
| `MIN_IOS_BUILD` | appconfig | これより古い iOS ビルドに「アップデートしてください」の画面を出す(閉じられない・#302) | 無し |
| `LATEST_IOS_BUILD` | appconfig | これより古いビルドに閉じられるアップデート案内を出す(#302) | 無し |
| `IOS_APP_STORE_URL` | appconfig | 上の案内から開く App Store の URL(`https://apps.apple.com/app/id<id>`) | 無し |
| `MIN_ANDROID_BUILD` | appconfig | `MIN_IOS_BUILD` の Android 版(versionCode) | 無し |
| `LATEST_ANDROID_BUILD` | appconfig | `LATEST_IOS_BUILD` の Android 版(versionCode) | 無し |
| `ANDROID_STORE_URL` | appconfig | Android の案内から開く Google Play の URL(`https://play.google.com/store/apps/details?id=<pkg>`) | 無し |
| `LOOP_SOURCE_GOOGLEPLAY` | feedback-loop(job・まだ定期の実行は無い) | `on` で Google Play のレビューを改善ループに取り込む(#2147)。`LOOP_GP_PACKAGE_NAME`(既定 `GOOGLE_PLAY_PACKAGE_NAME`)と、任意の `LOOP_GP_SERVICE_ACCOUNT_JSON`(Secret Manager から・空なら job の ADC)も読む。SA に Play Console のレビューの返信の権限が要る([feedback-loop.md](../backend/features/feedback-loop.md#環境変数)) | 無し(off) |
| `MIN_MACOS_BUILD` | appconfig | `MIN_IOS_BUILD` の Mac 版(CFBundleVersion・#981) | 無し |
| `LATEST_MACOS_BUILD` | appconfig | `LATEST_IOS_BUILD` の Mac 版(CFBundleVersion) | 無し |
| `MAC_APP_STORE_URL` | appconfig | Mac の案内から開く Mac App Store の URL(`https://apps.apple.com/app/id<id>` / `macappstore://`) | 無し |
| `FEEDBACK_DAILY_LIMIT` | appconfig | 1 人 1 日のフィードバック送信の上限。`0` で数えない | 5 |
| `LOOP_GENFAIL_SOURCE` | feedback-loop(job・まだ定期の実行は無い) | 生成の失敗の急増(`LOOP_SOURCE_GENFAIL=on`)を読む先。`file` = `LOOP_GENFAIL_LOGS` のファイル、`logging` = Cloud Logging の API(#2146)。`logging` の時は `LOOP_GENFAIL_PROJECT`(既定 `GOOGLE_CLOUD_PROJECT`)・`LOOP_GENFAIL_SERVICES`(既定 `ai-<APP_ENV>`)・`LOOP_GENFAIL_MAX_LINES`(既定 50000)も読む。job の SA に `roles/logging.viewer` が要る([feedback-loop.md](../backend/features/feedback-loop.md#環境変数)) | `file` |
| `FEEDS_DISABLED` | appconfig | 外部の公開データ(#348)の provider を止める(カンマ区切り・`all` = 全部)。止めた provider は 503 で、部品は「読み込めませんでした」([external-feeds.md](../backend/features/external-feeds.md)) | 無し(止めない) |
| `FEEDS_USER_PER_HOUR` / `FEEDS_UPSTREAM_PER_MINUTE` | appconfig | 外部の公開データの 1 人 1 時間の回数 / インスタンスあたり 1 分の出どころへの取得 | 120 / 30 |
| `BOOK_LOOKUP_ENABLED` | appconfig・ai(2 つとも同じ値にする) | ISBN から本の情報を引く `lookupBook` を動かす(`1` / `true`)。**既定で止めている**(openBD の利用条件が本の紹介のためなので・2026-11-08 のオーナー判断)。appconfig は止めている間 503 `reason=disabled`(ボタンは「本の情報の取得は今使えません」)、ai は止めている間 `lookupBook` を生成しない。片方だけに入れない([book-lookup.md](../backend/features/book-lookup.md#既定で止めている理由)) | 無し(止める) |
| `BOOK_LOOKUP_DISABLED` | appconfig・ai | `lookupBook` を止める(`1` / `true`・`FEEDS_DISABLED=all` でも止まる)。`BOOK_LOOKUP_ENABLED` より強い | 無し |
| `BOOK_LOOKUP_USER_PER_HOUR` / `BOOK_LOOKUP_UPSTREAM_PER_MINUTE` | appconfig | 本の情報の 1 人 1 時間の回数 / インスタンスあたり 1 分の出どころ(openBD)への取得 | 60 / 30 |
| `AI_PAUSED` | ai | `1`(`true` / `on` も可)で AI の生成だけを止める(#511)。generate(`mode=undo` を除く)・chat・text が Claude を呼ばずに 503 `ai_paused` を返す。AI を止める時はこれを使う([ai-pause.md](ai-pause.md)) | 無し(止めない) |
| `PRO_GATE_MODE` | ai・appconfig・runner(3 つとも同じ値にする) | 有料の機能(共有データ・自動の動き・アプリの中のオンラインの AI)の強制(#316・[pro.md](../backend/features/pro.md))。`enforce` = Pro でなければ 402 `pro_required` / runner は `skipped_plan`。`log` = 止めずに、止めるはずだった事実を構造化ログ `pro_gate.would_block`(`where`・`uid_hash`・`features`・`blocked:false`)に出すだけ(runner は private のアプリを #316 の前の無料の枠で動かす)。`off` = 見ない(#316 の前と同じ)。共有データの自動の動きは #338 のとおりどの値でも Pro だけ。それ以外の値は起動しない(打ち間違いで外れないように)。段階的に出す時は `log` で `would_block` の量を見てから `enforce` | `enforce` |
| `TEXT_FILTER` | appconfig | 表示できない言葉のフィルタ(#321・[moderation.md](moderation.md))の切り替え。`enforce` = 当たれば 422 `text_not_allowed`。`log` = 保存して、ログ `moderation: text would be refused` に場所と種類だけを出す(文字は出さない)。`off` = 見ない。誤検知で書き込めない人が出た時に、デプロイせずに `log` / `off` へ落とす。それ以外の値は起動しない | `enforce` |
| `RATE_LIMIT_GLOBAL_DAILY` | ai | 環境全体の 1 日の AI 上限(費用ブレーカー)([rate-limit.md](../backend/rate-limit.md))。AI を止める用途には `AI_PAUSED` を使う。`1` にして止めるのは、`AI_PAUSED` の入っていない版が動いている時の予備だけ([incident-runbook.md §5](../ops/incident-runbook.md#5-手順-a-claude-api-の障害上限)) | 2000 |
| `RATE_LIMIT_ANON_DAILY` / `_USER_DAILY` / `_PRO_DAILY` / `_IP_DAILY` | ai(`_IP_DAILY` 以外は appconfig も) | 利用者ごと・IP ごとの 1 日の上限([rate-limit.md](../backend/rate-limit.md))。appconfig は `GET /v1/me/usage` の上限の表示に読むので、変える時は両方に同じ値を入れる | 30 / 60 / 300 / 300 |
| `RATE_LIMIT_ANON_DAILY_<OS>` / `_USER_DAILY_<OS>` / `_PRO_DAILY_<OS>` | ai・appconfig(同じ値にする) | 上の利用者ごとの上限を OS ごとに上書き(#2133)。`<OS>` は `IOS` / `ANDROID` / `MACOS` / `WINDOWS` / `LINUX` / `OTHER`(`X-Client-Platform` をそろえた値・ヘッダ無しや未知は `OTHER`)。例 `RATE_LIMIT_USER_DAILY_WINDOWS=20`。ヘッダは偽れるので下げる向きに使う([rate-limit.md](../backend/rate-limit.md))。ai は起動ログ `ai: rate limit on (...)` に上書きがある時だけ `platformLimits=` を出す | 未設定(上書きなし) |
| `RATE_LIMIT_WEIGHT_*` | ai・appconfig | 呼び出し 1 回で数える重み([rate-limit.md](../backend/rate-limit.md))。appconfig は `GET /v1/me/usage` の `weights` に出す | 3 / 2 / 1 / 1 / 2 |
| `RATE_LIMIT_WAIVED_HOURLY` | ai | 1 人 1 時間に、数えずに返す `/v1/text` の 402 などの回数。超えた分は利用上限を数える([rate-limit.md](../backend/rate-limit.md#返した依頼の上限))。`0` で数えない | 30 |
| `RATE_LIMIT_WAIVED_IP_HOURLY` | ai | 同じ IP から 1 時間に、数えずに返す `/v1/text` の 402 などの回数(匿名のアカウントを作り直しても同じ IP で数える)。超えた分は利用上限を数える([rate-limit.md](../backend/rate-limit.md#返した依頼の上限))。`0` で数えない | 120 |
| `AI_CREATOR_PAID_APP_DAILY` | ai | 共有のアプリで作成者の Pro が払う `/v1/text` の、アプリごとの 1 日の回数。超えると 429 `rate_limited`(`scope: app`)([rate-limit.md](../backend/rate-limit.md#作成者が払う呼び出しの上限))。`0` で数えない | 300 |
| `AI_CREATOR_PAID_MEMBER_DAILY` | ai | 共有のアプリで作成者の Pro が払う `/v1/text` の、アプリ × 参加者ごとの 1 日の回数(aiChat も含む)。超えると 429 `rate_limited`(`scope: app_member`)([rate-limit.md](../backend/rate-limit.md#作成者が払う呼び出しの上限))。`0` で数えない | 30 |
| `RUNNER_GLOBAL_DAILY` | runner(prod だけ) | 環境全体の 1 日の自動の動きの実行の上限(費用の遮断・[#338](../decisions/338-server-feature-cost-model.md#上限案))。`0` で数えない | 10000 |
| `RUNNER_LIMIT_*` | runner(prod だけ) | アプリ・作成者・きっかけの人・受け取る人ごとの 1 日の上限と、1 回の実行の形の上限(`backend/internal/runner/quota/config.go` の `ConfigFromEnv`・#345)。`_FREE` / `_PRO` の組は無料 / Pro。**自動の動きは Pro だけ(#316)なので `_FREE` は今は使われない**(Pro でない作成者のルールは数える前に `skipped_plan`)。`0` で数えない | 実行 100 / 2000(アプリ)・300 / 3000(作成者)、書き込み 100 / 2000、通知の宛先 50 / 1000、受け取る人 20(1 アプリ)・100(合計)、きっかけの人 200、1 回の宛先 1 / 200・書き込み 1 / 10、定期のルール 1 / 5 |
| `APNS_KEY_ID` / `APNS_TEAM_ID` / `APNS_TOPIC` | runner(prod だけ) | APNs の鍵の ID・Apple Developer のチーム ID・アプリの bundle id(#269)。鍵(`APNS_AUTH_KEY`)を入れた時は 3 つとも要る(無いと起動しない)。秘密ではないが repo には書かず gcloud で入れる([runner.md](runner.md#プッシュ通知を端末に届ける)) | 無し |
| `APNS_AUTH_KEY` | runner(prod だけ) | APNs の鍵(.p8 の中身)。**Secret Manager の `APNS_AUTH_KEY` から deploy-cloudrun.yml が注入する**(GitHub の変数 `RUNNER_APNS=1` の時だけ)。env に直接書かない。無ければ通知はログに形を出すだけ | 無し(ログだけ) |
| `RUNNER_APNS_ENVIRONMENT` | runner(prod だけ) | `production` / `sandbox`。送る APNs の宛先と、送る端末の `environment`。TestFlight・App Store の版は production、Xcode から入れた開発署名の版は sandbox。1 つの runner はどちらか片方だけに送る。iOS の構成との対応: Release(`aps-environment` = production)→ `https://api.push.apple.com`、Debug / Staging(development)→ `https://api.sandbox.push.apple.com`(`backend/internal/runner/notify/apns` の `HostProduction` / `HostDevelopment`)。Review(審査用)は通知を外していて `aps-environment` が無い。runner は prod だけなので、stg の backend を向く Debug / Staging の版には今は届かない(`ios/project.yml` の `METAMON_APS_ENV`) | `production` |
| `RUNNER_AUTO_NOTIFY_PAUSE_REPORTERS` | runner(prod だけ) | 1 つのアプリ(アプリそのもの・その中の投稿)を 24 時間でこの人数(別々の人)が通報したら、そのアプリの通知を自動で一時停止する(#339・#346)。止めるのは通知だけ(非公開化などは人)。`0` で自動では止めない | 3 |
| `EXT_DISABLED` | runner | `1` で外の API(#1493)を全部止める(止め弁)。止めた間の呼び出しは 503 `ext_unavailable`。数えない・コインも引かない([external-apis.md](../backend/features/external-apis.md)) | 無し(止めない) |
| `EXT_<PROVIDER>_ENABLED` | runner | `1` で外の API の provider を 1 つ入れる(`GOOGLE_CALENDAR` などの大文字)。偽の provider(`FAKE_FREE` / `FAKE_PAID` / `FAKE_OAUTH`)は stg とローカルだけで、`APP_ENV=production` では入れても動かない | 無し(off) |
| `EXT_TOKEN_KMS_KEY` | runner | 利用者ごとの OAuth のトークンを暗号化する Cloud KMS の鍵(`projects/…/cryptoKeys/…`)。無いと OAuth の要る provider は動かない(エミュレータでは使い捨ての鍵) | 無し |
| `EXT_OAUTH_CALLBACK_BASE` / `EXT_OAUTH_APP_RETURN` | runner | OAuth の戻り先の runner の https の頭(`/oauth/callback/{provider}` を足す)/ 戻った後に端末へ返す先。前者が無いと OAuth の要る provider は動かない | 無し / `tailorit://connected` |
| `EXT_COST_USD_<PROVIDER>_<OP>` / `COIN_RESERVE_EXT_<PROVIDER>` | runner | 有料の API の 1 回の原価(USD・op が量を返さない時)/ 押さえるコイン。`COIN_BILLING_MODE=enforce` で押さえが 0 の有料の provider は動かない。値は非公開の資料 | `0` / `0` |
| `EXT_TIMEOUT` / `EXT_MAX_CONCURRENT` | runner | 外の API の 1 回の時間切れ / 1 台で同時に呼ぶ数(超えたら 503 `ext_busy`) | `8s` / `8` |
| `EXT_RUNNER_URL` | appconfig | 外の API(#1493)で呼ぶ runner の頭(`https://…`。ローカルは `http://localhost:…`)。https の時はその URL を audience にした ID トークンを付ける(runner の invoker に appconfig の SA が要る)。無いと `/v1/apps/{appId}/ext/*`・`/v1/me/connections/*` は 503 `ext_unavailable`(一覧は空) | 無し(off) |
| `EXT_USER_PER_HOUR` | appconfig | 外の API を 1 人が 1 時間に呼べる回数(超えると 429 `rate_limited`)。`0` で数えない | `120` |
| `RUNNER_AUTO_NOTIFY_PAUSE_FOR` | runner(prod だけ) | 上の自動の一時停止の長さ。`48h` のように書く。1 時間未満は起動しない。運営が `moderate resume-notify` で先に解ける([incident-runbook.md 手順 F](../ops/incident-runbook.md#13-手順-f-悪用自動の動き)) | `72h` |
| `RUNNER_NOTIFY_PAUSED` | runner(prod だけ) | `1`(`true` / `on` も可)で通知だけを全体で止める(#339・#346)。ルールは動き、書き込みはする。止めている間の通知は後から送らない | 無し(止めない) |
| `RUNNER_PAUSED` | runner(prod だけ) | `1`(`true` / `on` も可)で自動の動きを全体で止める(#337 の停止スイッチの「全体」)。止めている間に取り出したきっかけは `skipped_paused` で終わらせ、再開しても後からまとめて実行しない | 無し(止めない) |
| `RUNNER_SCHEDULE_MAX_LATE` | runner(prod だけ) | 定期のルール(#342)の実行が期限からこれより遅れたら `skipped_late` にして実行しない(後からまとめて実行しない)。`45m` のように書く。1 分未満は起動しない | `30m` |
| `RUNNER_TICK_INTERVAL` | runner(prod だけ) | ローカル用。`10s` のように書くと、runner が自分でこの間隔で outbox を処理する。本番は Cloud Scheduler の `runner-tick` が毎分 `POST /internal/tick` を呼ぶので入れない | 無し(`/internal/tick` を呼ばれた時だけ) |
| `RUNNER_OUTBOX` | appconfig | `1` で、serverRules(自動の動き)の条件に合う記録の書き込みを、同じトランザクションで outbox(`runner/{APP_ENV}/outbox`)に積む(#341)。時刻のきっかけを持つ(持っていた)DSL の保存では `rules_changed` を同じトランザクションで積み、`GET /dsl` で `runner/{APP_ENV}/apps/*` の `lastOpenedAt` を 1 アプリ 1 日 1 回まで書く(#342)。入れるのは人の承認を得てから、runner-prod と `runner-tick` が動いているのを確かめた後([runner.md](runner.md)。runner が無いまま入れると積まれるだけで動かない)。stg には入れない(#288 の分離まで runner は prod だけ) | 無し(積まない) |
| `AI_SERVER_RULES` | ai | `1` で、「自動の動き」を頼む発話の時に serverRules(自動の動き)を作る呼び出しを足す(#343)。runner と `RUNNER_OUTBOX` が動いていない環境で入れると、作れても動かないルールができる。入れるのは `RUNNER_OUTBOX=1` と同じ時([runner.md](runner.md))。入れると、生成と相談の指示文の「サーバーからのプッシュ通知はできない」も「後の工程で作る・作れる(作成者の Pro)」に替わる(共有データの無い版と「この機能なしで作る」は替えない・`internal/ai/server_push_wording.go`) | 無し(作らない) |
| `AI_ON_DEMAND_RULES` | ai | 過去の JPHACKS 作品の作り方(`jphacks_products*_rules.go` の 28 項目・約 21 KB)を、キャッシュする共通部分から外し、要る時だけキャッシュの後ろの別のブロックで送る(`internal/ai/on_demand_rules.go`)。送る項目は「きっかけの言葉に当たったもの(編集では今のアプリの部品・名前も)」と「小さなモデルの振り分け(`AI_RECIPE_MODEL`・最大 1.5 秒待つ。間に合わない・失敗した時は言葉だけ)」を合わせたもの。`0` で止めて、前と同じく全部を毎回送る(送る内容はバイト単位で前と同じ)。作り方が抜けた・作りが変わったと思ったら、まず `0` にして比べる | 無し(使う) |
| `AI_RECIPE_MODEL` | ai | 上の振り分けに使うモデル。呼び出しは `ai.usage` に `operation=recipes` で 1 行ずつ出る | `claude-haiku-5-5` |
| `PLAN_FREE_APPS` / `PLAN_PRO_APPS` | appconfig | プランごとの上限(#317・#630)。1 人が持てるアプリの数(自分で作ったもの + 参加した共有のアプリ。空のアプリは数えない)。コインで買った枠(`COIN_PRICE_APP_SLOT`)がこの上に足される。`0` で数えない([rate-limit.md](../backend/rate-limit.md#持てるアプリの数630)) | 2 / 5 |
| `PLAN_FREE_RECORDS_PER_APP` / `PLAN_PRO_RECORDS_PER_APP` | appconfig | 1 つのアプリのレコードの件数(その人が見える分: 自分の分 + 共有の分)。共有のアプリは作成者のプランで数える。`0` で数えない | 5000 / 50000 |
| `PLAN_FREE_PHOTOS_PER_APP` / `PLAN_PRO_PHOTOS_PER_APP` | appconfig | 1 つのアプリの写真の枚数(1 枚 300 KB まで = 容量の上限)。`0` で数えない | 500 / 5000 |
| `PLAN_LIMITS_MODE` | appconfig | 上の 3 つの強制。`enforce` = 403 `plan_limit_reached` / `log` = 止めずにログ `plan_limit.would_block` だけ / `off` = 数えない(#317 の前と同じ)。それ以外の値は起動しない | `enforce` |
| `AI_FREE_DAILY_BUDGET_USD` | ai | 全体の遮断(#317)。その日(JST)の AI の費用(`ai.usage` の `est_cost_usd` の合計・USD)がこれを超えたら、Pro でない人の generate・chat を 503 `free_plan_paused` で止める(0:00 JST に自動で戻る)。`0` / 無しで無効([rate-limit.md](../backend/rate-limit.md#全体の遮断317)) | 無し(無効) |
| `AI_FREE_BUDGET_RESUME_DAY` | ai | 上の遮断を手で戻す。`2026-11-07` のように JST の日付を入れると、その日だけ止めない(翌日からはまた効く) | 無し |
| `ALLOWANCE_PRO_RUNS` / `_RECIPIENTS` / `_WRITES` / `_READS` | appconfig・runner(同じ値にする) | Pro の作成者ごとの月(JST)の枠(#630 の 3 段目)。runner は `COIN_BILLING_MODE` が `log` / `enforce` の時に実行・通知の宛先・書き込み・読み取りを月ごとに数え(`rateLimits` の `{APP_ENV}_{yyyymm}_ma_{kind}_{uid}`・共有アプリは作成者の分)、`enforce` では枠を超えた分を止める(下のまとまりを買えれば続ける)。`log` は止めずにログ `coins.would_overflow` だけ。`0` = 数えるだけで止めない。`GET /v1/me/coins` の `allowance` に使った量を出す(appconfig は runner と同じ `APP_ENV`・`RATE_LIMIT_COLLECTION` で読む)。日の上限(`RUNNER_LIMIT_*`)は暴走の歯止めとして先に効く | `0` |
| `CALLS_ENABLED` | appconfig・ai(同じ値にする) | 通話(#447・[calls.md](../backend/features/calls.md))を動かす(`1` / `true`)。appconfig は `POST /v1/apps/{appId}/calls/token` と LiveKit の webhook を出し、ai は callButton を作る。止めている間 appconfig は 404 `calls_disabled`、ai は「通話はできない」と答える。on なのに `LIVEKIT_*` が無いと appconfig は起動しない | 無し(止める) |
| `CALL_BLOCK_MINUTES` | appconfig | 1 回のトークンで押さえる分(この分の前にアプリがもう一度押さえる)。1〜240 | `30` |
| `CALL_HOLD_GRACE` | appconfig | 押さえの期限に足す時間(webhook の遅れの分)。webhook が来なければまとまり + これで戻る | `10m` |
| `CALL_MAX_PARTICIPANTS` | appconfig | 1 つの部屋の人数(2〜50) | `8` |
| `CALL_TOKENS_PER_HOUR` | appconfig | 1 人が 1 時間にトークンを取れる回数(`0` = 数えない) | `30` |
| `CALL_TOKEN_TTL` | appconfig | トークンで部屋に入れる時間(1m〜1h) | `10m` |
| `COIN_BILLING_MODE` | appconfig・ai・runner(同じ値にする) | コイン(#630・[630-token-billing.md](../decisions/630-token-billing.md))。`off` = 何も読み書きしない(今のまま)。`log` = 財布と履歴を作り(最初の分・Pro の毎月の分・パックの購入)、AI の呼び出し(generate・chat・text)が引くはずの量はログ `coins.would_spend` / `coins.would_block` に出すだけ(止めない)。`enforce` = 呼び出しごとに Claude の前にコインを押さえ、終わったら実費(押さえた量まで)を引き、失敗は戻す。足りなければ 402 `coins_insufficient`、同じ `X-Request-Id` の再送は 409 `duplicate_request`。それ以外の値は起動しない | `off` |
| `COIN_ESTIMATE_GENERATE` / `_EDIT` / `_CHAT` / `_TEXT` | appconfig | 押す前に出す「約 N コイン」(`GET /v1/coins/config` の `estimates`)。`0` は出さない | `0` |
| `COIN_HOLD_TTL` | appconfig・ai | 確定されない押さえを戻すまでの時間(`10m` のように書く・1 分以上)。過ぎた押さえは、その人の残高を次に読んだ時(残高の取得・次の押さえ)に戻す | `10m` |
| `COIN_INITIAL_GRANT` | appconfig・ai(同じ値にする) | サインイン済み(匿名でない)アカウントに 1 回だけ足すコイン(最初に財布を読んだ時。ai は AI の呼び出しで押さえる時に読む)。`0` の間は足さず、後で入れると次に読んだ人から足す | `0` |
| `COIN_MIN_PER_CALL` | appconfig・ai | 数える呼び出し 1 回の最小のコイン | `1` |
| `COIN_OVERFLOW_BLOCK_RUNS` / `_RECIPIENTS` / `_WRITES` / `_READS` | appconfig・runner(同じ値にする) | 枠を超えた後にコインで買う 1 まとまりで増える量。作成者が「超えた後のコイン払い」を ON にしていて、今月の `spent` + 値段が月の上限以下なら、runner が 1 まとまりずつ買う(台帳 `spend_overflow`・まとまりごとに 1 行・同じまとまりを二重に買わない)。`0` = その種類は買えない(枠で止める) | `0` |
| `COIN_OVERFLOW_PRICE_RUNS` / `_RECIPIENTS` / `_WRITES` / `_READS` | appconfig・runner(同じ値にする) | 1 まとまりのコイン。**値は非公開の資料で決め、repo に書かない**。`0` = 買えない。`GET /v1/coins/config` の `overflow.blocks` に出す | `0` |
| `COIN_PACKS` | appconfig | コインのパック(消耗型)の `商品id=枚数` をカンマ区切り(例の形: `<id>=<枚数>,<id>=<枚数>`)。この id だけを `POST /v1/me/coins/apple` で受ける(bundle id・環境は `APPSTORE_*` と同じ)。値は非公開の資料・App Store Connect | 無し(売らない) |
| `COIN_PER_CALL_MINUTE` | appconfig | 通話 1 分のコイン(呼び出した本人の財布から引く・#630)。値は非公開の資料 | `0`(数えない) |
| `COIN_PER_USD` | appconfig・ai | 1 回の費用(`est_cost_usd`)をコインに直す率: `ceil(est_cost_usd × COIN_PER_USD)`。**値は非公開の資料で決め、repo に書かない**。`/v1/coins/config` にも出さない | `0`(数えない) |
| `COIN_PRICE_APP_SLOT` | appconfig | 持てるアプリの枠 1 つのコイン(`POST /v1/me/coins/slot`・期限なし・#630)。**値は非公開の資料で決め、repo に書かない**。`0` = 枠を買えない。`COIN_BILLING_MODE=enforce` の時だけ買える(`log` は前に買った枠を数えるだけ・`off` は何も読まない)。`GET /v1/coins/config` の `appSlotCoins` に出す | `0` |
| `COIN_PRO_MONTHLY` | appconfig・ai(同じ値にする) | Pro の請求期間ごとのコイン(期間の終わりに消える・繰り越さない) | `0` |
| `COIN_RESERVE_GENERATE` / `_EDIT` / `_CHAT` / `_TEXT` | appconfig・ai | 呼び出しの前に押さえる量(その処理の上の方の費用)。確定は押さえた量を超えない。`COIN_BILLING_MODE=enforce` の時は 4 つとも 1 以上でないと ai が起動しない(0 のままだとその処理から引けない)。値は非公開の資料 | `0` |
| `COIN_RESERVE_TEXT_TASK` / `_TEXT_LONG` / `_TEXT_IMAGE` | ai | `/v1/text` の重い呼び出し(task・長文と aiChat・写真)だけ別に押さえる量。重さと同じ順で大きい方を使う。履歴の処理は `text` のまま。値は非公開の資料 | 無し(`COIN_RESERVE_TEXT`) |
| `LIVEKIT_API_KEY` / `LIVEKIT_API_SECRET` | appconfig | LiveKit の API キーと秘密(トークンの署名と webhook の確認)。**Secret Manager に置き、repo・ログ・Issue に書かない**(下の「シークレット」) | 無し |
| `LIVEKIT_URL` | appconfig | LiveKit Cloud のプロジェクトの接続先(`wss://<project>.livekit.cloud`)。アプリに返す | 無し |
| `LOG_LEVEL` | 両方 | ログの詳しさ(調査の間だけ `debug` など) | info |

この表に無い変数は、運用で変えないでください。変えたい時は Issue にして、ワークフローか本書に足します。

ワークフローの `--update-env-vars` と上の 2 つの表は `scripts/check-deploy-env.sh` で突き合わせます(デプロイの最初に動き、ずれていればビルド前に止まる)。ワークフローに変数を足す・外す時は、本書の表も同じ PR で直します。

## 変え方

```sh
SERVICE=appconfig-stg   # appconfig-prod / ai-stg / ai-prod
gcloud run services update "$SERVICE" --region asia-northeast1 --project jphacks-kz2601 \
  --update-env-vars KEY=VALUE
```

例: 全体の上限を一時的に下げる。

```sh
gcloud run services update ai-stg --region asia-northeast1 --project jphacks-kz2601 \
  --update-env-vars RATE_LIMIT_GLOBAL_DAILY=200
```

例: 無料プランの生成の遮断(#317)を今日だけ解く(明日からはまた効く)。

```sh
gcloud run services update ai-stg --region asia-northeast1 --project jphacks-kz2601 \
  --update-env-vars AI_FREE_BUDGET_RESUME_DAY=$(TZ=Asia/Tokyo date +%F)
```

値にカンマが入る時(`NOTICE_JSON` など)は、区切り文字を `@` に変えます(`^@^`。gcloud の `topic escaping`)。

```sh
NOTICE='{"id":"2026-11-02-ai","severity":"warning","message":{"ja":"…","en":"…"}}'
gcloud run services update appconfig-stg --region asia-northeast1 --project jphacks-kz2601 \
  --update-env-vars "^@^NOTICE_JSON=$NOTICE"
```

戻す(既定値に戻す)時は消します。

```sh
gcloud run services update appconfig-stg --region asia-northeast1 --project jphacks-kz2601 \
  --remove-env-vars NOTICE_JSON
```

変えた値は次のデプロイの後も残ります。**障害が終わったら消し忘れない**ようにします。

## FIRESTORE_DATABASE(復元時)

**デプロイが決める値のままにします。** 運用の値にすると、復元(#294・[firestore-backup.md](firestore-backup.md))で一時的に向けた DB や、打ち間違えた DB ID が、誰も気づかないまま残り続けます。ワークフローに書いてある値と実際の接続先がずれると、障害の調べ方を誤ります。

- **ふだん**: Environment 変数 `FIRESTORE_DATABASE` は入れない。デプロイのたびに `(default)` に戻る。
- **一時的に切り替える**(数時間・次のデプロイまでで良い時): gcloud で変える。次のデプロイで `(default)` に戻る。

  ```sh
  gcloud run services update appconfig-prod --region asia-northeast1 --project jphacks-kz2601 \
    --update-env-vars FIRESTORE_DATABASE=restore-<YYYYMMDD>
  gcloud run services update ai-prod --region asia-northeast1 --project jphacks-kz2601 \
    --update-env-vars FIRESTORE_DATABASE=restore-<YYYYMMDD>
  ```

- **切り替えたまま運用する**(直したコードのデプロイで戻ると困る時): GitHub → Settings → Environments → `production`(stg なら `staging`)→ Variables に `FIRESTORE_DATABASE` = `restore-<YYYYMMDD>` を入れる。以後のデプロイはこの値を使う。**repo の Variables ではなく Environment の Variables に入れる**(repo に入れると stg / prod の両方が変わる)。
- **戻す**: Environment 変数を消して、`workflow_dispatch` でデプロイする(または `(default)` を gcloud で入れる)。

appconfig と ai は必ず同じ DB に向けます(ai の上限の数え方も同じ DB を使う)。

## シークレット

ai の `ANTHROPIC_API_KEY` は `--set-secrets` のまま(全置換)にします。

- 良い所: ワークフローから外したシークレットは次のデプロイで外れる。要らなくなった鍵の参照が残り続けない。
- 困る所: gcloud で手で足したシークレット参照は次のデプロイで消える。ずっと要るシークレットはワークフローに書く。
- appconfig にはシークレットのフラグを付けていないので、デプロイは appconfig のシークレット参照を変えません。
- 通話(#447)の `LIVEKIT_API_KEY` / `LIVEKIT_API_SECRET` は Secret Manager に置き、appconfig に参照として付けます(`CALLS_ENABLED` を入れる前に。ワークフローに足すか手で付けるかは人が決める)。

## ワークフローから変数を外す時

`--update-env-vars` では、ワークフローから外した変数も Cloud Run に残ります。外す時はどちらかを行います。

- ワークフローの `gcloud run deploy` に、1 回だけ `--remove-env-vars=KEY` を足してデプロイし、次の PR で消す(`--update-env-vars` と同時に使える。remove が先に効く)。
- 人が gcloud で消す: `gcloud run services update <svc> --region asia-northeast1 --project jphacks-kz2601 --remove-env-vars KEY`

`ALLOW_SHARED_PROD_DB` のように安全のための変数は、ワークフローから消さずに `0` を入れて残します(残った `1` で本番の DB を使い続けないように)。

## 今の値を見る

```sh
gcloud run services describe appconfig-stg --region asia-northeast1 --project jphacks-kz2601 \
  --format='yaml(spec.template.spec.containers[0].env)'
```

シークレット参照は `valueFrom.secretKeyRef` として出ます(値そのものは出ません)。
