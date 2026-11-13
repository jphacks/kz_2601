package jp.kz2601.metamon.desktop.store

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeFalse
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File

/** 読めない設定ファイルは `<名前>.corrupt` に退けて空から始める(setAside)。 */
class JsonKeyValueStoreTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private fun settings(text: String): File = tmp.newFolder().resolve("settings.json").apply { writeText(text) }

    private fun aside(f: File) = File(f.parentFile, "${f.name}.corrupt")

    /** 退けた物が元の中身のまま残り、空の状態で始まる。 */
    private fun assertSetAside(f: File, original: String) {
        val store = JsonKeyValueStore(f)
        assertTrue("空から始める", store.keys().isEmpty())
        assertFalse("元の名前には残さない", f.exists())
        assertEquals("元のファイルは別名で残す", original, aside(f).readText())
    }

    @Test
    fun brokenJsonIsSetAside() {
        val text = """{"a": 1,"""
        assertSetAside(settings(text), text)
    }

    @Test
    fun emptyFileIsSetAside() {
        assertSetAside(settings(""), "")
    }

    @Test
    fun nonObjectRootIsSetAside() {
        val text = "[1, 2]"
        assertSetAside(settings(text), text)
    }

    @Test
    fun unreadableFileIsSetAside() {
        val text = """{"a": 1}"""
        val f = settings(text)
        assumeFalse("権限で読めなくできない環境(root・Windows)では見ない", !f.setReadable(false) || f.canRead())
        try {
            val store = JsonKeyValueStore(f)
            assertTrue(store.keys().isEmpty())
            assertFalse(f.exists())
            assertTrue(aside(f).isFile)
        } finally {
            aside(f).setReadable(true)
        }
        assertEquals(text, aside(f).readText())
    }

    @Test
    fun secondLaunchKeepsTheSetAsideFile() {
        val broken = """{"a": """
        val f = settings(broken)
        JsonKeyValueStore(f).putString("lang", "ja")

        // 2 回目の起動: 書き直したファイルは読め、退けた物はそのまま
        val again = JsonKeyValueStore(f)
        assertEquals("ja", again.getString("lang"))
        assertEquals(broken, aside(f).readText())

        // 何も書かずに終えた後の起動でも残る
        val g = settings(broken)
        JsonKeyValueStore(g)
        assertTrue(JsonKeyValueStore(g).keys().isEmpty())
        assertEquals(broken, aside(g).readText())
    }

    @Test
    fun aNewCorruptionReplacesTheOldSetAsideFile() {
        // 今の規則: また壊れた時は前の .corrupt を新しい物で置き換える(溜め込まない)
        val f = settings("first")
        JsonKeyValueStore(f)
        f.writeText("second")
        JsonKeyValueStore(f)
        assertEquals("second", aside(f).readText())
    }

    @Test
    fun missingFileStartsEmptyWithoutSetAside() {
        val f = tmp.newFolder().resolve("settings.json")
        assertNull(JsonKeyValueStore(f).getString("x"))
        assertFalse(aside(f).exists())
    }
}
