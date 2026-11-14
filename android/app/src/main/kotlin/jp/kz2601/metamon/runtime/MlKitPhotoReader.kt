package jp.kz2601.metamon.runtime

import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import com.google.android.gms.tasks.Task
import com.google.mlkit.vision.barcode.BarcodeScanning
import com.google.mlkit.vision.common.InputImage
import com.google.mlkit.vision.face.FaceDetection
import com.google.mlkit.vision.face.FaceDetectorOptions
import com.google.mlkit.vision.label.ImageLabeling
import com.google.mlkit.vision.label.defaults.ImageLabelerOptions
import com.google.mlkit.vision.text.TextRecognition
import com.google.mlkit.vision.text.japanese.JapaneseTextRecognizerOptions
import jp.kz2601.metamon.dsl.PhotoAnalyze
import jp.kz2601.metamon.dsl.PhotoAnalyzeKind
import jp.kz2601.metamon.renderer.components.media.OcrPiece
import jp.kz2601.metamon.renderer.components.media.PhotoClassifier
import jp.kz2601.metamon.renderer.components.media.PhotoClues
import jp.kz2601.metamon.renderer.components.media.PhotoColor
import jp.kz2601.metamon.renderer.components.media.PhotoReader
import jp.kz2601.metamon.renderer.components.media.needsLabels
import jp.kz2601.metamon.renderer.components.media.needsText
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlin.coroutines.resume

/**
 * photoField.analyze の端末内の読み取り(#1119・iOS の `PhotoVision`)。ML Kit の Play 開発者サービス版だけを使い、
 * 写真は端末の外に出さない・課金しない。部品がまだ落ちていない・失敗した読み取りは空の手がかり(= 読めなかった)にする。
 * 骨格(jointAngle)は ML Kit の姿勢が端末に同梱する版しか無いので読まない。
 */
class MlKitPhotoReader(private val context: Context) : PhotoReader {
    override suspend fun clues(jpeg: ByteArray, specs: List<PhotoAnalyze>): PhotoClues = withContext(Dispatchers.Default) {
        val bitmap = decode(jpeg) ?: return@withContext PhotoClues()
        try {
            val image = InputImage.fromBitmap(bitmap, 0)
            val kinds = specs.map { it.kind }.toSet()
            PhotoClues(
                lines = if (specs.any { it.needsText }) textLines(image) else emptyList(),
                labels = if (specs.any { it.needsLabels }) labels(image) else emptyList(),
                barcodes = if (PhotoAnalyzeKind.BARCODE in kinds) barcodes(image) else emptyList(),
                peopleCount = if (PhotoAnalyzeKind.PEOPLE_COUNT in kinds) peopleCount(image) else null,
                color = if (PhotoAnalyzeKind.COLOR in kinds) PhotoColor.dominant(centerPixels(bitmap)) else null,
            )
        } finally {
            bitmap.recycle()
        }
    }

    /** 文字認識(日本語の認識は英数字も読む)。同じ高さの断片を 1 行にまとめ、上から。 */
    private suspend fun textLines(image: InputImage): List<String> {
        val recognizer = TextRecognition.getClient(JapaneseTextRecognizerOptions.Builder().build())
        return try {
            val text = recognizer.process(image).awaitOrNull() ?: return emptyList()
            val pieces = text.textBlocks.flatMap { it.lines }.mapNotNull { line ->
                val box = line.boundingBox ?: return@mapNotNull null
                OcrPiece(line.text, box.left.toFloat(), box.top.toFloat(), box.width().toFloat(), box.height().toFloat())
            }
            OcrPiece.lines(pieces)
        } finally {
            recognizer.close()
        }
    }

    /** 画像のラベル(表の形の英語・信頼度順・上位 10)。 */
    private suspend fun labels(image: InputImage): List<String> {
        val labeler = ImageLabeling.getClient(ImageLabelerOptions.Builder().setConfidenceThreshold(LABEL_CONFIDENCE).build())
        return try {
            labeler.process(image).awaitOrNull().orEmpty()
                .sortedByDescending { it.confidence }
                .take(10)
                .map { PhotoClassifier.key(it.text) }
        } finally {
            labeler.close()
        }
    }

    /** バーコード・QR の中身(見つかった順・前後の空白を落とす)。 */
    private suspend fun barcodes(image: InputImage): List<String> {
        val scanner = BarcodeScanning.getClient()
        return try {
            scanner.process(image).awaitOrNull().orEmpty().mapNotNull { it.rawValue?.trim()?.takeIf(String::isNotEmpty) }
        } finally {
            scanner.close()
        }
    }

    /** 顔の数(特徴・表情は取らない・誰かは見分けない)。失敗したら null。 */
    private suspend fun peopleCount(image: InputImage): Int? {
        val detector = FaceDetection.getClient(
            FaceDetectorOptions.Builder().setPerformanceMode(FaceDetectorOptions.PERFORMANCE_MODE_FAST).build(),
        )
        return try {
            detector.process(image).awaitOrNull()?.size
        } finally {
            detector.close()
        }
    }

    /** 値の写真は PhotoCodec が正立・長い辺 800px に縮めて保存しているので、そのまま開く。 */
    private fun decode(jpeg: ByteArray): Bitmap? = BitmapFactory.decodeByteArray(jpeg, 0, jpeg.size)

    companion object {
        private const val LABEL_CONFIDENCE = 0.5f
        private const val COLOR_SIDE = 32

        /** 写真の中央 60% を 32×32 に縮めた画素(0xRRGGBB)。主な色を数える元(iOS の `centerPixels`)。 */
        internal fun centerPixels(bitmap: Bitmap): List<Int> {
            val w = bitmap.width
            val h = bitmap.height
            val cw = (w * 0.6).toInt()
            val ch = (h * 0.6).toInt()
            if (cw <= 0 || ch <= 0) return emptyList()
            val center = Bitmap.createBitmap(bitmap, (w * 0.2).toInt(), (h * 0.2).toInt(), cw, ch)
            val small = Bitmap.createScaledBitmap(center, COLOR_SIDE, COLOR_SIDE, true)
            val pixels = IntArray(COLOR_SIDE * COLOR_SIDE)
            small.getPixels(pixels, 0, COLOR_SIDE, 0, 0, COLOR_SIDE, COLOR_SIDE)
            if (small !== center) small.recycle()
            if (center !== bitmap) center.recycle()
            return pixels.map { it and 0xFFFFFF }
        }
    }
}

/** ML Kit の Task を待つ。失敗(部品がまだ落ちていない等)は null。 */
private suspend fun <T> Task<T>.awaitOrNull(): T? = suspendCancellableCoroutine { cont ->
    addOnCompleteListener { t -> cont.resume(if (t.isSuccessful) t.result else null) }
}
