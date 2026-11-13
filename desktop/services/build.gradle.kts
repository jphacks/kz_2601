import org.jetbrains.kotlin.gradle.dsl.JvmTarget
import java.nio.file.FileSystems
import java.nio.file.Paths

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
// 共有ソースの型の宣言にだけ出る Android の型(Activity・Context・SharedPreferences)の置き換え。デスクトップでは呼ばない
val shimSources = file("src/shim/kotlin")

// 取り込むファイル。名前が変わると include は黙って何も拾わないので、checkSharedIncludes で 1 つ以上当たるかを見る(#2815)
val sharedIncludes = listOf(
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
    "jp/kz2601/metamon/services/consent/AiButtonMetrics.kt",
    "jp/kz2601/metamon/services/consent/AiConsentStore.kt",
    // 同意が無ければ ai へ送らない包み(#2160)
    "jp/kz2601/metamon/services/consent/ConsentGatedAiClient.kt",
    "jp/kz2601/metamon/services/consent/CrashReportingPreference.kt",
    "jp/kz2601/metamon/services/consent/KeyValueStore.kt",
    "jp/kz2601/metamon/services/discover/DiscoverClient.kt",
    "jp/kz2601/metamon/services/discover/DiscoverModel.kt",
    "jp/kz2601/metamon/services/discover/PublishModel.kt",
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
    "jp/kz2601/metamon/services/live/HttpDiscoverClient.kt",
    "jp/kz2601/metamon/services/live/HttpFeedClient.kt",
    "jp/kz2601/metamon/services/live/HttpFollowsClient.kt",
    "jp/kz2601/metamon/services/live/HttpReleaseRequestClient.kt",
    // 通報・ブロック(と仲間探し)の API クライアント(#2805)
    "jp/kz2601/metamon/services/live/HttpSocialClients.kt",
    "jp/kz2601/metamon/services/local/DocumentCache.kt",
    "jp/kz2601/metamon/services/local/RecordStore.kt",
    "jp/kz2601/metamon/services/local/SyncQueue.kt",
    "jp/kz2601/metamon/services/privacy/PublicPersonalData.kt",
    "jp/kz2601/metamon/services/release/ReleaseRequests.kt",
    "jp/kz2601/metamon/services/sharing/ShareSheetModel.kt",
    // DiscoverModel・PublishModel が使う通報・ブロックの型と作者名の保存(純 Kotlin)
    "jp/kz2601/metamon/services/social/AuthorProfile.kt",
    "jp/kz2601/metamon/services/social/MatchingClient.kt",
    "jp/kz2601/metamon/services/social/ModerationClient.kt",
    // 変化の通知(SSE `GET /v1/apps/{appId}/events`)とホットリロード(#2807)
    "jp/kz2601/metamon/services/sync/ChangeEvents.kt",
    "jp/kz2601/metamon/services/sync/DocumentSync.kt",
    "jp/kz2601/metamon/services/sync/HotReload.kt",
    "jp/kz2601/metamon/services/sync/RealtimeDocumentSync.kt",
    // client-config の notice(#333)。ServiceNotice の読み方の関数は ClientConfig.kt にある
    "jp/kz2601/metamon/services/update/ClientConfig.kt",
    "jp/kz2601/metamon/services/update/ServiceNotice.kt",
)

sourceSets {
    main {
        kotlin {
            srcDir(sharedServices)
            srcDir(shimSources)
            include(sharedIncludes)
        }
    }
}

/** include の 1 行ごとに、取り込み元(android/services か shim)のファイルに 1 つ以上当たるか。当たらない行を出して落とす */
val checkSharedIncludes = tasks.register("checkSharedIncludes") {
    group = "verification"
    description = "desktop/services の include が android/services のファイルに当たるかを確かめる"
    val roots = listOf(sharedServices, shimSources)
    val patterns = sharedIncludes
    inputs.files(roots)
    inputs.property("patterns", patterns)
    doLast {
        val files = roots.filter { it.isDirectory }.flatMap { root ->
            root.walkTopDown().filter { it.isFile }.map { it.relativeTo(root).invariantSeparatorsPath }.toList()
        }
        val missing = patterns.filter { p ->
            val glob = FileSystems.getDefault().getPathMatcher("glob:$p")
            files.none { glob.matches(Paths.get(it)) }
        }
        if (missing.isNotEmpty()) {
            throw GradleException(
                "desktop/services/build.gradle.kts の include が何のファイルにも当たりません(android/services で名前が変わった?):\n" +
                    missing.joinToString("\n") { "  - $it" },
            )
        }
    }
}

tasks.named("check") { dependsOn(checkSharedIncludes) }

dependencies {
    api(project(":dsl"))
    api(libs.kotlinx.coroutines.core)
    api(libs.okhttp)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.okhttp.mockwebserver)
}
