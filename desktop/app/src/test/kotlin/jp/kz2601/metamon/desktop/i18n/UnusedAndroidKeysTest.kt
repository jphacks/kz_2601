package jp.kz2601.metamon.desktop.i18n

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Android の strings.xml にあってデスクトップのソースが引いていないキーの一覧(#2819)。まだ移していない画面の手がかり。
 * 一覧を出すだけで落とさない。`./gradlew :desktop:test --tests '*UnusedAndroidKeysTest*' -i` で見る。
 * 「引いている」= main のソースに `"キー"` の文字列がある(`Strings["…"]`・キーの表)か、`"接頭辞_${…}"` の接頭辞で始まる。
 */
class UnusedAndroidKeysTest {
    private val android = File("../../android")
    private val src = File("src/main/kotlin")

    @Test
    fun printUnusedAndroidKeys() {
        assertTrue("android/ が無い: ${android.absolutePath}", android.isDirectory)
        assertTrue("ソースが無い: ${src.absolutePath}", src.isDirectory)
        val byFile = androidKeys()
        val sources = src.walkTopDown().filter { it.extension == "kt" }.sortedBy { it.path }.map { it.readText() }.toList()
        val used = usedKeys(sources)
        val all = byFile.values.flatten().toSet()
        assertTrue("Android のキーを読めていない", all.isNotEmpty())

        val report = StringBuilder()
        var unusedCount = 0
        for ((file, keys) in byFile) {
            val unused = unused(keys, used)
            if (unused.isEmpty()) continue
            unusedCount += unused.size
            report.append("\n# $file(${unused.size} / ${keys.size})\n")
            unused.forEach { report.append("  $it\n") }
        }
        println("デスクトップで引いていない Android のキー: $unusedCount / ${all.size}$report")
    }

    @Test
    fun matchesQuotedKeysAndTemplatePrefixesButNotComments() {
        val used = usedKeys(listOf("Strings[\"renderer_back\"]\nval k = \"voice_${'$'}{x}\"\n// \"settings_title\"\n * \"settings_close\"\n"))
        assertEquals(
            listOf("settings_close", "settings_title"),
            unused(listOf("renderer_back", "settings_title", "settings_close", "voice_tomorrow"), used),
        )
    }

    data class Used(val literals: Set<String>, val prefixes: Set<String>)

    /** `"キー"` と `"接頭辞_${…}"` の接頭辞。行コメント・KDoc の行は見ない */
    private fun usedKeys(sources: List<String>): Used {
        val code = sources.flatMap { it.lines() }.filterNot { it.trimStart().let { l -> l.startsWith("//") || l.startsWith("*") || l.startsWith("/*") } }
        val literals = code.flatMap { line -> Regex(""""([a-z][a-z0-9_]*)"""").findAll(line).map { it.groupValues[1] }.toList() }.toSet()
        val prefixes = code.flatMap { line -> Regex(""""([a-z][a-z0-9_]*_)\$""").findAll(line).map { it.groupValues[1] }.toList() }.toSet()
        return Used(literals, prefixes)
    }

    private fun unused(keys: List<String>, used: Used): List<String> =
        keys.filter { k -> k !in used.literals && used.prefixes.none { k.startsWith(it) } }.sorted()

    /** renderer と app の values/strings*.xml(日本語の側。英語は同じキー)。ファイルごとのキー */
    private fun androidKeys(): Map<String, List<String>> =
        listOf("renderer", "app").flatMap { module ->
            File(android, "$module/src/main/res/values").listFiles { f -> f.name.startsWith("strings") && f.extension == "xml" }.orEmpty()
                .sortedBy { it.name }
                .map { f ->
                    val parsed = f.inputStream().use(AndroidStringsXml::parse)
                    "$module/${f.name}" to (parsed.strings.keys + parsed.plurals.keys).toList()
                }
        }.toMap()
}
