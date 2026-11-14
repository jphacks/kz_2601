package jp.kz2601.metamon.desktop

import androidx.compose.ui.text.LinkAnnotation
import jp.kz2601.metamon.desktop.io.ExportClipboard
import jp.kz2601.metamon.desktop.io.ImageSource
import jp.kz2601.metamon.desktop.io.PdfIo
import jp.kz2601.metamon.desktop.io.Tsv
import jp.kz2601.metamon.desktop.net.DesktopApiConfig
import jp.kz2601.metamon.desktop.store.DesktopStore
import jp.kz2601.metamon.desktop.store.DeviceWipe
import jp.kz2601.metamon.desktop.store.SessionPersistence
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.logic.ExportTable
import jp.kz2601.metamon.dsl.logic.RecordImport
import jp.kz2601.metamon.dsl.records.entityModels
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.awt.Color
import java.awt.datatransfer.Clipboard
import java.awt.datatransfer.DataFlavor
import java.awt.datatransfer.StringSelection
import java.awt.image.BufferedImage
import java.io.ByteArrayOutputStream
import java.io.DataOutputStream
import java.io.File
import java.io.RandomAccessFile
import java.nio.file.Files
import java.util.Base64
import java.util.zip.CRC32
import javax.imageio.ImageIO

class LinksFilesWipeTest {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    @After
    fun tearDown() = scope.cancel()

    // 1. Markdown のリンクは openUrl の操作へ(既定の UriHandler で開かない)・開けない URL は文字だけ
    @Test
    fun markdownLinksGoThroughOpenUrl() {
        val opened = mutableListOf<String>()
        val links = Markdown.Links(null) { opened += it }
        val s = Markdown.inline("見て [ここ](https://example.com/a) と [悪い](javascript:alert`1`) と [ファイル](file:///etc/passwd)", links)
        assertEquals("見て ここ と 悪い と ファイル", s.text)
        val anns = s.getLinkAnnotations(0, s.length)
        assertEquals("通る URL だけリンク", 1, anns.size)
        val ann = anns.single().item
        assertTrue("LinkAnnotation.Url は使わない", ann is LinkAnnotation.Clickable)
        assertEquals("ここ", s.text.substring(anns.single().start, anns.single().end))
        (ann as LinkAnnotation.Clickable).linkInteractionListener!!.onClick(ann)
        assertEquals(listOf("https://example.com/a"), opened)
        // 押し口が無い時はリンクにしない
        assertTrue(Markdown.inline("[a](https://example.com)").getLinkAnnotations(0, 1).isEmpty())

        val action = markdownLinkAction("https://example.com/a")
        assertEquals(ActionType.OPEN_URL, action.type)
        assertEquals("https://example.com/a", action.url)
        assertTrue("行き先を見せて確かめる", action.confirmMessage!!.contains("https://example.com/a"))
    }

    // 2. TSV の見出し・セルにも CSV と同じ式よけ
    @Test
    fun tsvGuardsHeadersAndCells() {
        val tsv = Tsv.string(ExportTable("t", listOf("=HYPERLINK(1)", "名前"), listOf(listOf("+1", "@x"))))
        val lines = tsv.split("\r\n")
        assertEquals("'=HYPERLINK(1)\t名前", lines[0])
        assertEquals("'+1\t'@x", lines[1])
    }

    // 3. PDF: 大きさは開く前に・読むのは時間の上限つき・何が投げられても読めない扱い
    @Test
    fun pdfIsSizeCheckedAndParsedWithATimeout() {
        val big = File.createTempFile("big-", ".pdf").apply { deleteOnExit() }
        RandomAccessFile(big, "rw").use { it.setLength(RecordImport.MAX_PDF_BYTES + 1L) }
        assertEquals(PdfIo.TextResult.TooLarge, PdfIo.text(big))
        big.delete()

        val started = System.nanoTime()
        assertEquals(PdfIo.TextResult.Broken, PdfIo.parse(200) { Thread.sleep(10_000); error("unreachable") })
        assertTrue("待ち続けない", (System.nanoTime() - started) / 1_000_000 < 5_000)
        assertEquals(PdfIo.TextResult.Broken, PdfIo.parse(5_000) { throw OutOfMemoryError("x") })
        assertEquals(PdfIo.TextResult.Broken, PdfIo.parse(5_000) { throw StackOverflowError() })

        val pdf = File.createTempFile("ok-", ".pdf").apply { deleteOnExit(); writeBytes(PdfIo.data(ExportTable("t", listOf("a"), listOf(listOf("1"))))) }
        assertEquals("ファイルから開ける(画像だけの PDF)", PdfIo.TextResult.NoText, PdfIo.text(pdf))
        pdf.delete()
    }

    private fun chunk(out: DataOutputStream, type: String, data: ByteArray) {
        out.writeInt(data.size)
        val body = type.toByteArray(Charsets.US_ASCII) + data
        out.write(body)
        out.writeInt(CRC32().apply { update(body) }.value.toInt())
    }

