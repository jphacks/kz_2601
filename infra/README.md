# infra(GCP の構成をコードにする・#307)

prod(`jphacks-kz2601`)と stg(新しいプロジェクト・#288 の案 A2)を、同じ Terraform のコードと環境ごとの変数で作ります。

> **このディレクトリの apply はまだ誰も実行していません。** GCP を変える操作(apply・import)は、人が承認してから行います。

## 目次

1. [Terraform にした理由](#terraform-にした理由)
2. [管理するもの・しないもの](#管理するもの管理しないもの)
3. [state の置き場とロック](#state-の置き場とロック)
4. [stg を空のプロジェクトから作る](#stg-を空のプロジェクトから作る)
5. [既存の prod を取り込む](#既存の-prod-を取り込む)
6. [今の prod の IAM で気になるもの(コード化していない)](#今の-prod-の-iam-で気になるものコード化していない)

## Terraform にした理由

| 観点 | Terraform | gcloud のスクリプト |
|---|---|---|
| 2 回目以降の実行 | 差分だけを適用。何度実行しても同じ結果 | `create` が「既にある」で止まる。`describe` で分岐を書き足す必要がある |
| stg と prod の差分 | `plan` で見える。同じコードに変数だけ変えて流す | 2 つの環境の今の状態を比べる手段が無い |
| 手で変えたもの(ドリフト) | `plan` で見つかる | 気づけない |
| state | 要る(下の GCS バケット)。消えると取り込み直し | 要らない |
| 2 人で回せるか | GCS のロックで同時 apply を防げる。`plan` の結果を PR に貼ってレビューできる | ロック無し。同時に流すと途中の状態がぶつかる |
| 覚えること | HCL と import の書き方 | 無し(gcp-setup.md のコマンドそのまま) |
| 既存の prod | `import` ブロックで取り込み、差分 0 を確かめてから管理に移せる | 既存を壊さないよう全部のコマンドを冪等に書き直す必要がある |

**Terraform を推奨・採用します。** 決め手は「stg と prod の差分が `plan` で見えること」と「何度流しても同じになること」です。stg と prod の 2 つになると、手作業の差分が事故になる(#307 の背景)ので、差分を出す道具が要ります。state の管理が増えますが、GCS バケット 1 つで済み、ロックも付きます。

## 管理するもの・しないもの

ルートモジュール 1 つ(このディレクトリ)を、`envs/<env>.tfvars` で prod / stg に切り替えます。

| 対象 | ファイル | 補足 |
|---|---|---|
| API の有効化 | `main.tf` | `disable_on_destroy = false`。一覧に無い API は止めない |
| SA と IAM | `iam.tf`・`purge.tf` | `run-backend` / `github-deployer` / `purge-job` / `purge-scheduler`。今の prod のバインディングと同じ。**IAM は全部 `*_iam_member`(非権威的)** なので、ここに無いバインディングは消さない |
| WIF | `wif.tf` | 条件は「このリポジトリ かつ 許可した ref」。既定は prod = `refs/heads/main`、stg = `refs/heads/develop`(`wif_allowed_refs` で変更・`[]` で今と同じリポジトリだけ) |
| Artifact Registry | `data.tf` | `backend`(Docker) |
| Secret Manager | `data.tf` | **入れ物だけ**。値(version)は人が入れる。`run-backend` に accessor |
| Firestore | `data.tf` | `(default)`・Native。PITR と削除保護は変数。`prevent_destroy` と `deletion_policy = "ABANDON"` で Terraform からは消せない |
| 消去ジョブ | `purge.tf` | Cloud Run Job `account-purge` と Scheduler `account-purge-daily`(毎日 4:30 JST)。ジョブのイメージは `ignore_changes`(下) |
| 予算アラート | `budget.tf` | 50 / 90 / 100% と予測 100%。課金アカウントは変数 |

**管理しないもの**

| 対象 | 理由 |
|---|---|
| Cloud Run のサービス(`appconfig-*` / `ai-*`) | `deploy-cloudrun.yml` が `gcloud run deploy` で毎回、イメージ・env・secret・`allUsers` の invoker まで全部を設定し直している。Terraform でも持つと、デプロイのたびに `plan` に差分が出て、どちらが正か分からなくなる。`ignore_changes` で image だけ外しても、env(`GIT_SHA` が毎回変わる)や gcloud が付ける注釈で差分が出続ける。サービスの設定の正典は workflow に置く |
| 消去ジョブのイメージ | 初回の作成だけ Terraform で行い、`image` は `ignore_changes`。backend の更新ごとに `gcloud run jobs update --image` で差し替える(account-purge.md の 4) |
| プロジェクトの作成・課金のリンク | 課金アカウントの権限が要る人の作業。prod を誤って作り直す事故も避けたい |
| Firebase(プロジェクトへの追加・iOS アプリ・Auth のプロバイダ・Sign in with Apple) | Apple の鍵やコンソールでの操作が混ざるので人の作業 |
| サービスエージェント・`firebase-adminsdk-fbsvc` | Google / Firebase が自動で作り、権限も自動で付ける |
| GitHub の Environment Variables | `terraform output` の値を人が入れる |

## state の置き場とロック

- state は GCS バケット `jphacks-kz2601-tfstate`(prod のプロジェクト内)に置き、prefix で `env/prod` と `env/stg` を分ける(`envs/<env>.backend.hcl`)。
- バケットは **バージョニングを有効**にし、誤って壊した state を前の版に戻せるようにする。公開アクセス防止・均一なバケットレベルのアクセスも付ける。
- GCS backend はロックを持つ。2 人が同時に apply しても、後の人は「ロック中」で止まる。止まったときに `force-unlock` するのは、相手の apply が本当に終わっているのを確かめてから。
- 2 人の運用: 変更は PR に `terraform plan` の結果を貼ってレビュー → マージ後に 1 人が apply。apply は手元(各自の `gcloud auth application-default login`)から行い、CI からはまだ行わない。
- state には Secret の値は入らない(入れ物だけを管理しているため)。ただしリソースの属性は入るので、バケットの閲覧権限は 2 人に絞る。

バケットの作成(最初の 1 回だけ・人の作業):

```sh
gcloud storage buckets create gs://jphacks-kz2601-tfstate \
  --project=jphacks-kz2601 --location=asia-northeast1 \
  --uniform-bucket-level-access --public-access-prevention
gcloud storage buckets update gs://jphacks-kz2601-tfstate --versioning
```

## stg を空のプロジェクトから作る

### 人の作業(Terraform の前)

| # | やること |
|---|---|
| 1 | プロジェクトを作り、課金をつなぐ(`gcloud projects create <id>` → `gcloud billing projects link`)。ID は #288 の判断 4 |
| 2 | Firebase をプロジェクトに追加し、Authentication を開始(匿名・Apple を有効化)。iOS アプリ(`jp.kz2601.metamon.stg`・#292)を登録して plist を取る |
| 3 | Apple Developer に stg の App ID を足し、Sign in with Apple を付ける |
| 4 | `cp envs/stg.tfvars.example envs/stg.tfvars` して課金アカウントなどを埋める |

### コマンド(Terraform)

```sh
cd infra
terraform init -backend-config=envs/stg.backend.hcl
terraform apply -var-file=envs/stg.tfvars
```

### 人の作業(Terraform の後)

| # | やること |
|---|---|
| 5 | Secret の値を入れる: `printf %s "$KEY" \| gcloud secrets versions add ANTHROPIC_API_KEY --data-file=- --project=<stg>`(stg 用のキー・#289) |
| 6 | GitHub の Environment `staging` の Variables に `terraform output` の 4 つ(`GCP_PROJECT_ID` ほか)を入れる(#291) |
| 7 | develop に push して CI のデプロイが通ることを確かめる |
| 8 | 消去ジョブを使うなら、CI が push したイメージの digest を `purge_job_image` に入れてもう一度 `terraform apply` |

## 既存の prod を取り込む

`imports.prod.tf.example` に、今ある prod のリソースを全部取り込む `import` ブロックを書いてあります。初回の plan で「作り直し・削除が 0」であることを確かめてから apply します。

```sh
cd infra
cp envs/prod.tfvars.example envs/prod.tfvars   # 課金アカウント・import 用の変数を埋める
cp imports.prod.tf.example imports.tf
terraform init -backend-config=envs/prod.backend.hcl
terraform plan -var-file=envs/prod.tfvars -out=prod.tfplan
```

1. **まず差分 0 を確かめる**: `prod.tfvars` で一時的に `wif_allowed_refs = []`、`firestore_pitr_enabled = false`、`firestore_delete_protection = false` にする(今の prod と同じ値)。`purge_job_image` は何を入れても差分にならない(`ignore_changes`)。plan が `N to import, 0 to add, 0 to change, 0 to destroy` になれば正しく取り込めている。
   - provider だけが持つ値(`deletion_protection`・`deletion_policy` など API に無い項目)や、Scheduler の既定ヘッダ(`User-Agent`)が `~ update in-place` に出ることがある。GCP 側の値が変わらないものなので、中身を見て問題無ければ許容する。
2. 差分が出たら、コードを今の状態に合わせる(prod を変えない)。`destroy` や `must be replaced` が 1 つでも出たら apply しない。
3. 差分 0 の plan を apply して取り込む。その後 `imports.tf` を消す。
4. 目標の設定へ寄せるのは別の apply にする。どれも作り直しではなく in-place の変更:
   - Firestore の PITR と削除保護を有効化(#288 の進め方 0・リリースの前・人の承認)
   - WIF を `refs/heads/main` に絞る(#291・審査の期間 〜11/15 の後・人の承認)。絞った後は、develop からも main 以外からも prod へデプロイできなくなる(`workflow_dispatch` で production を選ぶときは main で実行する)

## 今の prod の IAM で気になるもの(コード化していない)

2026-10-30 に `get-iam-policy` などの読み取りで確かめたもの。意図して残すか、消すかを人が判断します。

| バインディング | 気になる点 | 提案 |
|---|---|---|
| Compute の既定 SA(`<番号>-compute@developer`)に `roles/editor`(プロジェクト) | 古い既定の付与。何にも使っていない(Cloud Run も Job も専用 SA)が、プロジェクト全体の編集権限 | 外す(使っていないことを Cloud Logging で確かめてから) |
| Cloud Build の旧 SA(`<番号>@cloudbuild`)に `roles/cloudbuild.builds.builder` | 初回の `gcloud run deploy --source` の名残。今の CI は docker build | 使わないなら外す |
| `firebase-adminsdk-fbsvc` に `roles/iam.serviceAccountTokenCreator`(プロジェクト) | Firebase が自動で付ける。プロジェクト単位なので、この SA は他の SA(deployer を含む)のトークンを作れる | Firebase の既定なので残す。この SA の鍵を作らないこと |
| `github-deployer` は 1 つで、WIF の条件はリポジトリだけ | develop からも main からも prod にデプロイできる(#291) | このコードの既定(prod は main だけ)へ寄せる |
| `github-deployer` に `roles/run.admin`(プロジェクト) | 広いが、`--allow-unauthenticated` に `run.services.setIamPolicy` が要る | 今は残す |
| Artifact Registry `cloud-run-source-deploy` | 初回の `--source` デプロイの名残(数 MB) | 使わないなら消す |
| 4 つの Cloud Run サービスに `allUsers` の `run.invoker` | 意図どおり(アプリ側で Firebase の ID トークンを検証) | CI が付ける。コード化しない |
| `ANTHROPIC_API_KEY` を stg と prod で共有 | stg の使いすぎが prod の上限を食う | stg のプロジェクトに別のキー(#289) |
