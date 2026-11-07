# runner を本番で動かす(自動の動き・M4)

runner(`backend/cmd/runner`・#341)は、appconfig が outbox(`runner/{APP_ENV}/outbox`)に積んだきっかけを、Cloud Scheduler の `runner-tick`(毎分 `POST /internal/tick`)で処理するサービスです。本書は本番(prod)に出す手順・確かめ方・止め方・戻し方です。

> **本書のコマンドはどれも GCP の設定を変えます。** 人の承認を得てから、人が行います。
> [docs/00_decisions.md「M4」](../00_decisions.md#m4-バックエンドの自動構築2026-11-06-決定)のとおり、runner は専用の SA で動かし、**#288 の分離までは prod だけ**で動かします(stg には出さない・stg の appconfig に `RUNNER_OUTBOX` も入れない)。

## 目次
- [全体の形](#全体の形)
- [既定では何も起きない](#既定では何も起きない)
- [出す手順](#出す手順)
- [動いたかの確かめ方](#動いたかの確かめ方)
- [プッシュ通知を端末に届ける](#プッシュ通知を端末に届ける)
- [止め方](#止め方)
- [戻し方](#戻し方)
- [発表で見せる場合の最小の手順](#発表で見せる場合の最小の手順)

## 全体の形

| もの | 作る所 | 中身 |
|---|---|---|
| SA `runner` | `infra/runner.tf` | runner の実行 SA。`roles/datastore.user` と `roles/firebaseauth.viewer`(送り手の作成時刻を読むだけ・#339 の 24 時間)。AI の鍵は持たない(#337 の B) |
| SA `runner-scheduler` | `infra/runner.tf` | Scheduler が OIDC のトークンを作る SA。`runner-prod` にだけ `roles/run.invoker` |
| deployer の actAs | `infra/runner.tf` | `github-deployer` が「runner として」デプロイする `roles/iam.serviceAccountUser` |
| Scheduler `runner-tick` | `infra/runner.tf` | 毎分(Asia/Tokyo)・`POST <runner の URL>/internal/tick`・OIDC・期限 90 秒・再試行なし(次の分が拾う) |
| Firestore の TTL | `infra/runner.tf` | collection group `done` / `outbox` / `rule_status` の `expireAt`(7 日 / 30 日 / 30 日・#341) |
| Secret の入れ物(任意) | `infra/runner.tf` | `runner_secret_ids`。APNs の鍵(`APNS_AUTH_KEY`)を入れる時だけ。読めるのは `runner` だけ(`run-backend` は読めない)。注入は GitHub の変数 `RUNNER_APNS=1` の時だけ([下](#プッシュ通知を端末に届ける)) |
| サービス `runner-prod` | `.github/workflows/deploy-cloudrun.yml` | 同じイメージを `--command=/app/runner`・`--no-allow-unauthenticated`・最大 2 台・`--timeout=90`・SA `runner` |
| appconfig の `RUNNER_OUTBOX=1` | 人(gcloud) | 書き込みと同じトランザクションで outbox に積む([runtime-config.md](runtime-config.md#運用で変える値)) |
| ai の `AI_SERVER_RULES=1` | 人(gcloud) | 「自動の動き」を頼む発話で serverRules を作る(#343) |

`schedules` / `stops` は今のコードが `expireAt` を書かないので TTL の対象にしていません(定期のルールと停止は消えては困る)。`rateLimits`(runner の上限の数も入る)の TTL は [account-purge.md の提案](account-purge.md#ratelimits-の-ttl提案)のまま別に決めます。

## 既定では何も起きない

| スイッチ | 既定 | 既定のときの動き |
|---|---|---|
| Terraform の `runner_enabled` | `false` | SA・IAM・Scheduler・TTL・Secret を 1 つも作らない |
| Terraform の `runner_service_url` | `""` | Scheduler と invoker を作らない(`runner_enabled = true` でも) |
| GitHub の変数 `RUNNER_DEPLOY` | 無し | deploy-cloudrun.yml の runner の段を飛ばす(stg は値に関係なく飛ばす) |
| appconfig の `RUNNER_OUTBOX` | 無し | outbox に積まない |
| ai の `AI_SERVER_RULES` | 無し | 自動の動きを作らない |

`runner_enabled = true` を stg の変数で入れると、plan の段で止まります(`precondition`)。

## 出す手順

前提: runner の入った版が `main` にある(prod のデプロイと WIF は `main` だけ)。以下 `PROJECT_ID=jphacks-kz2601`・`REGION=asia-northeast1`。

1. **承認を得る**(この PR のレビュー + 本番に出す判断)。

2. **Terraform(1 段目): SA・actAs・TTL を作る。** `infra/envs/prod.tfvars` に `runner_enabled = true` を入れる。

   ```sh
   cd infra
   terraform init -backend-config=envs/prod.backend.hcl
   terraform plan -var-file=envs/prod.tfvars -out=runner1.plan   # 足されるのが runner の分だけか確かめる
   terraform apply runner1.plan
   ```

   plan で見るもの: 足されるのは `google_service_account.runner*`・`google_project_iam_member.runner_datastore`・`google_project_iam_member.runner_firebaseauth_viewer`・`google_service_account_iam_member.deployer_actas_runner`・`google_firestore_field.runner_ttl["…"]` だけ。`google_firestore_field` に `index_config` の差分が出ていない(インデックスを変えない)。

3. **GitHub の変数を入れる。** Settings → Environments → `production` → Variables に `RUNNER_DEPLOY` = `1`(repo の Variables でもよい。stg はどちらでも飛ばす)。

4. **デプロイする。** Actions → deploy-cloudrun → Run workflow → Branch `main`・environment `production`(または次の main への push)。appconfig-prod / ai-prod も同じ版で入れ直されます。Job summary に `runner-prod` の行が出ればできています。

5. **Terraform(2 段目): Scheduler と invoker を作る。**

   ```sh
   gcloud run services describe runner-prod --region "$REGION" --project "$PROJECT_ID" --format='value(status.url)'
   ```

   出た URL を `prod.tfvars` の `runner_service_url` に入れて、もう一度 plan → apply。足されるのは `google_cloud_run_v2_service_iam_member.runner_scheduler_invoker` と `google_cloud_scheduler_job.runner_tick` だけ。apply する人は `runner-scheduler` の actAs(owner なら持っている)が要ります。

6. **runner が空の outbox を回せているか確かめる**([下](#動いたかの確かめ方)の 1〜3)。

7. **appconfig-prod に `RUNNER_OUTBOX=1` を入れる**(ここから、ルールに合う書き込みが outbox に積まれる)。

   ```sh
   gcloud run services update appconfig-prod --region "$REGION" --project "$PROJECT_ID" --update-env-vars RUNNER_OUTBOX=1
   ```

8. **ai-prod に `AI_SERVER_RULES=1` を入れる**(ここから、話しかけて自動の動きを作れる。発話ごとに Claude の呼び出しが 1 回増える)。

   ```sh
   gcloud run services update ai-prod --region "$REGION" --project "$PROJECT_ID" --update-env-vars AI_SERVER_RULES=1
   ```

9. **自動の動きを 1 つ作って確かめる**([下](#動いたかの確かめ方)の 4)。

`RUNNER_OUTBOX` と `AI_SERVER_RULES` は運用の値なので、次のデプロイの後も残ります。

## 動いたかの確かめ方

1. **/health**(非公開なので ID トークンを付ける。呼ぶ人に `run.invoker` が要る。owner は持っている)

   ```sh
   URL="$(gcloud run services describe runner-prod --region "$REGION" --project "$PROJECT_ID" --format='value(status.url)')"
   curl -fsS -H "Authorization: Bearer $(gcloud auth print-identity-token)" "$URL/health"
   # {"status":"ok","env":"prod","firestoreDatabase":"(default)","version":"<GIT_SHA 7 文字>"}
   curl -s -o /dev/null -w '%{http_code}\n' "$URL/health"   # トークン無しは 403(公開されていない)
   ```

2. **tick を 1 回すぐ走らせる**(1 分待たなくてよい)

   ```sh
   gcloud scheduler jobs run runner-tick --location "$REGION" --project "$PROJECT_ID"
   gcloud scheduler jobs describe runner-tick --location "$REGION" --project "$PROJECT_ID" --format='yaml(state,lastAttemptTime,status)'
   ```

   `status` が空(= 成功)で `state: ENABLED`。`status.code` が 7(PERMISSION_DENIED)/ 16(UNAUTHENTICATED)なら invoker か audience(`runner_service_url` と `status.url` の食い違い)を見る。

3. **起動と tick のログ**

   ```sh
   gcloud logging read 'resource.type="cloud_run_revision" AND resource.labels.service_name="runner-prod" AND ("runner: starting" OR "runner: tick")' \
     --project "$PROJECT_ID" --freshness=15m --limit=20 --format='value(timestamp,jsonPayload.message,jsonPayload)'
   ```

   `runner: starting` に `outbox_env=prod`・`paused=false`。`runner: tick` は積まれたきっかけがある時だけ `claimed` が 1 以上。

4. **自動の動きが走ったか**(7・8 の後。Pro のアカウントで、例「記録したら通知して」のアプリを作り、記録を 1 件足す)

   ```sh
   gcloud logging read 'resource.labels.service_name="runner-prod" AND jsonPayload.event="runner.run"' \
     --project "$PROJECT_ID" --freshness=15m --limit=10 --format=json
   ```

   `outcome` が `ok` なら動いている。`skipped_plan` は作成者が Pro でない(`PRO_GATE_MODE`・[pro.md](../backend/features/pro.md))、`skipped_paused` / `skipped_stopped` は止めている、`skipped_quota` / `skipped_global` は上限([runtime-config.md](runtime-config.md#運用で変える値)の `RUNNER_LIMIT_*`)。使った量は `jsonPayload.event="runner.usage"`。アプリの中の「実行の履歴」(`rule_status`)にも出る。

APNs の鍵を入れるまで([下](#プッシュ通知を端末に届ける))、通知は宛先を数えてログに形だけを出します(端末には届かない)。`runner: starting` の `apns=false` がその状態です。

## プッシュ通知を端末に届ける

runner は APNs の鍵が無い間、通知をログに出すだけです(`runner: starting` の `apns=false`)。鍵を入れると、宛先の判定(参加者・ブロック・同意・上限・停止・作成から 24 時間)を通った人の端末(`users/{uid}/devices`・iOS が `PUT /v1/me/devices/{installationId}` で登録)に送ります。APNs が 410 か 400 `BadDeviceToken` を返したトークンは消します。**鍵の値は repo・GitHub・チャットに置きません。**

前提: [出す手順](#出す手順)の 2(`runner_enabled = true`)が済んでいる。iOS 側で App ID の Push Notifications が有効で、`aps-environment` の入った版が配られている(#683)。

1. **APNs の鍵を作る**(Apple Developer の Account Holder / Admin)。Certificates, Identifiers & Profiles → Keys → +・「Apple Push Notifications service (APNs)」に印 → Configure で Environment は「Sandbox & Production」→ 作って `.p8` を 1 回だけダウンロードする。画面の **Key ID** と、Membership の **Team ID** を控える。`.p8` は再ダウンロードできないので、Secret Manager に入れたら手元のファイルは消す。

2. **Secret の入れ物を作る。** `infra/envs/prod.tfvars` に `runner_secret_ids = ["APNS_AUTH_KEY"]` を入れて plan → apply。足されるのは `google_secret_manager_secret.runner["APNS_AUTH_KEY"]` と `google_secret_manager_secret_iam_member.runner_accessor["APNS_AUTH_KEY"]` だけ。

3. **鍵の値を入れる**(手元のファイルから直接。値を画面やシェルの履歴に出さない)。

   ```sh
   gcloud secrets versions add APNS_AUTH_KEY --project "$PROJECT_ID" --data-file=<ダウンロードした .p8 のパス>
   ```

4. **鍵以外の値を入れる**(秘密ではないが repo には書かない。`--update-env-vars` なので次のデプロイの後も残る)。

   ```sh
   gcloud run services update runner-prod --region "$REGION" --project "$PROJECT_ID" \
     --update-env-vars APNS_KEY_ID=<Key ID>,APNS_TEAM_ID=<Team ID>,APNS_TOPIC=jp.kz2601.metamon,RUNNER_APNS_ENVIRONMENT=production
   ```

   `APNS_TOPIC` は配る版の bundle id(`ios/project.yml` の `PRODUCT_BUNDLE_IDENTIFIER`。`METAMON_BUNDLE_ID_SUFFIX` を付けた版ならその値)。`RUNNER_APNS_ENVIRONMENT` は TestFlight・App Store の版なら `production`、Xcode から入れた開発署名の版だけなら `sandbox`。1 つの runner は片方の環境の端末にだけ送ります。

5. **GitHub の変数 `RUNNER_APNS` = `1`** を Environment `production` に入れて、deploy-cloudrun を `main`・`production` で実行する(`APNS_AUTH_KEY` が `--set-secrets` で注入される。`RUNNER_APNS` が無いデプロイは `--clear-secrets` で外す)。

6. **確かめる。** `runner: starting` のログに `apns=true`・`apns_environment`・`account_age=true` が出る。テスト用のアカウント 2 つ(どちらも作って 24 時間以上・作成者は Pro)で共有のアプリに参加し、受け取る側で通知を許可して、もう一方が記録を足す。`runner.usage` の `notify_recipients` が 1 以上で、端末に届けばできています。届かない時は `runner.run` の `notifies`(`skipped` の `new_account` / `no_consent` / `no_device` / `unregistered` / `send_failed`)を見る。`send_failed` が全部なら Key ID・Team ID・topic・環境の食い違いを疑う。

止める: 全体の通知は [止め方](#止め方) の `RUNNER_NOTIFY_PAUSED=1`。鍵を外す時は `RUNNER_APNS` を消してデプロイ(`--clear-secrets`)→ ログだけに戻る。鍵が漏れた疑いの時は Apple Developer で鍵を Revoke し、新しい鍵で 1・3・4 をやり直す。

## 止め方

軽い順。どれも運営の操作で、作成者は解けません([incident-runbook.md 手順 F](../ops/incident-runbook.md#13-手順-f-悪用自動の動き))。

| 何を止める | やり方 | 止めている間 |
|---|---|---|
| 1 つのアプリ / ルール / 通知 | `go run ./cmd/moderate stop-app <appId> -env prod -reason "…" -allow-prod-db -yes`(`stop-rule`・`pause-notify` も。解除は `resume-*`)。backend で実行 | そのアプリだけ `skipped_stopped` |
| 全体の通知だけ | `gcloud run services update runner-prod --region "$REGION" --project "$PROJECT_ID" --update-env-vars RUNNER_NOTIFY_PAUSED=1` | ルールは動く。通知は後から送らない |
| 全体 | 同じく `--update-env-vars RUNNER_PAUSED=1` | 取り出したきっかけは `skipped_paused`。再開しても後からまとめて実行しない |
| tick そのもの | `gcloud scheduler jobs pause runner-tick --location "$REGION" --project "$PROJECT_ID"`(戻すは `resume`)。続けるなら `runner_tick_paused = true` も tfvars に入れる(次の apply で戻らないように) | きっかけは outbox に残る。再開すると古い順に処理する(定期のルールは `RUNNER_SCHEDULE_MAX_LATE` より遅れた分を捨てる) |

止めた事実をすぐ確かめる: `RUNNER_PAUSED` は新しいリビジョンに移った時点で効く。`runner: starting` のログに `paused=true` が出る。解除は `--remove-env-vars RUNNER_PAUSED`。

## 戻し方

上から順に行います(きっかけを積むのを先に止める)。

1. ai-prod: `gcloud run services update ai-prod --region "$REGION" --project "$PROJECT_ID" --remove-env-vars AI_SERVER_RULES`
2. appconfig-prod: `gcloud run services update appconfig-prod --region "$REGION" --project "$PROJECT_ID" --remove-env-vars RUNNER_OUTBOX`
3. tick を止める: `gcloud scheduler jobs pause runner-tick …`(上の表)
4. GitHub の変数 `RUNNER_DEPLOY` を消す(次のデプロイから runner の段を飛ばす。サービスは消えない)
5. 必要ならサービスを消す: `gcloud run services delete runner-prod --region "$REGION" --project "$PROJECT_ID"`
6. Terraform: `runner_service_url = ""` で apply(Scheduler と invoker が消える)→ `runner_enabled = false` で apply(SA・actAs・TTL が消える)。**TTL を消すと `expireAt` を過ぎた outbox などが残り続けます。** 戻した後もデータを消したいなら、TTL だけは残す(`runner_ttl_collection_groups` はそのままにして、`terraform state rm 'google_firestore_field.runner_ttl'` で管理から外してから `runner_enabled = false`)。

既に作られたルール(DSL の serverRules)はアプリに残ります。2 の後は書き込みがきっかけにならないので動きません。

## 発表で見せる場合の最小の手順

前提: 発表の版が `main` に入っている・デモ用のアカウントが Pro(Sandbox の購入で可)・承認済み。

| 順 | やること | 目安 |
|---|---|---|
| 1 | `runner_enabled = true` で plan → apply | 3 分 |
| 2 | Environment `production` に `RUNNER_DEPLOY=1` → deploy-cloudrun を `main`・`production` で実行 | 10〜12 分(ビルドとスモーク) |
| 3 | `runner_service_url` を入れて plan → apply | 2 分 |
| 4 | `/health` と `gcloud scheduler jobs run runner-tick` で確かめる | 2 分 |
| 5 | `RUNNER_OUTBOX=1`(appconfig-prod)と `AI_SERVER_RULES=1`(ai-prod) | 2 分 |
| 6 | デモのアプリで 1 回通しで試す(`runner.run` が `ok`) | 3〜5 分 |

**合計 25 分前後。** 発表の当日ではなく前日までに済ませ、1 回通しで試しておきます。見せ終わって続けない場合は[戻し方](#戻し方)の 1〜3(数分)で、積むのと動くのを止められます。

見せる時の注意:
- tick は毎分なので、書き込みから動きまで最大 1 分ほどかかる。待てない時は `gcloud scheduler jobs run runner-tick` を手元で叩く。
- 通知を端末に届けるには[プッシュ通知を端末に届ける](#プッシュ通知を端末に届ける)を先に済ませる。Xcode から入れた版は `RUNNER_APNS_ENVIRONMENT=sandbox` でないと届かない。鍵が無ければ「書き込み」や「実行の履歴」の方を見せる。
- 他人宛ての通知は、送り手(記録を足す人)のアカウントが作って 24 時間を過ぎていないと出ない(#339)。デモの送り手は前日までに作っておく。
- 作成者が Pro でないと `skipped_plan` で動かない(`PRO_GATE_MODE=enforce` が既定)。
