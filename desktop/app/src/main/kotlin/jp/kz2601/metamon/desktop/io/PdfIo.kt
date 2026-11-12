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

    /** 頁の画像の倍率(A4 の pt × SCALE = 画素)。 */
    const val SCALE = 2f
    private const val MARGIN = 36f

    /**
     * 表を A4 縦の PDF にする。日本語のフォントを PDF に埋め込む代わりに、OS のフォント(Java の論理フォント)で描いた画像を頁に貼る
     * (Windows・Linux で入っている CJK のフォントが違っても化けない)。文字は選べない。
     */
    fun data(table: ExportTable, emptyText: String = "記録がありません"): ByteArray = pdf(pages(table, emptyText).map { it.image })

    /** 書き出しの 1 頁: A4 を [SCALE] 倍で描いた画像と、その頁に載る行(`table.rows` の添字)。印刷(Print.kt)も同じ頁を使う。 */
    class Page(val image: BufferedImage, val rows: IntRange)

    /** 表を A4 の頁に割って描く。PDF の書き出しと印刷が同じ頁を使う。 */
    fun pages(table: ExportTable, emptyText: String = "記録がありません"): List<Page> {
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
        val starts = mutableListOf<Int>()
        var img: BufferedImage? = null
        var g: Graphics2D? = null
        var y = 0
        var next = 0

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
            starts += next
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
            next++
        }
        g?.dispose()
        check(img != null)
        return pages.mapIndexed { i, p -> Page(p, starts[i] until (starts.getOrNull(i + 1) ?: next)) }
    }

    /** 描いた頁の画像を A4 の PDF にする(1 枚 = 1 頁・頁いっぱいに貼る)。 */
    fun pdf(images: List<BufferedImage>): ByteArray {
        val page = PDRectangle.A4
        PDDocument().use { doc ->
            for (p in images) {
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

    /**
     * 列の幅に収まるよう末尾を「…」で切る。長いセルも速く([CLIP_MAX_CHARS] 字で先に切り、切る位置は二分探索)。
     * サロゲートペアの間では切らない。
     */
    internal fun clip(s: String, width: Int, measure: (String) -> Int): String {
        val head = if (s.length > CLIP_MAX_CHARS) s.substring(0, CLIP_MAX_CHARS) else s
        if (head.length == s.length && measure(s) <= width) return s
        // measure(head[0, end) + "…") <= width となる一番長い end
        var lo = 0
        var hi = head.length
        while (lo < hi) {
            val mid = (lo + hi + 1) / 2
            if (measure(head.substring(0, mid) + "…") <= width) lo = mid else hi = mid - 1
        }
        if (lo > 0 && Character.isHighSurrogate(head[lo - 1])) lo--
        return head.substring(0, lo) + "…"
    }

    /** 1 マスに描く最大の字数(A4 の 1 列にこれより多くは入らない)。 */
    internal const val CLIP_MAX_CHARS = 200
}
