# Android の CI(#356・#837)

> 状態: [`.github/workflows/android-ci.yml`](../../.github/workflows/android-ci.yml) に置いた(#837)。required check に `android-ci` を足すのは人間。

## 目次

1. [形](#形)
2. [CI で検証されるもの / 手元で確かめるもの](#ci-で検証されるもの--手元で確かめるもの)
3. [残り](#残り)

## 形

[#354 §8](../decisions/354-android-tech.md#ci-と費用) と `ios-ci` に揃える。

- **ubuntu の runner だけ**(macOS は使わない)。エミュレータも使わない(JVM の単体テストだけ)。androidTest(instrumented test)は**コンパイルだけ**確かめる(#2062)。
- `android-changes`(`dorny/paths-filter`)で変わった場所を見て、関係ない PR では Gradle を起動しない。
- workflow の `paths:` は使わない(required check が永遠に待ちになるため・`backend-ci` と同じ)。required check は集約ジョブ `android-ci`(スキップでも成功で終わる)。
- `google-services.json` は CI に置かない(Secret にも入れない)。無くてもビルドは警告で通る(`app/build.gradle.kts` の `MissingGoogleServicesStrategy.WARN`)。

| 変わった場所 | ジョブ | 回すもの |
|---|---|---|
| `android/dsl/**` だけ(PR) | `android-light` | `./gradlew :dsl:test` |
| `docs/dsl/conformance/expr/**` だけ | `android-light` | `./gradlew :dsl:test` |
| `docs/dsl/icon-map.json`・backend の `*.go`(テスト以外) | `android-light` | `:renderer` の `SymbolIconTableTest` |
| iOS の `ClientCapabilities.swift` | `android-light` | `:renderer` の `DeviceCapabilitiesTest` |
| iOS の `Localizable.xcstrings` | `android-light` | `:app` の `RuntimeCopyIosEnglishTest` |
| それ以外の `android/**`・`docs/dsl/conformance/**`・`docs/dsl/examples/**`・`docs/dsl/dsl.schema.json`・このワークフロー | `android-build` | `./gradlew test assembleStgDebug` + `./gradlew :app:compileStgDebugAndroidTestKotlin` + lint(非ブロッキング) |
| develop / main への push で `android/dsl/**` を含む | `android-build` | 同上 |
| 上記以外 | なし | なし(`android-ci` は成功で終わる) |

Gradle のキャッシュは `gradle/actions/setup-gradle`(PR は読むだけ)。

## CI で検証されるもの / 手元で確かめるもの

| | CI(`android-build`) | 手元 |
|---|---|---|
| JVM の単体テスト(`src/test/`) | ○ `./gradlew test` | — |
| `:app` の stg debug のビルド | ○ `./gradlew assembleStgDebug` | — |
| **androidTest のコンパイル**(`src/androidTest/`) | ○ `./gradlew :app:compileStgDebugAndroidTestKotlin` | — |
| **androidTest の実行**(`connectedStgDebugAndroidTest`) | ✗ **回さない** | **実機 / エミュレータで人が回す** |
| Android Lint | △ 非ブロッキング | — |
| ライセンス一覧の検査 | ✗ まだ(下の「残り」3) | `android/scripts/third-party-licenses.py --check` |

androidTest は**コンパイルまでが CI の責任範囲**。`:app:compileStgDebugAndroidTestKotlin` を別ステップで回しているので、本番コードを変えて `android/app/src/androidTest/` が壊れた PR は CI が赤くなる。逆に、**テストが通るかどうかは CI では分からない**ので、androidTest を足した / 直した PR では手元で回して結果を PR の「動作確認」に書く。

```sh
# 実機かエミュレータを 1 台つないでから
cd android && ./gradlew :app:connectedStgDebugAndroidTest
```

> **フル実行(エミュレータ)は今後の課題**(#2062 の 2 段目・下の「残り」4)。
> `reactivecircus/android-emulator-runner` などで ubuntu の runner に AVD を立てる案があるが、
> 「ubuntu のみ・エミュレータ不使用」という上の方針に関わるので、入れるかどうかは別途人が決める。
> 入れる時は (a) 既定の PR では回さない(develop への push / `workflow_dispatch` / 特定ラベルだけ)、
> (b) required check にはしない、(c) Test Orchestrator(`clearPackageData`)を併せて設定する
> ——androidTest をまとめて回すと `AppContainer` のつまみがプロセス越しに混ざるため。

## 残り

1. 緑になったら、ブランチの保護の required check に `android-ci` を足す(人間・`scripts/github/setup-branch-protection.sh`)。
2. lint をブロッキングにする: 既存の指摘を直す(または `lint-baseline.xml`)→ `continue-on-error` を外す。
3. ライセンス一覧の検査(`android/scripts/third-party-licenses.py --check`)を足す。今は checker-qual の判定で止まるので、直してから。
4. develop への push だけのエミュレータの試験(#354 §8・android-085・#2062 の 2 段目)。今は androidTest のコンパイルだけ(上の表)。
