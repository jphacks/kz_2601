package jp.kz2601.metamon.dsl.logic

import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.Fixtures
import jp.kz2601.metamon.dsl.records.JSONValue
import kotlinx.serialization.json.Json
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.time.Duration

/** iOS の CalendarEventPlanTests(addToCalendar: レコード → 新規イベントの中身)と BookLookupPlanTests(lookupBook)。 */
class CalendarBookTest {
    private val savedZone = FieldCoding.zone

    @Before
    fun setUp() {
        FieldCoding.zone = Fixtures.TOKYO
    }

    @After
    fun tearDown() {
        FieldCoding.zone = savedZone
    }

    // MARK: - カレンダー

    private val event = Entity.parse(
        """
        {"id":"event","name":"イベント","titleField":"name","fields":[
          {"id":"name","name":"名前","type":"string"},
          {"id":"day","name":"日","type":"date"},
          {"id":"start_at","name":"開始","type":"datetime"},
          {"id":"end_at","name":"終了","type":"datetime"},
          {"id":"place","name":"場所","type":"string"},
          {"id":"memo","name":"メモ","type":"text"},
          {"id":"round","name":"回","type":"integer"}]}
        """,
    )

    private fun draft(spec: CalendarEventRequest, data: Map<String, JSONValue>): CalendarEventDraft =
        (CalendarEventPlan.build(spec, event, data) as CalendarEventPlan.Result.Success).draft

    private val start = "2026-11-14T10:00:00+09:00"
    private val startDate get() = FieldCoding.strictDatetime(start)!!

    private fun s(v: String) = JSONValue.Str(v)
    private fun length(d: CalendarEventDraft) = Duration.between(d.start, d.end).seconds

    @Test
    fun timedEventDefaultsToSixtyMinutes() {
        val d = draft(CalendarEventRequest("start_at"), mapOf("name" to s(" JPHACKS 本戦 "), "start_at" to s(start)))
        assertEquals("JPHACKS 本戦", d.title)
        assertFalse(d.isAllDay)
        assertEquals(startDate, d.start)
        assertEquals(3600, length(d))
        assertNull(d.alertOffsetSeconds)
        assertNull(d.location)
    }

    @Test
    fun endFieldLocationNotesAndAlert() {
        val spec = CalendarEventRequest("start_at", "end_at", 30, "name", "place", "memo", 1440)
        val d = draft(
            spec,
            mapOf("name" to s("発表"), "start_at" to s(start), "end_at" to s("2026-11-14T12:30:00+09:00"), "place" to s("金沢"), "memo" to s("スライド持参")),
        )
        assertEquals(9000, length(d))
        assertEquals("金沢", d.location)
        assertEquals("スライド持参", d.notes)
        assertEquals(-86400L, d.alertOffsetSeconds)
    }

    @Test
    fun emptyOrEarlierEndFallsBackToDuration() {
        val spec = CalendarEventRequest("start_at", "end_at", durationMinutes = 90)
        for (end in listOf(JSONValue.Null, s(""), s("2026-11-14T09:00:00+09:00"))) {
            assertEquals("$end", 5400, length(draft(spec, mapOf("start_at" to s(start), "end_at" to end))))
        }
    }

    @Test
    fun dateFieldIsAllDay() {
        val d = draft(CalendarEventRequest("day", alertMinutes = 0), mapOf("day" to s("2026-11-14")))
        assertTrue(d.isAllDay)
        assertEquals(FieldCoding.strictDay("2026-11-14"), d.start)
        assertEquals(d.start, d.end)
        assertEquals(0L, d.alertOffsetSeconds)
    }

    @Test
    fun titleFallsBackToEntityName() {
        assertEquals("イベント", draft(CalendarEventRequest("start_at"), mapOf("start_at" to s(start))).title)
        assertEquals("3", draft(CalendarEventRequest("start_at", titleField = "round"), mapOf("start_at" to s(start), "round" to JSONValue.Num(3.0))).title)
    }

    @Test
    fun missingStartFailsWithFieldName() {
        assertEquals(CalendarEventPlan.Result.NoStart("開始"), CalendarEventPlan.build(CalendarEventRequest("start_at"), event, mapOf("start_at" to s("あした"))))
    }

    @Test
    fun requestDecodes() {
        val raw = """{"startField":"start_at","alertMinutes":0,"locationField":"place"}"""
        assertEquals(CalendarEventRequest("start_at", locationField = "place", alertMinutes = 0), CalendarEventRequest.from(Json.parseToJsonElement(raw)))
        assertNull(CalendarEventRequest.from(Json.parseToJsonElement("""{"endField":"x"}""")))
    }

    // MARK: - 本

    private val bookEntity = Entity.parse(
        """
        {"id":"book","name":"本","titleField":"title","fields":[
          {"id":"isbn","name":"ISBN","type":"string"},
          {"id":"title","name":"書名","type":"string"},
          {"id":"author","name":"著者","type":"text"},
          {"id":"publisher","name":"出版社","type":"string"},
          {"id":"published","name":"出版日","type":"date"},
          {"id":"pages","name":"ページ","type":"integer"}]}
        """,
    )

    private val book = BookInfo(
        isbn = "9784780802047", title = "おにぎりレシピ101", authors = "山田玲子／著 水野菜生／英訳",
        publisher = "ポット出版", publishedDate = "2014-04", source = BookInfo.Source("openBD", "https://openbd.jp/"),
    )

    @Test
    fun normalizeIsbn() {
        assertEquals("9784780802047", BookLookupPlan.normalizeISBN("978-4-7808-0204-7"))
        assertEquals("ISBN-10 は 13 桁に", "9784780802047", BookLookupPlan.normalizeISBN("4780802040"))
        assertEquals("9784780802047", BookLookupPlan.normalizeISBN("4-7808-0204-0"))
        assertEquals("末尾の X", "9780804429573", BookLookupPlan.normalizeISBN("080442957X"))
        for (bad in listOf("", "9784780802048", "4780802041", "4900000000004", "X780802040", "978478080204", "abc", "https://evil.example/")) {
            assertNull(bad, BookLookupPlan.normalizeISBN(bad))
        }
    }

    @Test
    fun valuesFillMappedFields() {
        val spec = BookLookupFields("isbn", "title", "author", "publisher", "published")
        val v = BookLookupPlan.values(spec, book, bookEntity)
        assertEquals(s("おにぎりレシピ101"), v["title"])
        assertEquals(s("山田玲子／著 水野菜生／英訳"), v["author"])
        assertEquals(s("ポット出版"), v["publisher"])
        assertEquals("date の field は月だけでも日付に", s("2014-04-01"), v["published"])
        assertNull(v["isbn"])
    }

    @Test
    fun valuesSkipMissingWrongTypeAndEmpty() {
        val spec = BookLookupFields("isbn", titleField = "pages", authorsField = "nope", publisherField = "publisher")
        assertEquals(emptyMap<String, JSONValue>(), BookLookupPlan.values(spec, book.copy(publisher = "  "), bookEntity))
    }

    @Test
    fun dateString() {
        assertEquals("2014-01-01", BookLookupPlan.dateString("2014"))
        assertEquals("2014-04-08", BookLookupPlan.dateString("2014-04-08"))
        assertNull(BookLookupPlan.dateString("2014-13"))
        assertNull(BookLookupPlan.dateString("平成26年"))
    }
}
