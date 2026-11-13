package jp.kz2601.metamon.desktop.i18n

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** strings_desktop.xml の日本語と英語で、キーと書式の数がそろっているか(#1759)。 */
class DesktopStringsParityTest {
    private fun load(dir: String): Map<String, String> {
        val path = "i18n/desktop/$dir/strings_desktop.xml"
        val xml = javaClass.classLoader.getResourceAsStream(path)!!.bufferedReader(Charsets.UTF_8).use { it.readText() }
        return Regex("""<string\s+name="([^"]+)"[^>]*>(.*?)</string>""", RegexOption.DOT_MATCHES_ALL)
            .findAll(xml).associate { it.groupValues[1] to it.groupValues[2] }
    }

    /** %1$s・%2$d・%s などの書式。%% は数えない */
    private fun formats(value: String): List<String> =
        Regex("""%(?:\d+\$)?[sdf]""").findAll(value.replace("%%", "")).map { it.value }.toList()

    private val ja = load("values")
    private val en = load("values-en")

    @Test
    fun bothFilesAreNotEmpty() {
        assertTrue(ja.isNotEmpty())
        assertTrue(en.isNotEmpty())
    }

    @Test
    fun keysMatch() {
        assertEquals("英語にだけ無いキー", emptySet<String>(), ja.keys - en.keys)
        assertEquals("日本語にだけ無いキー", emptySet<String>(), en.keys - ja.keys)
    }

    @Test
    fun formatCountsMatch() {
        val diff = ja.keys.intersect(en.keys).filter {
            formats(ja.getValue(it)).sorted() != formats(en.getValue(it)).sorted()
        }
        assertEquals("書式の数が違うキー", emptyList<String>(), diff)
    }
}
