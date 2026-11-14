package jp.kz2601.metamon.runtime

import android.graphics.Color
import android.graphics.Paint
import android.graphics.Typeface
import android.graphics.pdf.PdfDocument
import jp.kz2601.metamon.dsl.logic.ExportTable
import java.io.ByteArrayOutputStream

/** A4 縦の簡単な報告: 見出し・補足・表(見出し行はページごとに繰り返す)。iOS の `ExportPDF`(グラフの画像はまだ載せない)。 */
object ExportPdf {
    private const val PAGE_W = 595
    private const val PAGE_H = 842
    private const val MARGIN = 40f
    private const val CONTENT_W = PAGE_W - MARGIN * 2
    private const val ROW_H = 20f
    private const val CELL_PAD = 4f

    fun data(table: ExportTable, emptyText: String): ByteArray {
        val doc = PdfDocument()
        try {
            val title = paint(20f, bold = true)
            val note = paint(10f, color = Color.GRAY)
            val cell = paint(10f)
            val head = paint(10f, bold = true)
            val pageNoPaint = paint(9f, color = Color.GRAY).apply { textAlign = Paint.Align.CENTER }
            val headerFill = Paint().apply { color = Color.rgb(0xEE, 0xEE, 0xF0) }
            val separator = Paint().apply { color = Color.rgb(0xDD, 0xDD, 0xE0) }

            var pageNo = 0
            var page: PdfDocument.Page? = null
            var y = MARGIN
            fun newPage() {
                page?.let(doc::finishPage)
                pageNo += 1
                page = doc.startPage(PdfDocument.PageInfo.Builder(PAGE_W, PAGE_H, pageNo).create())
                page!!.canvas.drawText("$pageNo", PAGE_W / 2f, PAGE_H - MARGIN + 20f, pageNoPaint)
                y = MARGIN
            }
            newPage()

            y += 20f
            page!!.canvas.drawText(fit(table.title, title, CONTENT_W), MARGIN, y, title)
            y += 12f
            for (n in table.notes) {
                y += 14f
                page!!.canvas.drawText(fit(n, note, CONTENT_W), MARGIN, y, note)
            }
            y += 14f

            if (table.rows.isEmpty()) {
                page!!.canvas.drawText(emptyText, MARGIN, y + 14f, note)
            } else {
                val widths = columnWidths(table, cell)
                fun header() {
                    page!!.canvas.drawRect(MARGIN, y, MARGIN + CONTENT_W, y + ROW_H, headerFill)
                    drawRow(page!!, table.headers, y, widths, table.numericColumns, head)
                    y += ROW_H
                }
                if (y + ROW_H * 2 > PAGE_H - MARGIN) newPage()
                header()
                for (row in table.rows) {
                    if (y + ROW_H > PAGE_H - MARGIN) {
                        newPage()
                        header()
                    }
                    drawRow(page!!, row, y, widths, table.numericColumns, cell)
                    y += ROW_H
                    page!!.canvas.drawRect(MARGIN, y - 0.5f, MARGIN + CONTENT_W, y, separator)
                }
            }
            page?.let(doc::finishPage)
            return ByteArrayOutputStream().use { out ->
                doc.writeTo(out)
                out.toByteArray()
            }
        } finally {
            doc.close()
        }
    }

    /** 内容の長さに応じた列幅(見出しと先頭 50 行で測る)。合計を本文幅に合わせ、細すぎる列は 36pt を下限に。 */
    private fun columnWidths(table: ExportTable, paint: Paint): List<Float> {
        val natural = table.headers.indices.map { i ->
            val cells = listOf(table.headers[i]) + table.rows.take(50).map { it.getOrElse(i) { "" } }
            val w = cells.maxOf { paint.measureText(it) }
            (w + CELL_PAD * 2).coerceIn(36f, 220f)
        }
        val total = natural.sum().takeIf { it > 0f } ?: return natural
        return natural.map { it * CONTENT_W / total }
    }

    private fun drawRow(page: PdfDocument.Page, cells: List<String>, y: Float, widths: List<Float>, numeric: List<Boolean>, paint: Paint) {
        var x = MARGIN
        val baseline = y + ROW_H - 6f
        for ((i, w) in widths.withIndex()) {
            val text = fit(cells.getOrElse(i) { "" }.replace('\n', ' '), paint, w - CELL_PAD * 2)
            if (numeric.getOrElse(i) { false }) {
                page.canvas.drawText(text, x + w - CELL_PAD - paint.measureText(text), baseline, paint)
            } else {
                page.canvas.drawText(text, x + CELL_PAD, baseline, paint)
            }
            x += w
        }
    }

    /** 1 行に収まらなければ末尾を「…」に。 */
    private fun fit(text: String, paint: Paint, width: Float): String {
        if (paint.measureText(text) <= width) return text
        val n = paint.breakText(text, true, (width - paint.measureText("…")).coerceAtLeast(0f), null)
        return text.take(n) + "…"
    }

    private fun paint(size: Float, bold: Boolean = false, color: Int = Color.BLACK) = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        textSize = size
        this.color = color
        typeface = if (bold) Typeface.DEFAULT_BOLD else Typeface.DEFAULT
    }
}
