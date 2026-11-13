package jp.kz2601.metamon.desktop.marks

import jp.kz2601.metamon.desktop.net.ClientPlatform
import jp.kz2601.metamon.desktop.net.DesktopApi
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Mark
import jp.kz2601.metamon.dsl.records.MarkState
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.services.live.HttpAppConfigClient
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.yield
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException

/** 共有のアプリの印の読み取りと、押した時の増減の規則(#2810)。 */
class RecordMarksTest {
    private val like = Mark("like", "heart", "いいね")
    private val save = Mark("save", "bookmark", "保存")
    private val posts = Entity(id = "posts", scope = "shared", marks = listOf(like, save))

    private fun record(count: Int = 2, mine: Boolean = false) =
        Record("r1", marks = mapOf("like" to MarkState(count, mine)))

    @Test
    fun readsCountsAndMineInDeclaredOrder() {
        val got = RecordMarks.read(posts, record(3, mine = true))
        assertEquals(listOf(RecordMark(like, MarkState(3, true)), RecordMark(save, MarkState(0, false))), got)
        assertEquals(MarkState(3, true), RecordMarks.read(posts, record(3, true), "like")!!.state)
        assertEquals(null, RecordMarks.read(posts, record(), "unknown"))
    }

    @Test
    fun privateEntityHasNoMarks() {
        assertTrue(RecordMarks.read(posts.copy(scope = null), record()).isEmpty())
        assertTrue(RecordMarks.read(null, record()).isEmpty())
        assertEquals(null, RecordMarks.read(posts.copy(scope = "private"), record(), "like"))
    }

    @Test
    fun pressAddsOneOptimisticallyThenConfirms() = runBlocking {
        val reply = CompletableDeferred<MarkState>()
        val sent = mutableListOf<Boolean>()
        val confirmed = mutableListOf<MarkState>()
        val t = MarkToggle(send = { _, _, on -> sent += on; reply.await() }, onConfirmed = { _, _, s -> confirmed += s })
        val r = record(2, mine = false)

        val job = async { t.toggle(r, "like") }
        yield()
        // 返事を待たずに 1 増えている
        assertEquals(MarkState(3, true), t.shown(r, "like"))
        assertEquals(listOf(true), sent)

        reply.complete(MarkState(5, true))
        assertEquals(MarkState(5, true), job.await())
        assertEquals(listOf(MarkState(5, true)), confirmed)
        // 上書きは外れ、記録の値に戻る(記録への書き込みは onConfirmed の役目)
        assertEquals(MarkState(2, false), t.shown(r, "like"))
    }

    @Test
    fun secondPressCancels() = runBlocking {
        val sent = mutableListOf<Boolean>()
        val t = MarkToggle(send = { _, _, on -> sent += on; if (on) MarkState(3, true) else MarkState(2, false) })

        // 1 人 1 回: 押してある記録をもう一度押すと取り消し(DELETE)
        assertEquals(MarkState(2, false), t.toggle(record(3, mine = true), "like"))
        assertEquals(listOf(false), sent)
        // 取り消しで 0 未満にしない
        assertEquals(MarkState(0, false), MarkState(0, true).toggled())
    }

    @Test
    fun failureRestoresPreviousCount() = runBlocking {
        val t = MarkToggle(send = { _, _, _ -> throw IOException("offline") })
        val r = record(2, mine = false)

        val failure = runCatching { t.toggle(r, "like") }.exceptionOrNull()
        assertTrue(failure is IOException)
        assertEquals(MarkState(2, false), t.shown(r, "like"))
    }

    @Test
    fun laterPressSurvivesEarlierFailure() = runBlocking {
        val first = CompletableDeferred<MarkState>()
        val second = CompletableDeferred<MarkState>()
        val replies = ArrayDeque(listOf(first, second))
        val t = MarkToggle(send = { _, _, _ -> replies.removeFirst().await() })
        val r = record(2, mine = false)

        val a = async { runCatching { t.toggle(r, "like") } }
        yield()
        val b = async { runCatching { t.toggle(r, "like") } }
        yield()
        // 連打: 付けて → 取り消して、元の数
        assertEquals(MarkState(2, false), t.shown(r, "like"))

        first.completeExceptionally(IOException("offline"))
        a.await()
        // 先の失敗で後の押下を消さない
        assertEquals(MarkState(2, false), t.shown(r, "like"))
        second.complete(MarkState(2, false))
        b.await()
        assertEquals(MarkState(2, false), t.shown(r, "like"))
    }

    @Test
    fun senderUsesPutAndDelete() = runBlocking {
        val server = MockWebServer().apply { start() }
        try {
            val client = HttpAppConfigClient(
                DesktopApi.http(
                    baseUrl = server.url("/").toString(),
                    tokens = DesktopApi.devTokens("dev-token"),
                    platform = ClientPlatform.LINUX,
                    client = DesktopApi.okHttp(ClientPlatform.LINUX, "ua"),
                ),
            )
            val send = MarkToggle.sender(client, "app1", "posts")
            server.enqueue(MockResponse().setHeader("Content-Type", "application/json").setBody("""{"count":3,"mine":true}"""))
            server.enqueue(MockResponse().setHeader("Content-Type", "application/json").setBody("""{"count":2,"mine":false}"""))

            assertEquals(MarkState(3, true), send("r1", "like", true))
            assertEquals(MarkState(2, false), send("r1", "like", false))

            val put = server.takeRequest()
            assertEquals("PUT", put.method)
            assertTrue(put.path!!, put.path!!.endsWith("/records/r1/marks/like"))
            assertTrue(put.path!!, put.path!!.startsWith("/v1/apps/app1/"))
            assertEquals("DELETE", server.takeRequest().method)
        } finally {
            server.shutdown()
        }
    }
}
