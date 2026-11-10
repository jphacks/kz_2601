package jp.kz2601.metamon.language

import org.junit.Assert.assertEquals
import org.junit.Test
import org.w3c.dom.Element
import java.io.File
import java.util.Locale
import javax.xml.parsers.DocumentBuilderFactory

/**
 * 英語で数と名詞が合うか(「1 records」にしない)。全モジュールの <plurals> を読み、
 * Android と同じく英語は 1 なら one・それ以外は other、日本語は other だけを使う。
 */
class EnglishPluralsTest {
    private val androidRoot: File = File(System.getProperty("user.dir")).absoluteFile.parentFile

    /** モジュール/values-xx のすべての plurals: 名前 → (quantity → 文)。 */
    private fun plurals(dir: String): Map<String, Map<String, String>> {
        val out = mutableMapOf<String, Map<String, String>>()
        val files = androidRoot.listFiles().orEmpty().flatMap { File(it, "src/main/res/$dir").listFiles { f -> f.extension == "xml" }.orEmpty().toList() }
        for (file in files) {
            val nodes = DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(file).getElementsByTagName("plurals")
            for (i in 0 until nodes.length) {
                val e = nodes.item(i) as Element
                val items = e.getElementsByTagName("item")
                out[e.getAttribute("name")] = (0 until items.length).associate { j ->
                    val item = items.item(j) as Element
                    item.getAttribute("quantity") to item.textContent.replace("\\'", "'").replace("\\\"", "\"")
                }
            }
        }
        return out
    }

    private val en = plurals("values-en")
    private val ja = plurals("values")

    private fun english(name: String, quantity: Int, vararg args: Any): String {
        val forms = en.getValue(name)
        return String.format(Locale.US, forms.getValue(if (quantity == 1) "one" else "other"), *args)
    }

    @Test
    fun englishHasOneAndOtherAndJapaneseHasOther() {
        val problems = mutableListOf<String>()
        en.forEach { (name, forms) -> if (forms.keys != setOf("one", "other")) problems += "values-en $name: ${forms.keys}" }
        ja.forEach { (name, forms) -> if ("other" !in forms) problems += "values $name: other が無い" }
        assertEquals(problems.joinToString("\n"), emptyList<String>(), problems)
    }

    @Test
    fun oneAndTwoReadNaturally() {
        val cases = listOf(
            english("account_unsent_after_flush", 1, 1) to "1 record couldn’t be sent. If you sign out without sending, it’ll be lost.",
            english("account_unsent_after_flush", 2, 2) to "2 records couldn’t be sent. If you sign out without sending, they’ll be lost.",
            english("account_unsent_offline", 1, 1) to "You have 1 unsent record. You’re offline, so it can’t be sent now. If you sign out without sending, it’ll be lost.",
            english("account_unsent_offline", 2, 2) to "You have 2 unsent records. You’re offline, so they can’t be sent now. If you sign out without sending, they’ll be lost.",
            english("account_unsent_will_send", 1, 1) to "You have 1 unsent record. It’ll be sent before you sign out.",
            english("account_unsent_will_send", 2, 2) to "You have 2 unsent records. They’ll be sent before you sign out.",
            english("discover_byline", 1, "Kota", 1) to "Kota · 1 user",
            english("discover_byline", 2, "Kota", 2) to "Kota · 2 users",
            english("discover_byline_mine", 1, "Kota", 1) to "Kota · 1 user · Published by you",
            english("discover_byline_mine", 2, "Kota", 2) to "Kota · 2 users · Published by you",
            english("publish_live", 1, 1) to "Published on Discover · 1 install",
            english("publish_live", 2, 2) to "Published on Discover · 2 installs",
            english("feedback_composer_max_length", 1, 1) to "Up to 1 character",
            english("feedback_composer_max_length", 2, 2) to "Up to 2 characters",
            english("members_count", 1, 1) to "1 person",
            english("members_count", 2, 2) to "2 people",
            english("ai_save_added", 1, 1) to "Added 1 record",
            english("ai_save_added", 2, 2) to "Added 2 records",
            english("ai_save_added_partial", 1, 1, 3) to "Added 1 record (3 couldn't be added)",
            english("ai_save_added_partial", 2, 2, 1) to "Added 2 records (1 couldn't be added)",
            english("renderer_import_added", 1, 1) to "Added 1 record",
            english("renderer_import_added", 2, 2) to "Added 2 records",
            english("renderer_import_added_partly", 1, 1, 1) to "Added 1 record (1 couldn't be added)",
            english("renderer_import_added_partly", 2, 2, 1) to "Added 2 records (1 couldn't be added)",
            english("renderer_import_csv_count", 1, "Budget", 1) to "Add 1 record to \"Budget\".",
            english("renderer_import_csv_count", 2, "Budget", 2) to "Add 2 records to \"Budget\".",
            english("renderer_import_csv_over", 1, 500, 5000, 1) to "1 row over the limit (500 per import, 5000 in total) will be skipped",
            english("renderer_import_csv_over", 2, 500, 5000, 2) to "2 rows over the limit (500 per import, 5000 in total) will be skipped",
            english("renderer_import_body", 1, "a.txt", 1, "Notes") to "Add 1 record with 1 character from \"a.txt\" to \"Notes\".",
            english("renderer_import_body", 2, "a.txt", 2, "Notes") to "Add 1 record with 2 characters from \"a.txt\" to \"Notes\".",
            english("renderer_import_body_truncated", 1, 1) to "It's long, so only the first 1 character will be added.",
            english("renderer_import_body_truncated", 2, 2) to "It's long, so only the first 2 characters will be added.",
            english("lists_chart_summary_one", 1, 1, "Mar", "12") to "1 item. Mar: 12",
            english("lists_records_count", 1, 1) to "1 record",
            english("lists_records_count", 2, 2) to "2 records",
            english("play_bracket_late", 1, 1) to "1 entrant added after the start is not in this bracket",
            english("play_bracket_late", 2, 2) to "2 entrants added after the start are not in this bracket",
        )
        cases.forEach { (actual, expected) -> assertEquals(expected, actual) }
        assertEquals(
            english("feedback_composer_consent", 2, 2).replace("2 days", "1 day"),
            english("feedback_composer_consent", 1, 1),
        )
    }

    @Test
    fun japaneseKeepsTheSameSentenceForAnyCount() {
        assertEquals(mapOf("other" to "%1\$d 人"), ja.getValue("members_count"))
        assertEquals(mapOf("other" to "%1\$d 件を追加しました"), ja.getValue("renderer_import_added"))
    }
}
