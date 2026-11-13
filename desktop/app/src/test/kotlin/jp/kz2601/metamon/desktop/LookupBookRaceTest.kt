package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionBookLookup
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.JSONValue
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test
import java.nio.file.Files

/** 検索がすぐ失敗しても、答えが「探しています」に上書きされない(待ち合わせ無しの偽物で)。 */
class LookupBookRaceTest {
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

    /** 検索をその場で走らせる: dispatch が返る前に答えが出る(すぐ失敗する通信と同じ順)。 */
    @Test
    fun instantFailureIsNotOverwrittenBySearching() {
        val s = DesktopSession(dsl, MockStore())
        s.host = DesktopActionHost(
            BookLookupClient { error("落ちた") },
            Files.createTempDirectory("lookup-race").toFile().apply { deleteOnExit() },
            NoopNotifier,
            voice = null,
            lookupThread = { it() },
        )
        val form = s.form(s.top)!!
        form.values["isbn"] = JSONValue.Str("9784000000000")
        s.dispatch(lookup, ActionContext(s.top, form))
        assertEquals(Strings["host_book_unavailable"], s.message)
    }

    /** 本物のスレッドでも、すぐ失敗した答えの後に「探しています」へ戻らない。 */
    @Test
    fun instantFailureOnThreadIsNotOverwrittenBySearching() {
        val searching = Strings["desktop_book_searching"]
        repeat(50) {
            val s = DesktopSession(dsl, MockStore())
            s.host = DesktopActionHost(
                BookLookupClient { error("落ちた") },
                Files.createTempDirectory("lookup-race").toFile().apply { deleteOnExit() },
                NoopNotifier,
                voice = null,
            )
            val form = s.form(s.top)!!
            form.values["isbn"] = JSONValue.Str("9784000000000")
            s.dispatch(lookup, ActionContext(s.top, form))
            val until = System.nanoTime() + 5_000_000_000L
            while (s.message == searching && System.nanoTime() < until) Thread.sleep(1)
            // 答えが出た後に「探しています」へ戻らない
            Thread.sleep(5)
            assertEquals(Strings["host_book_unavailable"], s.message)
        }
    }

    private object NoopNotifier : Notifier {
        override fun schedule(n: jp.kz2601.metamon.dsl.Notification): String = ""

        override fun cancel(id: String): String? = null
    }
}
