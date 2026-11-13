package jp.kz2601.metamon.desktop.net

import jp.kz2601.metamon.desktop.store.DesktopStore
import kotlinx.coroutines.runBlocking
import kotlin.system.exitProcess

/**
 * 手元の確かめ用: stg の appconfig からアプリの一覧を出す(#1748)。
 * `TAILORIT_API=stg TAILORIT_DEV_TOKEN=<ID トークン> ./gradlew :desktop:listApps`
 */
fun main() = runBlocking {
    val config = DesktopApiConfig.fromEnvironment()
    val store = DesktopStore.open()
    val client = DesktopApi.appConfig(config, store.documents)
    if (client == null) {
        System.err.println("TAILORIT_API=stg を付けて実行してください($config)")
        exitProcess(2)
    }
    println("$config platform=${ClientPlatform.current().header}")
    // 並びを合わせ、写しを DocumentCache に置く(#2378)。圏外・5xx なら前回の写し
    when (val r = ServerApps(client, store.documents) { true }.load()) {
        is ServerAppsResult.Loaded -> {
            if (r.fromCache) println("(cached copy)")
            r.apps.forEach { println("${it.id}\t${it.name}\tv${it.currentVersion}") }
        }
        is ServerAppsResult.Failed -> System.err.println("読めませんでした: ${r.error::class.simpleName}")
        ServerAppsResult.SignedOut -> Unit
    }
}
