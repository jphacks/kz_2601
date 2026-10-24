# 02 GitHub 運用(1ページ)

> ルールの正典は `docs/00_decisions.md` の「Git運用」「CI」節。ここはその手順書。

## ブランチ

| ブランチ | 役割 | 入口 |
|---|---|---|
| `main` | 本番(Cloud Run prod)。実機テスト済みのものだけ | `develop` / `hotfix/*` からの PR |
| `develop` | 統合(Cloud Run staging) | `feature/*` / `release/*` からの PR |
| `feature/<topic>` | 日常の作業。1 PR = 1 トピック | `develop` から切る |
| `hotfix/<topic>` | 本番の緊急修正 | `main` から切る → `main` と `develop` 両方に PR |
| `release/<ver>` | デモ前の凍結・微調整(必要な時だけ) | `develop` から切る → `main` に PR、終わったら `develop` に戻す |

```sh
git switch develop && git pull
git switch -c feature/voice-screen        # 例: feature/<kebab-case>
# ... 作業 ...
git push -u origin feature/voice-screen
```

## PR の出し方(Draft → Open)

1. まず **Draft** で出す(`gh pr create --draft --base develop`)。Draft の間は backend-ci / ios-ci は走らない(secrets-scan だけ走る)。
2. 動作確認が終わったら **Ready for review** にする → CI 発火(backend-ci は 5分以内厳守、ios-ci は build+test が通れば可・上限25分)。
3. PR 本文はテンプレートに沿う(背景 / スコープ / 機能要件(docs/01 の ID) / 変更点 / 動作確認 / シナリオ / 後続対応 / チェック)。UI 変更はスクショ必須。
4. `.github/CODEOWNERS` でレビュアーが自動アサインされるが **approve は必須ではない(0 approve)**。相手が忙しければ自分でマージしてよい。

```sh
gh pr create --draft --base develop --title "feat(ios): 音声入力画面の骨格"
gh pr ready            # Draft 解除
```

## マージ手順

- 条件: CI 3本が緑(`backend-ci` / `ios-ci` / `secrets-scan`)・会話(レビューコメント)が全部 Resolved。
- **マージは人間が GitHub の Merge ボタン or `gh pr merge` で行う**。bot(GitHub Apps)はブランチ保護でマージ不可。
- `feature/*` → `develop`: Squash merge 推奨(履歴が1コミット=1PRになる)。タイトルは Conventional Commits に直す。
- `develop` → `main`: **実機(iPhone 13)で E2E を通してから**。Merge commit(squash しない。develop と main の履歴を揃える)。マージすると Cloud Run prod に自動デプロイ。
- 変更を触らない領域の CI(例: iOS だけ触った PR の `backend-ci`)は自動で skip され、required check は満たされる。

## hotfix / release

```sh
# hotfix: main から切り、main と develop の両方に PR
git switch main && git pull && git switch -c hotfix/crash-on-launch
# ... 修正 ...
gh pr create --base main    --title "fix(ios): 起動時クラッシュ"
gh pr create --base develop --title "fix(ios): 起動時クラッシュ (backport)"

# release: デモ前に develop を凍結したい時だけ
git switch develop && git pull && git switch -c release/demo-day2
gh pr create --base main --title "release: Day2 デモ版"
```

## コミット規約(Conventional Commits)

`<type>(<scope>): <日本語 or 英語の要約>`

| type | 用途 | 例 |
|---|---|---|
| `feat` | 機能追加 | `feat(ai): DSL パッチ生成エンドポイント` |
| `fix` | バグ修正 | `fix(ios): 音声停止時にクラッシュ` |
| `docs` | ドキュメントのみ | `docs: 00_decisions に DB 選定を追記` |
| `chore` | 設定・雑務 | `chore: gitignore に xcresult を追加` |
| `ci` | CI/CD | `ci: ios-ci に DerivedData キャッシュ` |
| `refactor` / `test` | 挙動を変えない整理 / テスト | |

scope は `ios` / `appconfig` / `ai` / `dsl` / `infra` あたり。1コミット1意図。`WIP` コミットは squash 前提なら可。

## CI が落ちた時の見方

