# 認証情報の棚卸しとローテーション手順(#301)

本書は kz_2601 が使う認証情報(API キー・署名鍵・CI の認証)の **名前・用途・置き場所・管理者・差し替え手順** をまとめる。

- **値は本書にもリポジトリにも書かない。** 置き場所は「どこに入っているか(名前)」だけを書く。
- 値をチャット・Issue・PR・ログ・スクリーンショットに貼らない。貼ってしまったら、その認証情報を下の手順で差し替える。
- GCP 側の構築は [gcp-setup.md](gcp-setup.md)、Terraform 管理分は [`infra/`](../../infra/README.md) を参照。

## 目次
1. [一覧](#1-一覧)
2. [Anthropic API キー](#2-anthropic-api-キー)
3. [Firebase の API キー(iOS / Web)](#3-firebase-の-api-キーios--web)
4. [Sign in with Apple の鍵](#4-sign-in-with-apple-の鍵)
5. [App Store Connect API キー](#5-app-store-connect-api-キー)
6. [GitHub Actions → GCP(Workload Identity Federation)](#6-github-actions--gcpworkload-identity-federation)
7. [ローカル開発用の値](#7-ローカル開発用の値)
8. [リポジトリへの混入防止(gitleaks)](#8-リポジトリへの混入防止gitleaks)

---

## 1. 一覧

| 認証情報 | 用途 | 置き場所(名前のみ) | 管理者 |
|---|---|---|---|
| Anthropic API キー | `ai-*` サービスが Claude API を呼ぶ | GCP Secret Manager `ANTHROPIC_API_KEY` → Cloud Run `ai-stg` / `ai-prod` に `ANTHROPIC_API_KEY:latest` として注入 | GCP プロジェクトのオーナー(@ko-tarou) |
| Firebase iOS API キー | iOS アプリの Firebase Auth / SDK 初期化 | `GoogleService-Info.plist`(prod)/ `GoogleService-Info-Staging.plist`(stg)。リポジトリ外の開発者ローカルフォルダ(`METAMON_SECRETS_DIR`)に置き、`ios/scripts/sync-firebase-plist.sh` で git-ignore 済みの `ios/Firebase/` にコピー | Firebase プロジェクトのオーナー |
| Firebase Web API キー | スモーク(`backend/cmd/smoke`)・`scripts/demo-check.sh` の匿名サインイン | GitHub Actions **Variable** `FIREBASE_WEB_API_KEY`。手元では環境変数 `FIREBASE_WEB_API_KEY` | Firebase プロジェクトのオーナー |
| Sign in with Apple の鍵(.p8) | Firebase Auth の Apple プロバイダ(アカウント削除時の Apple トークン取り消しを含む) | Firebase Auth の Apple プロバイダ設定。原本はリポジトリ外(パスワード管理ツール) | Apple Developer Program のアカウント保有者 / Admin |
| App Store Connect API キー(.p8) | 手元からのビルドのアップロード(`xcrun altool` 等) | 開発者ローカルの `~/.appstoreconnect/private_keys/AuthKey_<KEY_ID>.p8`(リポジトリ外) | App Store Connect の Admin |
| GitHub Actions の GCP 認証 | `deploy-cloudrun.yml` が Artifact Registry に push し Cloud Run にデプロイする | **鍵ファイルなし**(Workload Identity Federation)。GitHub Actions Variables `GCP_PROJECT_ID` / `GCP_REGION` / `GCP_WORKLOAD_IDENTITY_PROVIDER` / `GCP_SERVICE_ACCOUNT` | GCP プロジェクトのオーナー |

- GitHub Actions **Secrets は使っていない**(上の 5 つはすべて Variables。値が公開されても認証にはならない識別子か、クライアントに配布される前提の Firebase Web API キー)。
- Secret Manager にあるのは `ANTHROPIC_API_KEY` だけ。入れ物は Terraform(`infra/` の `secret_ids`)で作り、値(version)は人が入れる。
- `OPENAI_API_KEY`(`/v1/transcribe` 用)は任意で、現在は登録していない。登録する場合は Anthropic と同じ手順で `ai-*` に注入する。

---

## 2. Anthropic API キー

置き場所: Secret Manager `ANTHROPIC_API_KEY`。Cloud Run は `:latest` を参照する(`deploy-cloudrun.yml` の `--set-secrets`)。

### 差し替え手順

値はクリップボード経由で標準入力に流し、シェル履歴・ファイルに残さない。

1. [Anthropic Console](https://console.anthropic.com/) → API Keys → **Create Key** で新しいキーを作り、コピーする(旧キーはまだ消さない)。
2. Secret Manager に新バージョンを追加する。
   ```sh
   pbpaste | gcloud secrets versions add ANTHROPIC_API_KEY \
     --project=jphacks-kz2601 --data-file=-
   pbcopy < /dev/null   # クリップボードを空にする
   ```
3. 稼働中のサービスに新バージョンを反映する(新しいリビジョンが作られる)。
   ```sh
   for svc in ai-stg ai-prod; do
     gcloud run services update "$svc" \
       --project=jphacks-kz2601 --region=asia-northeast1 \
       --update-secrets=ANTHROPIC_API_KEY=ANTHROPIC_API_KEY:latest
   done
   ```
4. 確認する。
   - `gcloud run services describe ai-stg --project=jphacks-kz2601 --region=asia-northeast1 --format='value(status.latestReadyRevisionName)'` で新リビジョンが Ready。
   - `cd backend && FIREBASE_WEB_API_KEY=<Firebase Web API key> make smoke-stg` が 0 failed(Claude を実際に呼ぶ)。prod は `make smoke-prod`。
5. Console で **旧キーを無効化(Disable)→ 問題がなければ削除** する。
6. Secret Manager の旧バージョンを無効化する。
   ```sh
   gcloud secrets versions list ANTHROPIC_API_KEY --project=jphacks-kz2601
   gcloud secrets versions disable <旧バージョン番号> --secret=ANTHROPIC_API_KEY --project=jphacks-kz2601
   ```

以後のデプロイも `:latest` を参照するので、workflow の変更は不要。

---

## 3. Firebase の API キー(iOS / Web)

Firebase の API キーはクライアント(アプリ)に同梱される前提の識別子で、アクセス制御は Firebase Auth の ID トークン検証と Firestore 側(backend 経由)で行う。キー自体は **iOS キーはアプリの Bundle ID(`jp.kz2601.metamon` 系)、Web キーは必要な API(Identity Toolkit 等)に制限する** 方針で運用する。制限は Google Cloud Console → APIs & Services → Credentials で設定する。

### 差し替え手順(iOS キー)

1. Google Cloud Console → APIs & Services → Credentials で、iOS 用の新しいキーを作り、旧キーと同じ制限(アプリの Bundle ID・必要な API)を付ける。
2. Firebase Console → プロジェクトの設定 → iOS アプリから `GoogleService-Info.plist` をダウンロードし直す(stg 用があれば `GoogleService-Info-Staging.plist` も)。
3. リポジトリ外のローカルフォルダ(`METAMON_SECRETS_DIR`)の plist を置き換え、`ios/scripts/sync-firebase-plist.sh` を実行する(コピー先が git-ignore されていることをスクリプトが確認する)。
4. Debug / Release でビルドし、サインインと API 呼び出しが通ることを確認する。新しいキーを含むビルドを配布する。
5. 旧キーを使うビルドが配布されていないこと(TestFlight / App Store の現行ビルド)を確かめてから、旧キーを削除する。

### 差し替え手順(Web キー)

1. Credentials で新しいキーを作り、旧キーと同じ API 制限を付ける。
2. GitHub Actions Variable を更新する。
   ```sh
   pbpaste | gh variable set FIREBASE_WEB_API_KEY -R jphacks/kz_2601
   pbcopy < /dev/null
   ```
3. `deploy-cloudrun.yml` を staging で `workflow_dispatch` し、job summary のスモークが匿名サインインで通ることを確認する(失敗時は「未認証 401 の確認だけ」にフォールバックした表示になる)。
4. 手元のスクリプト(`scripts/demo-check.sh`・`make smoke-*`)で使う値も差し替える。
5. 旧キーを削除する。

---

## 4. Sign in with Apple の鍵

置き場所: Firebase Console → Authentication → Sign-in method → Apple の「OAuth コードフローの構成」(Services ID・Team ID・Key ID・秘密鍵)。.p8 の原本はリポジトリ外(パスワード管理ツール)。背景は [apple-token-revocation.md](../legal/apple-token-revocation.md)。

### 差し替え手順

1. [Apple Developer](https://developer.apple.com/account/resources/authkeys/list) → Certificates, IDs & Profiles → **Keys** → 新しいキーを作り、**Sign in with Apple** を有効にしてプライマリ App ID を選ぶ。
2. `.p8` をダウンロードする(**1 回しか落とせない**)。Key ID を控え、.p8 はパスワード管理ツールに保管する。
3. Firebase Console の Apple プロバイダで **Key ID と秘密鍵(.p8 の中身)** を新しいものに置き換えて保存する。
4. 確認する: 実機で Sign in with Apple → サインインできる。テスト用アカウントでアカウント削除 → Apple のトークン取り消しがエラーにならない(Cloud Logging / アプリの表示)。
5. Apple Developer で **旧キーを Revoke** する。手元に残った旧 .p8 を削除する。

---

## 5. App Store Connect API キー

置き場所: アップロード作業をする開発者のローカル `~/.appstoreconnect/private_keys/AuthKey_<KEY_ID>.p8`(`xcrun altool` が既定で探す場所)。Key ID と Issuer ID はコマンド引数・各自のローカル設定で渡し、リポジトリには置かない。

### 差し替え手順

1. [App Store Connect](https://appstoreconnect.apple.com/access/integrations/api) → ユーザとアクセス → 統合 → App Store Connect API → **キーを生成**(必要最小のロール。アップロードだけなら App Manager 以下で足りるかを生成時に選ぶ)。
2. `.p8` をダウンロードする(**1 回しか落とせない**)。
3. 配置して権限を絞る。
   ```sh
   mkdir -p ~/.appstoreconnect/private_keys
   mv ~/Downloads/AuthKey_<NEW_KEY_ID>.p8 ~/.appstoreconnect/private_keys/
   chmod 600 ~/.appstoreconnect/private_keys/AuthKey_<NEW_KEY_ID>.p8
   ```
4. アップロードに使う Key ID を新しいものに替え、1 回アップロード(または `xcrun altool --list-apps --apiKey <NEW_KEY_ID> --apiIssuer <ISSUER_ID>`)で認証が通ることを確認する。
5. App Store Connect で **旧キーを取り消し**、ローカルの旧 .p8 を削除する。

---

## 6. GitHub Actions → GCP(Workload Identity Federation)

GitHub Actions は OIDC トークンで GCP のデプロイ用サービスアカウントになりすます。**長期の鍵ファイルは発行していない**ので、定期的な差し替え対象はない。プロバイダは対象リポジトリ(`jphacks/kz_2601`)からのトークンだけを受け入れる設定。

置き場所: GitHub Actions Variables `GCP_PROJECT_ID` / `GCP_REGION` / `GCP_WORKLOAD_IDENTITY_PROVIDER` / `GCP_SERVICE_ACCOUNT`。GCP 側のプール・プロバイダ・サービスアカウントは Terraform([`infra/`](../../infra/README.md))で管理する。

### 作り直す場合の手順

1. `infra/` の Terraform でプロバイダ(またはサービスアカウント)を作り直す(`terraform plan` で差分を確認してから `apply`)。
2. 名前が変わった場合は Variables を更新する。
   ```sh
   gh variable set GCP_WORKLOAD_IDENTITY_PROVIDER -R jphacks/kz_2601 --body '<projects/.../providers/...>'
   gh variable set GCP_SERVICE_ACCOUNT -R jphacks/kz_2601 --body '<deployer SA のメール>'
   ```
3. `deploy-cloudrun.yml` を staging で `workflow_dispatch` し、認証 → デプロイ → `/health` が通ることを確認する。
4. 旧プロバイダ / 旧サービスアカウントを無効化・削除する。

---

## 7. ローカル開発用の値

- `backend` をローカルで Claude 付きで動かす・`cmd/ai-eval` を回す時は、環境変数 `ANTHROPIC_API_KEY` を各自の **個人キー** で渡す(本番の `ANTHROPIC_API_KEY` を手元にコピーしない)。`.env` / `.env.*` は git-ignore 済み。
- 個人キーは各自が Console で作成・失効させる。漏えいが疑われたら Console で無効化して作り直す。

---

## 8. リポジトリへの混入防止(gitleaks)

- `.gitignore` で `.env`・`*.p8`・`*.pem`・`GoogleService-Info*.plist`・`serviceAccountKey*.json` を除外している。
- GitHub Actions の `secrets-scan` ワークフロー(gitleaks)が **すべての pull_request と push** で走る(Draft でも実行)。
- 設定は `.gitleaks.toml`(既定ルール + 許可リスト)と `.gitleaksignore`(履歴上の個別の誤検知)。許可しているのは **ローカル開発用のプレースホルダ**(`AUTH_MODE=dev` で使う `Bearer dev-smoke` などの固定文字列)だけで、実際のトークンではない。新しい誤検知を許可するときは、理由を同じファイルのコメントに書く。
- gitleaks が検出したら: 値を消すコミットを積むだけでは履歴に残るので、**先に該当の認証情報を本書の手順で差し替える**。
