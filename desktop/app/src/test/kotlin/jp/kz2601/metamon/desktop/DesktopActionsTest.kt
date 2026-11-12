package jp.kz2601.metamon.desktop

import com.google.zxing.BarcodeFormat
import com.google.zxing.qrcode.QRCodeWriter
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.Notification
import jp.kz2601.metamon.dsl.Presentation
import jp.kz2601.metamon.dsl.logic.CalendarEventDraft
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.navigation.Route
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.awt.image.BufferedImage
import java.nio.file.Files
import java.time.Instant
import java.time.ZoneId
import java.time.ZonedDateTime

/** アクション 16 種のうち OS に頼む物(書き出し・カレンダー・本・通知・音声など)と、sheet / fullScreen の遷移。 */
class DesktopActionsTest {
    private val silent = object : Notifier {
        val scheduled = mutableListOf<String>()
        override fun schedule(n: Notification): String = "予約 ${n.id}".also { scheduled += n.id }
        override fun cancel(id: String): String? = null
    }

    private fun session(name: String, store: MockStore = MockStore()): Pair<DesktopSession, java.io.File> {
        val dir = Files.createTempDirectory("desktop-actions").toFile()
        val s = DesktopSession(Examples.load(name), store)
        s.host = DesktopActionHost(BookLookupClient.mock, dir, silent)
        return s to dir
    }

    private fun DesktopSession.run(a: Action) = dispatch(a, ActionContext(top, form(top)))

    @Test
    fun sheetAndFullScreenStackOnTopAndClose() {
        val (s, _) = session("kakeibo")
        val home = s.top
        s.run(Action(type = ActionType.NAVIGATE, screen = "expense_form", presentation = Presentation.SHEET))
        assertEquals(Route.SHEET, s.top.stack)
        s.back()
        assertEquals(home, s.top)
        s.run(Action(type = ActionType.NAVIGATE, screen = "expense_form", presentation = Presentation.FULL_SCREEN))
        assertEquals(Route.FULL, s.top.stack)
        s.back()
        assertEquals(home, s.top)
    }

    @Test
    fun exportWritesCsvAndReportsCount() {
        val store = MockStore.fromJson("""{"expense": [{"id": "a", "data": {"amount": 1200, "category": "食費", "date": "2026-11-10"}}]}""")
        val (s, dir) = session("kakeibo", store)
        s.run(Action(type = ActionType.EXPORT, entity = "expense", title = "支出"))
        val csv = dir.resolve("支出.csv")
        assertTrue(s.message.orEmpty(), csv.isFile)
        assertTrue(csv.readText().contains("1200") || csv.readText().contains("1,200"))
        assertTrue(s.message!!.startsWith("1 件を書き出しました"))
        assertTrue(s.effects.last().toString().contains("\"type\":\"export\""))
        s.run(Action(type = ActionType.EXPORT, entity = "expense", title = "支出", format = jp.kz2601.metamon.dsl.ActionFormat.PDF))
        val pdf = dir.resolve("支出.pdf")
        assertTrue(s.message.orEmpty(), pdf.isFile)
        assertEquals("%PDF", pdf.readBytes().copyOfRange(0, 4).decodeToString())
    }

    @Test
    fun unavailableActionsSayWhy() {
        val (s, _) = session("shopping")
        s.run(Action(type = ActionType.OPEN_VOICE))
        assertEquals(DesktopActionHost.UNAVAILABLE_VOICE, s.message)
        s.run(Action(type = ActionType.START_ACTIVITY))
        assertEquals(DesktopActionHost.UNAVAILABLE_ACTIVITY, s.message)
        s.run(Action(type = ActionType.FOLLOW))
        assertEquals(DesktopActionHost.UNAVAILABLE_FOLLOW, s.message)
        s.run(Action(type = ActionType.OPEN_URL, url = "javascript:alert(1)"))
        assertTrue(s.message!!.startsWith("openUrl:"))
        s.run(Action(type = ActionType.SCHEDULE_NOTIFICATION, notification = Notification(id = "n1", title = "t", body = "b")))
        assertEquals(listOf("n1"), silent.scheduled)
    }

    @Test
    fun resetFormClearsDraft() {
        val (s, _) = session("kakeibo")
        s.run(Action(type = ActionType.NAVIGATE, screen = "expense_form"))
        val form = s.form(s.top)!!
        form.values["memo"] = JSONValue.Str("パン")
        s.run(Action(type = ActionType.RESET_FORM))
        assertTrue(form.values["memo"]?.isNull ?: true)
    }

    @Test
    fun icsAllDayAndTimed() {
        val zone = ZoneId.of("Asia/Tokyo")
        val start = ZonedDateTime.of(2026, 11, 12, 0, 0, 0, 0, zone).toInstant()
        val allDay = Ics.event(CalendarEventDraft("会議; 定例", start, start, true, "金沢", null, -600), "u1", Instant.EPOCH, zone)
        assertTrue(allDay.contains("DTSTART;VALUE=DATE:20261112\r\n"))
        assertTrue(allDay.contains("DTEND;VALUE=DATE:20261113\r\n"))
        assertTrue(allDay.contains("SUMMARY:会議\\; 定例\r\n"))
        assertTrue(allDay.contains("TRIGGER:-PT10M"))
        val timed = Ics.event(CalendarEventDraft("x", start, start.plusSeconds(3600), false, null, null, null), "u2", Instant.EPOCH, zone)
        assertTrue(timed.contains("DTSTART:20261111T150000Z"))
        assertTrue(timed.contains("DTEND:20261111T160000Z"))
    }

    @Test
    fun notificationFiresAtNextTime() {
        val zone = ZoneId.of("Asia/Tokyo")
        val now = ZonedDateTime.of(2026, 11, 12, 8, 0, 0, 0, zone)
        assertEquals(now.withHour(7).plusDays(1), silent.nextFire(Notification("a", "t", "b", time = "07:00"), now))
        assertEquals(now.withHour(9), silent.nextFire(Notification("a", "t", "b", time = "09:00"), now))
        assertNull(silent.nextFire(Notification("a", "t", "b"), now))
    }

    @Test
    fun bookLookupResponseParses() {
        val b = HttpBookLookupClient.parse("""{"isbn":"9784000000000","title":"本","authors":"著者","source":{"name":"openBD","url":"https://openbd.jp"}}""")
        assertEquals("本", b.title)
        assertEquals("openBD", b.source.name)
    }

    @Test
    fun qrDecodesFromImage() {
        val m = QRCodeWriter().encode("TAILOR-42", BarcodeFormat.QR_CODE, 200, 200)
        val img = BufferedImage(m.width, m.height, BufferedImage.TYPE_INT_RGB)
        for (x in 0 until m.width) for (y in 0 until m.height) img.setRGB(x, y, if (m[x, y]) 0 else 0xFFFFFF)
        assertEquals("TAILOR-42", InputLeaves.decodeCode(img))
    }

    @Test
    fun inputLabelsFallBackLikeIos() {
        val dsl = Examples.load("kakeibo")
        val s = DesktopSession(dsl)
        val form = s.form(Route("expense_form"))!!
        val date = dsl.screens.flatMap { it.allComponents() }.first { it.type == jp.kz2601.metamon.dsl.ComponentType.DATE_PICKER }
        assertEquals(date.label ?: form.entity.field(date.field!!)!!.name, InputLeaves.label(date, form))
    }
}
