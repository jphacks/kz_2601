package jp.kz2601.metamon.desktop.store

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File
import java.nio.file.Files
import java.nio.file.LinkOption

/** 端末の片付け: sweepStray・emptyDir の消した数・入れ子・無いフォルダ。 */
class DeviceWipeTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private fun file(parent: File, path: String) = File(parent, path).apply { parentFile.mkdirs(); writeText("x") }

    private fun link(at: File, target: File): Boolean =
        runCatching { Files.createSymbolicLink(at.toPath(), target.toPath()) }.isSuccess

    @Test
    fun sweepStrayRemovesNestedCorruptAndTmpOnly() {
        val root = tmp.newFolder("root")
        val stray = listOf(file(root, "settings.json.corrupt"), file(root, "records/app/e.json.tmp"), file(root, "records/app/deep/f.corrupt"))
        val keep = listOf(file(root, "windows.json"), file(root, "records/app/e.json"), file(root, "a.tmp.bak"), file(root, "corrupt"))
        File(root, "dir.tmp").mkdirs()

        assertEquals(3, DeviceWipe.sweepStray(root))
        stray.forEach { assertFalse(it.path, it.exists()) }
        keep.forEach { assertTrue(it.path, it.exists()) }
        // 名前が .tmp のフォルダは消さない
        assertTrue(File(root, "dir.tmp").isDirectory)
        assertEquals(0, DeviceWipe.sweepStray(root))
    }

    @Test
    fun sweepStrayOnMissingOrFileIsZero() {
        assertEquals(0, DeviceWipe.sweepStray(File(tmp.root, "nowhere")))
        val f = file(tmp.root, "x.tmp")
        assertEquals(0, DeviceWipe.sweepStray(f))
        assertTrue(f.exists())
    }

    @Test
    fun sweepStrayDoesNotFollowLinks() {
        val root = tmp.newFolder("root")
        val outside = tmp.newFolder("outside")
        val target = file(outside, "keep.tmp")
        assumeTrue("symlink not supported", link(File(root, "link"), outside))
        assertEquals(0, DeviceWipe.sweepStray(root))
        assertTrue(target.exists())
    }

    @Test
    fun emptyDirKeepsFolderAndCountsTopLevelEntries() {
        val dir = tmp.newFolder("photos")
        file(dir, "pick-1.png")
        file(dir, "pick-2.png")
        file(dir, "drag-1/rows.csv")
        file(dir, "drag-1/inner/more.csv")

        // 入れ子のフォルダは丸ごと 1 つと数える
        assertEquals(3, DeviceWipe.emptyDir(dir))
        assertTrue(dir.isDirectory)
        assertEquals(0, dir.listFiles()!!.size)
        assertEquals(0, DeviceWipe.emptyDir(dir))
    }

    @Test
    fun emptyDirOnMissingFolderIsZero() {
        val missing = File(tmp.root, "nowhere")
        assertEquals(0, DeviceWipe.emptyDir(missing))
        assertFalse(missing.exists())
    }

    @Test
    fun emptyDirRemovesLinkButNotItsTarget() {
        val dir = tmp.newFolder("drag")
        val outside = tmp.newFolder("outside")
        val target = file(outside, "keep.txt")
        assumeTrue("symlink not supported", link(File(dir, "link"), outside))
        assertEquals(1, DeviceWipe.emptyDir(dir))
        assertFalse(Files.exists(File(dir, "link").toPath(), LinkOption.NOFOLLOW_LINKS))
        assertTrue(target.exists())
    }
}
