package jp.kz2601.metamon.desktop.io

import jp.kz2601.metamon.desktop.Examples
import jp.kz2601.metamon.desktop.MockStore
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

class OpenWithFilesTest {
    private val all: (File) -> Boolean = { true }

    @Test
    fun pathsAndFileUrisBecomeFiles() {
        assertEquals(File("/home/a/data.csv"), OpenWithFiles.fileOf("/home/a/data.csv", all))
        assertEquals(File("/home/a/my data.CSV"), OpenWithFiles.fileOf("file:///home/a/my%20data.CSV", all))
        assertEquals(File("/x/y.tsv"), OpenWithFiles.fileOf("file:/x/y.tsv", all))
    }

    @Test
    fun unsupportedMissingOrMalformedAreSkipped() {
        assertEquals(null, OpenWithFiles.fileOf("/a/b.png", all))
        assertEquals(null, OpenWithFiles.fileOf("/a/b.csv") { false })
        assertEquals(null, OpenWithFiles.fileOf("file://%zz.csv", all))
        assertEquals(null, OpenWithFiles.fileOf("shopping", all))
    }

    @Test
    fun fromArgsKeepsOrderDropsDuplicatesAndCaps() {
        val args = listOf("shopping", "/a.csv", "/b.csv", "file:///a.csv", "x.png")
        assertEquals(listOf(File("/a.csv"), File("/b.csv")), OpenWithFiles.fromArgs(args, all))
        assertEquals(listOf("shopping", "x.png"), OpenWithFiles.withoutFiles(args, all))
        val many = (1..20).map { "/f$it.csv" }
        assertEquals(OpenWithFiles.MAX_FILES, OpenWithFiles.fromArgs(many, all).size)
    }

    @Test
    fun realFileIsAcceptedAndDirectoryIsNot() {
        val f = File.createTempFile("open-with-", ".csv").apply { deleteOnExit() }
        assertEquals(listOf(f), OpenWithFiles.fromArgs(listOf(f.absolutePath)))
        val dir = File(f.parentFile, "open-with-dir.csv").apply { mkdirs(); deleteOnExit() }
        assertEquals(emptyList<File>(), OpenWithFiles.fromArgs(listOf(dir.absolutePath)))
    }

    @Test
    fun mimeTypeLineHasTrailingSemicolon() {
        assertEquals("MimeType=text/csv;text/tab-separated-values;", OpenWithFiles.mimeTypeLine())
    }

    @Test
    fun inboxHandsOverOnceAndBumpsRevision() {
        val before = OpenWithInbox.revision
        OpenWithInbox.offer(emptyList())
        assertEquals(before, OpenWithInbox.revision)
        OpenWithInbox.offer(listOf(File("/a.csv")))
        assertEquals(before + 1, OpenWithInbox.revision)
        assertEquals(listOf(File("/a.csv")), OpenWithInbox.take())
        assertTrue(OpenWithInbox.take().isEmpty())
    }

    @Test
    fun csvArgRoutesToTheListImportLikeADrop() {
        val io = DesktopIo(Examples.load("shopping"), { MockStore() }, {})
        val entity = io.entities.first()
        val header = entity.fields.first().id
        val f = File.createTempFile("open-with-", ".csv").apply { deleteOnExit(); writeText("$header\nx\n") }
        val list = Component(type = ComponentType.LIST, entity = entity.id)
        val files = OpenWithFiles.fromArgs(listOf(f.absolutePath))
        assertTrue(io.dropFiles(files, listOf(list), null))
        assertNotNull(io.pending)
        // 受け先が無い画面ではドロップと同じ断り
        val io2 = DesktopIo(Examples.load("shopping"), { MockStore() }, {})
        assertFalse(io2.dropFiles(files, emptyList(), null))
        assertNotNull(io2.notice)
    }
}
