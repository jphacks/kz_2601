# デスクトップ版の作り

デスクトップ版(`desktop/`・Windows・Linux)が Android のソースをどう取り込んでいるかと、Android 側を直した時にデスクトップのどのテストが回るか。動かし方と「今できること」は [`desktop/README.md`](../../desktop/README.md)。

内容は `origin/develop`(748767fd6)の `desktop/settings.gradle.kts`・`desktop/build.gradle.kts`・`desktop/app/build.gradle.kts`・`desktop/services/build.gradle.kts`・`.github/workflows/desktop-ci.yml` から書いた。

## 目次

- [全体](#全体)
- [取り込んでいる Android のソース](#取り込んでいる-android-のソース)
- [取り込みの決まり](#取り込みの決まり)
- [置き換え層(shim)](#置き換え層shim)
- [文言・アイコン・DSL の例](#文言アイコンdsl-の例)
- [適合テストと許可リスト](#適合テストと許可リスト)
- [CI が回る条件](#ci-が回る条件)
- [早見表: Android のこれを直したら](#早見表-android-のこれを直したら)

## 全体

Gradle のビルドは Android とは別(`desktop/settings.gradle.kts`)。Android SDK 無しでビルドできるようにするため。

| モジュール | 場所 | 中身 |
|---|---|---|
| `:dsl` | `android/dsl`(同じソースをそのまま) | DSL の型・読み込み・式・記録の規則。出力先だけ `desktop/build/dsl` に分ける(`desktop/build.gradle.kts`。Android のビルドと同時に回しても `android/dsl/build` を取り合わない) |
| `:services` | `desktop/services` | `android/services` の純 Kotlin の部分を名指しで取り込む(下の表) |
| `:desktop` | `desktop/app` | ウィンドウ・画面の描画・OS に頼む処理。`:dsl` と `:services` を使う |

ライブラリの版は Android と同じ `android/gradle/libs.versions.toml` を読み、デスクトップだけの物(Compose Multiplatform の版)は `desktop/settings.gradle.kts` で足す。JDK は 17。

## 取り込んでいる Android のソース

名指しの一覧の場所は 2 つ。ファイルを足す・消す時はここを直す。

| 取り込む物 | 一覧の場所 | ファイル |
|---|---|---|
| `android/services/src/main/kotlin` | `desktop/services/build.gradle.kts` の `include(...)` | `AuthService.kt`・`AppConfigClient.kt`・`CrashReporter.kt`・`PlanUsage.kt`・`consent/AiConsentStore.kt`・`consent/CrashReportingPreference.kt`・`consent/KeyValueStore.kt`・`http/*.kt`・`live/HttpAppConfigClient.kt`・`local/DocumentCache.kt`・`local/RecordStore.kt`・`local/SyncQueue.kt`・`update/ClientConfig.kt`・`update/ServiceNotice.kt`(どれも `jp/kz2601/metamon/services/` の下) |
| `android/renderer/src/main/kotlin/jp/kz2601/metamon/renderer` | `desktop/app/build.gradle.kts` の `syncSharedRenderer` | `form/FormDraft.kt`・`navigation/NavigationResolver.kt`・`theme/icons/SymbolIconTable.kt`・`theme/icons/VendoredIcons.kt` |
| `android/dsl` | `desktop/settings.gradle.kts`(`:dsl` の `projectDir`) | モジュールの全部 |

`syncSharedRenderer` はビルドの時に `desktop/app/build/generated/sharedRenderer` へ写してからコンパイルする。

## 取り込みの決まり

- `android/` のファイルは変えない。デスクトップに合わせたい時は、デスクトップ側で包むか上書きする(例: `X-Client-Platform` は共有の `ClientInfo` が付ける `android` を、`desktop/app/.../net/ClientPlatform.kt` の OkHttp の段で `windows`・`linux`・`macos` に上書きする)。
- 取り込むファイルは Android の API を呼ばない物だけ。`android.*` の import は、下の shim にある型(引数・戻り値にだけ出る物)に限る。例: `AuthService.kt` は `android.app.Activity` を import するが、使うのは `signInWithApple(activity: Activity)` の引数だけ。
- shim に無い Android の型を使うファイルを `include` に足すと、`:services`・`:desktop` のコンパイルが落ちる。

## 置き換え層(shim)

`desktop/services/src/shim/kotlin` に、共有のソースの引数・戻り値にだけ出てくる Android の型を同じ名前で置く。デスクトップからは呼ばない(呼ぶと止まる物もある)。

| ファイル | 置き換える型 | どこに出てくるか |
|---|---|---|
| `android/app/Activity.kt` | `android.app.Activity`(中身の無いクラス) | `AuthService` の引数(Apple のサインイン) |
| `android/content/Context.kt` | `android.content.Context`(呼ぶと `error`) | `SharedPreferencesKeyValueStore.of` の引数 |
| `android/content/SharedPreferences.kt` | `android.content.SharedPreferences`(interface だけ) | 同上。デスクトップの保存は `desktop/app/.../store/JsonKeyValueStore` |

共有のソースが新しい Android の型を使い始めたら、ここに足すか、そのファイルを取り込みから外す。

## 文言・アイコン・DSL の例

ビルドの時に、次の物を `desktop/app` のリソースに写す(`desktop/app/build.gradle.kts`)。

| タスク | 元 | jar の中 |
|---|---|---|
| `syncAndroidStrings` | `android/renderer/src/main/res`・`android/app/src/main/res` の `values/strings*.xml`(日本語)・`values-en/strings*.xml`(英語) | `i18n/renderer/...`・`i18n/app/...` と一覧の `i18n/index.txt` |
| `syncIconMap` | `docs/dsl/icon-map.json` | `icons/icon-map.json` |
| `processResources` | `docs/dsl/examples/*.json` | `examples/<名前>.json` |

デスクトップだけの文言は `desktop/app/src/main/resources/i18n/desktop` の同じ形の XML に置き、キーは `desktop_` で始める。

## 適合テストと許可リスト

- `./gradlew :desktop:test :dsl:test`(`desktop/` で)。`:dsl:test` は `android/dsl` のテストをデスクトップのビルドで回す。
- `desktop/app/src/test/.../DesktopConformanceTest.kt` は全 OS 共通の期待値 `docs/dsl/conformance/cases` と `docs/dsl/examples` を読む(テストの system property `metamon.dslDocsDir`)。
- 違ってよい所(描けない type・まだ比べない props)は `desktop/app/src/test/.../ConformanceAllowlist.kt` に理由付きで書く。そこに無い違いはテストが落ちる。
- テストの言語とテーマは `tailorit.lang=ja`・`tailorit.theme=light` に固定する(テストを回す機械の設定に左右されない)。

## CI が回る条件

`.github/workflows/desktop-ci.yml`。PR(Draft でない)・`develop` と `main` への push・手動で動き、`dorny/paths-filter` で回すかを決める。

| 変わった場所 | テスト(ubuntu・windows の `:desktop:test :dsl:test`) | 配布物(deb・msi) |
|---|---|---|
| `desktop/**`(`*.md` 以外) | 回る | push・手動の時だけ |
| `android/dsl/**`(`*.md` 以外)・`android/gradle/libs.versions.toml` | 回る | 同上 |
| `android/renderer/src/main/**`・`android/services/src/main/**`(`*.md` 以外) | 回る | 同上 |
| `android/app/src/main/res/values*/**` | 回る | 同上 |
| `docs/dsl/conformance/**`・`docs/dsl/examples/**` | 回る | 同上 |
| `.github/workflows/desktop-ci.yml` | 回る | 同上 |
| 上のどれでもない | 回らない(`desktop-ci` は成功) | 作らない |
| 手動(`workflow_dispatch`) | 必ず回る | 作る |

`android/renderer`・`android/services` は取り込んでいないファイルを直しても回る(フィルタはディレクトリ単位)。

## 早見表: Android のこれを直したら

| Android で直した物 | デスクトップへの影響 | 手元で回すもの |
|---|---|---|
| `android/dsl` の何か | そのまま使っている。型・式・記録の規則が変わる | `cd desktop && ./gradlew :desktop:test :dsl:test` |
| `android/services` の上の表のファイル | API クライアント・保存・送信待ちが変わる。Android の API を使い始めるとデスクトップのコンパイルが落ちる | 同上 |
| `android/services` のそれ以外 | 影響しない(CI は回る) | 要らない |
| `android/renderer` の `FormDraft.kt`・`NavigationResolver.kt`・`theme/icons/` の 2 つ | 下書き・遷移・アイコンの表が変わる | 同上 |
| `android/renderer` のそれ以外 | 影響しない(CI は回る) | 要らない |
| `android/renderer`・`android/app` の `strings*.xml` | 同じキーで引く文言が変わる。キーの名前を変える・消す時は、デスクトップでそのキーを引いている所も直す | 同上 |
| `android/gradle/libs.versions.toml` | Kotlin・ライブラリの版が変わる | 同上 |
| `docs/dsl/conformance`・`docs/dsl/examples` | 適合テストの期待値・同梱の例が変わる | 同上 |
| `android/app` のコード | 影響しない | 要らない |
