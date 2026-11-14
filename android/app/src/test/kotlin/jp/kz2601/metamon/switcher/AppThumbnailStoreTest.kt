package jp.kz2601.metamon.switcher

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/** 切り替え一覧のサムネの置き場と撮る条件(#1199・iOS の AppThumbnailStore)。絵そのものは端末でしか作れないのでファイルと条件を見る。 */
class AppThumbnailStoreTest {
    @get:Rule
    val tmp = TemporaryFolder()

    @Test
    fun fileNameKeepsOnlySafeCharacters() {
        assertEquals("app-1_x.jpg", AppThumbnailStore.fileName("app-1_x"))
        assertEquals("______etc_passwd.jpg", AppThumbnailStore.fileName("../../etc/passwd"))
        assertEquals("日記.jpg", AppThumbnailStore.fileName("日記"))
    }

    @Test
    fun capturesOnlyRealScreensWhenNothingCoversThem() {
        assertTrue(AppThumbnailStore.shouldCapture("app-1", isVoiceOnly = false, blocked = false))
        assertFalse("音声だけの初期画面は撮らない", AppThumbnailStore.shouldCapture("app-1", isVoiceOnly = true, blocked = false))
        assertFalse("シート・お知らせ・組み上げの間は撮らない", AppThumbnailStore.shouldCapture("app-1", isVoiceOnly = false, blocked = true))
        assertFalse(AppThumbnailStore.shouldCapture("", isVoiceOnly = false, blocked = false))
    }

    @Test
    fun writesOneFilePerAppAndOverwrites() {
        val dir = tmp.newFolder("metamon-thumbs")
        val store = AppThumbnailStore(dir)
        val target = store.file("app-1")!!
        AppThumbnailStore.write(target, byteArrayOf(1, 2))
        AppThumbnailStore.write(target, byteArrayOf(3))
        assertArrayEquals("撮り直すと上書き", byteArrayOf(3), target.readBytes())
        assertEquals(listOf("app-1.jpg"), dir.list()!!.toList())
    }

    @Test
    fun removeAllDeletesTheFolderAndBumpsRevision() {
        val dir = tmp.newFolder("metamon-thumbs")
        val store = AppThumbnailStore(dir)
        AppThumbnailStore.write(store.file("a")!!, byteArrayOf(1))
        AppThumbnailStore.write(store.file("b")!!, byteArrayOf(1))
        store.remove("a")
        assertFalse(store.file("a")!!.exists())
        assertTrue(store.file("b")!!.exists())
        val before = store.revision.value
        store.removeAll()
        assertFalse(dir.exists())
        assertTrue("一覧が描き直す", store.revision.value > before)
    }

    @Test
    fun memoryOnlyStoreHasNoFiles() {
        val store = AppThumbnailStore(null)
        assertNull(store.file("a"))
        assertNull(store.image("a"))
        store.removeAll()
    }
}
