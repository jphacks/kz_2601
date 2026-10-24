# GCP 構築手順書(JPHACKS 2026 / kz_2601)

> 正典は `docs/00_decisions.md`。本書はそれを **gcloud / firebase CLI で手を動かして実現する runbook**。
> 操作者: @ko-tarou。Terraform なし。所要目安: Step 0〜3 で 60〜90 分、Step 4 で 30 分。
>
> 記法
> - **☐ あなたがやる** … ブラウザ/コンソール/Apple Developer 等、人が操作する箇所
> - **▶ 実行** … そのままターミナルに貼れるコマンド
> - **要確認** … 執筆時点で裏取りしきれていない箇所(実行時に画面の文言で判断)
>
> 前提: Mac に gcloud SDK 542 / firebase CLI 15.x / Go 1.24 / Docker(任意) が入っている。GCP への操作は **すべてあなたのアカウントで** 行う(Claude Code は実行しない)。

> **進捗(2026-10-24)**: Step 0(0-1〜0-5)は Claude Code が実行済み。プロジェクト `jphacks-kz2601`(番号 `378953239426`)作成・課金リンク・API 有効化・SA 2つ・Artifact Registry `backend` 作成済み。
>
> **進捗(2026-10-24 午後・Claude Code 実行済み)**: Step 1-1 / 1-2 / 1-4、Step 2、Step 3、Step 4-1〜4-3、8-2(予算)まで完了。**残りは Step 1-3(Sign in with Apple = あなたの手作業)、4-5(CI 動作確認)、Step 5(ローカル)**。
>
> | 項目 | 実値 |
> |---|---|
> | Firebase コンソール | https://console.firebase.google.com/project/jphacks-kz2601/overview |
> | Firebase Auth(プロバイダ) | https://console.firebase.google.com/project/jphacks-kz2601/authentication/providers … **匿名認証は有効化済み**(REST `projects.updateConfig`)。Apple は未設定(1-3 を実施) |
> | Firebase iOS アプリ | 表示名 `Metamon` / App ID `1:378953239426:ios:6106ede996c98d1f70dd9b` / Bundle ID `jp.kz2601.metamon`。`GoogleService-Info.plist` は repo 外 `~/Desktop/JPHACKS2026/secrets/` に書き出し済み(iOS ターゲット直下へコピーして使う。コミット禁止) |
> | Cloud Run `appconfig-stg` | https://appconfig-stg-378953239426.asia-northeast1.run.app (旧形式 https://appconfig-stg-zgciuf3iha-an.a.run.app) |
> | Cloud Run `ai-stg` | https://ai-stg-378953239426.asia-northeast1.run.app (旧形式 https://ai-stg-zgciuf3iha-an.a.run.app)。`--command=/app/ai`、`ANTHROPIC_API_KEY` を Secret 注入済み |
> | ヘルスチェック実測 | **両サービスとも `/healthz` は Google Front End が HTML 404 を返し、コンテナに届かない**(リクエストログに残らない。`/`・`/foo`・`/healthz/` は Go の `404 page not found` が返るのでコンテナ自体は稼働中・ローカル起動では `/healthz` 200)。`*.run.app` で `/healthz` が予約パス扱いになっている模様。**backend のヘルスパスを `/health`(または `/api/healthz`)に変え、`deploy-cloudrun.yml` の curl も合わせる**こと(要対応・backend 担当) |
> | Cloud Run コンソール | https://console.cloud.google.com/run?project=jphacks-kz2601 |
> | WIF プロバイダ | `projects/378953239426/locations/global/workloadIdentityPools/github/providers/github`(条件 `assertion.repository == 'jphacks/kz_2601'`) |
> | GitHub Variables(repo 単位) | `GCP_PROJECT_ID` / `GCP_REGION` / `GCP_WORKLOAD_IDENTITY_PROVIDER` / `GCP_SERVICE_ACCOUNT`(`.github/workflows/deploy-cloudrun.yml` の変数名に合わせた。本書 4-3 の `GCP_WIF_PROVIDER` / `GCP_DEPLOY_SA` / `GCP_RUNTIME_SA` は旧案) |
> | GitHub Environments | `staging` / `production` 作成済み |
> | 予算 | `kz2601 monthly` 4,500 JPY/月(課金アカウントが JPY のため。50/90/100%+予測100%) |
>
> **既知の不一致(GitHub 整備担当と要すり合わせ)**: 現行 `deploy-cloudrun.yml` は (a) サービス名が `<SERVICE_PREFIX>-<cmd>`(既定 `morph-appconfig` / `morph-ai`)で stg/prod の区別が無く、本書の `appconfig-stg` 等と別名になる、(b) Secret 名が `anthropic-api-key` だが実体は `ANTHROPIC_API_KEY`、(c) buildpacks 前提(`GOOGLE_BUILDABLE`)だが `backend/Dockerfile` があると Dockerfile が優先され、`--command=/app/ai` が無い ai はappconfig バイナリで起動してしまう。本書 4-4 の YAML に寄せるのを推奨。
>
> **zsh で `#` 行を貼ると `command not found: #` が出る**のは無害だが、気になるなら先に `setopt interactivecomments` を実行しておく。
>
> **注意**: プロジェクトの表示名(`--name`)に `_` は使えない(ID だけでなく表示名にも文字種制限がある)。

