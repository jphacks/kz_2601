# CONTRIBUTING

Tailor It(jphacks/kz_2601)に変更を入れる人とエージェントのための規約。
正典は [`docs/00_decisions.md`](docs/00_decisions.md)、手順の詳細は [`docs/02_github_workflow.md`](docs/02_github_workflow.md)。ここはその要約と、そこに書いていない約束。
エージェント(Claude Code)は加えて [`CLAUDE.md`](CLAUDE.md) を読む。

## 目次

1. [ブランチ](#ブランチ)
2. [コミット](#コミット)
3. [Issue](#issue)
4. [PR](#pr)
5. [レビューの観点](#レビューの観点)
6. [ローカルでのテストと lint](#ローカルでのテストと-lint)
7. [秘密情報](#秘密情報)

## ブランチ

| ブランチ | 役割 |
|---|---|
| `develop` | **既定のブランチ。PR は基本ここに出す**。マージで Cloud Run staging にデプロイ |
| `main` | リリース(本番)。`develop` から実機テスト済みのものだけ。マージで Cloud Run prod にデプロイ |
| `feature/<topic>` | 機能追加 |
| `fix/<topic>` | バグ修正 |
| `docs/<topic>` | ドキュメントだけの変更 |
| `test/` `chore/` `ci/` | テスト・雑務・CI |
| `hotfix/<topic>` | 本番の緊急修正(`main` から切り、`main` と `develop` の両方に PR) |

- `develop` から切る: `git fetch origin && git switch -c feature/voice-screen origin/develop`
- `<topic>` は英語の kebab-case。Issue 番号を末尾に付けてよい(例: `docs/contributing-300`)。
- `develop` / `main` に直接 push しない(ブランチ保護あり)。

## コミット

[Conventional Commits](https://www.conventionalcommits.org/ja/)。**件名は日本語**(履歴に合わせる)。

```
<type>(<scope>): <日本語の要約>(#Issue番号)
```

- type: `feat` / `fix` / `docs` / `test` / `refactor` / `chore` / `ci`
- scope: `ios` / `backend` / `ai` / `appconfig` / `dsl` / `infra` / `legal` など
- 例: `feat(backend): 退会から 30 日を過ぎたアカウントを物理削除する purge ジョブ (#458)`
- 1 コミット 1 意図。

## Issue

- テンプレート(バグ / 機能 / タスク)から作る。**「完了の条件」と「依存」は必須**(依存が無ければ「なし」)。
- マイルストーンと優先度ラベル(`priority:P1`〜`P3`)を付ける。
- 着手したら `in-progress` ラベルを付ける。人がやる作業は `human`、判断待ちは `decision`。

## PR

- base は **`develop`**(リリースの PR だけ `main`)。まず Draft で出し、動作確認が済んだら Ready for review(CI はそこで走る)。
- 本文は [`PULL_REQUEST_TEMPLATE.md`](.github/PULL_REQUEST_TEMPLATE.md) どおり。UI の変更はスクショ必須。
- **Assignee**(作った人)と **Milestone**(Issue と同じもの)を付ける。レビュアーは CODEOWNERS で自動。
  - 例外: `/jphacks-task` でエージェントが出す PR にはマイルストーンを付けない(マイルストーンが「進めてよい範囲」の判定に使われるため。`.claude/skills/jphacks-task/SKILL.md`)。
- 本文に `Closes #N` を書く。ただし **base が `develop` なので、マージしても Issue は自動で閉じない**(GitHub は既定ブランチ `main` へのマージでしか閉じない)。**マージした人が Issue を手で閉じる**(`gh issue close N -c "#PR で対応"`)。
- 1 PR = 1 トピック。大きくなったら分ける。
- マージ条件: CI(`backend-ci` / `ios-ci` / `secrets-scan`)が緑、レビューコメントが全部 Resolved。マージは人間だけ(エージェントはしない)。`feature/*` → `develop` は Squash merge。

## レビューの観点

| 観点 | 見るところ |
|---|---|
| スコープ | Issue の「完了の条件」を満たすか。関係ない変更が混ざっていないか |
| 契約 | DSL は `docs/dsl/dsl.schema.json` が正典(`backend/pkg/dsl/schema.json` と一致)。API は `docs/backend/openapi-*.yaml`。二重定義していないか |
| 安全 | 秘密情報・個人情報がコード・ログ・テストデータに無いか。認可(他人の appId / uid を触れないか) |
| 審査用の版 | 審査で外す部品に触れたら `ios/scripts/build-review.sh build` が通るか(`docs/ios/review-build.md`) |
| テスト | 直したバグに回帰テストがあるか。CI 任せにせずローカルで通したか |
| 読みやすさ | 名前・コメントは短く。正典(docs)の更新が要る変更なら docs も同じ PR で直す |

コメントの頭にタグを付ける: `[must]`(直さないとマージしない)/ `[imo]` / `[nits]` / `[Q]`。

## ローカルでのテストと lint

### Go(`backend/`)

```sh
cd backend
go vet ./...
go test ./... -race -count=1
golangci-lint run --timeout=3m ./...   # CI(backend-ci)と同じ。設定ファイルは無く既定のルール
make schema-check                      # DSL スキーマの正典とコピーが一致しているか
```

backend-ci は 5 分以内が必須。遅くなったらテストを削らず原因を直す。

### iOS(`ios/`)

```sh
cd ios
xcodegen generate                       # .xcodeproj はコミットしない(project.yml から生成)
xcodebuild -scheme Metamon -destination 'platform=iOS Simulator,name=iPhone 17' test CODE_SIGNING_ALLOWED=NO
(cd Packages/MetamonDSL && swift test)  # DSL だけなら数秒
```

- 審査用の版に関わる変更は `./scripts/build-review.sh build` も通す。
- 書式: `swift format lint --recursive --configuration .swift-format Metamon MetamonWidget MetamonTests MetamonUITests Packages`(Xcode 16 以降に同梱)。CI の `ios-format` ジョブでも回るが、**今は非ブロッキング**(既存コードに指摘が多いため)。自分が触った行は直すと良い。既存コードの一括整形は専用の PR でやる。

## 秘密情報

public リポジトリ。次のものは**絶対にコミットしない**(`.gitignore` と gitleaks の二重防御があるが、頼らない)。

- `GoogleService-Info.plist`(リポ外 `~/Desktop/JPHACKS2026/secrets/` から `ios/scripts/sync-firebase-plist.sh` でコピー)
- `*.p8`(APNs / App Store Connect の鍵)・`*.pem`・`serviceAccountKey*.json`
- `.env` / `.env.*`(ローカルは `~/Desktop/JPHACKS2026/.env`、リポ外)
- API キー・トークンをコード・ログ・PR 本文・Issue に貼らない。

`secrets-scan` が赤になったら: 本物なら**先にキーを無効化(ローテート)**してから履歴を消す。誤検知なら `.gitleaksignore` に fingerprint を足す。
