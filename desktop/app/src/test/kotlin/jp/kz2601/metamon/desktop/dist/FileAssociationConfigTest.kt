package jp.kz2601.metamon.desktop.dist

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/** 配布の設定(#2995): build.gradle.kts に CSV の関連付けが 1 つだけ入っていることを確かめる。jpackage 自体は手元の Windows で確かめる。 */
class FileAssociationConfigTest {
    private val build = File("build.gradle.kts").readText()

    @Test
    fun csvIsAssociated() {
        assertTrue(Regex("""fileAssociation\(\s*mimeType\s*=\s*"text/csv"\s*,\s*extension\s*=\s*"csv"""").containsMatchIn(build))
    }

    @Test
    fun onlyCsvIsAssociated() {
        assertEquals(1, Regex("""\bfileAssociation\(""").findAll(build).count())
    }
}