    /** 見出しだけ [w]×[h] の PNG(中身は空)。 */
    private fun pngHeader(w: Int, h: Int): ByteArray {
        val bytes = ByteArrayOutputStream()
        val out = DataOutputStream(bytes)
        out.write(byteArrayOf(0x89.toByte(), 'P'.code.toByte(), 'N'.code.toByte(), 'G'.code.toByte(), 0x0D, 0x0A, 0x1A, 0x0A))
        val ihdr = ByteArrayOutputStream().also { b -> DataOutputStream(b).apply { writeInt(w); writeInt(h); write(byteArrayOf(8, 0, 0, 0, 0)) } }.toByteArray()
        chunk(out, "IHDR", ihdr)
        chunk(out, "IDAT", byteArrayOf(0x78, 0x9C.toByte(), 0x03, 0x00, 0x00, 0x00, 0x00, 0x01))
        chunk(out, "IEND", ByteArray(0))
        return bytes.toByteArray()
    }

    private fun encoded(w: Int, h: Int, format: String = "png"): ByteArray {
        val img = BufferedImage(w, h, BufferedImage.TYPE_INT_RGB)
        img.createGraphics().apply { color = Color.BLUE; fillRect(0, 0, w, h) }.dispose()
        return ByteArrayOutputStream().also { ImageIO.write(img, format, it) }.toByteArray()
    }

    // 4. 画像: 縦横を先に読んで大きすぎる物は開かない・描く絵は必ず縮める
    @Test
    fun hugeImagesAreRefusedBeforeDecoding() {
        val huge = pngHeader(10_000, 5_000)
        assertEquals(10_000 to 5_000, ImageSource.dimensions(huge))
        assertFalse(ImageSource.withinPixelLimit(huge))
        assertNull(ImageLoader.decode(huge))
        assertNull(ImageLoader.decode(huge, maxPx = ImageLoader.THUMBNAIL_PX, withPng = false))
        val smallJpg = encoded(8, 8, "jpg")
        assertTrue(ImageSource.withinPixelLimit(smallJpg))
        assertFalse("読めない物は開かない", ImageSource.withinPixelLimit("not an image".toByteArray()))

        val wide = ImageLoader.decode(encoded(2_000, 10), withPng = false)!!
        assertEquals("maxPx を渡さなくても縮める", ImageLoader.DISPLAY_PX, wide.bitmap.width)
    }

    // 5. 記録の https の画像は、アプリの置き場の外なら押すまで読まない
    @Test
    fun externalRecordImagesNeedAClick() {
        val hosts = ImageLoader.hostsFrom(mapOf("TAILORIT_MEDIA_HOSTS" to "media.example.jp, cdn.example.jp"))
        assertTrue(java.net.URI(DesktopApiConfig.STG_APPCONFIG_URL).host in hosts)
        assertTrue("media.example.jp" in hosts)
        assertTrue(ImageLoader.needsConsent("https://elsewhere.example.net/p.png", fromRecord = true, hosts = hosts))
        assertTrue("ホストの無い URL も", ImageLoader.needsConsent("https:///x.png", fromRecord = true, hosts = hosts))
        assertFalse(ImageLoader.needsConsent("https://media.example.jp/p.png", fromRecord = true, hosts = hosts))
        assertFalse(ImageLoader.needsConsent("https://MEDIA.example.jp/p.png", fromRecord = true, hosts = hosts))
        assertFalse("DSL に書かれた画像はそのまま", ImageLoader.needsConsent("https://elsewhere.example.net/p.png", fromRecord = false, hosts = hosts))
        assertFalse(ImageLoader.needsConsent("data:image/png;base64,AA==", fromRecord = true, hosts = hosts))

        ImageLoader.clear()
        val src = "https://elsewhere.invalid/p.png"
        assertTrue(ImageLoader.blocked(src, fromRecord = true))
        assertNull("押す前は通信しない", ImageLoader.load(src, allowFile = true))
        ImageLoader.approve(src)
        assertFalse(ImageLoader.blocked(src, fromRecord = true))
        ImageLoader.clear()
        assertTrue("消したら押し直し", ImageLoader.blocked(src, fromRecord = true))
    }

    // 6. ics: 改行の形をそろえ、制御文字を落とし、75 オクテットで折る
    @Test
    fun icsEscapesAndFolds() {
        assertEquals("a\\nb\\nc\\nd\\ne\\nf", Ics.escape("a\r\nb\rc\nd e f"))
        assertEquals("xy\tz", Ics.escape("x\u0000\u0007y\u001B\tz\u007F"))
        assertEquals("a\\;b\\,c\\\\d", Ics.escape("a;b,c\\d"))
        assertFalse("CR・LF が値に残らない", Ics.escape("SUMMARY\r\nEND:VEVENT").contains('\n'))

        fun check(line: String) {
            val folded = Ics.fold(line)
            folded.split("\r\n").forEachIndexed { i, l ->
                assertTrue("75 オクテットまで: $l", l.toByteArray(Charsets.UTF_8).size <= 75)
                if (i > 0) assertTrue(l.startsWith(" "))
            }
            assertEquals("戻せる", line, folded.replace("\r\n ", ""))
        }
        check("DESCRIPTION:" + "a".repeat(200))
        check("SUMMARY:" + "あ".repeat(60))
        check("SUMMARY:" + "😀".repeat(40))
        assertEquals("短い行はそのまま", "SUMMARY:x", Ics.fold("SUMMARY:x"))

        val ics = Ics.event(
            jp.kz2601.metamon.dsl.logic.CalendarEventDraft("長い" + "題".repeat(50), java.time.Instant.EPOCH, java.time.Instant.EPOCH, false, null, "1行目\r\n2行目", null),
            "u",
            java.time.Instant.EPOCH,
        )
        ics.removeSuffix("\r\n").split("\r\n").forEach { assertTrue(it.toByteArray(Charsets.UTF_8).size <= 75) }
        assertTrue(ics.contains("DESCRIPTION:1行目\\n2行目"))
    }

