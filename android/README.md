# Android アプリ(Tailor It)

Kotlin + Jetpack Compose のネイティブアプリ。技術の選び方は [`docs/decisions/354-android-tech.md`](../docs/decisions/354-android-tech.md)、DSL の扱いは [`docs/decisions/351-dsl-cross-platform.md`](../docs/decisions/351-dsl-cross-platform.md)。今は骨格(#356)だけで、話しかける前の画面(`docs/dsl/examples/initial.json`)が出る。

## 目次

1. [モジュール](#モジュール)
2. [要るもの](#要るもの)
3. [ビルドとテスト](#ビルドとテスト)
4. [stg と prod](#stg-と-prod)
5. [Firebase の設定](#firebase-の設定)
6. [版を上げる](#版を上げる)

## モジュール

iOS の `Metamon{DSL,Renderer,Services}` + アプリ本体と 1 対 1。依存の向きは Gradle で固定してある。

| モジュール | 種類 | iOS の対応 | 中身 |
|---|---|---|---|
| `:dsl` | 純 Kotlin(JVM) | `MetamonDSL` | DSL の型と読み込み(kotlinx.serialization)。知らない type・キー・値で落とさない |
| `:renderer` | Android ライブラリ + Compose | `MetamonRenderer` | DSL を描く。`:services` は知らず、能力は `DSLRuntime` で受ける |
| `:services` | Android ライブラリ | `MetamonServices` | 認証(Firebase / モック)・接続先・HTTP(OkHttp + kotlinx.serialization)・appconfig / ai のクライアント |
| `:app` | Android アプリ | `ios/Metamon` | 起動・組み立て(`AppContainer`)・flavor |

`:dsl` の型(`AppDSL`・`Screen`・`Component`・`Action`・`Expr`・enum・部品ごとのキー表 `DslKeys`)は `dsl/src/main/kotlin/.../dsl/generated/` にあり、スキーマ(`backend/pkg/dsl/schema.json`)から `backend/cmd/dslgen` が作る(#351 §7)。**手で直さない**。スキーマを変えたら作り直してコミットする。

```sh
cd backend && make dsl-gen     # 生成物が古いと backend の go test(TestGeneratedKotlinIsUpToDate)が落ちる
```

読み方は iOS と同じく寛容: 知らない type は `ComponentType.UNKNOWN`・知らない enum の値は `UNKNOWN`(`VoiceEntry` は `FLOATING`)・省略できるキーの型違いは null・配列の壊れた要素は捨てる。必須のキー(`id` など)が無い・型違いの要素は、その要素だけ配列から落ちる。手書きは `Lenient.kt`(寛容な読み方)と `AppDSL.kt`(封筒・読み込み・最後の手段)だけ。

## 要るもの

- JDK 17 以上(手元は Homebrew の `openjdk`(26)で動く。CI は 17)
- Android SDK(platform 36)。`android/local.properties` に `sdk.dir=$HOME/Library/Android/sdk` を書くか、`ANDROID_HOME` を設定する
- Gradle は wrapper(`./gradlew`)が取ってくる。版は `gradle/wrapper/gradle-wrapper.properties`

## ビルドとテスト

```sh
cd android
./gradlew test                 # JVM の単体テスト(全モジュール・stg / prod の両方)
./gradlew :dsl:test            # DSL だけ(docs/dsl/examples の全例を読む)
./gradlew assembleStgDebug     # stg の debug APK -> app/build/outputs/apk/stg/debug/
./gradlew assembleProdDebug    # prod の debug APK
./gradlew installStgDebug      # つないだ端末・エミュレータに入れる
```

手元で他のビルド(iOS など)と同時に走らせる時は、ロックで 1 本ずつにする。

```sh
/usr/bin/lockf -k ~/Desktop/JPHACKS2026/.locks/gradle.lock ./gradlew test assembleStgDebug
```

Gradle のメモリ(2g)とワーカー数(4)は `gradle.properties` で抑えてある。CI の形は [`docs/android/ci.md`](../docs/android/ci.md)。

## stg と prod

product flavor `env` で分ける。値は `BuildConfig` に入る(iOS の `ios/project.yml` と同じ接続先)。

| flavor | applicationId | アプリ名 | `AI_BASE_URL` / `APPCONFIG_BASE_URL` |
|---|---|---|---|
| `stg` | `jp.kz2601.metamon.stg` | Tailor It β | `ai-stg-…` / `appconfig-stg-…` |
| `prod` | `jp.kz2601.metamon` | Tailor It | `ai-prod-…` / `appconfig-prod-…` |

stg の画面の下には接続先と認証の状態(`stg / auth=mock / uid=…`)が出る。

## Firebase の設定

`google-services.json` は**コミットしない**(`.gitignore` 済み)。無くてもビルドは通り(警告だけ)、起動すると認証はモック(固定 uid・トークン無し)で動く。iOS で plist が無い時と同じ。

本物で動かす時は、リポ外の secrets フォルダから flavor ごとの置き場へコピーする。

```sh
android/scripts/sync-google-services.sh
#   $METAMON_SECRETS_DIR(既定 ~/Desktop/JPHACKS2026/secrets)の
#   google-services.json     -> app/src/prod/google-services.json(必須)
#   google-services-stg.json -> app/src/stg/google-services.json(任意)
```

Firebase Console に Android アプリ(`jp.kz2601.metamon`・stg を同じプロジェクトにするなら `jp.kz2601.metamon.stg` も)を登録して json を落とすのは人間の作業。

json がある時だけ `google-services` と Crashlytics の Gradle プラグインが当たる(`app/build.gradle.kts`)。json が無いビルドは Firebase を初期化せず、認証は固定 uid のモック・App Check のヘッダ無し・クラッシュ報告無しで動く。

### サインイン(#840)

未サインインならサインイン画面(`app/.../auth/SignInScreen.kt`)を出す。方法は 3 つ。

| 方法 | しくみ | 要る設定 |
|---|---|---|
| Google | Credential Manager で Google の ID トークンを取り、Firebase に渡す | Firebase で Google を有効化・アプリの SHA-1 を登録。Web クライアント ID は json の `default_web_client_id`(gradle の `metamon.<flavor>.googleWebClientId` で上書きできる)。無ければボタンを出さない |
| Apple | Firebase の `OAuthProvider("apple.com")`(Custom Tab) | Firebase で Apple を有効化・Apple の Services ID とリダイレクト URL |
| 匿名 | Firebase の匿名認証 | Firebase で匿名を有効化 |

backend への `Authorization: Bearer` は Firebase の ID トークン(`AuthTokenProvider`)。`AppContainer.make(context, mockAuth = true)` で Firebase を使わずモックに差し替えられる。

### App Check と Crashlytics

- App Check: release は Play Integrity、debug ビルドは debug provider(`app/src/{debug,release}/.../AppCheckProviders.kt`)。debug のトークンは初回起動時に logcat に出るので Firebase Console に登録する。取れない時はヘッダ無しで送る。
- Crashlytics: 送るのは SDK が集めるクラッシュの情報と uid だけ。`CrashReporter` に `log`・カスタムキーの口は無い(話した内容・記録・DSL を送らない)。オン / オフの設定は android-053。

## 版を上げる

版は `gradle/libs.versions.toml` だけで決める。今は compileSdk 36 に合わせて、compileSdk 37 を求める版(Compose 1.12・core 1.19・lifecycle 2.11)を使っていない。SDK の platform 37 を入れたら、compileSdk と一緒に上げる。
