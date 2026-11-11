package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.ConnectivityMonitor
import jp.kz2601.metamon.services.local.RecordStore
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.services.local.SyncQueue
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.test.TestScope
import java.io.File
import java.io.IOException
import java.lang.reflect.Proxy
import kotlin.coroutines.EmptyCoroutineContext

/** iOS の `AppRuntime(document:services: .mock)` に当たる組み立て。サーバーには届かない(圏外)。 */
object RuntimeFixture {
    private val examplesDir = File(System.getProperty("metamon.examplesDir") ?: "../../docs/dsl/examples")

    fun dsl(name: String): AppDSL = AppDSL.decode(File(examplesDir, "$name.json").readText())

    /** どの呼び出しも通信断で失敗する appconfig。 */
    val offlineClient: AppConfigClient = Proxy.newProxyInstance(
        AppConfigClient::class.java.classLoader,
        arrayOf(AppConfigClient::class.java),
    ) { _, method, _ ->
        if (method.name == "toString") "offlineClient" else throw IOException("offline")
    } as AppConfigClient

    class Offline : ConnectivityMonitor {
        override val online: StateFlow<Boolean> = MutableStateFlow(false)
    }

    fun TestScope.runtime(
        dsl: AppDSL,
        appId: String = "test-app",
        host: RuntimeHost = UnavailableHost,
        uid: String? = null,
    ): AppRuntime {
        val store = RecordStore(null, EmptyCoroutineContext)
        val client = offlineClient
        val queue = SyncQueue(null, store, client, BackoffPolicy.immediate, sleep = {}, io = EmptyCoroutineContext)
        val engine = RecordSyncEngine(store, queue, client, Offline(), backgroundScope)
        return AppRuntime(AppDocument(appId, 1, dsl), engine, backgroundScope, host, currentUserId = { uid })
    }
}