    // 7. この端末のデータを消す: 残り物・一時フォルダ・通知・画像・クリップボード
    @Test
    fun wipeCleansLeftovers() {
        val root = Files.createTempDirectory("wipe-root-").toFile()
        val records = File(root, "records/app").apply { mkdirs() }
        val corrupt = File(root, "settings.json.corrupt").apply { writeText("{") }
        val tmp = File(records, "e.json.tmp").apply { writeText("[]") }
        val keep = File(root, "windows.json").apply { writeText("{}") }
        val photos = Files.createTempDirectory("wipe-photos-").toFile()
        val drag = Files.createTempDirectory("wipe-drag-").toFile()
        File(photos, "pick-1.png").writeText("x")
        File(drag, "drag-1").apply { mkdirs(); File(this, "rows.csv").writeText("a") }

        val clipboard = Clipboard("test")
        assertTrue(ExportClipboard.put("名前\t学年\r\n山田\t1\r\n", clipboard))
        Notifier.Tray.arm("wipe-test", 60_000) {}
        assertTrue(Notifier.Tray.pendingCount() >= 1)
        ImageLoader.load("data:image/png;base64," + Base64.getEncoder().encodeToString(encoded(4, 4)), allowFile = false)
        assertTrue(ImageLoader.size() >= 1)

        val r = DeviceWipe.run(root, listOf(photos, drag), clipboard)
        assertEquals(2, r.strayFiles)
        assertFalse(corrupt.exists())
        assertFalse(tmp.exists())
        assertTrue("他のファイルは残す", keep.exists())
        assertTrue(photos.isDirectory && photos.listFiles()!!.isEmpty())
        assertTrue(drag.isDirectory && drag.listFiles()!!.isEmpty())
        assertTrue(r.timers >= 1)
        assertEquals(0, Notifier.Tray.pendingCount())
        assertEquals(0, ImageLoader.size())
        assertTrue(r.clipboardCleared)
        assertEquals("", clipboard.getData(DataFlavor.stringFlavor))

        // 利用者が別の物をコピーした後なら触らない
        ExportClipboard.put("表", clipboard)
        clipboard.setContents(StringSelection("自分のメモ"), null)
        assertFalse(DeviceWipe.run(null, emptyList(), clipboard).clipboardCleared)
        assertEquals("自分のメモ", clipboard.getData(DataFlavor.stringFlavor))

        root.deleteRecursively()
        photos.deleteRecursively()
        drag.deleteRecursively()
    }

    // 7. 消している間は他のウィンドウの書き換えを受けず、積んだ書き込みを流し終えてから消す
    @Test
    fun wipeBlocksEditsAndDrainsWritesFirst() = runBlocking {
        val store = DesktopStore(null)
        val mock = DesktopApiConfig.fromEnvironment(emptyMap())
        val order = mutableListOf<String>()
        val hub = AppHub(
            openRecords = { id, dsl ->
                val p = SessionPersistence.create(store, id, mock)
                val models = dsl.entityModels()
                SharedRecords.opening(p, scope) { p.load(models) }
            },
            wipeStore = { order += "store:${store.records.rows("shopping", "groceries").size}"; store.wipe() },
        )
        val app = hub.app("shopping").also { withTimeout(5_000) { it.records.awaitLoaded() } }
        val models = app.dsl.entityModels()

        app.records.beginWipe()
        app.records.update(models, MockStore().save("groceries", null, mapOf("name" to "牛乳")))
        assertTrue("消している間は受けない", app.records.store.list("groceries").isEmpty())
        app.records.endWipe()

        // 書いてすぐ消しても、書き込みが消した後に残らない
        app.records.update(models, MockStore().save("groceries", null, mapOf("name" to "卵")))
        hub.wipeLocalData()
        assertEquals(listOf("store:0"), order)
        assertTrue(store.records.rows("shopping", "groceries").isEmpty())
        assertTrue(app.records.store.rows.isEmpty())
        assertFalse(app.records.wiping)
        app.records.update(models, MockStore().save("groceries", null, mapOf("name" to "パン")))
        assertEquals("消し終えたら書ける", 1, app.records.store.list("groceries").size)
    }
}
