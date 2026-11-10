# Android の CI(#356・#837)

> 状態: [`.github/workflows/android-ci.yml`](../../.github/workflows/android-ci.yml) に置いた(#837)。required check に `android-ci` を足すのは人間。

## 目次

1. [形](#形)
2. [残り](#残り)

## 形

[#354 §8](../decisions/354-android-tech.md#ci-と費用) と `ios-ci` に揃える。

- **ubuntu の runner だけ**(macOS は使わない)。エミュレータも使わない(JVM の単体テストだけ)。
- `android-changes`(`dorny/paths-filter`)で変わった場所を見て、関係ない PR では Gradle を起動しない。
- workflow の `paths:` は使わない(required check が永遠に待ちになるため・`backend-ci` と同じ)。required check は集約ジョブ `android-ci`(スキップでも成功で終わる)。
- `google-services.json` は CI に置かない(Secret にも入れない)。無くてもビルドは警告で通る(`app/build.gradle.kts` の `MissingGoogleServicesStrategy.WARN`)。

| 変わった場所 | ジョブ | 回すもの |
|---|---|---|
| `android/dsl/**` だけ(PR) | `android-light` | `./gradlew :dsl:test` |
| `docs/dsl/conformance/expr/**` だけ | `android-light` | `./gradlew :dsl:test` |
| `docs/dsl/icon-map.json`・backend の `*.go`(テスト以外) | `android-light` | `:renderer` の `SymbolIconTableTest` |
| それ以外の `android/**`・`docs/dsl/conformance/**`・`docs/dsl/examples/**`・`docs/dsl/dsl.schema.json`・このワークフロー | `android-build` | `./gradlew test assembleStgDebug` + lint(非ブロッキング) |
| develop / main への push で `android/dsl/**` を含む | `android-build` | 同上 |
| 上記以外 | なし | なし(`android-ci` は成功で終わる) |

Gradle のキャッシュは `gradle/actions/setup-gradle`(PR は読むだけ)。

## 残り

1. 緑になったら、ブランチの保護の required check に `android-ci` を足す(人間・`scripts/github/setup-branch-protection.sh`)。
2. lint をブロッキングにする: 既存の指摘を直す(または `lint-baseline.xml`)→ `continue-on-error` を外す。
3. ライセンス一覧の検査(`android/scripts/third-party-licenses.py --check`)を足す。今は checker-qual の判定で止まるので、直してから。
4. develop への push だけのエミュレータの試験(#354 §8・android-085)。
