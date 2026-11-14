package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** iOS の OpenUrlActionTests: `openUrl` も openUrlButton と同じ許可リストを通す。外に渡す処理は差し替えて確かめる。 */
class OpenUrlActionTest {
    private class RecordingHost : RuntimeHost {
        val opened = mutableListOf<String>()
        override suspend fun openUrl(url: String): Boolean {
            opened += url
            return true
        }
    }

    private fun openUrl(url: String?) = Action(type = ActionType.OPEN_URL, url = url)

    private val ctx = ActionContext(Route("home", stack = Route.tabStack("home")))

    @Test
    fun allowedUrlsAreOpened() = runTest {
        for (s in listOf("https://example.com/a?b=1", "line://nv/chat", "tel:0761234567", "mailto:a@b.c", "HTTPS://EXAMPLE.COM")) {
            val host = RecordingHost()
            val rt = runtime(RuntimeFixture.dsl("warikan"), host = host)
            rt.run(openUrl(s), ctx)
            assertEquals(s, listOf(s), host.opened)
            assertNull(s, rt.toast)
        }
    }

    @Test
    fun disallowedUrlsAreNotOpened() = runTest {
        for (s in listOf("http://example.com", "ftp://example.com", "javascript:alert(1)", "file:///tmp/x", "telegram://x", "tailorit://import/x", "", "not a url", "%%%")) {
            val host = RecordingHost()
            val rt = runtime(RuntimeFixture.dsl("warikan"), host = host)
            rt.run(openUrl(s), ctx)
            assertTrue(s, host.opened.isEmpty())
            assertNotNull("拒んだことを知らせる: $s", rt.toast)
        }
    }

    @Test
    fun missingUrlIsNotOpened() = runTest {
        val host = RecordingHost()
        val rt = runtime(RuntimeFixture.dsl("warikan"), host = host)
        rt.run(openUrl(null), ctx)
        assertTrue(host.opened.isEmpty())
    }
}
