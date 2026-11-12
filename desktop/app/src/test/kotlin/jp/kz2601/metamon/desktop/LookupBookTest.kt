package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionBookLookup
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.logic.BookInfo
import jp.kz2601.metamon.dsl.records.JSONValue
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.nio.file.Files
import java.util.Collections

/** 本の検索(lookupBook): ISBN の形・見つからない・フォームに入る値。検索は作り物の BookLookupClient で通信しない。 */
class LookupBookTest {
    private val dsl = AppDSL.decode(
        """
        {
          "schemaVersion": 1,
          "app": { "name": "蔵書" },
          "initialScreen": "book_form",
          "entities": [{
            "id": "book", "name": "本", "titleField": "title",
            "fields": [
              { "id": "isbn", "name": "ISBN", "type": "string" },
              { "id": "title", "name": "書名", "type": "string" },
              { "id": "author", "name": "著者", "type": "string" },
              { "id": "publisher", "name": "出版社", "type": "string" },
              { "id": "published", "name": "出版日", "type": "date" }
            ]
          }],
          "screens": [{ "id": "book_form", "title": "本", "entity": "book", "components": [] }]
        }
        """.trimIndent(),
    )

    private val lookup = Action(
        type = ActionType.LOOKUP_BOOK,
        bookLookup = ActionBookLookup("isbn", "title", "author", "publisher", "published"),
    )

    private val asked: MutableList<String> = Collections.synchronizedList(mutableListOf())
    private var langBefore = Strings.lang

    @Before
    fun setUp() {
        langBefore = Strings.lang
        Strings.lang = Lang.JA
    }

    @After
    fun tearDown() {
        Strings.lang = langBefore
    }

    private fun session(book: BookInfo?): DesktopSession {
        val s = DesktopSession(dsl, MockStore())
        val client = BookLookupClient { isbn -> asked += isbn; book }
        s.host = DesktopActionHost(client, Files.createTempDirectory("lookup-book").toFile().apply { deleteOnExit() }, NoopNotifier, voice = null)
        return s
    }

    private fun DesktopSession.lookupWith(isbn: String?) {
        val form = form(top)!!
        isbn?.let { form.values["isbn"] = JSONValue.Str(it) }
        dispatch(lookup, ActionContext(top, form))
    }

    /** 検索は別のスレッド。「探しています」から変わるまで待つ(最大 5 秒)。 */
    private fun DesktopSession.awaitResult(): String? {
        val searching = Strings["desktop_book_searching"]
        val until = System.nanoTime() + 5_000_000_000L
        while (message == searching && System.nanoTime() < until) Thread.sleep(10)
        return message
    }

    private val book = BookInfo(
        isbn = "9784000000000",
        title = "見本の本",
        authors = "見本 太郎",
        publisher = "見本書房",
        publishedDate = "2024-04",
        source = BookInfo.Source("openBD", "https://openbd.jp"),
    )

    @Test
    fun lookupBookFillsFields() {
        val s = session(book)
        // ハイフンを外した ISBN-13 の数字で聞く
        s.lookupWith("978-4-00-000000-0")
        assertEquals(Strings["desktop_book_searching"], s.message)
        assertEquals(Strings.format("host_book_filled", "見本の本", "openBD"), s.awaitResult())
        assertEquals(listOf("9784000000000"), asked.toList())
        val values = s.form(s.top)!!.values
        assertEquals("見本の本", values["title"]?.stringValue)
        assertEquals("見本 太郎", values["author"]?.stringValue)
        assertEquals("見本書房", values["publisher"]?.stringValue)
        assertEquals("出版日は日付にする", "2024-04-01", values["published"]?.stringValue)
        assertEquals("ISBN は打ったまま", "978-4-00-000000-0", values["isbn"]?.stringValue)
    }

    @Test
    fun lookupBookNotFound() {
        val s = session(null)
        s.lookupWith("9784000000000")
        assertEquals(Strings["host_book_not_found"], s.awaitResult())
        assertEquals("この ISBN の本は見つかりませんでした。手で入力してください", s.message)
        assertNull("見つからなければ入れない", s.form(s.top)!!.values["title"]?.takeIf { it != JSONValue.Null })
    }

    @Test
    fun lookupBookFailureSaysUnavailable() {
        val s = DesktopSession(dsl, MockStore())
        s.host = DesktopActionHost({ error("落ちた") }, Files.createTempDirectory("lookup-book").toFile().apply { deleteOnExit() }, NoopNotifier, voice = null)
        s.lookupWith("9784000000000")
        assertEquals(Strings["host_book_unavailable"], s.awaitResult())
    }

    @Test
    fun lookupBookChecksIsbnBeforeAsking() {
        val s = session(book)
        s.lookupWith(null)
        assertEquals(Strings["host_book_isbn_empty"], s.message)
        s.lookupWith("  ")
        assertEquals(Strings["host_book_isbn_empty"], s.message)
        s.lookupWith("9784000000001") // 検査数字が合わない
        assertEquals(Strings["host_book_isbn_invalid"], s.message)
        s.lookupWith("ISBN")
        assertEquals(Strings["host_book_isbn_invalid"], s.message)
        // フォームの外(一覧の行など)からは読まない
        s.dispatch(lookup, ActionContext(s.top, null))
        assertEquals(Strings["host_book_form_only"], s.message)
        // bookLookup の無いアクション
        s.dispatch(Action(type = ActionType.LOOKUP_BOOK), ActionContext(s.top, s.form(s.top)))
        assertEquals(Strings["host_book_unavailable"], s.message)
        assertTrue("どれも検索しない", asked.isEmpty())
    }

    private object NoopNotifier : Notifier {
        override fun schedule(n: jp.kz2601.metamon.dsl.Notification): String = ""

        override fun cancel(id: String): String? = null
    }
}