## 目次
1. [全体像](#1-全体像)
2. [Step 0 事前準備](#2-step-0-事前準備)
3. [Step 1 Firebase(Firestore / Auth / iOS アプリ登録)](#3-step-1-firebase)
4. [Step 2 Secret Manager](#4-step-2-secret-manager)
5. [Step 3 Cloud Run 初回デプロイ](#5-step-3-cloud-run-初回デプロイ)
6. [Step 4 GitHub Actions からの自動デプロイ(WIF)](#6-step-4-github-actions-からの自動デプロイ)
7. [Step 5 ローカル開発](#7-step-5-ローカル開発)
8. [コスト・無料枠とガードレール](#8-コスト無料枠とガードレール)
9. [チェックリスト](#9-チェックリスト)
10. [トラブルシュート](#10-トラブルシュート)
11. [付録: 名前の一覧](#11-付録-名前の一覧)

---

## 1. 全体像

```mermaid
flowchart LR
  subgraph Dev["開発者 / GitHub"]
    GH["GitHub Actions<br/>(deploy-backend.yml)"]
    Mac["Mac<br/>gcloud / firebase CLI"]
  end

  subgraph Apple["Apple"]
    AppleID["Sign in with Apple<br/>(Apple Developer: App ID / Key)"]
  end

  subgraph GCP["GCP プロジェクト (asia-northeast1)"]
    WIF["Workload Identity<br/>Federation"]
    AR["Artifact Registry<br/>backend/ イメージ"]
    subgraph Run["Cloud Run"]
      AC["appconfig-{stg,prod}<br/>(DSL JSON CRUD)"]
      AI["ai-{stg,prod}<br/>(音声/文字→Claude→DSL)"]
    end
    FS[("Firestore<br/>Native mode")]
    SM["Secret Manager<br/>ANTHROPIC_API_KEY<br/>OPENAI_API_KEY"]
    FBA["Firebase Auth<br/>(Apple プロバイダ)"]
    SA_run["SA: run-backend"]
    SA_dep["SA: github-deployer"]
  end

  iOS["iOS アプリ<br/>(SwiftUI, GoogleService-Info.plist)"]
  Claude["Claude API"]

  iOS -- "Sign in with Apple" --> AppleID
  iOS -- "IDトークン取得" --> FBA
  iOS -- "Authorization: Bearer <Firebase ID token>" --> AC
  iOS -- "同上" --> AI
  AC -- "IDトークン検証 / 読み書き" --> FS
  AI -- "DSL パッチ反映" --> AC
  AI -- "生成" --> Claude
  AI -. "実行時に注入" .-> SM
  Run -. "実行 ID" .-> SA_run
  Mac -- "初回 gcloud run deploy --source" --> Run
  GH -- "OIDC(鍵ファイル無し)" --> WIF
  WIF -- "なりすまし許可" --> SA_dep
  SA_dep -- "docker push" --> AR
  SA_dep -- "gcloud run deploy --image" --> Run
```

設計上のポイント
- **Cloud Run 自体は `--allow-unauthenticated`**。認証は Go 側で Firebase ID トークンを検証する(アプリ層)。
- **1 イメージに appconfig / ai の両バイナリ**を同梱(`backend/Dockerfile`)。Cloud Run の `--command=/app/ai` でサービスごとに起動バイナリを切り替える。ビルドは 1 回で済む。
- 環境は **1 プロジェクト内でサービス名サフィックスで分ける**(`-stg` = develop、`-prod` = main)。プロジェクトを 2 つ作らないのは課金/Firebase/Apple 設定の二重化を避けるため。
- 秘密鍵は **Secret Manager のみ**。GitHub 側には JSON 鍵を置かない(WIF)。

---

## 2. Step 0 事前準備

### 0-1. 変数を決める(この手順書全体で使う)

まずプロジェクト ID を決める。GCP のプロジェクト ID は **全世界で一意・6〜30 文字・小文字英数とハイフン・先頭は英字・後から変更不可**。

| 変数 | 推奨値 | 備考 |
|---|---|---|
| `PROJECT_ID` | `jphacks-kz2601` | 取られていたら `jphacks-kz2601-2026` など |
| `REGION` | `asia-northeast1` | 東京。Cloud Run / Firestore / Artifact Registry 全部これ |
| `BUNDLE_ID` | `jp.kz2601.metamon` | iOS の Bundle Identifier。Apple / Firebase / Xcode で **完全一致**させる |
| `GITHUB_REPO` | `jphacks/kz_2601` | WIF の条件に使う |

**▶ 実行**(新しいターミナルを開くたびに貼る。`~/.zshrc` に入れてもよい)

```sh
export PROJECT_ID="jphacks-kz2601"
export REGION="asia-northeast1"
export BUNDLE_ID="jp.kz2601.metamon"
export GITHUB_REPO="jphacks/kz_2601"
export RUN_SA="run-backend@${PROJECT_ID}.iam.gserviceaccount.com"
export DEPLOY_SA="github-deployer@${PROJECT_ID}.iam.gserviceaccount.com"
```

### 0-2. 課金アカウント

**☐ あなたがやる**
1. https://console.cloud.google.com/billing を開き、課金アカウントが 1 つ以上あることを確認する(無ければ作成。無料トライアルの $300 クレジットがあればそれで十分)。
2. 課金アカウント ID(`XXXXXX-XXXXXX-XXXXXX` 形式)を控える。

**▶ 実行**(ログインと課金アカウント確認)

```sh
gcloud auth login
gcloud billing accounts list
# OPEN が True の行の ACCOUNT_ID(例: 01BAFA-8017F4-58A712 の形式)をコピーして↓
export BILLING_ACCOUNT="XXXXXX-XXXXXX-XXXXXX"
```

### 0-3. プロジェクト作成・課金リンク・既定値

**▶ 実行**

```sh
gcloud projects create "$PROJECT_ID" --name="JPHACKS2026 kz2601" --set-as-default
gcloud billing projects link "$PROJECT_ID" --billing-account="$BILLING_ACCOUNT"
gcloud config set project "$PROJECT_ID"
gcloud config set run/region "$REGION"
gcloud config set artifacts/location "$REGION"
export PROJECT_NUMBER="$(gcloud projects describe "$PROJECT_ID" --format='value(projectNumber)')"
echo "PROJECT_NUMBER=$PROJECT_NUMBER"
```

> `projects create` が `already exists` なら ID が世界で取られている。別 ID にして `PROJECT_ID` を更新。

### 0-4. 必要 API を 1 コマンドで有効化

**▶ 実行**(1〜2 分かかる)

```sh
gcloud services enable \
  run.googleapis.com \
  cloudbuild.googleapis.com \
  artifactregistry.googleapis.com \
  compute.googleapis.com \
  firestore.googleapis.com \
  secretmanager.googleapis.com \
  firebase.googleapis.com \
  identitytoolkit.googleapis.com \
  iam.googleapis.com \
  iamcredentials.googleapis.com \
  sts.googleapis.com \
  cloudresourcemanager.googleapis.com \
  cloudbilling.googleapis.com \
  billingbudgets.googleapis.com
```

| API | 用途 |
|---|---|
| run / cloudbuild / artifactregistry | Cloud Run と `--source` ビルド |
| compute | Cloud Build が新規プロジェクトでは Compute 既定 SA を使うため(要確認: 無くても通る場合あり) |
| firestore / firebase / identitytoolkit | Firestore・Firebase・Firebase Auth |
| secretmanager | API キー保管 |
| iam / iamcredentials / sts / cloudresourcemanager | SA 作成・WIF・IAM バインディング |
| cloudbilling / billingbudgets | 予算アラート |

### 0-5. サービスアカウント 2 つと Artifact Registry

**▶ 実行**

```sh
# Cloud Run の実行 ID(Firestore / Secret にアクセスする側)
gcloud iam service-accounts create run-backend --display-name="Cloud Run runtime (appconfig/ai)"
gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:${RUN_SA}" --role="roles/datastore.user"

# GitHub Actions からデプロイする ID(Step 4 で WIF と紐付け)
gcloud iam service-accounts create github-deployer --display-name="GitHub Actions deployer"
gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:${DEPLOY_SA}" --role="roles/run.admin"
gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:${DEPLOY_SA}" --role="roles/artifactregistry.writer"
# deployer が「run-backend として」サービスをデプロイするための actAs
gcloud iam service-accounts add-iam-policy-binding "$RUN_SA" \
  --member="serviceAccount:${DEPLOY_SA}" --role="roles/iam.serviceAccountUser"

# CI がイメージを push する Docker リポジトリ
gcloud artifacts repositories create backend \
  --repository-format=docker --location="$REGION" \
  --description="kz_2601 backend images"
```

---

## 3. Step 1 Firebase

### 1-1. Firebase にリンク

**▶ 実行**

```sh
firebase login
firebase projects:addfirebase "$PROJECT_ID"
firebase use "$PROJECT_ID"      # リポジトリ直下で実行すると .firebaserc ができる(コミットして良い)
```

### 1-2. Firestore(Native mode)作成

**ロケーションは作成後に変更できない**。必ず `asia-northeast1`。`--type` の既定が `firestore-native` なので明示は保険。

**▶ 実行**

```sh
gcloud firestore databases create \
  --location="$REGION" \
  --type=firestore-native
gcloud firestore databases describe --format='value(name,type,locationId)'
# 期待: projects/<ID>/databases/(default)  FIRESTORE_NATIVE  asia-northeast1
```

> 無料枠(1 GiB / 50k 読み取り/日 等)は `(default)` データベースにのみ適用される。名前付き DB は作らない。
> セキュリティルール: Go サーバー(Admin SDK)経由でしか触らないので、クライアント直アクセスは閉じる。Firebase コンソール → Firestore → ルールで `allow read, write: if false;` のまま(既定の本番モードで OK)。

### 1-3. Firebase Authentication を開始し Apple プロバイダを有効化

#### (a) Apple Developer 側

**☐ あなたがやる**(https://developer.apple.com/account/resources/identifiers/list)

1. **App ID を作る**: Identifiers → `+` → App IDs → App → Description 任意、Bundle ID = **Explicit** `jp.kz2601.metamon` → Capabilities で **Sign In with Apple** にチェック → Register。
   - 既に Xcode の自動署名で作られていれば、その App ID を Edit して Sign In with Apple を ON にするだけ。
2. **Key を作る**(Firebase の OAuth code flow 設定用。iOS ネイティブのみなら省略可だが、後述の理由で作っておくことを推奨):
   Keys → `+` → Key Name 任意 → **Sign in with Apple** にチェック → Configure → Primary App ID に 1. の App ID → Save → Continue → Register → **`.p8` をダウンロード(1 回しか落とせない)**。
   - 控える値: **Key ID**(10 桁)、**Team ID**(右上のアカウント名横 / Membership に表示、10 桁)。
   - `.p8` は `~/DubVault` のような repo 外に保存。`.gitignore` で `*.p8` は除外済みだが repo 配下に置かない。
3. **Services ID**(**Web / Android からもサインインする場合のみ必要**。iOS ネイティブのみの今回は原則不要):
   Identifiers → `+` → Services IDs → Identifier `jp.kz2601.metamon.signin` → Register → 一覧から開き Sign In with Apple を ON → Configure →
   Primary App ID = 1.、Domains and Subdomains = `<PROJECT_ID>.firebaseapp.com`、Return URLs = `https://<PROJECT_ID>.firebaseapp.com/__/auth/handler` → Next → Done → Continue → Save。

> Apple Developer Program(有料・個人)が必須。無料アカウントでは Sign In with Apple capability が付けられない。

#### (b) Firebase コンソール側

**☐ あなたがやる**(https://console.firebase.google.com/project/<PROJECT_ID>/authentication)

1. Authentication → **始める**(Identity Toolkit を初期化)。
2. Sign-in method → 新しいプロバイダを追加 → **Apple** → 有効にする。
3. 入力欄(2026-10 時点のコンソール文言は **要確認**):

| 欄 | 入れる値 | iOS ネイティブのみの場合 |
|---|---|---|
| Services ID | `jp.kz2601.metamon.signin`(a-3 で作った場合) | 空欄で可(コンソールに "Apple プラットフォームでは省略可" 相当の注記があるはず。無ければ a-3 を作って入れる) |
| Apple チーム ID | Team ID(10 桁) | 任意だが入れておく |
| キー ID | a-2 の Key ID | 任意だが入れておく |
| 秘密鍵 | a-2 の `.p8` の中身(`-----BEGIN PRIVATE KEY-----` ごと貼る) | 任意だが入れておく |

4. 保存。
5. 設定 → **承認済みドメイン** に `<PROJECT_ID>.firebaseapp.com` と `localhost` があることを確認(既定で入っている)。

> OAuth code flow(Team ID / Key ID / 秘密鍵)を入れておくと、アカウント削除時のトークン失効(App Store 審査要件)とメールリレーが使える。ハッカソン提出だけなら必須ではないが、後から `.p8` を再発行する手間を考えると最初に入れるのが楽。

#### (c) Xcode / xcodegen 側(iOS 担当 = あなた)

- Signing & Capabilities に **Sign in with Apple** を追加(xcodegen なら `entitlements` に `com.apple.developer.applesignin: [Default]`)。
- Team は個人の Apple Developer チーム。実機 iPhone 13 で検証(シミュレータの Sign in with Apple は Apple ID サインインが必要で不安定)。
- Firebase SDK は SwiftPM で `firebase-ios-sdk` の `FirebaseAuth` のみ。Apple サインイン後 `OAuthProvider.appleCredential(withIDToken:rawNonce:fullName:)` で Firebase にサインインし、`user.getIDToken()` を Cloud Run への `Authorization: Bearer` に載せる。

### 1-4. iOS アプリ登録と `GoogleService-Info.plist`

**▶ 実行**

```sh
firebase apps:create IOS "kz2601 iOS" --bundle-id="$BUNDLE_ID" --project="$PROJECT_ID"
# 出力の "App ID: 1:123...:ios:abc..." を控える
firebase apps:list IOS --project="$PROJECT_ID"
export FIREBASE_IOS_APP_ID="1:${PROJECT_NUMBER}:ios:xxxxxxxxxxxxxxxx"   # 上の出力に置き換え

# plist を iOS アプリのターゲット直下に書き出す(パスは xcodegen の sources に合わせて調整)
firebase apps:sdkconfig IOS "$FIREBASE_IOS_APP_ID" --out ios/App/GoogleService-Info.plist
```

**コミット禁止**。`.gitignore` に `GoogleService-Info.plist` が入っていることを確認:

```sh
git check-ignore -v ios/App/GoogleService-Info.plist   # 何か表示されれば ignore されている
```

> public リポジトリなので、万一 push したら Firebase コンソールで API キーを再生成し、履歴からも消す(`git filter-repo`)。
> チームの 2 人目が必要な場合は Slack/DM で plist を直接渡す(repo・Issue・PR に貼らない)。

---

## 4. Step 2 Secret Manager

### 2-1. キーを発行する

**☐ あなたがやる**
- **Claude API キー**: https://console.anthropic.com/ → API Keys → Create Key(Workspace は既定で可)。`sk-ant-...` を控える。
- (任意)**OpenAI API キー**: WhisperKit(オンデバイス)で足りない場合のみ。https://platform.openai.com/api-keys → Create new secret key。

### 2-2. 登録(値はファイル経由・履歴に残さない)

**▶ 実行**

```sh
# `read -s` で入力すればシェル履歴に残らない
read -s -p "ANTHROPIC_API_KEY: " ANTHROPIC_API_KEY; echo
printf '%s' "$ANTHROPIC_API_KEY" | gcloud secrets create ANTHROPIC_API_KEY \
  --replication-policy=user-managed --locations="$REGION" --data-file=-
unset ANTHROPIC_API_KEY

# 任意: OpenAI
read -s -p "OPENAI_API_KEY (空ならスキップ): " OPENAI_API_KEY; echo
if [ -n "$OPENAI_API_KEY" ]; then
  printf '%s' "$OPENAI_API_KEY" | gcloud secrets create OPENAI_API_KEY \
    --replication-policy=user-managed --locations="$REGION" --data-file=-
fi
unset OPENAI_API_KEY
```

キーを差し替えるとき(新バージョン追加。Cloud Run は `:latest` を参照しているので再デプロイで反映):

```sh
printf '%s' "sk-ant-新しいキー" | gcloud secrets versions add ANTHROPIC_API_KEY --data-file=-
```

### 2-3. Cloud Run 実行 SA に `secretAccessor` を付与(シークレット単位)

**▶ 実行**

```sh
for s in ANTHROPIC_API_KEY OPENAI_API_KEY; do
  gcloud secrets describe "$s" >/dev/null 2>&1 && \
  gcloud secrets add-iam-policy-binding "$s" \
    --member="serviceAccount:${RUN_SA}" --role="roles/secretmanager.secretAccessor"
done
gcloud secrets list
```

> プロジェクト全体に `secretAccessor` を付けない(最小権限)。

---

## 5. Step 3 Cloud Run 初回デプロイ

`backend/Dockerfile` が置いてあるので `gcloud run deploy --source` は **Dockerfile を自動採用**する(buildpacks は使わない)。まず `-stg` を上げる。`-prod` は Step 4 の CI で main マージ時に作られる(手動で同じコマンドを `-prod` で叩いても良い)。

### 3-1. appconfig(ソースからビルド)

**▶ 実行**(リポジトリ直下から。初回は Artifact Registry リポジトリ `cloud-run-source-deploy` を作るか聞かれるので `Y`。ビルド込みで 3〜5 分)

```sh
cd backend
gcloud run deploy appconfig-stg \
  --source . \
  --region="$REGION" \
  --service-account="$RUN_SA" \
  --allow-unauthenticated \
  --port=8080 \
  --cpu=1 --memory=512Mi \
  --min-instances=0 --max-instances=3 \
  --concurrency=80 --timeout=60 \
  --set-env-vars="GOOGLE_CLOUD_PROJECT=${PROJECT_ID},APP_ENV=stg"
cd ..
```

### 3-2. ai(同じイメージを `--command` で ai として起動)

ビルドを繰り返さず、3-1 で出来たイメージを使い回す。Claude 呼び出しは数十秒かかるので `--timeout=300`。

**▶ 実行**

```sh
export APPCONFIG_BASE_URL="$(gcloud run services describe appconfig-stg --region="$REGION" --format='value(status.url)')"
export BACKEND_IMAGE="$(gcloud run services describe appconfig-stg --region="$REGION" --format='value(spec.template.spec.containers[0].image)')"
echo "$APPCONFIG_BASE_URL"; echo "$BACKEND_IMAGE"

gcloud run deploy ai-stg \
  --image="$BACKEND_IMAGE" \
  --command=/app/ai \
  --region="$REGION" \
  --service-account="$RUN_SA" \
  --allow-unauthenticated \
  --port=8080 \
  --cpu=1 --memory=512Mi \
  --min-instances=0 --max-instances=3 \
  --concurrency=20 --timeout=300 \
  --set-env-vars="GOOGLE_CLOUD_PROJECT=${PROJECT_ID},APP_ENV=stg,APPCONFIG_BASE_URL=${APPCONFIG_BASE_URL}" \
  --set-secrets="ANTHROPIC_API_KEY=ANTHROPIC_API_KEY:latest"
```

> OPENAI_API_KEY も使うなら `--set-secrets="ANTHROPIC_API_KEY=ANTHROPIC_API_KEY:latest,OPENAI_API_KEY=OPENAI_API_KEY:latest"`。
> `--set-secrets` は既存のシークレット設定を全置換、追加だけなら `--update-secrets`。
> ソースから直接 ai を上げたい場合は 3-1 のコマンドを `ai-stg` + `--command=/app/ai` にすれば良い(ビルドがもう 1 回走る)。

### 3-3. ヘルスチェック

**▶ 実行**

```sh
for svc in appconfig-stg ai-stg; do
  url="$(gcloud run services describe "$svc" --region="$REGION" --format='value(status.url)')"
  printf '%-14s %s -> ' "$svc" "$url"
  curl -s -o /dev/null -w '%{http_code}\n' "$url/healthz"
done
# 期待: 両方 200
```

ログ確認:

```sh
gcloud run services logs read ai-stg --region="$REGION" --limit=50
```

### 3-4. Go 側で前提にしてよい実行時環境(実装担当向けメモ)

| 項目 | 値 |
|---|---|
| 待ち受け | `PORT`(=8080) |
| プロジェクト ID | `GOOGLE_CLOUD_PROJECT` 環境変数(Firebase Admin SDK / Firestore クライアントがこれを読む) |
| 認証情報 | ADC。Cloud Run 上では `run-backend` SA が自動で使われる(鍵ファイル不要) |
| Firebase ID トークン検証 | `firebase.google.com/go/v4` の `auth.Client.VerifyIDToken`。公開鍵取得のみで IAM 権限は不要 |
| Firestore | `cloud.google.com/go/firestore`、`(default)` DB。権限は `roles/datastore.user`(付与済み) |
| Claude API キー | `ANTHROPIC_API_KEY` 環境変数(Secret Manager から注入) |
| ai → appconfig | `APPCONFIG_BASE_URL`。当面はクライアントから受けた Firebase ID トークンをそのまま転送する想定(**要設計確認**: 後で Cloud Run の service-to-service ID トークンに切替可) |

---

## 6. Step 4 GitHub Actions からの自動デプロイ

方針: **鍵ファイル無し(Workload Identity Federation)**。CI は Docker イメージを 1 回ビルドして Artifact Registry に push し、同じイメージで appconfig / ai を `--image` デプロイする。
- `develop` に push → `appconfig-stg` / `ai-stg`
- `main` に push → `appconfig-prod` / `ai-prod`

### 4-1. WIF プール・プロバイダ(GitHub OIDC)

**▶ 実行**

```sh
gcloud iam workload-identity-pools create github \
  --location=global --display-name="GitHub Actions"

gcloud iam workload-identity-pools providers create-oidc github \
  --location=global --workload-identity-pool=github \
  --display-name="GitHub OIDC" \
  --issuer-uri="https://token.actions.githubusercontent.com" \
  --attribute-mapping="google.subject=assertion.sub,attribute.repository=assertion.repository,attribute.ref=assertion.ref,attribute.actor=assertion.actor" \
  --attribute-condition="assertion.repository == '${GITHUB_REPO}'"

export WIF_PROVIDER="$(gcloud iam workload-identity-pools providers describe github \
  --location=global --workload-identity-pool=github --format='value(name)')"
echo "$WIF_PROVIDER"
# 期待: projects/<PROJECT_NUMBER>/locations/global/workloadIdentityPools/github/providers/github
```

> GitHub issuer のプロバイダは `--attribute-condition` が必須(無いとエラー)。このリポジトリ以外からのトークンは拒否される。

### 4-2. deployer SA を GitHub リポジトリから使えるようにする

**▶ 実行**

```sh
gcloud iam service-accounts add-iam-policy-binding "$DEPLOY_SA" \
  --role="roles/iam.workloadIdentityUser" \
  --member="principalSet://iam.googleapis.com/projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/github/attribute.repository/${GITHUB_REPO}"
```

> `main` ブランチからのみ prod デプロイを許したい場合は、別 SA を作って `--member=".../attribute.ref/refs/heads/main"` で縛る(ハッカソンでは 1 SA で十分)。

### 4-3. GitHub 側に入れる値

**☐ あなたがやる**: リポジトリ → Settings → Secrets and variables → Actions → **Variables** タブ(いずれも機密ではないが、まとめて Secrets に入れても動く)

| 種別 | 名前 | 値 | 取得コマンド |
|---|---|---|---|
| Variable | `GCP_PROJECT_ID` | `jphacks-kz2601` | `echo $PROJECT_ID` |
| Variable | `GCP_REGION` | `asia-northeast1` | - |
| Variable | `GCP_WIF_PROVIDER` | `projects/<番号>/locations/global/workloadIdentityPools/github/providers/github` | `echo $WIF_PROVIDER` |
| Variable | `GCP_DEPLOY_SA` | `github-deployer@<PROJECT_ID>.iam.gserviceaccount.com` | `echo $DEPLOY_SA` |
| Variable | `GCP_RUNTIME_SA` | `run-backend@<PROJECT_ID>.iam.gserviceaccount.com` | `echo $RUN_SA` |

Secrets に入れるものは **無し**(API キーは Secret Manager、認証は WIF)。

### 4-4. ワークフロー(GitHub 整備担当が `.github/workflows/deploy-backend.yml` として追加)

`docs/00_decisions.md` の CI(lint+test、Draft では発火しない)とは別ファイル。push トリガーのみ(fork PR からは動かない)。

```yaml
name: deploy-backend

on:
  push:
    branches: [develop, main]
    paths: ["backend/**", ".github/workflows/deploy-backend.yml"]

concurrency:
  group: deploy-backend-${{ github.ref }}
  cancel-in-progress: false

permissions:
  contents: read
  id-token: write   # WIF に必須

jobs:
  deploy:
    runs-on: ubuntu-latest
    timeout-minutes: 15
    env:
      PROJECT_ID: ${{ vars.GCP_PROJECT_ID }}
      REGION: ${{ vars.GCP_REGION }}
      RUN_SA: ${{ vars.GCP_RUNTIME_SA }}
      ENV_SUFFIX: ${{ github.ref_name == 'main' && 'prod' || 'stg' }}
    steps:
      - uses: actions/checkout@v4

      - uses: google-github-actions/auth@v2
        with:
          workload_identity_provider: ${{ vars.GCP_WIF_PROVIDER }}
          service_account: ${{ vars.GCP_DEPLOY_SA }}

      - uses: google-github-actions/setup-gcloud@v2

      - name: Build & push image
        run: |
          gcloud auth configure-docker "${REGION}-docker.pkg.dev" --quiet
          IMAGE="${REGION}-docker.pkg.dev/${PROJECT_ID}/backend/backend:${GITHUB_SHA}"
          docker build -t "$IMAGE" backend
          docker push "$IMAGE"
          echo "IMAGE=$IMAGE" >> "$GITHUB_ENV"

      - name: Deploy appconfig
        run: |
          gcloud run deploy "appconfig-${ENV_SUFFIX}" \
            --image="$IMAGE" --region="$REGION" \
            --service-account="$RUN_SA" --allow-unauthenticated \
            --port=8080 --cpu=1 --memory=512Mi \
            --min-instances=0 --max-instances=3 --concurrency=80 --timeout=60 \
            --set-env-vars="GOOGLE_CLOUD_PROJECT=${PROJECT_ID},APP_ENV=${ENV_SUFFIX}" \
            --quiet

      - name: Deploy ai
        run: |
          APPCONFIG_BASE_URL="$(gcloud run services describe "appconfig-${ENV_SUFFIX}" --region="$REGION" --format='value(status.url)')"
          gcloud run deploy "ai-${ENV_SUFFIX}" \
            --image="$IMAGE" --command=/app/ai --region="$REGION" \
            --service-account="$RUN_SA" --allow-unauthenticated \
            --port=8080 --cpu=1 --memory=512Mi \
            --min-instances=0 --max-instances=3 --concurrency=20 --timeout=300 \
            --set-env-vars="GOOGLE_CLOUD_PROJECT=${PROJECT_ID},APP_ENV=${ENV_SUFFIX},APPCONFIG_BASE_URL=${APPCONFIG_BASE_URL}" \
            --set-secrets="ANTHROPIC_API_KEY=ANTHROPIC_API_KEY:latest" \
            --quiet

      - name: Smoke test
        run: |
          for svc in appconfig ai; do
            url="$(gcloud run services describe "${svc}-${ENV_SUFFIX}" --region="$REGION" --format='value(status.url)')"
            curl -fsS "$url/healthz" && echo " <- ${svc}-${ENV_SUFFIX} OK"
          done
```

> `--allow-unauthenticated` を CI から付けるには deployer に `roles/run.admin` が要る(0-5 で付与済み)。
> `-prod` 初回は CI が新規作成する。手動で先に作っておく場合は Step 3 のコマンドの `-stg` を `-prod` に読み替える。

### 4-5. 動作確認

**☐ あなたがやる**: `backend/` に空コミットでも良いので `develop` に push → Actions が緑になり、Step 3-3 のヘルスチェックが通ることを確認。

---

## 7. Step 5 ローカル開発

### 5-1. Firestore / Auth エミュレータ

Java 11+ が必要(手元は OpenJDK 26 で OK)。Firestore エミュレータの既定ポート 8080 は appconfig と衝突するので **8089** に変える。

**☐ あなたがやる**: リポジトリ直下に `firebase.json` を作る(コミットして良い)

```json
{
  "emulators": {
    "firestore": { "port": 8089 },
    "auth": { "port": 9099 },
    "ui": { "enabled": true, "port": 4000 },
    "singleProjectMode": true
  }
}
```

**▶ 実行**(`demo-` で始まるプロジェクト ID はエミュレータ専用。実プロジェクトに絶対触らない)

```sh
firebase emulators:start --only firestore,auth --project demo-kz2601
# UI: http://localhost:4000
```

### 5-2. ADC(実 GCP の Firestore / Secret をローカルから叩くとき)

エミュレータだけで開発するなら不要。実 Firestore を見たい時だけ。

**▶ 実行**

```sh
gcloud auth application-default login
gcloud auth application-default set-quota-project "$PROJECT_ID"
```

### 5-3. 環境変数と起動

`backend/.env`(`.gitignore` 済み)に書く:

```sh
# backend/.env  -- コミットしない
GOOGLE_CLOUD_PROJECT=demo-kz2601
FIRESTORE_EMULATOR_HOST=localhost:8089
FIREBASE_AUTH_EMULATOR_HOST=localhost:9099
APPCONFIG_BASE_URL=http://localhost:8080
ANTHROPIC_API_KEY=sk-ant-...        # ai を動かす時だけ。Secret Manager と同じ値
```

**▶ 実行**(ターミナル 2 枚)

```sh
cd backend && set -a && . ./.env && set +a && go run ./cmd/appconfig          # :8080
cd backend && set -a && . ./.env && set +a && PORT=8081 go run ./cmd/ai       # :8081
curl -s localhost:8080/healthz; echo; curl -s localhost:8081/healthz; echo
```

Docker で Cloud Run と同じイメージを試す:

```sh
cd backend && docker build -t kz2601-backend . && \
docker run --rm -p 8080:8080 --env-file .env kz2601-backend && \
docker run --rm -p 8081:8080 --env-file .env --entrypoint /app/ai kz2601-backend
```

> iOS 実機からローカルの Go に繋ぐときは Mac の LAN IP(`ipconfig getifaddr en0`)を使い、`Info.plist` の ATS で http を許可するか、素直に `-stg` の Cloud Run を向ける。

---

## 8. コスト・無料枠とガードレール

### 8-1. 想定コスト

2 日間・デモ規模なら GCP 側は **ほぼ 0 円**。実コストは Claude API。無料枠の数値は 2026-10 時点の記憶ベース(**要確認**: https://cloud.google.com/free)。

| サービス | 無料枠(月・目安) | ハッカソンでの見込み |
|---|---|---|
| Cloud Run | 200 万リクエスト、36 万 GB 秒、18 万 vCPU 秒(asia-northeast1 は Tier 1) | 枠内 |
| Firestore `(default)` | 1 GiB、読み 5 万/日、書き 2 万/日 | 枠内 |
| Secret Manager | 6 アクティブバージョン、1 万アクセス | 枠内(`:latest` 参照はインスタンス起動時のみ) |
| Artifact Registry | 0.5 GB | イメージ ~15 MB × push 回数。古いタグは Day2 に掃除 |
| Cloud Build | 無料枠あり(分数は要確認) | `--source` は初回だけ。CI は GitHub Runner でビルドするので Cloud Build 不使用 |
| Firebase Auth | Sign in with Apple は無料(Spark) | 0 |
| WIF / IAM / 予算 | 無料 | 0 |
| Claude API | 有料 | Sonnet 5.5: 入力 $2 / 出力 $10 per 1M tokens。DSL 生成 1 回 ≒ 数千 token → 1 回 1〜3 円 |

### 8-2. 予算アラート(通知のみ。自動停止はしない)

課金アカウントの通貨に合わせる(日本の個人アカウントは通常 JPY。USD なら `30USD` 等)。

**▶ 実行**

```sh
gcloud billing budgets create \
  --billing-account="$BILLING_ACCOUNT" \
  --display-name="kz2601 monthly" \
  --budget-amount=3000JPY \
  --filter-projects="projects/${PROJECT_ID}" \
  --threshold-rule=percent=0.5 \
  --threshold-rule=percent=0.9 \
  --threshold-rule=percent=1.0 \
  --threshold-rule=percent=1.0,basis=forecasted-spend
gcloud billing budgets list --billing-account="$BILLING_ACCOUNT"
```

> 通知先は既定で課金管理者のメール。**☐ あなたがやる**: https://console.cloud.google.com/billing → 予算とアラート → 作った予算 → メール通知が自分に来る設定になっているか確認。

### 8-3. Cloud Run 側の上限

- `--max-instances=3`(Step 3 / CI で設定済み)。暴走しても 3 コンテナ以上は立たない。
- `--min-instances=0` を維持(常駐させない)。デモ直前だけコールドスタート回避で `--min-instances=1` にし、終わったら 0 に戻す:

```sh
gcloud run services update ai-prod --region="$REGION" --min-instances=1   # デモ前
gcloud run services update ai-prod --region="$REGION" --min-instances=0   # デモ後
```

- Firestore に暴走ループで書かないよう、ai 側は 1 リクエスト 1 パッチ(リトライ上限付き)で実装する。

### 8-4. Claude / OpenAI 側の使用上限

**☐ あなたがやる**
- **Anthropic Console**: https://console.anthropic.com/ → Settings → **Limits**(組織の月間使用上限 / spend limit を設定できる画面。メニュー名は **要確認**)。ハッカソンなら $20〜30 程度で十分。Usage 画面で日次消費も見える。
- **OpenAI**(使う場合): https://platform.openai.com/ → Settings → Limits → Monthly budget。
- キーは Secret Manager にしか置かない。漏れた疑いがあれば Console で Revoke → `gcloud secrets versions add` → Cloud Run 再デプロイ(`gcloud run services update ai-stg --region=$REGION --update-secrets=ANTHROPIC_API_KEY=ANTHROPIC_API_KEY:latest` で新リビジョン)。

---

## 9. チェックリスト

全部が「期待結果」になれば構築完了。上から順に 1 回流す。

| # | 項目 | URL / コマンド | 期待結果 |
|---|---|---|---|
| 1 | プロジェクトと課金 | `gcloud billing projects describe $PROJECT_ID --format='value(billingEnabled)'` | `True` |
| 2 | API 有効 | `gcloud services list --enabled --filter='name:(run OR firestore OR secretmanager OR artifactregistry OR iamcredentials)' --format='value(config.name)'` | 5 行出る |
| 3 | SA 2 つ | `gcloud iam service-accounts list --format='value(email)'` | `run-backend@…` と `github-deployer@…` |
| 4 | Firebase リンク | `firebase projects:list` | `$PROJECT_ID` が載る |
| 5 | Firestore Native / 東京 | `gcloud firestore databases describe --format='value(type,locationId)'` | `FIRESTORE_NATIVE asia-northeast1` |
| 6 | Apple プロバイダ | https://console.firebase.google.com/project/`$PROJECT_ID`/authentication/providers | Apple が「有効」 |
| 7 | iOS アプリ登録 | `firebase apps:list IOS` | Bundle ID が `$BUNDLE_ID` |
| 8 | plist が ignore | `git check-ignore -v ios/App/GoogleService-Info.plist` | `.gitignore:…` が表示 |
| 9 | Secret 存在 | `gcloud secrets list --format='value(name)'` | `ANTHROPIC_API_KEY`(+`OPENAI_API_KEY`) |
| 10 | Secret 権限 | `gcloud secrets get-iam-policy ANTHROPIC_API_KEY --format='value(bindings.members)'` | `serviceAccount:run-backend@…` |
| 11 | appconfig-stg | `curl -s -o /dev/null -w '%{http_code}' $(gcloud run services describe appconfig-stg --region=$REGION --format='value(status.url)')/healthz` | `200` |
| 12 | ai-stg | 同上を `ai-stg` で | `200` |
| 13 | ai にシークレット注入 | `gcloud run services describe ai-stg --region=$REGION --format='yaml(spec.template.spec.containers[0].env)'` | `ANTHROPIC_API_KEY` が `secretKeyRef` で載る |
| 14 | 実行 SA | `gcloud run services describe ai-stg --region=$REGION --format='value(spec.template.spec.serviceAccountName)'` | `run-backend@…` |
| 15 | WIF プロバイダ | `gcloud iam workload-identity-pools providers describe github --location=global --workload-identity-pool=github --format='value(state)'` | `ACTIVE` |
| 16 | GitHub Variables | リポジトリ Settings → Actions → Variables | 5 つ揃っている |
| 17 | CI デプロイ | `develop` へ push → Actions | `deploy-backend` 緑、Smoke test OK |
| 18 | 予算 | `gcloud billing budgets list --billing-account=$BILLING_ACCOUNT --format='value(displayName)'` | `kz2601 monthly` |
| 19 | エミュレータ | `firebase emulators:start --only firestore,auth --project demo-kz2601` | `All emulators ready` |
| 20 | 実機サインイン | iPhone 13 で Sign in with Apple → Firebase コンソール Authentication → Users | ユーザーが 1 人増える |

---

## 10. トラブルシュート

| 症状 | 原因 | 対処 |
|---|---|---|
| `gcloud services enable` で `FAILED_PRECONDITION` / `Billing account … is not found` | 課金未リンク、または課金アカウントが無効 | 0-2/0-3 をやり直す。`gcloud billing projects describe $PROJECT_ID` で `billingEnabled: true` を確認 |
| `PERMISSION_DENIED: Cloud Run Admin API has not been used in project … before or it is disabled` | API 未有効(数分の伝播待ちのことも) | 0-4 を再実行し 1〜2 分待つ |
| `projects create` が `The project ID you specified is already in use` | ID が他人に取られている | 別 ID。`PROJECT_ID` を更新して以降を続ける |
| `gcloud run deploy --source` が Cloud Build の権限エラー(`…compute@developer.gserviceaccount.com does not have permission` 等) | 新規プロジェクトの Cloud Build は Compute 既定 SA で動く。既定 SA が無い/権限不足 | `compute.googleapis.com` を有効化(0-4)。それでも駄目なら `gcloud projects add-iam-policy-binding $PROJECT_ID --member="serviceAccount:${PROJECT_NUMBER}-compute@developer.gserviceaccount.com" --role="roles/cloudbuild.builds.builder"` |
| `--source` で `cloud-run-source-deploy` の作成確認が出る | 初回のみ | `Y` |
| `curl <run.app URL>/healthz` が Google の HTML 404(`Error 404 (Not Found)!!1`)。`/` や `/healthz/` は Go の `404 page not found` | `/healthz` が Google Front End に吸われてコンテナに届かない(2026-10-24 実測。リクエストログにも残らない) | ヘルスパスを `/health` 等に変更して再デプロイ。CI の Smoke test / ヘルスチェック step も同じパスに |
| Cloud Run が `Container failed to start… listen on PORT=8080` | バイナリが 8080 を聞いていない / `--command` のパス違い | `/app/appconfig` or `/app/ai`。ログ `gcloud run services logs read <svc> --region=$REGION` |
| ai が起動時に `Permission denied on secret` | 2-3 の `secretAccessor` 未付与、または `--service-account` を付け忘れて既定 SA で動いている | チェックリスト 10・14 を確認して再デプロイ |
| Firestore `databases create` が `already exists` / Datastore モードで作ってしまった | 1 プロジェクト 1 `(default)`。ロケーション・モードは後から変更不可 | 間違えたら `gcloud firestore databases delete --database='(default)'` して作り直す(データが空のうちに)。`--delete-protection` を付けていたら先に `databases update --no-delete-protection` |
| Firestore のロケーション選択が UI でグレーアウト(確定できない) | 既に App Engine / 以前の Firestore でロケーションが固定されている | `gcloud firestore databases describe` で現状確認。別リージョンに固定済みなら新プロジェクトを作るのが早い |
| `firebase projects:addfirebase` が `already a Firebase project` | 済み | 無視して次へ |
| Firebase の Apple プロバイダ保存時に Services ID 必須と言われる | コンソールの仕様差(要確認) | Apple 側で Services ID(1-3 a-3)を作って入れる。Return URL は `https://<PROJECT_ID>.firebaseapp.com/__/auth/handler` |
| Apple Services ID の「ドメイン検証」で詰まる | Apple は Domains 登録時に検証ファイル(`apple-developer-domain-association.txt`)を要求することがある。Firebase ホスティング配下のドメインにはユーザーが置けない | **iOS ネイティブのみなら Services ID 自体が不要**なので飛ばす。Web 対応が必要になった時点で Firebase Hosting を有効化して検証ファイルを `/.well-known/` に配置(要確認: 近年は検証が任意化) |
| サインイン後 Firebase が `invalid_client` / `The audience in ID Token … does not match` | Bundle ID と Firebase iOS アプリ / Apple App ID の不一致、または Services ID とネイティブの取り違え | `BUNDLE_ID` が Xcode・Apple App ID・`firebase apps:list IOS` で完全一致か確認 |
| Xcode で Sign in with Apple capability が追加できない | 無料の Apple ID(Program 未加入)または Team 選択ミス | 個人の有料 Developer チームを選ぶ |
| シミュレータで Apple サインインが進まない | シミュレータの Apple ID 設定依存 | 実機 iPhone 13 で検証する |
| Actions で `Unable to get ACTIONS_ID_TOKEN_REQUEST_URL` | `permissions: id-token: write` が無い | ワークフローの `permissions` を確認 |
| Actions で `Permission 'iam.serviceAccounts.getAccessToken' denied` / `attribute condition` エラー | 4-2 の binding 漏れ、`GITHUB_REPO` の綴り/大文字小文字違い、attribute-condition とリポジトリ不一致 | `gcloud iam service-accounts get-iam-policy $DEPLOY_SA` で principalSet を確認。リポジトリ名は `jphacks/kz_2601` と完全一致 |
| `docker push` が `denied: Permission "artifactregistry.uploadArtifacts" denied` | deployer の `artifactregistry.writer` 漏れ、または `configure-docker` 未実行 | 0-5 / 4-4 を確認 |
| CI の `gcloud run deploy` が `iam.serviceaccounts.actAs` で失敗 | deployer に runtime SA への `serviceAccountUser` が無い | 0-5 の `add-iam-policy-binding "$RUN_SA"` を再実行 |
| `firebase emulators:start` が Java エラー / ポート競合 | Java 未インストール、8080 を appconfig が使用中 | `java -version` を確認。`firebase.json` で firestore を 8089 に |
| ローカル Go が実 Firestore に繋ぎに行く | `FIRESTORE_EMULATOR_HOST` 未設定 | `.env` を読み込んでから起動(5-3) |
| 予算アラートが来ない | 通知先未設定、通貨不一致で作成失敗 | `gcloud billing budgets list` で存在確認。通貨は課金アカウントに合わせる |

---

## 11. 付録: 名前の一覧

| 種類 | 名前 |
|---|---|
| GCP プロジェクト | `jphacks-kz2601`(仮) |
| リージョン | `asia-northeast1` |
| Cloud Run サービス | `appconfig-stg` / `ai-stg` / `appconfig-prod` / `ai-prod` |
| 実行 SA | `run-backend@<PROJECT_ID>.iam.gserviceaccount.com` |
| デプロイ SA | `github-deployer@<PROJECT_ID>.iam.gserviceaccount.com` |
| Artifact Registry | `asia-northeast1-docker.pkg.dev/<PROJECT_ID>/backend/backend:<sha>`(CI)、`…/cloud-run-source-deploy/…`(`--source` 自動作成) |
| Secret | `ANTHROPIC_API_KEY` / `OPENAI_API_KEY`(任意) |
| WIF | pool `github` / provider `github` |
| Firebase iOS アプリ Bundle ID | `jp.kz2601.metamon` |
| Apple | App ID = Bundle ID、Key(Sign in with Apple)、Services ID `jp.kz2601.metamon.signin`(Web 用・任意) |
| エミュレータ用プロジェクト | `demo-kz2601` |
| Cloud Run 環境変数 | `GOOGLE_CLOUD_PROJECT`, `APP_ENV`(`stg`/`prod`), `APPCONFIG_BASE_URL`(ai のみ), `ANTHROPIC_API_KEY`(Secret) |

関連ファイル: `backend/Dockerfile`(両バイナリ同梱)、`backend/.gcloudignore`、`.gitignore`(plist / .p8 / .env を除外)。
