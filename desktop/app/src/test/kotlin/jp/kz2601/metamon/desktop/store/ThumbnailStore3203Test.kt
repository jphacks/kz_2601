package jp.kz2601.metamon.desktop.store

import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/** アプリの写しの置き場(#3203・iOS の AppThumbnailStore): 1 アプリ 1 枚・消したアプリの片付け・数の上限・サインアウト。 */
class ThumbnailStore3203Test {
    @get:Rule
    val tmp = TemporaryFolder()

    private var now = 1_800_000_000_000L
    private val png = byteArrayOf(0x89.toByte(), 'P'.code.toByte(), 'N'.code.toByte(), 'G'.code.toByte())

    private fun store(max: Int = 50) = ThumbnailStore(tmp.root.resolve("thumbnails"), max) { now }

    @Test
    fun onePngPerAppAndReadBack() {
        val s = store()
        s.put("app-1", png)
        s.put("app-1", byteArrayOf(1, 2, 3))
        assertEquals(1, s.count())
        assertArrayEquals(byteArrayOf(1, 2, 3), s.get("app-1"))
        assertTrue(tmp.root.resolve("thumbnails/app-1.png").isFile)
        assertArrayEquals("再起動しても読める", byteArrayOf(1, 2, 3), store().get("app-1"))
        assertNull(s.get("other"))
    }

    @Test
    fun unsafeIdsStayInsideTheFolder() {
        val s = store()
        s.put("../evil/a b", png)
        assertEquals("___evil_a_b.png", s.file("../evil/a b").name)
        assertEquals(tmp.root.resolve("thumbnails"), s.file("../evil/a b").parentFile)
        assertArrayEquals(png, s.get("../evil/a b"))
    }

    @Test
    fun thumbnailPrunesDeletedApps() {
        val s = store()
        s.put("keep", png)
        s.put("gone", png)
        tmp.root.resolve("thumbnails/x.png.123.tmp").writeText("half")
        assertEquals(1, s.prune(setOf("keep", "never-captured")))
        assertTrue(s.contains("keep"))
        assertFalse(s.contains("gone"))
        assertFalse("書きかけも消す", tmp.root.resolve("thumbnails/x.png.123.tmp").exists())
    }

    @Test
    fun thumbnailDropsOldestOverLimit() {
        val s = store(max = 2)
        s.put("a", png)
        now += 1_000
        s.put("b", png)
        now += 1_000
        s.put("a", png) // a を撮り直すと b が一番古い
        now += 1_000
        s.put("c", png)
        assertEquals(2, s.count())
        assertFalse(s.contains("b"))
        assertTrue(s.contains("a") && s.contains("c"))
    }

    @Test
    fun revisionAdvancesOnChange() {
        val s = store()
        val start = s.revision.value
        s.put("a", png)
        s.remove("a")
        s.remove("a") // 無い物を消しても進まない
        assertEquals(start + 2, s.revision.value)
    }

    @Test
    fun memoryOnlyWithoutDir() {
        val s = ThumbnailStore(null, maxCount = 1) { now }
        s.put("a", png)
        now += 1
        s.put("b", png)
        assertNull(s.get("a"))
        assertEquals(1, s.prune(emptySet()))
        assertEquals(0, s.count())
    }

    @Test
    fun signOutWipesThumbnails() = runBlocking {
        val root = tmp.newFolder("data")
        val desktop = DesktopStore(root)
        desktop.thumbnails.put("a", png)
        assertTrue(root.resolve("thumbnails/a.png").isFile)
        desktop.wipe()
        assertFalse(root.resolve("thumbnails").exists())
        assertNull(desktop.thumbnails.get("a"))
    }
}
