package jp.kz2601.metamon.desktop.io

import jp.kz2601.metamon.dsl.logic.RecordImport
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.RandomAccessFile

/**
 * 本文を読むのにかける時間の上限(このテストだけ)。testdata の PDF はフォントを埋め込んでいないので、
 * PDFBox が OS のフォントを探しに行く。Windows の CI ではその一度目が既定の [PdfIo.PARSE_TIMEOUT_MS] に
 * 収まらず、時間切れで `Broken` になっていた(#3747)。
 */
private const val LONG_TIMEOUT_MS = 120_000L

/** PDF の本文の取り出し(#2641): testdata の PDF で取り出した文字と、読めない時の扱いを確かめる。 */
class PdfIoTest {
    @get:Rule
    val temp = TemporaryFolder()

    private fun fixture(name: String): ByteArray =
        javaClass.getResourceAsStream("/pdf/$name")!!.use { it.readBytes() }

    private fun file(name: String): java.io.File =
        temp.newFile(name).also { it.writeBytes(fixture(name)) }

    private fun textOf(result: PdfIo.TextResult): String {
        assertTrue("$result は本文ではない", result is PdfIo.TextResult.Text)
        return (result as PdfIo.TextResult.Text).text
    }

    @Test
    fun japaneseTextComesOutAsIs() {
        assertTrue(textOf(PdfIo.text(fixture("japanese.pdf"), LONG_TIMEOUT_MS)).contains("日本語のテスト"))
    }

    @Test
    fun bothPagesComeOutInOrder() {
        val text = textOf(PdfIo.text(fixture("two-pages.pdf"), LONG_TIMEOUT_MS))
        assertTrue(text.contains("Page one body"))
        assertTrue(text.contains("Page two body"))
        assertTrue("頁の順に並ぶ", text.indexOf("Page one body") < text.indexOf("Page two body"))
    }

    @Test
    fun pageWithoutTextIsNoText() {
        assertEquals(PdfIo.TextResult.NoText, PdfIo.text(fixture("no-text.pdf"), LONG_TIMEOUT_MS))
    }

    @Test
    fun brokenPdfIsBroken() {
        assertEquals(PdfIo.TextResult.Broken, PdfIo.text(fixture("broken.pdf"), LONG_TIMEOUT_MS))
    }

    @Test
    fun sameResultsFromAFile() {
        assertTrue(textOf(PdfIo.text(file("japanese.pdf"), LONG_TIMEOUT_MS)).contains("日本語のテスト"))
        assertEquals(PdfIo.TextResult.NoText, PdfIo.text(file("no-text.pdf"), LONG_TIMEOUT_MS))
        assertEquals(PdfIo.TextResult.Broken, PdfIo.text(file("broken.pdf"), LONG_TIMEOUT_MS))
    }

    @Test
    fun missingFileIsBroken() {
        assertEquals(PdfIo.TextResult.Broken, PdfIo.text(temp.newFolder("dir")))
        assertEquals(PdfIo.TextResult.Broken, PdfIo.text(java.io.File(temp.root, "none.pdf")))
    }

    @Test
    fun tooLargeIsRefusedBeforeOpening() {
        val big = temp.newFile("big.pdf")
        RandomAccessFile(big, "rw").use { it.setLength(RecordImport.MAX_PDF_BYTES + 1L) }
        assertEquals(PdfIo.TextResult.TooLarge, PdfIo.text(big))
        assertEquals(PdfIo.TextResult.TooLarge, PdfIo.text(ByteArray(RecordImport.MAX_PDF_BYTES + 1)))
    }

    @Test
    fun slowParseTimesOutAsBroken() {
        val result = PdfIo.parse(timeoutMs = 50L) {
            Thread.sleep(5_000)
            error("ここまで来ない")
        }
        assertEquals(PdfIo.TextResult.Broken, result)
    }
}
