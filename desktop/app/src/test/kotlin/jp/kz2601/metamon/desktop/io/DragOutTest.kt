package jp.kz2601.metamon.desktop.io

import jp.kz2601.metamon.desktop.Examples
import jp.kz2601.metamon.desktop.MockStore
import jp.kz2601.metamon.desktop.RowSelect
import jp.kz2601.metamon.desktop.RowSelection
import jp.kz2601.metamon.desktop.TableCells
import jp.kz2601.metamon.desktop.TableCopy
import jp.kz2601.metamon.dsl.logic.CsvWriter
import jp.kz2601.metamon.dsl.records.FieldCoding
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.awt.Color
import java.awt.datatransfer.DataFlavor
import java.awt.datatransfer.Transferable
import java.awt.datatransfer.UnsupportedFlavorException
import java.awt.image.BufferedImage
import java.io.ByteArrayOutputStream
import java.io.File
import java.nio.file.FileSystems
import java.nio.file.Files
import java.nio.file.attribute.PosixFilePermissions
import java.time.ZoneId
import java.util.Base64
import java.util.TimeZone
import javax.imageio.ImageIO

/** 外への引き出し(#1764): 行は TSV の文字 + CSV のファイル、画像は PNG のファイル。 */
class DragOutTest {
    init {
        TimeZone.setDefault(TimeZone.getTimeZone("Asia/Tokyo"))
        FieldCoding.zone = ZoneId.of("Asia/Tokyo")
    }

    private val store = MockStore()
        .save("member", null, mapOf("name" to "山田 太郎", "grade" to "2", "part" to "前衛", "joined" to "2026-04-10"))
        .save("member", null, mapOf("name" to "鈴木 一郎", "grade" to "1", "part" to "後衛", "joined" to "2026-05-01"))
        .save("member", null, mapOf("name" to "佐藤, \"花子\"", "part" to "未定", "joined" to "2025-04-01"))
    private val io = DesktopIo(Examples.load("file_import"), { store }, {})

    @Suppress("UNCHECKED_CAST")
    private fun files(t: Transferable): List<File> = t.getTransferData(DataFlavor.javaFileListFlavor) as List<File>

    @Test
    fun listRowsDragAsTheSameTsvAsCopyAndACsvFile() {
        val ids = store.list("member").map { it.id }
        val picked = listOf(ids[0], ids[2])
        val t = io.rowsTransferable("member", picked)!!
        assertEquals(listOf(DataFlavor.stringFlavor, DataFlavor.javaFileListFlavor, InternalDrag.FLAVOR), t.transferDataFlavors.toList())

        val member = io.entity("member")!!
        val table = RecordIo.exportTable(member.name, member, store, io.entities, null, ids = picked)
        val tsv = t.getTransferData(DataFlavor.stringFlavor) as String
        assertEquals("Ctrl+C と同じ TSV", Tsv.string(table), tsv)
        assertTrue(tsv.startsWith("名前\t学年\tパート\t入部日\r\n"))
        assertFalse("選んでいない行は入らない", "鈴木" in tsv)

        val csv = files(t).single()
        assertEquals("${member.name}.csv", csv.name)
        val bytes = csv.readBytes()
        assertArrayEquals("書き出しと同じ CSV(BOM 付き)", RecordIo.csv(table), bytes)
        assertTrue(bytes.copyOfRange(0, 3).contentEquals(CsvWriter.BOM))
        assertTrue("\"佐藤, \"\"花子\"\"\"" in String(bytes, Charsets.UTF_8))
        assertEquals("何度聞いても同じファイル", csv, files(t).single())
        csv.delete()
    }

    @Test
    fun nothingToDragWithoutRowsOrEntity() {
        assertNull(io.rowsTransferable("member", emptyList()))
        assertNull(io.rowsTransferable("nope", listOf("x")))
    }

    @Test
    fun tableRowsDragTheVisibleTextsInViewOrder() {
        val headers = listOf("品名", "金額")
        val rows = listOf(TableCells.Row("1", listOf("りんご", "¥120")), TableCells.Row("2", listOf("=SUM(A1)", "¥80")))
        val sel = RowSelection(selected = setOf("1", "2"), ids = setOf("1", "2"))
        val t = DragOut.rows(TableCopy.table("買い物/今週", headers, rows.reversed(), RowSelect.dragged(sel, "1")))
        assertEquals(TableCopy.tsv(headers, rows.reversed(), sel), t.getTransferData(DataFlavor.stringFlavor))
        val csv = files(t).single()
        assertEquals("ファイル名に使えない文字は _", "買い物_今週.csv", csv.name)
        assertEquals("﻿品名,金額\r\n'=SUM(A1),¥80\r\nりんご,¥120\r\n", String(csv.readBytes(), Charsets.UTF_8))
        csv.delete()
    }

    @Test
    fun draggingAnUnselectedRowTakesOnlyThatRow() {
        val sel = RowSelection(selected = setOf("1", "2"), anchor = "1", ids = setOf("1", "2", "3"))
        assertEquals(setOf("1", "2"), RowSelect.dragged(sel, "2"))
        assertEquals(setOf("3"), RowSelect.dragged(sel, "3"))
        assertEquals(setOf("1"), RowSelect.dragged(RowSelection(), "1"))
    }

    private fun encoded(format: String): ByteArray {
        val img = BufferedImage(6, 4, BufferedImage.TYPE_INT_RGB)
        img.createGraphics().apply { color = Color.RED; fillRect(0, 0, 6, 4) }.dispose()
        return ByteArrayOutputStream().also { ImageIO.write(img, format, it) }.toByteArray()
    }

