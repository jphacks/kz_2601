package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.io.DesktopIo
import jp.kz2601.metamon.desktop.io.DragOut
import jp.kz2601.metamon.desktop.io.FileNames
import jp.kz2601.metamon.desktop.io.PdfIo
import jp.kz2601.metamon.desktop.io.ImageSource
import jp.kz2601.metamon.desktop.store.DesktopStore
import jp.kz2601.metamon.desktop.store.JsonKeyValueStore
import jp.kz2601.metamon.desktop.store.MOVE_RETRIES
import jp.kz2601.metamon.desktop.store.sweepTempFiles
import jp.kz2601.metamon.desktop.store.writeAtomically
import jp.kz2601.metamon.dsl.CalendarEvent
import kotlinx.coroutines.runBlocking
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
import java.nio.file.FileSystemException
import java.nio.file.Files
import java.nio.file.StandardCopyOption
import java.util.concurrent.LinkedBlockingQueue
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
    fun concurrentPutsThroughOneStoreKeepEveryKeyAndLeaveNoTempFiles() {
        val dir = tmp.newFolder()
        val f = File(dir, "kv.json")
        val kv = JsonKeyValueStore(f)
        val pool = Executors.newFixedThreadPool(8)
        val start = CountDownLatch(1)
        val errors = java.util.Collections.synchronizedList(mutableListOf<Throwable>())
        repeat(40) { i ->
            pool.execute {
                start.await()
                runCatching { kv.putInt("k$i", i) }.onFailure { errors += it }
            }
        }
        start.countDown()
        pool.shutdown()
        assertTrue(pool.awaitTermination(30, TimeUnit.SECONDS))
        assertTrue(errors.toString(), errors.isEmpty())
        // 読み直しても 40 件そろう(最後の置き換えが全部を持つ)
        val reread = JsonKeyValueStore(f)
        assertEquals((0 until 40).map { "k$it" }.toSet(), reread.keys())
        assertEquals(39, reread.getInt("k39"))
        assertEquals("一時ファイルが残らない", listOf("kv.json"), dir.list()!!.toList())

        // 置き換えられなかった時も一時ファイルを残さない
        runCatching { writeAtomically(f, "{}", move = { _, to -> throw AccessDeniedException(to.toString()) }, pause = {}) }
        assertEquals(listOf("kv.json"), dir.list()!!.toList())
    }

    @Test
    fun replaceRetriesAnyFileSystemExceptionThenGivesUp() {
        val f = File(tmp.newFolder(), "kv.json")
        var calls = 0
        writeAtomically(f, "{\"a\":1}", move = { from, to ->
            calls++
            if (calls < 3) throw FileSystemException(to.toString(), null, "The process cannot access the file")
            Files.move(from, to, StandardCopyOption.REPLACE_EXISTING)
        }, pause = {})
        assertEquals(3, calls)
        assertEquals("{\"a\":1}", f.readText())

        calls = 0
        val failed = runCatching { writeAtomically(f, "{}", move = { _, to -> calls++; throw FileSystemException(to.toString()) }, pause = {}) }
        assertTrue(failed.exceptionOrNull() is FileSystemException)
        assertEquals(MOVE_RETRIES + 1, calls)
        assertEquals("{\"a\":1}", f.readText())
        assertEquals(listOf("kv.json"), f.parentFile.list()!!.toList())
    }

    @Test
    fun leftoverTempFilesAreRemovedOnOpenAndWipe() = runBlocking {
        val root = tmp.newFolder("store")
        val records = File(root, "records/shopping").apply { mkdirs() }
        File(root, "settings.json").writeText("{\"tailorit.lastAppId\": \"kakeibo\"}")
        val leftovers = listOf(
            File(root, "settings.json.123456789.tmp"),
            File(root, "consent.json.tmp"),
            File(records, "item.json.tmp"),
            File(records, "item.json.42.tmp"),
        ).onEach { it.writeText("half") }
        val keep = File(records, "item.json").apply { writeText("{}") }

        val store = DesktopStore.open(root)
        assertTrue(leftovers.none { it.exists() })
        assertTrue(keep.isFile)
        assertEquals("kakeibo", store.lastAppId)

        File(root, "settings.json.99.tmp").writeText("half")
        store.wipe()
        assertFalse(File(root, "settings.json.99.tmp").exists())
        assertEquals(0, sweepTempFiles(null))
    }

    // MARK: - 1b. 2 つ目の起動は起動中の方へ引き継ぐ

    @Test
    fun handOffDeliversArgsOnlyWithTheRightToken() {
        val dir = tmp.newFolder("handoff")
        val got = LinkedBlockingQueue<List<String>>()
        SingleInstance.Server.start(dir).use { server ->
            // 受け手を決める前に届いた分も、決めた時に渡す
            assertTrue(SingleInstance.handOff(dir, listOf("kakeibo", "", "home")))
            server.setHandler { got += it }
            assertEquals(listOf("kakeibo", "", "home"), got.poll(5, TimeUnit.SECONDS))
            assertTrue(SingleInstance.handOff(dir, emptyList()))
            assertEquals(emptyList<String>(), got.poll(5, TimeUnit.SECONDS))

            // 合言葉が違えば受け取らない(2 つ目は「起動中です」を出す側に戻る)
            val info = File(dir, SingleInstance.INSTANCE_FILE)
            val port = info.readLines().first()
            info.writeText("$port\nwrong\n")
            assertFalse(SingleInstance.handOff(dir, listOf("shopping")))
            assertNull(got.poll(300, TimeUnit.MILLISECONDS))
        }
        // 口が閉じている・ファイルが無い時も false(落ちない)
        assertFalse(SingleInstance.handOff(dir, emptyList()))
        assertFalse(SingleInstance.handOff(tmp.newFolder("none"), emptyList()))
    }

    @Test
    fun anotherProcessHoldsTheLockAndReceivesTheHandOff() {
        val dir = tmp.newFolder("two-process")
        val java = File(System.getProperty("java.home"), "bin/java").path
        val child = ProcessBuilder(java, "-cp", System.getProperty("java.class.path"), LockHolderMain::class.java.name, dir.absolutePath)
            .redirectErrorStream(true)
            .start()
        try {
            val out = child.inputStream.bufferedReader()
            assertEquals("claimed", out.readLine())
            assertNull("別のプロセスが持っている間は取れない", SingleInstance.acquire(dir))
            assertTrue(SingleInstance.handOff(dir, listOf("kakeibo")))
            assertEquals("got kakeibo", out.readLine())
            child.outputStream.close()
            assertTrue(child.waitFor(20, TimeUnit.SECONDS))
        } finally {
            child.destroyForcibly()
        }
        val again = SingleInstance.acquire(dir)
        assertNotNull("終わったら取れる", again)
        again!!.close()
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

        // 読んだ量で数える: 上限ちょうどは読み、1 バイトでも超えたら null
        val exact = tmp.newFile("exact.bin").apply { writeBytes(ByteArray(1000)) }
        assertEquals(1000, ImageSource.readFile(exact, 1000)?.size)
        assertNull(ImageSource.readFile(exact, 999))
        // 写真の値(file URI)も同じ上限
        val bigImage = sparse("big.png", ImageSource.MAX_BYTES + 1L)
        assertNull(ImageSource.bytes(bigImage.toURI().toString(), allowFile = true))
    }

    @Test
    fun dataUrlIsRejectedByLengthBeforeDecoding() {
        val ok = java.util.Base64.getEncoder().encodeToString(ByteArray(30))
        assertEquals(30, ImageSource.dataUrl("data:image/png;base64,$ok", max = 30)?.size)
        assertNull(ImageSource.dataUrl("data:image/png;base64,$ok", max = 29))
        // 解けない文字列でも長ければ解く前に断る(例外にならない)
        assertNull(ImageSource.dataUrl("data:image/png;base64," + "!".repeat(1000), max = 30))
        assertEquals(40L, ImageSource.base64Length(30))
        assertEquals(44L, ImageSource.base64Length(31))
        val huge = "data:image/png;base64," + "A".repeat((ImageSource.base64Length(ImageSource.MAX_BYTES) + 4).toInt())
        assertNull(ImageSource.bytes(huge, allowFile = false))
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
        assertTrue("一時ファイルが残らない", dir.list()!!.none { it.endsWith(".tmp") })

        // 描いている間に別の一言が出たら、描き終えても上書きしない
        run(Action(type = ActionType.EXPORT, entity = "expense", title = "支出", format = ActionFormat.PDF))
        s.message = "他の操作の結果"
        jobs.last()()
        assertEquals("他の操作の結果", s.message)
        assertTrue(File(dir, "支出 (2).pdf").isFile)
    }

    @Test
    fun calendarHandOffGoesToTheTempFolderNotDownloads() {
        val store = MockStore.fromJson("""{"expense": [{"id": "a", "data": {"amount": 1200, "category": "食費", "date": "2026-11-10"}}]}""")
        val dl = tmp.newFolder("dl")
        val cal = File(tmp.root, "cal")
        val opened = mutableListOf<File>()
        val s = DesktopSession(Examples.load("kakeibo"), store)
        s.host = DesktopActionHost(BookLookupClient.mock, dl, silent, calendarDir = cal, openFile = { opened += it; true })
        val add = Action(type = ActionType.ADD_TO_CALENDAR, entity = "expense", calendarEvent = CalendarEvent(startField = "date", titleField = "category"))
        repeat(2) { s.dispatch(add, ActionContext(s.top, null, recordId = "a")) }
        assertEquals(s.message, 2, opened.size)
        assertEquals("ダウンロードには残さない", 0, dl.list()!!.size)
        assertEquals("名前は同じ・(2) を付けない", listOf("食費.ics", "食費.ics"), opened.map { it.name })
        assertEquals("フォルダは別", 2, opened.map { it.parentFile }.toSet().size)
        assertTrue(opened.all { it.parentFile.parentFile == cal && it.parentFile.name.startsWith(DesktopActionHost.CALENDAR_PREFIX) })
        assertTrue(opened.first().readText().startsWith("BEGIN:VCALENDAR"))

        // 1 日より古い物は起動時に消す
        opened.first().parentFile.setLastModified(0)
        assertEquals(1, DesktopActionHost.sweepCalendar(root = cal))
        assertTrue(opened.last().isFile)
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
