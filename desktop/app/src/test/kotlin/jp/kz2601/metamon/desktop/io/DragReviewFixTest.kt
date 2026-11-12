package jp.kz2601.metamon.desktop.io

import jp.kz2601.metamon.desktop.Examples
import jp.kz2601.metamon.desktop.ImageLoader
import jp.kz2601.metamon.desktop.MockStore
import jp.kz2601.metamon.desktop.RowSelect
import jp.kz2601.metamon.desktop.RowSelection
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.records.FieldCoding
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test
import java.awt.Color
import java.awt.datatransfer.DataFlavor
import java.awt.image.BufferedImage
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.File
import java.nio.file.Files
import java.time.ZoneId
import java.util.Base64
import java.util.TimeZone
import javax.imageio.ImageIO

/** 行の選び・外への引き出しのレビュー指摘(#1919・#1921 の後)。 */
class DragReviewFixTest {
    init {
        TimeZone.setDefault(TimeZone.getTimeZone("Asia/Tokyo"))
        FieldCoding.zone = ZoneId.of("Asia/Tokyo")
    }

    private val store = MockStore()
        .save("member", null, mapOf("name" to "山田 太郎", "joined" to "2026-04-10"))
        .save("member", null, mapOf("name" to "鈴木 一郎", "joined" to "2026-05-01"))
        .save("member", null, mapOf("name" to "佐藤 花子", "joined" to "2025-04-01"))
    private val io = DesktopIo(Examples.load("file_import"), { store }, {})
    private val photo = Component(type = ComponentType.PHOTO_FIELD, field = "photo")
    private val list = Component(type = ComponentType.LIST, entity = "member")

    private fun encoded(format: String, w: Int = 6, h: Int = 4): ByteArray {
        val img = BufferedImage(w, h, BufferedImage.TYPE_INT_RGB)
        img.createGraphics().apply { color = Color.RED; fillRect(0, 0, w, h) }.dispose()
        return ByteArrayOutputStream().also { ImageIO.write(img, format, it) }.toByteArray()
    }

    @Suppress("UNCHECKED_CAST")
    private fun files(t: java.awt.datatransfer.Transferable): List<File> = t.getTransferData(DataFlavor.javaFileListFlavor) as List<File>

    // 1. 引き出した一時ファイルを自分のフォームに落としても、終わる時に消える場所を指さない
    @Test
    fun droppingADraggedTempImageKeepsAPersistentCopy() {
        val png = encoded("png")
        val temp = files(DragOut.imageOfPng(png, "レシート")).single()
        assertTrue(DragOut.isTemp(temp))
        val draft = mutableMapOf<String, String>()
        assertTrue(io.dropFiles(listOf(temp), listOf(photo), draft))
        val kept = fileOf(draft["photo"]!!)!!
        assertFalse("一時フォルダの外", DragOut.isTemp(kept))
        assertTrue(kept.name.endsWith(".png"))
        assertArrayEquals(png, kept.readBytes())
        temp.delete()
        assertTrue("元が消えても残る", kept.isFile)
        kept.delete()

        // 外から落としたファイルはそのまま(写さない)
        val outside = File.createTempFile("photo-", ".png").apply { writeBytes(png); deleteOnExit() }
        assertFalse(DragOut.isTemp(outside))
        val d2 = mutableMapOf<String, String>()
        assertTrue(io.dropFiles(listOf(outside), listOf(photo), d2))
        assertEquals(outside.toURI().toString(), d2["photo"])
    }

    // 2. アプリの中で引いた行を落とし直しても取り込みが始まらない
    @Test
    fun internalRowDropIsRejected() {
        val ids = store.list("member").map { it.id }
        val rows = io.rowsTransferable("member", listOf(ids[0]))!!
        assertTrue(InternalDrag.isInternal(rows))
        assertFalse(io.drop(rows, listOf(list), mutableMapOf()))
        assertNull("取り込みの確認は出ない", io.pending)
        assertFalse("画像は印を持たない(フォームに落とせる)", InternalDrag.isInternal(DragOut.imageOfPng(encoded("png"), "x")))
        assertFalse(InternalDrag.isInternal(FileTransferable(emptyList())))
        assertFalse(InternalDrag.isInternal(null))

        // 外から落とした CSV は今までどおり取り込みの確認
        val csv = File.createTempFile("members-", ".csv").apply { writeText("名前\n田中\n"); deleteOnExit() }
        assertTrue(io.drop(FileTransferable(listOf(csv)), listOf(list), mutableMapOf()))
        assertNotNull(io.pending)
    }

    // 3. PNG は読んだ時に作っておき、引く時は変換しない。画像そのものもその PNG から
    @Test
    fun pngIsPreparedAtLoadAndImageFlavorComesFromIt() {
        val loaded = ImageLoader.decode(encoded("jpg"))!!
        val png = loaded.png!!
        assertEquals("\u0089PNG", String(png.copyOfRange(0, 4), Charsets.ISO_8859_1))
        val t = DragOut.imageOfPng(png, "写真")
        assertSame("変換せずそのまま", png, t.png)
        val img = t.getTransferData(DataFlavor.imageFlavor) as BufferedImage
        assertEquals(6, img.width)
        assertSame("2 度目は作り直さない", img, t.getTransferData(DataFlavor.imageFlavor))
        assertNull("引き出さない画像は PNG にしない", ImageLoader.decode(encoded("jpg"), withPng = false)!!.png)
        files(t).single().delete()
    }

