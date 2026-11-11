package jp.kz2601.metamon.renderer.components.media

import jp.kz2601.metamon.dsl.PhotoAnalyze
import jp.kz2601.metamon.dsl.PhotoAnalyzeKind
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import java.time.ZoneId
import java.time.ZonedDateTime
import java.time.format.DateTimeFormatter
import java.util.Locale
import kotlin.math.roundToLong

/**
 * `photoField.analyze`(iOS の `PhotoReader`)。iOS は端末内(Vision)で読み、写真を端末の外に出さない・課金しない。
 * Android も同じく端末内(ML Kit・:app の `MlKitPhotoReader`)で読み、[jp.kz2601.metamon.renderer.DSLRuntime.photoReader]
 * で渡す。無ければ analyze は無視して写真の入力として動く。
 */
interface PhotoReader {
    /** JPEG 1 枚から、[specs] が要る手がかりだけを取り出す。 */
    suspend fun clues(jpeg: ByteArray, specs: List<PhotoAnalyze>): PhotoClues
}

/** 1 枚の写真から取り出した手がかり(iOS の `PhotoClues` のうち、いま値にできるもの)。 */
data class PhotoClues(
    /** 文字認識の行(上から) */
    val lines: List<String> = emptyList(),
    /** 画像分類のラベル(英語・小文字・信頼度順) */
    val labels: List<String> = emptyList(),
    /** バーコード・QR の中身(見つかった順) */
    val barcodes: List<String> = emptyList(),
    val peopleCount: Int? = null,
    /** 主な色の名前 */
    val color: String? = null,
)

/** 読み取りの結果。values は into の field id → 保存する形の値。 */
data class PhotoReadOutcome(val values: Map<String, JSONValue> = emptyMap(), val missed: List<String> = emptyList())

/** 文字認識(OCR)が要るか(iOS の `needsText`)。 */
val PhotoAnalyze.needsText: Boolean
    get() = kind in setOf(PhotoAnalyzeKind.TEXT, PhotoAnalyzeKind.AMOUNT, PhotoAnalyzeKind.DATE, PhotoAnalyzeKind.CLASSIFY, PhotoAnalyzeKind.DESCRIBE)

/** 画像分類のラベルが要るか(iOS の `needsLabels`)。 */
val PhotoAnalyze.needsLabels: Boolean
    get() = kind in setOf(PhotoAnalyzeKind.CLASSIFY, PhotoAnalyzeKind.DESCRIBE, PhotoAnalyzeKind.LABEL, PhotoAnalyzeKind.LABELS)

object PhotoReading {
    /** いま値にできる kind。jointAngle は骨格の読み取り(ML Kit の姿勢は端末に同梱する大きい部品)が無いので読めなかった扱い。 */
    val supportedKinds = PhotoAnalyzeKind.entries.toSet() - setOf(PhotoAnalyzeKind.JOINT_ANGLE, PhotoAnalyzeKind.UNKNOWN)

    /** describe のラベルの前置き(iOS の `L("写っているもの: ")`)。 */
    const val DESCRIBE_PREFIX = "写っているもの: "

    /** 手がかり → 各 into の値。読めなかった・型に入らない項目は missed(今の値のまま)。 */
    fun apply(clues: PhotoClues, specs: List<PhotoAnalyze>, entity: Entity, zone: ZoneId = ZoneId.systemDefault()): PhotoReadOutcome {
        val values = linkedMapOf<String, JSONValue>()
        val missed = mutableListOf<String>()
        for (spec in specs) {
            val field = entity.field(spec.into)
            val v = field?.let { jsonValue(reading(spec, it.options, clues), it.type, zone) }
            if (v == null) missed += spec.into else values[spec.into] = v
        }
        return PhotoReadOutcome(values, missed)
    }

    /** 端末内 AI を使わない読み方(iOS で Apple Intelligence が無い時と同じ)。 */
    internal fun reading(spec: PhotoAnalyze, fieldOptions: List<String>?, clues: PhotoClues): Any? {
        val text = clues.lines.joinToString("\n")
        val labels = clues.labels
        return when (spec.kind) {
            PhotoAnalyzeKind.TEXT -> text.takeIf { it.isNotEmpty() }
            PhotoAnalyzeKind.AMOUNT -> ReceiptParser.amount(clues.lines)
            PhotoAnalyzeKind.DATE -> ReceiptParser.date(clues.lines)
            PhotoAnalyzeKind.CLASSIFY -> {
                val options = spec.options.ifEmpty { fieldOptions.orEmpty() }
                if (options.size < 2 || (labels.isEmpty() && text.isEmpty())) null else PhotoClassifier.classify(options, labels, text)
            }
            PhotoAnalyzeKind.DESCRIBE -> when {
                labels.isNotEmpty() -> DESCRIBE_PREFIX + labels.take(5).map(PhotoClassifier::japanese).joinToString("、")
                text.isNotEmpty() -> text.take(200)
                else -> null
            }
            PhotoAnalyzeKind.LABEL -> PhotoClassifier.names(labels, 1).firstOrNull()
            PhotoAnalyzeKind.LABELS -> PhotoClassifier.names(labels, 5).takeIf { it.isNotEmpty() }?.joinToString("、")
            PhotoAnalyzeKind.BARCODE -> clues.barcodes.firstOrNull()
            PhotoAnalyzeKind.PEOPLE_COUNT -> clues.peopleCount?.toDouble()
            PhotoAnalyzeKind.COLOR -> clues.color
            PhotoAnalyzeKind.JOINT_ANGLE, PhotoAnalyzeKind.UNKNOWN -> null
        }
    }

    private val isoDateTime = DateTimeFormatter.ofPattern("yyyy-MM-dd'T'HH:mm:ssXXX")

    /** 読んだ値を into の型の保存形式に(iOS の `PhotoReadingValue.jsonValue`)。入らない組み合わせは null。 */
    fun jsonValue(reading: Any?, type: FieldType, zone: ZoneId = ZoneId.systemDefault()): JSONValue? = when {
        reading is String && type in setOf(FieldType.STRING, FieldType.TEXT, FieldType.ENUM) ->
            reading.trim().takeIf { it.isNotEmpty() }?.let { JSONValue.Str(it) }
        reading is Double && (type == FieldType.INTEGER || type == FieldType.CURRENCY) -> JSONValue.Num(reading.roundToLong().toDouble())
        reading is Double && type == FieldType.NUMBER -> JSONValue.Num((reading * 10).roundToLong() / 10.0)
        reading is ReadDate && type == FieldType.DATE -> JSONValue.Str(String.format(Locale.ROOT, "%04d-%02d-%02d", reading.year, reading.month, reading.day))
        reading is ReadDate && type == FieldType.DATETIME -> JSONValue.Str(
            ZonedDateTime.of(reading.year, reading.month, reading.day, reading.hour ?: 0, reading.minute ?: 0, 0, 0, zone).format(isoDateTime),
        )
        else -> null
    }
}
