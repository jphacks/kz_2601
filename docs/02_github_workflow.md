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
2. 動作確認が終わったら **Ready for review** にする → CI 発火(backend-ci は 5分以内厳守、ios-ci は変更のあったモジュールのテストだけ回す・上限30分。下の「ios-ci の仕組み」)。
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
- `scripts-ci`(`.github/workflows/scripts-ci.yml`)は `*.sh`・`scripts/**`・`.github/workflows/**` を触った時だけ走り、shellcheck・`scripts/ai-cost-test.sh`・actionlint を回す。required check ではない(ローカルでは `git ls-files -z '*.sh' | xargs -0 shellcheck` と `actionlint`)。

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

## ios-ci の仕組み(変更のあったモジュールだけテスト)

macOS runner は Linux の 10 倍の分単価なので、ubuntu の `ios-changes` ジョブ(`dorny/paths-filter`)で変わった場所を見て、必要なテストだけ macOS で回す。required check は ubuntu の集約ジョブ `ios-ci`(macOS ジョブがスキップでも成功で終わる)。

| 変わった場所 | macOS ジョブ | 回るテスト |
|---|---|---|
| `ios/Packages/MetamonDSL/**` だけ | `ios-dsl` | macOS の `xcodebuild test`(MetamonDSLTests・`ios/scripts/mac-package-test.sh`)。アプリのビルド・シミュレータ無し |
| MetamonDSL + 他の iOS 変更 | `ios-xcode` | MetamonDSLTests / MetamonRendererTests / MetamonServicesTests / MetamonTests |
| `ios/Packages/MetamonRenderer/**` | `ios-xcode` | MetamonRendererTests + MetamonTests |
| `ios/Packages/MetamonServices/**` | `ios-xcode` | MetamonServicesTests + MetamonTests |
| アプリ(`ios/Metamon/**` `ios/MetamonWidget/**` `ios/MetamonTests/**` `ios/project.yml` `ios/StoreKit/**` `docs/dsl/examples/**`) | `ios-xcode` | MetamonTests |
| `ios/MetamonUITests/**` だけ | `ios-xcode` | なし(build-for-testing でコンパイル確認のみ) |
| `.github/workflows/ios-ci.yml` | `ios-xcode` | UI 以外のユニットテスト全部 |
| 上記以外(`ios/README.md` `ios/scripts/**` 等) | なし | なし |
| develop / main への push(上記のどれかが変更) | `ios-xcode` | 全部(UI テスト MetamonUITests 含む) |

- `ios-xcode` は `xcodebuild build-for-testing` を 1 回 → `test-without-building -only-testing:<対象>` の順。
- **UI テスト(MetamonUITests)は PR では回さない**(常に `-skip-testing:MetamonUITests`)。develop / main への push で回る。
- PR で UI テストも回したい時は、PR に **`ui-tests` ラベル**を付けて、Checks タブの ios-ci で **Re-run all jobs**(または次の push)。判定ジョブが毎回 API でラベルを読み直す。ラベルを付けただけでは走らない(無関係なラベル操作で実行中の CI がキャンセルされるのを避けるため)。
  ```sh
  gh label create ui-tests --color 1D76DB --description "PR でも iOS の UI テストを回す"   # 初回だけ
  gh pr edit <番号> --add-label ui-tests
  gh run rerun <run-id>
  ```

## mac-ci(Mac アプリと共有パッケージの macOS ビルド・テスト)

