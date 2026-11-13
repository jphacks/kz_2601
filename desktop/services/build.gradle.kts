import org.jetbrains.kotlin.gradle.dsl.JvmTarget

// :services — android/services の純 Kotlin の部分(API クライアント・レコードの保存・送信待ちの列)を同じソースのまま JVM でビルドする(#1748)。
// android/ は変えない。Android の API を使うファイルは含めず、要るファイルだけを名指しで取り込む。
plugins {
    alias(libs.plugins.kotlin.jvm)
    alias(libs.plugins.kotlin.serialization)
}

java {
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
}

kotlin {
    compilerOptions { jvmTarget.set(JvmTarget.JVM_17) }
}

val sharedServices = rootDir.resolve("../android/services/src/main/kotlin")

sourceSets {
    main {
        kotlin {
            srcDir(sharedServices)
            // 共有ソースの型の宣言にだけ出る Android の型(Activity・Context・SharedPreferences)の置き換え。デスクトップでは呼ばない
            srcDir("src/shim/kotlin")
            include(
                "android/app/*.kt",
                "android/content/*.kt",
                "jp/kz2601/metamon/services/AuthService.kt",
                "jp/kz2601/metamon/services/AppConfigClient.kt",
                "jp/kz2601/metamon/services/CrashReporter.kt",
                "jp/kz2601/metamon/services/PlanUsage.kt",
                "jp/kz2601/metamon/services/consent/AiConsentStore.kt",
                "jp/kz2601/metamon/services/consent/CrashReportingPreference.kt",
                "jp/kz2601/metamon/services/consent/KeyValueStore.kt",
                "jp/kz2601/metamon/services/follows/FollowStore.kt",
                "jp/kz2601/metamon/services/follows/FollowsClient.kt",
                "jp/kz2601/metamon/services/http/*.kt",
                "jp/kz2601/metamon/services/live/HttpAppConfigClient.kt",
                "jp/kz2601/metamon/services/live/HttpFollowsClient.kt",
                "jp/kz2601/metamon/services/live/HttpReleaseRequestClient.kt",
                "jp/kz2601/metamon/services/local/DocumentCache.kt",
                "jp/kz2601/metamon/services/local/RecordStore.kt",
                "jp/kz2601/metamon/services/local/SyncQueue.kt",
                "jp/kz2601/metamon/services/release/ReleaseRequests.kt",
                "jp/kz2601/metamon/services/sharing/ShareSheetModel.kt",
            )
        }
    }
}

dependencies {
    api(project(":dsl"))
    api(libs.kotlinx.coroutines.core)
    api(libs.okhttp)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.okhttp.mockwebserver)
}
