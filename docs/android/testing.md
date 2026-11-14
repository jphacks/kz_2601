# Android のテスト(JVM のテスト・androidTest)

どのテストがどこにあり、どのコマンドで回り、CI で回るか。CI の形そのものは [ci.md](ci.md)、ビルドのコマンドは [android/README.md](../../android/README.md#ビルドとテスト)。

## 目次

1. [種類](#種類)
2. [モジュールごとのコマンドと重さ](#モジュールごとのコマンドと重さ)
3. [1 つのテストだけ回す](#1-つのテストだけ回す)
4. [適合テスト(docs/dsl/conformance)](#適合テストdocsdslconformance)
5. [androidTest(実機・エミュレータ)](#androidtest実機エミュレータ)
6. [CI で回す物・回さない物](#ci-で回す物回さない物)
7. [決めること](#決めること)

## 種類

| 種類 | 置き場 | 動く所 | 回し方 |
|---|---|---|---|
| JVM の単体テスト | 各モジュールの `src/test/`(`:app` は debug だけの物が `src/testDebug/` にもある) | 手元の JVM(端末は要らない) | `./gradlew test` |
| androidTest(instrumented test・Compose の画面のテスト) | `:app` の `src/androidTest/` だけ(`FeedbackComposerUiTest`・`AiConsentUiTest`) | 実機かエミュレータ | `./gradlew :app:connectedStgDebugAndroidTest` |

- **Robolectric は使っていない**(どのモジュールの `build.gradle.kts` にも依存が無い)。Android の API が要るテストは androidTest に置く。JVM のテストは JUnit 4・`kotlinx-coroutines-test`・OkHttp の `MockWebServer` で書く。
- JVM のテストは debug の版だけある(`testReleaseUnitTest` などの release 用のタスクは無い)。`:app` は flavor `env` があるので stg と prod の 2 つ。

## モジュールごとのコマンドと重さ

`cd android` してから。手元で他のビルドと同時に走らせる時は、頭に `/usr/bin/lockf -k ~/Desktop/JPHACKS2026/.locks/gradle.lock` を付けて 1 本ずつにする([android/README.md](../../android/README.md#ビルドとテスト))。

| モジュール | コマンド | テストのファイル数(目安) | 重さ |
|---|---|---|---|
| 全部 | `./gradlew test` | — | 一番重い。下の 6 つを全部回す。whisper.cpp(CMake)はビルドしない |
| `:dsl`(純 Kotlin) | `./gradlew :dsl:test` | 約 30 | 軽い。Android のビルドが要らない。`docs/dsl/examples` の全例と `docs/dsl/conformance/expr` を読む |
| `:renderer` | `./gradlew :renderer:testDebugUnitTest` | 約 60 | 中くらい。適合テスト(下)を含む |
| `:services` | `./gradlew :services:testDebugUnitTest` | 約 65 | 中くらい |
| `:whisper` | `./gradlew :whisper:testDebugUnitTest` | 3 | 軽い(JNI の本体は呼ばない) |
| `:app`(stg) | `./gradlew :app:testStgDebugUnitTest` | 約 115 | 重い。全モジュールのコンパイルが要る |
| `:app`(prod) | `./gradlew :app:testProdDebugUnitTest` | 同じ物 | stg と同じテストを prod の `BuildConfig` で回す |

- 時間の目安: CI の `android-build`(`./gradlew test assembleStgDebug` と androidTest のコンパイル・lint)で、Gradle のキャッシュが効いた時で 3 分前後、効かない時で 10〜13 分(2026-11 の develop への push の実績)。
- 推測: 手元で 1 つのモジュールだけ回す時は、2 回目以降(Gradle のデーモンと設定のキャッシュが残っている時)は `:dsl:test` で数十秒、`:app` で数分。初回は依存の取得とコンパイルで延びる。
- テストの入力に repo の他の場所(`docs/dsl/**`・backend の `*.go` など)を使うテストは、その場所を Gradle の入力に入れてある(`dsl/build.gradle.kts`・`renderer/build.gradle.kts`)。そこが変われば up-to-date にならずに回り直す。

## 1 つのテストだけ回す

```sh
./gradlew :dsl:test --tests jp.kz2601.metamon.dsl.expr.ExprConformanceTest
./gradlew :renderer:testDebugUnitTest --tests jp.kz2601.metamon.renderer.ConformanceTest
./gradlew :app:testStgDebugUnitTest --tests 'jp.kz2601.metamon.language.*'
```

結果の HTML は各モジュールの `build/reports/tests/<タスク名>/index.html`。

## 適合テスト(docs/dsl/conformance)

同じ DSL を iOS と Android で描いた時に同じになるかを、画面を描かずに比べる試験。中身と期待値の決まりは [docs/dsl/conformance/README.md](../dsl/conformance/README.md)(期待値を書き直すのは iOS だけ・Android は合わせる側)。

| 何を比べるか | Android のテスト | コマンド |
|---|---|---|
| 計算式(`docs/dsl/conformance/expr/`) | `:dsl` の `ExprConformanceTest` | `./gradlew :dsl:test --tests jp.kz2601.metamon.dsl.expr.ExprConformanceTest` |
| 画面の木(`docs/dsl/conformance/cases/`) | `:renderer` の `ConformanceTest`(AI チャットは `AiChatConformanceTest`) | `./gradlew :renderer:testDebugUnitTest --tests jp.kz2601.metamon.renderer.ConformanceTest` |

- 読む場所は Gradle が `systemProperty`(`metamon.dslExamplesDir`・`metamon.dslDocsDir`)で渡す。Gradle を通さずに回すと「`metamon.dslDocsDir` が未設定(Gradle から実行する)」で落ちる。IDE から 1 つだけ回す時も Gradle 経由で回す。
- 合わない時の直し方は [conformance/README.md の「Android で合わせる時の注意」](../dsl/conformance/README.md#android-で合わせる時の注意)。

## androidTest(実機・エミュレータ)

```sh
# 実機かエミュレータを 1 台つないでから
./gradlew :app:connectedStgDebugAndroidTest
```

- 端末にアプリとテスト用の APK を入れて回すので、JVM のテストよりずっと重い(ビルド + 端末への install + 画面の操作)。
- テストはつまみ(`METAMON_MOCK` など・[android/README.md の「モックとつまみ」](../../android/README.md#モックとつまみ))を extra で渡して、外の API に繋がずに回す。
- コンパイルだけなら端末は要らない: `./gradlew :app:compileStgDebugAndroidTestKotlin`(CI はここまで)。
- androidTest を足した・直した PR では、手元で回した結果を PR の「動作確認」に書く([ci.md](ci.md#ci-で検証されるもの--手元で確かめるもの))。

## CI で回す物・回さない物

`.github/workflows/android-ci.yml`(ubuntu だけ・エミュレータ無し・#2062)。どの変更でどれが回るかの表は [ci.md の「形」](ci.md#形)。

| | CI | 手元 |
|---|---|---|
| JVM の単体テスト全部(`./gradlew test`) | ○ `android-build`(Android の広い変更・develop / main への push) | 変えたモジュールだけでもよい |
| JVM のテストの一部(`:dsl:test`・`:renderer` の決まったテスト・`:app` の `RuntimeCopyIosEnglishTest`) | ○ `android-light`(DSL・アイコン表・iOS の文言などだけの変更) | — |
| 適合テスト | ○ 上の 2 つに含まれる | — |
| androidTest のコンパイル | ○ `android-build` | — |
| androidTest の実行 | ✗ 回さない(#2062 の 2 段目はまだ) | 実機 / エミュレータで人が回す |

## 決めること

- androidTest を CI で実行するか(エミュレータを入れるか・いつ回すか)。#2062 と [ci.md の「残り」](ci.md#残り)。
- Android の API が要る JVM のテストを増やす時に Robolectric を入れるか(今は入れていない。入れると JVM のテストが重くなる)。
