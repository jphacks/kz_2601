// Windows・Linux(と macOS)のデスクトップ版。Compose Multiplatform Desktop(JVM)。
// Android SDK なしでビルドできるよう Android の Gradle ビルドとは分け、純 Kotlin の :dsl だけを同じソースのまま取り込む。
pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
    // 版は Android と同じ表を使う(Kotlin・serialization をずらさない)。デスクトップだけの物はここで足す。
    versionCatalogs {
        create("libs") {
            from(files("../android/gradle/libs.versions.toml"))
            version("composeMultiplatform", "1.12.2")
            plugin("compose-multiplatform", "org.jetbrains.compose").versionRef("composeMultiplatform")
        }
    }
}

rootProject.name = "tailorit-desktop"

include(":dsl")
project(":dsl").projectDir = file("../android/dsl")

// android/services の純 Kotlin の部分(API クライアント・保存・送信待ち)。取り込むファイルは desktop/services で名指しする
include(":services")
project(":services").projectDir = file("services")

include(":desktop")
project(":desktop").projectDir = file("app")
