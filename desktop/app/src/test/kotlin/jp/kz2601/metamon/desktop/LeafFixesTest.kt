package jp.kz2601.metamon.desktop

import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.WindowPosition
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.ExprNumber
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.awt.Rectangle
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import javax.imageio.ImageIO

/** 入力の部品・描画・印刷・通知・ウィンドウの位置の細かい直し。 */
class LeafFixesTest {
    @Test
    fun fileNameNeverThrowsAndHandlesWindowsPathsAndSpaces() {
        assertEquals("a b.mp3", InputLeaves.fileName("file:/tmp/a%20b.mp3"))
        assertEquals("a b.mp3", InputLeaves.fileName("C:\\Users\\me\\My Music\\a b.mp3"))
        assertEquals("a b.mp3", InputLeaves.fileName("/tmp/My Music/a b.mp3"))
        assertEquals("y.m4a", InputLeaves.fileName("https://example.com/x/y.m4a"))
        assertEquals("%zz", InputLeaves.fileName("%zz"))
        assertEquals("::", InputLeaves.fileName("::"))
    }

    @Test
    fun drawingKeepsTheCanvasSize() {
        // 600×300 より広い Canvas の右の方に描いた線も PNG に残る
        val file = Drawing.writePng(listOf(listOf(Offset(700f, 150f), Offset(780f, 150f))), 800, 200)
        val img = ImageIO.read(file)
        file.delete()
        assertEquals(800, img.width)
        assertEquals(200, img.height)
        assertTrue("線が黒く残る", (img.getRGB(740, 150) and 0x00FFFFFF) < 0x404040)
        // 大きさが分からない(0)でも投げない
        Drawing.writePng(emptyList(), 0, 0).delete()
    }

    @Test
    fun ratingIsAnInputOnlyWithAField() {
        val display = Component(type = ComponentType.RATING, value = ExprNumber(3.0))
        val input = Component(type = ComponentType.RATING, field = "stars")
        assertFalse(InputLeaves.isInput(display))
        assertTrue(InputLeaves.isInput(input))
        val s = DesktopSession(Examples.load("shopping"))
        assertEquals("表示の rating はフォームが無くても出す", "rendered", s.tapState(display, null))
        assertEquals("needsForm", s.tapState(input, null))
        assertTrue(InputLeaves.isInput(Component(type = ComponentType.PHOTO_FIELD, field = "p")))
    }

    @Test
    fun printRunnerStartsOneJobAtATime() {
        val queued = mutableListOf<() -> Unit>()
        val runner = PrintRunner(background = { queued += it })
        var sent = 0
        val results = mutableListOf<String>()
        val prepared = DesktopPrint.Prepared(java.awt.print.PrinterJob.getPrinterJob(), javax.print.attribute.HashPrintRequestAttributeSet(), null)
        val start = { runner.start({ prepared }, { sent++; "ok" }, { results += it }, onMain = { it() }) }
        assertTrue(start())
        assertTrue(runner.busy)
        assertFalse("送っている間の 2 回目は始めない", start())
        assertEquals(1, queued.size)
        queued.single()()
        assertEquals(1, sent)
        assertEquals(listOf("ok"), results)
        assertFalse(runner.busy)
        // やめた時は送らずに一言だけ
        assertTrue(runner.start({ DesktopPrint.Prepared(null, prepared.attrs, "cancelled") }, { sent++; "ok" }, { results += it }, onMain = { it() }))
        assertEquals(listOf("ok", "cancelled"), results)
        assertEquals(1, sent)
        assertFalse(runner.busy)
    }

    @Test
    fun trayTimersSurviveConcurrentScheduleAndCancel() {
        val pool = Executors.newFixedThreadPool(8)
        repeat(400) { i ->
            pool.submit {
                val id = "t-${i % 50}"
                if (i % 2 == 0) Notifier.Tray.arm(id, 0) {} else Notifier.Tray.cancel(id)
            }
        }
        pool.shutdown()
        assertTrue(pool.awaitTermination(10, TimeUnit.SECONDS))
        // 予約を全部消すと、表に何も残らない(取り消し・鳴った後の掃除が競合しても壊れない)
        Notifier.Tray.pending().forEach { Notifier.Tray.cancel(it) }
        assertTrue(Notifier.Tray.pending().isEmpty())
    }

    private fun at(b: WindowBounds.Bounds): Pair<Float, Float>? = (b.position as? WindowPosition.Absolute)?.let { it.x.value to it.y.value }

    @Test
    fun windowBoundsStayOnAConnectedScreen() {
        val screens = listOf(Rectangle(0, 0, 1920, 1080))
        val gone = WindowBounds.fit(WindowBounds.Bounds(DpSize(480.dp, 820.dp), WindowPosition(2500.dp, 100.dp)), screens)
        assertEquals("外した画面の位置は既定に戻す", WindowPosition.PlatformDefault, gone.position)
        val big = WindowBounds.fit(WindowBounds.Bounds(DpSize(3000.dp, 2000.dp), WindowPosition(10.dp, 10.dp)), screens)
        assertEquals(DpSize(1920.dp, 1080.dp), big.size)
        assertEquals(0f to 0f, at(big))
        val partly = WindowBounds.fit(WindowBounds.Bounds(DpSize(480.dp, 820.dp), WindowPosition(1700.dp, 600.dp)), screens)
        assertEquals("はみ出した分だけ寄せる", 1440f to 260f, at(partly))
        val same = WindowBounds.Bounds(DpSize(480.dp, 820.dp), WindowPosition(100.dp, 100.dp))
        assertEquals(100f to 100f, at(WindowBounds.fit(same, screens)))
        assertEquals(DpSize(480.dp, 820.dp), WindowBounds.fit(same, screens).size)
        assertEquals("画面が分からなければそのまま", same, WindowBounds.fit(same, emptyList()))
    }
}
