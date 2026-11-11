package jp.kz2601.metamon.desktop.i18n

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import java.util.Locale

/** 文言: Android の strings.xml を同じキーで引く・英語へ落ちる・言語の決め方(#1759)。 */
class StringsTest {
    private val table = StringTable.fromClasspath()

    @Test
    fun languageFollowsOverrideThenOsAndOnlyJaIsJapanese() {
        assertEquals(Lang.JA, Lang.resolve("ja_JP"))
        assertEquals(Lang.JA, Lang.resolve("ja-JP"))
        assertEquals(Lang.JA, Lang.resolve("ja"))
        assertEquals(Lang.EN, Lang.resolve("en_US"))
        assertEquals(Lang.EN, Lang.resolve("fr-FR"))
        assertEquals(Lang.JA, Lang.resolve(null, Locale.JAPAN))
        assertEquals(Lang.EN, Lang.resolve(null, Locale.FRANCE))
        assertEquals(Lang.EN, Lang.resolve("  ", Locale.US))
    }

    @Test
    fun readsAndroidStringsWithTheSameKeys() {
        // android/renderer と android/app の values・values-en をビルド時に同梱した物
        assertEquals("戻る", table.get(Lang.JA, "renderer_back"))
        assertEquals("Back", table.get(Lang.EN, "renderer_back"))
        assertEquals("削除", table.get(Lang.JA, "runtime_confirm_delete"))
        assertEquals("Delete", table.get(Lang.EN, "runtime_confirm_delete"))
        assertEquals("Please fill in 金額", table.format(Lang.EN, "runtime_missing_required", "金額"))
        assertEquals("金額 を入力してください", table.format(Lang.JA, "runtime_missing_required", "金額"))
        // Android のエスケープ(\')
        assertEquals("There's no form to save", table.get(Lang.EN, "runtime_no_form"))
    }

    @Test
    fun missingKeyFallsBackToEnglishThenJapaneseThenKey() {
        val t = StringTable(
            mapOf(
                Lang.JA to mapOf("both" to "両方", "ja_only" to "日本語だけ"),
                Lang.EN to mapOf("both" to "Both", "en_only" to "English only"),
            ),
        )
        assertEquals("両方", t.get(Lang.JA, "both"))
        assertEquals("English only", t.get(Lang.JA, "en_only"))
        assertEquals("日本語だけ", t.get(Lang.EN, "ja_only"))
        assertEquals("nowhere", t.get(Lang.EN, "nowhere"))
        assertEquals("nowhere", t.get(Lang.JA, "nowhere"))
        // 日本語でも英語でもない OS は英語
        assertEquals("Both", t.get(Lang.resolve("de_DE"), "both"))
    }

    @Test
    fun pluralsPickOneOnlyInEnglish() {
        val t = StringTable(
            emptyMap(),
            mapOf(
                Lang.EN to mapOf("items" to mapOf("one" to "%1\$d item", "other" to "%1\$d items")),
                Lang.JA to mapOf("items" to mapOf("other" to "%1\$d 件")),
            ),
        )
        assertEquals("1 item", t.plural(Lang.EN, "items", 1, 1))
        assertEquals("3 items", t.plural(Lang.EN, "items", 3, 3))
        assertEquals("1 件", t.plural(Lang.JA, "items", 1, 1))
    }

    @Test
    fun unescapeFollowsAndroidRules() {
        assertEquals("Couldn't", AndroidStringsXml.unescape("Couldn\\'t"))
        assertEquals(", ", AndroidStringsXml.unescape("\", \""))
        assertEquals("a\nb", AndroidStringsXml.unescape("a\\nb"))
        assertEquals("say \"hi\"", AndroidStringsXml.unescape("say \\\"hi\\\""))
        assertEquals("é", AndroidStringsXml.unescape("\\u00e9"))
        val parsed = AndroidStringsXml.parse(
            """<resources><string name="a">A&amp;B</string><plurals name="p"><item quantity="one">x</item><item quantity="other">y</item></plurals></resources>""".byteInputStream(),
        )
        assertEquals("A&B", parsed.strings["a"])
        assertEquals(mapOf("one" to "x", "other" to "y"), parsed.plurals["p"])
    }

    @Test
    fun everyKeyUsedInTheAppExistsInJapaneseAndEnglish() {
        val src = File("src/main/kotlin")
        assertTrue("ソースが無い: ${src.absolutePath}", src.isDirectory)
        val key = Regex("""Strings(?:\[|\.format\(|\.plural\()\s*"([a-z0-9_]+)"""")
        val used = src.walkTopDown().filter { it.extension == "kt" }.flatMap { f -> key.findAll(f.readText()).map { it.groupValues[1] } }.toSortedSet()
        assertTrue("文言のキーが拾えない", used.size > 20)
        val missing = used.filter { !table.has(Lang.JA, it) || !table.has(Lang.EN, it) }
        assertTrue("日本語か英語が無いキー: $missing", missing.isEmpty())
    }

    @Test
    fun withLangSwitchesAndRestores() {
        val before = Strings.lang
        val inside = Strings.withLang(Lang.EN) { Strings["renderer_back"] }
        assertEquals("Back", inside)
        assertEquals(before, Strings.lang)
    }
}
