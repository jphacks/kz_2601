package jp.kz2601.metamon.desktop.update

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.StringTable
import jp.kz2601.metamon.desktop.net.ClientPlatform
import jp.kz2601.metamon.desktop.net.DesktopApiConfig
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException

/** 更新の帯と先へ進めない画面(#2377): 判定の 3 つの結果で出す物・閉じた版・読む間隔・MOCK。 */
class UpdateBanner2377Test {
    private val url = "https://tailor-it.app/download/windows"
    private val config = """{"minWindowsBuild":10,"latestWindowsBuild":20,"windowsDownloadUrl":"$url"}"""

    @Test
    fun viewPerDecision() {
        assertEquals(UpdateView.Hidden, UpdateView.of(UpdateDecision.None, null, url))
        assertEquals(UpdateView.Banner(20, url), UpdateView.of(UpdateDecision.Recommended(20), null, url))
        assertEquals(UpdateView.Blocking(10, null), UpdateView.of(UpdateDecision.Required(10), null, null))
    }

    @Test
    fun dismissedBannerStaysHiddenOnlyForThatVersion() {
        assertEquals(UpdateView.Hidden, UpdateView.of(UpdateDecision.Recommended(20), 20, url))
        assertEquals(UpdateView.Banner(21, url), UpdateView.of(UpdateDecision.Recommended(21), 20, url))
        // 閉じても「更新してください」は出す
        assertEquals(UpdateView.Blocking(10, url), UpdateView.of(UpdateDecision.Required(10), 20, url))
    }

    @Test
    fun downloadUrlFromConfigHttpsOnly() {
        assertEquals(url, UpdateDownload.url(config, ClientPlatform.WINDOWS))
        assertNull(UpdateDownload.url(config, ClientPlatform.LINUX))
        assertNull(UpdateDownload.url(config, ClientPlatform.MACOS))
        assertNull(UpdateDownload.url("""{"linuxDownloadUrl":"http://example.com/x"}""", ClientPlatform.LINUX))
        assertNull(UpdateDownload.url("""{"linuxDownloadUrl":"javascript:alert(1)"}""", ClientPlatform.LINUX))
        assertNull(UpdateDownload.url("""{"linuxDownloadUrl":3}""", ClientPlatform.LINUX))
        assertNull(UpdateDownload.url("not json", ClientPlatform.LINUX))
        assertEquals("https://example.com/l", UpdateDownload.url("""{"linuxDownloadUrl":" https://example.com/l "}""", ClientPlatform.LINUX))
    }

    @Test
    fun refreshDismissAndRemember() = runBlocking {
        val store = MemoryUpdateDismissStore()
        var body = config
        val s = UpdateState({ body }, ClientPlatform.WINDOWS, build = "15", dismissals = store)
        assertEquals(UpdateView.Hidden, s.view)
        assertTrue(s.refresh())
        assertEquals(UpdateView.Banner(20, url), s.view)
        s.dismiss()
        assertEquals(UpdateView.Hidden, s.view)
        assertEquals(20, store.get())
        // 次の起動でも同じ版なら出さない
        val next = UpdateState({ body }, ClientPlatform.WINDOWS, build = "15", dismissals = store)
        next.refresh()
        assertEquals(UpdateView.Hidden, next.view)
        // 新しい版が来たらまた出す
        body = """{"latestWindowsBuild":30}"""
        next.refresh()
        assertEquals(UpdateView.Banner(30, null), next.view)
    }

    @Test
    fun requiredBlocksAndCannotBeDismissed() = runBlocking {
        val s = UpdateState({ config }, ClientPlatform.WINDOWS, build = "9", dismissals = MemoryUpdateDismissStore())
        s.refresh()
        assertEquals(UpdateView.Blocking(10, url), s.view)
        s.dismiss()
        assertEquals(UpdateView.Blocking(10, url), s.view)
    }

    @Test
    fun failedReadKeepsThePreviousResult() = runBlocking {
        var fail = false
        val s = UpdateState({ if (fail) throw IOException("offline") else config }, ClientPlatform.WINDOWS, build = "15")
        s.refresh()
        fail = true
        assertFalse(s.refresh())
        assertEquals(UpdateView.Banner(20, url), s.view)
    }

    @Test
    fun readsAtStartAndEverySixHours() = runBlocking {
        var reads = 0
        val sleeps = mutableListOf<Long>()
        val s = UpdateState({ reads++; "{}" }, ClientPlatform.LINUX, build = "1")
        runCatching {
            s.run { ms ->
                sleeps += ms
                if (sleeps.size == 3) throw StopLoop()
            }
        }
        assertEquals(3, reads)
        assertEquals(List(3) { 6 * 60 * 60 * 1000L }, sleeps)
    }

    @Test
    fun mockNeverReads() = runBlocking {
        val s = UpdateState.create(DesktopApiConfig(mode = DesktopApiConfig.Mode.MOCK), MemoryUpdateDismissStore())
        assertFalse(s.refresh())
        s.run { error("MOCK では待たない") }
        assertEquals(UpdateView.Hidden, s.view)
    }

    @Test
    fun textsInBothLanguages() {
        val table = StringTable.fromClasspath()
        val keys = listOf(
            "update_recommended_title", "update_required_title", "desktop_update_required_body", "desktop_update_open_download",
            "desktop_update_no_download_url", "desktop_close", "desktop_licenses_browse_failed",
        )
        for (lang in Lang.entries) for (key in keys) assertTrue("$lang $key", table.hasString(lang, key))
    }

    private class StopLoop : RuntimeException()
}
