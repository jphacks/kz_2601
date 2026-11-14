package jp.kz2601.metamon.account

import jp.kz2601.metamon.runtime.RuntimeFixture
import jp.kz2601.metamon.services.local.AppListCache
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.DocumentCache
import jp.kz2601.metamon.services.local.RecordStore
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.services.local.SyncQueue
import jp.kz2601.metamon.switcher.AppThumbnailStore
import jp.kz2601.metamon.voice.ChatHistoryStore
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import kotlin.coroutines.EmptyCoroutineContext

/** 位置の記録(#869)はサインアウト・アカウント削除の後片付けと、記録中のアプリの削除で止める。 */
class LocationTrackingCleanupTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private fun TestScope.engine(): RecordSyncEngine {
        val store = RecordStore(null, EmptyCoroutineContext)
        val client = RuntimeFixture.offlineClient
        val queue = SyncQueue(null, store, client, BackoffPolicy.immediate, sleep = {}, io = EmptyCoroutineContext)
        return RecordSyncEngine(store, queue, client, RuntimeFixture.Offline(), backgroundScope)
    }

    @Test
    fun wipeForSignOutOrAccountDeletionStopsTracking() = runTest {
        var stops = 0
        LocalDataWipe(
            engine(), DocumentCache(null, EmptyCoroutineContext), AppListCache(null),
            resetRuntime = {},
            forgetDeviceState = {},
            stopLocationTracking = { stops++ },
        )()
        assertEquals(1, stops)
    }

    @Test
    fun deletingAnAppStopsTrackingForThatApp() = runTest {
        val stopped = mutableListOf<String>()
        val forgotten = mutableListOf<String>()
        LocalAppForget(
            engine(), DocumentCache(null, EmptyCoroutineContext), AppThumbnailStore(null), ChatHistoryStore(tmp.newFolder("chat")),
            forgetNotifications = { forgotten += it },
            stopLocationTracking = { stopped += it },
        )("app-1")
        assertEquals(listOf("app-1"), stopped)
        assertEquals(listOf("app-1"), forgotten)
    }
}
