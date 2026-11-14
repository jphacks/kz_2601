package jp.kz2601.metamon.dsl.logic

import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.Fixtures
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** #1018: 生成した型(Action.calendarEvent・Action.bookLookup・fileImport.map)から logic の型へ写す。iOS の各 Plan テストの「デコード」の分。 */
class DslSpecsTest {
    private fun app(components: String) = AppDSL.decode(
        """{"schemaVersion":1,"app":{"name":"a","voiceEntry":"none"},"initialScreen":"home",
            "screens":[{"id":"home","title":"h","entity":"event","components":[$components]}]}""",
    )

    private fun walk(c: Component): List<Component> = listOf(c) + c.children.flatMap(::walk)

    @Test
    fun calendarEventFromAction() {
        val dsl = app(
            """{"type":"button","label":"予定に追加","action":{"type":"addToCalendar","entity":"event",
                "calendarEvent":{"startField":"start_at","alertMinutes":0,"locationField":"place"}}}""",
        )
        val action = dsl.screens[0].components[0].action!!
        assertEquals(ActionType.ADD_TO_CALENDAR, action.type)
        assertEquals(CalendarEventRequest("start_at", locationField = "place", alertMinutes = 0), action.calendarEvent!!.request)
        assertEquals(setOf("event"), dsl.formEntityIds)
    }

    @Test
    fun bookLookupFromAction() {
        val dsl = app(
            """{"type":"button","label":"ISBN で引く","action":{"type":"lookupBook",
                "bookLookup":{"isbnField":"isbn","titleField":"title","authorsField":"author"}}}""",
        )
        val action = dsl.screens[0].components[0].action!!
        assertEquals(BookLookupFields("isbn", titleField = "title", authorsField = "author"), action.bookLookup!!.fields)
        assertNull(app("""{"type":"button","label":"x","action":{"type":"lookupBook"}}""").screens[0].components[0].action?.bookLookup)
    }

    @Test
    fun fileImportMapFromExample() {
        val dsl = AppDSL.decode(Fixtures.examplesDir.resolve("file_import.json").readText())
        val c = dsl.screens.flatMap { s -> s.components.flatMap(::walk) }.first { it.type == ComponentType.FILE_IMPORT }
        assertEquals(listOf(CsvColumnMapping("氏名", "name")), c.map.csvMappings)
    }
}
