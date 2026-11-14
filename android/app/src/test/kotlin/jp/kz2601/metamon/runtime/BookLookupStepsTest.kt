package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionBookLookup
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.logic.BookInfo
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.services.books.BookLookupClient
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Test
import java.io.IOException

/** lookupBook(#911・iOS の BookLookupSteps): フォームの ISBN から引いて書名などを上書き。失敗はメッセージだけ。 */
class BookLookupStepsTest {
    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"本棚"},"initialScreen":"form",
         "entities":[{"id":"book","name":"本","fields":[
           {"id":"isbn","name":"ISBN","type":"string"},
           {"id":"title","name":"書名","type":"string"},
           {"id":"author","name":"著者","type":"string"},
           {"id":"published","name":"出版日","type":"date"}
         ]}],
         "screens":[{"id":"form","title":"本","entity":"book","components":[]}]}
        """.trimIndent(),
    )
    private val action = Action(
        type = ActionType.LOOKUP_BOOK,
        bookLookup = ActionBookLookup(isbnField = "isbn", titleField = "title", authorsField = "author", publishedDateField = "published"),
    )
    private val book = BookInfo("9784780802047", "本の名前", authors = "著者名", publishedDate = "2014-04", source = BookInfo.Source("openBD", "https://openbd.jp"))

    private fun TestScope.form(isbn: String): FormDraft =
        FormDraft(runtime(dsl).entityModel("book")!!).apply { values["isbn"] = JSONValue.Str(isbn) }

    @Test
    fun fillsFieldsAndNormalizesIsbn() = runTest {
        var asked: String? = null
        val f = form("4-7808-0204-0")
        val msg = BookLookupSteps.run(action, f) { isbn ->
            asked = isbn
            book
        }
        assertEquals("9784780802047", asked)
        assertEquals(UiText.Res(R.string.host_book_filled, "本の名前", "openBD"), msg)
        assertEquals(JSONValue.Str("本の名前"), f.values["title"])
        assertEquals(JSONValue.Str("著者名"), f.values["author"])
        assertEquals(JSONValue.Str("2014-04-01"), f.values["published"])
        assertEquals(JSONValue.Str("9784780802047"), f.values["isbn"])
    }

    @Test
    fun badInputNeverCallsTheServer() = runTest {
        val never = BookLookupClient { error("呼ばない") }
        assertEquals(BookLookupSteps.FORM_ONLY, BookLookupSteps.run(action, null, never))
        assertEquals(BookLookupSteps.ISBN_EMPTY, BookLookupSteps.run(action, form("  "), never))
        assertEquals(BookLookupSteps.ISBN_INVALID, BookLookupSteps.run(action, form("123"), never))
        assertEquals("つながらない構成", BookLookupSteps.UNAVAILABLE, BookLookupSteps.run(action, form("9784780802047"), null))
    }

    @Test
    fun failuresBecomeMessages() = runTest {
        fun failing(e: Exception) = BookLookupClient { throw e }
        val off = ApiError.Server(503, "unavailable", "off", JsonObject(mapOf("reason" to JsonPrimitive("disabled"))))
        assertEquals(BookLookupSteps.NOT_FOUND, BookLookupSteps.run(action, form("9784780802047"), failing(ApiError.Server(404, "not_found", ""))))
        assertEquals(BookLookupSteps.OFF, BookLookupSteps.run(action, form("9784780802047"), failing(off)))
        assertEquals(BookLookupSteps.ISBN_INVALID, BookLookupSteps.run(action, form("9784780802047"), failing(ApiError.Server(400, "bad", ""))))
        assertEquals(BookLookupSteps.UNAVAILABLE, BookLookupSteps.run(action, form("9784780802047"), failing(ApiError.Server(503, "unavailable", ""))))
        assertEquals(BookLookupSteps.UNAVAILABLE, BookLookupSteps.run(action, form("9784780802047"), failing(IOException("offline"))))
    }

    @Test
    fun editsWhileWaitingAreKept() = runTest {
        val f = form("9784780802047")
        val msg = BookLookupSteps.run(action, f) {
            f.values["title"] = JSONValue.Str("手で直した")
            book
        }
        assertEquals(UiText.Res(R.string.host_book_filled, "本の名前", "openBD"), msg)
        assertEquals("手で直した欄は上書きしない", JSONValue.Str("手で直した"), f.values["title"])
        assertEquals(JSONValue.Str("著者名"), f.values["author"])

        val g = form("9784780802047")
        val none = BookLookupSteps.run(action, g) {
            g.values["isbn"] = JSONValue.Str("9780804429573")
            book
        }
        assertNull("ISBN を打ち直したら古い結果を入れない", none)
        assertNotEquals(JSONValue.Str("本の名前"), g.values["title"])
    }
}
