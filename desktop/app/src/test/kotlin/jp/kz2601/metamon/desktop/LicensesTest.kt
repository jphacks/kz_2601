package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.settings.LicenseEntry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File

/** サードパーティのライセンス: ビルド時に作る一覧・実行環境の legal・パレットの入り口。 */
class LicensesTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private val bundled: List<LicenseEntry> by lazy { checkNotNull(DesktopLicenses.bundled()) { "一覧が読めない" } }

    @Test
    fun generatedListHasTheAppAndKnownDependencies() {
        assertTrue("一覧が空", bundled.size > 10)
        val app = bundled.first()
        assertEquals(LicenseEntry.Kind.APP, app.kind)
        // 種類・本文・著作権の表示はリポジトリの LICENSE と一致する(LICENSE を差し替えたら一覧も追従する)
        val repoLicense = File(checkNotNull(System.getProperty("metamon.repoLicense")) { "metamon.repoLicense が無い" }).readText()
        assertEquals(normalize(repoLicense), normalize(app.text))
        val heading = repoLicense.lineSequence().map { it.trim() }.first { it.isNotEmpty() }
        assertTrue("LICENSE の 1 行目「$heading」に ${app.license} が無い", heading.contains(app.license))
        assertEquals(repoLicense.lines().map { it.trim() }.filter { it.startsWith("Copyright") }, app.copyright)
        // 実際に使っている依存(PDF・QR・Kotlin)が POM の種類・版付きで入っている
        val pdfbox = bundled.single { it.name == "org.apache.pdfbox" }
        assertEquals("Apache-2.0", pdfbox.license)
        assertTrue(pdfbox.text.contains("Apache License"))
        assertNotNull(pdfbox.version)
        // jar の NOTICE と、LICENSE の Apache-2.0 より後ろ(取り込んだ部品の表示)を拾う
        assertTrue(pdfbox.notices.any { it.title == "pdfbox (NOTICE)" && it.text.contains("Apache PDFBox") })
        assertTrue(pdfbox.notices.any { it.title == "pdfbox (LICENSE)" && it.text.contains("EXTERNAL COMPONENTS") })
        assertTrue(bundled.any { it.name == "com.google.zxing" })
        assertTrue(bundled.any { it.name == "org.jetbrains.kotlin" })
        assertTrue(bundled.any { it.name == "org.jetbrains.compose.ui" })
        // Maven を通らない同梱物(licenses/vendored)
        val icons = bundled.single { it.id == "material-icons-extended" }
        assertEquals(listOf("Copyright The Android Open Source Project"), icons.copyright)
    }

    private fun normalize(s: String) = s.replace("\r\n", "\n").lines().joinToString("\n") { it.trimEnd() }.trim('\n')

    @Test
    fun everyEntryHasTextAndUniqueId() {
        assertEquals("id が重なっている", bundled.size, bundled.map { it.id }.toSet().size)
        bundled.forEach { assertTrue("${it.id} の本文が空", it.text.isNotBlank()) }
        // 判定できない種類は生成で止まる(推測で埋めない)ので、ここに来るのは知っている種類だけ
        assertTrue(bundled.all { it.license in setOf("MIT", "Apache-2.0", "BSD-2-Clause", "BSD-3-Clause") })
    }

    @Test
    fun javaRuntimeReadsLegalDirectoryAndSkipsDuplicates() {
        val home = tmp.newFolder("jre")
        home.resolve("legal/java.base").mkdirs()
        home.resolve("legal/java.desktop").mkdirs()
        home.resolve("legal/java.base/LICENSE").writeText("\nThe GNU General Public License (GPL)\n\nVersion 2\n")
        home.resolve("legal/java.base/ASSEMBLY_EXCEPTION").writeText("exception")
        home.resolve("legal/java.base/icu.md").writeText("icu")
        home.resolve("legal/java.desktop/ASSEMBLY_EXCEPTION").writeText("exception")
        home.resolve("legal/java.desktop/LICENSE").writeText("same")
        home.resolve("legal/java.desktop/mesa3d.md").writeText("mesa")
        val e = checkNotNull(DesktopLicenses.javaRuntime(home))
        assertEquals("The GNU General Public License (GPL)", e.license)
        assertEquals(listOf("ASSEMBLY_EXCEPTION", "icu", "mesa3d"), e.notices.map { it.title })
        assertNull("legal が無ければ出さない", DesktopLicenses.javaRuntime(tmp.newFolder("empty")))
    }

    @Test
    fun paletteAndHelpOpenTheLicenses() {
        val s = DesktopSession(Examples.load("shopping"), MockStore())
        val k = DesktopKeyboard(s.dsl, s, HostOs.WINDOWS)
        val command = k.commands().single { it.id == "about:licenses" }
        assertEquals(LicensesText.TITLE, command.title)
        // 英語の名前でも探せる
        assertEquals(command.id, CommandRanking.rank(k.commands(), "licenses").first().id)
        assertFalse(k.licensesOpen)
        command.run()
        assertTrue(k.licensesOpen)
    }
}
