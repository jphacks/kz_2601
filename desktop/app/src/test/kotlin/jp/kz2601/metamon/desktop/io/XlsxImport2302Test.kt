package jp.kz2601.metamon.desktop.io

import jp.kz2601.metamon.desktop.Examples
import jp.kz2601.metamon.desktop.MockStore
import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.dsl.Accept
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.logic.RecordImport
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/** fileImport・ドロップ・ファイルの貼り付けで .xlsx を CSV と同じ確認に渡す(#2302)。testdata は src/test/resources/xlsx/sample.xlsx。 */
class XlsxImport2302Test {
    private var store = MockStore()
    private val io = DesktopIo(Examples.load("kakeibo"), { store }, { store = it })
    private val importer = Component(type = ComponentType.FILE_IMPORT, entity = "expense", accept = listOf(Accept.CSV))

    private val sample: ByteArray = javaClass.getResourceAsStream("/xlsx/sample.xlsx")!!.use { it.readBytes() }

    private fun sampleFile(name: String = "家計.xlsx"): File {
        val dir = kotlin.io.path.createTempDirectory("xlsx2302").toFile().apply { deleteOnExit() }
        return File(dir, name).apply {
            writeBytes(sample)
            deleteOnExit()
        }
    }

    @Test
    fun xlsxIsATableFile() {
        assertEquals(FileKind.XLSX, FileKind.of("Book1.XLSX"))
        assertTrue(FileKind.XLSX.isTable)
        assertTrue(FileKind.CSV.isTable)
        assertEquals(FileKind.OTHER, FileKind.of("old.xls"))
        assertEquals(listOf("CSV", "XLSX", "PDF"), RecordIo.acceptLabels(listOf(Accept.CSV, Accept.PDF)))
    }

    @Test
    fun fileImportOffersTheFirstSheetForConfirmation() {
        Strings.withLang(Lang.JA) { io.importFile(sampleFile(), importer) }
        val p = io.pending!!
        assertEquals("expense", p.entityId)
        // シートが 3 枚あるので、確認の見出しに読んだシートの名前を出す
        assertEquals("家計.xlsx(シート「家計」)", p.label)
        val matched = p.plan.matched.map { it.first }
        assertTrue(matched.toString(), matched.containsAll(listOf("日付", "金額", "メモ")))
        // 1 行目(日付の書式・数)は読める
        assertTrue(p.plan.rows.isNotEmpty())
        io.confirm()
        assertTrue(store.list("expense").isNotEmpty())
    }

    @Test
    fun dropRoutesXlsxToTheImporterOrTheList() {
        assertTrue(io.dropFiles(listOf(sampleFile()), listOf(importer), null))
        assertTrue(io.pending != null)

        io.pending = null
        val list = Component(type = ComponentType.LIST, entity = "expense")
        assertTrue("fileImport が無くても一覧に落とせる", io.dropFiles(listOf(sampleFile("b.xlsx")), listOf(list), null))
        assertTrue(io.pending!!.label.startsWith("b.xlsx"))
    }

    @Test
    fun pastedXlsxFileUsesTheSameRoute() {
        val clip = FileTransferable(listOf(sampleFile()))
        assertTrue(io.paste(clip, listOf(importer), null))
        assertTrue(io.pending != null)
    }

    @Test
    fun importerWithoutCsvRefusesXlsx() {
        io.importFile(sampleFile(), importer.copy(accept = listOf(Accept.PDF)))
        assertNull(io.pending)
        assertEquals(Strings["desktop_io_unsupported_file"], io.notice)
    }

    @Test
    fun brokenAndOversizedFilesAreRefused() {
        val entity = io.entity("expense")!!
        io.importXlsx("not a zip".toByteArray(), entity, null, "x.xlsx")
        assertEquals(Strings["desktop_io_xlsx_unreadable"], io.notice)
        io.importXlsx(ByteArray(RecordImport.MAX_TEXT_BYTES + 1), entity, null, "x.xlsx")
        assertEquals(Strings["desktop_io_file_too_large"], io.notice)
        assertNull(io.pending)
    }
}
