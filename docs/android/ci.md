# Android の CI(案・#356)

> 状態: **案**。ワークフローのファイル(`.github/workflows/android-ci.yml`)はまだ置いていない。置く時はこの文書の YAML をそのまま使い、required check に `android-ci` を足す(人間)。

## 目次

1. [形](#形)
2. [ワークフロー](#ワークフロー)
3. [置く時にやること](#置く時にやること)

## 形

[#354 §8](../decisions/354-android-tech.md#ci-と費用) と `ios-ci` に揃える。

- **ubuntu の runner だけ**(macOS は使わない)。エミュレータも使わない(JVM の単体テストだけ)。
- `android-changes`(`dorny/paths-filter`)で変わった場所を見て、関係ない PR では Gradle を起動しない。
- workflow の `paths:` は使わない(required check が永遠に待ちになるため・`backend-ci` と同じ)。required check は集約ジョブ `android-ci`(スキップでも成功で終わる)。
- `google-services.json` は CI に置かない(Secret にも入れない)。無くてもビルドは警告で通る(`app/build.gradle.kts` の `MissingGoogleServicesStrategy.WARN`)。

| 変わった場所 | ジョブ | 回すもの |
|---|---|---|
| `android/dsl/**` だけ | `android-dsl` | `./gradlew :dsl:test` |
| `docs/dsl/examples/**`(`:dsl` のテストが読み、アプリにも同梱) | `android-build` | 全部 |
| それ以外の `android/**`・このワークフロー | `android-build` | `./gradlew test assembleStgDebug` |
| 上記以外 | なし | なし(`android-ci` は成功で終わる) |

## ワークフロー

```yaml
# Android(Kotlin / Compose)の CI。PR が Open(非Draft) / develop・main への push で発火。
# ubuntu だけで回す(macOS の 1/10 の分単価)。エミュレータは使わない(JVM の単体テストだけ)。
# required check 名 = 集約ジョブ `android-ci`。関係ない変更ではビルドをスキップして成功で終わる。
name: android-ci

on:
  pull_request:
    types: [opened, synchronize, reopened, ready_for_review]
  push:
    branches: [develop, main]

concurrency:
  group: android-ci-${{ github.event.pull_request.number || github.ref }}
  cancel-in-progress: ${{ github.event_name == 'pull_request' }}

permissions:
  contents: read
  pull-requests: read

jobs:
  android-changes:
    name: android-changes
    if: github.event_name == 'push' || github.event.pull_request.draft == false
    runs-on: ubuntu-latest
    timeout-minutes: 3
    outputs:
      dsl: ${{ steps.filter.outputs.dsl }}
      app: ${{ steps.filter.outputs.other == 'true' || steps.filter.outputs.examples == 'true' || steps.filter.outputs.workflow == 'true' }}
    steps:
      - uses: actions/checkout@v7
      # every: 1 つのフィルタの中のパターンを全部満たすファイルだけ数える(! の除外を効かせるため)
      - uses: dorny/paths-filter@v4
        id: filter
        with:
          predicate-quantifier: every
          filters: |
            dsl:
              - 'android/dsl/**'
            other:
              - 'android/**'
              - '!android/dsl/**'
              - '!android/**/*.md'
            examples:
              - 'docs/dsl/examples/**'
            workflow:
              - '.github/workflows/android-ci.yml'

  android-dsl:
    name: android-dsl
    needs: android-changes
    if: needs.android-changes.outputs.dsl == 'true' && needs.android-changes.outputs.app != 'true'
    runs-on: ubuntu-latest
    timeout-minutes: 15
    defaults:
      run:
        working-directory: android
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-java@v6
        with:
          distribution: temurin
          java-version: '17'
      - uses: gradle/actions/setup-gradle@v6
        with:
          cache-read-only: ${{ github.event_name == 'pull_request' }}
      - run: ./gradlew :dsl:test --console=plain

  android-build:
    name: android-build
    needs: android-changes
    if: needs.android-changes.outputs.app == 'true'
    runs-on: ubuntu-latest
    timeout-minutes: 30
    defaults:
      run:
        working-directory: android
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-java@v6
        with:
          distribution: temurin
          java-version: '17'
      - uses: gradle/actions/setup-gradle@v6
        with:
          cache-read-only: ${{ github.event_name == 'pull_request' }}
      # ubuntu-latest には Android SDK が入っている(ANDROID_HOME)。platform 36 が無ければ AGP が取ってくる
      - run: ./gradlew test assembleStgDebug --console=plain
      - name: テストの結果を残す
        if: failure()
        uses: actions/upload-artifact@v7
        with:
          name: android-test-reports
          path: android/**/build/reports/tests/
          retention-days: 7

  android-ci:
    name: android-ci
    needs: [android-changes, android-dsl, android-build]
    if: always() && (github.event_name == 'push' || github.event.pull_request.draft == false)
    runs-on: ubuntu-latest
    timeout-minutes: 2
    steps:
      - name: 結果を集約
        env:
          CHANGES: ${{ needs.android-changes.result }}
          DSL: ${{ needs.android-dsl.result }}
          BUILD: ${{ needs.android-build.result }}
        run: |
          echo "android-changes=${CHANGES} android-dsl=${DSL} android-build=${BUILD}"
          if [ "$CHANGES" != "success" ]; then
            echo "::error::android-changes が ${CHANGES}"
            exit 1
          fi
          for r in "$DSL" "$BUILD"; do
            case "$r" in
              success|skipped) ;;
              *) echo "::error::ジョブが ${r}"; exit 1 ;;
            esac
          done
```

## 置く時にやること

1. 上の YAML を `.github/workflows/android-ci.yml` に置く PR を出す(この PR で Android の骨格がビルドできることを CI で確かめる)。
2. 緑になったら、ブランチの保護の required check に `android-ci` を足す(人間)。
3. actions の版(`checkout@v7`・`setup-java@v6`・`setup-gradle@v6`・`upload-artifact@v7`)は置く時にもう一度確かめる。
4. 後で足す: `lint`(`./gradlew lintStgDebug`)、Robolectric の Compose テスト(`:renderer`)、develop への push だけのエミュレータの試験(#354 §8)。
