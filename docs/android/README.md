# Android

Android アプリ(#354・#356〜#360)の文書の入口。

| 文書 | 中身 |
|---|---|
| [`android/README.md`](../../android/README.md) | モジュール・ビルドとテストのコマンド・stg / prod・Firebase の設定 |
| [`ci.md`](ci.md) | CI の案(ワークフローの YAML。まだ置いていない) |
| [`play-billing.md`](play-billing.md) | Tailor It Pro の購入(Google Play Billing)と、backend・Play Console で残っていること |
| [`play-background-location.md`](play-background-location.md) | Play Console の裏での位置情報の申告の文(場所に着いたら知らせる) |
| [`../decisions/354-android-tech.md`](../decisions/354-android-tech.md) | 技術の選び方(UI・音声・API レベル・モジュール・DSL の型) |
| [`../decisions/351-dsl-cross-platform.md`](../decisions/351-dsl-cross-platform.md) | DSL を iOS 以外でも描く方針 |

## 道具の版(2026-11-11 時点)

| 道具 | 版 | 置き場 |
|---|---|---|
| Gradle | 9.8.2 | `android/gradle/wrapper/gradle-wrapper.properties` |
| Android Gradle Plugin | 9.5.1(Kotlin は AGP の組み込み) | `android/gradle/libs.versions.toml` |
| Kotlin | 2.4.21 | 同上 |
| Compose BOM | 2026.06.01(Compose 1.11.4) | 同上 |
| compileSdk / targetSdk / minSdk | 36 / 36 / 31 | 各モジュールの `build.gradle.kts` |
| JDK | 17 以上(手元は 26 で確認・CI は 17) | — |

Compose 1.12(BOM 2026.08.00 以降)・androidx.core 1.19・lifecycle 2.11 は compileSdk 37 を求めるので、platform 37 を入れて compileSdk を上げる時に一緒に上げる。

## よく使うコマンド

```sh
cd android
/usr/bin/lockf -k ~/Desktop/JPHACKS2026/.locks/gradle.lock ./gradlew test assembleStgDebug
```

詳しくは [`android/README.md`](../../android/README.md#ビルドとテスト)。
