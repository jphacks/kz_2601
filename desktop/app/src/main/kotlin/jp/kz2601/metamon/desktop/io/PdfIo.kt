package jp.kz2601.metamon.desktop.io

import jp.kz2601.metamon.dsl.logic.ExportTable
import jp.kz2601.metamon.dsl.logic.RecordImport
import org.apache.pdfbox.Loader
import org.apache.pdfbox.pdmodel.PDDocument
import org.apache.pdfbox.pdmodel.PDPage
import org.apache.pdfbox.pdmodel.PDPageContentStream
import org.apache.pdfbox.pdmodel.common.PDRectangle
import org.apache.pdfbox.pdmodel.encryption.InvalidPasswordException
import org.apache.pdfbox.pdmodel.graphics.image.LosslessFactory
import org.apache.pdfbox.text.PDFTextStripper
import java.awt.Color
import java.awt.Font
import java.awt.Graphics2D
import java.awt.RenderingHints
import java.awt.image.BufferedImage
import java.io.ByteArrayOutputStream

/** PDF の取り込み(本文を読む)と書き出し(表)。Apache PDFBox(JVM 版)。 */
object PdfIo {
    sealed interface TextResult {
        data class Text(val text: String) : TextResult

        /** パスワードが要る(iOS の `isLocked`・#1319) */
        data object Locked : TextResult

        /** 文字が入っていない(スキャン)。デスクトップ版は文字認識をしない */
        data object NoText : TextResult

        data object TooLarge : TextResult

        data object Broken : TextResult
    }

    /** 本文(先頭から)。暗号化でも空のパスワードで開けて抜き出しが許されていれば読む。 */
    fun text(bytes: ByteArray): TextResult {
        if (bytes.size > RecordImport.MAX_PDF_BYTES) return TextResult.TooLarge
        val doc = try {
            Loader.loadPDF(bytes)
        } catch (_: InvalidPasswordException) {
            return TextResult.Locked
        } catch (_: Exception) {
            return TextResult.Broken
        }
        return doc.use {
            if (it.isEncrypted && !it.currentAccessPermission.canExtractContent()) return TextResult.Locked
            val text = runCatching { PDFTextStripper().getText(it) }.getOrNull() ?: return TextResult.Broken
            if (text.isBlank()) TextResult.NoText else TextResult.Text(text)
        }
    }

    private const val SCALE = 2f
    private const val MARGIN = 36f

    /**
     * 表を A4 縦の PDF にする。日本語のフォントを PDF に埋め込む代わりに、OS のフォント(Java の論理フォント)で描いた画像を頁に貼る
     * (Windows・Linux で入っている CJK のフォントが違っても化けない)。文字は選べない。
     */
    fun data(table: ExportTable, emptyText: String = "記録がありません"): ByteArray {
        val page = PDRectangle.A4
        val w = (page.width * SCALE).toInt()
        val h = (page.height * SCALE).toInt()
        val m = (MARGIN * SCALE).toInt()
        val body = Font(Font.SANS_SERIF, Font.PLAIN, (9 * SCALE).toInt())
        val bold = body.deriveFont(Font.BOLD)
        val titleFont = Font(Font.SANS_SERIF, Font.BOLD, (16 * SCALE).toInt())
        val noteFont = body.deriveFont(8 * SCALE)
        val lineH = (16 * SCALE).toInt()
        val cols = table.headers.size.coerceAtLeast(1)
        val colW = (w - 2 * m) / cols

        val pages = mutableListOf<BufferedImage>()
        var img: BufferedImage? = null
        var g: Graphics2D? = null
        var y = 0

        fun newPage(first: Boolean) {
            g?.dispose()
            val i = BufferedImage(w, h, BufferedImage.TYPE_INT_RGB)
            val gg = i.createGraphics()
            gg.setRenderingHint(RenderingHints.KEY_TEXT_ANTIALIASING, RenderingHints.VALUE_TEXT_ANTIALIAS_ON)
            gg.setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON)
            gg.color = Color.WHITE
            gg.fillRect(0, 0, w, h)
            gg.color = Color.BLACK
            pages += i
            img = i
            g = gg
            y = m
            if (first) {
                gg.font = titleFont
                y += gg.fontMetrics.ascent
                gg.drawString(table.title, m, y)
                y += lineH / 2
                gg.font = noteFont
                table.notes.forEach { n ->
                    y += lineH
                    gg.color = Color.DARK_GRAY
                    gg.drawString(n, m, y)
                }
                gg.color = Color.BLACK
                y += lineH
            }
            if (table.headers.isNotEmpty()) {
                gg.font = bold
                y += lineH
                drawRow(gg, table.headers, emptyList(), m, y, colW)
                gg.drawLine(m, y + lineH / 3, w - m, y + lineH / 3)
            }
            gg.font = body
        }

        newPage(first = true)
        if (table.rows.isEmpty()) {
            y += lineH
            g!!.color = Color.GRAY
            g!!.drawString(emptyText, m, y)
        }
        for (r in table.rows) {
            if (y + lineH > h - m) newPage(first = false)
            y += lineH
            drawRow(g!!, r, table.numericColumns, m, y, colW)
        }
        g?.dispose()
        check(img != null)

        PDDocument().use { doc ->
            for (p in pages) {
                val pdPage = PDPage(page)
                doc.addPage(pdPage)
                val image = LosslessFactory.createFromImage(doc, p)
                PDPageContentStream(doc, pdPage).use { it.drawImage(image, 0f, 0f, page.width, page.height) }
            }
            return ByteArrayOutputStream().also { doc.save(it) }.toByteArray()
        }
    }

    private fun drawRow(g: Graphics2D, cells: List<String>, numeric: List<Boolean>, x0: Int, y: Int, colW: Int) {
        val fm = g.fontMetrics
        val pad = (4 * SCALE).toInt()
        cells.forEachIndexed { i, raw ->
            val text = clip(raw.replace('\n', ' ').replace('\r', ' '), colW - 2 * pad) { fm.stringWidth(it) }
            val x = x0 + i * colW + if (numeric.getOrElse(i) { false }) colW - pad - fm.stringWidth(text) else 0
            g.drawString(text, x, y)
        }
    }

    /** 列の幅に収まるよう末尾を「…」で切る。 */
    internal fun clip(s: String, width: Int, measure: (String) -> Int): String {
        if (measure(s) <= width) return s
        var end = s.length
        while (end > 0 && measure(s.substring(0, end) + "…") > width) end--
        return s.substring(0, end) + "…"
    }
}
