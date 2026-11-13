package jp.kz2601.metamon.desktop.i18n

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * #2417: [HardcodedJapaneseTest.allowed](直書きを残してよい一覧)に、もう無い場所が残っていたら落とす。
 * 直した後に古い行が残ると、同じ文を後からまた直書きしても通ってしまう。
 */
class HardcodedJapaneseAllowlist2417Test {
    private val base = HardcodedJapaneseTest()

    /** 一覧の (パス, 文字列) のうち、そのファイルに今は無いもの。 */
    private fun stale(allowed: Map<String, Set<String>>, src: File): List<String> {
        val out = mutableListOf<String>()
        for ((path, literals) in allowed.toSortedMap()) {
            val file = File(src, path)
            if (!file.isFile) {
                out += "$path (ファイルが無い)"
                continue
            }
            val present = base.literals(file.readText()).map { it.third }.toSet()
            for (l in literals.sorted()) if (l !in present) out += "$path \"$l\""
        }
        return out
    }

    @Test
    fun everyAllowedEntryStillExists() {
        assertTrue("ソースが無い: ${base.src.absolutePath}", base.src.isDirectory)
        val gone = stale(base.allowed, base.src)
        assertTrue("もう無い場所が一覧に残っています。HardcodedJapaneseTest の allowed から消してください:\n" + gone.joinToString("\n"), gone.isEmpty())
    }

    @Test
    fun detectsMissingFileAndMissingLiteral() {
        val dir = java.nio.file.Files.createTempDirectory("allow2417").toFile()
        try {
            File(dir, "A.kt").writeText("val a = \"残る\"\nval b = \"別\"\n")
            val got = stale(mapOf("A.kt" to setOf("残る", "消えた"), "B.kt" to setOf("x")), dir)
            assertEquals(listOf("A.kt \"消えた\"", "B.kt (ファイルが無い)"), got)
            assertEquals(emptyList<String>(), stale(mapOf("A.kt" to setOf("残る", "別")), dir))
        } finally {
            dir.deleteRecursively()
        }
    }
}
