package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.io.DesktopIo
import jp.kz2601.metamon.desktop.io.DragOut
import jp.kz2601.metamon.desktop.io.FileNames
import jp.kz2601.metamon.desktop.io.PdfIo
import jp.kz2601.metamon.desktop.store.writeAtomically
import jp.kz2601.metamon.dsl.Accept
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionFormat
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Notification
import jp.kz2601.metamon.dsl.logic.RecordImport
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File
import java.io.RandomAccessFile
import java.nio.file.AccessDeniedException
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import kotlin.random.Random

/** 起動の重複・一時ファイル・長いセル・大きいファイル・書き出しの名前・画面の無い印刷。 */
class RobustnessTest {
    @get:Rule
    val tmp = TemporaryFolder()

    // MARK: - 1. 2 つ目を起動しない・一時ファイルの名前

    @Test
    fun secondInstanceCannotTakeTheLockUntilTheFirstReleases() {
        val dir = tmp.newFolder("data")
        val first = SingleInstance.acquire(dir)
        assertNotNull(first)
        assertTrue(File(dir, SingleInstance.LOCK_FILE).isFile)
        assertNull("持っている間は取れない", SingleInstance.acquire(dir))
        first!!.close()
        val again = SingleInstance.acquire(dir)
        assertNotNull("離したら取れる", again)
        again!!.close()
        // 別の保存先なら同時に取れる
        val a = SingleInstance.acquire(tmp.newFolder("a"))
        val b = SingleInstance.acquire(tmp.newFolder("b"))
        assertNotNull(a)
        assertNotNull(b)
        a!!.close()
        b!!.close()
    }

    @Test
    fun lockThatCannotBeCreatedDoesNotBlockStartup() {
        // 保存先がファイル(フォルダを作れない)でも起動は止めない
        val notADir = tmp.newFile("not-a-dir")
        assertNotNull(SingleInstance.acquire(notADir))
    }

    @Test
    fun concurrentAtomicWritesUseTheirOwnTempFilesAndLeaveNoneBehind() {
        val dir = tmp.newFolder()
        val f = File(dir, "kv.json")
        val pool = Executors.newFixedThreadPool(8)
        val start = CountDownLatch(1)
        val errors = java.util.Collections.synchronizedList(mutableListOf<Throwable>())
        repeat(40) { i ->
            pool.execute {
                start.await()
                runCatching { writeAtomically(f, "{\"n\":$i}") }.onFailure { errors += it }
            }
        }
        start.countDown()
        pool.shutdown()
        assertTrue(pool.awaitTermination(30, TimeUnit.SECONDS))
        // Windows の置き換えの失敗(AccessDenied)は試し直すが、一時ファイルの取り合いでは落ちない
        assertTrue(errors.toString(), errors.none { it !is AccessDeniedException })
        assertTrue(Regex("""\{"n":\d+}""").matches(f.readText()))
        assertEquals("一時ファイルが残らない", listOf("kv.json"), dir.list()!!.toList())

        // 置き換えられなかった時も一時ファイルを残さない
        runCatching { writeAtomically(f, "{}", move = { _, to -> throw AccessDeniedException(to.toString()) }, pause = {}) }
        assertEquals(listOf("kv.json"), dir.list()!!.toList())
    }

    // MARK: - 2. 長いセルを速く切る

    @Test
    fun clipMatchesTheLinearCutAndMeasuresFewTimes() {
        fun linear(s: String, width: Int, measure: (String) -> Int): String {
            if (measure(s) <= width) return s
            var end = s.length
            while (end > 0 && measure(s.substring(0, end) + "…") > width) end--
            return s.substring(0, end) + "…"
        }
        val rnd = Random(7)
        repeat(200) {
            val s = (0 until rnd.nextInt(0, 60)).map { "aあW."[rnd.nextInt(4)] }.joinToString("")
            val w = rnd.nextInt(0, 80)
            val measure = { t: String -> t.sumOf { c -> if (c.code > 0x7f) 2 else 1 } }
            assertEquals("$s / $w", linear(s, w, measure), PdfIo.clip(s, w, measure))
        }
        // 10 万字のセルでも測るのは数十回まで、長さも上限まで
        var calls = 0
        val huge = "x".repeat(100_000)
        val out = PdfIo.clip(huge, 50) { calls++; it.length }
        assertEquals("x".repeat(49) + "…", out)
        assertTrue("測った回数 $calls", calls <= 12)
        // 幅に余裕があっても上限を超えた分は「…」
        val wide = PdfIo.clip(huge, 1_000_000) { it.length }
        assertEquals(PdfIo.CLIP_MAX_CHARS + 1, wide.length)
        assertTrue(wide.endsWith("…"))
        // サロゲートペアの間では切らない
        val emoji = "😀😀😀"
        assertEquals("😀…", PdfIo.clip(emoji, 4) { it.length })
    }

    // MARK: - 3. 大きすぎるファイルは読む前に断る

    private fun sparse(name: String, size: Long): File = tmp.newFile(name).also { RandomAccessFile(it, "rw").use { r -> r.setLength(size) } }

