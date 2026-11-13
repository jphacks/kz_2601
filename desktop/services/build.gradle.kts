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
                // 自動の動きの一覧と「直しますか?」の規則(#3356)
                "jp/kz2601/metamon/services/automations/Automations.kt",
                "jp/kz2601/metamon/services/consent/AiConsentStore.kt",
                "jp/kz2601/metamon/services/consent/CrashReportingPreference.kt",
                "jp/kz2601/metamon/services/consent/KeyValueStore.kt",
                // 外の公開データ(feed)の読み込みと前回の写し(#2806)
                "jp/kz2601/metamon/services/feeds/FeedCache.kt",
                "jp/kz2601/metamon/services/feeds/FeedClient.kt",
                "jp/kz2601/metamon/services/follows/FollowStore.kt",
                "jp/kz2601/metamon/services/follows/FollowsClient.kt",
                "jp/kz2601/metamon/services/http/*.kt",
                "jp/kz2601/metamon/services/live/AiTextCall.kt",
                "jp/kz2601/metamon/services/live/GenerateStreamEvent.kt",
                "jp/kz2601/metamon/services/live/HttpAIClient.kt",
                "jp/kz2601/metamon/services/live/HttpAppConfigClient.kt",
                "jp/kz2601/metamon/services/live/HttpAutomationsClient.kt",
                "jp/kz2601/metamon/services/live/HttpFeedClient.kt",
                "jp/kz2601/metamon/services/live/HttpFollowsClient.kt",
                "jp/kz2601/metamon/services/live/HttpReleaseRequestClient.kt",
                "jp/kz2601/metamon/services/local/DocumentCache.kt",
                "jp/kz2601/metamon/services/local/RecordStore.kt",
                "jp/kz2601/metamon/services/local/SyncQueue.kt",
                "jp/kz2601/metamon/services/privacy/PublicPersonalData.kt",
                "jp/kz2601/metamon/services/release/ReleaseRequests.kt",
                "jp/kz2601/metamon/services/sharing/ShareSheetModel.kt",
                // client-config の notice(#333)。ServiceNotice の読み方の関数は ClientConfig.kt にある
                "jp/kz2601/metamon/services/update/ClientConfig.kt",
                "jp/kz2601/metamon/services/update/ServiceNotice.kt",
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
