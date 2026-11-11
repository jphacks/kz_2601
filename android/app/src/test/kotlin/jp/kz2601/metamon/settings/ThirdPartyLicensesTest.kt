package jp.kz2601.metamon.settings

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/** #898 ライセンスの一覧: 同梱の JSON(android/scripts/third-party-licenses.py が生成)が読め、全件に本文がある。 */
class ThirdPartyLicensesTest {
    private val bundled = File("src/main/res/raw/third_party_licenses.json").readText()

    @Test
    fun bundledListStartsWithThisAppAndEveryEntryHasText() {
        val entries = ThirdPartyLicenses.parse(bundled) ?: error("同梱の一覧が読めない")
        assertEquals(LicenseEntry.Kind.APP, entries.first().kind)
        assertEquals("MIT", entries.first().license)
        assertTrue("依存が入っている", entries.count { it.kind == LicenseEntry.Kind.DEPENDENCY } > 10)
        assertEquals("id が重ならない", entries.size, entries.map { it.id }.toSet().size)
        entries.forEach {
            assertTrue("${it.id} の本文が無い", it.text.isNotBlank())
            assertTrue("${it.id} のライセンスが無い", it.license.isNotBlank())
        }
        assertTrue("Kotlin の標準ライブラリ", entries.any { it.name == "org.jetbrains.kotlin" && it.license == "Apache-2.0" })
    }

    @Test
    fun brokenJsonIsNull() {
        assertNull(ThirdPartyLicenses.parse("{}"))
        assertNull(ThirdPartyLicenses.parse("not json"))
    }

    @Test
    fun paragraphsJoinWrappedLinesAndSplitOnBlankLines() {
        val text = "  Apache License\n  Version 2.0\n\n\n   1. Definitions.\n      \"License\" shall mean\n"
        assertEquals(listOf("Apache License Version 2.0", "1. Definitions. \"License\" shall mean"), ThirdPartyLicenses.paragraphs(text))
    }
}
