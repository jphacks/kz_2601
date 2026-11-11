package jp.kz2601.metamon.feedback

import jp.kz2601.metamon.services.feedback.FeedbackContext
import jp.kz2601.metamon.services.feedback.FeedbackSubmission
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

/** フィードバックの画面は後片付け(サインアウト・削除)で閉じ、撮っている途中のものも開かない(#1315)。 */
class FeedbackComposerLauncherTest {
    private val opened = FeedbackOpen(
        FeedbackContext(app = null, device = FeedbackSubmission.Device("1.0", "1", "Android 15", "Pixel", "ja")),
        screenshot = null,
    )

    @Test
    fun closeDropsTheOpenComposer() = runTest {
        val launcher = FeedbackComposerLauncher()
        launcher.show { opened }
        assertNotNull(launcher.open.value)

        launcher.close()

        assertNull(launcher.open.value)
        assertFalse(launcher.isOpen)
    }

    @Test
    fun closingWhileCapturingDoesNotOpenTheCapturedScreen() = runTest {
        val launcher = FeedbackComposerLauncher()
        val shot = CompletableDeferred<FeedbackOpen>()
        launch { launcher.show { shot.await() } }
        runCurrent()

        launcher.close() // 撮っている間にサインアウトの後片付け
        shot.complete(opened)
        runCurrent()

        assertNull("前の人の画面を撮ったものを次の人に開かない", launcher.open.value)
        assertFalse(launcher.isOpen)

        launcher.show { opened } // 次はふつうに開ける
        assertNotNull(launcher.open.value)
    }
}