    @Test
    fun oversizedCsvTextAndPdfAreRejectedBeforeReading() {
        var updates = 0
        val io = DesktopIo(Examples.load("file_import"), { MockStore() }, { updates++ })
        val csvImport = Component(type = ComponentType.FILE_IMPORT, entity = "member", accept = listOf(Accept.CSV))
        val docImport = Component(type = ComponentType.FILE_IMPORT, entity = "doc", field = "body", accept = listOf(Accept.PDF, Accept.TEXT))
        val tooLarge = jp.kz2601.metamon.desktop.i18n.Strings["desktop_io_file_too_large"]

        io.importFile(sparse("big.csv", RecordImport.MAX_TEXT_BYTES + 1L), csvImport)
        assertEquals(tooLarge, io.notice)
        assertNull(io.pending)

        io.notice = null
        io.importFile(sparse("big.txt", RecordImport.MAX_TEXT_BYTES + 1L), docImport)
        assertEquals(tooLarge, io.notice)

        io.notice = null
        io.importFile(sparse("big.pdf", RecordImport.MAX_PDF_BYTES + 1L), docImport)
        assertEquals(tooLarge, io.notice)
        assertEquals(0, updates)

        // 一覧に落とした CSV も同じ
        io.notice = null
        val dropped = sparse("dropped.csv", RecordImport.MAX_TEXT_BYTES + 1L)
        io.dropFiles(listOf(dropped), listOf(Component(type = ComponentType.LIST, entity = "member")), null)
        assertEquals(tooLarge, io.notice)
        assertNull(io.pending)

        // 上限ちょうどまでは読む(中身が CSV でなければ別の一言)
        assertFalse(DesktopIo.tooLarge(sparse("ok.csv", RecordImport.MAX_TEXT_BYTES.toLong()), RecordImport.MAX_TEXT_BYTES))
    }

    // MARK: - 4. 書き出しは上書きしない・Windows の予約名

    @Test
    fun reservedWindowsNamesArePrefixed() {
        for (n in listOf("CON", "con.csv", "PRN.ics", "AUX", "NUL.tar.gz", "COM1", "com9.pdf", "LPT1", "COM0", "COM¹", "COM²", "LPT³.csv", "CONIN$", "conout$.csv", "CON .csv")) {
            assertTrue(n, FileNames.isReserved(n))
            assertEquals("_$n", FileNames.guard(n))
        }
        for (n in listOf("CONSOLE", "COM10", "LPT", "部員.csv", "CONIN", "_CON", "a.CON")) {
            assertFalse(n, FileNames.isReserved(n))
            assertEquals(n, FileNames.guard(n))
        }
        assertEquals("_COM¹", DragOut.fileName("COM¹", "rows"))
        assertEquals("_CONOUT$", DragOut.fileName("CONOUT$", "rows"))
    }

    @Test
    fun createFreePicksTheNextNumberAndNeverOverwrites() {
        val dir = File(tmp.root, "Downloads")
        val a = FileNames.createFree(dir, "支出.csv").apply { writeText("1") }
        val b = FileNames.createFree(dir, "支出.csv").apply { writeText("2") }
        val c = FileNames.createFree(dir, "支出.csv")
        assertEquals(listOf("支出.csv", "支出 (2).csv", "支出 (3).csv"), listOf(a, b, c).map { it.name })
        assertEquals("1", a.readText())
        assertEquals("_CON (2).ics", FileNames.createFree(dir, "CON.ics").let { FileNames.createFree(dir, "CON.ics") }.name)
        assertEquals("README (2)", FileNames.createFree(dir, "README").let { FileNames.createFree(dir, "README") }.name)
    }

    private val silent = object : Notifier {
        override fun schedule(n: Notification): String = ""
        override fun cancel(id: String): String? = null
    }

    @Test
    fun exportKeepsEarlierFilesAndRendersPdfInTheBackground() {
        val store = MockStore.fromJson("""{"expense": [{"id": "a", "data": {"amount": 1200, "category": "食費", "date": "2026-11-10"}}]}""")
        val dir = tmp.newFolder("dl")
        val jobs = mutableListOf<() -> Unit>()
        val s = DesktopSession(Examples.load("kakeibo"), store)
        s.host = DesktopActionHost(BookLookupClient.mock, dir, silent, background = { jobs += it })
        fun run(a: Action) = s.dispatch(a, ActionContext(s.top, s.form(s.top)))

        File(dir, "支出.csv").writeText("前の書き出し")
        run(Action(type = ActionType.EXPORT, entity = "expense", title = "支出"))
        assertEquals("前の書き出し", File(dir, "支出.csv").readText())
        assertTrue(s.message.orEmpty(), File(dir, "支出 (2).csv").isFile)
        assertTrue(s.message!!.contains("支出 (2).csv"))

        run(Action(type = ActionType.EXPORT, entity = "expense", title = "CON"))
        assertTrue(File(dir, "_CON.csv").isFile)

        // PDF は頁を描くので裏で。押した時は「書き出しています」、描き終えたら場所を出す
        run(Action(type = ActionType.EXPORT, entity = "expense", title = "支出", format = ActionFormat.PDF))
        assertEquals(jp.kz2601.metamon.desktop.i18n.Strings["desktop_export_writing"], s.message)
        assertEquals(1, jobs.size)
        jobs.single()()
        val pdf = File(dir, "支出.pdf")
        assertEquals("%PDF", pdf.readBytes().copyOfRange(0, 4).decodeToString())
        assertTrue(s.message!!.contains(pdf.absolutePath))
    }

    // MARK: - 5. 画面の無い DSL では印刷しない

    @Test
    fun printWithoutScreensIsDisabled() {
        val dsl = Examples.load("kakeibo")
        assertTrue(DesktopPrint.canPrint(dsl))
        val empty = dsl.copy(screens = emptyList())
        assertFalse(DesktopPrint.canPrint(empty))
        assertNull(ScreenPrint.document(empty, "home", MockStore()))
        assertNotNull("無い画面 id は最初の画面", ScreenPrint.document(dsl, "nope", MockStore()))
    }
}
