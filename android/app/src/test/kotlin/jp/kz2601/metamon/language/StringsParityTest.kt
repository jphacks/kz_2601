package jp.kz2601.metamon.language

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.w3c.dom.Element
import java.io.File
import javax.xml.parsers.DocumentBuilderFactory

/**
 * 日本語(values)と英語(values-en)の文言の対応が欠けていないか(#843)。全モジュールの src/main/res を見る。
 * - キーが両方にある(translatable="false" は日本語だけで良い)
 * - 書式の引数(%1$s など)が揃っている
 * - locales_config.xml の言語と values-* が揃っている
 */
class StringsParityTest {
    /** Gradle の単体テストはモジュールの場所で動く(android/app)。 */
    private val androidRoot: File = File(System.getProperty("user.dir")).absoluteFile.parentFile

    private val modules: List<File> = androidRoot.listFiles().orEmpty()
        .filter { File(it, "src/main/res/values").isDirectory }
        .sortedBy { it.name }

    private data class Entry(val text: String, val translatable: Boolean)

    private fun strings(valuesDir: File): Map<String, Entry> {
        val out = sortedMapOf<String, Entry>()
        val files = valuesDir.listFiles { f -> f.extension == "xml" }.orEmpty().sortedBy { it.name }
        for (file in files) {
            val doc = DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(file)
            val children = doc.documentElement.childNodes
            for (i in 0 until children.length) {
                val e = children.item(i) as? Element ?: continue
                if (e.tagName !in setOf("string", "plurals", "string-array")) continue
                val key = "${e.tagName}/${e.getAttribute("name")}"
                check(key !in out) { "${file.path}: $key が二重に定義されています" }
                out[key] = Entry(e.textContent, e.getAttribute("translatable") != "false")
            }
        }
        return out
    }

    private fun formatArgs(text: String): List<String> =
        Regex("""%(\d+\$)?[-#+ 0,(]*\d*(\.\d+)?[sdfxXoeEgGc]""").findAll(text).map { it.value }.sorted().toList()

    @Test
    fun everyModuleWithStringsIsScanned() {
        assertTrue("モジュールが見つかりません: $androidRoot", modules.any { it.name == "app" })
        assertTrue(modules.any { it.name == "renderer" })
    }

    @Test
    fun japaneseAndEnglishHaveTheSameKeys() {
        val problems = mutableListOf<String>()
        for (module in modules) {
            val ja = strings(File(module, "src/main/res/values"))
            val en = strings(File(module, "src/main/res/values-en"))
            val needed = ja.filterValues { it.translatable }.keys
            (needed - en.keys).forEach { problems += "${module.name}: 英語が無い $it" }
            (en.keys - ja.keys).forEach { problems += "${module.name}: 日本語(既定)が無い $it" }
            ja.filterValues { !it.translatable }.keys.intersect(en.keys).forEach {
                problems += "${module.name}: translatable=false なのに英語がある $it"
            }
            en.filter { it.value.text.isBlank() }.keys.forEach { problems += "${module.name}: 英語が空 $it" }
        }
        assertEquals(problems.joinToString("\n"), emptyList<String>(), problems)
    }

    @Test
    fun formatArgumentsMatch() {
        val problems = mutableListOf<String>()
        for (module in modules) {
            val ja = strings(File(module, "src/main/res/values"))
            val en = strings(File(module, "src/main/res/values-en"))
            for ((key, entry) in ja) {
                val other = en[key] ?: continue
                if (key.startsWith("plurals/")) continue // 数ごとの形は言語で数が違う
                if (formatArgs(entry.text) != formatArgs(other.text)) {
                    problems += "${module.name}: $key の引数が違う ja=${formatArgs(entry.text)} en=${formatArgs(other.text)}"
                }
            }
        }
        assertEquals(problems.joinToString("\n"), emptyList<String>(), problems)
    }

    @Test
    fun localeConfigListsDefaultJapaneseAndEveryTranslation() {
        val doc = DocumentBuilderFactory.newInstance().newDocumentBuilder()
            .parse(File(androidRoot, "app/src/main/res/xml/locales_config.xml"))
        val nodes = doc.getElementsByTagName("locale")
        val declared = (0 until nodes.length).map { (nodes.item(it) as Element).getAttribute("android:name") }.toSet()

        val translated = modules.flatMap { module ->
            File(module, "src/main/res").listFiles().orEmpty()
                .map { it.name }
                .filter { Regex("""values-[a-z]{2}(-r[A-Z]{2})?""").matches(it) }
                .map { it.removePrefix("values-").replace("-r", "-") }
        }.toSet()

        assertEquals(setOf("ja") + translated, declared)
        assertEquals(AppLanguage.entries.mapNotNull { it.tag }.toSet(), declared)
    }
}
