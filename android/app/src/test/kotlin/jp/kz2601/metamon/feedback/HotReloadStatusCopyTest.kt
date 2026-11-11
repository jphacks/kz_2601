package jp.kz2601.metamon.feedback

import jp.kz2601.metamon.R
import jp.kz2601.metamon.services.sync.HotReloadStatus
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** 下部の「リアルタイム / ポーリング中」の文言(#1108)。 */
class HotReloadStatusCopyTest {
    @Test
    fun offIsHidden() = assertNull(HotReloadStatusCopy.of(HotReloadStatus.OFF))

    @Test
    fun realtimeAndPollingHaveLabelAndDescription() {
        assertEquals(
            HotReloadStatusCopy.Copy(R.string.hot_reload_realtime, R.string.hot_reload_realtime_a11y, live = true),
            HotReloadStatusCopy.of(HotReloadStatus.REALTIME),
        )
        assertEquals(
            HotReloadStatusCopy.Copy(R.string.hot_reload_polling, R.string.hot_reload_polling_a11y, live = false),
            HotReloadStatusCopy.of(HotReloadStatus.POLLING),
        )
    }
}