    @Test
    fun imagesDragAsPngFiles() {
        for (format in listOf("jpg", "png", "bmp")) {
            val t = DragOut.image(encoded(format), "レシート:1")!!
            assertEquals(DataFlavor.javaFileListFlavor, t.transferDataFlavors.first())
            val file = files(t).single()
            assertEquals("レシート_1.png", file.name)
            val bytes = file.readBytes()
            assertEquals("PNG の印($format)", "\u0089PNG", String(bytes.copyOfRange(0, 4), Charsets.ISO_8859_1))
            val back = ImageIO.read(file)
            assertEquals(6, back.width)
            assertEquals(4, back.height)
            val direct = t.getTransferData(DataFlavor.imageFlavor) as BufferedImage
            assertEquals(6, direct.width)
            file.delete()
        }
    }

    @Test
    fun brokenImagesAndOtherFlavorsAreRefused() {
        assertNull(DragOut.image("not an image".toByteArray(), "x"))
        val t = DragOut.image(encoded("png"), "")!!
        assertEquals("名前が無ければ既定の名前", "image.png", files(t).single().name.also { files(t).single().delete() })
        try {
            t.getTransferData(DataFlavor.stringFlavor)
            throw AssertionError("文字は載せない")
        } catch (_: UnsupportedFlavorException) {
        }
    }

    @Test
    fun imageSourceReadsLocalFilesOnlyForRecordValues() {
        val png = encoded("png")
        val dataUrl = "data:image/png;base64," + Base64.getEncoder().encodeToString(png)
        assertArrayEquals(png, ImageSource.bytes(dataUrl, allowFile = false))
        val f = File.createTempFile("photo-", ".png", UserDirs.photos).apply { writeBytes(png); deleteOnExit() }
        assertArrayEquals("photoField の値(file URI)", png, ImageSource.bytes(f.toURI().toString(), allowFile = true))
        assertNull("DSL に書かれた URL では手元のファイルを読まない", ImageSource.bytes(f.toURI().toString(), allowFile = false))
        val txt = File.createTempFile("note-", ".txt", UserDirs.photos).apply { writeText("x"); deleteOnExit() }
        assertNull("画像でないファイルは読まない", ImageSource.bytes(txt.toURI().toString(), allowFile = true))
        assertNull(ImageSource.bytes("http://example.com/a.png", allowFile = true))
        assertNull(ImageSource.bytes(null, allowFile = true))
    }

    @Test
    fun recordFileValuesAreReadOnlyUnderThePhotoFolder() {
        val png = encoded("png")
        val dir = Files.createTempDirectory("photos-").toFile().apply { deleteOnExit() }
        val inside = File(dir, "a.png").apply { writeBytes(png); deleteOnExit() }
        assertArrayEquals(png, ImageSource.bytes(inside.toURI().toString(), allowFile = true, photoDir = dir))
        val outside = File.createTempFile("elsewhere-", ".png").apply { writeBytes(png); deleteOnExit() }
        assertNull("写真フォルダの外", ImageSource.bytes(outside.toURI().toString(), allowFile = true, photoDir = dir))
        val escaped = dir.toURI().toString() + "../" + outside.name
        assertNull("../ で外へ出ない", ImageSource.bytes(escaped, allowFile = true, photoDir = dir))
        listOf(
            "file:////server/share/a.png",
            "file://server/share/a.png",
            "file:\\\\server\\share\\a.png",
            "file://///server/share/a.png",
            "file://localhost" + inside.toURI().path,
        ).forEach { assertNull(it, UserDirs.photoFile(it, dir)) }
        assertNull("フォルダそのもの", UserDirs.photoFile(dir.toURI().toString(), dir))
        // 選んだ・落とした写真は写真フォルダへ写して読む
        val adopted = UserDirs.adoptPhoto(outside, dir).apply { deleteOnExit() }
        assertEquals(dir.canonicalFile, adopted.canonicalFile.parentFile)
        assertArrayEquals(png, ImageSource.bytes(adopted.toURI().toString(), allowFile = true, photoDir = dir))
        assertEquals("既に下にあればそのまま", inside.canonicalFile, UserDirs.adoptPhoto(inside, dir))
    }

    @Test
    fun appFoldersArePerUserAndOwnerOnly() {
        val root = UserDirs.root.canonicalFile
        assertTrue(UserDirs.drag.canonicalFile.toPath().startsWith(root.toPath()))
        assertTrue(UserDirs.photos.canonicalFile.toPath().startsWith(root.toPath()))
        val csv = files(io.rowsTransferable("member", store.list("member").map { it.id })!!).single().apply { deleteOnExit() }
        assertTrue("引き出すファイルは利用者のフォルダの下", csv.canonicalFile.toPath().startsWith(UserDirs.drag.canonicalFile.toPath()))
        if ("posix" in FileSystems.getDefault().supportedFileAttributeViews()) {
            val owner = PosixFilePermissions.fromString("rwx------")
            assertEquals(owner, Files.getPosixFilePermissions(root.toPath()))
            assertEquals(owner, Files.getPosixFilePermissions(UserDirs.drag.toPath()))
            assertEquals(owner, Files.getPosixFilePermissions(UserDirs.photos.toPath()))
            // 他の物(リンク)が先に置いてあれば使わない
            val base = Files.createTempDirectory("ud-").toFile().apply { deleteOnExit() }
            val link = File(base, "taken")
            Files.createSymbolicLink(link.toPath(), base.toPath())
            assertNull(UserDirs.ownedDir(link.toPath()))
            link.delete()
            val made = UserDirs.ownedDir(File(base, "mine").toPath())!!
            assertEquals(owner, Files.getPosixFilePermissions(made))
            made.toFile().delete()
        }
        if (!System.getProperty("os.name").lowercase().contains("linux")) assertTrue(root.name.endsWith(UserDirs.userTag()))
    }
}