1. PR の Checks タブ → 赤い job をクリック → 失敗した step を開く(`::error` 行が要点)。
2. 各 job の典型的な原因:
   - `backend-ci`: `golangci-lint` の指摘(未使用変数・エラー未処理)、`go test -race` のデータ競合。ローカルで `cd backend && go vet ./... && go test ./... -race` を先に通す。
   - `ios-ci`: ビルドエラーはローカルの `xcodegen generate && xcodebuild build -destination 'generic/platform=iOS Simulator' CODE_SIGNING_ALLOWED=NO` で再現。**タイムアウト(25分)** なら SwiftPM の依存が重い → キャッシュが効いているか(`SwiftPM キャッシュ` step の cache-hit)を見る。macOS runner は分単価が高いので、原因を特定せずに Re-run を連打しない。
   - `secrets-scan`: gitleaks が秘密らしき文字列を検出。**本物なら即座にキーを無効化(ローテート)**してから履歴を消す(`git filter-repo`)。誤検知なら `.gitleaksignore` に fingerprint を追加。
3. 直して push すれば同じ PR の古い実行は自動キャンセルされ再実行される。
4. どうしても原因不明なら `gh run view <run-id> --log-failed` でログを全部落として読む。

## deploy-cloudrun(backend の自動デプロイ)

`.github/workflows/deploy-cloudrun.yml`。GCP 側の構築手順と実値は `docs/infra/gcp-setup.md`。

| トリガー | デプロイ先(Cloud Run / `asia-northeast1`) | GitHub Environment |
|---|---|---|
| `develop` に push | `appconfig-stg` / `ai-stg` | `staging` |
| `main` に push | `appconfig-prod` / `ai-prod` | `production` |
| `workflow_dispatch` | 選んだ環境(backend 未変更でも強制デプロイ) | 選んだ環境 |

- push では `backend/**` かこの workflow 自身に変更がある時だけ本体ジョブが走る(`dorny/paths-filter`。workflow の `paths:` は required check 対策で使わない)。
- 流れ: `backend/Dockerfile` を 1 回 `docker build`(appconfig / ai 両バイナリ同梱)→ Artifact Registry `asia-northeast1-docker.pkg.dev/jphacks-kz2601/backend/backend:<sha>` → 同じイメージで `appconfig-<env>` と `ai-<env>`(`--command=/app/ai`、`ANTHROPIC_API_KEY` は Secret Manager から注入)を `gcloud run deploy --image`。
- 最後に両サービスの **`GET /health`** が 200 であることを確認し、URL を job summary に出す。**ヘルスパスは `/health`**(Cloud Run の Google Front End は `/healthz` をコンテナに届けず HTML 404 を返すため。`/healthz` はローカル用に残っている)。
- `--set-env-vars` / `--set-secrets` は全置換。手で足した env/secret は次のデプロイで消えるので、恒久的に要るものは workflow に書く。

認証は Workload Identity Federation(鍵ファイル無し)。GitHub **Secrets は不要**で、repo Variables に以下を入れる(登録済み)。

| Variable | 値 |
|---|---|
| `GCP_PROJECT_ID` | `jphacks-kz2601` |
| `GCP_REGION` | `asia-northeast1` |
| `GCP_WORKLOAD_IDENTITY_PROVIDER` | `projects/378953239426/locations/global/workloadIdentityPools/github/providers/github` |
| `GCP_SERVICE_ACCOUNT` | `github-deployer@jphacks-kz2601.iam.gserviceaccount.com` |

`github-deployer` に必要な IAM(付与済み): `roles/run.admin`、`roles/artifactregistry.writer`(プロジェクト)、`roles/iam.serviceAccountUser`(`run-backend` SA に対して。`--service-account` の actAs 用)、`roles/iam.workloadIdentityUser`(WIF の principalSet から。`auth` アクションの SA なりすましに `getAccessToken` を含む)。`roles/iam.serviceAccountTokenCreator` は不要(署名/ID トークン発行をしないため)。

## 初回セットアップ(オーナーが1回だけ)

```sh
# main/develop のブランチ保護(人間のみマージ可・CI 必須・force push 禁止)。develop は push 済み(無ければ自動作成)
scripts/github/setup-branch-protection.sh --dry-run   # 内容確認(API 書き込みなし)
scripts/github/setup-branch-protection.sh             # 適用
```

詳細はスクリプト冒頭のコメント参照(classic branch protection を選んだ理由、注意点)。