    // 4. 一覧の写真: 大きさの上限・縮めた絵・URL ごとの小さなキャッシュ
    @Test
    fun thumbnailsAreCappedDownscaledAndCached() {
        assertNull("上限を超えたら読まない", ImageSource.capped(ByteArrayInputStream(ByteArray(11)), max = 10))
        assertEquals(10, ImageSource.capped(ByteArrayInputStream(ByteArray(10)), max = 10)!!.size)
        val huge = "data:image/png;base64," + Base64.getEncoder().encodeToString(ByteArray(ImageSource.MAX_BYTES + 1))
        assertNull(ImageSource.bytes(huge, allowFile = false))

        val big = ImageLoader.decode(encoded("png", 1000, 500), maxPx = ImageLoader.THUMBNAIL_PX)!!
        assertEquals(ImageLoader.THUMBNAIL_PX, big.bitmap.width)
        assertEquals(ImageLoader.THUMBNAIL_PX / 2, big.bitmap.height)
        assertEquals("小さい画像はそのまま", 6, ImageLoader.decode(encoded("png"), maxPx = ImageLoader.THUMBNAIL_PX)!!.bitmap.width)
        val full = ImageLoader.decode(encoded("png", 1000, 500), maxPx = ImageLoader.THUMBNAIL_PX)!!.png!!
        assertEquals("引き出すのは元の大きさ", 1000, ImageIO.read(ByteArrayInputStream(full)).width)

        ImageLoader.clear()
        fun url(i: Int) = "data:image/png;base64," + Base64.getEncoder().encodeToString(encoded("png", 2 + i, 2))
        val first = ImageLoader.load(url(0), allowFile = false, maxPx = ImageLoader.THUMBNAIL_PX)!!
        assertSame("同じ URL は覚えた物", first, ImageLoader.load(url(0), allowFile = false, maxPx = ImageLoader.THUMBNAIL_PX))
        assertSame(first, ImageLoader.cached(url(0), allowFile = false, maxPx = ImageLoader.THUMBNAIL_PX))
        for (i in 1..ImageLoader.CACHE_SIZE) ImageLoader.load(url(i), allowFile = false, maxPx = ImageLoader.THUMBNAIL_PX)
        assertEquals(ImageLoader.CACHE_SIZE, ImageLoader.size())
        assertNull("古い物から捨てる", ImageLoader.cached(url(0), allowFile = false, maxPx = ImageLoader.THUMBNAIL_PX))

        val f = File.createTempFile("photo-", ".png").apply { writeBytes(encoded("png")); deleteOnExit() }
        assertNotNull(ImageLoader.load(f.toURI().toString(), allowFile = true))
        assertNull("手元のファイルは覚えない", ImageLoader.cached(f.toURI().toString(), allowFile = true, maxPx = null))
        ImageLoader.clear()
    }

    // 5. 一覧のコピー・引き出しは画面の並び
    @Test
    fun listCopyAndDragFollowTheScreenOrder() {
        val ids = store.list("member").map { it.id }
        val screen = listOf(ids[1], ids[2], ids[0]) // 画面で並べ替えた後
        val sel = RowSelection(selected = setOf(ids[0], ids[1]), ids = ids.toSet())
        val dragged = RowSelect.dragged(sel, ids[0], screen)
        assertEquals(listOf(ids[1], ids[0]), dragged)
        assertEquals(sel.inOrder(screen), dragged)
        assertEquals(listOf(ids[2]), RowSelect.dragged(sel, ids[2], screen))

        val tsv = io.rowsTransferable("member", dragged)!!.getTransferData(DataFlavor.stringFlavor) as String
        val names = tsv.split("\r\n").drop(1).filter { it.isNotEmpty() }.map { it.substringBefore('\t') }
        assertEquals("作った順でも古い順でもなく画面の並び", listOf("鈴木 一郎", "山田 太郎"), names)
    }

    // 6. 残った一時フォルダの片付けと Windows の予約名
    @Test
    fun oldDragFoldersAreSweptAndReservedNamesPrefixed() {
        val root = Files.createTempDirectory("drag-root-").toFile().apply { deleteOnExit() }
        val now = System.currentTimeMillis()
        val day = 24L * 60 * 60 * 1000
        val old = File(root, "drag-old").apply { mkdirs(); File(this, "a.png").writeText("x"); setLastModified(now - 2 * day) }
        val fresh = File(root, "drag-new").apply { mkdirs(); setLastModified(now - day / 2) }
        val other = File(root, "keep-me").apply { mkdirs(); setLastModified(now - 3 * day) }
        assertEquals(1, DragOut.sweep(now, root = root))
        assertFalse(old.exists())
        assertTrue(fresh.exists())
        assertTrue("drag- 以外は触らない", other.exists())
        root.deleteRecursively()

        assertEquals("_CON", DragOut.fileName("CON", "rows"))
        assertEquals("_con", DragOut.fileName("con", "rows"))
        assertEquals("_nul.backup", DragOut.fileName("nul.backup", "rows"))
        assertEquals("_Com1", DragOut.fileName("Com1", "rows"))
        assertEquals("_lpt9", DragOut.fileName("lpt9", "rows"))
        assertEquals("_aux", DragOut.fileName("aux.", "rows"))
        assertEquals("CONSOLE", DragOut.fileName("CONSOLE", "rows"))
        assertEquals("COM10", DragOut.fileName("COM10", "rows"))
        assertEquals("部員", DragOut.fileName("部員", "rows"))
    }
}