`.github/workflows/mac-ci.yml`(#928)。ios-ci と同じく ubuntu の `mac-changes` で変わった場所を見て、必要な時だけ macOS で回す。**required check にするかはオーナーが判断する**(今の `REQUIRED_CHECKS` には入っていない)。

- きっかけ: PR(opened / synchronize / reopened / ready_for_review・Draft は除く)と `develop` / `main` への push。PR では新しい push で古い実行を取り消す。push では取り消さない。
- ジョブ(名前は workflow と同じ): `mac-changes`(ubuntu・判定)→ `mac-build-test`(`macos-26`・上限 40 分)→ `mac-ci`(ubuntu・集約。`mac-build-test` がスキップでも成功、失敗・キャンセルなら失敗)。
- `ios/MetamonMac` が無いブランチでは `mac-build-test` を飛ばす。

| 変わった場所 | Mac アプリ(MetamonMac)のビルドと MetamonMacTests | パッケージのテスト(macOS) |
|---|---|---|
| `ios/Packages/MetamonDSL/**`・DSL のテストデータ(`docs/dsl/conformance/expr/**`・`docs/dsl/dsl.schema.json`・`docs/dsl/examples/**`・`backend/internal/ai/examples/**`・`backend/pkg/dsl/testdata/**`) | する | DSL / Renderer / Services(DSL は全員の依存) |
| `ios/Packages/MetamonRenderer/**`・`docs/dsl/conformance/**`・`docs/dsl/examples/**` | する | Renderer |
| `ios/Packages/MetamonServices/**` | する | Services |
| `ios/MetamonMac/**`・`ios/MetamonMacTests/**`・`ios/Shared/**`(iOS と共有するランタイムの部品)・`ios/project.yml`・`ios/scripts/select-firebase-plist.sh` | する | なし |
| `.github/workflows/mac-ci.yml`・`ios/scripts/mac-package-test.sh` | する | 全部 |
| 上のどれも変わっていない | しない | なし(`mac-ci` は成功) |
| `develop` / `main` への push(上のどれかが変更) | する | 全部 |

**回す物**: `xcodegen generate` → `MetamonMac` の Debug ビルド(`platform=macOS`)→ Mac アプリをホストにした単体テスト `MetamonMacTests`(ビルドする時はいつも)→ 表で決まったパッケージのテスト(`ios/scripts/mac-package-test.sh <パッケージ名>`・各パッケージのフォルダで `xcodebuild test -destination platform=macOS`)。

**回さない物**(重い・要らない物は macOS の分を使わない):

- 署名・アーカイブ・配布用のビルド(`CODE_SIGNING_ALLOWED=NO`。証明書や Firebase の plist などの秘密を使わない)。
- iOS のシミュレータ・iOS アプリ・UI テスト(iOS は ios-ci が受け持つ)。
- 関係の無い変更の時の macOS ジョブ(判定は ubuntu の `mac-changes` で済ませる)。

**時間の目安**: `mac-build-test` は全部を回す時(develop への push)で 10〜15 分。キャッシュ(SwiftPM・`DerivedData-mac`)が効かない時は延びる。判定と集約は数十秒。macOS runner は分単価が高いので、原因を見ずに Re-run を重ねない。失敗した時はテスト結果(`mac-test-results`・3 日)が artifact に上がる。

**ios-ci との分担**:

| | ios-ci | mac-ci |
|---|---|---|
| 対象 | iOS アプリ(シミュレータ)と、それが使うパッケージ | Mac アプリ(MetamonMac)と、それが使うパッケージ |
| MetamonDSL だけの変更 | `ios-dsl` が macOS で MetamonDSLTests を回す | `mac-build-test` が DSL / Renderer / Services を回す |
| UI テスト | develop / main への push(PR は `ui-tests` ラベルの時) | 無し |

- 今は MetamonDSL だけを変えた PR で、DSL のテストが ios-ci と mac-ci の両方で回る(二重)。まとめ方は #1135 で決める(案: ios-ci の `ios-dsl` をやめて mac-ci に任せる)。**#1135 の決まりが出たら、この節と ios-ci の表をそれに合わせて直す。**
- 手元で回す時、repo か DerivedData が `~/Desktop`・`~/Documents`・`~/Downloads` の下にあると、テストの実行役が読めずに落ちる(macOS の保護)。repo(clone)と DerivedData(`MAC_DERIVED_DATA`)の両方を保護の外(例: `~/src`)に置く。CI では起きない。

## backend-emulator(Firestore エミュレータ試験・必須ではない)

`.github/workflows/backend-emulator.yml`(#546)。`backend-ci` は 5 分上限なので別 workflow。

**何を回すか**: エミュレータが無いと `t.Skip("FIRESTORE_EMULATOR_HOST not set")` で飛ばされる store まわりのテスト。対象のパッケージは決め打ちせず、`scripts/firestore-emulator.sh test` がその文字列を持つ `*_test.go` を grep して集める(今は `internal/store`・`internal/appconfig`・`internal/runner` とその下の `queue` / `quota`・`internal/ai`・`internal/coins` など)。

**いつ回るか**:

| きっかけ | 試験ジョブ `backend-emulator-test` |
|---|---|
| PR(opened / synchronize / reopened / ready_for_review・Draft は除く)で `backend/**`・`.github/workflows/backend-emulator.yml`・`scripts/firestore-emulator.sh` のどれかが変わった | 回る |
| 上の 3 つのどれも変わっていない PR | 飛ばす(集約ジョブは成功) |
| `develop` / `main` への push(push 前との差分に上の 3 つのどれかがある) | 回る |

- workflow の `paths:` は使わず、`backend-emulator-changes`(`dorny/paths-filter`)で判定する(`backend-ci` と同じ理由: required にした時に check が待ちのままにならないように)。
- ジョブは 3 つ: `backend-emulator-changes`(判定)→ `backend-emulator-test`(試験・上限 15 分)→ `backend-emulator`(集約。試験を飛ばしたら成功、回したらその結果)。
- 時間の目安: 試験ジョブが回った時で 3〜4 分(gcloud とエミュレータの取得込み)。回らない時は判定と集約で数十秒。

**中身**: gcloud SDK の `cloud-firestore-emulator`(版は固定)を `127.0.0.1:8089`・プロジェクト `demo-kz2601` で起動し、集めたパッケージに `go test -count=1 -json` をかける。`demo-` プロジェクトで認証情報も渡さないので実 GCP には繋がらない。

**skip されていないかの確かめ方**(`scripts/firestore-emulator.sh` の `test` が見る):

- 出力に `FIRESTORE_EMULATOR_HOST not set` が 1 回でも出たら失敗(`::error::emulator tests were skipped`)。
- Firestore 系テスト(名前が `TestFirestore*` か、サブテストが `*/firestore`)が 1 件でもスキップされたら失敗し、その名前を並べる。
- 通った Firestore 系テストの件数を job summary に `Firestore emulator tests passed: <件数>` で出す。0 件なら失敗。件数が前より急に減っていたら、テストが別の理由でスキップされていないかを見る。

**落ちた時の見方**:

1. `backend-emulator` が赤 → 中身は `backend-emulator-test` の方。そちらを開く。
2. `Firestore エミュレータ起動` の step で落ちた → 90 秒で応答しなかった。ログ(`emulator.log`)はその step の中と、最後の `エミュレータログ(失敗時)` に出る。gcloud・Java の取得の失敗なら Re-run でよいことが多い。
3. `go test` の step で落ちた → 上の `::error::` 行(スキップ・件数 0)か、普通のテストの失敗。手元で `make -C backend test-emulator` で再現する(エミュレータが無ければ一時的に起動して止める)。開発用に起動しっぱなしにするなら `make -C backend emulator`(別ターミナル)。
4. `backend-emulator-changes` が赤 → paths-filter の失敗。Re-run。

**required check にはまだしていない**(`scripts/github/setup-branch-protection.sh` の `REQUIRED_CHECKS` は `backend-ci` / `ios-ci` / `secrets-scan` の 3 本)。必須にするかはオーナーが判断する(決めること)。必須にする時に登録する名前は集約ジョブの `backend-emulator`。

## CI が落ちた時の見方

1. PR の Checks タブ → 赤い job をクリック → 失敗した step を開く(`::error` 行が要点)。
2. 各 job の典型的な原因:
   - `backend-ci`: `golangci-lint` の指摘(未使用変数・エラー未処理)、`go test -race` のデータ競合。ローカルで `cd backend && go vet ./... && go test ./... -race` を先に通す。
   - `ios-ci`: 実体は `ios-dsl` / `ios-xcode` ジョブ(`ios-ci` はその結果を集約するだけ)。赤い方を開く。DSL だけなら `ios/scripts/mac-package-test.sh MetamonDSL` で再現(`swift test` は文字列カタログを訳さないので英語の文言のテストだけ落ちる)。ビルドエラーはローカルの `xcodegen generate && xcodebuild build -destination 'generic/platform=iOS Simulator' CODE_SIGNING_ALLOWED=NO` で再現。**タイムアウト(30分)** なら SwiftPM の依存が重い → キャッシュが効いているか(`SwiftPM キャッシュ` step の cache-hit)を見る。macOS runner は分単価が高いので、原因を特定せずに Re-run を連打しない。
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
- 手で回す: Actions の deploy-cloudrun → Run workflow で `environment`(`staging` / `production`)と `staged_rollout`(`dry-run` / `execute`・staging だけ効く)を選ぶ。`gh workflow run deploy-cloudrun.yml -f environment=staging` でも同じ。
- 同じ環境へのデプロイは直列(`concurrency: deploy-<環境>`・`cancel-in-progress: false`)。後から来た実行は前の実行が終わるまで待つ(途中で切らない)。待っている実行が 2 本になると、古い方は GitHub が取り消す(最新の版だけが出る)。

**ジョブ**(名前は workflow の `name:` と同じ):

| ジョブ | 役割 |
|---|---|
| `deploy-changes` | 変更の判定と、行き先(`environment`・`suffix` = `stg` / `prod`)・段階的なリリースの形(`rollout` = `off` / `dry-run` / `execute`)を決める |
| `deploy` | 下の流れ全部。GitHub Environment(`staging` / `production`)に紐づく。上限は 20 分(`execute` の時だけ 70 分) |

**環境ごとの違い**:

| | staging(`develop` / 手で staging) | production(`main` / 手で production) |
|---|---|---|
| サービス | `appconfig-stg` / `ai-stg` | `appconfig-prod` / `ai-prod`、変数 `RUNNER_DEPLOY=1` の時だけ `runner-prod`(非公開・手順は [docs/infra/runner.md](infra/runner.md)) |
| `APP_ENV` / `ALLOW_SHARED_PROD_DB` | `stg` / `1`(DB の分離まで本番と同じ DB を使う) | `prod` / `0` |
| 段階的なリリース | push は `dry-run`(100% で出してコマンドを log に出すだけ)。手で `execute` を選ぶと `--no-traffic` で出して 5 → 25 → 100%([staged-rollout.md](infra/staged-rollout.md#deploy-の-workflow-での動き)) | `off`(いつも 100%。`execute` を選んでも warning を出して無視) |
| スモーク(非ブロッキング) | デモ 3 本 | 家計簿 1 本 + `-cleanup` |
| `APP_CHECK_MODE`・`FIRESTORE_DATABASE` | GitHub の Environment 変数(未設定なら `off`・`(default)`) | 同じ(Environment ごとに別の値を入れられる) |

**env の表の検査(`scripts/check-deploy-env.sh`)との関係**: `deploy` の最初の step で `scripts/check-deploy-env.sh` を回し、workflow の `--update-env-vars` のキーが [runtime-config.md の「デプロイが決める値」](infra/runtime-config.md#デプロイが決める値)の表と合っているか(`--set-env-vars` を使っていないか・「運用で変える値」の表のキーを上書きしていないか)を見る。ずれていればビルドの前に止まる。同じ検査は `scripts-ci` でも回るので、workflow か runtime-config.md を変える PR の時点で気づける。env を足す・外す時は workflow と runtime-config.md の表を同じ PR で直す。

**止める・戻す(人の操作)**:

- 走っているデプロイを止める: Actions の画面で Cancel。`execute` の段階の途中なら、旧のリビジョンに 100% 戻してから終わる([staged-rollout.md の「人が途中で止める」](infra/staged-rollout.md#人が途中で止める))。それ以外の step の途中で止めると、そこまでに出したサービスは新しい版のまま。
- 出てしまった悪い版を戻す: 前のリビジョンにトラフィックを戻す([incident-runbook.md 手順 B](ops/incident-runbook.md#6-手順-b-悪いデプロイを戻す))。次の push で同じ版がまた出ないよう、`develop` / `main` に戻すコミットを入れる。
- 推測: 自動デプロイそのものを一時的に止めたい時は、Actions の画面で workflow を Disable するのが一番早い(workflow に止めるための変数は無い)。止めるかどうか・誰が戻すかは決めること。
- 流れ: `backend/Dockerfile` を 1 回 `docker build`(appconfig / ai 両バイナリ同梱)→ Artifact Registry `asia-northeast1-docker.pkg.dev/jphacks-kz2601/backend/backend:<sha>` → 同じイメージで `appconfig-<env>` と `ai-<env>`(`--command=/app/ai`、`ANTHROPIC_API_KEY` は Secret Manager から注入)を `gcloud run deploy --image`。
- 最後に両サービスの **`GET /health`** が 200 であることを確認し、URL を job summary に出す。**ヘルスパスは `/health`**(Cloud Run の Google Front End は `/healthz` をコンテナに届けず HTML 404 を返すため。`/healthz` はローカル用に残っている)。
- その後 **スモーク(`backend/cmd/smoke`・非ブロッキング)** を流して表を job summary に貼る(失敗してもデプロイは成功扱い)。staging はデモ 3 本(Claude 3 回)、**production は家計簿 1 本だけ(`-only kakeibo`・Claude 2 回)+ `-cleanup`** で作った `smoke-*` アプリを `DELETE /v1/apps/{id}`(archive)する。認証は `FIREBASE_WEB_API_KEY`(repo Variable)で Firebase 匿名ユーザーの ID トークンを取る。手元から同じことをするには `cd backend && FIREBASE_WEB_API_KEY=<key> make smoke-stg` / `make smoke-prod`(#66)。
- env は `--update-env-vars`(#509)。workflow が決める値だけを上書きし、運用で手で入れた値(`NOTICE_JSON`・`MIN_IOS_BUILD`・`RATE_LIMIT_GLOBAL_DAILY` など)は残す。workflow から外した変数は Cloud Run に残るので `--remove-env-vars` で消す。シークレットは `--set-secrets`(全置換)のまま。一覧と変え方は [`docs/infra/runtime-config.md`](infra/runtime-config.md)。

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
