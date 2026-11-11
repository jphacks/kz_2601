package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.renderer.components.importing.PdfText
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** PDF の本文の組み立て(iOS の FileTextReading.pdfText と同じ規則)。 */
class PdfTextTest {
    private class FakePages(
        private val layers: List<String?>,
        private val ocr: Map<Int, List<String>?> = emptyMap(),
    ) : PdfText.PdfPages {
        val ocrCalls = mutableListOf<Int>()
        override val pageCount get() = layers.size
        override fun layerText(index: Int) = layers[index]
        override fun recognizedLines(index: Int): List<String>? {
            ocrCalls += index
            return if (index in ocr) ocr[index] else listOf("ocr$index")
        }
    }

    @Test
    fun textLayerIsUsedAsIsAndScansAreRecognized() {
        val pages = FakePages(listOf("  1 ページ目\n", null, "   ", "4"), ocr = mapOf(1 to listOf("請求書", "合計 1,200 円"), 2 to emptyList()))
        assertEquals("1 ページ目\n\n請求書\n合計 1,200 円\n\n4", PdfText.assemble(pages))
        assertEquals(listOf(1, 2), pages.ocrCalls)
    }

    @Test
    fun recognitionStopsAtTheLimitAndUndrawablePagesDoNotCount() {
        val pages = FakePages(List(5) { null }, ocr = mapOf(0 to null))
        assertEquals("ocr1\n\nocr2", PdfText.assemble(pages, maxOcrPages = 2))
        assertEquals(listOf(0, 1, 2), pages.ocrCalls)
    }

    @Test
    fun emptyPdfIsEmptyText() {
        assertEquals("", PdfText.assemble(FakePages(emptyList())))
    }

    @Test
    fun renderScaleKeepsLongSideAt2000AndAtMost3x() {
        assertEquals(2000f / 842f, PdfText.renderScale(595f, 842f)!!, 0.0001f)
        assertEquals(3f, PdfText.renderScale(100f, 200f)!!, 0f)
        assertNull(PdfText.renderScale(0f, 100f))
    }
}
