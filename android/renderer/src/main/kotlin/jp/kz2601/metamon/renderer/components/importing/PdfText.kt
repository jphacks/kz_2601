package jp.kz2601.metamon.renderer.components.importing

import jp.kz2601.metamon.dsl.logic.RecordImport

/** PDF の本文の組み立て(iOS の `FileTextReading.pdfText`)。読み方は [PdfPages] で差し替える(端末では [PdfTextReader])。 */
object PdfText {
    /** 1 つの PDF のページを読む口。 */
    interface PdfPages {
        val pageCount: Int

        /** ページの文字の層(無ければ空・null) */
        fun layerText(index: Int): String?

        /** ページを画像にして文字認識した行。描けなかったら null(認識の回数に数えない) */
        fun recognizedLines(index: Int): List<String>?
    }

    /** 文字の層があるページはそのまま、無いページは文字認識(最大 [maxOcrPages] ページ)。ページの間は空行。 */
    fun assemble(pages: PdfPages, maxOcrPages: Int = RecordImport.MAX_OCR_PAGES): String {
        val out = mutableListOf<String>()
        var ocr = 0
        for (i in 0 until pages.pageCount) {
            val text = pages.layerText(i)?.trim().orEmpty()
            if (text.isNotEmpty()) {
                out += text
            } else if (ocr < maxOcrPages) {
                val lines = pages.recognizedLines(i) ?: continue
                ocr += 1
                if (lines.isNotEmpty()) out += lines.joinToString("\n")
            }
        }
        return out.joinToString("\n\n")
    }

    /** 文字認識に渡す倍率(72dpi の点から): 長辺 2000px まで・3 倍まで。大きさが無いページは null。 */
    fun renderScale(width: Float, height: Float): Float? {
        if (width <= 0f || height <= 0f) return null
        return minOf(2000f / maxOf(width, height), 3f)
    }
}
