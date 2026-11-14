package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.CalendarEvent
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.logic.CalendarEventDraft
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId

/** addToCalendar(#911): フォームの入力中の値・行タップの記録から予定を作り、ACTION_INSERT の値にする。 */
class CalendarStepsTest {
    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"予定"},"initialScreen":"list",
         "entities":[{"id":"event","name":"予定","titleField":"name","fields":[
           {"id":"name","name":"名前","type":"string"},
           {"id":"at","name":"日時","type":"datetime"},
           {"id":"day","name":"日","type":"date"},
           {"id":"place","name":"場所","type":"string"}
         ]}],
         "screens":[{"id":"list","title":"一覧","components":[{"type":"list","entity":"event"}]}]}
        """.trimIndent(),
    )
    private val route = Route("list")

    private fun action(start: String = "at", entity: String? = "event") =
        Action(type = ActionType.ADD_TO_CALENDAR, entity = entity, calendarEvent = CalendarEvent(startField = start, locationField = "place", durationMinutes = 30))

    @Test
    fun formValuesBecomeDraftEvenUnsaved() = runTest {
        val rt = runtime(dsl)
        val form = FormDraft(rt.entityModel("event")!!)
        form.values["name"] = JSONValue.Str("歯医者")
        form.values["at"] = JSONValue.Str("2026-11-12T10:00:00Z")
        form.values["place"] = JSONValue.Str("駅前")
        val d = CalendarSteps.draft(action(entity = null), ActionContext(route, form = form), rt) as CalendarSteps.Draft.Ready
        assertEquals("歯医者", d.draft.title)
        assertEquals(Instant.parse("2026-11-12T10:30:00Z"), d.draft.end)
        val x = CalendarSteps.insertExtras(d.draft)
        assertFalse(x.allDay)
        assertEquals(Instant.parse("2026-11-12T10:00:00Z").toEpochMilli(), x.beginMillis)
        assertEquals("駅前", x.location)
        assertNull(x.description)
    }

    @Test
    fun rowTapUsesTheRecord() = runTest {
        val rt = runtime(dsl)
        val id = rt.createRecord("event", mapOf("name" to JSONValue.Str("会議"), "at" to JSONValue.Str("2026-11-12T01:00:00Z")))!!
        val d = CalendarSteps.draft(action(), ActionContext(route, recordId = id), rt) as CalendarSteps.Draft.Ready
        assertEquals("会議", d.draft.title)
    }

    @Test
    fun missingStartOrRecordIsAToast() = runTest {
        val rt = runtime(dsl)
        val id = rt.createRecord("event", mapOf("name" to JSONValue.Str("会議")))!!
        assertEquals(
            CalendarSteps.Draft.Invalid(UiText.Res(R.string.host_calendar_no_start, "日時")),
            CalendarSteps.draft(action(), ActionContext(route, recordId = id), rt),
        )
        assertEquals(CalendarSteps.Draft.Invalid(CalendarSteps.NO_RECORD), CalendarSteps.draft(action(), ActionContext(route), rt))
        assertEquals(CalendarSteps.Draft.Invalid(CalendarSteps.NO_RECORD), CalendarSteps.draft(action(entity = "nope"), ActionContext(route, recordId = id), rt))
    }

    @Test
    fun allDayEndsAtNextMidnight() {
        val zone = ZoneId.of("Asia/Tokyo")
        val start = LocalDate.of(2026, 11, 12).atStartOfDay(zone).toInstant()
        val draft = CalendarEventDraft("休み", start, start, isAllDay = true, location = null, notes = "メモ", alertOffsetSeconds = null)
        val x = CalendarSteps.insertExtras(draft, zone)
        assertTrue(x.allDay)
        assertEquals(start.toEpochMilli(), x.beginMillis)
        assertEquals(LocalDate.of(2026, 11, 13).atStartOfDay(zone).toInstant().toEpochMilli(), x.endMillis)
        assertEquals("メモ", x.description)
    }

    @Test
    fun runtimeHandsCalendarToHost() = runTest {
        var called = 0
        val host = object : RuntimeHost {
            override suspend fun addToCalendar(action: Action, context: ActionContext, runtime: AppRuntime) {
                called += 1
            }
        }
        val rt = runtime(dsl, host = host)
        rt.run(action(), ActionContext(route))
        assertEquals(1, called)
    }
}
