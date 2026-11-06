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
| `FEEDBACK_DAILY_LIMIT` | appconfig | 1 人 1 日のフィードバック送信の上限。`0` で数えない | 5 |
| `AI_PAUSED` | ai | `1`(`true` / `on` も可)で AI の生成だけを止める(#511)。generate(`mode=undo` を除く)・chat・text が Claude を呼ばずに 503 `ai_paused` を返す。AI を止める時はこれを使う([ai-pause.md](ai-pause.md)) | 無し(止めない) |
| `PRO_GATE_MODE` | ai・appconfig・runner(3 つとも同じ値にする) | 有料の機能(共有データ・自動の動き・アプリの中のオンラインの AI)の強制(#316・[pro.md](../backend/features/pro.md))。`enforce` = Pro でなければ 402 `pro_required` / runner は `skipped_plan`。`log` = 止めずに、止めるはずだった事実を構造化ログ `pro_gate.would_block`(`where`・`uid_hash`・`features`・`blocked:false`)に出すだけ(runner は private のアプリを #316 の前の無料の枠で動かす)。`off` = 見ない(#316 の前と同じ)。共有データの自動の動きは #338 のとおりどの値でも Pro だけ。それ以外の値は起動しない(打ち間違いで外れないように)。段階的に出す時は `log` で `would_block` の量を見てから `enforce` | `enforce` |
| `RATE_LIMIT_GLOBAL_DAILY` | ai | 環境全体の 1 日の AI 上限(費用ブレーカー)([rate-limit.md](../backend/rate-limit.md))。AI を止める用途には `AI_PAUSED` を使う。`1` にして止めるのは、`AI_PAUSED` の入っていない版が動いている時の予備だけ([incident-runbook.md §5](../ops/incident-runbook.md#5-手順-a-claude-api-の障害上限)) | 2000 |
| `RATE_LIMIT_ANON_DAILY` / `_USER_DAILY` / `_PRO_DAILY` / `_IP_DAILY` | ai | 利用者ごと・IP ごとの 1 日の上限([rate-limit.md](../backend/rate-limit.md)) | 30 / 60 / 300 / 300 |
| `RATE_LIMIT_WEIGHT_*` | ai | 呼び出し 1 回で数える重み([rate-limit.md](../backend/rate-limit.md)) | 3 / 2 / 1 / 1 |
| `RUNNER_GLOBAL_DAILY` | runner(prod だけ) | 環境全体の 1 日の自動の動きの実行の上限(費用の遮断・[#338](../decisions/338-server-feature-cost-model.md#上限案))。`0` で数えない | 10000 |
| `RUNNER_LIMIT_*` | runner(prod だけ) | アプリ・作成者・きっかけの人・受け取る人ごとの 1 日の上限と、1 回の実行の形の上限(`backend/internal/runner/quota/config.go` の `ConfigFromEnv`・#345)。`_FREE` / `_PRO` の組は無料 / Pro。**自動の動きは Pro だけ(#316)なので `_FREE` は今は使われない**(Pro でない作成者のルールは数える前に `skipped_plan`)。`0` で数えない | 実行 100 / 2000(アプリ)・300 / 3000(作成者)、書き込み 100 / 2000、通知の宛先 50 / 1000、受け取る人 20(1 アプリ)・100(合計)、きっかけの人 200、1 回の宛先 1 / 200・書き込み 1 / 10、定期のルール 1 / 5 |
| `RUNNER_AUTO_NOTIFY_PAUSE_REPORTERS` | runner(prod だけ) | 1 つのアプリ(アプリそのもの・その中の投稿)を 24 時間でこの人数(別々の人)が通報したら、そのアプリの通知を自動で一時停止する(#339・#346)。止めるのは通知だけ(非公開化などは人)。`0` で自動では止めない | 3 |
| `RUNNER_AUTO_NOTIFY_PAUSE_FOR` | runner(prod だけ) | 上の自動の一時停止の長さ。`48h` のように書く。1 時間未満は起動しない。運営が `moderate resume-notify` で先に解ける([incident-runbook.md 手順 F](../ops/incident-runbook.md#13-手順-f-悪用自動の動き)) | `72h` |
| `RUNNER_NOTIFY_PAUSED` | runner(prod だけ) | `1`(`true` / `on` も可)で通知だけを全体で止める(#339・#346)。ルールは動き、書き込みはする。止めている間の通知は後から送らない | 無し(止めない) |
| `RUNNER_PAUSED` | runner(prod だけ) | `1`(`true` / `on` も可)で自動の動きを全体で止める(#337 の停止スイッチの「全体」)。止めている間に取り出したきっかけは `skipped_paused` で終わらせ、再開しても後からまとめて実行しない | 無し(止めない) |
| `RUNNER_SCHEDULE_MAX_LATE` | runner(prod だけ) | 定期のルール(#342)の実行が期限からこれより遅れたら `skipped_late` にして実行しない(後からまとめて実行しない)。`45m` のように書く。1 分未満は起動しない | `30m` |
| `RUNNER_TICK_INTERVAL` | runner(prod だけ) | ローカル用。`10s` のように書くと、runner が自分でこの間隔で outbox を処理する。本番は Cloud Scheduler の `runner-tick` が毎分 `POST /internal/tick` を呼ぶので入れない | 無し(`/internal/tick` を呼ばれた時だけ) |
| `RUNNER_OUTBOX` | appconfig | `1` で、serverRules(自動の動き)の条件に合う記録の書き込みを、同じトランザクションで outbox(`runner/{APP_ENV}/outbox`)に積む(#341)。時刻のきっかけを持つ(持っていた)DSL の保存では `rules_changed` を同じトランザクションで積み、`GET /dsl` で `runner/{APP_ENV}/apps/*` の `lastOpenedAt` を 1 アプリ 1 日 1 回まで書く(#342)。入れるのは人の承認を得てから、runner-prod と `runner-tick` が動いているのを確かめた後([runner.md](runner.md)。runner が無いまま入れると積まれるだけで動かない)。stg には入れない(#288 の分離まで runner は prod だけ) | 無し(積まない) |
| `AI_SERVER_RULES` | ai | `1` で、「自動の動き」を頼む発話の時に serverRules(自動の動き)を作る呼び出しを足す(#343)。runner と `RUNNER_OUTBOX` が動いていない環境で入れると、作れても動かないルールができる。入れるのは `RUNNER_OUTBOX=1` と同じ時([runner.md](runner.md)) | 無し(作らない) |
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
