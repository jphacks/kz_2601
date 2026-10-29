# CLAUDE.md(エージェント向け)

Tailor It(jphacks/kz_2601)で動く Claude Code などのエージェントの規約。人間の規約は [`CONTRIBUTING.md`](CONTRIBUTING.md)。両方に従う。迷ったら止まって人間に聞く。

## 読む順

1. この `CLAUDE.md`
2. [`CONTRIBUTING.md`](CONTRIBUTING.md) — ブランチ・コミット・PR・テストの手順
3. [`docs/00_decisions.md`](docs/00_decisions.md) — 正典(決定事項)。ここと食い違うことはしない
4. [`docs/02_github_workflow.md`](docs/02_github_workflow.md) — CI とマージの詳細
5. 担当の Issue(`gh issue view <番号> -R jphacks/kz_2601`)。「完了の条件」と「依存」を確かめる
6. 触る領域の docs: DSL は `docs/dsl/README.md` と `docs/dsl/dsl.schema.json`、iOS は `docs/ios/architecture.md`、backend は `backend/README.md`、審査用の版は `docs/ios/review-build.md`

Issue をまとめて回す時は `.claude/skills/jphacks-task/SKILL.md`(`/jphacks-task`)の手順に従う。

## 触ってよい範囲

- 担当 Issue の「やること」の範囲だけ。範囲外の直したい所は、直さずに報告する(または別 Issue を提案)。
- `docs/00_decisions.md`(正典)の変更・Issue / PR のマイルストーン変更は人間の判断。勝手にしない。
- DSL スキーマは `docs/dsl/dsl.schema.json` が正典。`backend/pkg/dsl/schema.json` と同じ内容に保つ(`make -C backend schema-check`)。
- AI のプロンプトを足す時は `backend/internal/ai/` に新しいファイルを作り `init()` で足す(`prompt.go`・`catalog.go` の本体は触らない)。

## 作業のしかた

- **git worktree で作業する**(他のエージェント・人間の作業ツリーを汚さない)。ブランチは `origin/develop` から切る: `git fetch origin && git checkout -b <feature|fix|docs|test|chore>/<短い英語名> origin/develop`
- コミットは Conventional Commits、件名は日本語(`CONTRIBUTING.md`)。
- テストを通してから報告する(Go: `go test ./...`・`golangci-lint run ./...`・`make schema-check`。iOS: `xcodegen generate` → `xcodebuild test`)。
- **`xcodebuild` は同時に 1 本だけ**。`/usr/bin/lockf -k "$TMPDIR/xcodebuild.lock" xcodebuild ...` で直列にする。
- **シミュレータを作らない・消さない**(`xcrun simctl create/delete/erase` をしない)。既存のものを使う。
- 報告は**日本語**で、短く: ブランチ・コミット・テストの件数・できなかったこと・人間の判断が要ること。

## してはいけないこと

- **`develop` / `main` に直接 push しない。**
- **PR を自分でマージしない・approve しない。**
- **承認・確認を表すラベルを自分で付けない。** 人間が確認した印は人間だけが付ける。
- 自分の PR にマイルストーンを付けて、進めてよい範囲を広げない(`/jphacks-task` の判定を参照)。
- 他の人の PR のブランチに push しない。
- **秘密情報を出力しない。** API キー・トークン・`GoogleService-Info.plist`・`.p8`・`.env` の中身をログ・コミット・PR・Issue・チャットに出さない。コミットもしない。
- **本番(prod)・stg の Firestore に書き込まない。GCP(Cloud Run・Secret Manager・IAM など)・Firebase・App Store Connect の設定を変えない。** 必要なら人間の承認を得てから、人間の指示どおりに。
- GitHub のリポジトリ設定(ブランチ保護・ラベル定義・Actions の設定)を変えない。

## 困った時

- 正典と Issue が食い違う・範囲が大きすぎる・人間の判断が要る → 作業を止めて報告する。推測で進めない。
- CI が赤 → `docs/02_github_workflow.md` の「CI が落ちた時の見方」。macOS の CI を原因不明のまま Re-run しない(分単価が高い)。
