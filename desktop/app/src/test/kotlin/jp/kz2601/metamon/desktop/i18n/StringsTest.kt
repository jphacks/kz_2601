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
        val texts = src.walkTopDown().filter { it.extension == "kt" }.map { it.readText() }.toList()
        fun used(call: String) = texts.flatMap { t -> Regex("""Strings(?:$call)\s*"([a-z0-9_]+)"""").findAll(t).map { it.groupValues[1] } }.toSortedSet()
        val strings = used("""\[|\.format\(""")
        val plurals = used("""\.plural\(""")
        assertTrue("文言のキーが拾えない", strings.size > 20 && plurals.isNotEmpty())
        // plural は <plurals> に、[] と format は <string> にある(引き違えると キーそのもの が出る)
        val missing = strings.filter { !table.hasString(Lang.JA, it) || !table.hasString(Lang.EN, it) } +
            plurals.filter { !table.hasPlural(Lang.JA, it) || !table.hasPlural(Lang.EN, it) }
        assertTrue("日本語か英語が無いキー: $missing", missing.isEmpty())
    }

    @Test
    fun desktopKeysAreSortedAndTheSameInBothLanguages() {
        fun keys(dir: String) = Regex("""<string name="([a-z0-9_]+)"""")
            .findAll(File("src/main/resources/i18n/desktop/$dir/strings_desktop.xml").readText()).map { it.groupValues[1] }.toList()
        val ja = keys("values")
        val en = keys("values-en")
        assertEquals("名前順", ja.sorted(), ja)
        assertEquals("日本語と英語で同じキー", ja, en)
        assertTrue(ja.all { it.startsWith("desktop_") })
    }

    @Test
    fun movedStringsKeepTheJapaneseText() {
        assertEquals("このファイルは取り込めません", table.get(Lang.JA, "desktop_io_unsupported_file"))
        assertEquals("CSV・TSV", listOf("CSV", "TSV").joinToString(table.get(Lang.JA, "desktop_io_kind_separator")))
        assertEquals("CSV, TSV", listOf("CSV", "TSV").joinToString(table.get(Lang.EN, "desktop_io_kind_separator")))
        assertEquals("印刷日 2026/11/12 09:30", table.format(Lang.JA, "desktop_print_date", "2026/11/12 09:30"))
        assertEquals("80 件", table.format(Lang.JA, "host_export_count", 80))
        assertEquals("牛乳 を入力してください", table.format(Lang.JA, "desktop_input_required", "牛乳"))
    }

    @Test
    fun withLangSwitchesAndRestores() {
        val before = Strings.lang
        val inside = Strings.withLang(Lang.EN) { Strings["renderer_back"] }
        assertEquals("Back", inside)
        assertEquals(before, Strings.lang)
    }
}
