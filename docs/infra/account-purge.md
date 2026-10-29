# 退会アカウントの消去ジョブ(#458)

プライバシーポリシー §4「論理削除し、30 日以内に消去します」を実行する仕組みです。`DELETE /v1/me` は論理削除だけをします。`backend/cmd/purge` が、論理削除から 30 日を過ぎたアカウントのデータを毎日消します。

> **本書の手順はまだ実行していません。** GCP の設定は承認を得てから、人が行います。stg と prod は Firestore を共有しているので、ジョブは **1 つの環境(prod)にだけ** 作ります。

## 目次
- [消すもの・残すもの](#消すもの残すもの)
- [処理の流れ](#処理の流れ)
- [設定の手順(人の作業)](#設定の手順人の作業)
- [確認と運用](#確認と運用)
- [rateLimits の TTL(提案)](#ratelimits-の-ttl提案)

## 消すもの・残すもの

| 対象 | 扱い |
| --- | --- |
| `users/{uid}/apps/**`(アプリ・版・自分だけの記録) | 消す |
| `shares/*`(本人が出した共有リンク) | 消す |
| `public_apps/*`(本人の「探す」掲載)と `installs/*` | 消す |
| `release_requests/*`(連絡先を含む) | 消す |
| `app_owners/*`(本人が作ったアプリの記録) | 消す(下の「残す共有アプリ」の分は残す) |
| `public_apps/*/installs/{uid}`(本人が他の人の掲載を入れた記録) | 消す |
| `apps_public/*/members/{uid}`(参加者としての表示名) | 消す |
| `apps_public/.../marks/*/by/{uid}`(いいね・保存) | 消す。件数(`markCounts`)も 1 減らす |
| 本人だけが使っていた共有アプリ(`apps_public/{appId}` と記録) | 消す |
| 他の人が参加・投稿している、本人の共有アプリ(`apps_public/{appId}`・記録・参加者・`app_owners`) | **残す**(その人たちの共有の場のため) |
| 他の人と共有しているアプリへの本人の投稿 | **残す**(ポリシー §4)。ただし表示名(`authorName`)は消す。`author` の uid は残るが、Firebase のアカウントは消えるので、誰かにはたどれない |
| Firebase Auth のユーザー | 消す(アプリが退会時に消しているので、ここは取りこぼしの保険) |
| `users/{uid}` | `{status: "purged", purgedAt}` だけの墓標にする(`deletedAt` は消す)。同じ uid が来ても 410 を返し続け、二度と対象にならない |
| `rateLimits/*_uid_{uid}` | ジョブでは消さない。日付ごとの回数だけで、TTL で消す(下の提案) |

## 処理の流れ

1. `users` を `deletedAt < 今 - 30 日` で古い順に最大 `-limit` 件(既定 50)取ります(単一フィールドの自動インデックスで足り、複合インデックスは要りません)。
2. 1 件ずつ、`PurgeAccount`(データ)→ Firebase Auth の削除 → `MarkAccountPurged`(墓標)の順に行います。
3. 途中で失敗したアカウントは墓標にしないので、次の日にもう一度対象になります。どの段も、残っているものだけを消すので、やり直しても安全です。
4. 1 件でも失敗したら終了コード 1 を返します(Cloud Run Jobs の失敗として残る)。

ログは JSON(Cloud Logging)で、uid の代わりにハッシュ(`account`)と件数だけを書きます。記録の中身や表示名は書きません。

| フラグ | 既定 | 意味 |
| --- | --- | --- |
| `-dry-run` | false | 対象を数えてログに出すだけ。何も消さない |
| `-limit` | 50 | 1 回で扱う最大件数 |
| `-older-than` | 720h | 論理削除からの経過時間。24h 未満は拒否する |
| `-delete-auth` | true | Firebase Auth のユーザーも消す |

ローカル(エミュレータ)で試す: `make emulator` の後に `make -C backend run-purge`(既定は `-dry-run`。`PURGE_FLAGS="-older-than=24h"` などで変える)。

## 設定の手順(人の作業)

前提: [gcp-setup.md](gcp-setup.md) の環境(`PROJECT_ID=jphacks-kz2601`、`REGION=asia-northeast1`、Artifact Registry `backend`)。イメージは `deploy-cloudrun.yml` が作るもの(`/app/purge` を同梱)をそのまま使います。

```bash
export PROJECT_ID=jphacks-kz2601
export REGION=asia-northeast1
export JOB_SA="purge-job@${PROJECT_ID}.iam.gserviceaccount.com"
export SCHED_SA="purge-scheduler@${PROJECT_ID}.iam.gserviceaccount.com"
export IMAGE="${REGION}-docker.pkg.dev/${PROJECT_ID}/backend/backend:<main の SHA>"
```

### 1. ジョブ用のサービスアカウント

appconfig / ai の `run-backend` に Auth の管理権限を足さないよう、専用の SA を作ります。

```bash
gcloud iam service-accounts create purge-job --display-name="Account purge job" --project="$PROJECT_ID"
gcloud projects add-iam-policy-binding "$PROJECT_ID" --member="serviceAccount:${JOB_SA}" --role="roles/datastore.user"
gcloud projects add-iam-policy-binding "$PROJECT_ID" --member="serviceAccount:${JOB_SA}" --role="roles/firebaseauth.admin"
# deployer が「purge-job として」ジョブを作るための actAs(人が作るなら不要)
gcloud iam service-accounts add-iam-policy-binding "$JOB_SA" \
  --member="serviceAccount:github-deployer@${PROJECT_ID}.iam.gserviceaccount.com" --role="roles/iam.serviceAccountUser"
```

### 2. Cloud Run Job を作り、まず dry-run で動かす

```bash
gcloud run jobs create account-purge \
  --project="$PROJECT_ID" --region="$REGION" \
  --image="$IMAGE" --command=/app/purge --args="-dry-run,-older-than=696h" \
  --service-account="$JOB_SA" \
  --set-env-vars="GOOGLE_CLOUD_PROJECT=${PROJECT_ID}" \
  --tasks=1 --max-retries=1 --task-timeout=30m

gcloud run jobs execute account-purge --project="$PROJECT_ID" --region="$REGION" --wait
```

ログに `purge: start`(`candidates` = 対象の数)と `purge: would purge` が出ることを確かめます。問題が無ければ dry-run を外します。

```bash
gcloud run jobs update account-purge --project="$PROJECT_ID" --region="$REGION" --args="-limit=50,-older-than=696h"
```

### 3. Cloud Scheduler で毎日動かす

```bash
gcloud iam service-accounts create purge-scheduler --display-name="Account purge scheduler" --project="$PROJECT_ID"
gcloud run jobs add-iam-policy-binding account-purge --project="$PROJECT_ID" --region="$REGION" \
  --member="serviceAccount:${SCHED_SA}" --role="roles/run.invoker"

gcloud scheduler jobs create http account-purge-daily \
  --project="$PROJECT_ID" --location="$REGION" \
  --schedule="30 4 * * *" --time-zone="Asia/Tokyo" \
  --http-method=POST \
  --uri="https://run.googleapis.com/v2/projects/${PROJECT_ID}/locations/${REGION}/jobs/account-purge:run" \
  --oauth-service-account-email="$SCHED_SA"
```

毎日 4:30(JST)に動きます。消去は論理削除から最長で 31 日後になるので、ポリシーの「30 日以内」に収めるため、上の手順ではジョブに `-older-than=696h`(29 日)を渡しています。

### 4. イメージの更新

ジョブは作った時のイメージのままです。`deploy-cloudrun.yml` は今はジョブを更新しないので、backend を変えたら次で揃えます(CI に足す場合は deployer SA に 1 の actAs が要る)。

```bash
gcloud run jobs update account-purge --project="$PROJECT_ID" --region="$REGION" --image="$IMAGE"
```

## 確認と運用

| 確認 | 方法 |
| --- | --- |
| 毎日動いているか | `gcloud run jobs executions list --job=account-purge --region=$REGION --project=$PROJECT_ID` |
| 何件消したか | Cloud Logging で `jsonPayload.msg="purge: done"`(`purged` / `failed`) |
| 失敗 | `jsonPayload.msg="purge: account failed"`。同じ `account` が翌日も失敗していないか見る |
| 止めたい | `gcloud scheduler jobs pause account-purge-daily --location=$REGION --project=$PROJECT_ID` |

## rateLimits の TTL(提案)

`rateLimits/*` は日付ごとの回数で、`expireAt`(その日の終わり + 1 日)を持っています([rate-limit.md](../backend/rate-limit.md))。2026-10-30 に読み取りで確かめたところ、TTL ポリシーは **まだ有効になっていません**(`gcloud firestore fields ttls list` が 0 件)。uid を含む文書が残り続けるので、次を 1 回実行することを提案します(未実行)。

```bash
gcloud firestore fields ttls update expireAt --collection-group=rateLimits --enable-ttl --project="$PROJECT_ID"
```

TTL の削除は期限から最大 24 時間ほど遅れることがあります。それでも退会から 30 日以内には消えます。
