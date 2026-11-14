package jp.kz2601.metamon.desktop.marks

import jp.kz2601.metamon.desktop.A11y
import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.net.ClientPlatform
import jp.kz2601.metamon.desktop.net.DesktopApi
import jp.kz2601.metamon.dsl.ListItem
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Mark
import jp.kz2601.metamon.dsl.records.MarkState
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.services.live.HttpAppConfigClient
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.util.concurrent.TimeUnit

/** 一覧の行の印のボタン(#2811): 押す・取り消す・失敗で戻る を MockWebServer で。 */
class RowMarksTest {
    private val like = Mark("like", "heart", "いいね")
    private val posts = Entity(id = "posts", scope = "shared", marks = listOf(like))
    private val row = Record("r1", marks = mapOf("like" to MarkState(2, false)))

    private lateinit var server: MockWebServer
    private lateinit var marks: RowMarks

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        val client = HttpAppConfigClient(
            DesktopApi.http(
                baseUrl = server.url("/").toString(),
                tokens = DesktopApi.devTokens("dev-token"),
                platform = ClientPlatform.LINUX,
                client = DesktopApi.okHttp(ClientPlatform.LINUX, "ua"),
            ),
        )
        // 画面の仮 id(local-a)はサーバーの id(srv-a)へ、local-b はまだ送っていない作成
        marks = RowMarks.live(client, "app1") { _, id ->
            when (id) {
                "local-a" -> "srv-a"
                "local-b" -> null
                else -> id
            }
        }
    }

    @After
    fun tearDown() = server.shutdown()

    private fun reply(body: String) = server.enqueue(MockResponse().setHeader("Content-Type", "application/json").setBody(body))

    @Test
    fun pressPutsThenSecondPressDeletes() = runBlocking {
        reply("""{"count":3,"mine":true}""")
        assertTrue(marks.press("posts", row, "like"))
        val put = server.takeRequest(5, TimeUnit.SECONDS)!!
        assertEquals("PUT", put.method)
        assertTrue(put.path!!, put.path!!.startsWith("/v1/apps/app1/") && put.path!!.endsWith("/records/r1/marks/like"))
        // 記録は印を持たないので、返事の値を覚えて出す
        assertEquals(MarkState(3, true), marks.shown("posts", row, "like"))

        reply("""{"count":2,"mine":false}""")
        assertTrue(marks.press("posts", row, "like"))
        assertEquals("DELETE", server.takeRequest(5, TimeUnit.SECONDS)!!.method)
        assertEquals(MarkState(2, false), marks.shown("posts", row, "like"))
    }

    @Test
    fun serverErrorRestoresCount() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(503).setBody("""{"error":"unavailable"}"""))
        assertFalse(marks.press("posts", row, "like"))
        assertEquals("PUT", server.takeRequest(5, TimeUnit.SECONDS)!!.method)
        assertEquals(MarkState(2, false), marks.shown("posts", row, "like"))
    }

    @Test
    fun usesServerIdAndSkipsUnsentRows() = runBlocking {
        reply("""{"count":1,"mine":true}""")
        assertTrue(marks.press("posts", Record("local-a"), "like"))
        assertTrue(server.takeRequest(5, TimeUnit.SECONDS)!!.path!!.endsWith("/records/srv-a/marks/like"))
        assertEquals(MarkState(1, true), marks.shown("posts", Record("local-a"), "like"))

        // まだ送っていない作成は送らずに戻す
        assertFalse(marks.press("posts", Record("local-b"), "like"))
        assertEquals(MarkState(0, false), marks.shown("posts", Record("local-b"), "like"))
        assertEquals(1, server.requestCount)
    }

    @Test
    fun onlyDeclaredMarkOnSharedEntity() {
        assertEquals(like, RowMarks.mark(ListItem(title = "t", mark = "like"), posts))
        assertNull(RowMarks.mark(ListItem(title = "t"), posts))
        assertNull(RowMarks.mark(ListItem(title = "t", mark = "save"), posts))
        assertNull(RowMarks.mark(ListItem(title = "t", mark = "like"), posts.copy(scope = "private")))
        assertNull(RowMarks.mark(ListItem(title = "t", mark = "like"), null))
        assertEquals("heart.fill", RowMarks.symbol("heart", on = true))
        assertEquals("bookmark", RowMarks.symbol("bookmark", on = true))
    }

    @Test
    fun readsCountAndWhetherMine() {
        assertEquals("いいね 3 件・付けています", A11y.mark(like, MarkState(3, true)))
        assertEquals("いいね 0 件", A11y.mark(like, MarkState(0, false)))
        assertEquals("いいねを外す", A11y.markAction(like, MarkState(3, true)))
        assertEquals("いいねを付ける", A11y.markAction(like, MarkState(3, false)))
        Strings.withLang(Lang.EN) {
            assertEquals("いいね 3, added by you", A11y.mark(like, MarkState(3, true)))
            assertEquals("Add いいね", A11y.markAction(like, MarkState(0, false)))
        }
    }
}
