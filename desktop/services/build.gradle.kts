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
                // 会話(/v1/chat)・作る(/v1/generate)・aiButton(/v1/text)のクライアント(#2160 の取り込みの部分・#2380)
                "jp/kz2601/metamon/services/AIClient.kt",
                "jp/kz2601/metamon/services/AuthService.kt",
                "jp/kz2601/metamon/services/AppConfigClient.kt",
                "jp/kz2601/metamon/services/CrashReporter.kt",
                "jp/kz2601/metamon/services/PlanUsage.kt",
                "jp/kz2601/metamon/services/consent/AiConsentStore.kt",
                // 同意が無ければ ai へ送らない包み(#2160)
                "jp/kz2601/metamon/services/consent/ConsentGatedAiClient.kt",
                "jp/kz2601/metamon/services/consent/CrashReportingPreference.kt",
                "jp/kz2601/metamon/services/consent/KeyValueStore.kt",
                "jp/kz2601/metamon/services/http/*.kt",
                "jp/kz2601/metamon/services/live/AiTextCall.kt",
                "jp/kz2601/metamon/services/live/GenerateStreamEvent.kt",
                "jp/kz2601/metamon/services/live/HttpAIClient.kt",
                "jp/kz2601/metamon/services/live/HttpAppConfigClient.kt",
                "jp/kz2601/metamon/services/local/DocumentCache.kt",
                "jp/kz2601/metamon/services/local/RecordStore.kt",
                "jp/kz2601/metamon/services/local/SyncQueue.kt",
            )
        }
    }
}

dependencies {
    api(project(":dsl"))
    api(libs.kotlinx.coroutines.core)
    api(libs.okhttp)
}
