package jp.kz2601.metamon.renderer.components.importing

import android.content.Context
import com.google.android.gms.tasks.Tasks
import com.google.mlkit.vision.common.InputImage
import com.google.mlkit.vision.text.TextRecognition
import com.google.mlkit.vision.text.TextRecognizer
import com.google.mlkit.vision.text.japanese.JapaneseTextRecognizerOptions
import com.tom_roush.pdfbox.android.PDFBoxResourceLoader
import com.tom_roush.pdfbox.pdmodel.PDDocument
import com.tom_roush.pdfbox.rendering.PDFRenderer
import com.tom_roush.pdfbox.text.PDFTextStripper
import java.util.concurrent.TimeUnit

/**
 * PDF の本文を端末内で取り出す(#1181・iOS の PDFKit + Vision)。文字の層は PdfBox-Android、スキャンのページは描画して
 * ML Kit の文字認識(日本語・端末内。認識の部品は Play 開発者サービスが落とす)。メモリで読むだけで、保存・送信しない。
 * 重いので main 以外のスレッドで呼ぶ。読めない・開くパスワードが要るものは null。
 */
object PdfTextReader {
    private const val OCR_TIMEOUT_SECONDS = 30L

    fun text(context: Context, bytes: ByteArray): String? {
        PDFBoxResourceLoader.init(context.applicationContext)
        val doc = open(bytes) ?: return null
        val recognizer = lazy { TextRecognition.getClient(JapaneseTextRecognizerOptions.Builder().build()) }
        return try {
            PdfText.assemble(Pages(doc) { recognize(recognizer.value, it) })
        } catch (_: Exception) {
            null
        } finally {
            if (recognizer.isInitialized()) recognizer.value.close()
            runCatching { doc.close() }
        }
    }

    /**
     * 空のパスワードで開く(#1319)。印刷・コピーの制限だけの暗号化(開くパスワードが無い)は読む。開くパスワードが要るもの
     * (iOS の isLocked)・壊れたものは null。標準の暗号(RC4 / AES)は端末の javax.crypto で解くので BouncyCastle は要らない。
     */
    internal fun open(bytes: ByteArray): PDDocument? = runCatching { PDDocument.load(bytes, "") }.getOrNull()

    /** 認識できない(部品が落とせない・時間切れ)時は空(そのページは飛ばす)。 */
    private fun recognize(recognizer: TextRecognizer, image: InputImage): List<String> =
        runCatching {
            Tasks.await(recognizer.process(image), OCR_TIMEOUT_SECONDS, TimeUnit.SECONDS)
                .textBlocks.flatMap { b -> b.lines.map { it.text.trim() } }.filter { it.isNotEmpty() }
        }.getOrDefault(emptyList())

    private class Pages(private val doc: PDDocument, private val ocr: (InputImage) -> List<String>) : PdfText.PdfPages {
        private val stripper = PDFTextStripper()
        private val renderer by lazy { PDFRenderer(doc) }

        override val pageCount: Int get() = doc.numberOfPages

        override fun layerText(index: Int): String? = runCatching {
            stripper.startPage = index + 1
            stripper.endPage = index + 1
            stripper.getText(doc)
        }.getOrNull()

        override fun recognizedLines(index: Int): List<String>? {
            val box = doc.getPage(index).mediaBox
            val scale = PdfText.renderScale(box.width, box.height) ?: return null
            val bitmap = runCatching { renderer.renderImage(index, scale) }.getOrNull() ?: return null
            return try {
                ocr(InputImage.fromBitmap(bitmap, 0))
            } finally {
                bitmap.recycle()
            }
        }
    }
}
