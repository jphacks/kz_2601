package jp.kz2601.metamon.account

import jp.kz2601.metamon.feedback.FeedbackComposerLauncher
import jp.kz2601.metamon.feedback.FeedbackOpen
import jp.kz2601.metamon.runtime.RuntimeFixture
import jp.kz2601.metamon.services.feedback.FeedbackContext
import jp.kz2601.metamon.services.feedback.FeedbackSubmission
import jp.kz2601.metamon.services.local.AppListCache
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.DocumentCache
import jp.kz2601.metamon.services.local.RecordStore
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.services.local.SyncQueue
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.fail
import org.junit.Test
import kotlin.coroutines.EmptyCoroutineContext

/** 後片付けで、開いているフィードバックの画面(前の人の画面のスクショ・書きかけ)を閉じる(#1315)。 */
class LocalDataWipeFeedbackTest {
    private val opened = FeedbackOpen(
        FeedbackContext(app = null, device = FeedbackSubmission.Device("1.0", "1", "Android 15", "Pixel", "ja")),
        screenshot = null,
    )

    private fun TestScope.wipe(launcher: FeedbackComposerLauncher, resetRuntime: () -> Unit = {}): LocalDataWipe {
        val store = RecordStore(null, EmptyCoroutineContext)
        val client = RuntimeFixture.offlineClient
        val queue = SyncQueue(null, store, client, BackoffPolicy.immediate, sleep = {}, io = EmptyCoroutineContext)
        val engine = RecordSyncEngine(store, queue, client, RuntimeFixture.Offline(), backgroundScope)
        return LocalDataWipe(
            engine, DocumentCache(null, EmptyCoroutineContext), AppListCache(null),
            resetRuntime = resetRuntime,
            forgetDeviceState = {},
            closeFeedback = launcher::close,
        )
    }

    @Test
    fun wipeClosesTheFeedbackComposer() = runTest {
        val launcher = FeedbackComposerLauncher()
        launcher.show { opened }
        assertNotNull(launcher.open.value)

        wipe(launcher)()

        assertNull(launcher.open.value)
    }

    @Test
    fun composerOpenedDuringTheWipeIsClosedToo() = runTest {
        val launcher = FeedbackComposerLauncher()

        wipe(launcher, resetRuntime = { runBlocking { launcher.show { opened } } })()

        assertNull("消している間に開いたもの(前の人の画面)も残さない", launcher.open.value)
    }

    @Test
    fun failedWipeStillClosesTheComposer() = runTest {
        val launcher = FeedbackComposerLauncher()
        launcher.show { opened }

        try {
            wipe(launcher, resetRuntime = { error("boom") })()
            fail()
        } catch (_: IllegalStateException) {
        }

        assertNull(launcher.open.value)
    }
}
